// parentreview_test.go — untagged Tier-1 unit tests for ParentReviewDeliveryPrompt and ParentReviewBrief.

package loomengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// newParentReviewStencilsDir seeds a temp stencils directory with the two parent-review stencils from the embedded defaults.
func newParentReviewStencilsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string][]byte{
		"loom-template-parent-review-delivery": stencils.LoomTemplateParentReviewDelivery,
		"loom-template-parent-review-brief":    stencils.LoomTemplateParentReviewBrief,
	} {
		path := stencilstore.Path(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	return dir
}

func TestParentReviewDeliveryPrompt_OneLineNamingReviewerSlugRequest(t *testing.T) {
	dir := newParentReviewStencilsDir(t)
	got, err := ParentReviewDeliveryPrompt(dir, "add-json-flag", "/w/_lyx/reviews/parent-review/round-1/request.json", "hub:orch")
	if err != nil {
		t.Fatalf("ParentReviewDeliveryPrompt(...) = _, %v; want nil error", err)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("ParentReviewDeliveryPrompt(...) = %q; want one line", got)
	}
	for _, want := range []string{"hub:orch", "add-json-flag", "/w/_lyx/reviews/parent-review/round-1/request.json", "SendMessage", "lyx loom review delivered", "--failed"} {
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
}
