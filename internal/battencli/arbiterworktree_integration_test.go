// arbiterworktree_integration_test.go pins which worktree batten's arbiter edits in and which parent its agents are told.
// It is a white-box "package battencli" test on a hubforge hub, since the wiring it covers is unexported.

package battencli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedarbiter"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLoadedDriverAgentDeps_NameTheRunsParentOrch covers the parent name a member's tackler and arbiter prompts are told.
func TestLoadedDriverAgentDeps_NameTheRunsParentOrch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "m", fabricengine.AddOptions{SkipPush: true, Parent: "b"})
	c := &battenCLI{location: h.Location, slug: "m"}

	tackler, err := c.loadedTacklerDeps()
	if err != nil {
		t.Fatalf("loadedTacklerDeps() = _, %v; want nil error", err)
	}
	arbiter, err := c.loadedArbiterDeps()
	if err != nil {
		t.Fatalf("loadedArbiterDeps() = _, %v; want nil error", err)
	}

	if tackler.Prompt.ParentName != "tst:b:orch" || arbiter.Prompt.ParentName != "tst:b:orch" {
		t.Errorf("tackler, arbiter ParentName = %q, %q; want tst:b:orch", tackler.Prompt.ParentName, arbiter.Prompt.ParentName)
	}
}

// TestWithTaskWorktree covers the worktree batten's arbiter may edit.
// A present task worktree gives the arbiter a seam and a prompt worktree over the task worktree, never the prime.
// An absent task worktree gives it no seam, so the call is verdict-only, and leaves the prompt worktree as it was.
func TestWithTaskWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	primePath := h.Location.WorktreePath()
	base := shedarbiter.Deps{Prompt: shedarbiter.PromptInput{Worktree: primePath}}

	tests := []struct {
		name         string
		slug         string
		wantSeam     bool
		wantWorktree string
	}{
		{"a present task worktree gives a seam over it", hubforge.TemplatePairSlug, true, h.PairCodeWorktree(hubforge.TemplatePairSlug)},
		{"an absent task worktree gives no seam", "no-such-task", false, primePath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &battenCLI{location: h.Location, slug: tt.slug}

			got, err := c.withTaskWorktree(base)

			if err != nil {
				t.Fatalf("withTaskWorktree() error = %v; want nil", err)
			}
			if (got.Worktree != nil) != tt.wantSeam {
				t.Errorf("withTaskWorktree() seam set = %v; want %v", got.Worktree != nil, tt.wantSeam)
			}
			if got.Prompt.Worktree != tt.wantWorktree {
				t.Errorf("withTaskWorktree() prompt worktree = %q; want %q", got.Prompt.Worktree, tt.wantWorktree)
			}
		})
	}
}
