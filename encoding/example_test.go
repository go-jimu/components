package encoding_test

import (
	"fmt"

	"github.com/go-jimu/components/encoding"
	_ "github.com/go-jimu/components/encoding/json"
)

type examplePayload struct {
	Name string `json:"name"`
}

func ExampleGetCodec() {
	codec := encoding.GetCodec("json")
	if codec == nil {
		panic("json codec is not registered")
	}

	data, err := codec.Marshal(examplePayload{Name: "agent"})
	if err != nil {
		panic(err)
	}

	fmt.Println(codec.Name(), string(data))

	// Output:
	// json {"name":"agent"}
}
