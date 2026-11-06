package loader_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-jimu/components/config/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Option struct {
	Name string `yaml:"name"`
	Age  int    `yaml:"age"`
}

// The loader should read defaults when no profile is active.
func TestLoader(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")

	opt := new(Option)
	err := loader.Load(opt)
	assert.NoError(t, err)
	assert.EqualValues(t, opt, &Option{Name: "foobar", Age: 18})

	t.Setenv("JIMU_PROFILES_ACTIVE", "test")
	opt = new(Option)
	err = loader.Load(opt, loader.WithConfigFilePrefix("app"))
	assert.NoError(t, err)
	assert.EqualValues(t, opt, &Option{Name: "nihao", Age: 18})
}

// Profile loading should merge the explicit prefix+alias file over defaults.
func TestLoad_ProfileFileRequiresExplicitPrefixAndFallsBackToDefaults(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")

	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", "name: defaults\nage: 18\n")
	writeConfig(t, dir, "foobar_dev.yml", "name: foobar-dev\n")

	var opt Option
	err := loader.Load(
		&opt,
		loader.WithConfigurationDirectory(dir, "defaults"),
		loader.WithConfigFilePrefix("foobar"),
		loader.WithProfilesAlias("dev"),
	)

	require.NoError(t, err)
	assert.EqualValues(t, Option{Name: "foobar-dev", Age: 18}, opt)
}

// Profile loading should not pick up files that only match the environment suffix.
func TestLoad_ProfileFileIgnoresMatchingSuffixWithDifferentPrefix(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")

	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", "name: defaults\nage: 18\n")
	writeConfig(t, dir, "foobar_dev.yml", "name: foobar-dev\n")
	writeConfig(t, dir, "other_dev.yml", "name: other-dev\nage: 99\n")

	var opt Option
	err := loader.Load(
		&opt,
		loader.WithConfigurationDirectory(dir, "defaults"),
		loader.WithConfigFilePrefix("foobar"),
		loader.WithProfilesAlias("dev"),
	)

	require.NoError(t, err)
	assert.EqualValues(t, Option{Name: "foobar-dev", Age: 18}, opt)
}

// Without a profile alias, environment-specific files should not override defaults.
func TestLoad_WithoutProfileAliasLoadsOnlyDefaults(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")

	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", "name: defaults\nage: 18\n")
	writeConfig(t, dir, "foobar_dev.yml", "name: foobar-dev\nage: 99\n")

	var opt Option
	err := loader.Load(
		&opt,
		loader.WithConfigurationDirectory(dir, "defaults"),
		loader.WithConfigFilePrefix("foobar"),
	)

	require.NoError(t, err)
	assert.EqualValues(t, Option{Name: "defaults", Age: 18}, opt)
}

