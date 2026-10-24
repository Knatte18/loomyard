//go:build tier0

// config_test.go — tier 0 unit tests for darnengine.LoadConfig, darnengine.TreeScopedTargets and darnengine.MigrateConfig.
//
// Seeds a bare t.TempDir() with just a _lyx/config/darn.yaml file (no real hub, no SeedConfig, no git spawn).

package darnengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

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
		Writer: "opus[medium]", WriterTimeoutMin: 180, VerifyAttempts: 3,
		TreeScope: map[string]string{},
	}
	mutate(&cfg)
	return cfg
}

// withTreeScope returns the template with its tree_scope replaced by the given text after the key.
func withTreeScope(trees string) string {
	return strings.Replace(ConfigTemplate(), "tree_scope: {}", "tree_scope:"+trees, 1)
}

// TestLoadConfig covers the template's defaults, the tree_scope key's loading and refusals, a retired key, a writer that fails the model-spec grammar, and a base directory with no darn.yaml.
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		seed     bool
		want     Config
		wantErr  []string
		wantIs   error
	}{
		{
			name: "trees load", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...\n  docs: \"\""),
			want:     templateConfig(func(c *Config) { c.TreeScope = map[string]string{"tools": "./tools/...", "docs": ""} }),
		},
		{
			name: "a trailing slash on a tree key is trimmed", seed: true,
			contents: withTreeScope("\n  tools/: ./tools/..."),
			want:     templateConfig(func(c *Config) { c.TreeScope = map[string]string{"tools": "./tools/..."} }),
		},
		{
			name: "a retired verify key still loads", seed: true,
			contents: ConfigTemplate() + "verify: go test ./...\n",
			want:     templateConfig(func(*Config) {}),
		},
		{
			name: "a retired tree_scope_verify key still loads", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...") + "tree_scope_verify: \"lyx gate test --wait --tier 1 {targets}\"\n",
			want:     templateConfig(func(c *Config) { c.TreeScope = map[string]string{"tools": "./tools/..."} }),
		},
		{
			name: "tree keys written twice", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...\n  tools/: ./tools/..."),
			wantErr:  []string{`"tools" and "tools/"`}, wantIs: ErrTreeScopeKey,
		},
		{
			name: "template defaults", contents: ConfigTemplate(), seed: true,
			want: templateConfig(func(*Config) {}),
		},
		{
			name: "invalid writer spec", seed: true,
			contents: strings.Replace(ConfigTemplate(), "writer: opus[medium]", "writer: \"opus[medium\"", 1),
			wantErr:  []string{"writer"},
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
			seedDarnConfig(t, board, withTreeScope("\n  "+strconv.Quote(key)+": x"))
			_, err := LoadConfig(configengine.Dirs{Board: board}, "darn")
			if !errors.Is(err, ErrTreeScopeKey) || !strings.Contains(err.Error(), strconv.Quote(key)) {
				t.Errorf("LoadConfig() with tree key %q = _, %v; want an error wrapping ErrTreeScopeKey and naming the key", key, err)
			}
		}
	})

	for key, local := range map[string]string{"verify": "verify: rm -rf .\n", "tree_scope": "tree_scope: {docs: \"\"}\n", "tree_scope_verify": "tree_scope_verify: rm\n"} {
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

// TestTreeScopedTargets covers tree scope being off with no tree, a path lying in a tree by equality or prefix and never by a shared name prefix, a path under nested trees touching each, the touched trees' targets deduplicated in tree-path order, a diff with a production path, and a scoped diff with no target answering no targets.
func TestTreeScopedTargets(t *testing.T) {
	t.Parallel()

	trees := map[string]string{"docs": "", "tools": "./tools/...", "scripts": "./tools/...", "cmd/x": "./cmd/x"}
	tests := []struct {
		name       string
		trees      map[string]string
		paths      []string
		want       []string
		wantScoped bool
	}{
		{name: "tree scope off with no tree", trees: map[string]string{}, paths: []string{"docs/a.md"}},
		{name: "a docs and a tools path", paths: []string{"docs/a.md", "tools/x/y.go"}, want: []string{"./tools/..."}, wantScoped: true},
		{name: "two trees sharing one target yield it once", paths: []string{"scripts/a.sh", "tools/b.go"}, want: []string{"./tools/..."}, wantScoped: true},
		{name: "targets come in tree-path order", paths: []string{"tools/b.go", "cmd/x/main.go"}, want: []string{"./cmd/x", "./tools/..."}, wantScoped: true},
		{name: "a path under nested trees touches both", trees: map[string]string{"docs": "./docs-tests", "docs/api": "./api"}, paths: []string{"docs/api/a.md"}, want: []string{"./docs-tests", "./api"}, wantScoped: true},
		{name: "a tree does not claim a path sharing only its name prefix", paths: []string{"toolsx/a.go"}},
		{name: "a path equal to a tree path lies in it", paths: []string{"tools"}, want: []string{"./tools/..."}, wantScoped: true},
		{name: "a non-ASCII path lies in its tree", paths: []string{"docs/ä.md"}, wantScoped: true},
		{name: "one production path makes the diff not tree-scoped", paths: []string{"docs/a.md", "internal/x/x.go"}},
		{name: "an empty path list is scoped with no targets", wantScoped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			configured := trees
			if tt.trees != nil {
				configured = tt.trees
			}
			got, scoped := TreeScopedTargets(Config{TreeScope: configured}, tt.paths)
			if !slices.Equal(got, tt.want) || scoped != tt.wantScoped {
				t.Errorf("TreeScopedTargets(%q) = %q, %v; want %q, %v", tt.paths, got, scoped, tt.want, tt.wantScoped)
			}
		})
	}
}

