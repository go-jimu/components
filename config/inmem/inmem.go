package inmem

import "github.com/go-jimu/components/config"

type inmem struct {
	key    string
	data   []byte
	format string
}

var _ config.Source = (*inmem)(nil)

// NewSource returns a Source that loads data as one entry under key, decoded
// by the codec registered for format, such as "json" or "yaml". Its
// watcher never reports a change.
func NewSource(key string, data []byte, format string) config.Source {
	return &inmem{
		key:    key,
		data:   data,
		format: format,
	}
}

func (im *inmem) Load() ([]*config.KeyValue, error) {
	return []*config.KeyValue{{Key: im.key, Value: im.data, Format: im.format}}, nil
}

func (im *inmem) Watch() (config.Watcher, error) {
	w, err := NewWatcher()
	if err != nil {
		return nil, err
	}
	return w, nil
}
