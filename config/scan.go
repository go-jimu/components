package config

import (
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
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
			n := json.Number(strings.TrimSpace(s))
			if _, err := n.Float64(); err == nil {
				return n
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
		if ft, ok := matchField(fields, k); ok {
			dst[k] = coerce(v, ft)
			continue
		}
		dst[k] = v
	}
	return dst
}

type jsonField struct {
	name string
	typ  reflect.Type
}

// jsonFields lists the fields encoding/json would decode into, including
// fields promoted from untagged embedded structs.
func jsonFields(t reflect.Type) []jsonField {
	var fields []jsonField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if f.Anonymous && name == "" && ft.Kind() == reflect.Struct {
			fields = append(fields, jsonFields(ft)...)
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fields = append(fields, jsonField{name: name, typ: f.Type})
	}
	return fields
}

// matchField follows encoding/json: an exact name match wins, otherwise the
// first case-insensitive match.
func matchField(fields []jsonField, key string) (reflect.Type, bool) {
	for _, f := range fields {
		if f.name == key {
			return f.typ, true
		}
	}
	for _, f := range fields {
		if strings.EqualFold(f.name, key) {
			return f.typ, true
		}
	}
	return nil, false
}
