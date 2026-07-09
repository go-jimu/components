package loader_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-jimu/components/config/loader"
)

type exampleOptions struct {
	Name string `yaml:"name"`
	Age  int    `yaml:"age"`
}

func ExampleLoad() {
	oldProfile, hadProfile := os.LookupEnv(loader.ProfilesActiveFromEnvVar)
	os.Unsetenv(loader.ProfilesActiveFromEnvVar)
	defer func() {
		if hadProfile {
			os.Setenv(loader.ProfilesActiveFromEnvVar, oldProfile)
			return
		}
		os.Unsetenv(loader.ProfilesActiveFromEnvVar)
	}()

	dir, err := os.MkdirTemp("", "components-loader-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	if err := os.WriteFile(filepath.Join(dir, "defaults.yml"), []byte("name: default\nage: 18\n"), 0o600); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app_dev.yml"), []byte("name: dev\n"), 0o600); err != nil {
		panic(err)
	}

	var opts exampleOptions
	err = loader.Load(
		&opts,
		loader.WithConfigurationDirectory(dir, "defaults"),
		loader.WithConfigFilePrefix("app"),
		loader.WithProfilesAlias("dev"),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(opts.Name, opts.Age)

	// Output:
	// dev 18
}
