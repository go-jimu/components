package env

import (
	"os"
	"strings"

	"github.com/go-jimu/components/config"
)

type env struct {
	prefixs []string
	// override selects the override-source key mapping; see NewOverrideSource.
	override bool
}

// NewSource returns a Source that loads environment variables as flat
// key/value entries; see "Flat source" in the package documentation. With
// prefixes, it keeps only the variables that start with one of them and
// strips the first matching prefix plus one following "_", so APP_NAME with
// prefix "APP" becomes key NAME. A variable equal to a prefix is skipped.
// Without prefixes, it loads every variable unchanged. Its watcher never
// reports a change.
func NewSource(prefixs ...string) config.Source {
	return &env{prefixs: prefixs}
}

// NewOverrideSource returns a Source whose variables override nested
// configuration paths; see "Override source" in the package documentation.
// Prefixes match and are stripped as in NewSource. The rest of the name is
// lower-cased and split into levels at each "__", so APP_DATABASE__MAX_CONN
// with prefix "APP" becomes key database.max_conn. A variable whose name
// yields an empty level or contains "." is skipped. Its watcher never reports
// a change.
func NewOverrideSource(prefixs ...string) config.Source {
	return &env{prefixs: prefixs, override: true}
}

func (e *env) Load() (kv []*config.KeyValue, err error) {
	return e.load(os.Environ()), nil
}

func (e *env) load(envStrings []string) []*config.KeyValue {
	var kv []*config.KeyValue
	for _, envstr := range envStrings {
		var k, v string
		subs := strings.SplitN(envstr, "=", 2)
		k = subs[0]
		if len(subs) > 1 {
			v = subs[1]
		}

		if len(e.prefixs) > 0 {
			p, ok := matchPrefix(e.prefixs, k)
			if !ok || len(p) == len(k) {
				continue
			}
			// trim prefix
			k = strings.TrimPrefix(k, p)
			k = strings.TrimPrefix(k, "_")
		}

		if e.override {
			var ok bool
			if k, ok = overrideKey(k); !ok {
				continue
			}
		}

		if len(k) != 0 {
			kv = append(kv, &config.KeyValue{
				Key:   k,
				Value: []byte(v),
			})
		}
	}
	return kv
}

func (e *env) Watch() (config.Watcher, error) {
	w, err := NewWatcher()
	if err != nil {
		return nil, err
	}
	return w, nil
}

// overrideKey maps a prefix-stripped variable name to a configuration path:
// lower-cased, with "__" separating levels. It reports false when the name
// contains "." or yields an empty level.
func overrideKey(name string) (string, bool) {
	if strings.Contains(name, ".") {
		return "", false
	}
	levels := strings.Split(strings.ToLower(name), "__")
	for _, level := range levels {
		if level == "" {
			return "", false
		}
	}
	return strings.Join(levels, "."), true
}

func matchPrefix(prefixes []string, s string) (string, bool) {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return p, true
		}
	}
	return "", false
}
