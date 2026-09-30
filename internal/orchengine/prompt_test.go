package orchengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// seedStencils writes every shipped default into a temporary stencils directory.
func seedStencils(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	reg := stencils.Registry()
	for _, name := range reg.Names() {
		def, _ := reg.Default(name)
		path := stencilstore.Path(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, def, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRenderStartPrompt(t *testing.T) {
	got, err := RenderStartPrompt(seedStencils(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "hub orchestrator") {
		t.Errorf("start prompt missing role text: %q", got)
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