// TestMigrateConfig covers the three states of a hub file's tree_scope_verify: held empty empties tree_scope and prunes the key, held set prunes the key and keeps tree_scope, absent changes nothing; a second pass over each changes nothing, and a malformed document is an invalid-config error.
func TestMigrateConfig(t *testing.T) {
	t.Parallel()

	const treesText = "\n  tools: ./tools/..."
	withVerify := func(trees, verify string) string {
		return strings.Replace(withTreeScope(trees), "driver_kind:", "tree_scope_verify: "+strconv.Quote(verify)+"\ndriver_kind:", 1)
	}
	tests := []struct {
		name      string
		existing  string
		wantTrees map[string]string
		wantLines int
	}{
		{name: "held empty empties tree_scope and prunes the key", existing: withVerify(treesText, ""), wantTrees: map[string]string{}, wantLines: 1},
		{name: "held set prunes the key and keeps tree_scope", existing: withVerify(treesText, "lyx gate test --wait --tier 1 {targets}"), wantTrees: map[string]string{"tools": "./tools/..."}, wantLines: 1},
		{name: "absent changes nothing and keeps the trees the hub holds", existing: withTreeScope(treesText), wantTrees: map[string]string{"tools": "./tools/..."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, lines, err := MigrateConfig([]byte(tt.existing))
			if err != nil {
				t.Fatalf("MigrateConfig() = _, _, %v; want nil error", err)
			}
			if len(lines) != tt.wantLines {
				t.Errorf("MigrateConfig() lines = %q; want %d", lines, tt.wantLines)
			}
			if tt.wantLines == 0 && string(got) != tt.existing {
				t.Errorf("MigrateConfig() = %q; want the document unchanged", got)
			}
			if strings.Contains(string(got), "tree_scope_verify") {
				t.Errorf("MigrateConfig() = %q; want no tree_scope_verify key", got)
			}

			board := t.TempDir()
			seedDarnConfig(t, board, string(got))
			cfg, err := LoadConfig(configengine.Dirs{Board: board}, "darn")
			if err != nil || !reflect.DeepEqual(cfg.TreeScope, tt.wantTrees) {
				t.Errorf("LoadConfig() after MigrateConfig = %v, %v; want tree_scope %v", cfg.TreeScope, err, tt.wantTrees)
			}

			again, againLines, err := MigrateConfig(got)
			if err != nil || len(againLines) != 0 || string(again) != string(got) {
				t.Errorf("second MigrateConfig() = %q, %q, %v; want the document unchanged with no lines", again, againLines, err)
			}
		})
	}

	t.Run("a malformed document is an invalid-config error", func(t *testing.T) {
		t.Parallel()
		for _, existing := range []string{"writer: [\n", "- a\n- b\n"} {
			if _, _, err := MigrateConfig([]byte(existing)); !errors.Is(err, configengine.ErrInvalid) {
				t.Errorf("MigrateConfig(%q) error = %v; want ErrInvalid", existing, err)
			}
		}
	})
}
