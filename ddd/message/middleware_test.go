package message_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/go-jimu/components/ddd/message"
	"github.com/stretchr/testify/require"
)

type logTraceKey struct{}

type loggedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]slog.Value
	trace any
}

type logRecorder struct {
	mu      sync.Mutex
	records []loggedRecord
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *logRecorder) Handle(ctx context.Context, record slog.Record) error {
	attrs := map[string]slog.Value{}
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, loggedRecord{record.Level, record.Message, attrs, ctx.Value(logTraceKey{})})
	return nil
}

func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *logRecorder) WithGroup(string) slog.Handler      { return r }

// Intent: Chain runs middleware in declaration order around Handle, skips nil
// middleware, and keeps the handler's kinds so Subscribe routes as before.
func TestChainOrderAndListening(t *testing.T) {
	var calls []string
	trace := func(name string) message.Middleware {
		return func(next message.HandleFunc) message.HandleFunc {
			return func(ctx context.Context, msg message.Message) error {
				calls = append(calls, name)
				return next(ctx, msg)
			}
		}
	}
	handler := handlerFunc{
		kinds: []message.Kind{"order.paid", "order.refunded"},
		handle: func(context.Context, message.Message) error {
			calls = append(calls, "handler")
			return nil
		},
	}

	chained := message.Chain(handler, trace("a"), nil, trace("b"))
	require.Equal(t, handler.Listening(), chained.Listening())
	require.NoError(t, chained.Handle(context.Background(), newTestMessage(t, "order.paid")))
	require.Equal(t, []string{"a", "b", "handler"}, calls)
}

// Intent: Chain of a nil handler is nil, so Subscribe still reports
// ErrNilHandler instead of panicking on a wrapped nil.
func TestChainNilHandler(t *testing.T) {
	require.Nil(t, message.Chain(nil, message.Logging(nil)))
	require.ErrorIs(t, message.NewRouter().Subscribe(message.Chain(nil)), message.ErrNilHandler)
}

// Intent: Logging records success and failure at Debug, since the provider's
// failure policy decides how serious a failure is, names the message, and
// returns the handler's error unchanged.
func TestLoggingRecordsHandleAtDebug(t *testing.T) {
	handlerErr := errors.New("db down")
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"handled", nil, "message handled"},
		{"failed", handlerErr, "message handler failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := &logRecorder{}
			handler := message.Chain(handlerFunc{
				kinds:  []message.Kind{"order.paid"},
				handle: func(context.Context, message.Message) error { return tt.err },
			}, message.Logging(slog.New(logs)))
			msg, err := message.New("order.paid", newTestMessage(t, "order.paid").Payload(),
				message.WithID("msg-1"), message.WithKey("order-1"))
			require.NoError(t, err)

			ctx := context.WithValue(context.Background(), logTraceKey{}, "trace-1")
			require.ErrorIs(t, handler.Handle(ctx, msg), tt.err)

			require.Len(t, logs.records, 1)
			rec := logs.records[0]
			require.Equal(t, slog.LevelDebug, rec.level)
			require.Equal(t, tt.msg, rec.msg)
			require.Equal(t, "trace-1", rec.trace)
			require.Equal(t, "order.paid", rec.attrs["message_kind"].String())
			require.Equal(t, "msg-1", rec.attrs["message_id"].String())
			require.Equal(t, "order-1", rec.attrs["message_key"].String())
			require.Equal(t, slog.KindInt64, rec.attrs["elapsed_ms"].Kind())
			errAttr, hasErr := rec.attrs["error"]
			require.Equal(t, tt.err != nil, hasErr)
			if tt.err != nil {
				require.Equal(t, slog.KindGroup, errAttr.Kind(), "error is logged with sloghelper.Error")
			}
		})
	}
}
