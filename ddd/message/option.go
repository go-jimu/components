package message

import "time"

type Option func(*messageConfig)

type messageConfig struct {
	id         string
	idSet      bool
	key        string
	occurredAt time.Time
	headers    map[string]string
}

func WithID(id string) Option {
	return func(cfg *messageConfig) {
		cfg.id = id
		cfg.idSet = true
	}
}

func WithKey(key string) Option {
	return func(cfg *messageConfig) {
		cfg.key = key
	}
}

func WithOccurredAt(occurredAt time.Time) Option {
	return func(cfg *messageConfig) {
		cfg.occurredAt = occurredAt
	}
}

func WithHeader(key, value string) Option {
	return func(cfg *messageConfig) {
		if cfg.headers == nil {
			cfg.headers = make(map[string]string)
		}
		cfg.headers[key] = value
	}
}

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
