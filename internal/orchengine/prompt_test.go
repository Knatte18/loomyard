package orchengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/orchrole"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	testRolePath         = "/orch/role.md"
	testNoteTemplatePath = "/orch/note-template.md"
)

func seedStencils(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

//testtiming:keep pins the pointer's one-line shape and the autonomy line with no presence path
func TestWithLaunchFacts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const pointer = "Read the role at /orch/role.md."
	tests := []struct {
		name string
		// presence is the presence the file holds; noPath leaves the presence path empty instead.
		presence slackengine.Presence
		noPath   bool
		// autonomy is the seam's answer; nilSeam leaves the seam nil instead.
		autonomy bool
		nilSeam  bool
		want     string
	}{
		{name: "OnUnderSlack", presence: slackengine.PresenceSlack, autonomy: true, want: pointer + presenceLine + autonomyOnLine},
		{name: "OffUnderSlack", presence: slackengine.PresenceSlack, want: pointer + presenceLine + autonomyOffLine},
		{name: "OnUnderTerminal", presence: slackengine.PresenceTerminal, autonomy: true, want: pointer + autonomyOnLine},
		{name: "OffUnderTerminal", presence: slackengine.PresenceTerminal, want: pointer + autonomyOffLine},
		{name: "OnWithNoPresencePath", noPath: true, autonomy: true, want: pointer + autonomyOnLine},
		{name: "OffWithNoPresencePath", noPath: true, want: pointer + autonomyOffLine},
		{name: "NilSeamReadsOff", presence: slackengine.PresenceSlack, nilSeam: true, want: pointer + presenceLine + autonomyOffLine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var p Paths
			if !tt.noPath {
				p.SlackPresencePath = presenceFileIn(t, tt.presence)
			}
			if !tt.nilSeam {
				p.Autonomy = func() bool { return tt.autonomy }
			}
			got := WithLaunchFacts(p, pointer)
			if got != tt.want {
				t.Errorf("WithLaunchFacts = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "\n") {
				t.Errorf("WithLaunchFacts = %q, want one line", got)
			}
		})
	}
}

func TestRenderFiles_WriteWithoutLeadingComment(t *testing.T) {
	tiergate.Require(t, 0)
	cases := []struct {
		name   string
		file   string
		render func(dir, path string) error
		want   []string
		// gone lists text the render must not carry.
		gone []string
	}{
		{name: "role", file: "role.md", render: func(_, path string) error { return RenderRoleFile(path) },
			want: []string{"Decision for <slug>:"},
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

// TestRenderRoleFile_WritesEachOnDemandFileBesideTheRoleThatNamesIt pins that every on-demand file reaches the session: rendered beside the role and free of its comment.
func TestRenderRoleFile_WritesEachOnDemandFileBesideTheRoleThatNamesIt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	path := filepath.Join(t.TempDir(), "orch", "role.md")
	if err := RenderRoleFile(path); err != nil {
		t.Fatal(err)
	}
	for name := range orchrole.OnDemand {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(path), name))
		if err != nil {
			t.Errorf("on-demand file %s: %v", name, err)
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(string(data)), "# ") {
			t.Errorf("on-demand file %s = %q; want it to open with its heading", name, data)
		}
	}
}

//testtiming:keep its covering test TestRenderPointers_MultiLineOverrideFails reaches the same render paths but asserts only the multi-line refusal, not each stencil's one-line shape and named paths
func TestRenderPointers_OneLineNamingTheirPaths(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
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
