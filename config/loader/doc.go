// Package loader provides an opinionated application configuration loader.
//
// It loads defaults from a configuration directory, optionally overlays an
// explicit profile file selected by alias or environment variable, and scans the
// merged result into a caller-provided struct.
package loader
