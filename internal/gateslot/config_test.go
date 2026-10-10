// config_test.go covers loading the hub-wide gate config over a bare temp directory seeded with just a gate.yaml, with no hub and no git.

package gateslot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var templateDefaults = Config{Slots: 2, GoParallel: 4, CLIWaitSec: 300}

func seedGateConfig(t *testing.T, contents string) string {
	t.Helper()
	baseDir := t.TempDir()
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if contents != "" {
		if err := os.WriteFile(filepath.Join(configDir, "gate.yaml"), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return baseDir
}

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		want     Config
		wantErr  string
	}{
		{"template loads to its defaults", ConfigTemplate(), templateDefaults, ""},
		{"zero slots", "slots: 0\ngo_parallel: 4\ncli_wait_sec: 300\n", Config{}, "slots"},
		{"zero go_parallel", "slots: 2\ngo_parallel: 0\ncli_wait_sec: 300\n", Config{}, "go_parallel"},
		{"zero cli_wait_sec", "slots: 2\ngo_parallel: 4\ncli_wait_sec: 0\n", Config{}, "cli_wait_sec"},
		{"absent file names the way forward", "", Config{}, "lyx fabric reconcile"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := LoadConfig(seedGateConfig(t, tc.contents))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("LoadConfig() error = %v; want one naming %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("LoadConfig() = (%+v, %v); want (%+v, nil)", got, err, tc.want)
			}
		})
	}
}

func TestTemplateConfigAndLimits(t *testing.T) {
	t.Parallel()

	cfg, err := TemplateConfig()
	if err != nil || cfg != templateDefaults {
		t.Fatalf("TemplateConfig() = (%+v, %v); want (%+v, nil)", cfg, err, templateDefaults)
	}
	want := Limits{Slots: 2, GoParallel: 4, CLIWait: 300 * time.Second}
	if got := cfg.Limits(); got != want {
		t.Errorf("Limits() = %+v; want %+v", got, want)
	}
}
