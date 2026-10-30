package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-jimu/components/config"
	"github.com/go-jimu/components/config/file"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reloading a lower-priority source must not override values from a
// higher-priority source.
func TestReloadKeepsSourcePriority(t *testing.T) {
	dir := t.TempDir()
	defaults := filepath.Join(dir, "defaults.yaml")
	profile := filepath.Join(dir, "prod.yaml")
	writeFile(t, defaults, "name: default\nversion: 1\n")
	writeFile(t, profile, "name: prod\n")

	c := config.New(config.WithSource(file.NewSource(defaults), file.NewSource(profile)))
	require.NoError(t, c.Load())
	defer c.Close()

	version := c.Value("version")
	writeFile(t, defaults, "name: default\nversion: 2\n")

	assert.Eventually(t, func() bool {
		v, err := version.Int()
		return err == nil && v == 2
	}, 3*time.Second, 20*time.Millisecond)
	name, err := c.Value("name").String()
	require.NoError(t, err)
	assert.Equal(t, "prod", name)
}

// Reloading should re-resolve placeholders that reference changed values.
func TestReloadResolvesPlaceholdersAgainstNewValues(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	writeFile(t, base, "host: a\naddr: ${host}:80\n")

	c := config.New(config.WithSource(file.NewSource(base)))
	require.NoError(t, c.Load())
	defer c.Close()

	addr := c.Value("addr")
	writeFile(t, base, "host: b\naddr: ${host}:80\n")

	assert.Eventually(t, func() bool {
		v, err := addr.String()
		return err == nil && v == "b:80"
	}, 3*time.Second, 20*time.Millisecond)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
