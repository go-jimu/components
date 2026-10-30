package config

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestAtomicValue_Bool(t *testing.T) {
	vlist := []interface{}{"1", "t", "T", "true", "TRUE", "True", true, 1, int32(1)}
	for _, x := range vlist {
		v := atomicValue{}
		v.Store(x)
		b, err := v.Bool()
		if err != nil {
			t.Fatal(err)
		}
		if !b {
			t.Fatal("b is not equal to true")
		}
	}

	vlist = []interface{}{"0", "f", "F", "false", "FALSE", "False", false, 0, int32(0)}
	for _, x := range vlist {
		v := atomicValue{}
		v.Store(x)
		b, err := v.Bool()
		if err != nil {
			t.Fatal(err)
		}
		if b {
			t.Fatal("b is not equal to false")
		}
	}

	vlist = []interface{}{"bbb", "-1"}
	for _, x := range vlist {
		v := atomicValue{}
		v.Store(x)
		_, err := v.Bool()
		if err == nil {
			t.Fatal("err is nil")
		}
	}
}

func TestAtomicValue_Int(t *testing.T) {
	vlist := []interface{}{"123123", float64(123123), int64(123123), int32(123123), 123123}
	for _, x := range vlist {
		v := atomicValue{}
		v.Store(x)
		b, err := v.Int()
		if err != nil {
			t.Fatal(err)
		}
		if b != 123123 {
			t.Fatal("b is not equal to 123123")
		}
	}

	vlist = []interface{}{"bbb", "-x1", true}
	for _, x := range vlist {
		v := atomicValue{}
		v.Store(x)
		_, err := v.Int()
		if err == nil {
			t.Fatal("err is nil")
		}
	}
}

func TestAtomicValue_Float(t *testing.T) {
	vlist := []interface{}{"123123.1", 123123.1}
	for _, x := range vlist {
		v := atomicValue{}
		v.Store(x)
		b, err := v.Float()
		if err != nil {
			t.Fatal(err)
		}
		if b != 123123.1 {
			t.Fatal("b is not equal to 123123.1")
		}
	}

	vlist = []interface{}{"bbb", "-x1"}
	for _, x := range vlist {
		v := atomicValue{}
		v.Store(x)
		_, err := v.Float()
		if err == nil {
			t.Fatal("err is nil")
		}
	}
}

type ts struct {
	Name string
	Age  int
}

func (t ts) String() string {
	return fmt.Sprintf("%s%d", t.Name, t.Age)
}

func TestAtomicValue_String(t *testing.T) {
	vlist := []interface{}{"1", float64(1), int64(1), 1, int64(1)}
	for _, x := range vlist {
		v := atomicValue{}
		v.Store(x)
		b, err := v.String()
		if err != nil {
			t.Fatal(err)
		}
		if b != "1" {
			t.Fatal("b is not equal to 1")
		}
	}

	v := atomicValue{}
	v.Store(true)
	b, err := v.String()
	if err != nil {
		t.Fatal(err)
	}
	if b != "true" {
		t.Fatal(`b is not equal to "true"`)
	}

	v = atomicValue{}
	v.Store(ts{
		Name: "test",
		Age:  10,
	})
	b, err = v.String()
	if err != nil {
		t.Fatal(err)
	}
	if b != "test10" {
		t.Fatal(`b is not equal to "test10"`)
	}
}

func TestAtomicValue_Duration(t *testing.T) {
	vlist := []interface{}{int64(5)}
	for _, x := range vlist {
		v := atomicValue{}
		v.Store(x)
		b, err := v.Duration()
		if err != nil {
			t.Fatal(err)
		}
		if b != time.Duration(5) {
			t.Fatal("b is not equal to time.Duration(5)")
		}
	}
}

// Intent: Duration must accept the same forms as Scan into a time.Duration
// field, so "5s" from a file or placeholder works through Value as well.
func TestAtomicValue_DurationParsesStrings(t *testing.T) {
	tests := []struct {
		in   interface{}
		want time.Duration
	}{
		{in: "5s", want: 5 * time.Second},
		{in: " 1m30s ", want: 90 * time.Second},
		{in: "250ms", want: 250 * time.Millisecond},
		{in: "5", want: time.Duration(5)},
		{in: float64(5), want: time.Duration(5)},
	}
	for _, tt := range tests {
		v := atomicValue{}
		v.Store(tt.in)
		got, err := v.Duration()
		if err != nil {
			t.Fatalf("Duration(%#v) error = %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("Duration(%#v) = %v, want %v", tt.in, got, tt.want)
		}
	}

	v := atomicValue{}
	v.Store("five seconds")
	if _, err := v.Duration(); err == nil {
		t.Fatal("Duration(\"five seconds\") should fail")
	}
}

func TestAtomicValue_Slice(t *testing.T) {
	vlist := []interface{}{int64(5)}
	v := atomicValue{}
	v.Store(vlist)
	slices, err := v.Slice()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range slices {
		b, err := v.Duration()
		if err != nil {
			t.Fatal(err)
		}
		if b != time.Duration(5) {
			t.Fatal("b is not equal to time.Duration(5)")
		}
	}
}

func TestAtomicValue_Map(t *testing.T) {
	vlist := make(map[string]interface{})
	vlist["5"] = int64(5)
	vlist["text"] = "text"
	v := atomicValue{}
	v.Store(vlist)
	m, err := v.Map()
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range m {
		if k == "5" {
			b, err := v.Duration()
			if err != nil {
				t.Fatal(err)
			}
			if b != time.Duration(5) {
				t.Fatal("b is not equal to time.Duration(5)")
			}
		} else {
			b, err := v.String()
			if err != nil {
				t.Fatal(err)
			}
			if b != "text" {
				t.Fatal(`b is not equal to "text"`)
			}
		}
	}
}

func TestAtomicValue_Scan(t *testing.T) {
	v := atomicValue{}
	err := v.Scan(&struct {
		A string `json:"a"`
	}{"a"})
	if err != nil {
		t.Fatal(err)
	}

	err = v.Scan(&struct {
		A string `json:"a"`
	}{"a"})
	if err != nil {
		t.Fatal(err)
	}
}

// Intent: a missing key yields a Value whose accessors all report the lookup
// error instead of panicking.
func TestErrValueReportsErrorFromEveryAccessor(t *testing.T) {
	v := errValue{err: ErrNotFound}
	checks := map[string]error{}
	_, checks["Bool"] = v.Bool()
	_, checks["Int"] = v.Int()
	_, checks["Float"] = v.Float()
	_, checks["Duration"] = v.Duration()
	_, checks["String"] = v.String()
	_, checks["Slice"] = v.Slice()
	_, checks["Map"] = v.Map()
	checks["Scan"] = v.Scan(new(struct{}))
	for name, err := range checks {
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s error = %v, want ErrNotFound", name, err)
		}
	}
	v.Store(1)
	if v.Load() != nil {
		t.Fatal("errValue must stay empty after Store")
	}
}

// Intent: with WithResolveActualTypes, a value that is a single placeholder
// keeps the type of its resolved text.
func TestWithResolveActualTypesConvertsWholePlaceholders(t *testing.T) {
	c := New(
		WithSource(newTestJSONSource(`{"port":"${PORT:8080}","addr":"${HOST:a}:${PORT:8080}"}`)),
		WithResolveActualTypes(true),
	)
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if port := c.Value("port").Load(); port != int64(8080) {
		t.Fatalf("port = %#v, want int64(8080)", port)
	}
	if addr, _ := c.Value("addr").String(); addr != "a:8080" {
		t.Fatalf("addr = %q, want %q", addr, "a:8080")
	}
}
