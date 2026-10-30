package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	// init encoding.
	_ "github.com/go-jimu/components/encoding/json"
	_ "github.com/go-jimu/components/encoding/proto"
	_ "github.com/go-jimu/components/encoding/toml"
	_ "github.com/go-jimu/components/encoding/yaml"
	"github.com/go-jimu/components/sloghelper"
)

var (
	// ErrNotFound is key not found.
	ErrNotFound = errors.New("key not found")
	// ErrTypeAssert is type assert error.
	ErrTypeAssert = errors.New("type assert error")

	_ Config = (*config)(nil)
)

// Observer is config observer.
type Observer func(string, Value)

// Config is a config interface.
type Config interface {
	Load() error
	Scan(v interface{}) error
	Value(key string) Value
	Watch(key string, o Observer) error
	Close() error
}

// Watch retries a failing source with exponential backoff between these bounds.
const (
	watchRetryInitial = time.Second
	watchRetryCap     = 30 * time.Second
)

type config struct {
	opts     options
	reader   *reader
	cached   sync.Map
	watchers []Watcher

	observerMu sync.Mutex
	observers  map[string][]Observer

	done      chan struct{}
	closeOnce sync.Once

	// mu serializes reloads. snapshots holds the latest key values of each
	// source in source order, so a reload can rebuild the whole configuration
	// with the original priority instead of merging one changed source on top.
	mu        sync.Mutex
	snapshots [][]*KeyValue
}

// New new a config with options.
func New(opts ...Option) Config {
	o := options{
		decoder: defaultDecoder,
		merge:   defaultMerge,
	}
	for _, opt := range opts {
		opt(&o)
	}
	if o.resolver == nil {
		o.resolver = newResolver(o.lookup, o.actualTypes)
	}
	return &config{
		opts:      o,
		reader:    newReader(o),
		observers: make(map[string][]Observer),
		done:      make(chan struct{}),
	}
}

func (c *config) watch(index int, w Watcher) {
	delay := watchRetryInitial
	for {
		kvs, err := w.Next()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				slog.Info("config watcher stopped")
				return
			}
			slog.Error("failed to watch next config",
				slog.Duration("retry_in", delay), sloghelper.Error(err))
			select {
			case <-c.done:
				return
			case <-time.After(delay):
			}
			delay = nextWatchRetryDelay(delay)
			continue
		}
		delay = watchRetryInitial
		if len(kvs) == 0 {
			continue
		}
		if err = c.reload(index, kvs); err != nil {
			slog.Error("failed to reload config", sloghelper.Error(err))
			continue
		}
		c.notify()
	}
}

// reload replaces the changed key values of one source and rebuilds the
// configuration from every source snapshot in their original order.
func (c *config) reload(index int, kvs []*KeyValue) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := upsertKeyValues(c.snapshots[index], kvs)
	if err := c.reader.Reset(flattenKeyValues(c.snapshots, index, snapshot)...); err != nil {
		return err
	}
	c.snapshots[index] = snapshot
	return nil
}

func nextWatchRetryDelay(delay time.Duration) time.Duration {
	return min(2*delay, watchRetryCap)
}

// notify updates cached values that changed and calls their observers. A
// value whose type changed cannot be stored into the existing Value, so the
// cache entry is replaced; Values obtained before the change keep the old
// value.
func (c *config) notify() {
	c.cached.Range(func(key, value interface{}) bool {
		k, _ := key.(string)
		v, _ := value.(Value)
		n, ok := c.reader.Value(k)
		if !ok || reflect.DeepEqual(n.Load(), v.Load()) {
			return true
		}
		if reflect.TypeOf(n.Load()) == reflect.TypeOf(v.Load()) {
			v.Store(n.Load())
		} else {
			c.cached.Store(k, n)
			v = n
		}
		for _, observe := range c.observersOf(k) {
			observe(k, v)
		}
		return true
	})
}

func (c *config) observersOf(key string) []Observer {
	c.observerMu.Lock()
	defer c.observerMu.Unlock()
	return append([]Observer(nil), c.observers[key]...)
}

// upsertKeyValues returns a copy of snapshot where entries with the same Key
// are replaced in place and new keys are appended.
func upsertKeyValues(snapshot, changed []*KeyValue) []*KeyValue {
	next := append([]*KeyValue(nil), snapshot...)
	for _, kv := range changed {
		replaced := false
		for i, old := range next {
			if old.Key == kv.Key {
				next[i] = kv
				replaced = true
				break
			}
		}
		if !replaced {
			next = append(next, kv)
		}
	}
	return next
}

func flattenKeyValues(snapshots [][]*KeyValue, index int, replacement []*KeyValue) []*KeyValue {
	var all []*KeyValue
	for i, snapshot := range snapshots {
		if i == index {
			snapshot = replacement
		}
		all = append(all, snapshot...)
	}
	return all
}

func (c *config) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, src := range c.opts.sources {
		kvs, err := src.Load()
		if err != nil {
			return err
		}
		for _, v := range kvs {
			slog.Debug(fmt.Sprintf("config loaded: %s format: %s", v.Key, v.Format))
		}
		if err = c.reader.Merge(kvs...); err != nil {
			slog.Error("failed to merge config source", sloghelper.Error(err))
			return err
		}
		c.snapshots = append(c.snapshots, kvs)
	}
	if err := c.reader.Resolve(); err != nil {
		slog.Error("failed to resolve config source", sloghelper.Error(err))
		return err
	}
	// Watch after every source is loaded, so a reload always sees a snapshot
	// for each source.
	for i, src := range c.opts.sources {
		w, err := src.Watch()
		if err != nil {
			slog.Error("failed to watch config source", sloghelper.Error(err))
			return err
		}
		c.watchers = append(c.watchers, w)
		go c.watch(i, w)
	}
	return nil
}

func (c *config) Value(key string) Value {
	if v, ok := c.cached.Load(key); ok {
		return v.(Value)
	}
	if v, ok := c.reader.Value(key); ok {
		c.cached.Store(key, v)
		return v
	}
	return &errValue{err: ErrNotFound}
}

func (c *config) Scan(v interface{}) error {
	data, err := c.reader.Source()
	if err != nil {
		return err
	}
	return scanJSON(data, v)
}

// Watch registers o to be called when the value at key changes. Every observer
// registered for a key is called.
func (c *config) Watch(key string, o Observer) error {
	if v := c.Value(key); v.Load() == nil {
		return ErrNotFound
	}
	c.observerMu.Lock()
	defer c.observerMu.Unlock()
	if c.observers == nil {
		c.observers = make(map[string][]Observer)
	}
	c.observers[key] = append(c.observers[key], o)
	return nil
}

func (c *config) Close() error {
	c.closeOnce.Do(func() {
		if c.done != nil {
			close(c.done)
		}
	})
	for _, w := range c.watchers {
		if err := w.Stop(); err != nil {
			return err
		}
	}
	return nil
}
