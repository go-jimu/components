// Package encoding provides a registry for named codecs.
//
// Codec packages such as encoding/json, encoding/yaml, encoding/toml, and
// encoding/proto register themselves from init. Consumers blank import the
// codec packages they need, then call GetCodec by content subtype.
package encoding
