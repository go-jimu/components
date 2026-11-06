// Package json registers the JSON codec with the encoding registry under the
// name "json".
//
// The codec encodes a protobuf message with protojson, configured by
// MarshalOptions and UnmarshalOptions, and any other value with the standard
// encoding/json. A value implementing json.Marshaler or json.Unmarshaler uses
// its own method instead. Unmarshal also accepts a pointer to a protobuf
// message pointer, allocating the message when it is nil.
package json
