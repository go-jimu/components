package config_test

import (
	"fmt"

	"github.com/go-jimu/components/config"
	"github.com/go-jimu/components/config/inmem"
)

func ExampleNew() {
	cfg := config.New(config.WithSource(inmem.NewSource("app", []byte(`{
		"server": {
			"addr": "127.0.0.1",
			"port": 8080
		}
	}`), "json")))
	if err := cfg.Load(); err != nil {
		panic(err)
	}
	defer cfg.Close()

	addr, err := cfg.Value("server.addr").String()
	if err != nil {
		panic(err)
	}
	port, err := cfg.Value("server.port").Int()
	if err != nil {
		panic(err)
	}

	fmt.Println(addr, port)

	// Output:
	// 127.0.0.1 8080
}
