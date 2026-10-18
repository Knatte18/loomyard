package orchengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		render func(dir, path, rulesPath string) error
		want   []string
		// rules is the rules file content to seed, none when empty; secondRules rewrites it before a second render.
		rules, secondRules string
		// wantRules is the text the role file ends with after the last render.
		wantRules string
		// wantGone is text the last render must not carry.
		wantGone string
	}{
		{name: "role", file: "role.md", render: renderRole, want: []string{"## Commands\n", testIndex, "## This repository\n", "Decision for <slug>:"}, wantRules: noRepositoryRules},
		{name: "role with rules", file: "role.md", render: renderRole, want: []string{"## This repository\n", "Only the orch deploys.\n"}, rules: "Only the orch deploys.\n", wantRules: "Only the orch deploys."},
		{name: "role rules rewritten", file: "role.md", render: renderRole, rules: "Only the orch deploys.\n", secondRules: "Only the operator deploys.\n", want: []string{"Only the operator deploys.\n"}, wantRules: "Only the operator deploys.", wantGone: "Only the orch deploys."},
		{name: "role whitespace-only rules", file: "role.md", render: renderRole, rules: "  \n\t\n", wantRules: noRepositoryRules},
		{name: "note template", file: "note-template.md", render: func(dir, path, _ string) error { return RenderNoteTemplateFile(dir, path) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, "orch", c.file)
			rulesPath := filepath.Join(root, "ORCHESTRATOR.md")
			if c.rules != "" {
				if err := os.WriteFile(rulesPath, []byte(c.rules), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			dir := seedStencils(t)
			if err := c.render(dir, path, rulesPath); err != nil {
				t.Fatal(err)
			}
			if c.secondRules != "" {
				if err := os.WriteFile(rulesPath, []byte(c.secondRules), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := c.render(dir, path, rulesPath); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if c.wantRules != "" && !strings.HasSuffix(strings.TrimRight(string(data), " \t\r\n"), c.wantRules) {
				t.Errorf("%s file does not end with %q", c.name, c.wantRules)
			}
			if c.wantGone != "" && strings.Contains(string(data), c.wantGone) {
				t.Errorf("%s file keeps %q after the rules were rewritten", c.name, c.wantGone)
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
		})
	}
}

func renderRole(dir, path, rulesPath string) error {
	return RenderRoleFile(dir, path, testIndex, rulesPath)
}

func TestRenderRoleFile_RefusalsWriteNoFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// override replaces the shipped role stencil when non-empty.
		override string
		// rulesDir creates a directory at the rules path.
		rulesDir bool
		// wantNamed lists substrings the error carries.
		wantNamed func(rulesPath string) []string
	}{
		{"no marker", "# Hub orchestrator\n\nNo marker here.\n", false, func(string) []string { return []string{roleStencilName, commandIndexMarker} }},
		{"no rules marker", "# Hub orchestrator\n\n{{.command_index}}\n", false, func(string) []string { return []string{roleStencilName, repositoryRulesMarker} }},
		{"no decision note title marker", "# Hub orchestrator\n\n{{.command_index}}\n\n{{.repository_rules}}\n", false, func(string) []string { return []string{roleStencilName, decisionNoteTitleMarker} }},
		{"unreadable rules", "", true, func(rulesPath string) []string { return []string{rulesPath} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := seedStencils(t)
			if c.override != "" {
				if err := os.WriteFile(stencilstore.Path(dir, roleStencilName), []byte(c.override), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			root := t.TempDir()
			path, rulesPath := filepath.Join(root, "role.md"), filepath.Join(root, "ORCHESTRATOR.md")
			if c.rulesDir {
				if err := os.MkdirAll(rulesPath, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			err := RenderRoleFile(dir, path, testIndex, rulesPath)
			if err == nil {
				t.Fatal("RenderRoleFile = nil; want a refusal")
			}
			for _, want := range c.wantNamed(rulesPath) {
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
		{"adopt", func() (string, error) { return RenderAdoptPrompt(dir, testRolePath) }, []string{testRolePath}},
		{"resume", func() (string, error) { return RenderResumePrompt(dir, testRolePath, "/tmp/h/one.md") }, []string{testRolePath, "/tmp/h/one.md"}},
		{"reload", func() (string, error) { return RenderReloadPrompt(dir, testRolePath) }, []string{testRolePath}},
		{"compact focus", func() (string, error) { return RenderCompactFocus(dir) }, nil},
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
		{adoptStencilName, func(dir string) (string, error) { return RenderAdoptPrompt(dir, testRolePath) }},
		{resumeStencilName, func(dir string) (string, error) { return RenderResumePrompt(dir, testRolePath, "/tmp/h/one.md") }},
		{reloadStencilName, func(dir string) (string, error) { return RenderReloadPrompt(dir, testRolePath) }},
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
