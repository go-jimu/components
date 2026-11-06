package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-jimu/components/encoding"
)

// Decoder decodes one KeyValue into the map for that entry, which is then
// merged; see WithDecoder for the default.
type Decoder func(*KeyValue, map[string]interface{}) error

// Resolver resolves placeholders in the merged configuration, in place. See
// "Placeholders".
type Resolver func(map[string]interface{}) error

// Option configures New.
type Option func(*options)

// Merge merges src, a decoded entry of type map[string]interface{}, into dst,
// a *map[string]interface{}. See "Sources" for the default.
type Merge func(dst, src interface{}) error

// Lookup returns the value of a placeholder name from outside the
// configuration, such as an environment variable, and reports whether name
// was found. See "Placeholders".
type Lookup func(name string) (string, bool)

type options struct {
	sources     []Source
	decoder     Decoder
	resolver    Resolver
	merge       Merge
	lookup      Lookup
	actualTypes bool
}

// WithSource sets the sources, lowest priority first, replacing any set
// before. See "Sources".
func WithSource(s ...Source) Option {
	return func(o *options) {
		o.sources = s
	}
}

// WithDecoder replaces the decoder that turns each loaded KeyValue into
// configuration entries. The default decoder decodes Value with the encoding
// codec named by Format into the configuration map; when Format is empty, it
// stores Value under Key, splitting a dotted Key into nested maps.
func WithDecoder(d Decoder) Option {
	return func(o *options) {
		o.decoder = d
	}
}

// WithResolveActualTypes selects the built-in resolver, replacing one set by
// an earlier WithResolver. With enableConvertToType, a value that is exactly
// one placeholder takes the type of its resolved text. See "Placeholders".
func WithResolveActualTypes(enableConvertToType bool) Option {
	return func(o *options) {
		o.resolver = nil
		o.actualTypes = enableConvertToType
	}
}

// WithResolver replaces the built-in placeholder resolver, which ignores
// WithLookup from then on. See "Placeholders".
func WithResolver(r Resolver) Option {
	return func(o *options) {
		o.resolver = r
	}
}

// WithLookup sets the Lookup the built-in resolver consults first for each
// placeholder. See "Placeholders".
func WithLookup(lookup Lookup) Option {
	return func(o *options) {
		o.lookup = lookup
	}
}

// WithMergeFunc replaces the Merge that merges each decoded entry over the
// earlier ones; the default is described in "Sources".
func WithMergeFunc(m Merge) Option {
	return func(o *options) {
		o.merge = m
	}
}

// defaultDecoder decode config from source KeyValue
// to target map[string]interface{} using src.Format codec.
func defaultDecoder(src *KeyValue, target map[string]interface{}) error {
	if src.Format == "" {
		// expand key "aaa.bbb" into map[aaa]map[bbb]interface{}
		keys := strings.Split(src.Key, ".")
		for i, k := range keys {
			if i == len(keys)-1 {
				target[k] = src.Value
			} else {
				sub := make(map[string]interface{})
				target[k] = sub
				target = sub
			}
		}
		return nil
	}
	if codec := encoding.GetCodec(src.Format); codec != nil {
		return codec.Unmarshal(src.Value, &target)
	}
	return fmt.Errorf("unsupported key: %s format: %s", src.Key, src.Format)
}
func newActualTypesResolver(enableConvertToType bool) func(map[string]interface{}) error {
	return newResolver(nil, enableConvertToType)
}

// defaultResolver resolve placeholder in map value,
// placeholder format in ${key:default}.
func defaultResolver(input map[string]interface{}) error {
	return resolver(input, mapper(input, nil), false)
}

func newResolver(lookup Lookup, toType bool) Resolver {
	return func(input map[string]interface{}) error {
		return resolver(input, mapper(input, lookup), toType)
	}
}

func resolver(input map[string]interface{}, mapper func(name string) string, toType bool) error {
	var resolve func(map[string]interface{}) error
	resolve = func(sub map[string]interface{}) error {
		for k, v := range sub {
			switch vt := v.(type) {
			case string:
				sub[k] = expand(vt, mapper, toType)
			case map[string]interface{}:
				if err := resolve(vt); err != nil {
					return err
				}
			case []interface{}:
				for i, iface := range vt {
					switch it := iface.(type) {
					case string:
						vt[i] = expand(it, mapper, toType)
					case map[string]interface{}:
						if err := resolve(it); err != nil {
							return err
						}
					}
				}
				sub[k] = vt
			}
		}
		return nil
	}
	return resolve(input)
}

func mapper(input map[string]interface{}, lookup Lookup) func(name string) string {
	mapper := func(name string) string {
		args := strings.SplitN(strings.TrimSpace(name), ":", 2)
		if lookup != nil {
			if v, ok := lookup(args[0]); ok {
				return v
			}
		}
		if v, has := readValue(input, args[0]); has {
			s, _ := v.String()
			return s
		} else if len(args) > 1 { // default value
			return args[1]
		}
		return ""
	}
	return mapper
}

func convertToType(input string) interface{} {
	// Check if the input is a string with quotes
	if strings.HasPrefix(input, "\"") && strings.HasSuffix(input, "\"") {
		// Trim the quotes and return the string value
		return strings.Trim(input, "\"")
	}

	// Try converting to bool
	if input == "true" || input == "false" {
		b, _ := strconv.ParseBool(input)
		return b
	}

	// Try converting to float64
	if strings.Contains(input, ".") {
		if f, err := strconv.ParseFloat(input, 64); err == nil {
			return f
		}
	}

	// Try converting to int64
	if i, err := strconv.ParseInt(input, 10, 64); err == nil {
		return i
	}

	// Default to string if no other conversion succeeds
	return input
}

var placeholderRegexp = regexp.MustCompile(`\${(.*?)}`)

// expand replaces ${name:default} placeholders in s. With toType, a value that
// is exactly one placeholder is converted to its actual type; placeholders
// embedded in a longer string are interpolated as strings.
func expand(s string, mapping func(string) string, toType bool) interface{} {
	if toType {
		if m := placeholderRegexp.FindStringSubmatch(s); m != nil && m[0] == s {
			return convertToType(mapping(m[1]))
		}
	}
	return placeholderRegexp.ReplaceAllStringFunc(s, func(placeholder string) string {
		m := mapping(placeholderRegexp.FindStringSubmatch(placeholder)[1])
		if toType {
			// Quotes only force the string type; they are not part of the text.
			if str, ok := convertToType(m).(string); ok {
				return str
			}
		}
		return m
	})
}
