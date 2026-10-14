// config_test.go — untagged Tier-1 unit tests for darnengine.LoadConfig and darnengine.VerifyCommand.
//
// Seeds a bare t.TempDir() with just a _lyx/config/darn.yaml file (no real hub, no SeedConfig, no git spawn).

package darnengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
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

// TestLoadConfig covers the template's defaults, a review budget of 0, a writer that fails the model-spec grammar, and a base directory with no darn.yaml.
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		seed     bool
		want     Config
		wantErr  []string
	}{
		{
			name: "template defaults", contents: ConfigTemplate(), seed: true,
			want: Config{Writer: "opus[medium]", WriterTimeoutMin: 180, Verify: "", VerifyAttempts: 3, ReviewMaxBounces: 1, ReviewHardMaxBounces: 4},
		},
		{
			name: "review off", seed: true,
			contents: strings.Replace(ConfigTemplate(), "review_max_bounces: 1", "review_max_bounces: 0", 1),
			want:     Config{Writer: "opus[medium]", WriterTimeoutMin: 180, Verify: "", VerifyAttempts: 3, ReviewMaxBounces: 0, ReviewHardMaxBounces: 4},
		},
		{
			name: "invalid writer spec", seed: true,
			contents: strings.Replace(ConfigTemplate(), "writer: opus[medium]", "writer: \"opus[medium\"", 1),
			wantErr:  []string{"writer"},
		},
		{
			name: "hard cap below 1", seed: true,
			contents: strings.Replace(ConfigTemplate(), "review_hard_max_bounces: 4", "review_hard_max_bounces: 0", 1),
			wantErr:  []string{"review_hard_max_bounces", "must be at least 1", "lyx config darn --set"},
		},
		{
			name: "negative budget", seed: true,
			contents: strings.Replace(ConfigTemplate(), "review_max_bounces: 1", "review_max_bounces: -1", 1),
			wantErr:  []string{"review_max_bounces", "set it to 0 for no review round", "lyx config darn --set"},
		},
		{
			name: "budget above the default hard cap", seed: true,
			contents: strings.Replace(ConfigTemplate(), "review_max_bounces: 1", "review_max_bounces: 5", 1),
			wantErr:  []string{"review_hard_max_bounces", "review_max_bounces", "lyx config darn --set", "lyx loom resume"},
		},
		{name: "not initialized", wantErr: []string{`not initialized here; run "lyx fabric reconcile"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			if tt.seed {
				seedDarnConfig(t, baseDir, tt.contents)
			}
			got, err := LoadConfig(configengine.Dirs{Board: baseDir}, "darn")
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("LoadConfig() = _, nil; want an error containing %q", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
					}
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

	t.Run("a local writer wins over the hub file's", func(t *testing.T) {
		t.Parallel()
		board, anchor := t.TempDir(), t.TempDir()
		seedDarnConfig(t, board, ConfigTemplate())
		writeLocalDarnConfig(t, anchor, "writer: sonnet[high]\n")

		got, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "darn")
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if got.Writer != "sonnet[high]" {
			t.Errorf("LoadConfig().Writer = %q; want %q", got.Writer, "sonnet[high]")
		}
	})

	t.Run("a local verify is refused naming the local file and the key", func(t *testing.T) {
		t.Parallel()
		board, anchor := t.TempDir(), t.TempDir()
		seedDarnConfig(t, board, ConfigTemplate())
		localPath := writeLocalDarnConfig(t, anchor, "verify: rm -rf .\n")

		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "darn")
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Fatalf("LoadConfig() error = %v; want ErrInvalid", err)
		}
		for _, want := range []string{localPath, "verify"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})

	t.Run("a local budget and hard cap load", func(t *testing.T) {
		t.Parallel()
		board, anchor := t.TempDir(), t.TempDir()
		seedDarnConfig(t, board, ConfigTemplate())
		writeLocalDarnConfig(t, anchor, "review_max_bounces: 2\nreview_hard_max_bounces: 3\n")

		got, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "darn")
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if got.ReviewMaxBounces != 2 || got.ReviewHardMaxBounces != 3 {
			t.Errorf("LoadConfig() budget, hard cap = %d, %d; want 2, 3", got.ReviewMaxBounces, got.ReviewHardMaxBounces)
		}
	})

	t.Run("a local budget over the hub's hard cap names the local and the hub route", func(t *testing.T) {
		t.Parallel()
		board, anchor := t.TempDir(), t.TempDir()
		seedDarnConfig(t, board, ConfigTemplate())
		writeLocalDarnConfig(t, anchor, "review_max_bounces: 5\n")

		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "darn")
		if err == nil {
			t.Fatal("LoadConfig() = _, nil; want a refusal")
		}
		for _, want := range []string{"lyx config darn --layer local --set review_max_bounces=<n>", "lyx board upsert", "lyx config darn --set review_hard_max_bounces=<n>"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})
}

// writeLocalDarnConfig writes contents as anchor's local darn override and returns its path.
// anchor gets an empty _lyx/config/ so the layered load sees an initialised worktree.
func writeLocalDarnConfig(t *testing.T, anchor, contents string) string {
	t.Helper()
	for _, dir := range []string{configengine.ConfigDir(anchor), configengine.ScratchDir(anchor)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
		}
	}
	localPath := configengine.LocalFile(anchor, "darn")
	if err := os.WriteFile(localPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", localPath, err)
	}
	return localPath
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
			got, err := VerifyCommand(configengine.Dirs{Board: baseDir})
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
