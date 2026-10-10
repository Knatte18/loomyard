// config_test.go — untagged Tier-1 unit tests for darnengine.LoadConfig and darnengine.VerifyCommand.
//
// Seeds a bare t.TempDir() with just a _lyx/config/darn.yaml file (no real hub, no SeedConfig, no git spawn).

package darnengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedDarnConfig creates <baseDir>/_lyx/config/darn.yaml with the given contents.
func seedDarnConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "darn.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile darn.yaml = %v; want nil", err)
	}
}

// TestLoadConfig covers the template's defaults, a writer that fails the model-spec grammar, and a base directory with no darn.yaml.
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		seed     bool
		want     Config
		wantErr  string
	}{
		{
			name: "template defaults", contents: ConfigTemplate(), seed: true,
			want: Config{Writer: "opus[medium]", WriterTimeoutMin: 180, Verify: "", VerifyAttempts: 3},
		},
		{
			name: "invalid writer spec", seed: true,
			contents: strings.Replace(ConfigTemplate(), "writer: opus[medium]", "writer: \"opus[medium\"", 1),
			wantErr:  "writer",
		},
		{name: "not initialized", wantErr: `not initialized here; run "lyx fabric reconcile"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			if tt.seed {
				seedDarnConfig(t, baseDir, tt.contents)
			}
			got, err := LoadConfig(baseDir, "darn")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadConfig() = _, %v; want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() = _, %v; want nil error", err)
			}
			if got != tt.want {
				t.Errorf("LoadConfig() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// TestVerifyCommand covers the verify value as written, and the missing-file and empty-value answers, each an error that names the way forward.
func TestVerifyCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		seed     bool
		want     string
		wantErr  bool
	}{
		{name: "value as written", seed: true, contents: strings.Replace(ConfigTemplate(), `verify: ""`, "verify: go test ./...", 1), want: "go test ./..."},
		{name: "empty value", seed: true, contents: ConfigTemplate(), wantErr: true},
		{name: "missing file", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			if tt.seed {
				seedDarnConfig(t, baseDir, tt.contents)
			}
			got, err := VerifyCommand(baseDir)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), verifyWayForward) {
					t.Fatalf("VerifyCommand() = %q, %v; want an error naming %q", got, err, verifyWayForward)
				}
				if got != "" {
					t.Errorf("VerifyCommand() = %q alongside the error; want empty", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("VerifyCommand() = %q, %v; want %q, nil", got, err, tt.want)
			}
		})
	}
}
