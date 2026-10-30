package encoding_test

import (
	"encoding/xml"
	"fmt"
	"runtime/debug"
	"sync"
	"testing"

	"github.com/go-jimu/components/encoding"
)

type codec struct{}

func (c codec) Marshal(v interface{}) ([]byte, error) {
	panic("implement me")
}

func (c codec) Unmarshal(data []byte, v interface{}) error {
	panic("implement me")
}

func (c codec) Name() string {
	return ""
}

// codec2 is a Codec implementation with xml.
type codec2 struct{}

func (codec2) Marshal(v interface{}) ([]byte, error) {
	return xml.Marshal(v)
}

func (codec2) Unmarshal(data []byte, v interface{}) error {
	return xml.Unmarshal(data, v)
}

func (codec2) Name() string {
	return "xml"
}

func TestRegisterCodec(t *testing.T) {
	f := func() { encoding.RegisterCodec(nil) }
	funcDidPanic, panicValue, _ := didPanic(f)
	if !funcDidPanic {
		t.Fatalf("func should panic\n\tPanic value:\t%#v", panicValue)
	}
	if panicValue != "cannot register a nil Codec" {
		t.Fatalf("panic error got %s want cannot register a nil Codec", panicValue)
	}
	f = func() {
		encoding.RegisterCodec(codec{})
	}
	funcDidPanic, panicValue, _ = didPanic(f)
	if !funcDidPanic {
		t.Fatalf("func should panic\n\tPanic value:\t%#v", panicValue)
	}
	if panicValue != "cannot register Codec with empty string result for Name()" {
		t.Fatalf("panic error got %s want cannot register Codec with empty string result for Name()", panicValue)
	}
	codec := codec2{}
	encoding.RegisterCodec(codec)
	got := encoding.GetCodec("xml")
	if got != codec {
		t.Fatalf("RegisterCodec(%v) want %v got %v", codec, codec, got)
	}
}

// PanicTestFunc defines a func that should be passed to the assert.Panics and assert.NotPanics
// methods, and represents a simple func that takes no arguments, and returns nothing.
type PanicTestFunc func()

// didPanic returns true if the function passed to it panics. Otherwise, it returns false.
func didPanic(f PanicTestFunc) (bool, interface{}, string) {
	didPanic := false
	var message interface{}
	var stack string
	func() {
		defer func() {
			if message = recover(); message != nil {
				didPanic = true
				stack = string(debug.Stack())
			}
		}()

		// call the target function
		f()
	}()

	return didPanic, message, stack
}

// namedCodec is a Codec whose name can be chosen by the test.
type namedCodec struct {
	codec2
	name string
}

func (c namedCodec) Name() string {
	return c.name
}

// Intent: lookups must match registration, which normalizes names to lowercase,
// so a format taken from a file extension such as "YAML" finds the codec.
func TestGetCodecIsCaseInsensitive(t *testing.T) {
	registered := namedCodec{name: "Case-Test"}
	encoding.RegisterCodec(registered)

	for _, name := range []string{"case-test", "CASE-TEST", "Case-Test"} {
		if got := encoding.GetCodec(name); got != registered {
			t.Fatalf("GetCodec(%q) = %v, want %v", name, got, registered)
		}
	}
}

// Intent: registering at runtime while other goroutines look up codecs must be
// race free.
func TestRegistryConcurrentRegisterAndGet(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			encoding.RegisterCodec(namedCodec{name: fmt.Sprintf("concurrent-%d", i)})
		}(i)
		go func() {
			defer wg.Done()
			_ = encoding.GetCodec("xml")
		}()
	}
	wg.Wait()

	for i := 0; i < 8; i++ {
		if encoding.GetCodec(fmt.Sprintf("concurrent-%d", i)) == nil {
			t.Fatalf("codec concurrent-%d not registered", i)
		}
	}
}
