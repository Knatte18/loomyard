//go:build tier0

// config_test.go — tier 0 unit tests for driftengine.LoadConfig, Config.Step and the drift paths.
//
// Seeds a bare t.TempDir() with just a _lyx/config/drift.yaml file (no real hub, no git spawn).

package driftengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// seedDriftConfig creates <baseDir>/_lyx/config/drift.yaml with the given contents.
func seedDriftConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "drift.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile drift.yaml = %v; want nil", err)
	}
}

// withKey returns the template with the line of key replaced by line.
func withKey(t *testing.T, key, line string) string {
	t.Helper()
	var out []string
	replaced := false
	for _, templateLine := range strings.Split(ConfigTemplate(), "\n") {
		if strings.HasPrefix(templateLine, key+":") {
			out = append(out, line)
			replaced = true
			continue
		}
		out = append(out, templateLine)
	}
	if !replaced {
		t.Fatalf("template holds no key %q", key)
	}
	return strings.Join(out, "\n")
}

// templateConfig returns the Config the embedded template loads as, changed by mutate.
func templateConfig(mutate func(*Config)) Config {
	cfg := Config{BaselineModel: "sonnet[medium]", VerifyModel: "opus[high]", VerifyMaxRuns: 8}
	mutate(&cfg)
	return cfg
}

