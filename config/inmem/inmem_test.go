package inmem

import (
	"bytes"
	"testing"

	"github.com/go-jimu/components/config"
)

type TestJSON struct {
	Test struct {
		Settings struct {
			IntKey      int     `json:"int_key"`
			FloatKey    float64 `json:"float_key"`
			DurationKey int     `json:"duration_key"`
			StringKey   string  `json:"string_key"`
		} `json:"settings"`
		Server struct {
			Addr string `json:"addr"`
			Port int    `json:"port"`
		} `json:"server"`
	} `json:"test"`
	Foo []struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	} `json:"foo"`
}

var (
	data = `
	{
		"test":{
			"settings":{
				"int_key":1000,
				"float_key":1000.1,
				"duration_key":10000,
				"string_key":"string_value"
			},
			"server":{
				"addr":"127.0.0.1",
				"port":8000
			}
		},
		"foo":[
			{
				"name":"nihao",
				"age":18
			},
			{
				"name":"nihao",
				"age":18
			}
		]
	}`
)

func TestLoad(t *testing.T) {
	source := NewSource("test", []byte(data), "json")
	kvs, err := source.Load()
	if err != nil {
		t.Error(err)
	}
	if !bytes.Equal(kvs[0].Value, []byte(data)) {
		t.Error("value changed")
	}
}

func TestConfig(t *testing.T) {
	conf := config.New(config.WithSource(NewSource("default", []byte(data), "json")))
	if err := conf.Load(); err != nil {
		t.Error(err)
	}

	c := new(TestJSON)
	if err := conf.Scan(c); err != nil {
		t.Error(err)
	}
	t.Log(c)
}
