package orchengine

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

func seedStencils(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

func TestRenderStartPrompt(t *testing.T) {
	got, err := RenderStartPrompt(seedStencils(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "hub orchestrator") {
		t.Errorf("start prompt missing role text: %q", got)
	}
	for _, want := range []string{"lyx loom review", "one-shot fork"} {
		if !strings.Contains(got, want) {
			t.Errorf("start prompt missing %q: %q", want, got)
		}
	}
}

func TestRenderHandoffInstruction(t *testing.T) {
	got, err := RenderHandoffInstruction(seedStencils(t), "/tmp/h/one.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "/scribe:handoff ") {
		t.Errorf("handoff instruction should start with /scribe:handoff: %q", got)
	}
	if strings.ContainsAny(got, "\r\n") || !strings.Contains(got, "/tmp/h/one.md") {
		t.Errorf("handoff instruction must be one line containing the path: %q", got)
	}
}

func TestRenderResumePrompt(t *testing.T) {
	got, err := RenderResumePrompt(seedStencils(t), "/tmp/h/one.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(got, "\r\n") || !strings.Contains(got, "/tmp/h/one.md") {
		t.Errorf("resume prompt must be one line containing the path: %q", got)
	}
	for _, want := range []string{"lyx loom review", "one-shot fork"} {
		if !strings.Contains(got, want) {
			t.Errorf("resume prompt missing %q: %q", want, got)
		}
	}
}

func TestRenderAdoptPrompt(t *testing.T) {
	got, err := RenderAdoptPrompt(seedStencils(t))
	if err != nil {
		t.Fatal(err)
	}
	if got == "" || strings.ContainsAny(got, "\r\n") {
		t.Errorf("adopt prompt must be one non-empty line: %q", got)
	}
}

func TestRenderSoftHandoffInstruction(t *testing.T) {
	got, err := RenderSoftHandoffInstruction(seedStencils(t), "/tmp/h/one.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(got, "\r\n") || !strings.Contains(got, "/tmp/h/one.md") || !strings.Contains(got, "DEFER") {
		t.Errorf("soft instruction must be one line with the path and DEFER: %q", got)
	}
}

func TestRenderCompactFocus(t *testing.T) {
	got, err := RenderCompactFocus(seedStencils(t))
	if err != nil {
		t.Fatal(err)
	}
	if got == "" || strings.ContainsAny(got, "\r\n") {
		t.Errorf("compact focus must be one non-empty line: %q", got)
	}
}

func TestRenderAdoptAndSoft_MultiLineOverrideFails(t *testing.T) {
	cases := []struct {
		name   string
		render func(dir string) (string, error)
	}{
		{compactStencilName, RenderCompactFocus},
		{adoptStencilName, RenderAdoptPrompt},
		{softHandoffStencilName, func(dir string) (string, error) { return RenderSoftHandoffInstruction(dir, "/tmp/h/one.md") }},
	}
	for _, c := range cases {
		dir := seedStencils(t)
		if err := os.WriteFile(stencilstore.Path(dir, c.name), []byte("line one\nline two\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := c.render(dir); err == nil || !strings.Contains(err.Error(), c.name) {
			t.Errorf("%s: want error naming the stencil, got %v", c.name, err)
		}
	}
}

func TestRenderResumePrompt_MultiLineOverrideFails(t *testing.T) {
	dir := seedStencils(t)
	path := stencilstore.Path(dir, resumeStencilName)
	if err := os.WriteFile(path, []byte("line one {{.handoff_path}}\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := RenderResumePrompt(dir, "/tmp/h/one.md")
	if err == nil || !strings.Contains(err.Error(), resumeStencilName) {
		t.Fatalf("want error naming %s, got %v", resumeStencilName, err)
	}
}
