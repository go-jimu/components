// Package encoding is a registry of codecs, each registered under a name such
// as "json".
//
// # Using a codec
//
// Blank import the package of each codec in use; its init registers the codec.
// Then look the codec up by name with GetCodec:
//
//	import _ "github.com/go-jimu/components/encoding/json"
//
//	codec := encoding.GetCodec("json")
//
// The built-in codec packages are encoding/json, encoding/yaml, encoding/toml,
// and encoding/proto; each package documentation states its names.
//
// # Writing a codec
//
// Implement Codec and call RegisterCodec from the package's init.
package encoding
