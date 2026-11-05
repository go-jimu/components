package loader_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-jimu/components/config/loader"
)

type exampleOptions struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func ExampleLoad() {
	oldProfile, hadProfile := os.LookupEnv(loader.ProfilesActiveFromEnvVar)
	if err := os.Unsetenv(loader.ProfilesActiveFromEnvVar); err != nil {
		panic(err)
	}
	// Restoring the environment and removing dir are best-effort cleanup.
	defer func() {
		if hadProfile {
			_ = os.Setenv(loader.ProfilesActiveFromEnvVar, oldProfile)
			return
		}
		_ = os.Unsetenv(loader.ProfilesActiveFromEnvVar)
	}()

	dir, err := os.MkdirTemp("", "components-loader-example")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

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
