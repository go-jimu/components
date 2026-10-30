# config

`config` loads key/value configuration from one or more sources, merges the
decoded data, resolves placeholders, and exposes values through path lookup or
struct scanning.

Use the root package when you need a configurable `Config` object:

```go
cfg := config.New(
	config.WithSource(file.NewSource("./configs")),
	config.WithLookup(func(name string) (string, bool) {
		return os.LookupEnv("APP_" + name)
	}),
)
if err := cfg.Load(); err != nil {
	return err
}

addr, err := cfg.Value("server.http.addr").String()
```

Use `config/loader` when you want the opinionated application-config flow:

```go
var opts struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

err := loader.Load(
	&opts,
	loader.WithConfigurationDirectory("./configs", "defaults"),
	loader.WithConfigFilePrefix("app"),
	loader.WithProfilesAlias("dev"),
)
```

`WithSource` replaces the source list, so pass every source in one call. Later
sources override earlier ones.

## Scanning

`Scan` decodes through `encoding/json`, so struct fields use `json` tags. Before
decoding, string values are converted to the kind of the target field:
`"8080"` fills an `int`, `"true"` fills a `bool`, and `"5s"` or `"1m30s"` fills a
`time.Duration`. Numbers and booleans also fill `string` fields. Values that
cannot be converted are reported by `encoding/json` as usual.

## Placeholders and environment variables

String values may contain `${name}` or `${name:default}` placeholders. A
placeholder is resolved from the `WithLookup` function first, then from a
configuration path such as `${server.host}`, then from the default.

Environment variables are meant to be referenced through placeholders, not
merged as an override layer: flat variable names cannot address nested paths.

```yaml
server:
  addr: ${HOST:0.0.0.0}:${PORT:8000}
  port: ${PORT:8000}
  timeout: ${TIMEOUT:5s}
```

`config/loader` resolves `${PORT}` from the environment variable
`<prefix>PORT`, where the prefix comes from `loader.WithEnvVarsPrefix`. With
`WithEnvVarsPrefix("app")`, `APP_PORT=9090` sets both `server.port` and the
port part of `server.addr`.

Source packages:

- `config/file` loads files or directories.
- `config/env` reads environment variables by prefix.
- `config/inmem` supplies an in-memory source for tests or embedded defaults.
- `config/loader` merges defaults and explicit profile files into a struct,
  resolving placeholders from prefixed environment variables.
