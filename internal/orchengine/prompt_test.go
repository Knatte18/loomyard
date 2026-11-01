//go:build tier0

package orchengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

const (
	testRolePath         = "/orch/role.md"
	testNoteTemplatePath = "/orch/note-template.md"
	testIndex            = "- board\n  - list: show the board\n"
)

func seedStencils(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

func TestRenderFiles_WriteWithoutLeadingComment(t *testing.T) {
	cases := []struct {
		name   string
		file   string
		render func(dir, path string) error
		want   []string
		// gone lists text the render must not carry.
		gone []string
	}{
		{name: "role", file: "role.md", render: func(dir, path string) error { return RenderRoleFile(dir, path, testIndex) },
			want: []string{"## Commands\n", testIndex, "Decision for <slug>:"},
			gone: []string{"## Hub-wide rules", "## This orch's rules"}},
		{name: "note template", file: "note-template.md", render: RenderNoteTemplateFile},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "orch", c.file)
			if err := c.render(seedStencils(t), path); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) == 0 {
				t.Errorf("%s file is empty", c.name)
			}
			if strings.Contains(string(data), "<!--") {
				t.Errorf("%s file keeps the stencil's leading comment", c.name)
			}
			for _, want := range c.want {
				if !strings.Contains(string(data), want) {
					t.Errorf("%s file lacks %q", c.name, want)
				}
			}
			for _, gone := range c.gone {
				if strings.Contains(string(data), gone) {
					t.Errorf("%s file carries %q", c.name, gone)
				}
			}
		})
	}
}

func TestRenderRoleFile_RefusalsWriteNoFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// override replaces the shipped role stencil.
		override string
		// wantNamed lists substrings the error carries.
		wantNamed []string
	}{
		{"no marker", "# Hub orchestrator\n\nNo marker here.\n", []string{roleStencilName, commandIndexMarker}},
		{"no decision note title marker", "# Hub orchestrator\n\n{{.command_index}}\n", []string{roleStencilName, decisionNoteTitleMarker}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := seedStencils(t)
			if err := os.WriteFile(stencilstore.Path(dir, roleStencilName), []byte(c.override), 0o644); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "role.md")
			err := RenderRoleFile(dir, path, testIndex)
			if err == nil {
				t.Fatal("RenderRoleFile = nil; want a refusal")
			}
			for _, want := range c.wantNamed {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("RenderRoleFile error = %v; want it to name %q", err, want)
				}
			}
			if _, statErr := os.Stat(path); statErr == nil {
				t.Error("a refused render wrote the role file")
			}
		})
	}
}

//testtiming:keep its covering test TestRenderPointers_MultiLineOverrideFails reaches the same render paths but asserts only the multi-line refusal, not each stencil's one-line shape and named paths
func TestRenderPointers_OneLineNamingTheirPaths(t *testing.T) {
	dir := seedStencils(t)
	cases := []struct {
		name   string
		render func() (string, error)
		want   []string
	}{
		{"start", func() (string, error) { return RenderStartPrompt(dir, testRolePath) }, []string{testRolePath}},
		{"resume", func() (string, error) { return RenderResumePrompt(dir, testRolePath, "/tmp/h/one.md") }, []string{testRolePath, "/tmp/h/one.md"}},
		{"reload", func() (string, error) { return RenderReloadPrompt(dir, testRolePath) }, []string{testRolePath}},
		{"compact focus", func() (string, error) { return RenderCompactFocus(dir) }, nil},
		{"usage-limit wake", func() (string, error) {
			return RenderUsageLimitWake(dir, time.Date(2026, 10, 28, 14, 0, 0, 0, time.FixedZone("CET", 3600)))
		}, []string{"reset at 2026-10-28 14:00 +01:00", "lym tender revive"}},
		{"usage-limit wake with no reset time", func() (string, error) { return RenderUsageLimitWake(dir, time.Time{}) }, []string{"could not be read", "lym tender revive"}},
	}
	for _, c := range cases {
		got, err := c.render()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got == "" || strings.ContainsAny(got, "\r\n") {
			t.Errorf("%s: must be one non-empty line: %q", c.name, got)
		}
		for _, want := range c.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s: missing %q: %q", c.name, want, got)
			}
		}
	}
}

//testtiming:keep pins the refusal of a multi-line stencil override naming the stencil, a guard its covering tests do not reach
func TestRenderPointers_MultiLineOverrideFails(t *testing.T) {
	cases := []struct {
		name   string
		render func(dir string) (string, error)
	}{
		{compactStencilName, RenderCompactFocus},
		{startStencilName, func(dir string) (string, error) { return RenderStartPrompt(dir, testRolePath) }},
		{resumeStencilName, func(dir string) (string, error) { return RenderResumePrompt(dir, testRolePath, "/tmp/h/one.md") }},
		{reloadStencilName, func(dir string) (string, error) { return RenderReloadPrompt(dir, testRolePath) }},
		{usageLimitWakeStencil, func(dir string) (string, error) { return RenderUsageLimitWake(dir, time.Time{}) }},
	}
	for _, c := range cases {
		dir := seedStencils(t)
		if err := os.WriteFile(stencilstore.Path(dir, c.name), []byte("line one {{.role_path}} {{.handoff_path}} {{.note_template_path}}\nline two\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := c.render(dir); err == nil || !strings.Contains(err.Error(), c.name) {
			t.Errorf("%s: want error naming the stencil, got %v", c.name, err)
		}
	}
}
