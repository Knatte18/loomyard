// parentseat_test.go tables the chair's parent consolidation part of the review step.
// The parent coverage gate entry is tested in parentreview_test.go, beside the ask record it reads.

package burlerengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestParentRulesBlock proves a round with no parent-review ask renders the review step unchanged.
// A round with an ask renders that same step with the parent consolidation part added after a blank line.
func TestParentRulesBlock(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stencilsDir := newTestStencilsDir(t)
	p := newComposableProfile(t)
	plain, err := composeReviewPrompt(stencilsDir, testSpecsDir, "claude", &p, "", "", "", testReviewFiles)
	if err != nil {
		t.Fatalf("composeReviewPrompt(no block) = %v; want nil error", err)
	}
	plainStep := plain.Files[reviewStepFile].Content

	tests := []struct {
		name      string
		opts      RunOpts
		wantBlock bool
	}{
		{name: "no parent-review ask", opts: RunOpts{}},
		{name: "a parent-review ask", opts: RunOpts{ParentAsk: 2}, wantBlock: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			block, err := parentRulesBlock(stencilsDir, "claude", tt.opts)
			if err != nil {
				t.Fatalf("parentRulesBlock() = %v; want nil error", err)
			}
			if (block != "") != tt.wantBlock {
				t.Fatalf("parentRulesBlock() = %q; want a block: %v", block, tt.wantBlock)
			}
			composed, err := composeReviewPrompt(stencilsDir, testSpecsDir, "claude", &p, "", "", block, testReviewFiles)
			if err != nil {
				t.Fatalf("composeReviewPrompt() = %v; want nil error", err)
			}
			step := composed.Files[reviewStepFile].Content
			if !tt.wantBlock {
				if step != plainStep || strings.Contains(step, "parent-review") {
					t.Errorf("review step of a round with no parent ask differs from the plain step or names the parent review:\n%s", step)
				}
				return
			}
			if !strings.Contains(step, block) {
				t.Errorf("review step of a parent-review round lacks the parent block %q", block)
			}
			if !strings.HasPrefix(block, "\n\n") {
				t.Errorf("parentRulesBlock() = %q; want a block led by a blank line", block)
			}
			if got := strings.Replace(step, block, "", 1); got != plainStep {
				t.Errorf("review step without the parent block differs from the plain step\n got: %q\nwant: %q", got, plainStep)
			}
		})
	}
}
