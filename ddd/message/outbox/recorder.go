package outbox

import (
	"context"

	"github.com/go-jimu/components/ddd/message"
)

type Recorder interface {
	Record(ctx context.Context, messages ...message.Message) error
}

type RecorderOption func(*recorderConfig)

type recorderConfig struct{}

type StoreRecorder struct {
	store Store
	codec Codec
}

func NewRecorder(store Store, codec Codec, opts ...RecorderOption) (*StoreRecorder, error) {
	if store == nil {
		return nil, ErrNilStore
	}
	if codec == nil {
		return nil, ErrNilCodec
	}
	cfg := recorderConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &StoreRecorder{store: store, codec: codec}, nil
}

func (r *StoreRecorder) Record(ctx context.Context, messages ...message.Message) error {
	if len(messages) == 0 {
		return nil
	}
	records := make([]Record, 0, len(messages))
	for _, msg := range messages {
		record, err := r.codec.Encode(msg)
		if err != nil {
			return err
		}
		records = append(records, record)
	}
	return r.store.Append(ctx, records...)
}
