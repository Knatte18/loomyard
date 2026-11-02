// prompt_test.go covers RenderPrompt over the shipped frame and a one-marker test body: the values it fills, the directives it renders and the inputs it refuses.

package gatefixer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// testBodyStencil is the caller body the test writes into the seeded stencils directory: one marker.
const testBodyStencil = "gatefix-body-test"

// seededInputs returns the inputs of a render over a seeded stencils directory holding the test body.
func seededInputs(t *testing.T) PromptInputs {
	t.Helper()
	dir := stencilkit.Seed(t)
	if err := os.WriteFile(stencilstore.Path(dir, testBodyStencil), []byte("The body marker is {{.body_marker}}.\n"), 0o644); err != nil {
		t.Fatalf("write the test body: %v", err)
	}
	return PromptInputs{
		StencilsDir: dir,
		BodyStencil: testBodyStencil,
		BodyValues:  map[string]string{"body_marker": "BODYVALUE"},
		ReportPath:  "/lym/report.md",
		ReplyPath:   "/lym/reply.md",
	}
}

// TestRenderPrompt pins the values the frame and body are filled with, the parent and friction directives' two variants, and each refusal.
func TestRenderPrompt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("FillsThePathsAndTheBodyValue", func(t *testing.T) {
		t.Parallel()
		text, err := RenderPrompt(seededInputs(t))
		if err != nil {
			t.Fatalf("RenderPrompt() error = %v; want nil", err)
		}
		for _, want := range []string{"/lym/report.md", "/lym/reply.md", "The body marker is BODYVALUE."} {
			if !strings.Contains(string(text), want) {
				t.Errorf("RenderPrompt() lacks %q", want)
			}
		}
		if strings.Contains(string(text), "{{") {
			t.Errorf("RenderPrompt() holds an unfilled marker")
		}
	})

	t.Run("BodyValueNamedReportPathIsOverwritten", func(t *testing.T) {
		t.Parallel()
		in := seededInputs(t)
		in.BodyValues["report_path"] = "/body/report.md"
		text, err := RenderPrompt(in)
		if err != nil {
			t.Fatalf("RenderPrompt() error = %v; want nil", err)
		}
		if !strings.Contains(string(text), "/lym/report.md") || strings.Contains(string(text), "/body/report.md") {
			t.Errorf("RenderPrompt() = %q; want the frame's report path and not the body's", text)
		}
	})

	t.Run("ParentDirectiveTakesItsNamedAndNoParentVariants", func(t *testing.T) {
		t.Parallel()
		in := seededInputs(t)
		none, err := parentdirective.Directive(in.StencilsDir, "", false)
		if err != nil {
			t.Fatalf("parentdirective.Directive(no parent) = %v", err)
		}
		named, err := parentdirective.Directive(in.StencilsDir, "ly:orch", false)
		if err != nil {
			t.Fatalf("parentdirective.Directive(named) = %v", err)
		}

		text, err := RenderPrompt(in)
		if err != nil || !strings.Contains(string(text), none) {
			t.Fatalf("RenderPrompt(no parent) = %v; want the no-parent directive %q", err, none)
		}
		in.ParentName = "ly:orch"
		text, err = RenderPrompt(in)
		if err != nil || !strings.Contains(string(text), named) || strings.Contains(string(text), none) {
			t.Errorf("RenderPrompt(named parent) = %v; want the named directive %q and not the no-parent one", err, named)
		}
	})

	t.Run("FrictionDirectiveRendersOnlyWithANotePath", func(t *testing.T) {
		t.Parallel()
		in := seededInputs(t)
		without, err := RenderPrompt(in)
		if err != nil {
			t.Fatalf("RenderPrompt() error = %v; want nil", err)
		}
		in.NotePath = filepath.Join("/lym", "friction", "note.md")
		with, err := RenderPrompt(in)
		if err != nil {
			t.Fatalf("RenderPrompt(NotePath) error = %v; want nil", err)
		}
		if !strings.Contains(string(with), in.NotePath) || strings.Contains(string(without), in.NotePath) {
			t.Errorf("note path present with a NotePath = %v, without = %v; want present only with it", strings.Contains(string(with), in.NotePath), strings.Contains(string(without), in.NotePath))
		}
	})

	refusals := []struct {
		name     string
		mutate   func(in *PromptInputs)
		wantText string
	}{
		{"EmptyReplyPath", func(in *PromptInputs) { in.ReplyPath = "" }, "ReplyPath"},
		{"EmptyReportPath", func(in *PromptInputs) { in.ReportPath = "" }, "ReportPath"},
		{"MissingBodyStencil", func(in *PromptInputs) { in.BodyStencil = "gatefix-body-absent" }, "gatefix-body-absent"},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := seededInputs(t)
			tt.mutate(&in)
			text, err := RenderPrompt(in)
			if err == nil || text != nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("RenderPrompt() = %q, %v; want a refusal naming %q", text, err, tt.wantText)
			}
		})
	}
}
