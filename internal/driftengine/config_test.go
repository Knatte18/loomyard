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
	wantCapture := CommandStep{Command: "profile --out {baseline_dir}", Env: map[string]string{"NAME": "1"}, TimeoutMin: 45}
	if !reflect.DeepEqual(cfg.Capture, wantCapture) {
		t.Errorf("LoadConfig() capture = %+v; want %+v", cfg.Capture, wantCapture)
	}
	wantCompare := CommandStep{Command: "cmp {baseline_dir}", TimeoutMin: 5}
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
	wantCompare := CommandStep{Command: "cmp {baseline_dir}", Env: map[string]string{"NAME": "1"}, TimeoutMin: 5}
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

// TestCheckDirs pins that a declared step whose dir resolves to no existing directory under the worktree root is refused.
// The refusal names the step's key and the resolved path.
// An existing dir, an unset dir and an undeclared step pass.
func TestCheckDirs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	root := t.TempDir()
	anchor := filepath.Join(root, "dotnet")
	if err := os.MkdirAll(anchor, 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v; want nil", err)
	}
	tests := []struct {
		name    string
		capture string
		// compare is the compare line; empty leaves the template's undeclared one.
		compare string
		// wantErr are the texts the refusal names; empty when the dirs pass.
		wantErr []string
	}{
		{name: "AnUndeclaredStepPasses", capture: "capture: {}"},
		{
			name:    "ADirMissingUnderTheWorktreeRootOfTheCompareIsRefused",
			capture: "capture: {}",
			compare: `compare: {command: "c {baseline_dir}", dir: gone, timeout_min: 1}`,
			wantErr: []string{`drift config key "compare.dir"`, filepath.Join(root, "gone"), "lym config drift"},
		},
		{name: "NoDirPasses", capture: `capture: {command: "p {baseline_dir}", timeout_min: 1}`},
		{name: "AnExistingDirUnderTheWorktreeRootPasses", capture: `capture: {command: "p {baseline_dir}", dir: dotnet, timeout_min: 1}`},
		{
			name:    "ADirMissingUnderTheWorktreeRootIsRefused",
			capture: `capture: {command: "p {baseline_dir}", dir: dotnet/dotnet, timeout_min: 1}`,
			wantErr: []string{`drift config key "capture.dir"`, filepath.Join(root, "dotnet", "dotnet"), "lym config drift"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := t.TempDir()
			lines := map[string]string{"capture": tt.capture}
			if tt.compare != "" {
				lines["compare"] = tt.compare
			}
			seedDriftConfig(t, board, withKeys(t, lines))
			err := CheckDirs(configengine.Dirs{Board: board}, root, anchor)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Errorf("CheckDirs() = %v; want nil", err)
				}
				return
			}
			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("CheckDirs() = %v; want an error naming %q", err, want)
				}
			}
		})
	}
}

