//go:build tier0

// removedverbs_test.go pins that no production source, stencil or skill names a verb lym no longer has,
// so every way forward names the verb that replaced it.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// removedVerbs are the commands that no longer exist; each is named by `lym beam start`, `lym adopt <beam>`, `lym adopt root`, `lym creel post` or `lym creel interrupt` now.
var removedVerbs = []string{
	"lym orch start",
	"lym orch cleanup",
	"lym prime",
	"lym beam promote",
	"lym adopt --prime",
	"lym shuttle send",
	"lym shuttle interrupt",
}

// removedVerbHits returns one "line N: verb" entry per line of content that names a removed verb.
func removedVerbHits(content string) []string {
	var hits []string
	for i, line := range strings.Split(content, "\n") {
		for _, verb := range removedVerbs {
			if strings.Contains(line, verb) {
				hits = append(hits, fmt.Sprintf("line %d: %s", i+1, verb))
			}
		}
	}
	return hits
}

func TestRemovedVerbs_NamedNowhere(t *testing.T) {
	t.Parallel()

	t.Run("FixtureRows", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			content string
			want    []string
		}{
			{"PlantedLine", "ok\nrun `lym orch start` again\n", []string{"line 2: lym orch start"}},
			{"EveryRemovedVerb", "lym orch cleanup\nlym prime\nlym beam promote\nlym adopt --prime", []string{
				"line 1: lym orch cleanup", "line 2: lym prime", "line 3: lym beam promote", "line 4: lym adopt --prime",
			}},
			{"CleanTree", "run `lym beam start` or `lym adopt root`\n", nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := removedVerbHits(tt.content)
				if strings.Join(got, "|") != strings.Join(tt.want, "|") {
					t.Errorf("removedVerbHits() = %q; want %q", got, tt.want)
				}
			})
		}
	})

	t.Run("Tree", func(t *testing.T) {
		t.Parallel()
		var failures []string
		scan := func(f *scankit.File) {
			for _, hit := range removedVerbHits(string(f.Data)) {
				failures = append(failures, fmt.Sprintf("%s: %s", f.Rel, hit))
			}
		}

		scanned := scankit.Walk(t, scankit.Options{}, scan)
		scanned += scankit.Walk(t, scankit.Options{
			Roots:  []string{"contracts/stencils", "plugins/ly/skills"},
			Filter: scankit.All,
			Exts:   []string{".md"},
		}, scan)

		scankit.RequireFloor(t, scanned, 20, "removed verbs")
		if len(failures) > 0 {
			t.Errorf("a removed verb is named; name the verb that replaced it:\n%s", strings.Join(failures, "\n"))
		}
	})
}
