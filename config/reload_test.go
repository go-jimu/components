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

// Every observer registered for a key is notified; a later Watch must not
// replace an earlier one.
func TestWatchNotifiesEveryObserver(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	writeFile(t, base, "port: 80\n")

	c := config.New(config.WithSource(file.NewSource(base)))
	require.NoError(t, c.Load())
	defer c.Close()

	first, second := make(chan int64, 1), make(chan int64, 1)
	require.NoError(t, c.Watch("port", func(_ string, v config.Value) {
		n, _ := v.Int()
		first <- n
	}))
	require.NoError(t, c.Watch("port", func(_ string, v config.Value) {
		n, _ := v.Int()
		second <- n
	}))
	writeFile(t, base, "port: 8080\n")

	for name, ch := range map[string]chan int64{"first": first, "second": second} {
		select {
		case n := <-ch:
			assert.Equal(t, int64(8080), n, name)
		case <-time.After(3 * time.Second):
			t.Fatalf("%s observer was not notified", name)
		}
	}
}

// A value whose type changes on reload is still updated and reported to its
// observers.
func TestWatchNotifiesWhenValueTypeChanges(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	writeFile(t, base, "port: 80\n")

	c := config.New(config.WithSource(file.NewSource(base)))
	require.NoError(t, c.Load())
	defer c.Close()

	notified := make(chan string, 1)
	require.NoError(t, c.Watch("port", func(_ string, v config.Value) {
		s, _ := v.String()
		notified <- s
	}))
	writeFile(t, base, "port: auto\n")

	select {
	case s := <-notified:
		assert.Equal(t, "auto", s)
	case <-time.After(3 * time.Second):
		t.Fatal("observer was not notified of the type change")
	}
	port, err := c.Value("port").String()
	require.NoError(t, err)
	assert.Equal(t, "auto", port)
}

// Loading again, such as a retry, replaces the configuration instead of
// stacking a second copy of every source, so reloads still take effect.
func TestLoadAgainKeepsReloading(t *testing.T) {
	dir := t.TempDir()
	defaults := filepath.Join(dir, "defaults.yaml")
	profile := filepath.Join(dir, "prod.yaml")
	writeFile(t, defaults, "name: default\nversion: 1\n")
	writeFile(t, profile, "name: prod\n")

	c := config.New(config.WithSource(file.NewSource(defaults), file.NewSource(profile)))
	require.NoError(t, c.Load())
	require.NoError(t, c.Load())
	defer c.Close()

	version := c.Value("version")
	writeFile(t, defaults, "name: default\nversion: 2\n")

	assert.Eventually(t, func() bool {
		v, err := version.Int()
		return err == nil && v == 2
	}, 3*time.Second, 20*time.Millisecond)
}

// Loading again reports values that changed since the previous Load.
func TestLoadAgainNotifiesChangedValues(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	writeFile(t, base, "port: 80\n")

	c := config.New(config.WithSource(file.NewSource(base)))
	require.NoError(t, c.Load())
	defer c.Close()

	notified := make(chan int64, 2)
	require.NoError(t, c.Watch("port", func(_ string, v config.Value) {
		n, _ := v.Int()
		notified <- n
	}))
	writeFile(t, base, "port: 8080\n")
	require.NoError(t, c.Load())

	select {
	case n := <-notified:
		assert.Equal(t, int64(8080), n)
	case <-time.After(3 * time.Second):
		t.Fatal("observer was not notified")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
