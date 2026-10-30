package config

import (
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	durationType        = reflect.TypeOf(time.Duration(0))
	jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// scanJSON unmarshals data into v after coercing values to the kinds of the
// target fields. Placeholders and env values always resolve to strings, so
// "8080" must become a number for an int field and "5s" a duration for a
// time.Duration field. Values that cannot be coerced are left as they are and
// reported by json.Unmarshal.
func scanJSON(data []byte, v interface{}) error {
	target := reflect.TypeOf(v)
	if target == nil || target.Kind() != reflect.Ptr {
		return json.Unmarshal(data, v)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw interface{}
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	coerced, err := json.Marshal(coerce(raw, target))
	if err != nil {
		return err
	}
	return json.Unmarshal(coerced, v)
}

func coerce(value interface{}, t reflect.Type) interface{} {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == durationType {
		return coerceDuration(value)
	}
	// Types with their own decoding, such as time.Time, keep the raw value.
	if reflect.PointerTo(t).Implements(jsonUnmarshalerType) || reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return value
	}

	switch t.Kind() {
	case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
		return coerceComposite(value, t)
	default:
		return coerceScalar(value, t.Kind())
	}
}

func coerceScalar(value interface{}, kind reflect.Kind) interface{} {
	switch kind {
	case reflect.Bool:
		if s, ok := value.(string); ok {
			if b, err := strconv.ParseBool(strings.TrimSpace(s)); err == nil {
				return b
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		if s, ok := value.(string); ok {
			if s = strings.TrimSpace(s); isJSONNumber(s) {
				return json.Number(s)
			}
		}
	case reflect.String:
		switch val := value.(type) {
		case json.Number:
			return val.String()
		case bool:
			return strconv.FormatBool(val)
		}
	default:
	}
	return value
}

// isJSONNumber reports whether s is a JSON number literal. NaN and Inf parse
// as floats but have no JSON form, so they stay strings for json.Unmarshal to
// report.
func isJSONNumber(s string) bool {
	if s == "" || (s[0] != '-' && (s[0] < '0' || s[0] > '9')) {
		return false
	}
	return json.Valid([]byte(s))
}

func coerceComposite(value interface{}, t reflect.Type) interface{} {
	switch t.Kind() {
	case reflect.Struct:
		if m, ok := value.(map[string]interface{}); ok {
			return coerceStruct(m, t)
		}
	case reflect.Map:
		if m, ok := value.(map[string]interface{}); ok {
			dst := make(map[string]interface{}, len(m))
			for k, v := range m {
				dst[k] = coerce(v, t.Elem())
			}
			return dst
		}
	case reflect.Slice, reflect.Array:
		if s, ok := value.([]interface{}); ok {
			dst := make([]interface{}, len(s))
			for i, v := range s {
				dst[i] = coerce(v, t.Elem())
			}
			return dst
		}
	default:
	}
	return value
}

func coerceDuration(value interface{}) interface{} {
	s, ok := value.(string)
	if !ok {
		return value
	}
	s = strings.TrimSpace(s)
	if d, err := time.ParseDuration(s); err == nil {
		return int64(d)
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return value
}

func coerceStruct(m map[string]interface{}, t reflect.Type) map[string]interface{} {
	fields := jsonFields(t)
	dst := make(map[string]interface{}, len(m))
	for k, v := range m {
		if f, ok := matchField(fields, k); ok {
			dst[k] = f.coerce(v)
			continue
		}
		dst[k] = v
	}
	return dst
}

type jsonField struct {
	name      string
	typ       reflect.Type
	depth     int
	tagged    bool
	stringOpt bool
}

// coerce converts v for the field. A field tagged ",string" takes its scalar
// value quoted, so numbers and bools become strings and strings stay as they
// are.
func (f jsonField) coerce(v interface{}) interface{} {
	if f.stringOpt {
		return coerceScalar(v, reflect.String)
	}
	return coerce(v, f.typ)
}

// jsonFields lists the fields encoding/json would decode into, including
// fields promoted from untagged embedded structs, and keeps only the dominant
// field for each name.
func jsonFields(t reflect.Type) []jsonField {
	return dominantFields(collectFields(t, 0, map[reflect.Type]bool{}))
}

func collectFields(t reflect.Type, depth int, path map[reflect.Type]bool) []jsonField {
	if path[t] {
		return nil
	}
	path[t] = true
	defer delete(path, t)

	var fields []jsonField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if f.Anonymous && name == "" && ft.Kind() == reflect.Struct {
			fields = append(fields, collectFields(ft, depth+1, path)...)
			continue
		}
		if !f.IsExported() {
			continue
		}
		field := jsonField{name: name, typ: f.Type, depth: depth, tagged: name != ""}
		if !field.tagged {
			field.name = f.Name
		}
		field.stringOpt = hasStringOption(opts, ft.Kind())
		fields = append(fields, field)
	}
	return fields
}

// hasStringOption reports whether encoding/json applies the ",string" option,
// which it honors only for scalar fields.
func hasStringOption(opts string, kind reflect.Kind) bool {
	// reflect.Bool through reflect.Float64 are the bool, integer, and float kinds.
	scalar := (kind >= reflect.Bool && kind <= reflect.Float64) || kind == reflect.String
	return scalar && slices.Contains(strings.Split(opts, ","), "string")
}

// dominantFields applies the encoding/json rule for fields sharing a name: the
// shallowest wins, a tagged field beats untagged ones at the same depth, and a
// tie leaves the name to no field. Field order is kept.
func dominantFields(fields []jsonField) []jsonField {
	byName := make(map[string][]jsonField)
	for _, f := range fields {
		byName[f.name] = append(byName[f.name], f)
	}
	dominant := make([]jsonField, 0, len(fields))
	for _, f := range fields {
		if winner, ok := dominantField(byName[f.name]); ok && winner == f {
			dominant = append(dominant, f)
		}
	}
	return dominant
}

func dominantField(fields []jsonField) (jsonField, bool) {
	minDepth := fields[0].depth
	for _, f := range fields[1:] {
		minDepth = min(minDepth, f.depth)
	}
	var shallowest, tagged []jsonField
	for _, f := range fields {
		if f.depth != minDepth {
			continue
		}
		shallowest = append(shallowest, f)
		if f.tagged {
			tagged = append(tagged, f)
		}
	}
	switch {
	case len(tagged) == 1:
		return tagged[0], true
	case len(tagged) == 0 && len(shallowest) == 1:
		return shallowest[0], true
	default:
		return jsonField{}, false
	}
}

// matchField follows encoding/json: an exact name match wins, otherwise the
// first case-insensitive match.
func matchField(fields []jsonField, key string) (jsonField, bool) {
	for _, f := range fields {
		if f.name == key {
			return f, true
		}
	}
	for _, f := range fields {
		if strings.EqualFold(f.name, key) {
			return f, true
		}
	}
	return jsonField{}, false
}
