// watchdog_test.go pins watchdog.go's pure surface: watchdogOption's validate/normalize contract, the wake cadence rule and both embedded templates' watchdog default.
// Untagged: nothing here spawns a process or sleeps (Test Tier Purity Invariant).

package reedengine

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestNextWakeCadence pins the shared backoff rule: base on a changed cycle, otherwise double the current wait up to the ceiling.
func TestNextWakeCadence(t *testing.T) {
	const base, ceiling = time.Second, 8 * time.Second
	tests := []struct {
		name    string
		current time.Duration
		changed bool
		want    time.Duration
	}{
		{"changed resets to base", 4 * time.Second, true, base},
		{"unchanged doubles", 2 * time.Second, false, 4 * time.Second},
		{"unchanged from base doubles", base, false, 2 * time.Second},
		{"unchanged caps at ceiling", 6 * time.Second, false, ceiling},
		{"unchanged at ceiling stays", ceiling, false, ceiling},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextWakeCadence(tt.current, base, ceiling, tt.changed); got != tt.want {
				t.Errorf("NextWakeCadence(%v, %v, %v, %v) = %v, want %v", tt.current, base, ceiling, tt.changed, got, tt.want)
			}
		})
	}
}

//testtiming:keep pins the watchdog option validating and normalizing: on and off in any case or padding, and an empty, numeric, true, yes or misspelled value rejected naming the offending value; its covering tests run this code without asserting it
func TestWatchdogOption(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    bool
		wantErr bool
	}{
		{"on", "on", true, false},
		{"upper_ON", "ON", true, false},
		{"whitespace_on", " on ", true, false},
		{"off", "off", false, false},
		{"upper_OFF", "OFF", false, false},
		{"whitespace_off", " off ", false, false},
		{"invalid_empty", "", false, true},
		{"invalid_numeric", "1", false, true},
		{"invalid_true", "true", false, true},
		{"invalid_yes", "yes", false, true},
		{"invalid_onn", "onn", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := watchdogOption(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("watchdogOption(%q) = %v, nil; want error", tt.raw, got)
				}
				if !strings.Contains(err.Error(), tt.raw) {
					t.Errorf("watchdogOption(%q) error = %v; want it to contain the offending value", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("watchdogOption(%q) = %v, %v; want %v, nil", tt.raw, got, err, tt.want)
			}
			if got != tt.want {
				t.Errorf("watchdogOption(%q) = %v; want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestWatchdogTemplateDefault_BothGOOS reads both embedded template YAML files directly by relative
// path, since ConfigTemplate() only ever exposes the GOOS this build embedded (template_posix.go
// carries !windows, template_windows.go carries windows) — the same limit
// TestLoadConfig_UninitializedFallsBackToTemplate documents in config_test.go.
//
//testtiming:keep pins both embedded templates and the shipped accessor declaring the watchdog default line, reading the posix and windows files directly since ConfigTemplate exposes only one; its covering tests run this code without asserting it
func TestWatchdogTemplateDefault_BothGOOS(t *testing.T) {
	for _, path := range []string{"template_posix.yaml", "template_windows.yaml"} {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if !strings.Contains(string(raw), "watchdog: ${env:LYX_REED_WATCHDOG:-on}") {
				t.Errorf("%s does not declare the expected watchdog default line", path)
			}
		})
	}

	// The accessor this build actually ships must carry the same line.
	if !strings.Contains(ConfigTemplate(), "watchdog: ${env:LYX_REED_WATCHDOG:-on}") {
		t.Errorf("ConfigTemplate() does not declare the expected watchdog default line")
	}
}
