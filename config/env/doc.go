// Package env provides config.Source implementations backed by environment
// variables. Pick the constructor by how the variables reach the
// configuration:
//
//   - NewOverrideSource, to override nested configuration paths directly.
//   - NewSource, to feed ${NAME} placeholders through flat keys.
//
// Both read the environment once per Load and never report a change.
//
// # Override source
//
// NewOverrideSource maps each variable to a configuration path: strip the
// prefix, lower-case the rest, and split it into levels at each "__". With
// prefix "APP":
//
//	APP_PORT=9090                 ->  port
//	APP_DATABASE__MAX_CONN=20     ->  database.max_conn
//	APP_HTTP__TLS__ENABLED=true   ->  http.tls.enabled
//
// A name that yields an empty level, such as APP_A____B, or contains "." is
// skipped.
//
// List it last in config.WithSource so its values override the files:
//
//	cfg := config.New(config.WithSource(
//		file.NewSource("./configs"),
//		env.NewOverrideSource("APP"),
//	))
//
// Values arrive as strings; Scan and Value convert them to the target kind,
// so "20" fills an int field. Levels address map keys, so write configuration
// keys in lower case to make them reachable, and override a slice as a whole
// from a file rather than element by element. Pass a prefix: without one,
// every variable becomes a key, and USER would override a "user" key.
//
// # Flat source
//
// NewSource keeps each variable name as one flat key, with case preserved:
// APP_SERVICE_NAME with prefix "APP" becomes key SERVICE_NAME. Reference
// those keys from configuration files with placeholders such as
// ${SERVICE_NAME:default}.
package env
