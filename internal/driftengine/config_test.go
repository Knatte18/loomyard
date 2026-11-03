// config_test.go — tier 0 unit tests for driftengine.LoadConfig, Config.Step and the drift paths.
//
// Seeds a bare t.TempDir() with just a _lym/config/drift.yaml file (no real hub, no git spawn).

package driftengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// TestConfigSpec_SetNestedKeys pins that a --set reaches one key inside capture or compare, a number decoding as a number and an env value as a string, so the file still loads;
// and that a key a step lacks is refused naming it, leaving the file byte for byte.
func TestConfigSpec_SetNestedKeys(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	baseDir := t.TempDir()
	seedDriftConfig(t, baseDir, withKey(t, "capture", `capture: {command: "profile --out {baseline_dir}", timeout_min: 30}`))
	dirs := configengine.Dirs{Board: baseDir}
	pairs := []yamlengine.KV{
		{Key: "capture.timeout_min", Value: "45"},
		{Key: "capture.env.NAME", Value: "1"},
		{Key: "compare.command", Value: "cmp {baseline_dir}"},
		{Key: "compare.timeout_min", Value: "5"},
	}
	if _, err := configengine.Set(dirs, ConfigSpec(), configengine.LayerDefault, pairs); err != nil {
		t.Fatalf("Set() = %v; want nil", err)
	}
	cfg, err := LoadConfig(dirs)
	if err != nil {
		t.Fatalf("LoadConfig() = %v; want nil", err)
	}
	wantCapture := CommandStep{Command: "profile --out {baseline_dir}", Dir: ".", Env: map[string]string{"NAME": "1"}, TimeoutMin: 45}
	if !reflect.DeepEqual(cfg.Capture, wantCapture) {
		t.Errorf("LoadConfig() capture = %+v; want %+v", cfg.Capture, wantCapture)
	}
	wantCompare := CommandStep{Command: "cmp {baseline_dir}", Dir: ".", TimeoutMin: 5}
	if !reflect.DeepEqual(cfg.Compare, wantCompare) {
		t.Errorf("LoadConfig() compare = %+v; want %+v", cfg.Compare, wantCompare)
	}

	path := configengine.ConfigFile(baseDir, configModule)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = configengine.Set(dirs, ConfigSpec(), configengine.LayerDefault, []yamlengine.KV{{Key: "capture.timeoutt", Value: "1"}})
	if err == nil || !strings.Contains(err.Error(), "capture holds no key timeoutt") {
		t.Errorf("Set(capture.timeoutt) = %v; want a refusal naming timeoutt", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("drift file after the refusal = %q; want it unchanged %q", after, before)
	}
}

// TestConfigSpec_SetWholeSteps pins that a --set writes capture and compare whole from the flow mapping the template's own description prescribes, so the file loads with both steps declared;
// and that a whole step holding a key a step lacks is refused naming it, leaving the file byte for byte.
func TestConfigSpec_SetWholeSteps(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	baseDir := t.TempDir()
	seedDriftConfig(t, baseDir, ConfigTemplate())
	dirs := configengine.Dirs{Board: baseDir}
	pairs := []yamlengine.KV{
		{Key: "capture", Value: `{command: "profile --out {baseline_dir}", dir: tests, timeout_min: 30}`},
		{Key: "compare", Value: `{command: "cmp {baseline_dir}", env: {NAME: 1}, timeout_min: 5}`},
	}
	if _, err := configengine.Set(dirs, ConfigSpec(), configengine.LayerDefault, pairs); err != nil {
		t.Fatalf("Set() = %v; want nil", err)
	}
	cfg, err := LoadConfig(dirs)
	if err != nil {
		t.Fatalf("LoadConfig() = %v; want nil", err)
	}
	wantCapture := CommandStep{Command: "profile --out {baseline_dir}", Dir: "tests", TimeoutMin: 30}
	if !reflect.DeepEqual(cfg.Capture, wantCapture) {
		t.Errorf("LoadConfig() capture = %+v; want %+v", cfg.Capture, wantCapture)
	}
	wantCompare := CommandStep{Command: "cmp {baseline_dir}", Dir: ".", Env: map[string]string{"NAME": "1"}, TimeoutMin: 5}
	if !reflect.DeepEqual(cfg.Compare, wantCompare) {
		t.Errorf("LoadConfig() compare = %+v; want %+v", cfg.Compare, wantCompare)
	}

	path := configengine.ConfigFile(baseDir, configModule)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = configengine.Set(dirs, ConfigSpec(), configengine.LayerDefault, []yamlengine.KV{{Key: "capture", Value: `{command: "p {baseline_dir}", timeoutt: 1}`}})
	if err == nil || !strings.Contains(err.Error(), "capture holds no key timeoutt") {
		t.Errorf("Set(capture={..., timeoutt: 1}) = %v; want a refusal naming timeoutt", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("drift file after the refusal = %q; want it unchanged %q", after, before)
	}
}

// seedDriftConfig creates <baseDir>/_lym/config/drift.yaml with the given contents.
func seedDriftConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lym", "config")
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
	return withKeys(t, map[string]string{key: line})
}

// withKeys returns the template with the line of each key of lines replaced by its line.
func withKeys(t *testing.T, lines map[string]string) string {
	t.Helper()
	var out []string
	replaced := map[string]bool{}
	for _, templateLine := range strings.Split(ConfigTemplate(), "\n") {
		key, _, _ := strings.Cut(templateLine, ":")
		if line, ok := lines[key]; ok {
			out = append(out, line)
			replaced[key] = true
			continue
		}
		out = append(out, templateLine)
	}
	for key := range lines {
		if !replaced[key] {
			t.Fatalf("template holds no key %q", key)
		}
	}
	return strings.Join(out, "\n")
}

// seedLocalDrift seeds a board holding the template and an anchor whose local drift layer holds local, and returns their Dirs.
func seedLocalDrift(t *testing.T, local string) configengine.Dirs {
	t.Helper()
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
	return configengine.Dirs{Anchor: anchor, Board: board}
}

// templateConfig returns the Config the embedded template loads as, changed by mutate.
func templateConfig(mutate func(*Config)) Config {
	cfg := Config{BaselineModel: "sonnet[medium]", VerifyModel: "opus[high]", VerifyMaxRuns: 8}
	mutate(&cfg)
	return cfg
}

// TestLoadConfig covers the template's defaults, a declared step and its defaults, every refusal, the local layer closing both executable keys, a base directory with no drift.yaml, Step, and the drift paths.
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
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
			name: "verify_interactive loads", seed: true,
			contents: withKey(t, "verify_interactive", "verify_interactive: true"),
			want:     templateConfig(func(c *Config) { c.VerifyInteractive = true }),
		},
		{
			name: "scope full loads", seed: true,
			contents: withKey(t, "scope", "scope: full"),
			want:     templateConfig(func(c *Config) { c.Scope = ScopeFull }),
		},
		{
			name: "scope partial loads with its skipped tests", seed: true,
			contents: withKeys(t, map[string]string{"scope": "scope: partial", "skip": "skip: [slow, flaky]"}),
			want:     templateConfig(func(c *Config) { c.Scope, c.Skip = ScopePartial, []string{"slow", "flaky"} }),
		},
		{
			name: "an unknown scope", seed: true,
			contents: withKey(t, "scope", "scope: some"),
			wantErr:  []string{`"scope"`, "run_settings", "lym config drift"},
		},
		{
			name: "scope partial with no skipped test", seed: true,
			contents: withKey(t, "scope", "scope: partial"),
			wantErr:  []string{`"skip"`, "run_settings", "lym config drift"},
		},
		{
			name: "a skipped test under scope full", seed: true,
			contents: withKeys(t, map[string]string{"scope": "scope: full", "skip": "skip: [slow]"}),
			wantErr:  []string{`"skip"`, "run_settings", "lym config drift"},
		},
		{
			name: "a capture without the baseline placeholder", seed: true,
			contents: withKey(t, "capture", "capture: {command: profile, timeout_min: 30}"),
			wantErr:  []string{"capture.command", "{baseline_dir}", "lym config drift"},
		},
		{
			name: "a compare without a command", seed: true,
			contents: withKey(t, "compare", "compare: {timeout_min: 30}"),
			wantErr:  []string{"compare.command", "lym config drift"},
		},
		{
			name: "a capture without a timeout", seed: true,
			contents: withKey(t, "capture", "capture: {command: \"p {baseline_dir}\"}"),
			wantErr:  []string{"capture.timeout_min", "lym config drift"},
		},
		{
			name: "a capture timeout below 1", seed: true,
			contents: withKey(t, "capture", "capture: {command: \"p {baseline_dir}\", timeout_min: 0}"),
			wantErr:  []string{"capture.timeout_min", "lym config drift"},
		},
		{
			name: "verify_max_runs below 1", seed: true,
			contents: withKey(t, "verify_max_runs", "verify_max_runs: 0"),
			wantErr:  []string{"verify_max_runs", "lym config drift"},
		},
		{
			name: "an invalid baseline model", seed: true,
			contents: withKey(t, "baseline_model", "baseline_model: \"sonnet[medium\""),
			wantErr:  []string{"baseline_model", "lym config drift"},
		},
		{
			name: "an invalid verify model", seed: true,
			contents: withKey(t, "verify_model", "verify_model: \"opus[high\""),
			wantErr:  []string{"verify_model", "lym config drift"},
		},
		{name: "not initialized", wantErr: []string{`not initialized here; run "lym fabric reconcile"`}},
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
			_, err := LoadConfig(seedLocalDrift(t, local))
			if !errors.Is(err, configengine.ErrInvalid) || !strings.Contains(err.Error(), key) {
				t.Errorf("LoadConfig() error = %v; want ErrInvalid naming %q", err, key)
			}
		})
	}

	t.Run("a local scope with its skipped tests loads over the board's", func(t *testing.T) {
		t.Parallel()
		got, err := LoadConfig(seedLocalDrift(t, "scope: partial\nskip: [slow]\n"))
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if got.Scope != ScopePartial || !reflect.DeepEqual(got.Skip, []string{"slow"}) {
			t.Errorf("LoadConfig() scope, skip = %q, %q; want %q, [slow]", got.Scope, got.Skip, ScopePartial)
		}
	})

	t.Run("Step returns a declared step and refuses an undeclared one", func(t *testing.T) {
		t.Parallel()
		cfg := templateConfig(func(c *Config) { c.Capture = CommandStep{Command: "p {baseline_dir}", Dir: ".", TimeoutMin: 3} })

		got, err := cfg.Step("capture")
		if err != nil || !reflect.DeepEqual(got, CommandStep{Command: "p {baseline_dir}", Dir: ".", TimeoutMin: 3}) {
			t.Errorf("Step(capture) = %+v, %v; want the declared step", got, err)
		}
		if _, err := cfg.Step("compare"); err == nil || !strings.Contains(err.Error(), "lym config drift --set 'compare=") {
			t.Errorf("Step(compare) error = %v; want a refusal naming lym config drift --set", err)
		}
	})

	t.Run("the durable paths lie under DirRel", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		p := NewPaths(root)
		if got := DirRel(); got != "_lym/drift" {
			t.Errorf("DirRel() = %q; want _lym/drift", got)
		}
		durable := filepath.Join(root, filepath.FromSlash(DirRel())) + string(filepath.Separator)
		for name, file := range map[string]string{"Scope": p.Scope, "BaselineReport": p.BaselineReport, "VerifyReport": p.VerifyReport, "LogsDir": p.LogsDir} {
			if !strings.HasPrefix(file, durable) {
				t.Errorf("NewPaths().%s = %q; want it under %q", name, file, durable)
			}
		}
		ephemeral := filepath.Join(root, ".lym", "drift") + string(filepath.Separator)
		for name, file := range map[string]string{"Capture": p.Capture, "CaptureLock": p.CaptureLock, "CompareMarker": p.CompareMarker, "BaselineDir": p.BaselineDir, "SideDir": p.SideDir, "SkipFile": p.SkipFile, "Runs": p.Runs} {
			if !strings.HasPrefix(file, ephemeral) {
				t.Errorf("NewPaths().%s = %q; want it under %q", name, file, ephemeral)
			}
		}
	})
}
