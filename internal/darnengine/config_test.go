// config_test.go — untagged Tier-1 unit tests for darnengine.LoadConfig and darnengine.VerifyCommand.
//
// Seeds a bare t.TempDir() with just a _lyx/config/darn.yaml file (no real hub, no SeedConfig, no git spawn).

package darnengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
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

// templateConfig returns the Config the embedded template loads as, changed by mutate.
func templateConfig(mutate func(*Config)) Config {
	cfg := Config{
		Writer: "opus[medium]", WriterTimeoutMin: 180, Verify: "", VerifyAttempts: 3, ReviewMaxBounces: 1, ReviewHardMaxBounces: 4,
		ReviewerModel: burlerengine.ModelSpecList{"sonnet[high]"}, FixerModel: burlerengine.ModelSpecList{""},
		ReviewerFan: burlerengine.FanList{""}, FixerFan: burlerengine.FanList{""},
		TreeScope: map[string]string{},
	}
	mutate(&cfg)
	return cfg
}

// withTreeScope returns the template with its tree_scope replaced by the given text after the key and its tree_scope_verify by verify.
func withTreeScope(trees, verify string) string {
	return strings.NewReplacer("tree_scope: {}", "tree_scope:"+trees, `tree_scope_verify: ""`, "tree_scope_verify: "+strconv.Quote(verify)).Replace(ConfigTemplate())
}

