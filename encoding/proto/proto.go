// Package proto defines the protobuf codec. Importing this package will
// register the codec.
package proto

import (
	"fmt"
	"reflect"

	"github.com/go-jimu/components/encoding"
	"google.golang.org/protobuf/proto"
)

// Name is the name registered for the proto compressor.
const Name = "proto"

func init() {
	encoding.RegisterCodec(codec{})
}

// codec is a codec implementation with protobuf. It is the default codec for Transport.
type codec struct{}

func (codec) Marshal(v interface{}) ([]byte, error) {
	pm, err := getProtoMessage(v)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(pm)
}

func (codec) Unmarshal(data []byte, v interface{}) error {
	pm, err := getUnmarshalTarget(v)
	if err != nil {
		return err
	}
	return proto.Unmarshal(data, pm)
}

func (codec) Name() string {
	return Name
}

var protoMessageType = reflect.TypeOf((*proto.Message)(nil)).Elem()

// getProtoMessage returns the proto message v holds directly or through
// pointers.
func getProtoMessage(v interface{}) (proto.Message, error) {
	if msg, ok := v.(proto.Message); ok {
		return msg, nil
	}
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return nil, fmt.Errorf("not proto message: %T", v)
	}
	return getProtoMessage(val.Elem().Interface())
}

// getUnmarshalTarget returns a non-nil proto message to unmarshal into. Like
// encoding/json, it allocates a nil message that v points to.
func getUnmarshalTarget(v interface{}) (proto.Message, error) {
	val := reflect.ValueOf(v)
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil, fmt.Errorf("cannot unmarshal into nil %T", v)
		}
		if msg, ok := val.Interface().(proto.Message); ok {
			return msg, nil
		}
		elem := val.Elem()
		if elem.Kind() == reflect.Ptr && elem.IsNil() && elem.Type().Implements(protoMessageType) {
			elem.Set(reflect.New(elem.Type().Elem()))
		}
		val = elem
	}
	return nil, fmt.Errorf("not proto message: %T", v)
}
