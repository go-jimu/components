package config

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var (
	_ Value = (*atomicValue)(nil)
	_ Value = (*errValue)(nil)
)

// Value is config value interface.
type Value interface {
	Bool() (bool, error)
	Int() (int64, error)
	Float() (float64, error)
	String() (string, error)
	Duration() (time.Duration, error)
	Slice() ([]Value, error)
	Map() (map[string]Value, error)
	Scan(interface{}) error
	Load() interface{}
	Store(interface{})
}

// atomicValue is a Value whose content can change type, become null, or
// become not found without the atomicValue being replaced, so a Value returned
// by Config.Value keeps tracking its key across every reload.
type atomicValue struct {
	box atomic.Value // always a valueBox, so any content can be stored
}

// valueBox is the content of an atomicValue: a value of any type, including
// nil, or err while the key is absent.
type valueBox struct {
	v   interface{}
	err error
}

func (v *atomicValue) load() valueBox {
	b, _ := v.box.Load().(valueBox)
	return b
}

// Load returns the current value, or nil while the key is absent.
func (v *atomicValue) Load() interface{} {
	return v.load().v
}

// Store replaces the current value; val may be of any type, including nil.
func (v *atomicValue) Store(val interface{}) {
	v.box.Store(valueBox{v: val})
}

// storeNotFound marks the key as absent: accessors return ErrNotFound until
// the next Store.
func (v *atomicValue) storeNotFound() {
	v.box.Store(valueBox{err: ErrNotFound})
}

func (v *atomicValue) typeAssertError() error {
	if err := v.load().err; err != nil {
		return err
	}
	return fmt.Errorf("type assert to %v failed", reflect.TypeOf(v.Load()))
}

func (v *atomicValue) Bool() (bool, error) {
	switch val := v.Load().(type) {
	case bool:
		return val, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, string:
		return strconv.ParseBool(fmt.Sprint(val))
	}
	return false, v.typeAssertError()
}

func (v *atomicValue) Int() (int64, error) {
	switch val := v.Load().(type) {
	case int:
		return int64(val), nil
	case int8:
		return int64(val), nil
	case int16:
		return int64(val), nil
	case int32:
		return int64(val), nil
	case int64:
		return val, nil
	case uint:
		return uint64ToInt64(uint64(val))
	case uint8:
		return int64(val), nil
	case uint16:
		return int64(val), nil
	case uint32:
		return int64(val), nil
	case uint64:
		return uint64ToInt64(val)
	case float32:
		return int64(val), nil
	case float64:
		return int64(val), nil
	case string:
		return strconv.ParseInt(val, 10, 64)
	}
	return 0, v.typeAssertError()
}

// uint64ToInt64 rejects values above math.MaxInt64, as ParseInt does for a
// string.
func uint64ToInt64(u uint64) (int64, error) {
	if u > math.MaxInt64 {
		return 0, fmt.Errorf("value %d overflows int64", u)
	}
	return int64(u), nil
}

func (v *atomicValue) Slice() ([]Value, error) {
	vals, ok := v.Load().([]interface{})
	if !ok {
		return nil, v.typeAssertError()
	}
	slices := make([]Value, 0, len(vals))
	for _, val := range vals {
		a := new(atomicValue)
		a.Store(val)
		slices = append(slices, a)
	}
	return slices, nil
}

func (v *atomicValue) Map() (map[string]Value, error) {
	vals, ok := v.Load().(map[string]interface{})
	if !ok {
		return nil, v.typeAssertError()
	}
	m := make(map[string]Value, len(vals))
	for key, val := range vals {
		a := new(atomicValue)
		a.Store(val)
		m[key] = a
	}
	return m, nil
}

func (v *atomicValue) Float() (float64, error) {
	switch val := v.Load().(type) {
	case int:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case uint16:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	case float32:
		return float64(val), nil
	case float64:
		return val, nil
	case string:
		return strconv.ParseFloat(val, 64)
	}
	return 0.0, v.typeAssertError()
}

func (v *atomicValue) String() (string, error) {
	switch val := v.Load().(type) {
	case string:
		return val, nil
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprint(val), nil
	case []byte:
		return string(val), nil
	case fmt.Stringer:
		return val.String(), nil
	}
	return "", v.typeAssertError()
}

// Duration accepts duration strings such as "5s", like Scan into a
// time.Duration field, and falls back to an integer number of nanoseconds.
func (v *atomicValue) Duration() (time.Duration, error) {
	if s, ok := v.Load().(string); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(s)); err == nil {
			return d, nil
		}
		val, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(val), nil
	}
	val, err := v.Int()
	if err != nil {
		return 0, err
	}
	return time.Duration(val), nil
}

func (v *atomicValue) Scan(obj interface{}) error {
	if err := v.load().err; err != nil {
		return err
	}
	data, err := json.Marshal(v.Load())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, obj)
}

type errValue struct {
	err error
}

func (v errValue) Bool() (bool, error)              { return false, v.err }
func (v errValue) Int() (int64, error)              { return 0, v.err }
func (v errValue) Float() (float64, error)          { return 0.0, v.err }
func (v errValue) Duration() (time.Duration, error) { return 0, v.err }
func (v errValue) String() (string, error)          { return "", v.err }
func (v errValue) Scan(interface{}) error           { return v.err }
func (v errValue) Load() interface{}                { return nil }
func (v errValue) Store(interface{})                {}
func (v errValue) Slice() ([]Value, error)          { return nil, v.err }
func (v errValue) Map() (map[string]Value, error)   { return nil, v.err }
