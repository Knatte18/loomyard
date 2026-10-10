// parentreview_test.go — untagged Tier-1 unit tests for ParentReviewDeliveryPrompt and ParentReviewBrief.

package loomengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// newParentReviewStencilsDir returns a seeded stencils directory.
func newParentReviewStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

func TestParentReviewDeliveryPrompt_OneLineNamingReviewerSlugBrief(t *testing.T) {
	dir := newParentReviewStencilsDir(t)
	got, err := ParentReviewDeliveryPrompt(dir, "add-json-flag", "/w/_lyx/reviews/parent-review/round-1/brief.md", "hub:orch")
	if err != nil {
		t.Fatalf("ParentReviewDeliveryPrompt(...) = _, %v; want nil error", err)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("ParentReviewDeliveryPrompt(...) = %q; want one line", got)
	}
	for _, want := range []string{"hub:orch", "add-json-flag", "/w/_lyx/reviews/parent-review/round-1/brief.md", "SendMessage", "lyx loom review delivered", "--failed"} {
		if !strings.Contains(got, want) {
			t.Errorf("ParentReviewDeliveryPrompt(...) = %q; want it to contain %q", got, want)
		}
	}
}

func TestParentReviewDeliveryPrompt_RefusesMultiLineRender(t *testing.T) {
	dir := newParentReviewStencilsDir(t)
	got, err := ParentReviewDeliveryPrompt(dir, "slug", "/req", "line1\nline2")
	if err == nil {
		t.Fatalf("ParentReviewDeliveryPrompt(newline reviewer) = %q, nil; want error", got)
	}
}

func TestParentReviewBrief_NamesPathsAndSubmitLines(t *testing.T) {
	dir := newParentReviewStencilsDir(t)
	got, err := ParentReviewBrief(dir, "add-json-flag", "/w/decision.md", "/w/support.md")
	if err != nil {
		t.Fatalf("ParentReviewBrief(...) = _, %v; want nil error", err)
	}
	for _, want := range []string{
		"/w/decision.md",
		"/w/support.md",
		stencilstore.Path(dir, "burler-step-2-review"),
		"lyx board list",
		"lyx board get",
		"lyx loom review approve add-json-flag",
		"lyx loom review reject add-json-flag",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ParentReviewBrief(...) does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{{") {
		t.Errorf("ParentReviewBrief(...) contains an unrendered marker:\n%s", got)
	}

	// An advisor's notes file beside the two discussion files is never part of the brief: the parent review reads the two files only.
	discussionDir := t.TempDir()
	for _, name := range []string{"decision-record.md", "support-log.md", "advisor-1.md"} {
		if err := os.WriteFile(filepath.Join(discussionDir, name), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
		}
	}
	withNotes, err := ParentReviewBrief(dir, "add-json-flag", filepath.Join(discussionDir, "decision-record.md"), filepath.Join(discussionDir, "support-log.md"))
	if err != nil {
		t.Fatalf("ParentReviewBrief(...) beside an advisor notes file = _, %v; want nil error", err)
	}
	if strings.Contains(withNotes, "advisor-") {
		t.Errorf("ParentReviewBrief(...) beside an advisor notes file names an advisor path:\n%s", withNotes)
	}
}
