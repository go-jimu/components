package message

import "time"

// Option sets optional message metadata in New.
type Option func(*messageConfig)

type messageConfig struct {
	id         string
	idSet      bool
	key        string
	occurredAt time.Time
	headers    map[string]string
}

// WithID sets the unique message instance ID instead of a generated one. An
// empty id makes New return ErrEmptyID.
func WithID(id string) Option {
	return func(cfg *messageConfig) {
		cfg.id = id
		cfg.idSet = true
	}
}

// WithKey sets the ordering or routing group, usually the ID of the aggregate
// the message is about. Providers keep messages with the same key in order
// where they support it, as Kafka does per partition.
func WithKey(key string) Option {
	return func(cfg *messageConfig) {
		cfg.key = key
	}
}

// WithOccurredAt sets when the business fact occurred. Without it, or with a
// zero time, New uses the current time.
func WithOccurredAt(occurredAt time.Time) Option {
	return func(cfg *messageConfig) {
		cfg.occurredAt = occurredAt
	}
}

// WithHeader adds one extension header, such as a trace ID. A later header
// with the same key replaces an earlier one. Providers may reserve names for
// their own metadata.
func WithHeader(key, value string) Option {
	return func(cfg *messageConfig) {
		if cfg.headers == nil {
			cfg.headers = make(map[string]string)
		}
		cfg.headers[key] = value
	}
}

// WithHeaders adds every entry of headers as WithHeader does. The map is
// copied.
func WithHeaders(headers map[string]string) Option {
	return func(cfg *messageConfig) {
		if len(headers) == 0 {
			return
		}
		if cfg.headers == nil {
			cfg.headers = make(map[string]string, len(headers))
		}
		for key, value := range headers {
			cfg.headers[key] = value
		}
	}
}
