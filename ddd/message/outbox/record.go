package outbox

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/go-jimu/components/ddd/message"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusPublished  Status = "published"
	StatusFailed     Status = "failed"
)

type Record struct {
	ID         string
	MessageID  string
	Kind       message.Kind
	Key        string
	OccurredAt time.Time
	Payload    []byte
	Headers    map[string]string

	Status        Status
	Attempts      int
	NextAttemptAt time.Time
	LockedUntil   time.Time
	ClaimedBy     string
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (r Record) Clone() Record {
	r.Payload = cloneBytes(r.Payload)
	r.Headers = cloneHeaders(r.Headers)
	return r
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

func cloneHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	copied := make(map[string]string, len(headers))
	for key, value := range headers {
		copied[key] = value
	}
	return copied
}

func generateID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
