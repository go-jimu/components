package env

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-jimu/components/config"
	"github.com/go-jimu/components/config/file"
)

func Test_env_load_override(t *testing.T) {
	tests := []struct {
		name    string
		prefixs []string
		envs    []string
		want    []*config.KeyValue
	}{
		{
			name:    "splits levels at double underscore and lower-cases",
			prefixs: []string{"APP"},
			envs: []string{
				"APP_PORT=9090",
				"APP_DATABASE__MAX_CONN=20",
				"APP_HTTP__TLS__ENABLED=true",
				"OTHER__KEY=x",
			},
			want: []*config.KeyValue{
				{Key: "port", Value: []byte("9090")},
				{Key: "database.max_conn", Value: []byte("20")},
				{Key: "http.tls.enabled", Value: []byte("true")},
			},
		},
		{
			name:    "skips empty levels and dots",
			prefixs: []string{"APP_"},
			envs: []string{
				"APP_TRAILING__=1",
				"APP_A____B=1",
				"APP_A.B=1",
				"APP_OK=1",
			},
			want: []*config.KeyValue{
				{Key: "ok", Value: []byte("1")},
			},
		},
		{
			name:    "keeps a single underscore inside a level",
			prefixs: []string{"APP"},
			envs:    []string{"APP_SERVICE_NAME=svc"},
			want: []*config.KeyValue{
				{Key: "service_name", Value: []byte("svc")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &env{prefixs: tt.prefixs, override: true}
			got := e.load(tt.envs)
			if !reflect.DeepEqual(tt.want, got) {
				t.Errorf("env.load() = %v, want %v", got, tt.want)
			}
		})
	}
}

const _overrideYAML = `
port: 8000
database:
  dsn: "postgres://localhost/app"
  max_conn: 10
  timeout: 1s
servers:
  - a
  - b
`

func TestOverrideSourceScansModules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(_overrideYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_DATABASE__MAX_CONN", "20")
	t.Setenv("APP_DATABASE__TIMEOUT", "5s")
	t.Setenv("APP_CACHE__ADDR", "redis:6379") // key absent from the file

	c := config.New(config.WithSource(
		file.NewSource(dir),
		NewOverrideSource("APP"),
	))
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	type database struct {
		DSN     string        `json:"dsn"`
		MaxConn int           `json:"max_conn"`
		Timeout time.Duration `json:"timeout"`
	}
	var db database
	if err := c.Value("database").Scan(&db); err != nil {
		t.Fatal(err)
	}
	want := database{DSN: "postgres://localhost/app", MaxConn: 20, Timeout: 5 * time.Second}
	if db != want {
		t.Errorf("database = %+v, want %+v", db, want)
	}

	var cache struct {
		Addr string `json:"addr"`
	}
	if err := c.Value("cache").Scan(&cache); err != nil {
		t.Fatal(err)
	}
	if cache.Addr != "redis:6379" {
		t.Errorf("cache.addr = %q, want %q", cache.Addr, "redis:6379")
	}

	port, err := c.Value("port").Int()
	if err != nil || port != 9090 {
		t.Errorf("port = %d, %v; want 9090", port, err)
	}

	var all struct {
		Servers []string `json:"servers"`
	}
	if err := c.Scan(&all); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(all.Servers, []string{"a", "b"}) {
		t.Errorf("servers = %v, want [a b]", all.Servers)
	}
}
