// parentdirective_test.go pins PATTERN-parent-directive: every spawned role's opening stencil
// renders the parent directive, and no stencil outside the orch family tells an agent to ask the
// operator in its pane.
// It also pins PATTERN-edit-directive: every opening stencil renders the edit directive, and no other stencil restates the rule.
// The scans read stencil text only; the discussion role's interactive questions come from the
// `{{.mode_rules}}` marker, whose text is Go, never from a stencil.

package stencils

import (
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// carriesMarker reports whether body holds marker itself or through a registered stencil it includes.
func carriesMarker(body string, bodies map[string]string, marker string) bool {
	if strings.Contains(body, marker) {
		return true
	}
	included, err := stencil.IncludeNames([]byte(body))
	if err != nil {
		return false
	}
	for _, name := range included {
		if strings.Contains(bodies[name], marker) {
			return true
		}
	}
	return false
}

// openingStencilViolations returns one description per mapped stencil that is not registered or whose body lacks marker, itself or through a registered stencil it includes.
// bodies maps a registered stencil name to its agent-facing body.
func openingStencilViolations(opening map[string][]string, bodies map[string]string, marker string) []string {
	var violations []string
	for role, names := range opening {
		for _, name := range names {
			body, registered := bodies[name]
			if !registered {
				violations = append(violations, "role "+role+": stencil "+name+" is not registered")
				continue
			}
			if !carriesMarker(body, bodies, marker) {
				violations = append(violations, "role "+role+": stencil "+name+" lacks the "+marker+" marker")
			}
		}
	}
	sort.Strings(violations)
	return violations
}

// parentMarker and editMarker are the template markers every role's opening stencil carries.
var (
	parentMarker = "{{." + parentdirective.MarkerName + "}}"
	editMarker   = "{{." + editdirective.MarkerName + "}}"
)

// editRuleSentences returns each sentence of body that contains an editRulePhrases entry.
func editRuleSentences(body string) []string {
	var found []string
	for _, sentence := range sentenceSplitPattern.Split(body, -1) {
		if containsAny(strings.ToLower(sentence), editRulePhrases) {
			found = append(found, strings.TrimSpace(sentence))
		}
	}
	return found
}

// unboundedAskSentences returns each sentence of body that matches an askOperatorPhrases entry and holds no askOperatorBounds word.
func unboundedAskSentences(body string) []string {
	var found []string
	for _, sentence := range sentenceSplitPattern.Split(body, -1) {
		lower := strings.ToLower(sentence)
		if !containsAny(lower, askOperatorPhrases) || containsAny(lower, askOperatorBounds) {
			continue
		}
		found = append(found, strings.TrimSpace(sentence))
	}
	return found
}

// containsAny reports whether text contains any of words.
func containsAny(text string, words []string) bool {
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

// registeredBodies returns every registered stencil's agent-facing body keyed by name.
func registeredBodies(t *testing.T) map[string]string {
	t.Helper()

	reg := Registry()
	bodies := make(map[string]string)
	for _, name := range reg.Names() {
		def, ok := reg.Default(name)
		if !ok {
			t.Fatalf("Registry().Default(%q) = _, false; want true for a name Registry().Names() returned", name)
		}
		bodies[name] = stencil.StripLeadingComment(string(def))
	}
	return bodies
}

// TestStencils_OpeningStencilsRenderParentDirective fails when a role's opening stencil is unregistered or lacks the parent directive marker.
//
//testtiming:keep pins PATTERN-parent-directive's marker on every role's opening stencil, a guard that fires which no other test asserts
//lyx:guard
func TestStencils_OpeningStencilsRenderParentDirective(t *testing.T) {
	t.Parallel()

	for _, violation := range openingStencilViolations(roleOpeningStencils, registeredBodies(t), parentMarker) {
		t.Error(violation)
	}
}

// TestStencils_OpeningStencilsRenderEditDirective fails when a role's opening stencil is unregistered or lacks the edit directive marker.
//
//testtiming:keep pins PATTERN-edit-directive's marker on every role's opening stencil, a guard that fires which no other test asserts
//lyx:guard
func TestStencils_OpeningStencilsRenderEditDirective(t *testing.T) {
	t.Parallel()

	for _, violation := range openingStencilViolations(roleOpeningStencils, registeredBodies(t), editMarker) {
		t.Error(violation)
	}
}

// TestStencils_NoEditRuleRestatement fails for any registered stencil other than edit-directive with a sentence that restates the no-script edit rule.
//
//testtiming:keep pins PATTERN-edit-directive's rule that the edit-directive stencil is the only statement of the edit rule, a guard that fires which no other test asserts
//lyx:guard
func TestStencils_NoEditRuleRestatement(t *testing.T) {
	t.Parallel()

	for name, body := range registeredBodies(t) {
		if name == "edit-directive" {
			continue
		}
		for _, sentence := range editRuleSentences(body) {
			t.Errorf("stencil %q: %q restates the edit rule; only the edit-directive stencil states it", name, sentence)
		}
	}
}

// TestStencils_NoUnboundedAskOperator fails for any registered stencil outside the orch family with an ask-the-operator sentence that carries no prohibition word.
//
//testtiming:keep pins PATTERN-parent-directive's ban on an unbounded ask-the-operator sentence in a spawned role's stencil, a guard that fires which no other test asserts
//lyx:guard
func TestStencils_NoUnboundedAskOperator(t *testing.T) {
	t.Parallel()

	for name, body := range registeredBodies(t) {
		if strings.HasPrefix(name, "orch-") {
			continue
		}
		for _, sentence := range unboundedAskSentences(body) {
			t.Errorf("stencil %q: %q tells the agent to ask the operator; a spawned role escalates to its parent", name, sentence)
		}
	}
}

// TestParentDirectiveScans_SyntheticRows proves each scan fails on a stencil that breaks its rule and passes on one that does not.
//
//testtiming:keep proves each parent-directive scan fires on a stencil that breaks its rule, which the scans over the real stencils cannot show while those stencils comply
func TestParentDirectiveScans_SyntheticRows(t *testing.T) {
	t.Parallel()

	t.Run("opening stencil without the marker", func(t *testing.T) {
		t.Parallel()

		bodies := map[string]string{"with": "# T\n{{.parent_directive}}\nbody", "without": "# T\nbody"}
		got := openingStencilViolations(map[string][]string{"a": {"with"}, "b": {"without"}}, bodies, parentMarker)
		if len(got) != 1 || !strings.Contains(got[0], "without") {
			t.Errorf("openingStencilViolations = %v; want one violation naming %q", got, "without")
		}
	})

	t.Run("opening stencil without the edit marker", func(t *testing.T) {
		t.Parallel()

		bodies := map[string]string{"with": "# T\n{{.edit_directive}}\nbody", "without": "# T\n{{.parent_directive}}\nbody"}
		got := openingStencilViolations(map[string][]string{"a": {"with"}, "b": {"without"}}, bodies, editMarker)
		if len(got) != 1 || !strings.Contains(got[0], "without") {
			t.Errorf("openingStencilViolations = %v; want one violation naming %q", got, "without")
		}
	})

	t.Run("opening stencil carrying the marker only through an include", func(t *testing.T) {
		t.Parallel()

		bodies := map[string]string{
			"outer":  "# T\n{{template \"inner\"}}\nbody",
			"inner":  "# I\n{{.parent_directive}}\nbody",
			"bare":   "# T\n{{template \"hollow\"}}\nbody",
			"hollow": "# H\nbody",
		}
		got := openingStencilViolations(map[string][]string{"a": {"outer"}, "b": {"bare"}}, bodies, parentMarker)
		if len(got) != 1 || !strings.Contains(got[0], "bare") {
			t.Errorf("openingStencilViolations = %v; want one violation naming %q", got, "bare")
		}
	})

	t.Run("body restating the edit rule", func(t *testing.T) {
		t.Parallel()

		got := editRuleSentences("Fix it. If the change is large, rewrite it with a heredoc.")
		if len(got) != 1 || !strings.Contains(got[0], "heredoc") {
			t.Errorf("editRuleSentences = %v; want one sentence naming %q", got, "heredoc")
		}
	})

	t.Run("body without the edit rule phrases", func(t *testing.T) {
		t.Parallel()

		got := editRuleSentences("Read the plan. Write the report.")
		if len(got) != 0 {
			t.Errorf("editRuleSentences = %v; want none", got)
		}
	})

	t.Run("opening stencil not registered", func(t *testing.T) {
		t.Parallel()

		got := openingStencilViolations(map[string][]string{"a": {"missing"}}, map[string]string{}, parentMarker)
		if len(got) != 1 || !strings.Contains(got[0], "not registered") {
			t.Errorf("openingStencilViolations = %v; want one not-registered violation", got)
		}
	})

	t.Run("unbounded ask the operator", func(t *testing.T) {
		t.Parallel()

		got := unboundedAskSentences("When unsure, ask the operator what to do.")
		if len(got) != 1 {
			t.Errorf("unboundedAskSentences = %v; want one sentence", got)
		}
	})

	t.Run("bounded ask the operator", func(t *testing.T) {
		t.Parallel()

		got := unboundedAskSentences("Never ask the operator; ask your parent.")
		if len(got) != 0 {
			t.Errorf("unboundedAskSentences = %v; want none", got)
		}
	})
}