// TestLoadConfig covers the template's defaults, a review budget of 0, the model and fan keys' grammar and refusals, the tree-scope keys' loading and refusals, a writer that fails the model-spec grammar, and a base directory with no darn.yaml.
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	noTreeCommands := []string{"lyx config darn --set tree_scope.<tree>=<target>", "lyx config darn --set tree_scope_verify=", "is written in lyx config darn's editor"}
	tests := []struct {
		name     string
		contents string
		seed     bool
		want     Config
		wantErr  []string
		wantIs   error
	}{
		{
			name: "trees with an empty tree_scope_verify load", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...\n  docs: \"\"", ""),
			want:     templateConfig(func(c *Config) { c.TreeScope = map[string]string{"tools": "./tools/...", "docs": ""} }),
		},
		{
			name: "a trailing slash on a tree key is trimmed", seed: true,
			contents: withTreeScope("\n  tools/: ./tools/...", "lyx gate test {targets}"),
			want: templateConfig(func(c *Config) {
				c.TreeScope, c.TreeScopeVerify = map[string]string{"tools": "./tools/..."}, "lyx gate test {targets}"
			}),
		},
		{
			name: "tree_scope_verify with no tree", seed: true,
			contents: withTreeScope(" {}", "lyx gate test {targets}"),
			wantErr:  noTreeCommands, wantIs: ErrTreeScopeWithoutTrees,
		},
		{
			name: "tree_scope_verify without targets", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...", "lyx gate test"),
			wantErr:  []string{"lyx config darn --set tree_scope_verify=<command holding {targets}>"},
		},
		{
			name: "tree keys written twice", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...\n  tools/: ./tools/...", ""),
			wantErr:  []string{`"tools" and "tools/"`}, wantIs: ErrTreeScopeKey,
		},
		{
			name: "template defaults", contents: ConfigTemplate(), seed: true,
			want: templateConfig(func(*Config) {}),
		},
		{
			name: "review off", seed: true,
			contents: strings.Replace(ConfigTemplate(), "review_max_bounces: 1", "review_max_bounces: 0", 1),
			want:     templateConfig(func(c *Config) { c.ReviewMaxBounces = 0 }),
		},
		{
			name: "per-round model lists", seed: true,
			contents: strings.NewReplacer("darn_reviewer_model: sonnet[high]", "darn_reviewer_model:\n  - opus[high]\n  - sonnet[medium]", "darn_fixer_model: \"\"", "darn_fixer_model: sonnet[low]").Replace(ConfigTemplate()),
			want: templateConfig(func(c *Config) {
				c.ReviewerModel, c.FixerModel = burlerengine.ModelSpecList{"opus[high]", "sonnet[medium]"}, burlerengine.ModelSpecList{"sonnet[low]"}
			}),
		},
		{
			name: "empty model list reads as unset", seed: true,
			contents: strings.Replace(ConfigTemplate(), "darn_reviewer_model: sonnet[high]", "darn_reviewer_model: []", 1),
			want:     templateConfig(func(c *Config) { c.ReviewerModel = burlerengine.ModelSpecList{} }),
		},
		{
			name: "model entry that is no model-spec", seed: true,
			contents: strings.Replace(ConfigTemplate(), "darn_fixer_model: \"\"", "darn_fixer_model:\n  - sonnet[low]\n  - \"opus[effort\"", 1),
			wantErr:  []string{`darn config key "darn_fixer_model" entry 2`, "set the key to a model-spec"},
		},
		{
			name: "model value of the wrong shape", seed: true,
			contents: strings.Replace(ConfigTemplate(), "darn_reviewer_model: sonnet[high]", "darn_reviewer_model:\n  model: opus", 1),
			wantErr:  []string{`darn config key "darn_reviewer_model"`, "neither a model-spec nor a list of model-specs"},
		},
		{
			name: "per-round fan lists", seed: true,
			contents: strings.NewReplacer("darn_reviewer_fan: \"\"", "darn_reviewer_fan:\n  - review-code\n  - \"\"", "darn_fixer_fan: \"\"", "darn_fixer_fan: fix-code").Replace(ConfigTemplate()),
			want: templateConfig(func(c *Config) {
				c.ReviewerFan, c.FixerFan = burlerengine.FanList{"review-code", ""}, burlerengine.FanList{"fix-code"}
			}),
		},
		{
			name: "undefined fan", seed: true,
			contents: strings.Replace(ConfigTemplate(), "darn_fixer_fan: \"\"", "darn_fixer_fan:\n  - \"\"\n  - nosuchfan", 1),
			wantErr:  []string{`darn config key "darn_fixer_fan" entry 2`, `"nosuchfan"`, "define the fan in burler.yaml", `set the entry to ""`},
		},
		{
			name: "empty fan list", seed: true,
			contents: strings.Replace(ConfigTemplate(), "darn_reviewer_fan: \"\"", "darn_reviewer_fan: []", 1),
			wantErr:  []string{`darn config key "darn_reviewer_fan"`, "the list is empty", `write ""`},
		},
		{
			name: "fan value of the wrong shape", seed: true,
			contents: strings.Replace(ConfigTemplate(), "darn_reviewer_fan: \"\"", "darn_reviewer_fan:\n  model: opus", 1),
			wantErr:  []string{`darn config key "darn_reviewer_fan"`, "neither a fan name nor a list of fan names"},
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
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Errorf("LoadConfig() error = %v; want it to wrap %v", err, tt.wantIs)
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

	t.Run("a malformed tree key is refused naming the key as written", func(t *testing.T) {
		t.Parallel()
		for _, key := range []string{"", "/abs", "tools//x", "tools//", "./tools", "a/../b", `tools\x`} {
			board := t.TempDir()
			seedDarnConfig(t, board, withTreeScope("\n  "+strconv.Quote(key)+": x", ""))
			_, err := LoadConfig(configengine.Dirs{Board: board}, "darn")
			if !errors.Is(err, ErrTreeScopeKey) || !strings.Contains(err.Error(), strconv.Quote(key)) {
				t.Errorf("LoadConfig() with tree key %q = _, %v; want an error wrapping ErrTreeScopeKey and naming the key", key, err)
			}
		}
	})

	for key, local := range map[string]string{"verify": "verify: rm -rf .\n", "tree_scope": "tree_scope: {docs: \"\"}\n", "tree_scope_verify": "tree_scope_verify: rm\n", "darn_reviewer_fan": "darn_reviewer_fan: review-code\n", "darn_fixer_fan": "darn_fixer_fan: \"\"\n"} {
		t.Run("a local "+key+" is refused naming the local file and the key", func(t *testing.T) {
			t.Parallel()
			board, anchor := t.TempDir(), t.TempDir()
			seedDarnConfig(t, board, ConfigTemplate())
			localPath := writeLocalDarnConfig(t, anchor, local)

			_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "darn")
			if !errors.Is(err, configengine.ErrInvalid) {
				t.Fatalf("LoadConfig() error = %v; want ErrInvalid", err)
			}
			for _, want := range []string{localPath, key} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}

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

	t.Run("the shed override keys inherit by default, are read when set, and a local set 0 is a set 0", func(t *testing.T) {
		t.Parallel()
		spawnsOff, spawnsTwo := 0, 2
		tests := []struct {
			name  string
			board string
			local string
			want  sheddriver.Override
		}{
			{name: "the template inherits every key", board: ConfigTemplate()},
			{
				name:  "set values are read",
				board: strings.NewReplacer(`driver_kind: ""`, "driver_kind: go", `tackler: ""`, "tackler: sonnet[low]", "tackler_timeout_min: 0", "tackler_timeout_min: 5", "tackler_max_spawns: ~", "tackler_max_spawns: 2").Replace(ConfigTemplate()),
				want:  sheddriver.Override{DriverKind: "go", Tackler: "sonnet[low]", TacklerTimeoutMin: 5, TacklerMaxSpawns: &spawnsTwo},
			},
			{name: "a local tackler_max_spawns of 0 is a set 0", board: ConfigTemplate(), local: "tackler_max_spawns: 0\n", want: sheddriver.Override{TacklerMaxSpawns: &spawnsOff}},
		}
		for _, tt := range tests {
			board, anchor := t.TempDir(), t.TempDir()
			seedDarnConfig(t, board, tt.board)
			if tt.local != "" {
				writeLocalDarnConfig(t, anchor, tt.local)
			}
			got, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "darn")
			if err != nil {
				t.Fatalf("%s: LoadConfig() = _, %v; want nil error", tt.name, err)
			}
			if !reflect.DeepEqual(got.ShedOverride(), tt.want) {
				t.Errorf("%s: ShedOverride() = %+v; want %+v", tt.name, got.ShedOverride(), tt.want)
			}
		}
	})

	t.Run("an invalid shed override value is refused naming darn and the key", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct{ old, new, key string }{
			{`driver_kind: ""`, "driver_kind: fast", "driver_kind"},
			{`tackler: ""`, `tackler: "not a spec["`, "tackler"},
			{"tackler_timeout_min: 0", "tackler_timeout_min: -1", "tackler_timeout_min"},
			{"tackler_max_spawns: ~", "tackler_max_spawns: -1", "tackler_max_spawns"},
		} {
			board := t.TempDir()
			seedDarnConfig(t, board, strings.Replace(ConfigTemplate(), tt.old, tt.new, 1))
			_, err := LoadConfig(configengine.Dirs{Board: board}, "darn")
			if err == nil || !strings.Contains(err.Error(), "darn config key") || !strings.Contains(err.Error(), tt.key) {
				t.Errorf("LoadConfig() with %q = _, %v; want a refusal naming %q and %q", tt.new, err, "darn config key", tt.key)
			}
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

// TestMigrateConfig covers the rename of the retired fan keys with their values and the new key's template comment, a new key already set winning over its retired one, and a migrated document passing unchanged.
func TestMigrateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		existing  string
		want      string
		wantLines []string
	}{
		{
			name:      "retired fan keys are renamed",
			existing:  "writer: opus[medium]\nreview_fan: [review-code, \"\"] # old comment\nreview_fix_fan: fix-code\n",
			want:      "writer: opus[medium]\ndarn_reviewer_fan: [review-code, \"\"] # " + templateLineComment(t, ReviewerFanKey) + "\ndarn_fixer_fan: fix-code # " + templateLineComment(t, FixerFanKey) + "\n",
			wantLines: []string{"review_fan: renamed to darn_reviewer_fan", "review_fix_fan: renamed to darn_fixer_fan"},
		},
		{
			name:      "a set new key wins",
			existing:  "review_fan: review-code\ndarn_reviewer_fan: \"\"\n",
			want:      "darn_reviewer_fan: \"\"\n",
			wantLines: []string{"review_fan: removed, darn_reviewer_fan already set"},
		},
		{
			name:     "a migrated document is unchanged",
			existing: "darn_reviewer_fan: review-code\n",
			want:     "darn_reviewer_fan: review-code\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, lines, err := MigrateConfig([]byte(tt.existing))
			if err != nil {
				t.Fatalf("MigrateConfig() = _, _, %v; want nil error", err)
			}
			if string(got) != tt.want {
				t.Errorf("MigrateConfig() document = %q; want %q", got, tt.want)
			}
			if !reflect.DeepEqual(lines, tt.wantLines) {
				t.Errorf("MigrateConfig() lines = %q; want %q", lines, tt.wantLines)
			}
		})
	}

	t.Run("a document that does not parse is invalid", func(t *testing.T) {
		t.Parallel()
		if _, _, err := MigrateConfig([]byte("review_fan: [unterminated\n")); !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("MigrateConfig(unparsable) error = %v; want ErrInvalid", err)
		}
	})
}

