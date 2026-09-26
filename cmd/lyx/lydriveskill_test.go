// lydriveskill_test.go pins the ly-drive skill's recipe-blindness. It replaces the retired step-cap
// pin: the design's claim is that which recipe runs is a property of the seed alone, so the skill
// must name no shipped recipe, no `.lyx/` path and no recipe-owned command. The repo root is resolved
// through runtime.Caller, as sandbox_coverage_test.go does, because no package under the plugins tree
// compiles Go.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestLyDriveSkill_IsRecipeBlind fails for every shipped recipe name appearing as a whole word
// (case-insensitively, so "loomyard" does not match) and for every forbidden literal, naming the
// offending token and its line number.
func TestLyDriveSkill_IsRecipeBlind(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location via runtime.Caller")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(testFile)))

	skillPath := filepath.Join(repoRoot, "plugins", "ly", "skills", "ly-drive", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("could not read %s: %v", skillPath, err)
	}

	type forbiddenToken struct {
		label   string
		matches func(line string) bool
	}
	var tokens []forbiddenToken
	for _, name := range shedrun.RecipeNames() {
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(name) + `\b`)
		tokens = append(tokens, forbiddenToken{label: "recipe name " + name, matches: pattern.MatchString})
	}
	for _, literal := range []string{".lyx/", "lyx loom", "lyx batten"} {
		tokens = append(tokens, forbiddenToken{
			label:   "literal " + literal,
			matches: func(line string) bool { return strings.Contains(line, literal) },
		})
	}

	for index, line := range strings.Split(string(data), "\n") {
		for _, token := range tokens {
			if token.matches(line) {
				t.Errorf("%s:%d names %s; the skill must stay recipe-blind: %q", skillPath, index+1, token.label, line)
			}
		}
	}
}