// TestLoadConfig covers the template's defaults, a declared step and its defaults, every refusal, the local layer closing both executable keys, a base directory with no drift.yaml, Step, and the drift paths.
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	capture := "capture: {command: \"profile --out {baseline_dir}\", timeout_min: 30}"
	tests := []struct {
		name     string
		contents string
		seed     bool
		want     Config
		wantErr  []string
	}{
		{name: "template defaults", seed: true, contents: ConfigTemplate(), want: templateConfig(func(*Config) {})},
		{
			name: "a declared capture loads with dir defaulted", seed: true,
			contents: withKey(t, "capture", capture),
			want: templateConfig(func(c *Config) {
				c.Capture = CommandStep{Command: "profile --out {baseline_dir}", Dir: ".", TimeoutMin: 30}
			}),
		},
		{
			name: "a declared compare keeps its dir and env", seed: true,
			contents: withKey(t, "compare", "compare: {command: \"cmp {baseline_dir} {skip_file}\", dir: tools, env: {A: b}, timeout_min: 5}"),
			want: templateConfig(func(c *Config) {
				c.Compare = CommandStep{Command: "cmp {baseline_dir} {skip_file}", Dir: "tools", Env: map[string]string{"A": "b"}, TimeoutMin: 5}
			}),
		},
		{
			name: "enabled loads", seed: true,
			contents: withKey(t, "enabled", "enabled: true"),
			want:     templateConfig(func(c *Config) { c.Enabled = true }),
		},
		{
			name: "a capture without the baseline placeholder", seed: true,
			contents: withKey(t, "capture", "capture: {command: profile, timeout_min: 30}"),
			wantErr:  []string{"capture.command", "{baseline_dir}", "lyx config drift"},
		},
		{
			name: "a compare without a command", seed: true,
			contents: withKey(t, "compare", "compare: {timeout_min: 30}"),
			wantErr:  []string{"compare.command", "lyx config drift"},
		},
		{
			name: "a capture without a timeout", seed: true,
			contents: withKey(t, "capture", "capture: {command: \"p {baseline_dir}\"}"),
			wantErr:  []string{"capture.timeout_min", "lyx config drift"},
		},
		{
			name: "a capture timeout below 1", seed: true,
			contents: withKey(t, "capture", "capture: {command: \"p {baseline_dir}\", timeout_min: 0}"),
			wantErr:  []string{"capture.timeout_min", "lyx config drift"},
		},
		{
			name: "verify_max_runs below 1", seed: true,
			contents: withKey(t, "verify_max_runs", "verify_max_runs: 0"),
			wantErr:  []string{"verify_max_runs", "lyx config drift"},
		},
		{
			name: "an invalid baseline model", seed: true,
			contents: withKey(t, "baseline_model", "baseline_model: \"sonnet[medium\""),
			wantErr:  []string{"baseline_model", "lyx config drift"},
		},
		{
			name: "an invalid verify model", seed: true,
			contents: withKey(t, "verify_model", "verify_model: \"opus[high\""),
			wantErr:  []string{"verify_model", "lyx config drift"},
		},
		{name: "not initialized", wantErr: []string{`not initialized here; run "lyx fabric reconcile"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			if tt.seed {
				seedDriftConfig(t, baseDir, tt.contents)
			}
			got, err := LoadConfig(configengine.Dirs{Board: baseDir})
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadConfig() = %+v; want %+v", got, tt.want)
			}
		})
	}

	for key, local := range map[string]string{
		"capture": "capture: {command: \"p {baseline_dir}\", timeout_min: 1}\n",
		"compare": "compare: {command: \"c {baseline_dir}\", timeout_min: 1}\n",
	} {
		t.Run("a local "+key+" is refused naming the key", func(t *testing.T) {
			t.Parallel()
			board, anchor := t.TempDir(), t.TempDir()
			seedDriftConfig(t, board, ConfigTemplate())
			for _, dir := range []string{configengine.ConfigDir(anchor), configengine.ScratchDir(anchor)} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
				}
			}
			localPath := configengine.LocalFile(anchor, "drift")
			if err := os.WriteFile(localPath, []byte(local), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) = %v; want nil", localPath, err)
			}

			_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
			if !errors.Is(err, configengine.ErrInvalid) || !strings.Contains(err.Error(), key) {
				t.Errorf("LoadConfig() error = %v; want ErrInvalid naming %q", err, key)
			}
		})
	}

	t.Run("Step returns a declared step and refuses an undeclared one", func(t *testing.T) {
		t.Parallel()
		cfg := templateConfig(func(c *Config) { c.Capture = CommandStep{Command: "p {baseline_dir}", Dir: ".", TimeoutMin: 3} })

		got, err := cfg.Step("capture")
		if err != nil || !reflect.DeepEqual(got, CommandStep{Command: "p {baseline_dir}", Dir: ".", TimeoutMin: 3}) {
			t.Errorf("Step(capture) = %+v, %v; want the declared step", got, err)
		}
		if _, err := cfg.Step("compare"); err == nil || !strings.Contains(err.Error(), "lyx config drift --set 'compare=") {
			t.Errorf("Step(compare) error = %v; want a refusal naming lyx config drift --set", err)
		}
	})

	t.Run("the durable paths lie under DirRel", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		p := NewPaths(root)
		if got := DirRel(); got != "_lyx/drift" {
			t.Errorf("DirRel() = %q; want _lyx/drift", got)
		}
		durable := filepath.Join(root, filepath.FromSlash(DirRel())) + string(filepath.Separator)
		for name, file := range map[string]string{"Scope": p.Scope, "BaselineReport": p.BaselineReport, "VerifyReport": p.VerifyReport, "LogsDir": p.LogsDir} {
			if !strings.HasPrefix(file, durable) {
				t.Errorf("NewPaths().%s = %q; want it under %q", name, file, durable)
			}
		}
		ephemeral := filepath.Join(root, ".lyx", "drift") + string(filepath.Separator)
		for name, file := range map[string]string{"Capture": p.Capture, "CaptureLock": p.CaptureLock, "CompareMarker": p.CompareMarker, "BaselineDir": p.BaselineDir, "SideDir": p.SideDir, "SkipFile": p.SkipFile, "Runs": p.Runs} {
			if !strings.HasPrefix(file, ephemeral) {
				t.Errorf("NewPaths().%s = %q; want it under %q", name, file, ephemeral)
			}
		}
	})
}
