# encoding

`encoding` is a registry of codecs keyed by name. The rules and the built-in
codec packages live in the package documentation, which `go doc -all
github.com/go-jimu/components/encoding` prints; `example_test.go` runs the
lookup.

## Usage

```go
import (
	"github.com/go-jimu/components/encoding"
	_ "github.com/go-jimu/components/encoding/json"
)

codec := encoding.GetCodec("json")
if codec == nil {
	return errors.New("json codec is not registered")
}
data, err := codec.Marshal(value)
```
