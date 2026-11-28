// overview.go renders HEDDLE.md at the Bolt checkout's root: every entry heddle holds, by the place it lands in, for a reader.

package heddleengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fsx"
)

// OverviewFileName is the file at the Bolt checkout's root that shows every entry heddle holds.
const OverviewFileName = "HEDDLE.md"

// overviewIntro opens the overview under its title and names the sources the overview labels entries with.
const overviewIntro = "What heddle delivers to every worktree and session in this hub.\n" +
	"lym rewrites this file whole at every heddle write; change an entry with `lym heddle set`, never this file.\n" +
	"Two targets are shared: `agents-file` lands in both agent files, and `agents-prompt` in the prompt of every Claude and Codex session.\n" +
	"\n" +
	"Sources:\n" +
	"\n" +
	"- **repo** — this hub's `heddle.json`; yours to change.\n" +
	"- **lym** — shipped with lym; change it only through an override (`lym heddle set --layer " + OverrideLayer + "`).\n" +
	"- **lym, overridden** — shipped with lym, its text replaced by this hub's override.\n" +
	"- **derived** — rendered from this hub's config, such as `agents.yaml`.\n"

// minimumFence is the shortest code fence the overview opens.
const minimumFence = 3

// overviewGroup is one source group of a text place's entries, in the order the overview shows the groups.
// A collapsed group shows each entry's text folded under a one-line summary.
type overviewGroup struct {
	heading   string
	collapsed bool
	holds     func(LayeredEntry) bool
}

// overviewGroups puts what this hub sets before what lym ships, and folds away only lym's own unchanged texts.
var overviewGroups = []overviewGroup{
	{heading: "From this repository", holds: func(entry LayeredEntry) bool { return entry.Layer == LayerRepository }},
	{heading: "From lym, overridden by this hub", holds: func(entry LayeredEntry) bool { return entry.Overridden }},
	{heading: "Derived from this hub's config", holds: func(entry LayeredEntry) bool { return entry.Layer == LayerDerived }},
	{heading: "From lym", collapsed: true, holds: func(entry LayeredEntry) bool { return entry.Layer == LayerShipped && !entry.Overridden }},
}

// overviewPlace is one place entries land in: a file target, a JSON-object target or a prompt target,
// with the entries that reach it in render order.
type overviewPlace struct {
	target  Target
	entries []LayeredEntry
}

// RenderOverview rewrites OverviewFileName in the Bolt checkout boltDir from what heddle holds there now.
// It reports whether the file changed.
// It shows the shipped entries with any override in force.
// It also shows the entries derived from the hub's agents.yaml and heddle.json's entries.
// The derived entries come from the agents.yaml in the Bolt checkout alone, so a machine-layer agents.yaml is not read.
// When agents.yaml does not load, the overview says the effort defaults are left out and why.
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

// overviewText renders the overview of layers.
// A table of every entry by the place it lands in comes first, then a section per place in table order.
// A text or prompt place shows its entries by source group;
// a JSON-object place shows the object its entries fold to, each key marked with the entries that set it.
// A place no entry reaches is left out, and the layers' warnings show as notes under the intro.
func overviewText(layers Layers) string {
	var out strings.Builder
	out.WriteString("# Heddle\n\n" + overviewIntro)
	for _, warning := range []string{layers.OverrideWarning, layers.DerivedWarning} {
		if warning != "" {
			fmt.Fprintf(&out, "\nNote: %s\n", warning)
		}
	}
	places := overviewPlaces(layers.Ordered())
	out.WriteString("\n## At a glance\n\n| Lands in | Source | Entry | What it does |\n|---|---|---|---|\n")
	for _, place := range places {
		for _, entry := range place.entries {
			fmt.Fprintf(&out, "| %s | %s | `%s` | %s |\n", placeTitle(place.target), entrySource(entry), entry.Slug, tableCell(entrySummary(place.target, entry)))
		}
	}
	for _, place := range places {
		fmt.Fprintf(&out, "\n## %s\n\n%s\n", placeTitle(place.target), placeReach(place.target))
		if place.target.JSONObject {
			out.WriteString(objectPlaceText(place))
			continue
		}
		out.WriteString(textPlaceText(place))
	}
	return out.String()
}

// overviewPlaces returns every target that owns a file or a prompt, in table order, with the entries that reach it.
// A shared target is no place of its own: its entries show under each place it renders into.
// A target no entry reaches is left out.
func overviewPlaces(entries []LayeredEntry) []overviewPlace {
	var places []overviewPlace
	for _, target := range targetTable {
		if len(target.RendersInto) > 0 {
			continue
		}
		place := overviewPlace{target: target}
		for _, entry := range entries {
			if reachesFileTarget(target.Name, entry.Targets) {
				place.entries = append(place.entries, entry)
			}
		}
		if len(place.entries) > 0 {
			places = append(places, place)
		}
	}
	return places
}

// placeTitle names where target's entries land: its file, or the sessions whose system prompt it reaches.
func placeTitle(target Target) string {
	if target.Delivery == DeliveryPrompt {
		return "System prompt of " + promptReach(target)
	}
	return "`" + target.File + "`"
}

// promptReach names the sessions the prompt target reaches, such as "every orch session".
func promptReach(target Target) string {
	reached := "every"
	if target.Engine != "" {
		reached += " " + target.Engine
	}
	if target.Role != "" {
		reached += " " + target.Role
	}
	return reached + " session"
}

