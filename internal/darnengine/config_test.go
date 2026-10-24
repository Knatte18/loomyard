// config_test.go — untagged Tier-1 unit tests for darnengine.LoadConfig and darnengine.TreeScopedCommand.
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

// withTreeScope returns the template with its tree_scope replaced by the given text after the key and its tree_scope_verify by verify.
func withTreeScope(trees, verify string) string {
	return strings.NewReplacer("tree_scope: {}", "tree_scope:"+trees, `tree_scope_verify: ""`, "tree_scope_verify: "+strconv.Quote(verify)).Replace(ConfigTemplate())
}

// TestLoadConfig covers the template's defaults, the tree-scope keys' loading and refusals, a writer that fails the model-spec grammar, and a base directory with no darn.yaml.
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
			contents: withTreeScope("\n  tools/: ./tools/...", "lyx gate test --wait --tier 1 {targets}"),
			want: templateConfig(func(c *Config) {
				c.TreeScope, c.TreeScopeVerify = map[string]string{"tools": "./tools/..."}, "lyx gate test --wait --tier 1 {targets}"
			}),
		},
		{
			name: "tree_scope_verify with no tree", seed: true,
			contents: withTreeScope(" {}", "lyx gate test --wait --tier 1 {targets}"),
			wantErr:  noTreeCommands, wantIs: ErrTreeScopeWithoutTrees,
		},
		{
			name: "tree_scope_verify without targets", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...", "lyx gate test --wait --tier 1"),
			wantErr:  []string{"lyx config darn --set tree_scope_verify=<one lyx gate test --wait call holding {targets}>"},
		},
		{
			name: "tree_scope_verify of two calls joined by &&", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...", "lyx gate test --wait --tier 1 {targets} && lyx gate test --wait --tier 2 {targets}"),
			wantErr:  []string{`"tree_scope_verify" is not exactly one lyx gate test call`, "lyx config darn --set tree_scope_verify=<one lyx gate test --wait call holding {targets}>"},
		},
		{
			name: "tree_scope_verify without --wait", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...", "lyx gate test --tier 1 {targets}"),
			wantErr:  []string{`"tree_scope_verify" is not exactly one lyx gate test call`},
		},
		{
			name: "tree_scope_verify piped into another command", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...", "lyx gate test --wait --tier 1 {targets} | tee out"),
			wantErr:  []string{`"tree_scope_verify" is not exactly one lyx gate test call`},
		},
		{
			name: "tree_scope_verify that is not a gate call", seed: true,
			contents: withTreeScope("\n  tools: ./tools/...", "go test --wait {targets}"),
			wantErr:  []string{`"tree_scope_verify" is not exactly one lyx gate test call`},
		},
		{
			name: "a retired verify key still loads", seed: true,
			contents: ConfigTemplate() + "verify: go test ./...\n",
			want:     templateConfig(func(*Config) {}),
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
			seedDarnConfig(t, board, withTreeScope("\n  "+strconv.Quote(key)+": x", ""))
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

// TestTreeScopedCommand covers tree scope being off, a path lying in a tree by equality or prefix and never by a shared name prefix, a path under nested trees touching each, the touched trees' targets deduplicated in tree-path order, a diff with a production path, and a scoped diff with no target answering an empty command.
func TestTreeScopedCommand(t *testing.T) {
	t.Parallel()

	const command = "lyx gate test --wait --tier 1 {targets}"
	trees := map[string]string{"docs": "", "tools": "./tools/...", "scripts": "./tools/...", "cmd/x": "./cmd/x"}
	tests := []struct {
		name       string
		verify     string
		trees      map[string]string
		paths      []string
		want       string
		wantScoped bool
	}{
		{name: "tree scope off with an empty tree_scope_verify", verify: "", paths: []string{"docs/a.md"}},
		{name: "a docs and a tools path", verify: command, paths: []string{"docs/a.md", "tools/x/y.go"}, want: "lyx gate test --wait --tier 1 ./tools/...", wantScoped: true},
		{name: "two trees sharing one target yield it once", verify: command, paths: []string{"scripts/a.sh", "tools/b.go"}, want: "lyx gate test --wait --tier 1 ./tools/...", wantScoped: true},
		{name: "targets come in tree-path order", verify: command, paths: []string{"tools/b.go", "cmd/x/main.go"}, want: "lyx gate test --wait --tier 1 ./cmd/x ./tools/...", wantScoped: true},
		{name: "a path under nested trees touches both", verify: command, trees: map[string]string{"docs": "./docs-tests", "docs/api": "./api"}, paths: []string{"docs/api/a.md"}, want: "lyx gate test --wait --tier 1 ./docs-tests ./api", wantScoped: true},
		{name: "a tree does not claim a path sharing only its name prefix", verify: command, paths: []string{"toolsx/a.go"}},
		{name: "a path equal to a tree path lies in it", verify: command, paths: []string{"tools"}, want: "lyx gate test --wait --tier 1 ./tools/...", wantScoped: true},
		{name: "a non-ASCII path lies in its tree", verify: command, paths: []string{"docs/ä.md"}, want: "", wantScoped: true},
		{name: "one production path makes the diff not tree-scoped", verify: command, paths: []string{"docs/a.md", "internal/x/x.go"}},
		{name: "an empty path list is scoped with an empty command", verify: command, want: "", wantScoped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			configured := trees
			if tt.trees != nil {
				configured = tt.trees
			}
			got, scoped := TreeScopedCommand(Config{TreeScope: configured, TreeScopeVerify: tt.verify}, tt.paths)
			if got != tt.want || scoped != tt.wantScoped {
				t.Errorf("TreeScopedCommand(%q) = %q, %v; want %q, %v", tt.paths, got, scoped, tt.want, tt.wantScoped)
			}
		})
	}
}
