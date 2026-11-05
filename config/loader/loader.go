package loader

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-jimu/components/config"
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

	// Option configures Load.
	Option func(*loadOptions)
)

var defaultLoadOption = loadOptions{
	ConfigurationDirectory:            "./configs",
	DefaultConfigFileWithoutExtension: "defaults",
	EnvVarsPrefix:                     "",
	ProfilesAlias:                     "",
}

const (
	// ProfilesActiveFromEnvVar names the environment variable that selects
	// the profile alias. When it is set, it overrides WithProfilesAlias.
	ProfilesActiveFromEnvVar = "JIMU_PROFILES_ACTIVE"
)

// Load reads the defaults file and the active profile file, merges them, and
// scans the result into v, a pointer to a struct with json tags.
//
// It returns an error when the configuration directory cannot be read, when
// a profile alias is active without WithConfigFilePrefix, or when a file
// cannot be parsed or scanned into v. When no file matches, it returns nil and
// leaves v unchanged, so set defaults in v before calling Load.
func Load(v any, opts ...Option) error {
	o := new(loadOptions)
	*o = defaultLoadOption
	opts = append(opts, WithProfilesActiveFromEnvVar())
	for _, opt := range opts {
		opt(o)
	}

	sources, err := searchConfigInDir(o)
	if err != nil {
		return err
	}

	// Environment variables only feed ${NAME:default} placeholders. They are
	// not merged as an override layer, because their flat keys cannot address
	// nested configuration paths.
	conf := config.New(
		config.WithSource(sources...),
		config.WithLookup(envLookup(o.EnvVarsPrefix)),
	)
	// Load starts source watchers; a one-shot load must stop them. A watcher
	// that fails to stop does not change the values already scanned into v.
	defer func() { _ = conf.Close() }()
	if err = conf.Load(); err != nil {
		return err
	}
	return conf.Scan(v)
}

// envLookup resolves placeholder ${NAME} from environment variable prefix+NAME.
func envLookup(prefix string) config.Lookup {
	return func(name string) (string, bool) {
		return os.LookupEnv(prefix + name)
	}
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

// WithConfigurationDirectory sets the directory to search, "./configs" by
// default, and the defaults file name without extension, "defaults" by
// default.
func WithConfigurationDirectory(dir string, defaults string) Option {
	return func(opt *loadOptions) {
		opt.ConfigurationDirectory = dir
		opt.DefaultConfigFileWithoutExtension = defaults
	}
}

// WithConfigFilePrefix sets the profile file prefix: alias "dev" with prefix
// "app" reads "app_dev.<ext>". It is required when an alias is active.
func WithConfigFilePrefix(prefix string) Option {
	return func(opt *loadOptions) {
		opt.ConfigFilePrefix = strings.TrimSpace(prefix)
	}
}

// WithEnvVarsPrefix sets the prefix used to resolve ${NAME} placeholders:
// with prefix "app", ${NAME} reads APP_NAME. The prefix is upper-cased and
// gets a trailing "_". An empty prefix reads NAME itself, the default.
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

// WithProfilesAlias activates the profile alias, lower-cased, such as "dev".
// The JIMU_PROFILES_ACTIVE environment variable overrides it when set.
func WithProfilesAlias(alias string) Option {
	return func(opt *loadOptions) {
		if alias != "" {
			opt.ProfilesAlias = "_" + strings.ToLower(alias)
			return
		}
		opt.ProfilesAlias = ""
	}
}

// WithProfilesActiveFromEnvVar activates the alias named by the
// JIMU_PROFILES_ACTIVE environment variable when it is set. Load always
// applies it after the caller's options, so callers need not pass it.
func WithProfilesActiveFromEnvVar() Option {
	return func(opt *loadOptions) {
		profiles := os.Getenv(ProfilesActiveFromEnvVar)
		if profiles == "" {
			return
		}
		WithProfilesAlias(strings.ToLower(profiles))(opt)
	}
}
