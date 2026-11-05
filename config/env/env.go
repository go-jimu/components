package env

import (
	"os"
	"strings"

	"github.com/go-jimu/components/config"
)

type env struct {
	prefixs []string
}

// NewSource returns a Source that loads environment variables as key/value
// entries. With prefixes, it keeps only the variables that start with one of
// them and strips the first matching prefix plus one following "_", so
// APP_NAME with prefix "APP" becomes key NAME. A variable equal to a prefix
// is skipped. Without prefixes, it loads every variable unchanged. Its
// watcher never reports a change.
func NewSource(prefixs ...string) config.Source {
	return &env{prefixs: prefixs}
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

func matchPrefix(prefixes []string, s string) (string, bool) {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return p, true
		}
	}
	return "", false
}
