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

// overviewSection returns the part of text under the heading of target, up to the next target's heading.
func overviewSection(t *testing.T, text, target string) string {
	t.Helper()
	_, rest, found := strings.Cut(text, "\n## `"+target+"`\n")
	if !found {
		t.Fatalf("overview holds no section for %s: %q", target, text)
	}
	section, _, _ := strings.Cut(rest, "\n## `")
	return section
}

// TestRenderOverview pins the overview's sections, source lines and fenced blocks.
// It uses a store holding a text entry and a JSON-object entry.
// The store also holds a text entry with a backtick run and a claude-settings entry whose text is no object.
// It pins the widened fence of the backtick run and the non-object text shown as written.
// It also pins that a target no entry names says so, and that a second call changes nothing.
func TestRenderOverview(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	bolt := t.TempDir()
	saveStore(t, bolt, Store{Entries: []Entry{
		entryFor("house-rules", "Be kind.", 10, "claude-file"),
		entryFor("shared-rules", "Be brief.", 15, "agents-file"),
		entryFor("my-settings", `{"model": "opus"}`, 20, "claude-settings"),
		entryFor("fenced-rules", "Use ```go fences.", 30, "claude-file"),
		entryFor("loose-settings", "not json", 40, "claude-settings"),
	}})

	changed, err := RenderOverview(bolt)
	if err != nil || !changed {
		t.Fatalf("RenderOverview() = %v, %v; want true", changed, err)
	}
	text := readOverview(t, bolt)

	if !strings.HasPrefix(text, "# Heddle\n") {
		t.Errorf("overview opens with %q; want the # Heddle title", text[:min(len(text), 20)])
	}
	claudeFile := overviewSection(t, text, "claude-file")
	for _, want := range []string{
		"\nRendered into `CLAUDE.local.md` at every worktree's anchor. It also carries every entry aimed at `agents-file`.\n",
		"- From: shipped with lym\n",
		"````markdown\nUse ```go fences.\n````\n",
		"### `house-rules`\n\n- From: the hub's heddle.json\n- Order: 10\n- Targets: `claude-file`\n\n```markdown\nBe kind.\n```\n",
	} {
		if !strings.Contains(claudeFile, want) {
			t.Errorf("claude-file section lacks %q: %q", want, claudeFile)
		}
	}
	agentsFile := overviewSection(t, text, "agents-file")
	for _, want := range []string{
		"\nRendered into `CLAUDE.local.md`, `AGENTS.override.md` at every worktree's anchor.\n",
		"### `shared-rules`\n\n- From: the hub's heddle.json\n- Order: 15\n- Targets: `agents-file`\n\n```markdown\nBe brief.\n```\n",
	} {
		if !strings.Contains(agentsFile, want) {
			t.Errorf("agents-file section lacks %q: %q", want, agentsFile)
		}
	}
	settings := overviewSection(t, text, "claude-settings")
	for _, want := range []string{
		"- From: derived from the hub's agents.yaml\n",
		"```json\nnot json\n```\n",
		"### `my-settings`\n\n- From: the hub's heddle.json\n- Order: 20\n- Targets: `claude-settings`\n\n```json\n{\n  \"model\": \"opus\"\n}\n```\n",
	} {
		if !strings.Contains(settings, want) {
			t.Errorf("claude-settings section lacks %q: %q", want, settings)
		}
	}
	if section := overviewSection(t, text, "orch-prompt"); !strings.HasSuffix(section, "\nNo entry.\n") {
		t.Errorf("orch-prompt section = %q; want it to end with No entry.", section)
	}

	if changed, err := RenderOverview(bolt); err != nil || changed {
		t.Errorf("second RenderOverview() = %v, %v; want false", changed, err)
	}
}

// TestRenderOverviewSaysWhenTheEffortDefaultsAreLeftOut pins the overview of a hub whose agents.yaml fails to load.
// The render succeeds, the claude-settings section notes the omission, and no derived entry shows.
func TestRenderOverviewSaysWhenTheEffortDefaultsAreLeftOut(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	bolt := t.TempDir()
	writeAgentsConfig(t, bolt, invalidAgentsConfig)

	if _, err := RenderOverview(bolt); err != nil {
		t.Fatalf("RenderOverview() = %v", err)
	}
	settings := overviewSection(t, readOverview(t, bolt), "claude-settings")
	if !strings.Contains(settings, "\nNote: the effort defaults are left out of "+ClaudeSettingsFile+": ") {
		t.Errorf("claude-settings section lacks the left-out note: %q", settings)
	}
	if strings.Contains(settings, "### `"+AgentDefaultsSlug+"`") {
		t.Errorf("claude-settings section shows %s; want it left out: %q", AgentDefaultsSlug, settings)
	}
}

// TestRenderOverviewNamesAnOverriddenShippedEntry pins the overview of a shipped entry the hub overrides.
// It shows the override's text and says so in its source line.
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
	want := "### `" + shipped.Slug + "`\n\n- From: shipped with lym, its text overridden by the hub\n"
	text := readOverview(t, bolt)
	if !strings.Contains(text, want) || !strings.Contains(text, "```markdown\ntuned text\n```\n") {
		t.Errorf("overview = %q; want %q and the override's text", text, want)
	}
}
