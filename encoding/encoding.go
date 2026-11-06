package encoding

import (
	"strings"
	"sync"
)

// Codec encodes and decodes values in one format. Implementations are safe
// for concurrent use.
type Codec interface {
	// Marshal returns the encoding of v.
	Marshal(v interface{}) ([]byte, error)
	// Unmarshal decodes data into v.
	Unmarshal(data []byte, v interface{}) error
	// Name returns the name the Codec is registered under. It returns the
	// same value on every call.
	Name() string
}

var (
	registryMu       sync.RWMutex
	registeredCodecs = make(map[string]Codec)
)

// RegisterCodec registers codec under its Name, ignoring case; registering a
// name again replaces the previous Codec. It panics when codec is nil or its
// Name is empty. See "Writing a codec".
func RegisterCodec(codec Codec) {
	if codec == nil {
		panic("cannot register a nil Codec")
	}
	if codec.Name() == "" {
		panic("cannot register Codec with empty string result for Name()")
	}
	contentSubtype := strings.ToLower(codec.Name())
	registryMu.Lock()
	defer registryMu.Unlock()
	registeredCodecs[contentSubtype] = codec
}

// GetCodec returns the Codec registered under the name contentSubtype,
// ignoring case, so a format taken from a file extension such as "YAML" finds
// the "yaml" Codec. It returns nil when no Codec has that name; blank import
// the codec's package to register it. See "Using a codec".
func GetCodec(contentSubtype string) Codec {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registeredCodecs[strings.ToLower(contentSubtype)]
}
