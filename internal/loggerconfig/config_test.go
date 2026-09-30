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

func TestLoad_ValidNonDefaultValues(t *testing.T) {
	anchor := t.TempDir()
	seedConfig(t, anchor, "trace_retention_count: 7\ntrace_retention_days: 3\n")

	got, err := loggerconfig.Load(anchor)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := logger.RetentionBounds{Count: 7, MaxAge: 3 * 24 * time.Hour}
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestLoad_AbsentConfigReturnsDefaults(t *testing.T) {
	t.Run("absent _lyx", func(t *testing.T) {
		got, err := loggerconfig.Load(t.TempDir())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got != logger.DefaultRetentionBounds() {
			t.Errorf("Load = %+v, want %+v", got, logger.DefaultRetentionBounds())
		}
	})
	t.Run("absent logger.yaml", func(t *testing.T) {
		anchor := t.TempDir()
		if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := loggerconfig.Load(anchor)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got != logger.DefaultRetentionBounds() {
			t.Errorf("Load = %+v, want %+v", got, logger.DefaultRetentionBounds())
		}
	})
}

func TestConfigTemplate_MatchesDefaultRetentionBounds(t *testing.T) {
	anchor := t.TempDir()
	seedConfig(t, anchor, loggerconfig.ConfigTemplate())

	got, err := loggerconfig.Load(anchor)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != logger.DefaultRetentionBounds() {
		t.Errorf("template resolves to %+v, want %+v", got, logger.DefaultRetentionBounds())
	}
}

func TestLoad_InvalidValueErrorsNamingKey(t *testing.T) {
	keys := []string{"trace_retention_count", "trace_retention_days"}
	values := []string{"0", "-1", "1.5", "14.0", `"ten"`}

	for _, key := range keys {
		for _, value := range values {
			t.Run(key+"="+value, func(t *testing.T) {
				count, days := "200", "14"
				if key == "trace_retention_count" {
					count = value
				} else {
					days = value
				}
				anchor := t.TempDir()
				seedConfig(t, anchor, fmt.Sprintf("trace_retention_count: %s\ntrace_retention_days: %s\n", count, days))

				_, err := loggerconfig.Load(anchor)
				if err == nil {
					t.Fatal("Load: want error, got nil")
				}
				if !strings.Contains(err.Error(), key) {
					t.Errorf("error %q does not name key %q", err, key)
				}
			})
		}
	}
}

// TestLoad_DaysBeyondDurationRangeErrors pins that a day count too large for a time.Duration errors instead of overflowing MaxAge into a negative duration that would sweep every non-live trace.
func TestLoad_DaysBeyondDurationRangeErrors(t *testing.T) {
	anchor := t.TempDir()
	seedConfig(t, anchor, "trace_retention_count: 200\ntrace_retention_days: 1000000\n")

	_, err := loggerconfig.Load(anchor)
	if err == nil {
		t.Fatal("Load: want error for an out-of-range day count, got nil")
	}
	if !strings.Contains(err.Error(), "trace_retention_days") {
		t.Errorf("error %q does not name key %q", err, "trace_retention_days")
	}
}

// TestLoad_LargestDurationDayCountLoads pins that the largest day count a time.Duration holds still loads, with a positive MaxAge.
func TestLoad_LargestDurationDayCountLoads(t *testing.T) {
	anchor := t.TempDir()
	seedConfig(t, anchor, "trace_retention_count: 200\ntrace_retention_days: 106751\n")

	got, err := loggerconfig.Load(anchor)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.MaxAge <= 0 {
		t.Errorf("Load MaxAge = %v; want positive", got.MaxAge)
	}
}

func TestLoad_MissingKeyErrors(t *testing.T) {
	anchor := t.TempDir()
	seedConfig(t, anchor, "trace_retention_count: 200\n")

	if _, err := loggerconfig.Load(anchor); err == nil {
		t.Fatal("Load: want error for missing key, got nil")
	}
}

func TestConfigPath(t *testing.T) {
	anchor := t.TempDir()
	want := filepath.Join(anchor, "_lyx", "config", "logger.yaml")
	if got := loggerconfig.ConfigPath(anchor); got != want {
		t.Errorf("ConfigPath = %q, want %q", got, want)
	}
}
