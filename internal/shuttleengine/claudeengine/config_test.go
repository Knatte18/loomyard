// config_test.go pins claude.yaml's load: the template on an absent base, the stack of its layers, and the local-layer rule on its keys.

package claudeengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
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

// TestLoadConfig pins claude.yaml's resolution: the template on an absent base, a repo default under a worktree file, the local layer's admitted and refused keys, and the binary marker's environment expansion.
// It does not run in parallel: every row clears or sets LYX_SHUTTLE_CLAUDE, which is process-global.
func TestLoadConfig(t *testing.T) {
	clearMarker := func(t *testing.T) {
		t.Helper()
		t.Setenv("LYX_SHUTTLE_CLAUDE", "")
		os.Unsetenv("LYX_SHUTTLE_CLAUDE")
	}

	t.Run("an absent base resolves the template", func(t *testing.T) {
		clearMarker(t)
		cfg, err := LoadConfig(configengine.Dirs{Anchor: t.TempDir()})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		want := Config{
			Binary:              "",
			DenyAgentTool:       true,
			DenyAskUserQuestion: true,
			PromptCacheTTL:      "5m",
			PromptCacheTTLRoles: map[string]string{"driver": "1h", "webster": "1h"},
		}
		if !reflect.DeepEqual(cfg, want) {
			t.Errorf("LoadConfig() = %+v; want %+v", cfg, want)
		}
	})

	t.Run("the worktree file wins over the repo default", func(t *testing.T) {
		clearMarker(t)
		board, anchor := t.TempDir(), t.TempDir()
		writeClaudeLayer(t, configengine.ConfigFile(board, "claude"), strings.Replace(ConfigTemplate(), "deny_agent_tool: true", "deny_agent_tool: false", 1))
		writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), "prompt_cache_ttl: 1h\n")
		cfg, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if cfg.PromptCacheTTL != "1h" || cfg.DenyAgentTool {
			t.Errorf("LoadConfig() = %+v; want prompt_cache_ttl 1h from the worktree file and deny_agent_tool false from the repo default", cfg)
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

	t.Run("the environment marker resolves the binary", func(t *testing.T) {
		t.Setenv("LYX_SHUTTLE_CLAUDE", "/opt/claude/bin/claude")
		cfg, err := LoadConfig(configengine.Dirs{Anchor: t.TempDir()})
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if cfg.Binary != "/opt/claude/bin/claude" {
			t.Errorf("LoadConfig().Binary = %q; want the environment value", cfg.Binary)
		}
	})
}