// TestCheckRetiredPlaceholders pins that an enabled drift config refuses a step a run executes that holds {skip_file}, with a way forward a task or beam session can hand on, and passes every other config.
func TestCheckRetiredPlaceholders(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		leftoverCapture = `capture: {command: "p {baseline_dir} {skip_file}", timeout_min: 1}`
		leftoverCompare = `compare: {command: "c {baseline_dir} {skip_file}", timeout_min: 1}`
		cleanCompare    = `compare: {command: "c {baseline_dir}", timeout_min: 1}`
	)
	tests := []struct {
		name  string
		lines map[string]string
		// wantErr are the texts the refusal names; empty when the config passes.
		wantErr []string
	}{
		{name: "AnEnabledCleanConfigPasses", lines: map[string]string{"enabled": "enabled: true"}},
		{name: "ALeftoverWithDriftOffPasses", lines: map[string]string{"capture": leftoverCapture}},
		{
			name:    "ALeftoverInTheCaptureIsRefused",
			lines:   map[string]string{"enabled": "enabled: true", "capture": leftoverCapture},
			wantErr: []string{"the drift capture command holds {skip_file}", "the operator or the hub orch runs lym config reconcile --layer default --apply", "way forward"},
		},
		{
			name:  "ALeftoverInACompareThatNeverRunsPasses",
			lines: map[string]string{"enabled": "enabled: true", "compare": leftoverCompare},
		},
		{
			name:    "ALeftoverInTheCompareUnderDiffCommandIsRefused",
			lines:   map[string]string{"enabled": "enabled: true", "diff": "diff: command", "compare": leftoverCompare},
			wantErr: []string{"the drift compare command holds {skip_file}", "any other use of it from compare"},
		},
		{
			name:  "ACleanCompareUnderDiffCommandPasses",
			lines: map[string]string{"enabled": "enabled: true", "diff": "diff: command", "compare": cleanCompare},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := t.TempDir()
			seedDriftConfig(t, board, withKeys(t, tt.lines))

			err := CheckRetiredPlaceholders(configengine.Dirs{Board: board})

			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Errorf("CheckRetiredPlaceholders() = %v; want nil", err)
				}
				return
			}
			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("CheckRetiredPlaceholders() = %v; want an error naming %q", err, want)
				}
			}
		})
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
	cfg := Config{Diff: DiffLines, KeyPatterns: defaultTestKeyPatterns, Volatile: Volatile{Files: []string{}, Keys: []string{}}, ExpectModel: "sonnet[medium]", JudgeModel: "opus[high]"}
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
			name: "a declared capture with no dir loads with an empty dir", seed: true,
			contents: withKey(t, "capture", capture),
			want: templateConfig(func(c *Config) {
				c.Capture = CommandStep{Command: "profile --out {baseline_dir}", TimeoutMin: 30}
			}),
		},
		{
			name: "a declared compare keeps its dir and env", seed: true,
			contents: withKey(t, "compare", "compare: {command: \"cmp {baseline_dir}\", dir: tools, env: {A: b}, timeout_min: 5}"),
			want: templateConfig(func(c *Config) {
				c.Compare = CommandStep{Command: "cmp {baseline_dir}", Dir: "tools", Env: map[string]string{"A": "b"}, TimeoutMin: 5}
			}),
		},
		{
			name: "enabled loads", seed: true,
			contents: withKey(t, "enabled", "enabled: true"),
			want:     templateConfig(func(c *Config) { c.Enabled = true }),
		},
		{
			name: "judge_interactive loads", seed: true,
			contents: withKey(t, "judge_interactive", "judge_interactive: true"),
			want:     templateConfig(func(c *Config) { c.JudgeInteractive = true }),
		},
		{
			name: "diff command loads beside a declared compare", seed: true,
			contents: withKeys(t, map[string]string{"diff": "diff: command", "compare": `compare: {command: "cmp {baseline_dir}", timeout_min: 5}`}),
			want: templateConfig(func(c *Config) {
				c.Diff = DiffCommand
				c.Compare = CommandStep{Command: "cmp {baseline_dir}", TimeoutMin: 5}
			}),
		},
		{
			name: "a diff other than lines or command", seed: true,
			contents: withKey(t, "diff", "diff: words"),
			wantErr:  []string{`"diff"`, "lines", "command", "lym config drift"},
		},
		{
			name: "a key pattern with no value group", seed: true,
			contents: withKey(t, "key_patterns", `key_patterns: ['^(?P<key>\w+)$']`),
			wantErr:  []string{`"key_patterns"`, "pattern 0", `no named group "value"`, "lym config drift"},
		},
		{
			name: "volatile lists load", seed: true,
			contents: withKeys(t, map[string]string{"volatile_files": "volatile_files: [_source.txt, 'meta/*.txt']", "volatile_keys": "volatile_keys: [captured_at]"}),
			want: templateConfig(func(c *Config) {
				c.Volatile = Volatile{Files: []string{"_source.txt", "meta/*.txt"}, Keys: []string{"captured_at"}}
			}),
		},
		{
			name: "a volatile file that is no valid glob", seed: true,
			contents: withKey(t, "volatile_files", "volatile_files: ['[']"),
			wantErr:  []string{`"volatile_files"`, `"["`, "lym config drift"},
		},
		{
			name: "a blank volatile file", seed: true,
			contents: withKey(t, "volatile_files", "volatile_files: [' ']"),
			wantErr:  []string{`"volatile_files"`, "blank", "lym config drift"},
		},
		{
			name: "a blank volatile key", seed: true,
			contents: withKey(t, "volatile_keys", "volatile_keys: ['']"),
			wantErr:  []string{`"volatile_keys"`, "blank", "lym config drift"},
		},
		{
			name: "diff command with no compare declared", seed: true,
			contents: withKey(t, "diff", "diff: command"),
			wantErr:  []string{`"compare"`, "diff: command", "lym config drift"},
		},
		{
			name: "scope full loads", seed: true,
			contents: withKey(t, "scope", "scope: full"),
			want:     templateConfig(func(c *Config) { c.Scope = ScopeFull }),
		},
		{
			name: "an unknown scope", seed: true,
			contents: withKey(t, "scope", "scope: some"),
			wantErr:  []string{`"scope"`, "run_settings", "lym config drift"},
		},
		{
			name: "scope partial loads as full", seed: true,
			contents: withKey(t, "scope", "scope: partial"),
			want:     templateConfig(func(c *Config) { c.Scope, c.ScopeWasPartial = ScopeFull, true }),
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
			name: "an invalid expect model", seed: true,
			contents: withKey(t, "expect_model", "expect_model: \"sonnet[medium\""),
			wantErr:  []string{"expect_model", "lym config drift"},
		},
		{
			name: "an invalid judge model", seed: true,
			contents: withKey(t, "judge_model", "judge_model: \"opus[high\""),
			wantErr:  []string{"judge_model", "lym config drift"},
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

	t.Run("a local scope partial with the retired skip and verify_max_runs keys loads as full with both keys dropped", func(t *testing.T) {
		t.Parallel()
		got, err := LoadConfig(seedLocalDrift(t, "scope: partial\nskip: [slow]\nverify_max_runs: 3\n"))
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		want := templateConfig(func(c *Config) { c.Scope, c.ScopeWasPartial = ScopeFull, true })
		if !reflect.DeepEqual(got, want) {
			t.Errorf("LoadConfig() = %+v; want %+v", got, want)
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

	t.Run("StepWithHubEnv lays the env module's variables beneath the step's own env", func(t *testing.T) {
		t.Parallel()
		board := t.TempDir()
		envFile := configengine.ConfigFile(board, "env")
		if err := os.MkdirAll(filepath.Dir(envFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(envFile, []byte("vars:\n  COMMONLIB_REPO: /hub/commonlib\n  MODE: hub\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg := templateConfig(func(c *Config) {
			c.Capture = CommandStep{Command: "p {baseline_dir}", Env: map[string]string{"MODE": "step"}, TimeoutMin: 3}
		})

		got, err := cfg.StepWithHubEnv("capture", configengine.Dirs{Board: board})
		if err != nil {
			t.Fatalf("StepWithHubEnv(capture) error = %v; want nil", err)
		}
		if want := map[string]string{"COMMONLIB_REPO": "/hub/commonlib", "MODE": "step"}; !reflect.DeepEqual(got.Env, want) {
			t.Errorf("StepWithHubEnv(capture).Env = %v; want %v", got.Env, want)
		}
		if cfg.Capture.Env["COMMONLIB_REPO"] != "" {
			t.Errorf("StepWithHubEnv changed the config's own step env to %v", cfg.Capture.Env)
		}
		if _, err := cfg.StepWithHubEnv("compare", configengine.Dirs{Board: board}); err == nil {
			t.Error("StepWithHubEnv(compare) error = nil; want the undeclared step refused")
		}

		brokenBoard := t.TempDir()
		brokenFile := configengine.ConfigFile(brokenBoard, "env")
		if err := os.MkdirAll(filepath.Dir(brokenFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(brokenFile, []byte("vars:\n  BAD-NAME: x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if step, err := cfg.StepWithHubEnv("capture", configengine.Dirs{Board: brokenBoard}); err == nil || !strings.Contains(err.Error(), "vars.BAD-NAME") || !reflect.DeepEqual(step, CommandStep{}) {
			t.Errorf("StepWithHubEnv(capture) over a broken env config = %+v, %v; want an error naming vars.BAD-NAME and no step", step, err)
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
		for name, file := range map[string]string{"Scope": p.Scope, "LogsDir": p.LogsDir} {
			if !strings.HasPrefix(file, durable) {
				t.Errorf("NewPaths().%s = %q; want it under %q", name, file, durable)
			}
		}
		ephemeral := filepath.Join(root, ".lym", "drift") + string(filepath.Separator)
		for name, file := range map[string]string{"Capture": p.Capture, "CaptureLock": p.CaptureLock, "CompareMarker": p.CompareMarker, "BaselineDir": p.BaselineDir} {
			if !strings.HasPrefix(file, ephemeral) {
				t.Errorf("NewPaths().%s = %q; want it under %q", name, file, ephemeral)
			}
		}
	})
}
