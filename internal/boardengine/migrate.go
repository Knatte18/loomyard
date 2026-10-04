// migrate.go — in-memory conversion of old-shape board entries (tier and type) to kind and labels.
//
// Load decodes board.json, and the legacy tasks.json and notes.json path builds, through storedEntry, so one conversion serves both.
// Conversion never fails and never checks labels against config: card 3's write validation refuses a later write of an entry carrying an unconfigured label.
// Save writes Task, so tier and type drop on the first write after a load.

package boardengine

import (
	"slices"
	"strings"
	"unicode"
)

// storedEntry is the decode shape of one board.json entry: a Task plus the retired tier and type fields.
type storedEntry struct {
	Task
	Tier *int    `json:"tier"`
	Type *string `json:"type"`
}

// legacyTypeNames are the type names the retired type field and bracketed brief prefixes used.
var legacyTypeNames = []string{"feature", "bug", "chore", "design"}

// legacyTypeLabels maps a retired type to the labels it becomes; an absent or unknown type maps to enhancement.
func legacyTypeLabels(typ *string) []string {
	if typ == nil {
		return []string{"enhancement"}
	}
	switch *typ {
	case "bug":
		return []string{"bug"}
	case "design":
		return []string{"enhancement", "undecided"}
	default:
		return []string{"enhancement"}
	}
}

// migrateEntries converts every entry to the kind-and-labels shape.
// An entry with kind is untouched, an entry with tier migrates, and an entry with neither loads as a note with no labels.
// A migrated task's dependency on an entry that migrated to a note is dropped, since a task may depend only on a task.
// vocab may be nil, as in store-level tests.
func migrateEntries(entries []storedEntry, vocab *Vocabulary) []Task {
	out := make([]Task, len(entries))
	migrated := make([]bool, len(entries))
	migratedNotes := map[string]bool{}
	for i, e := range entries {
		out[i] = e.Task
		switch {
		case e.Kind != "":
		case e.Tier == nil:
			out[i].Kind = KindNote
			out[i].Labels = []string{}
		default:
			migrated[i] = true
			out[i] = migrateEntry(e, vocab)
			if out[i].Kind == KindNote {
				migratedNotes[out[i].Slug] = true
			}
		}
	}
	for i := range out {
		if !migrated[i] || out[i].Kind != KindTask {
			continue
		}
		out[i].DependsOn = slices.DeleteFunc(slices.Clone(out[i].DependsOn), func(dep string) bool {
			return migratedNotes[dep]
		})
	}
	return out
}

// migrateEntry converts one entry that has a tier and no kind; dependency rewriting across entries is migrateEntries' job.
func migrateEntry(e storedEntry, vocab *Vocabulary) Task {
	t := e.Task
	t.Kind = KindNote
	if *e.Tier == 1 {
		t.Kind = KindTask
	}

	labels := legacyTypeLabels(e.Type)
	var prefixes []string
	prefixes, t.Brief = splitBriefPrefixes(t.Brief)
	for _, p := range prefixes {
		if slices.Contains(legacyTypeNames, p) || (vocab != nil && vocab.IsType(p)) {
			continue
		}
		labels = append(labels, p)
	}
	t.Labels = dedupe(labels)

	if t.Kind == KindNote && len(t.DependsOn) > 0 {
		t.Body = appendDependsOn(t.Body, t.DependsOn)
		t.DependsOn = []string{}
	}
	return t
}

// splitBriefPrefixes returns the leading `[x]` tokens of brief in order and the brief without them and the whitespace after them.
func splitBriefPrefixes(brief string) ([]string, string) {
	var prefixes []string
	rest := brief
	for strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			break
		}
		label := rest[1:end]
		if label == "" || strings.ContainsFunc(label, func(r rune) bool { return r == '[' || unicode.IsSpace(r) }) {
			break
		}
		prefixes = append(prefixes, label)
		rest = strings.TrimLeftFunc(rest[end+1:], unicode.IsSpace)
	}
	if len(prefixes) == 0 {
		return nil, brief
	}
	return prefixes, rest
}

// dedupe keeps the first occurrence of each label.
func dedupe(labels []string) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// appendDependsOn adds the closing line that carries a migrated note's former dependencies, after a blank line when body is not empty.
func appendDependsOn(body string, deps []string) string {
	quoted := make([]string, len(deps))
	for i, d := range deps {
		quoted[i] = "`" + d + "`"
	}
	line := "Depends on " + strings.Join(quoted, ", ") + "."
	if body == "" {
		return line
	}
	return strings.TrimRight(body, "\n") + "\n\n" + line
}
