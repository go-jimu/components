package loader_test

import (
	"os"
	"path/filepath"
	"testing"

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

func writeConfig(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}
