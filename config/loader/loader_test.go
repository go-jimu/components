package loader_test

import (
	"testing"

	"github.com/go-jimu/components/config/loader"
	"github.com/stretchr/testify/assert"
)

type Option struct {
	Name string `yaml:"name"`
	Age  int    `yaml:"age"`
}

func TestLoader(t *testing.T) {
	opt := new(Option)
	err := loader.Load(opt)
	assert.NoError(t, err)
	assert.EqualValues(t, opt, &Option{Name: "foobar", Age: 18})

	t.Setenv("JIMU_PROFILES_ACTIVE", "test")
	opt = new(Option)
	err = loader.Load(opt)
	assert.NoError(t, err)
	assert.EqualValues(t, opt, &Option{Name: "nihao", Age: 18})
}
