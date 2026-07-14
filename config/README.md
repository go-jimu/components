# config

`config` loads key/value configuration from one or more sources, merges the
decoded data, resolves placeholders, and exposes values through path lookup or
struct scanning.

Use the root package when you need a configurable `Config` object:

```go
cfg := config.New(
	config.WithSource(file.NewSource("./configs")),
	config.WithSource(env.NewSource("APP_")),
)
if err := cfg.Load(); err != nil {
	return err
}

addr, err := cfg.Value("server.http.addr").String()
```

Use `config/loader` when you want the opinionated application-config flow:

```go
var opts struct {
	Name string `yaml:"name"`
	Age  int    `yaml:"age"`
}

err := loader.Load(
	&opts,
	loader.WithConfigurationDirectory("./configs", "defaults"),
	loader.WithConfigFilePrefix("app"),
	loader.WithProfilesAlias("dev"),
)
```

Source packages:

- `config/file` loads files or directories.
- `config/env` reads environment variables by prefix.
- `config/inmem` supplies an in-memory source for tests or embedded defaults.
- `config/loader` merges defaults and explicit profile files into a struct.
