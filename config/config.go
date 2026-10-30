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
	watchers []Watcher

	// cached is read without a lock; cacheMu serializes every write to it with
	// the reader read that produced the value, and guards observers. notifyMu
	// serializes notify, so observers of one key run one at a time and in
	// change order.
	cached    sync.Map
	cacheMu   sync.Mutex
	observers map[string][]Observer
	notifyMu  sync.Mutex

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

// notify updates cached values that changed and calls their observers.
// Observers run outside cacheMu, so they may call Value and Watch.
func (c *config) notify() {
	c.notifyMu.Lock()
	defer c.notifyMu.Unlock()

	for _, change := range c.refreshCache() {
		for _, observe := range change.observers {
			observe(change.key, change.value)
		}
	}
}

type change struct {
	key       string
	value     Value
	observers []Observer
}

// refreshCache brings every cached value up to date with the reader. A value
// whose type changed cannot be stored into the existing Value, so the cache
// entry is replaced; Values obtained before the change keep the old value. A
// removed key is cached as not found, so Value reports ErrNotFound and the
// key's observers hear about the removal and about the key coming back.
func (c *config) refreshCache() []change {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	var changes []change
	c.cached.Range(func(key, value interface{}) bool {
		k, _ := key.(string)
		v, _ := value.(Value)
		n, ok := c.reader.Value(k)
		if !ok {
			n = &errValue{err: ErrNotFound}
		}
		if reflect.DeepEqual(n.Load(), v.Load()) {
			return true
		}
		if ok && reflect.TypeOf(n.Load()) == reflect.TypeOf(v.Load()) {
			v.Store(n.Load())
		} else {
			c.cached.Store(k, n)
			v = n
		}
		changes = append(changes, change{
			key:       k,
			value:     v,
			observers: append([]Observer(nil), c.observers[k]...),
		})
		return true
	})
	return changes
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

// Load reads every source and replaces the whole configuration, then starts
// watching the sources. Load can be called again, for example to retry after
// an error: each call starts over from the sources, and the sources are
// watched once.
func (c *config) Load() error {
	if err := c.load(); err != nil {
		return err
	}
	c.notify()
	return nil
}

func (c *config) load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshots := make([][]*KeyValue, 0, len(c.opts.sources))
	for _, src := range c.opts.sources {
		kvs, err := src.Load()
		if err != nil {
			return err
		}
		for _, v := range kvs {
			slog.Debug(fmt.Sprintf("config loaded: %s format: %s", v.Key, v.Format))
		}
		snapshots = append(snapshots, kvs)
	}
	if err := c.reader.Reset(flattenKeyValues(snapshots, -1, nil)...); err != nil {
		slog.Error("failed to load config sources", sloghelper.Error(err))
		return err
	}
	c.snapshots = snapshots
	if len(c.watchers) > 0 {
		return nil
	}
	// Watch after every source is loaded, so a reload always sees a snapshot
	// for each source.
	watchers := make([]Watcher, 0, len(c.opts.sources))
	for _, src := range c.opts.sources {
		w, err := src.Watch()
		if err != nil {
			slog.Error("failed to watch config source", sloghelper.Error(err))
			for _, started := range watchers {
				_ = started.Stop()
			}
			return err
		}
		watchers = append(watchers, w)
	}
	c.watchers = watchers
	for i, w := range watchers {
		go c.watch(i, w)
	}
	return nil
}

func (c *config) Value(key string) Value {
	if v, ok := c.cachedValue(key); ok {
		return v
	}
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	return c.valueLocked(key)
}

func (c *config) valueLocked(key string) Value {
	if v, ok := c.cachedValue(key); ok {
		return v
	}
	if v, ok := c.reader.Value(key); ok {
		c.cached.Store(key, v)
		return v
	}
	return &errValue{err: ErrNotFound}
}

func (c *config) cachedValue(key string) (Value, bool) {
	v, ok := c.cached.Load(key)
	if !ok {
		return nil, false
	}
	value, ok := v.(Value)
	return value, ok
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
	// Caching the value and registering o under one lock means every change
	// after the value was read reaches o.
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if v := c.valueLocked(key); v.Load() == nil {
		return ErrNotFound
	}
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
