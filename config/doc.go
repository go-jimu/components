// Package config loads configuration from Sources, merges it, resolves
// ${name:default} placeholders, and serves it by path through Value or as a
// struct through Scan. Watched sources reload the configuration in place.
//
// Read by task:
//   - Loading application configuration into a struct: start at
//     config/loader, which composes this package from defaults, profile
//     files, and environment variables.
//   - Composing sources: "Sources", "Placeholders".
//   - Reading values: "Values", "Scanning".
//   - Reacting to changes: "Reload".
//   - Writing a Source: "Writing a source".
//
// # Sources
//
// Pass every source to one WithSource call, lowest priority first; a later
// WithSource replaces the list. Load reads the sources in order and merges
// them key by key: maps merge recursively, and any other value, a slice
// included, replaces the earlier one whole.
//
//	cfg := config.New(config.WithSource(
//		file.NewSource("./configs"),  // config/file: a file or a directory
//		env.NewOverrideSource("APP"), // config/env: APP_* overrides, last
//	))
//	if err := cfg.Load(); err != nil {
//		return err
//	}
//	defer func() { _ = cfg.Close() }()
//
// config/inmem supplies bytes held in memory, for tests and embedded
// defaults.
//
// Load returns the first error from a source, a decode, or a merge, and
// keeps the previous configuration. Call Load again to retry: each call
// starts over from the sources, and the sources are watched once. Close
// stops the watchers.
//
// # Placeholders
//
// A string value may hold ${name} or ${name:default} placeholders. They
// resolve after the merge, so they see every source, and each one takes
// the first of:
//  1. the WithLookup function, such as an environment variable lookup;
//  2. the value at configuration path name, such as ${server.host};
//  3. the default;
//  4. the empty string.
//
// A placeholder inside a longer string is interpolated as text. Resolved
// values are strings, which Value and Scan convert to the target kind. With
// WithResolveActualTypes(true), a value that is exactly one placeholder takes
// the type of its resolved text instead: a number, a bool, or, when quoted, a
// string.
//
// Point a path placeholder at a value that holds no placeholder: the
// referenced value may be read before its own placeholders resolve.
//
// WithResolver replaces this resolution, WithLookup included.
//
// Environment variables reach the configuration in two ways: a placeholder
// read through WithLookup fills the spot it sits in, and
// env.NewOverrideSource, listed last, overrides a path with no placeholder
// in the files. See the config/env package documentation.
//
// # Values
//
// Value takes a dotted path, such as "server.http.addr", and returns a Value
// that tracks the key for the life of the Config: a reload updates it in
// place, whatever the new type. While the key is removed, its methods return
// ErrNotFound and its Load returns nil. For a key absent at the call, Value
// returns a detached Value that only reports ErrNotFound; call Value again
// once the key exists.
//
// The accessors convert between scalar kinds: Int, Float, and Bool parse
// strings, Int truncates floats, String formats numbers and bools, and
// Duration accepts "5s" or an integer number of nanoseconds. Slice and Map
// wrap each element in a Value that does not track reloads. Value.Scan
// decodes like Config.Scan. A value that does not convert to the requested
// kind returns ErrTypeAssert wrapped.
//
// # Scanning
//
// Scan decodes the whole configuration into a pointer through
// encoding/json, so json tags name the fields. Before decoding, string
// values are converted to the kind of their target field: "8080" fills an
// int, "true" a bool, and "5s" or "1m30s" a time.Duration. Numbers and bools
// fill string fields and fields tagged ",string". Types that implement
// json.Unmarshaler or encoding.TextUnmarshaler, such as time.Time, receive
// the value as written. encoding/json reports values that cannot be
// converted, such as "NaN" for an int.
//
// Fields absent from the configuration keep their values, so set defaults in
// the struct before Scan.
//
// # Reload
//
// When a watched source reports a change, the Config rebuilds the whole
// configuration from every source in order, so source priority holds, then
// resolves placeholders and notifies observers. Readers see either the old
// or the new configuration. A reload that fails to decode or merge is
// logged and keeps the previous configuration. A Watcher error is logged and
// retried with backoff, from one second up to 30.
//
// Watch registers an Observer for a key. The Observer receives the key and
// its tracked Value each time a reload, or a repeated Load, changes the
// value, including when the key is removed and when it comes back. Observers
// of one Config run one at a time, in change order, on the goroutine that
// made the change: the changed source's watcher, or the caller of Load. A
// slow Observer delays that source's next reload. An Observer may call Value
// and Watch.
//
//	if err := cfg.Watch("log.level", func(key string, v config.Value) {
//		level, err := v.String() // ErrNotFound while the key is removed
//		...
//	}); err != nil {
//		return err // ErrNotFound: the key is absent
//	}
//
// # Writing a source
//
// Load returns the source's entries as KeyValues. Format names the codec,
// registered in the encoding package, that decodes Value; this package
// registers json, yaml, yml, toml, and proto. With an empty Format, Value is
// stored as a string at Key, a dotted Key nesting maps. Key identifies the
// entry within its source.
//
// Watch returns a Watcher. Next blocks until entries change and returns the
// changed ones: each replaces the source's entry with the same Key, or adds
// one; an empty result changes nothing. An entry stays until the next Load,
// even when the Watcher stops reporting it. Stop makes Next return an error
// wrapping context.Canceled, which ends the watch; any other error is
// retried, see "Reload". A source that never changes returns a Watcher whose
// Next blocks until Stop, such as env.NewWatcher.
package config
