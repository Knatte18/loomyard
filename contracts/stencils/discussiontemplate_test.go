// discussiontemplate_test.go pins loom-template-discussion.md's write fence: the set of paths the
// Discussion producer is forbidden to touch.
// The fence exists because this producer is the one agent loom spawns with an unrestricted,
// permission-bypassed shell and no scope restriction of its own -- fix-scope: overlay confines both
// overlay Burler rows to their Target.Paths and forbids them git entirely, and the Webster fork
// reviewers are read-only, but the Discussion writer had nothing. Observed live: it rewrote
// _lyx/config/loom.yaml mid-run, flipping discussion_interactive to true, which would have made the
// next unattended run of that worktree sit waiting for an operator who was not there.

package stencils

import (
	"strings"
	"testing"
)

// TestLoomTemplateDiscussion_FencesWhatItMayWrite asserts the template names every path class the
// producer must leave alone, plus the do-not-repair rule that keeps it from working around a broken
// environment instead of reporting it.
// Each assertion is a short, distinctive substring rather than a whole paragraph, following
// rubric_test.go's own precedent, so ordinary prose edits do not break this test.
func TestLoomTemplateDiscussion_FencesWhatItMayWrite(t *testing.T) {
	text := string(LoomTemplateDiscussion)

	tests := []struct {
		name   string
		phrase string
	}{
		{"declares the read-only default", "read-only to you"},
		{"fences the driver's own config", "`_lyx/config/`"},
		{"names reconcile --apply as an edit", "lyx config reconcile --apply"},
		{"fences the phase machine's status file", "`_lyx/shed/<slug>/`"},
		{"fences a later phase's artifact", "`_lyx/plan/`"},
		{"fences mutating git", "no `git add`"},
		{"forbids repairing a broken environment", "do not repair it"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(text, tt.phrase) {
				t.Errorf("LoomTemplateDiscussion does not contain %q", tt.phrase)
			}
		})
	}
}

// TestLoomTemplateDiscussion_FenceNamesTheTwoOutputMarkers asserts the fence states the two files the
// producer MAY write by their marker rather than by a hardcoded path, so the permitted set can never
// drift from the paths Step 5 actually tells it to write.
func TestLoomTemplateDiscussion_FenceNamesTheTwoOutputMarkers(t *testing.T) {
	text := string(LoomTemplateDiscussion)
	fenceStart := strings.Index(text, "## What you may write")
	if fenceStart == -1 {
		t.Fatal("LoomTemplateDiscussion has no \"## What you may write\" section")
	}
	fence := text[fenceStart:]

	for _, marker := range []string{"{{.decision_record_path}}", "{{.support_log_path}}"} {
		if !strings.Contains(fence, marker) {
			t.Errorf("the write fence does not name %s; a hardcoded path there would drift from Step 5", marker)
		}
	}
}

// TestLoomTemplateDiscussion_NamesBoardListAndParentReviewCarveOuts asserts Step 1 reads the wider board
// and the write fence carries both parent-review carve-outs.
func TestLoomTemplateDiscussion_NamesBoardListAndParentReviewCarveOuts(t *testing.T) {
	text := string(LoomTemplateDiscussion)
	fenceStart := strings.Index(text, "## What you may write")
	if fenceStart == -1 {
		t.Fatal("LoomTemplateDiscussion has no \"## What you may write\" section")
	}
	fence := text[fenceStart:]

	if !strings.Contains(text, "lyx board list") {
		t.Errorf("LoomTemplateDiscussion does not name %q", "lyx board list")
	}
	for _, phrase := range []string{"lyx loom review delivered", "parent-review entry to `## Review rounds`"} {
		if !strings.Contains(fence, phrase) {
			t.Errorf("the write fence does not carve out %q", phrase)
		}
	}
}

// TestLoomTemplateDiscussion_StatesAttackSurfaceBound asserts the template tells the discussion writer to state, in the introducing Decision, what a new edge or weakened guard can skip or let through and what bounds it.
func TestLoomTemplateDiscussion_StatesAttackSurfaceBound(t *testing.T) {
	text := string(LoomTemplateDiscussion)
	for _, phrase := range []string{"skip or let through", "what bounds it"} {
		if !strings.Contains(text, phrase) {
			t.Errorf("LoomTemplateDiscussion does not contain %q", phrase)
		}
	}
}