// placeReach says how target delivers its entries.
func placeReach(target Target) string {
	switch {
	case target.Delivery == DeliveryPrompt:
		return "Appended to the system prompt of " + promptReach(target) + " at its launch."
	case target.JSONObject:
		return "Merged into the file at every worktree's anchor; a key the file holds that heddle never set stays.\n" +
			"What heddle merges in, with the entry that sets each key:"
	}
	return "Rendered at every worktree's anchor."
}

// entrySource labels where entry comes from, in bold when this hub sets its text.
func entrySource(entry LayeredEntry) string {
	switch {
	case entry.Layer == LayerRepository, entry.Overridden:
		return "**" + plainSource(entry) + "**"
	}
	return plainSource(entry)
}

// plainSource labels where entry comes from, with no markup.
func plainSource(entry LayeredEntry) string {
	switch {
	case entry.Layer == LayerDerived:
		return "derived"
	case entry.Layer == LayerRepository:
		return "repo"
	case entry.Overridden:
		return "lym, overridden"
	}
	return "lym"
}

// entrySummary returns one line saying what entry does at target.
// A JSON-object target's entry is summed up by the keys it sets.
// A text entry is summed up by its first heading, or its first line when it has no heading.
func entrySummary(target Target, entry LayeredEntry) string {
	if target.JSONObject {
		object, ok := decodeObject(entry.Text)
		if !ok {
			return "holds no JSON object"
		}
		return codeList(slices.Sorted(maps.Keys(object)))
	}
	var first string
	for line := range strings.Lines(entry.Text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
		if first == "" {
			first = line
		}
	}
	return first
}

// tableCell makes text safe inside a markdown table cell.
func tableCell(text string) string {
	return strings.ReplaceAll(text, "|", `\|`)
}

// textPlaceText renders the entries of a text or prompt place by source group.
// An entry of an open group shows its whole text under its slug;
// one of a collapsed group shows it folded under its slug and summary.
func textPlaceText(place overviewPlace) string {
	var out strings.Builder
	for _, group := range overviewGroups {
		var held []LayeredEntry
		for _, entry := range place.entries {
			if group.holds(entry) {
				held = append(held, entry)
			}
		}
		if len(held) == 0 {
			continue
		}
		fmt.Fprintf(&out, "\n### %s\n", group.heading)
		for _, entry := range held {
			block := fenced("markdown", strings.Trim(entry.Text, "\n"))
			if group.collapsed {
				fmt.Fprintf(&out, "\n<details>\n<summary><code>%s</code> — %s</summary>\n\n%s\n</details>\n",
					html.EscapeString(entry.Slug), html.EscapeString(entrySummary(place.target, entry)), block)
				continue
			}
			fmt.Fprintf(&out, "\n#### `%s`\n\n%s", entry.Slug, block)
		}
	}
	return out.String()
}

// objectPlaceText renders the object a JSON-object place's entries fold to, as a jsonc block.
// Each run of top-level keys set by the same entries opens with a comment naming those entries and their sources.
// Keys come in the order their first entry sets them, an entry's own keys sorted.
// An entry whose text holds no JSON object is left out of the object and named in a note.
func objectPlaceText(place overviewPlace) string {
	folded, _, _, warnings := foldObjects(place.target, place.entries)
	var keys []string
	setters := map[string][]string{}
	for _, entry := range place.entries {
		object, ok := decodeObject(entry.Text)
		if !ok {
			continue
		}
		for _, key := range slices.Sorted(maps.Keys(object)) {
			if _, seen := setters[key]; !seen {
				keys = append(keys, key)
			}
			setters[key] = append(setters[key], entry.Slug+" ("+plainSource(entry)+")")
		}
	}
	var body strings.Builder
	body.WriteString("{\n")
	var lastComment string
	for i, key := range keys {
		comment := "// " + strings.Join(setters[key], ", ")
		if comment != lastComment {
			if i > 0 {
				body.WriteString("\n")
			}
			body.WriteString("  " + comment + "\n")
			lastComment = comment
		}
		separator := ","
		if i == len(keys)-1 {
			separator = ""
		}
		fmt.Fprintf(&body, "  %s: %s%s\n", encodeMember(key), encodeMember(folded[key]), separator)
	}
	body.WriteString("}")
	var out strings.Builder
	for _, warning := range warnings {
		fmt.Fprintf(&out, "\nNote: %s\n", warning)
	}
	out.WriteString("\n" + fenced("jsonc", body.String()))
	return out.String()
}

// encodeMember encodes value as a member of a top-level object indented by two spaces, with no HTML escaping.
func encodeMember(value any) string {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("  ", "  ")
	if err := encoder.Encode(value); err != nil {
		panic(fmt.Sprintf("heddleengine: a decoded JSON value did not encode: %v", err))
	}
	return strings.TrimSuffix(out.String(), "\n")
}

// codeList renders names as a comma-separated list of code spans.
func codeList(names []string) string {
	spans := make([]string, len(names))
	for i, name := range names {
		spans[i] = "`" + name + "`"
	}
	return strings.Join(spans, ", ")
}

// fenced renders body as a fenced block in language, the fence one backtick longer than body's longest backtick run.
func fenced(language, body string) string {
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
