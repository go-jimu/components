package config_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
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
	t.Cleanup(func() { require.NoError(t, c.Close()) })

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
	t.Cleanup(func() { require.NoError(t, c.Close()) })

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
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	first, second := make(chan int64, 16), make(chan int64, 16)
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
		awaitNotification(t, ch, name+" observer of 8080", func(n int64) bool { return n == 8080 })
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
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	notified := make(chan string, 16)
	require.NoError(t, c.Watch("port", func(_ string, v config.Value) {
		s, _ := v.String()
		notified <- s
	}))
	writeFile(t, base, "port: auto\n")

	awaitNotification(t, notified, "of the type change", func(s string) bool { return s == "auto" })
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
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	version := c.Value("version")
	writeFile(t, defaults, "name: default\nversion: 2\n")

	assert.Eventually(t, func() bool {
		v, err := version.Int()
		return err == nil && v == 2
	}, 3*time.Second, 20*time.Millisecond)
}

// Loading again reports values that changed since the previous Load. The
// source never signals a change on its own, so the notification can only come
// from Load. A file source would race: its watcher can read the file while
// os.WriteFile has truncated it and report the key as removed first.
func TestLoadAgainNotifiesChangedValues(t *testing.T) {
	source := newScriptedSource("port: 80\n")

	c := config.New(config.WithSource(source))
	require.NoError(t, c.Load())
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	notified := make(chan int64, 2)
	require.NoError(t, c.Watch("port", func(_ string, v config.Value) {
		n, _ := v.Int()
		notified <- n
	}))
	source.set("port: 8080\n")
	require.NoError(t, c.Load())

	select {
	case n := <-notified:
		assert.Equal(t, int64(8080), n)
	case <-time.After(3 * time.Second):
		t.Fatal("observer was not notified")
	}
}

// scriptedSource serves YAML that the test changes between loads. Its watcher
// reports a change only when the test calls push, so a test controls exactly
// which reloads happen.
type scriptedSource struct {
	mu     sync.Mutex
	data   string
	pushes chan string
}

func newScriptedSource(data string) *scriptedSource {
	return &scriptedSource{data: data, pushes: make(chan string)}
}

func (s *scriptedSource) set(data string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data
}

// push makes the watcher report data, as a file watcher reports a write.
func (s *scriptedSource) push(t *testing.T, data string) {
	t.Helper()
	select {
	case s.pushes <- data:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not take the change")
	}
}

func (s *scriptedSource) Load() ([]*config.KeyValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return []*config.KeyValue{scriptedKeyValue(s.data)}, nil
}

func (s *scriptedSource) Watch() (config.Watcher, error) {
	return &scriptedWatcher{pushes: s.pushes, stop: make(chan struct{})}, nil
}

func scriptedKeyValue(data string) *config.KeyValue {
	return &config.KeyValue{Key: "scripted.yaml", Value: []byte(data), Format: "yaml"}
}

type scriptedWatcher struct {
	pushes chan string
	stop   chan struct{}
	once   sync.Once
}

func (w *scriptedWatcher) Next() ([]*config.KeyValue, error) {
	select {
	case data := <-w.pushes:
		return []*config.KeyValue{scriptedKeyValue(data)}, nil
	case <-w.stop:
		return nil, context.Canceled
	}
}

func (w *scriptedWatcher) Stop() error {
	w.once.Do(func() { close(w.stop) })
	return nil
}

// A Value obtained before its key briefly disappears, as when a watcher reads
// a file that os.WriteFile has truncated, keeps tracking the key.
func TestValueTracksKeyAcrossTransientRemoval(t *testing.T) {
	source := newScriptedSource("host: a\naddr: ${host}:80\n")
	c := config.New(config.WithSource(source))
	require.NoError(t, c.Load())
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	addr := c.Value("addr")

	source.push(t, "")
	assert.Eventually(t, func() bool {
		_, err := addr.String()
		return errors.Is(err, config.ErrNotFound)
	}, 3*time.Second, 10*time.Millisecond)

	source.push(t, "host: b\naddr: ${host}:80\n")
	assert.Eventually(t, func() bool {
		v, err := addr.String()
		return err == nil && v == "b:80"
	}, 3*time.Second, 10*time.Millisecond)
}

// A Value obtained before its type changes keeps tracking the key.
func TestValueTracksKeyAcrossTypeChange(t *testing.T) {
	source := newScriptedSource("port: 80\n")
	c := config.New(config.WithSource(source))
	require.NoError(t, c.Load())
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	port := c.Value("port")

	source.push(t, "port: auto\n")

	assert.Eventually(t, func() bool {
		v, err := port.String()
		return err == nil && v == "auto"
	}, 3*time.Second, 10*time.Millisecond)
}

// A key with a null value reads as a value of no type instead of panicking.
func TestValueOfNullKey(t *testing.T) {
	c := config.New(config.WithSource(newScriptedSource("port:\nname: app\n")))
	require.NoError(t, c.Load())
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	v := c.Value("port")

	assert.Nil(t, v.Load())
	_, err := v.Int()
	assert.Error(t, err)
}

// awaitNotification reads notifications until one matches. A file source can
// report intermediate states first: os.WriteFile truncates the file before
// writing it, and the watcher may read it in between.
func awaitNotification[T any](t *testing.T, notified <-chan T, what string, match func(T) bool) {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case v := <-notified:
			if match(v) {
				return
			}
		case <-timeout:
			t.Fatalf("observer was not notified %s", what)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// A key removed on reload reads as not found, its observers hear about the
// removal, and they hear about the key again when it comes back.
func TestReloadReportsRemovedKeys(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	writeFile(t, base, "port: 80\nname: app\n")

	c := config.New(config.WithSource(file.NewSource(base)))
	require.NoError(t, c.Load())
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	type portNote struct {
		port int64
		err  error
	}
	notified := make(chan portNote, 16)
	require.NoError(t, c.Watch("port", func(_ string, v config.Value) {
		n, err := v.Int()
		notified <- portNote{port: n, err: err}
	}))

	writeFile(t, base, "name: app\n")
	awaitNotification(t, notified, "of the removal", func(n portNote) bool {
		return errors.Is(n.err, config.ErrNotFound)
	})
	_, err := c.Value("port").Int()
	require.ErrorIs(t, err, config.ErrNotFound)

	writeFile(t, base, "port: 8080\nname: app\n")
	awaitNotification(t, notified, "when the key came back", func(n portNote) bool {
		return n.err == nil && n.port == 8080
	})
	port, err := c.Value("port").Int()
	require.NoError(t, err)
	assert.Equal(t, int64(8080), port)
}
