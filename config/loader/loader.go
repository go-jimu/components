package loader

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-jimu/components/config"
	"github.com/go-jimu/components/config/env"
	"github.com/go-jimu/components/config/file"
)

type (
	loadOptions struct {
		ConfigurationDirectory            string // eg.configs
		DefaultConfigFileWithoutExtension string // eg. defaults
		ConfigFilePrefix                  string // eg. foobar
		EnvVarsPrefix                     string // eg. APP_
		ProfilesAlias                     string // eg. test
	}

	Option func(*loadOptions)
)

var defaultLoadOption = loadOptions{
	ConfigurationDirectory:            "./configs",
	DefaultConfigFileWithoutExtension: "defaults",
	EnvVarsPrefix:                     "",
	ProfilesAlias:                     "",
}

const (
	// ProfilesActiveFromEnvVar Environment variable for switching profiles.
	// When this environment variable is set, it will override `ProfilesAlias`.
	ProfilesActiveFromEnvVar = "JIMU_PROFILES_ACTIVE"
)

func Load(v any, opts ...Option) error {
	o := new(loadOptions)
	*o = defaultLoadOption
	opts = append(opts, WithProfilesActiveFromEnvVar())
	for _, opt := range opts {
		opt(o)
	}

	// load from config files
	sources, err := searchConfigInDir(o)
	if err != nil {
		return err
	}
	// load from environment vars
	sources = append(sources, env.NewSource(o.EnvVarsPrefix))

	conf := config.New(config.WithSource(sources...))
	if err = conf.Load(); err != nil {
		return err
	}
	return conf.Scan(v)
}

func searchConfigInDir(opts *loadOptions) ([]config.Source, error) {
	var defaultSource config.Source
	var extends []config.Source

	if opts.ProfilesAlias != "" && opts.ConfigFilePrefix == "" {
		return nil, fmt.Errorf("config file prefix is required when profiles alias is set")
	}

	path, err := filepath.Abs(opts.ConfigurationDirectory)
	if err != nil {
		return nil, err
	}

	profileConfigFile := opts.ConfigFilePrefix + opts.ProfilesAlias
	if err = filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		nonExt := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		if nonExt == opts.DefaultConfigFileWithoutExtension {
			defaultSource = file.NewSource(path)
			return nil
		}

		if opts.ProfilesAlias != "" && nonExt == profileConfigFile {
			extends = append(extends, file.NewSource(path))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if defaultSource != nil {
		o := []config.Source{defaultSource}
		return append(o, extends...), nil
	}
	return extends, nil
}

func WithConfigurationDirectory(dir string, defaults string) Option {
	return func(opt *loadOptions) {
		opt.ConfigurationDirectory = dir
		opt.DefaultConfigFileWithoutExtension = defaults
	}
}

func WithConfigFilePrefix(prefix string) Option {
	return func(opt *loadOptions) {
		opt.ConfigFilePrefix = strings.TrimSpace(prefix)
	}
}

func WithEnvVarsPrefix(prefix string) Option {
	return func(opt *loadOptions) {
		if prefix != "" {
			opt.EnvVarsPrefix = strings.ToUpper(prefix)
			if !strings.HasSuffix(opt.EnvVarsPrefix, "_") {
				opt.EnvVarsPrefix += "_"
			}
			return
		}
		opt.EnvVarsPrefix = ""
	}
}

func WithProfilesAlias(alias string) Option {
	return func(opt *loadOptions) {
		if alias != "" {
			opt.ProfilesAlias = "_" + strings.ToLower(alias)
			return
		}
		opt.ProfilesAlias = ""
	}
}

func WithProfilesActiveFromEnvVar() Option {
	return func(opt *loadOptions) {
		profiles := os.Getenv(ProfilesActiveFromEnvVar)
		if profiles == "" {
			return
		}
		WithProfilesAlias(strings.ToLower(profiles))(opt)
	}
}
