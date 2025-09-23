package yaml

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
	c := &yamlCodec{name: "yaml"}
	data, err := c.Marshal(obj)
	if err != nil {
		t.FailNow()
	}
	fmt.Println(string(data))
}

func TestUnmarshal(t *testing.T) {
	data := []byte(`name: foobar
age: 16
slices:
    - a
    - b
    - c
sub:
    - f: 12.34`)
	obj := new(example)
	c := &yamlCodec{name: "yaml"}
	err := c.Unmarshal(data, obj)
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
