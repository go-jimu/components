package taskqueue

// Option configures task metadata.
type Option func(*taskConfig)

type taskConfig struct {
	id           string
	headers      map[string]string
	payloadCodec string
}

// WithID sets a caller-assigned task identity that names one unit of work.
// Enqueue returns ErrDuplicateTask while the provider retains a task with the
// same ID. See Task.ID.
func WithID(id string) Option {
	return func(cfg *taskConfig) {
		cfg.id = id
	}
}

// WithHeader adds one task metadata header.
func WithHeader(key, value string) Option {
	return func(cfg *taskConfig) {
		if cfg.headers == nil {
			cfg.headers = make(map[string]string)
		}
		cfg.headers[key] = value
	}
}

// WithHeaders adds task metadata headers.
func WithHeaders(headers map[string]string) Option {
	return func(cfg *taskConfig) {
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

// WithPayloadCodec records the codec used to encode raw task payload bytes.
func WithPayloadCodec(codecName string) Option {
	return func(cfg *taskConfig) {
		cfg.payloadCodec = normalizePayloadCodec(codecName)
	}
}
