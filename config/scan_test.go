package config

import (
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
