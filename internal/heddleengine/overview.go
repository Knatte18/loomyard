// overview.go renders HEDDLE.md at the Bolt checkout's root: every entry heddle holds, grouped by target, for a reader.

package heddleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fsx"
)

// OverviewFileName is the file at the Bolt checkout's root that shows every entry heddle holds.
const OverviewFileName = "HEDDLE.md"

// overviewIntro opens the overview under its title.
const overviewIntro = "Every entry heddle delivers, grouped by the target it reaches, each group in render order.\n" +
	"lym rewrites this file whole at every heddle write; change an entry with `lym heddle set`, never this file.\n"

// minimumFence is the shortest code fence the overview opens.
const minimumFence = 3

// RenderOverview rewrites OverviewFileName in the Bolt checkout boltDir from what heddle holds there now.
// It reports whether the file changed.
// It shows the shipped entries with any override in force.
// It also shows the entries derived from the hub's agents.yaml and heddle.json's entries.
// The derived entries come from the agents.yaml in the Bolt checkout alone, so a machine-layer agents.yaml is not read.
// When agents.yaml does not load, the claude-settings section says the effort defaults are left out and why.
// It takes no lock; the caller holds the Bolt write lock.
func RenderOverview(boltDir string) (bool, error) {
	layers, err := ReadLayers(boltDir)
	if err != nil {
		return false, err
	}
	layers.addDerived(configengine.Dirs{Board: boltDir})
	text := overviewText(layers)
	current, err := os.ReadFile(filepath.Join(boltDir, OverviewFileName))
	switch {
	case err == nil && string(current) == text:
		return false, nil
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return false, fmt.Errorf("read %s: %w", OverviewFileName, err)
	}
	if err := fsx.AtomicWrite(boltDir, OverviewFileName, text); err != nil {
		return false, fmt.Errorf("write %s: %w", OverviewFileName, err)
	}
	return true, nil
}

// overviewText renders the overview of layers: a section per target in table order.
// Each entry aimed at the target sits under it in render order with its source, order, targets and text.
// A target no entry names says so, and the claude-settings section carries the layers' derived warning when one is set.
func overviewText(layers Layers) string {
	entries := layers.Ordered()
	var out strings.Builder
	out.WriteString("# Heddle\n\n" + overviewIntro)
	for _, target := range targetTable {
		fmt.Fprintf(&out, "\n## `%s`\n\n%s\n", target.Name, targetReach(target))
		if target.Name == claudeSettingsTarget && layers.DerivedWarning != "" {
			fmt.Fprintf(&out, "\nNote: %s\n", layers.DerivedWarning)
		}
		held := false
		for _, entry := range entries {
			if !slices.Contains(entry.Targets, target.Name) {
				continue
			}
			held = true
			fmt.Fprintf(&out, "\n### `%s`\n\n- From: %s\n- Order: %d\n- Targets: %s\n\n", entry.Slug, entrySource(entry), entry.Order, codeList(entry.Targets))
			out.WriteString(entryBlock(target, entry.Text))
		}
		if !held {
			out.WriteString("\nNo entry.\n")
		}
	}
	return out.String()
}

// targetReach describes where target delivers its entries.
func targetReach(target Target) string {
	if target.Delivery == DeliveryPrompt {
		reached := "every"
		if target.Engine != "" {
			reached += " " + target.Engine
		}
		if target.Role != "" {
			reached += " " + target.Role
		}
		return "Appended to the system prompt of " + reached + " session."
	}
	if target.JSONObject {
		return fmt.Sprintf("Merged into `%s` at every worktree's anchor; a key the file holds that heddle never set stays.", target.File)
	}
	reach := fmt.Sprintf("Rendered into %s at every worktree's anchor.", codeList(TargetFiles(target.Name)))
	for _, shared := range targetTable {
		if slices.Contains(shared.RendersInto, target.Name) {
			reach += fmt.Sprintf(" It also carries every entry aimed at `%s`.", shared.Name)
		}
	}
	return reach
}

// entrySource names where entry comes from.
func entrySource(entry LayeredEntry) string {
	switch {
	case entry.Layer == LayerDerived:
		return "derived from the hub's agents.yaml"
	case entry.Layer == LayerRepository:
		return "the hub's heddle.json"
	case entry.Overridden:
		return "shipped with lym, its text overridden by the hub"
	}
	return "shipped with lym"
}

// codeList renders names as a comma-separated list of code spans.
func codeList(names []string) string {
	spans := make([]string, len(names))
	for i, name := range names {
		spans[i] = "`" + name + "`"
	}
	return strings.Join(spans, ", ")
}

// entryBlock renders an entry's text as a fenced block:
// a JSON-object target's object indented as json, any other text as markdown.
// A JSON-object entry whose text holds no object shows its text as it is.
func entryBlock(target Target, text string) string {
	language := "markdown"
	if target.JSONObject {
		language = "json"
		if object, ok := decodeObject(text); ok {
			if encoded, err := encodeObject(object); err == nil {
				text = encoded
			}
		}
	}
	body := strings.Trim(text, "\n")
	fence := strings.Repeat("`", max(minimumFence, longestBacktickRun(body)+1))
	return fence + language + "\n" + body + "\n" + fence + "\n"
}

// longestBacktickRun returns the length of the longest run of backticks in text,
// so a fence one longer can enclose it.
func longestBacktickRun(text string) int {
	longest, run := 0, 0
	for _, r := range text {
		if r != '`' {
			run = 0
			continue
		}
		run++
		longest = max(longest, run)
	}
	return longest
}
