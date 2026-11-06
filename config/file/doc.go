// Package file provides a config.Source backed by a file or a directory.
//
// NewSource(path) loads the file at path, or every file directly in the
// directory at path, in name order, so a later name overrides an earlier
// one. Subdirectories and names that start with "." are skipped. Each file
// is one entry keyed by its file name and decoded by the codec its
// extension names, case-insensitively: .json, .yaml, .yml, or .toml. A file
// without an extension loads as one string value under its name, and any
// other extension fails Load, so keep only configuration files in the
// directory.
//
// The watcher reports each file written, created, or renamed into place, so
// the Config reloads it; a removed file keeps its values until the next
// Load.
package file
