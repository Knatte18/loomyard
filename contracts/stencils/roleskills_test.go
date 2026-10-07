// roleskills_test.go pins PATTERN-role-skills-typed: a skill reaches a spawned session because the
// spawning module names it on the launch spec and lyx types it, so no stencil asks an agent to load
// a skill.
// The scan reads each registered stencil's agent-facing body and flags an imperative load verb
// followed by a `<plugin>:<skill>` token in the same sentence, or any mention of the `Skill` tool.
// Naming a skill as a pointer, as the orch role stencil does for `ly:board`, carries no load verb
// and is not a match.

package stencils

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

var (
	// skillTokenPattern matches a `<plugin>:<skill>` token.
	skillTokenPattern = regexp.MustCompile(`\b[a-z][a-z0-9-]*:[a-z][a-z0-9-]*\b`)
	// loadVerbPattern matches an imperative load verb.
	loadVerbPattern = regexp.MustCompile(`(?i)\b(load|invoke)\b`)
	// skillToolPattern matches a mention of the Skill tool.
	skillToolPattern = regexp.MustCompile("(?i)\\bskill tool\\b|`Skill`")
	// sentenceSplitPattern splits a stencil body into sentences.
	sentenceSplitPattern = regexp.MustCompile(`[.!?]\s|\n`)
)

// skillLoadViolations returns one description per sentence of body that instructs an agent to load a skill, and one when body mentions the Skill tool.
func skillLoadViolations(body string) []string {
	var violations []string
	if skillToolPattern.MatchString(body) {
		violations = append(violations, "mentions the Skill tool")
	}
	for _, sentence := range sentenceSplitPattern.Split(body, -1) {
		loc := loadVerbPattern.FindStringIndex(sentence)
		if loc == nil {
			continue
		}
		if skillTokenPattern.MatchString(sentence[loc[1]:]) {
			violations = append(violations, "load instruction: "+strings.TrimSpace(sentence))
		}
	}
	return violations
}

// TestStencils_NoSkillLoadInstructions fails for every registered stencil whose agent-facing body tells an agent to load a skill or names the Skill tool.
//
//testtiming:keep pins PATTERN-role-skills-typed's ban on a stencil telling an agent to load a skill or naming the Skill tool, a guard that fires which no other test asserts
//lyx:guard
func TestStencils_NoSkillLoadInstructions(t *testing.T) {
	t.Parallel()

	reg := Registry()
	for _, name := range reg.Names() {
		def, ok := reg.Default(name)
		if !ok {
			t.Fatalf("Registry().Default(%q) = _, false; want true for a name Registry().Names() returned", name)
		}
		for _, violation := range skillLoadViolations(stencil.StripLeadingComment(string(def))) {
			t.Errorf("stencil %q: %s; a skill is named on the launch spec and typed by lyx, never asked for in a stencil", name, violation)
		}
	}
}

// TestSkillLoadViolations_SyntheticRows proves the scan fails on a stencil that breaks the rule and passes on one that does not.
//
//testtiming:keep proves the skill-load scan fires on a stencil that breaks the rule and stays quiet on a bare pointer, which the scan over the real stencils cannot show while those stencils comply
func TestSkillLoadViolations_SyntheticRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want bool
	}{
		{"load instruction", "First, load the scribe:prose skill and continue.", true},
		{"invoke instruction", "Invoke ly:board before you start.", true},
		{"Skill tool mention", "Use the Skill tool to read it.", true},
		{"bare pointer", "The mechanics are in the `ly:board` skill; this section is policy only.", false},
		{"load verb, skill token later in the sentence", "Load the plan file, then read scribe:prose rules.\nNothing else.", true},
		{"load verb, skill token in the next sentence", "Load the plan file.\nRead scribe:prose rules.", false},
		{"no match", "Read the card and implement it.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := len(skillLoadViolations(tt.body)) > 0
			if got != tt.want {
				t.Errorf("skillLoadViolations(%q) flagged = %v; want %v", tt.body, got, tt.want)
			}
		})
	}
}
