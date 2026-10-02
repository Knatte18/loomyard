// render.go — turns the entry list into the wiki's output files.
//
// Render is a pure function: entries in, a map of filename → content out (a single README.md built
// by renderTasksSection, plus design-*.md for any entry with a body).
// The README reads like manifest/roadmap.md: one section per tier (Planned, Next Up, Someday),
// then Done, each entry one numbered item.
// The tier names and their meaning lines are declared here alone; the data holds only the tier number.
// No I/O — the caller writes the files.
// The design files are built by renderDesigns; each opens with a header naming the entry's slug,
// tier section, type and status, then the stored body unchanged.
// RenderToDisk drives the write path and maintains a manifest sidecar (.board-rendered.json) so
// that renamed or removed outputs are cleaned up on the next render.

package boardengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
)

// renderManifestFile is the name of the sidecar that records the filenames the
// last render produced. It is gitignored (via ensureLockfilesIgnored) so it never
// adds commit churn, and it is never itself a member of the rendered file set.
const renderManifestFile = ".board-rendered.json"

// RenderToDisk renders the entries, persisting the board's output files.
func RenderToDisk(boardPath string, tasks []Task, out Outputs) error {
	files, err := Render(tasks, out)
	if err != nil {
		return err
	}
	for relPath, content := range files {
		if err := fsx.AtomicWrite(boardPath, relPath, content); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
	}

	previous := readRenderManifest(boardPath)
	for _, name := range previous {
		if _, kept := files[name]; !kept {
			// Best-effort removal: a stale file left behind is harmless and cleaned up
			// on the next render, mirroring the old removeOrphanProposals contract.
			os.Remove(filepath.Join(boardPath, name))
		}
	}

	writeRenderManifest(boardPath, files)
	return nil
}

// readRenderManifest returns the list of filenames from the previous render's manifest.
func readRenderManifest(boardPath string) []string {
	data, err := os.ReadFile(filepath.Join(boardPath, renderManifestFile))
	if err != nil {
		return nil
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil
	}
	return names
}

// writeRenderManifest persists the current render's file set to the manifest sidecar.
func writeRenderManifest(boardPath string, files map[string]string) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	data, err := json.Marshal(names)
	if err != nil {
		return
	}
	_ = fsx.AtomicWriteBytes(filepath.Join(boardPath, renderManifestFile), data)
}

// Render produces the board output files from the entry list.
func Render(tasks []Task, out Outputs) (map[string]string, error) {
	ordered, err := RenderOrder(tasks)
	if err != nil {
		return nil, err
	}

	result := map[string]string{
		out.Readme: renderTasksSection(ordered, out.DesignPrefix),
	}

	for name, content := range renderDesigns(tasks, out.DesignPrefix) {
		result[name] = content
	}
	return result, nil
}

// readmeSection is one README section: its heading name and the one-line meaning under it.
type readmeSection struct {
	name    string
	meaning string
}

// tierSections maps a tier number to its README section; this is the only place tier numbers become words.
var tierSections = map[int]readmeSection{
	1: {"Planned", "Concretized and claimable."},
	2: {"Next Up", "Planned next, but not yet concretized."},
	3: {"Someday", "Loose ideas."},
}

// doneSection is the README section holding every done entry, whatever its tier.
var doneSection = readmeSection{"Done", "Finished, awaiting `lyx board prune`."}

// tierName is the section name of a tier, or "tier N" for a tier outside MinTier..MaxTier.
func tierName(tier int) string {
	if s, ok := tierSections[tier]; ok {
		return s.name
	}
	return fmt.Sprintf("tier %d", tier)
}

// metaLine is `slug` · [middle ·] type[ · status], shared by the README item and the design doc header.
func metaLine(t Task, middle ...string) string {
	parts := append([]string{"`" + t.Slug + "`"}, middle...)
	parts = append(parts, t.Type)
	if t.Status != nil {
		parts = append(parts, *t.Status)
	}
	return strings.Join(parts, " · ")
}

// renderTasksSection builds the README: a title, an intro, one section per tier, then Done when any entry is done.
func renderTasksSection(ordered []TaskWithLayer, designPrefix string) string {
	lines := []string{
		"# Board",
		"",
		"Entries grouped by tier, in dependency order within each tier.",
		"",
	}

	writeSection := func(sec readmeSection, entries []TaskWithLayer) {
		lines = append(lines, "## "+sec.name, "", sec.meaning, "")
		for _, twl := range entries {
			lines = append(lines, renderEntry(twl.Task, designPrefix)...)
		}
		if len(entries) > 0 {
			lines = append(lines, "")
		}
	}

	for tier := MinTier; tier <= MaxTier; tier++ {
		var entries []TaskWithLayer
		for _, twl := range ordered {
			if !isDone(twl.Task) && twl.Tier == tier {
				entries = append(entries, twl)
			}
		}
		writeSection(tierSections[tier], entries)
	}

	var done []TaskWithLayer
	for _, twl := range ordered {
		if isDone(twl.Task) {
			done = append(done, twl)
		}
	}
	if len(done) > 0 {
		writeSection(doneSection, done)
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// renderEntry builds the lines of one numbered README item.
func renderEntry(t Task, designPrefix string) []string {
	lines := []string{fmt.Sprintf("1. **%s** — %s", t.Title, metaLine(t))}

	detail := t.Brief
	if t.Body != "" {
		if detail != "" {
			detail += " "
		}
		detail += fmt.Sprintf("[design](%s%s.md)", designPrefix, t.Slug)
	}
	if detail != "" {
		lines = append(lines, "   "+detail)
	}

	if len(t.DependsOn) > 0 {
		deps := make([]string, len(t.DependsOn))
		for i, d := range t.DependsOn {
			deps[i] = "`" + d + "`"
		}
		lines = append(lines, "   After "+strings.Join(deps, ", ")+".")
	}
	return lines
}

// renderDesigns returns one design-doc file entry per entry with a non-empty body,
// using the configured design prefix.
// The doc is a header (title, metadata line, dependencies) followed by the body byte-for-byte.
func renderDesigns(entries []Task, designPrefix string) map[string]string {
	hasBody := make(map[string]bool, len(entries))
	for _, e := range entries {
		hasBody[e.Slug] = e.Body != ""
	}

	designs := make(map[string]string)
	for _, e := range entries {
		if e.Body == "" {
			continue
		}
		header := []string{"# " + e.Title, "", metaLine(e, tierName(e.Tier)), ""}
		if len(e.DependsOn) > 0 {
			deps := make([]string, len(e.DependsOn))
			for i, d := range e.DependsOn {
				if hasBody[d] {
					deps[i] = fmt.Sprintf("[`%s`](%s%s.md)", d, designPrefix, d)
				} else {
					deps[i] = "`" + d + "`"
				}
			}
			header = append(header, "Depends on: "+strings.Join(deps, ", "), "")
		}
		designs[fmt.Sprintf("%s%s.md", designPrefix, e.Slug)] = strings.Join(header, "\n") + "\n" + e.Body
	}
	return designs
}

