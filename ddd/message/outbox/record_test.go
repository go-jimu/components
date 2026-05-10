package outbox_test

import (
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message"
	"github.com/go-jimu/components/ddd/message/outbox"
	"github.com/stretchr/testify/require"
)

// Clone must copy mutable payload and header data so store implementations can
// hand records to callers without exposing shared mutation.
func TestRecordCloneCopiesMutableFields(t *testing.T) {
	original := outbox.Record{
		ID:         "record-1",
		MessageID:  "message-1",
		Kind:       message.Kind("test.test_model"),
		Key:        "order-1",
		OccurredAt: time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		Payload:    []byte{1, 2, 3},
		Headers:    map[string]string{"tenant": "tenant-a"},
		Status:     outbox.StatusPending,
	}

	cloned := original.Clone()
	cloned.Payload[0] = 9
	cloned.Headers["tenant"] = "tenant-b"

	require.Equal(t, []byte{1, 2, 3}, original.Payload)
	require.Equal(t, map[string]string{"tenant": "tenant-a"}, original.Headers)
	require.Equal(t, "record-1", cloned.ID)
}
