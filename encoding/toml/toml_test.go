package toml

import (
	"fmt"
	"reflect"
	"testing"
)

type example struct {
	Name   string
	Age    int
	Slices []string
	Sub    []struct {
		F float64
	}
}

func TestMarshal(t *testing.T) {
	obj := &example{
		Name:   "foobar",
		Age:    16,
		Slices: []string{"a", "b", "c"},
		Sub:    []struct{ F float64 }{{F: 12.34}},
	}

	data, err := tomlCodec{}.Marshal(obj)
	if err != nil {
		t.FailNow()
	}
	fmt.Println(string(data))
}

func TestUnmarshal(t *testing.T) {
	data := []byte(`Name = "foobar"
	Age = 16
	Slices = ["a", "b", "c"]
	
	[[Sub]]
	  F = 12.34`)
	obj := new(example)
	err := tomlCodec{}.Unmarshal(data, obj)
	if err != nil {
		t.FailNow()
	}

	expected := &example{
		Name:   "foobar",
		Age:    16,
		Slices: []string{"a", "b", "c"},
		Sub:    []struct{ F float64 }{{F: 12.34}},
	}

	if !reflect.DeepEqual(obj, expected) {
		t.FailNow()
	}
}
