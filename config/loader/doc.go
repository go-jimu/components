// Package loader provides an opinionated application configuration loader.
//
// Load walks a configuration directory (./configs by default, including
// subdirectories) and reads two kinds of files, in this order:
//
//   - the defaults file, named <defaults>.<ext>, "defaults.yml" by default;
//   - when a profile alias is active, the profile file named
//     <prefix>_<alias>.<ext>, such as "app_dev.yml" for
//     WithConfigFilePrefix("app") and alias "dev".
//
// The extension selects the codec: .json, .yaml, .yml, or .toml. Values in the
// profile file override the defaults. Load then scans the merged result into
// the caller's struct through encoding/json, so tag struct fields with json
// tags.
//
// The alias comes from the environment variable JIMU_PROFILES_ACTIVE, or from
// WithProfilesAlias when that variable is unset. An active alias requires
// WithConfigFilePrefix.
//
// Environment variables are not merged as an override layer. Reference them
// from configuration files with ${NAME:default} placeholders; NAME is looked up
// as the environment variable prefix+NAME, where the prefix is set by
// WithEnvVarsPrefix. Load is one-shot and does not keep watching sources; use
// the config package directly to watch.
package loader
