# encoding

`encoding` is a small codec registry. Codec packages register themselves from
`init`, so consumers normally blank import the formats they want and then look
up a codec by content subtype.

## Usage

```go
import (
	"github.com/go-jimu/components/encoding"
	_ "github.com/go-jimu/components/encoding/json"
	_ "github.com/go-jimu/components/encoding/proto"
	_ "github.com/go-jimu/components/encoding/toml"
	_ "github.com/go-jimu/components/encoding/yaml"
)

codec := encoding.GetCodec("json")
data, err := codec.Marshal(value)
```

Built-in codec packages:

- `encoding/json`
- `encoding/yaml`
- `encoding/toml`
- `encoding/proto`
