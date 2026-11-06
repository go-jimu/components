# config

`config` loads configuration from sources, merges it, resolves
`${name:default}` placeholders, and serves it by path or as a struct. The rules
live in the package documentation; this README walks through the common
setups.

```sh
go doc -all github.com/go-jimu/components/config
go doc -all github.com/go-jimu/components/config/loader
```

## Application config: `config/loader`

Start here for application code. `loader.Load` reads `configs/defaults.yml`,
overrides it with the active profile file, here `configs/app_dev.yml`, and
scans the result into a struct:

```go
var opts struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

err := loader.Load(
	&opts,
	loader.WithConfigurationDirectory("./configs", "defaults"),
	loader.WithConfigFilePrefix("app"),
	loader.WithProfilesAlias("dev"), // JIMU_PROFILES_ACTIVE wins when set
)
```

## Environment variables

With one prefix, environment variables fill placeholders and override keys
that have none:

```yaml
# configs/defaults.yml
server:
  addr: ${HOST:0.0.0.0}:${PORT:8000}
  timeout: ${TIMEOUT:5s}
database:
  max_conn: 10
```

```go
var opts struct {
	Server struct {
		Addr    string        `json:"addr"`
		Timeout time.Duration `json:"timeout"`
	} `json:"server"`
	Database struct {
		MaxConn int `json:"max_conn"`
	} `json:"database"`
}

err := loader.Load(
	&opts,
	loader.WithEnvVarsPrefix("app"),
	loader.WithEnvOverrides(),
)
```

With `APP_PORT=9090` and `APP_DATABASE__MAX_CONN=20`, `opts` holds
`0.0.0.0:9090`, `5s`, and `20`. Rules: "Environment variables" in
`config/loader`, "Override source" in `config/env`, "Scanning" in `config`.

## Composing sources and watching

The root package composes sources directly and keeps watching them:

```go
cfg := config.New(
	config.WithSource(
		file.NewSource("./configs"),
		env.NewOverrideSource("APP"),
	),
	config.WithLookup(func(name string) (string, bool) {
		return os.LookupEnv("APP_" + name)
	}),
)
if err := cfg.Load(); err != nil {
	return err
}
defer func() { _ = cfg.Close() }()

addr := cfg.Value("server.addr") // tracks the key across reloads
if err := cfg.Watch("server.addr", func(_ string, v config.Value) {
	s, err := v.String() // config.ErrNotFound while the key is removed
	...
}); err != nil {
	return err
}
```

Rules: "Sources", "Placeholders", "Values", and "Reload" in `config`.
