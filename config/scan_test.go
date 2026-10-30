package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scanEmbedded struct {
	Retries int `json:"retries"`
}

type scanTarget struct {
	scanEmbedded
	Port     int               `json:"port"`
	Ratio    float64           `json:"ratio"`
	Debug    bool              `json:"debug"`
	Password string            `json:"password"`
	Timeout  time.Duration     `json:"timeout"`
	Interval *time.Duration    `json:"interval"`
	Ports    []uint16          `json:"ports"`
	Limits   map[string]int    `json:"limits"`
	Nested   struct{ Max int } `json:"nested"`
	At       time.Time         `json:"at"`
	Ignored  int               `json:"-"`
}

// Placeholder and env values are strings; Scan should coerce them to the
// target field kinds while keeping literal values working.
func TestScanJSONCoercesToTargetFieldKinds(t *testing.T) {
	data := []byte(`{
		"retries": "3",
		"port": "8080",
		"ratio": "0.5",
		"debug": "true",
		"password": 123456,
		"timeout": "5s",
		"interval": "1m",
		"ports": ["80", 443],
		"limits": {"a": "1"},
		"nested": {"max": "7"},
		"at": "2026-01-02T03:04:05Z",
		"Ignored": "x"
	}`)

	var got scanTarget
	require.NoError(t, scanJSON(data, &got))

	interval := time.Minute
	assert.Equal(t, 3, got.Retries)
	assert.Equal(t, 8080, got.Port)
	assert.Equal(t, 0.5, got.Ratio)
	assert.True(t, got.Debug)
	assert.Equal(t, "123456", got.Password)
	assert.Equal(t, 5*time.Second, got.Timeout)
	assert.Equal(t, &interval, got.Interval)
	assert.Equal(t, []uint16{80, 443}, got.Ports)
	assert.Equal(t, map[string]int{"a": 1}, got.Limits)
	assert.Equal(t, 7, got.Nested.Max)
	assert.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), got.At)
	assert.Zero(t, got.Ignored)
}

// Literal numbers and nanosecond durations should keep their existing behavior.
func TestScanJSONKeepsLiteralValues(t *testing.T) {
	var got scanTarget
	require.NoError(t, scanJSON([]byte(`{"port": 80, "debug": false, "timeout": 1000}`), &got))

	assert.Equal(t, 80, got.Port)
	assert.False(t, got.Debug)
	assert.Equal(t, time.Duration(1000), got.Timeout)
}

// Values that cannot be coerced should still surface json's type error.
func TestScanJSONReportsUncoercibleValues(t *testing.T) {
	var got scanTarget
	assert.Error(t, scanJSON([]byte(`{"port": "eighty"}`), &got))
	assert.Error(t, scanJSON([]byte(`{"timeout": "soon"}`), &got))
}

// Large integers must not lose precision through the coercion round trip.
func TestScanJSONPreservesLargeIntegers(t *testing.T) {
	var got struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, scanJSON([]byte(`{"id": 9007199254740993}`), &got))
	assert.Equal(t, int64(9007199254740993), got.ID)
}

// WithLookup values should win over config paths, which still fall back to
// placeholder defaults.
func TestWithLookupResolvesPlaceholdersBeforeConfigPaths(t *testing.T) {
	lookup := func(name string) (string, bool) {
		if name == "PORT" {
			return "9090", true
		}
		return "", false
	}
	c := New(
		WithSource(newTestJSONSource(`{"host":"h","port":"${PORT:80}","addr":"${host}:${PORT}","name":"${NAME:svc}"}`)),
		WithLookup(lookup),
	)
	require.NoError(t, c.Load())

	var got struct {
		Port int    `json:"port"`
		Addr string `json:"addr"`
		Name string `json:"name"`
	}
	require.NoError(t, c.Scan(&got))
	assert.Equal(t, 9090, got.Port)
	assert.Equal(t, "h:9090", got.Addr)
	assert.Equal(t, "svc", got.Name)
}

// A ",string" field takes its value quoted, as encoding/json defines; numbers
// and bools are quoted for it and placeholder strings pass through.
func TestScanJSONHonorsStringOption(t *testing.T) {
	var got struct {
		Port    int     `json:"port,string"`
		Ratio   float64 `json:"ratio,string"`
		Enabled bool    `json:"enabled,omitempty,string"`
	}
	require.NoError(t, scanJSON([]byte(`{"port": "8080", "ratio": 0.5, "enabled": true}`), &got))
	assert.Equal(t, 8080, got.Port)
	assert.Equal(t, 0.5, got.Ratio)
	assert.True(t, got.Enabled)
}

type scanInner struct {
	Port int `json:"port"`
}

type scanOuterTagged struct {
	scanInner
	Port string `json:"port"`
}

type scanOuterUntagged struct {
	scanInner
	Port string
}

// The shallower field wins over a promoted field of the same name, as in
// encoding/json, so the value is coerced for the field that receives it.
func TestScanJSONShallowerFieldWins(t *testing.T) {
	var tagged scanOuterTagged
	require.NoError(t, scanJSON([]byte(`{"port": 8080}`), &tagged))
	assert.Equal(t, "8080", tagged.Port)
	assert.Zero(t, tagged.scanInner.Port)

	// An untagged field is named "Port", so the exact name "port" belongs to
	// the promoted field; Scan matches what encoding/json decodes.
	var untagged, want scanOuterUntagged
	require.NoError(t, scanJSON([]byte(`{"port": "8080"}`), &untagged))
	require.NoError(t, json.Unmarshal([]byte(`{"port": 8080}`), &want))
	assert.Equal(t, want, untagged)
	assert.Equal(t, 8080, untagged.scanInner.Port)
}

type scanLeft struct {
	Port int
}

type scanRight struct {
	Port int
}

// Two promoted fields with the same name at the same depth cancel out, as in
// encoding/json, and the value is ignored.
func TestScanJSONAmbiguousFieldsIgnored(t *testing.T) {
	var got struct {
		scanLeft
		scanRight
	}
	require.NoError(t, scanJSON([]byte(`{"port": "8080"}`), &got))
	assert.Zero(t, got.scanLeft.Port)
	assert.Zero(t, got.scanRight.Port)
}

// NaN and Inf parse as floats but are not JSON numbers; Scan reports them as
// an unmarshal type error naming the field.
func TestScanJSONReportsNonFiniteNumbers(t *testing.T) {
	for _, value := range []string{"NaN", "Inf", "-Inf"} {
		var got struct {
			Ratio float64 `json:"ratio"`
		}
		err := scanJSON([]byte(`{"ratio": "`+value+`"}`), &got)
		var typeErr *json.UnmarshalTypeError
		require.ErrorAs(t, err, &typeErr, value)
		assert.Equal(t, "ratio", typeErr.Field, value)
	}
}
