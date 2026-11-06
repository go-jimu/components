// Package loader loads application configuration into a struct from a
// defaults file, an optional profile file, and environment variables.
//
// Load walks a configuration directory, including subdirectories, and reads
// two kinds of files, in this order:
//
//   - the defaults file, named <defaults>.<ext>, "defaults.yml" by default;
//   - when a profile alias is active, the profile file named
//     <prefix>_<alias>.<ext>, such as "app_dev.yml" for
//     WithConfigFilePrefix("app") and alias "dev".
//
// The directory is "./configs" by default, relative to the working
// directory. The extension selects the codec: .json, .yaml, .yml, or .toml.
// Values in the profile file override the defaults. Load then scans the
// merged result into the caller's struct; see "Scanning" in the config
// package documentation for json tags and string conversion.
//
// The alias comes from the environment variable JIMU_PROFILES_ACTIVE, or from
// WithProfilesAlias when that variable is unset. An active alias requires
// WithConfigFilePrefix.
//
// Load reads the files once. To react to changes, compose config.New
// directly; see "Reload" in the config package documentation.
//
// # Environment variables
//
// WithEnvVarsPrefix sets one prefix, such as "app" for APP_*, for both ways
// environment variables reach the configuration:
//
//   - Placeholders, always on: ${NAME:default} in a file reads APP_NAME.
//   - Overrides, with WithEnvOverrides: APP_DATABASE__MAX_CONN overrides
//     database.max_conn, as the last layer over the defaults and profile
//     files. Names map to paths as described under "Override source" in the
//     config/env package documentation.
//
// With both on, one variable serves both ways: APP_PORT fills ${PORT} and
// also overrides a top-level "port" key. An override replaces a value before
// placeholders resolve, so APP_SERVER__PORT wins over a ${PORT} written at
// server.port. WithEnvOverrides requires a non-empty prefix, so unrelated
// variables such as USER stay out of the configuration.
package loader
