package proto

import (
	"reflect"
	"testing"

	testData "github.com/go-jimu/components/encoding/testdata"
)

func TestName(t *testing.T) {
	c := new(codec)
	if !reflect.DeepEqual(c.Name(), "proto") {
		t.Errorf("no expect float_key value: %v, but got: %v", c.Name(), "proto")
	}
}

func TestCodec(t *testing.T) {
	c := new(codec)

	model := testData.TestModel{
		Id:    1,
		Name:  "kratos",
		Hobby: []string{"study", "eat", "play"},
	}

	m, err := c.Marshal(&model)
	if err != nil {
		t.Errorf("Marshal() should be nil, but got %s", err)
	}

	var res testData.TestModel

	err = c.Unmarshal(m, &res)
	if err != nil {
		t.Errorf("Unmarshal() should be nil, but got %s", err)
	}
	if !reflect.DeepEqual(res.Id, model.Id) {
		t.Errorf("ID should be %d, but got %d", res.Id, model.Id)
	}
	if !reflect.DeepEqual(res.Name, model.Name) {
		t.Errorf("Name should be %s, but got %s", res.Name, model.Name)
	}
	if !reflect.DeepEqual(res.Hobby, model.Hobby) {
		t.Errorf("Hobby should be %s, but got %s", res.Hobby, model.Hobby)
	}
}

func TestCodec2(t *testing.T) {
	c := new(codec)

	model := testData.TestModel{
		Id:    1,
		Name:  "kratos",
		Hobby: []string{"study", "eat", "play"},
	}

	m, err := c.Marshal(&model)
	if err != nil {
		t.Errorf("Marshal() should be nil, but got %s", err)
	}

	var res testData.TestModel
	rp := &res

	err = c.Unmarshal(m, &rp)
	if err != nil {
		t.Errorf("Unmarshal() should be nil, but got %s", err)
	}
	if !reflect.DeepEqual(res.Id, model.Id) {
		t.Errorf("ID should be %d, but got %d", res.Id, model.Id)
	}
	if !reflect.DeepEqual(res.Name, model.Name) {
		t.Errorf("Name should be %s, but got %s", res.Name, model.Name)
	}
	if !reflect.DeepEqual(res.Hobby, model.Hobby) {
		t.Errorf("Hobby should be %s, but got %s", res.Hobby, model.Hobby)
	}
}

func Test_getProtoMessage(t *testing.T) {
	p := &testData.TestModel{Id: 1}
	type args struct {
		v interface{}
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "test1", args: args{v: &testData.TestModel{}}, wantErr: false},
		{name: "test2", args: args{v: testData.TestModel{}}, wantErr: true},
		{name: "test3", args: args{v: &p}, wantErr: false},
		{name: "test4", args: args{v: 1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getProtoMessage(tt.args.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("getProtoMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

// Intent: the codec is looked up by name, so callers cannot see from the type
// that it only accepts proto messages. Invalid input must return an error
// instead of panicking.
func TestCodecRejectsInvalidInputWithoutPanic(t *testing.T) {
	c := codec{}
	data, err := c.Marshal(&testData.TestModel{Id: 1})
	if err != nil {
		t.Fatal(err)
	}
	var nilModel *testData.TestModel
	var nilInt *int

	tests := []struct {
		name string
		call func() error
	}{
		{name: "marshal non-proto", call: func() error { _, marshalErr := c.Marshal(1); return marshalErr }},
		{name: "marshal nil", call: func() error { _, marshalErr := c.Marshal(nil); return marshalErr }},
		{name: "marshal nil message pointer", call: func() error { _, marshalErr := c.Marshal(nilModel); return marshalErr }},
		{name: "marshal pointer to nil message pointer", call: func() error {
			_, marshalErr := c.Marshal(&nilModel)
			return marshalErr
		}},
		{name: "marshal struct value", call: func() error {
			_, marshalErr := c.Marshal(testData.TestModel{})
			return marshalErr
		}},
		{name: "unmarshal nil", call: func() error { return c.Unmarshal(data, nil) }},
		{name: "unmarshal nil message pointer", call: func() error { return c.Unmarshal(data, nilModel) }},
		{name: "unmarshal nil non-proto pointer", call: func() error { return c.Unmarshal(data, nilInt) }},
		{name: "unmarshal non-proto pointer", call: func() error { n := 0; return c.Unmarshal(data, &n) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var callErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panicked: %v", r)
					}
				}()
				callErr = tt.call()
			}()
			if callErr == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// Intent: like encoding/json, unmarshaling into a pointer to a nil message
// pointer allocates the message.
func TestCodecUnmarshalAllocatesNilMessagePointer(t *testing.T) {
	c := codec{}
	data, err := c.Marshal(&testData.TestModel{Id: 7, Name: "kratos"})
	if err != nil {
		t.Fatal(err)
	}

	var res *testData.TestModel
	if err = c.Unmarshal(data, &res); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if res == nil || res.Id != 7 || res.Name != "kratos" {
		t.Fatalf("Unmarshal() got %v", res)
	}
}

// Intent: Marshal accepts the same pointer-to-message shape as Unmarshal.
func TestCodecMarshalAcceptsPointerToMessage(t *testing.T) {
	c := codec{}
	model := &testData.TestModel{Id: 3}

	data, err := c.Marshal(&model)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var res testData.TestModel
	if err = c.Unmarshal(data, &res); err != nil {
		t.Fatal(err)
	}
	if res.Id != 3 {
		t.Fatalf("Id = %d, want 3", res.Id)
	}
}
