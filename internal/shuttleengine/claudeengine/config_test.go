// config_test.go pins claude.yaml's load.
// It covers the template on an absent base, the stack of its layers, and the local-layer rule on its keys.

package claudeengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeClaudeLayer writes document to path, creating its directory.
func writeClaudeLayer(t testing.TB, path, document string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestLoadConfig pins claude.yaml's resolution.
// It covers the template on an absent base and a repo default under a worktree file.
// It also covers the local layer's admitted and refused keys and the binary marker's environment expansion.
// It does not run in parallel: every row clears or sets LYM_SHUTTLE_CLAUDE, which is process-global.
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	clearMarker := func(t *testing.T) {
		t.Helper()
		t.Setenv("LYM_SHUTTLE_CLAUDE", "")
		os.Unsetenv("LYM_SHUTTLE_CLAUDE")
	}

	t.Run("an absent base resolves the template", func(t *testing.T) {
		clearMarker(t)
		cfg, err := LoadConfig(configengine.Dirs{Anchor: t.TempDir()})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		want := Config{
			Binary:              "",
			DenyAskUserQuestion: true,
			PromptCacheTTL:      "5m",
			PromptCacheTTLRoles: map[string]string{"driver": "5m", "webster": "1h", "orch": "1h"},
			KeepWarmCap:         "250s",
			KeepWarmCapRoles:    map[string]string{},
			Accounts:            []AccountConfig{},
			BatchingNudge: BatchingNudgeConfig{
				Enabled:       true,
				LookupChain:   true,
				ReadSlice:     true,
				EditChain:     true,
				ContextTokens: 150000,
				LookupRun:     3,
				EditRun:       2,
				SliceLines:    2000,
				MaxNudges:     5,
			},
			SessionScope: ScopeConfig{
				Enabled: true,
				SkillsOff: []string{
					"anthropic-skills:built-in-browser",
					"anthropic-skills:chrome-browser",
					"anthropic-skills:computer-use",
					"anthropic-skills:deep-research",
					"anthropic-skills:docs",
					"anthropic-skills:docx",
					"anthropic-skills:google-workspace",
					"anthropic-skills:import-memory",
					"anthropic-skills:morning",
					"anthropic-skills:pdf",
					"anthropic-skills:pptx",
					"anthropic-skills:skill-creator",
					"anthropic-skills:xlsx",
				},
			},
		}
		if !reflect.DeepEqual(cfg, want) {
			t.Errorf("LoadConfig() = %+v; want %+v", cfg, want)
		}
	})

	t.Run("the worktree file wins over the repo default", func(t *testing.T) {
		clearMarker(t)
		board, anchor := t.TempDir(), t.TempDir()
		writeClaudeLayer(t, configengine.ConfigFile(board, "claude"), strings.Replace(ConfigTemplate(), "deny_ask_user_question: true", "deny_ask_user_question: false", 1))
		writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), "prompt_cache_ttl: 1h\n")
		cfg, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if cfg.PromptCacheTTL != "1h" || cfg.DenyAskUserQuestion {
			t.Errorf("LoadConfig() = %+v; want prompt_cache_ttl 1h from the worktree file and deny_ask_user_question false from the repo default", cfg)
		}
	})

	t.Run("a repo default holding the retired deny_agent_tool loads without it", func(t *testing.T) {
		clearMarker(t)
		board, anchor := t.TempDir(), t.TempDir()
		writeClaudeLayer(t, configengine.ConfigFile(board, "claude"), ConfigTemplate()+"deny_agent_tool: false\n")
		writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), "")
		cfg, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if !cfg.DenyAskUserQuestion || cfg.PromptCacheTTL != "5m" {
			t.Errorf("LoadConfig() = %+v; want the template's values", cfg)
		}
	})

	t.Run("a local file holds the cache lifetimes", func(t *testing.T) {
		clearMarker(t)
		board, anchor := t.TempDir(), t.TempDir()
		writeClaudeLayer(t, configengine.ConfigFile(board, "claude"), ConfigTemplate())
		writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), "")
		writeClaudeLayer(t, configengine.LocalFile(anchor, "claude"), "prompt_cache_ttl: 1h\nprompt_cache_ttl_roles:\n  orch: 1h\n  reed: 5m\n")
		cfg, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		wantRoles := map[string]string{"orch": "1h", "reed": "5m"}
		if cfg.PromptCacheTTL != "1h" || !reflect.DeepEqual(cfg.PromptCacheTTLRoles, wantRoles) {
			t.Errorf("LoadConfig() = %+v; want prompt_cache_ttl 1h and roles %v", cfg, wantRoles)
		}
	})

	for _, key := range []string{"binary: x", "deny_agent_tool: false", "deny_ask_user_question: false"} {
		t.Run("a local "+strings.Split(key, ":")[0]+" is refused", func(t *testing.T) {
			clearMarker(t)
			board, anchor := t.TempDir(), t.TempDir()
			writeClaudeLayer(t, configengine.ConfigFile(board, "claude"), ConfigTemplate())
			writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), "")
			writeClaudeLayer(t, configengine.LocalFile(anchor, "claude"), key+"\n")
			_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
			name := strings.Split(key, ":")[0]
			if err == nil || !strings.Contains(err.Error(), "claude.local.yaml") || !strings.Contains(err.Error(), name) {
				t.Errorf("LoadConfig() = _, %v; want a refusal naming claude.local.yaml and %q", err, name)
			}
		})
	}

	// accountRows load the layer files below.
	// Each has DIR replaced by a temporary directory holding real/ and a link/ to it.
	// Each refusal row reaches its own branch after every earlier check passes.
	accountRows := []struct {
		name      string
		machine   string
		hub       string
		worktree  string
		wantParts []string
	}{
		{"a duplicate name", "accounts:\n  - {name: a, config_dir: DIR/x, weight: 1}\n  - {name: a, config_dir: DIR/y, weight: 1}\n", "", "", []string{"accounts", "a"}},
		{"an upper-case name", "accounts:\n  - {name: Work, config_dir: DIR/x, weight: 1}\n", "", "", []string{"accounts", "Work"}},
		{"a relative path", "accounts:\n  - {name: a, config_dir: rel/x, weight: 1}\n", "", "", []string{"accounts", "rel/x"}},
		{"two entries naming one directory through a symlink", "accounts:\n  - {name: a, config_dir: DIR/real, weight: 1}\n  - {name: b, config_dir: DIR/link, weight: 1}\n", "", "", []string{"accounts", "a", "b"}},
		{"a negative weight", "accounts:\n  - {name: a, config_dir: DIR/x, weight: -1}\n", "", "", []string{"accounts", "weight"}},
		{"an unknown window", "accounts:\n  - {name: a, config_dir: DIR/x, weight: 1, windows: {hourly: 50}}\n", "", "", []string{"accounts", "hourly"}},
		{"a threshold of 0", "accounts:\n  - {name: a, config_dir: DIR/x, weight: 1, windows: {five_hour: 0}}\n", "", "", []string{"accounts", "five_hour"}},
		{"a threshold of 101", "accounts:\n  - {name: a, config_dir: DIR/x, weight: 1, windows: {seven_day: 101}}\n", "", "", []string{"accounts", "seven_day"}},
		{"a hub pin naming an unlisted account", "accounts:\n  - {name: a, config_dir: DIR/x, weight: 1}\naccount_pins:\n  hub: ghost\n", "", "", []string{"account_pins.hub", "ghost"}},
		{"a beams pin naming a retired account", "accounts:\n  - {name: a, config_dir: DIR/x, weight: 0}\naccount_pins:\n  beams: a\n", "", "", []string{"account_pins.beams", "a"}},
		{"accounts in the hub file", "", "accounts:\n  - {name: a, config_dir: DIR/x, weight: 1}\n", "", []string{"accounts", "claude.yaml", "--layer default"}},
		{"a hub pin in the worktree file while the machine file holds it too", "accounts:\n  - {name: a, config_dir: DIR/x, weight: 1}\naccount_pins:\n  hub: a\n", "", "account_pins:\n  hub: a\n", []string{"account_pins.hub", "--layer worktree"}},
	}
	loadAccountRow := func(t *testing.T, machine, hub, worktree string) (Config, string, error) {
		t.Helper()
		clearMarker(t)
		dir := t.TempDir()
		t.Setenv("HOME", dir)
		if err := os.Mkdir(filepath.Join(dir, "real"), 0o755); err != nil {
			t.Fatalf("Mkdir(real) = %v; want nil", err)
		}
		if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
			t.Fatalf("Symlink(link) = %v; want nil", err)
		}
		board, anchor := t.TempDir(), t.TempDir()
		fill := func(doc string) string { return strings.ReplaceAll(doc, "DIR", dir) }
		hubDocument := ConfigTemplate()
		if hub != "" {
			hubDocument = strings.Replace(hubDocument, "accounts: []", strings.TrimSuffix(fill(hub), "\n"), 1)
		}
		writeClaudeLayer(t, configengine.ConfigFile(board, "claude"), hubDocument)
		writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), fill(worktree))
		if machine != "" {
			writeClaudeLayer(t, configengine.MachineFile(board, "claude"), fill(machine))
		}
		cfg, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
		return cfg, dir, err
	}

	t.Run("a valid machine-layer list loads with a leading ~/ expanded", func(t *testing.T) {
		cfg, dir, err := loadAccountRow(t, "accounts:\n  - {name: home, config_dir: '~/x/', weight: 2, windows: {five_hour: 80}}\n  - {name: work, config_dir: DIR/y, weight: 1}\naccount_pins:\n  hub: home\n", "", "")
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		wantAccounts := []AccountConfig{
			{Name: "home", ConfigDir: filepath.Join(dir, "x"), Weight: 2, Windows: map[string]int{"five_hour": 80}},
			{Name: "work", ConfigDir: filepath.Join(dir, "y"), Weight: 1},
		}
		if !reflect.DeepEqual(cfg.Accounts, wantAccounts) || cfg.AccountPins != (AccountPins{Hub: "home"}) {
			t.Errorf("LoadConfig() accounts = %+v, pins = %+v; want %+v and hub pin home", cfg.Accounts, cfg.AccountPins, wantAccounts)
		}
	})

	for _, row := range accountRows {
		t.Run("a refusal for "+row.name, func(t *testing.T) {
			_, _, err := loadAccountRow(t, row.machine, row.hub, row.worktree)
			if err == nil || !errors.Is(err, configengine.ErrInvalid) {
				t.Fatalf("LoadConfig() = _, %v; want an error marked configengine.ErrInvalid", err)
			}
			for _, part := range append(row.wantParts, "lym config claude --layer machine") {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("LoadConfig() error = %q; want it to contain %q", err, part)
				}
			}
		})
	}

	t.Run("a refusal for a leading ~/ with no home directory", func(t *testing.T) {
		clearMarker(t)
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		board, anchor := t.TempDir(), t.TempDir()
		writeClaudeLayer(t, configengine.ConfigFile(board, "claude"), ConfigTemplate())
		writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), "")
		writeClaudeLayer(t, configengine.MachineFile(board, "claude"), "accounts:\n  - {name: a, config_dir: '~/x', weight: 1}\n")
		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
		if err == nil || !errors.Is(err, configengine.ErrInvalid) {
			t.Fatalf("LoadConfig() = _, %v; want an error marked configengine.ErrInvalid", err)
		}
		for _, part := range []string{"accounts", "~/x", "home directory", "lym config claude --layer machine"} {
			if !strings.Contains(err.Error(), part) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err, part)
			}
		}
	})

	t.Run("the environment marker resolves the binary", func(t *testing.T) {
		t.Setenv("LYM_SHUTTLE_CLAUDE", "/opt/claude/bin/claude")
		cfg, err := LoadConfig(configengine.Dirs{Anchor: t.TempDir()})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if cfg.Binary != "/opt/claude/bin/claude" {
			t.Errorf("LoadConfig().Binary = %q; want the environment value", cfg.Binary)
		}
	})
}
