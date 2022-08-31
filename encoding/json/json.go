package json

import (
	"encoding/json"

	"github.com/go-jimu/components/encoding"
)

type jsonCodec struct{}

func (jc jsonCodec) Name() string {
	return "json"
}

func (jc jsonCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (jc jsonCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func init() {
	encoding.RegisterCodec(jsonCodec{})
}
