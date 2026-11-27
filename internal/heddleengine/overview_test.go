// overview_test.go drives RenderOverview over temporary directories standing in for the Bolt checkout.

package heddleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// readOverview returns the overview file RenderOverview wrote into bolt.
func readOverview(t *testing.T, bolt string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(bolt, OverviewFileName))
	if err != nil {
		t.Fatalf("read %s: %v", OverviewFileName, err)
	}
	return string(data)
}

// TestRenderOverview pins the overview's table and its sections by the place entries land in.
// The store holds text entries aimed at claude-file, at the shared agents-file and at orch-prompt.
// It also holds a claude-settings entry and one whose text is no object.
// It pins a table row per entry and place, the repository's texts in full, and lym's own folded under a summary.
// It pins a shared entry under each file it renders into, the widened fence of a backtick run and an escaped table pipe.
// It pins the settings object with its setters marked, a note for the entry that holds no object,
// a place no entry reaches left out, and a second call changing nothing.
func TestRenderOverview(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	shipped := shippedStore.Ordered()[0]
	bolt := t.TempDir()
	saveStore(t, bolt, Store{Entries: []Entry{
		entryFor("house-rules", "Be kind.", 10, "claude-file"),
		entryFor("shared-rules", "Be brief.", 15, "agents-file"),
		entryFor("my-settings", `{"model": "opus"}`, 20, "claude-settings"),
		entryFor("fenced-rules", "Use ```go fences | always.", 30, "claude-file"),
		entryFor("loose-settings", "not json", 40, "claude-settings"),
		entryFor("orch-notes", "# Orch notes\n\nStay calm.", 50, OrchPromptTarget),
	}})

	changed, err := RenderOverview(bolt)
	if err != nil || !changed {
		t.Fatalf("RenderOverview() = %v, %v; want true", changed, err)
	}
	text := readOverview(t, bolt)

	if !strings.HasPrefix(text, "# Heddle\n") {
		t.Errorf("overview opens with %q; want the # Heddle title", text[:min(len(text), 20)])
	}
	for _, want := range []string{
		"| `CLAUDE.local.md` | **repo** | `house-rules` | Be kind. |\n",
		"| `CLAUDE.local.md` | **repo** | `shared-rules` | Be brief. |\n",
		"| `AGENTS.override.md` | **repo** | `shared-rules` | Be brief. |\n",
		"| `CLAUDE.local.md` | **repo** | `fenced-rules` | Use ```go fences \\| always. |\n",
		"| `.claude/settings.local.json` | **repo** | `my-settings` | `model` |\n",
		"| `.claude/settings.local.json` | **repo** | `loose-settings` | holds no JSON object |\n",
		"| `.claude/settings.local.json` | derived | `" + AgentDefaultsSlug + "` | `effortLevel`, `env` |\n",
		"| System prompt of every orch session | **repo** | `orch-notes` | Orch notes |\n",
		"## `CLAUDE.local.md`\n\nRendered at every worktree's anchor.\n\n### From this repository\n\n" +
			"#### `house-rules`\n\n```markdown\nBe kind.\n```\n\n" +
			"#### `shared-rules`\n\n```markdown\nBe brief.\n```\n\n" +
			"#### `fenced-rules`\n\n````markdown\nUse ```go fences | always.\n````\n\n### From lym\n",
		"\n<details>\n<summary><code>" + shipped.Slug + "</code> — ",
		"## `AGENTS.override.md`\n\nRendered at every worktree's anchor.\n\n### From this repository\n\n" +
			"#### `shared-rules`\n\n```markdown\nBe brief.\n```\n\n### From lym\n",
		"\nNote: the text of entry \"loose-settings\" holds no JSON object",
		"\n\n  // my-settings (repo)\n  \"model\": \"opus\"\n}\n```\n",
		"## System prompt of every orch session\n\nAppended to the system prompt of every orch session at its launch.\n\n" +
			"### From this repository\n\n#### `orch-notes`\n\n```markdown\n# Orch notes\n\nStay calm.\n```\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("overview lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "System prompt of every claude session") {
		t.Errorf("overview shows the claude-prompt place, which no entry reaches:\n%s", text)
	}

	if changed, err := RenderOverview(bolt); err != nil || changed {
		t.Errorf("second RenderOverview() = %v, %v; want false", changed, err)
	}
}

// TestRenderOverviewSaysWhenTheEffortDefaultsAreLeftOut pins the overview of a hub whose agents.yaml fails to load.
// The render succeeds, a note says the effort defaults are left out, and no derived entry shows.
func TestRenderOverviewSaysWhenTheEffortDefaultsAreLeftOut(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	bolt := t.TempDir()
	writeAgentsConfig(t, bolt, invalidAgentsConfig)

	if _, err := RenderOverview(bolt); err != nil {
		t.Fatalf("RenderOverview() = %v", err)
	}
	text := readOverview(t, bolt)
	if !strings.Contains(text, "\nNote: the effort defaults are left out of "+ClaudeSettingsFile+": ") {
		t.Errorf("overview lacks the left-out note:\n%s", text)
	}
	if strings.Contains(text, "`"+AgentDefaultsSlug+"`") {
		t.Errorf("overview shows %s; want it left out:\n%s", AgentDefaultsSlug, text)
	}
}

// TestRenderOverviewNamesAnOverriddenShippedEntry pins the overview of a shipped entry the hub overrides.
// Its row says so in bold, and its section shows the override's text in full under its own group.
func TestRenderOverviewNamesAnOverriddenShippedEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	shipped := shippedStore.Ordered()[0]
	bolt := t.TempDir()
	writeHeddleConfig(t, bolt, LymOverrideKey+": true\n")
	var overrides Store
	if err := overrides.SetOverride(shipped.Slug, "tuned text"); err != nil {
		t.Fatal(err)
	}
	if err := SaveOverrides(bolt, overrides); err != nil {
		t.Fatal(err)
	}

	if _, err := RenderOverview(bolt); err != nil {
		t.Fatalf("RenderOverview() = %v", err)
	}
	text := readOverview(t, bolt)
	for _, want := range []string{
		"| **lym, overridden** | `" + shipped.Slug + "` | tuned text |\n",
		"\n### From lym, overridden by this hub\n\n#### `" + shipped.Slug + "`\n\n```markdown\ntuned text\n```\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("overview lacks %q:\n%s", want, text)
		}
	}
}
