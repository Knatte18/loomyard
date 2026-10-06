// config_test.go verifies logger.yaml's template, Load's degrading-absence and validation paths, and ConfigPath, seeded via plain os.MkdirAll/os.WriteFile against a t.TempDir() so the test stays untagged and spawn-free.

package loggerconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
)

func seedConfig(t *testing.T, anchor, content string) {
	t.Helper()
	if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(loggerconfig.ConfigPath(anchor), []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

func TestLoad_ResolvesBounds(t *testing.T) {
	t.Parallel()
	const day = 24 * time.Hour
	tests := []struct {
		name string
		// seed writes the config file into the anchor; nil leaves the anchor without a _lyx directory.
		seed func(t *testing.T, anchor string)
		want logger.RetentionBounds
	}{
		{
			name: "valid non-default values",
			seed: func(t *testing.T, anchor string) {
				seedConfig(t, anchor, "trace_retention_count: 7\ntrace_retention_days: 3\n")
			},
			want: logger.RetentionBounds{Count: 7, MaxAge: 3 * day},
		},
		{
			name: "missing key loads template default",
			seed: func(t *testing.T, anchor string) { seedConfig(t, anchor, "trace_retention_count: 7\n") },
			want: logger.RetentionBounds{Count: 7, MaxAge: 14 * day},
		},
		{
			name: "largest day count a duration holds",
			seed: func(t *testing.T, anchor string) {
				seedConfig(t, anchor, "trace_retention_count: 200\ntrace_retention_days: 106751\n")
			},
			want: logger.RetentionBounds{Count: 200, MaxAge: 106751 * day},
		},
		{
			name: "absent _lyx",
			want: logger.DefaultRetentionBounds(),
		},
		{
			name: "absent logger.yaml",
			seed: func(t *testing.T, anchor string) {
				if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: logger.DefaultRetentionBounds(),
		},
		{
			name: "template matches default bounds",
			seed: func(t *testing.T, anchor string) { seedConfig(t, anchor, loggerconfig.ConfigTemplate()) },
			want: logger.DefaultRetentionBounds(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor := t.TempDir()
			if tt.seed != nil {
				tt.seed(t, anchor)
			}

			got, err := loggerconfig.Load(anchor)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got != tt.want {
				t.Errorf("Load = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestLoad_InvalidValueErrorsNamingKey includes a day count too large for a time.Duration, which must error instead of overflowing MaxAge into a negative duration that would sweep every non-live trace.
func TestLoad_InvalidValueErrorsNamingKey(t *testing.T) {
	t.Parallel()
	tests := []struct{ key, value string }{
		{"trace_retention_count", "0"},
		{"trace_retention_count", "-1"},
		{"trace_retention_count", "1.5"},
		{"trace_retention_count", "14.0"},
		{"trace_retention_count", `"ten"`},
		{"trace_retention_days", "0"},
		{"trace_retention_days", "-1"},
		{"trace_retention_days", "1.5"},
		{"trace_retention_days", "14.0"},
		{"trace_retention_days", `"ten"`},
		{"trace_retention_days", "1000000"},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			t.Parallel()
			count, days := "200", "14"
			if tt.key == "trace_retention_count" {
				count = tt.value
			} else {
				days = tt.value
			}
			anchor := t.TempDir()
			seedConfig(t, anchor, fmt.Sprintf("trace_retention_count: %s\ntrace_retention_days: %s\n", count, days))

			_, err := loggerconfig.Load(anchor)
			if err == nil {
				t.Fatal("Load: want error, got nil")
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Errorf("error %q does not name key %q", err, tt.key)
			}
		})
	}
}

//testtiming:keep pins the exact config file path, which no covering test asserts
func TestConfigPath(t *testing.T) {
	t.Parallel()
	anchor := t.TempDir()
	want := filepath.Join(anchor, "_lyx", "config", "logger.yaml")
	if got := loggerconfig.ConfigPath(anchor); got != want {
		t.Errorf("ConfigPath = %q, want %q", got, want)
	}
}
