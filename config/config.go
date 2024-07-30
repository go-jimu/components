package config

import (
	"context"
	"encoding/json"
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

type config struct {
	opts      options
	reader    Reader
	cached    sync.Map
	observers sync.Map
	watchers  []Watcher
}

// New new a config with options.
func New(opts ...Option) Config {
	o := options{
		decoder:  defaultDecoder,
		resolver: defaultResolver,
		merge:    defaultMerge,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return &config{
		opts:   o,
		reader: newReader(o),
	}
}

func (c *config) watch(w Watcher) {
	for {
		kvs, err := w.Next()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				slog.Info("watcher's ctx was canceled")
				return
			}
			time.Sleep(time.Second)
			slog.Error("faield to watch next config", sloghelper.Error(err))
			continue
		}
		if err = c.reader.Merge(kvs...); err != nil {
			slog.Error("failed to merge next config", sloghelper.Error(err))
			continue
		}
		if err = c.reader.Resolve(); err != nil {
			slog.Error("failed to resolve next config", sloghelper.Error(err))
			continue
		}
		c.cached.Range(func(key, value interface{}) bool {
			k, _ := key.(string)
			v, _ := value.(Value)
			if n, ok := c.reader.Value(k); ok &&
				reflect.TypeOf(n.Load()) == reflect.TypeOf(v.Load()) &&
				!reflect.DeepEqual(n.Load(), v.Load()) {
				v.Store(n.Load())
				if o, ok := c.observers.Load(k); ok {
					o.(Observer)(k, v)
				}
			}
			return true
		})
	}
}

func (c *config) Load() error {
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
		w, err := src.Watch()
		if err != nil {
			slog.Error("failed to watch config source", sloghelper.Error(err))
			return err
		}
		c.watchers = append(c.watchers, w)
		go c.watch(w)
	}
	if err := c.reader.Resolve(); err != nil {
		slog.Error("failed to resolve config source", sloghelper.Error(err))
		return err
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
	return json.Unmarshal(data, v)
}

func (c *config) Watch(key string, o Observer) error {
	if v := c.Value(key); v.Load() == nil {
		return ErrNotFound
	}
	c.observers.Store(key, o)
	return nil
}

func (c *config) Close() error {
	for _, w := range c.watchers {
		if err := w.Stop(); err != nil {
			return err
		}
	}
	return nil
}
