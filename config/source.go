package config

// KeyValue is one entry a Source loads: Value decoded by the codec Format
// names, identified within its source by Key. See "Writing a source".
type KeyValue struct {
	Key    string
	Value  []byte
	Format string
}

// Source supplies configuration entries to a Config. See "Writing a source".
type Source interface {
	Load() ([]*KeyValue, error)
	Watch() (Watcher, error)
}

// Watcher reports a Source's changed entries until Stop. See "Writing a
// source".
type Watcher interface {
	Next() ([]*KeyValue, error)
	Stop() error
}
