// Package loader provides an opinionated application configuration loader.
//
// It loads defaults from a configuration directory, optionally overlays an
// explicit profile file selected by alias or environment variable, and scans the
// merged result into a caller-provided struct.
//
// Environment variables are not merged as an override layer. Reference them
// from configuration files with ${NAME:default} placeholders; NAME is looked up
// as the environment variable prefix+NAME, where the prefix is set by
// WithEnvVarsPrefix. Load is one-shot and does not keep watching sources.
package loader
