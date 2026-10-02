// parentreview.go composes the two parent-review prompts: the one-line delivery prompt typed into the live Discussion-Write session, and the reviewer brief the parent's one-shot fork reads.
// Each reads its stencil from stencilsDir at call time and fills it through internal/stencil.
// The SendMessage wording lives in the delivery stencil only, per the Shuttle Provider-Seam Invariant.

package loomengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

const (
	parentReviewDeliveryStencil = "loom-template-parent-review-delivery"
	parentReviewBriefStencil    = "loom-template-parent-review-brief"
	reviewFormatStencil         = "burler-step-2-review"
)

// ParentReviewDeliveryPrompt renders the delivery prompt for the run slug, naming the reviewer brief at briefPath and the reviewer's full agent name.
// It errors when the render contains a newline, since shuttle's Send refuses multi-line text.
func ParentReviewDeliveryPrompt(stencilsDir, slug, briefPath, reviewer string) (string, error) {
	template, err := stencilstore.Read(stencilsDir, parentReviewDeliveryStencil)
	if err != nil {
		return "", fmt.Errorf("loom: read %s: %w", parentReviewDeliveryStencil, err)
	}
	filled, err := stencil.Fill(template, map[string]string{
		"slug":       slug,
		"brief_path": briefPath,
		"reviewer":   reviewer,
	})
	if err != nil {
		return "", fmt.Errorf("loom: fill %s: %w", parentReviewDeliveryStencil, err)
	}
	out := strings.TrimSpace(string(filled))
	if strings.ContainsAny(out, "\r\n") {
		return "", fmt.Errorf("loom: %s renders to more than one line; shuttle's Send refuses multi-line text, so fix the stencil", parentReviewDeliveryStencil)
	}
	return out, nil
}

// ParentReviewBrief renders the reviewer brief for the run slug, pointing at the two discussion files and at the deployed review-format stencil.
func ParentReviewBrief(stencilsDir, slug, decisionRecordPath, supportLogPath string) (string, error) {
	template, err := stencilstore.Read(stencilsDir, parentReviewBriefStencil)
	if err != nil {
		return "", fmt.Errorf("loom: read %s: %w", parentReviewBriefStencil, err)
	}
	filled, err := stencil.Fill(template, map[string]string{
		"slug":                 slug,
		"decision_record_path": decisionRecordPath,
		"support_log_path":     supportLogPath,
		"review_format_path":   stencilstore.Path(stencilsDir, reviewFormatStencil),
	})
	if err != nil {
		return "", fmt.Errorf("loom: fill %s: %w", parentReviewBriefStencil, err)
	}
	return string(filled), nil
}