// Activating a profile without declaring the config-file prefix should fail loudly.
func TestLoad_ProfileAliasWithoutConfigFilePrefixFails(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")

	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", "name: defaults\nage: 18\n")
	writeConfig(t, dir, "foobar_dev.yml", "name: foobar-dev\nage: 99\n")

	var opt Option
	err := loader.Load(
		&opt,
		loader.WithConfigurationDirectory(dir, "defaults"),
		loader.WithProfilesAlias("dev"),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file prefix")
}

type envOption struct {
	Server struct {
		Addr    string        `json:"addr"`
		Port    int           `json:"port"`
		Debug   bool          `json:"debug"`
		Timeout time.Duration `json:"timeout"`
	} `json:"server"`
	Password string `json:"password"`
	Home     string `json:"home"`
}

// Environment variables should feed placeholders, including typed fields.
func TestLoad_ResolvesEnvPlaceholdersIntoTypedFields(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")
	t.Setenv("APP_HOST", "10.0.0.1")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_DEBUG", "true")

	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", `server:
  addr: ${HOST:0.0.0.0}:${PORT:80}
  port: ${PORT:80}
  debug: ${DEBUG:false}
  timeout: ${TIMEOUT:5s}
password: ${PASSWORD:123456}
`)

	var opt envOption
	err := loader.Load(&opt, loader.WithConfigurationDirectory(dir, "defaults"), loader.WithEnvVarsPrefix("app"))

	require.NoError(t, err)
	assert.Equal(t, "10.0.0.1:9090", opt.Server.Addr)
	assert.Equal(t, 9090, opt.Server.Port)
	assert.True(t, opt.Server.Debug)
	assert.Equal(t, 5*time.Second, opt.Server.Timeout)
	assert.Equal(t, "123456", opt.Password)
}

// Environment variables are not an override layer, so unrelated variables must
// not leak into struct fields.
func TestLoad_DoesNotMergeEnvAsOverrideLayer(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")
	t.Setenv("HOME", "/root")

	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", "server:\n  port: 80\n")

	var opt envOption
	require.NoError(t, loader.Load(&opt, loader.WithConfigurationDirectory(dir, "defaults")))
	assert.Empty(t, opt.Home)
	assert.Equal(t, 80, opt.Server.Port)
}

// A one-shot Load must stop the watchers started by config.Load.
func TestLoad_StopsSourceWatchers(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")
	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", "name: defaults\n")

	before := runtime.NumGoroutine()
	for i := 0; i < 20; i++ {
		var opt Option
		require.NoError(t, loader.Load(&opt, loader.WithConfigurationDirectory(dir, "defaults")))
	}
	assert.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= before+2
	}, time.Second, 10*time.Millisecond)
}

func writeConfig(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

type overrideOption struct {
	Name     string `json:"name"`
	Database struct {
		DSN     string `json:"dsn"`
		MaxConn int    `json:"max_conn"`
	} `json:"database"`
	Server struct {
		Port int `json:"port"`
	} `json:"server"`
}

// WithEnvOverrides layers prefixed variables over defaults and profile files.
func TestLoad_EnvOverridesWinOverProfileAndDefaults(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")
	t.Setenv("APP_DATABASE__MAX_CONN", "20")
	t.Setenv("APP_SERVER__PORT", "9090")
	t.Setenv("APP_NAME", "from-env")

	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", `name: defaults
database:
  dsn: postgres://localhost/app
  max_conn: 5
server:
  port: ${PORT:80}
`)
	writeConfig(t, dir, "app_dev.yml", "database:\n  max_conn: 10\n")

	var opt overrideOption
	err := loader.Load(&opt,
		loader.WithConfigurationDirectory(dir, "defaults"),
		loader.WithConfigFilePrefix("app"),
		loader.WithProfilesAlias("dev"),
		loader.WithEnvVarsPrefix("app"),
		loader.WithEnvOverrides(),
	)

	require.NoError(t, err)
	assert.Equal(t, "from-env", opt.Name)
	assert.Equal(t, "postgres://localhost/app", opt.Database.DSN)
	assert.Equal(t, 20, opt.Database.MaxConn)
	assert.Equal(t, 9090, opt.Server.Port, "override replaces the ${PORT} value")
}

// Without WithEnvOverrides, prefixed variables only feed placeholders.
func TestLoad_EnvOverridesOffByDefault(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")
	t.Setenv("APP_DATABASE__MAX_CONN", "20")

	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", "database:\n  max_conn: 5\n")

	var opt overrideOption
	require.NoError(t, loader.Load(&opt,
		loader.WithConfigurationDirectory(dir, "defaults"),
		loader.WithEnvVarsPrefix("app"),
	))
	assert.Equal(t, 5, opt.Database.MaxConn)
}

// An empty prefix would pull every variable, such as USER, into the tree.
func TestLoad_EnvOverridesRequirePrefix(t *testing.T) {
	t.Setenv(loader.ProfilesActiveFromEnvVar, "")

	dir := t.TempDir()
	writeConfig(t, dir, "defaults.yml", "name: defaults\n")

	var opt overrideOption
	err := loader.Load(&opt,
		loader.WithConfigurationDirectory(dir, "defaults"),
		loader.WithEnvOverrides(),
	)
	require.Error(t, err)
}
