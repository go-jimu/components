# config

`config` loads key/value configuration from one or more sources, merges the
decoded data, resolves placeholders, and exposes values through path lookup or
struct scanning.

Two entry points:

- `config/loader` loads application config into a struct: defaults plus an
  optional profile file, with placeholders resolved from prefixed environment
  variables. Start here for application code.
- The root package builds a `Config` from sources you choose, with path lookup
  and watching.

`config/loader`:

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

Root package:

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

`WithSource` replaces the source list, so pass every source in one call. Later
sources override earlier ones, also when a watched source reloads: a reload
rebuilds the configuration from every source in order, then notifies the
observers registered with `Watch`. A key that a reload removes reads as
`ErrNotFound` from `Value`, and its observers receive a Value whose `Load` is
nil; they are notified again if the key comes back. `Load` starts over from the sources on every
call, so retry it after an error; the sources are watched once.

## Scanning

`Scan` decodes through `encoding/json`, so struct fields use `json` tags. Before
decoding, string values are converted to the kind of the target field:
`"8080"` fills an `int`, `"true"` fills a `bool`, and `"5s"` or `"1m30s"` fills a
`time.Duration`. Numbers and booleans also fill `string` fields and fields
tagged `,string`. Values that cannot be converted, such as `"NaN"`, are reported
by `encoding/json` as usual.
`Value(path).Duration()` accepts the same duration strings.

## Placeholders and environment variables

String values may contain `${name}` or `${name:default}` placeholders. A
placeholder is resolved from the `WithLookup` function first, then from a
configuration path such as `${server.host}`, then from the default.

Reference environment variables through placeholders. A placeholder names the
exact path it fills, which a flat variable name such as `SERVER_PORT` cannot do
when merged as a source.

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

## Sources

- `config/file` loads files or directories; the file extension selects the
  codec, case-insensitively.
- `config/env` reads environment variables by prefix as a source.
- `config/inmem` supplies an in-memory source for tests or embedded defaults.
