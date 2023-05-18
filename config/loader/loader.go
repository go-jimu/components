package loader

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/go-jimu/components/config"
	"github.com/go-jimu/components/config/env"
	"github.com/go-jimu/components/config/file"
)

type (
	loadOptions struct {
		ConfigDirectory                   string // eg.configs
		DefaultConfigFileWithoutExtension string // eg. default
		EnvVarsPrefix                     string // eg. APP_
		ProfilesAlias                     string // eg, test,beta,prod
	}

	Option func(*loadOptions)
)

var defaultLoadOption = loadOptions{
	ConfigDirectory:                   "./configs",
	DefaultConfigFileWithoutExtension: "default",
	EnvVarsPrefix:                     "",
	ProfilesAlias:                     "",
}

func Load(v any, opts ...Option) error {
	o := new(loadOptions)
	*o = defaultLoadOption
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

	path, err := filepath.Abs(opts.ConfigDirectory)
	if err != nil {
		return nil, err
	}

	_ = filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		nonExt := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		if nonExt == opts.DefaultConfigFileWithoutExtension {
			defaultSource = file.NewSource(path)
			return nil
		}

		if strings.HasSuffix(nonExt, opts.ProfilesAlias) {
			extends = append(extends, file.NewSource(path))
		}
		return nil
	})
	if defaultSource != nil {
		o := []config.Source{defaultSource}
		return append(o, extends...), nil
	}
	return extends, nil
}

func WithConfigurat(dir string, defaults string) Option {
	return func(opt *loadOptions) {
		opt.ConfigDirectory = dir
		opt.DefaultConfigFileWithoutExtension = defaults
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
		}
		opt.ProfilesAlias = ""
	}
}
