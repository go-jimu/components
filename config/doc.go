// Package config provides source-based configuration loading, merging,
// placeholder resolution, value lookup, scanning, and watching.
//
// Use config.New when composing explicit sources such as config/file,
// config/env, and config/inmem. Use config/loader for the repository's
// opinionated defaults-plus-profile loading flow.
package config