// templateLineComment returns the template's line comment on key, without its leading "# ".
func templateLineComment(t *testing.T, key string) string {
	t.Helper()
	for _, line := range strings.Split(ConfigTemplate(), "\n") {
		if strings.HasPrefix(line, key+":") {
			_, comment, _ := strings.Cut(line, "  # ")
			return comment
		}
	}
	t.Fatalf("the template holds no key %q", key)
	return ""
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
		// wantEditor pins that the error names the editor way forward in place of the --set verify= one.
		wantEditor bool
	}{
		{name: "value as written", seed: true, contents: strings.Replace(ConfigTemplate(), `verify: ""`, "verify: go test ./...", 1), want: "go test ./..."},
		{name: "malformed tree key", seed: true, contents: withTreeScope("\n  ./tools: x", ""), wantErr: true, wantEditor: true},
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
			if tt.wantEditor {
				if err == nil || !strings.Contains(err.Error(), "lyx config darn's editor") || !strings.Contains(err.Error(), "then re-run the lyx command that refused") || strings.Contains(err.Error(), "--set verify=") {
					t.Fatalf("VerifyCommand() = %q, %v; want the editor way forward and no --set verify=", got, err)
				}
				return
			}
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

// TestTreeScopedCommand covers tree scope being off, a path lying in a tree by equality or prefix and never by a shared name prefix, the touched trees' targets deduplicated in tree-path order, and a diff with a production path.
func TestTreeScopedCommand(t *testing.T) {
	t.Parallel()

	const command = "lyx gate test --tags integration --guards {targets}"
	trees := map[string]string{"docs": "", "tools": "./tools/...", "scripts": "./tools/...", "cmd/x": "./cmd/x"}
	tests := []struct {
		name       string
		verify     string
		paths      []string
		want       string
		wantScoped bool
	}{
		{name: "tree scope off with an empty tree_scope_verify", verify: "", paths: []string{"docs/a.md"}},
		{name: "a docs and a tools path", verify: command, paths: []string{"docs/a.md", "tools/x/y.go"}, want: "lyx gate test --tags integration --guards ./tools/...", wantScoped: true},
		{name: "two trees sharing one target yield it once", verify: command, paths: []string{"scripts/a.sh", "tools/b.go"}, want: "lyx gate test --tags integration --guards ./tools/...", wantScoped: true},
		{name: "targets come in tree-path order", verify: command, paths: []string{"tools/b.go", "cmd/x/main.go"}, want: "lyx gate test --tags integration --guards ./cmd/x ./tools/...", wantScoped: true},
		{name: "a tree does not claim a path sharing only its name prefix", verify: command, paths: []string{"toolsx/a.go"}},
		{name: "a path equal to a tree path lies in it", verify: command, paths: []string{"tools"}, want: "lyx gate test --tags integration --guards ./tools/...", wantScoped: true},
		{name: "a non-ASCII path lies in its tree", verify: command, paths: []string{"docs/ä.md"}, want: "lyx gate test --tags integration --guards ", wantScoped: true},
		{name: "one production path makes the diff not tree-scoped", verify: command, paths: []string{"docs/a.md", "internal/x/x.go"}},
		{name: "an empty path list yields the command with an empty targets", verify: command, want: "lyx gate test --tags integration --guards ", wantScoped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, scoped := TreeScopedCommand(Config{TreeScope: trees, TreeScopeVerify: tt.verify}, tt.paths)
			if got != tt.want || scoped != tt.wantScoped {
				t.Errorf("TreeScopedCommand(%q) = %q, %v; want %q, %v", tt.paths, got, scoped, tt.want, tt.wantScoped)
			}
		})
	}
}
