package orchengine

import (
	"fmt"
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
		render func(dir, path, hubRulesPath, ownRulesPath string) error
		want   []string
		// hubRules and ownRules are the rules file contents to seed, none when empty; secondOwnRules rewrites the own file before a second render.
		hubRules, ownRules, secondOwnRules string
		// wantOrder lists text the last render carries, each after the previous one.
		wantOrder func(hubPath, ownPath string) []string
		// wantGone is text the last render must not carry.
		wantGone string
	}{
		{name: "role with both rules", file: "role.md", render: renderRole,
			want:     []string{"## Commands\n", testIndex, "Decision for <slug>:"},
			hubRules: "Only the orch deploys.\n", ownRules: "This thread owns the board.\n",
			wantOrder: func(string, string) []string {
				return []string{"## Hub-wide rules\n", "Only the orch deploys.\n", "## This orch's rules\n", "This thread owns the board.\n"}
			}},
		{name: "role with only hub rules", file: "role.md", render: renderRole,
			hubRules: "Only the orch deploys.\n",
			wantOrder: func(_, ownPath string) []string {
				return []string{"Only the orch deploys.\n", fmt.Sprintf(noOwnRules, ownPath)}
			}},
		{name: "role with only own rules", file: "role.md", render: renderRole,
			ownRules: "This thread owns the board.\n",
			wantOrder: func(hubPath, _ string) []string {
				return []string{fmt.Sprintf(noHubRules, hubPath), "This thread owns the board.\n"}
			}},
		{name: "role whitespace-only hub rules", file: "role.md", render: renderRole,
			hubRules: "  \n\t\n", ownRules: "This thread owns the board.\n",
			wantOrder: func(hubPath, _ string) []string {
				return []string{fmt.Sprintf(noHubRules, hubPath), "This thread owns the board.\n"}
			}},
		{name: "role own rules rewritten", file: "role.md", render: renderRole,
			ownRules: "Only the orch deploys.\n", secondOwnRules: "Only the operator deploys.\n",
			wantOrder: func(string, string) []string {
				return []string{"## This orch's rules\n", "Only the operator deploys.\n"}
			},
			wantGone: "Only the orch deploys."},
		{name: "note template", file: "note-template.md", render: func(dir, path, _, _ string) error { return RenderNoteTemplateFile(dir, path) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, "orch", c.file)
			hubPath, ownPath := filepath.Join(root, "board", "hub-rules.md"), filepath.Join(root, "own", "rules.md")
			seedRules := func(rulesPath, content string) {
				t.Helper()
				if content == "" {
					return
				}
				if err := os.MkdirAll(filepath.Dir(rulesPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(rulesPath, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			seedRules(hubPath, c.hubRules)
			seedRules(ownPath, c.ownRules)
			dir := seedStencils(t)
			if err := c.render(dir, path, hubPath, ownPath); err != nil {
				t.Fatal(err)
			}
			if c.secondOwnRules != "" {
				seedRules(ownPath, c.secondOwnRules)
				if err := c.render(dir, path, hubPath, ownPath); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if c.wantOrder != nil {
				rest := string(data)
				for _, want := range c.wantOrder(hubPath, ownPath) {
					i := strings.Index(rest, want)
					if i < 0 {
						t.Errorf("%s file lacks %q after the previous wanted text", c.name, want)
						break
					}
					rest = rest[i+len(want):]
				}
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

func renderRole(dir, path, hubRulesPath, ownRulesPath string) error {
	return RenderRoleFile(dir, path, testIndex, hubRulesPath, ownRulesPath)
}

func TestRenderRoleFile_RefusalsWriteNoFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// override replaces the shipped role stencil when non-empty.
		override string
		// dirAt picks which rules path gets a directory in place of a file: "hub", "own" or none.
		dirAt string
		// wantNamed lists substrings the error carries.
		wantNamed func(hubPath, ownPath string) []string
	}{
		{"no marker", "# Hub orchestrator\n\nNo marker here.\n", "", func(string, string) []string { return []string{roleStencilName, commandIndexMarker} }},
		{"no hub rules marker", "# Hub orchestrator\n\n{{.command_index}}\n", "", func(string, string) []string { return []string{roleStencilName, hubRulesMarker} }},
		{"no orch rules marker", "# Hub orchestrator\n\n{{.command_index}}\n\n{{.hub_rules}}\n", "", func(string, string) []string { return []string{roleStencilName, orchRulesMarker} }},
		{"no decision note title marker", "# Hub orchestrator\n\n{{.command_index}}\n\n{{.hub_rules}}\n\n{{.orch_rules}}\n", "", func(string, string) []string { return []string{roleStencilName, decisionNoteTitleMarker} }},
		{"unreadable hub rules", "", "hub", func(hubPath, _ string) []string { return []string{hubPath} }},
		{"unreadable own rules", "", "own", func(_, ownPath string) []string { return []string{ownPath} }},
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
			path := filepath.Join(root, "role.md")
			hubPath, ownPath := filepath.Join(root, "board", "hub-rules.md"), filepath.Join(root, "own", "rules.md")
			for dirAt, rulesPath := range map[string]string{"hub": hubPath, "own": ownPath} {
				if c.dirAt == dirAt {
					if err := os.MkdirAll(rulesPath, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			err := RenderRoleFile(dir, path, testIndex, hubPath, ownPath)
			if err == nil {
				t.Fatal("RenderRoleFile = nil; want a refusal")
			}
			for _, want := range c.wantNamed(hubPath, ownPath) {
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
