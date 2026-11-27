// spec_test.go covers the spec and opening prompt of the agent a status-bar agent button starts.

package operatoragent_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/operatoragent"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSessionSpec covers the spec a task worktree's agent and the prime's agent start under.
// It also covers the hub, worktree, place and parent each prompt names.
// A worktree with a PATTERN overview gets the PATTERN directive.
// It also covers the required inputs a call refuses by name.
func TestSessionSpec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stencilsDir := stencilkit.Seed(t)
	choice := shuttleengine.ModelChoice{Engine: "claude", Model: "opus", Effort: "high"}
	task := operatoragent.Inputs{
		StencilsDir:    stencilsDir,
		Name:           "my-agent",
		HubShortname:   "ly",
		HubPath:        "/hubs/loomyard",
		WorktreeRoot:   "/hubs/loomyard/fix-the-thing",
		Slug:           "fix-the-thing",
		ParentName:     "ly:orch",
		LanguageSkills: []string{"scribe:golang-testing"},
	}
	prime := task
	prime.WorktreeRoot, prime.Slug, prime.ParentName = "/hubs/loomyard/loomyard", "", ""
	patterned := task
	patterned.WorktreeRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(patterned.WorktreeRoot, "PATTERN.md"), []byte("- `PATTERN-example` — an example rule.\n"), 0o644); err != nil {
		t.Fatalf("write PATTERN.md: %v", err)
	}

	tests := []struct {
		name      string
		in        operatoragent.Inputs
		wantPlace string
		// wantAlso are further texts the prompt must hold.
		wantAlso []string
	}{
		{"TaskWorktreeNamesItsEntry", task, "the worktree of the board entry `fix-the-thing`", []string{"`ly:orch`"}},
		{"PrimeNamesThePrime", prime, "the hub's prime worktree", []string{"No parent is recorded for this run."}},
		{
			"WorktreeWithAPatternOverviewCarriesTheImplementerDirective", patterned, "the worktree of the board entry `fix-the-thing`",
			[]string{"## Constraints — do this before you write any code", "`PATTERN-example`"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec, err := operatoragent.SessionSpec(tt.in, choice, "claude")
			if err != nil {
				t.Fatalf("SessionSpec() = _, %v; want nil error", err)
			}
			if !spec.Interactive || !spec.Unawaited {
				t.Errorf("SessionSpec() Interactive, Unawaited = %v, %v; want both true", spec.Interactive, spec.Unawaited)
			}
			if !spec.EndsWithSession || !spec.NoCarryOver {
				t.Errorf("SessionSpec() EndsWithSession, NoCarryOver = %v, %v; want both true", spec.EndsWithSession, spec.NoCarryOver)
			}
			if spec.Role != "my-agent" || spec.ConfigRole != agentconfig.RoleAgent {
				t.Errorf("SessionSpec() Role, ConfigRole = %q, %q; want %q, %q", spec.Role, spec.ConfigRole, "my-agent", agentconfig.RoleAgent)
			}
			if spec.ModelChoice != choice || spec.PromptEngine != "claude" {
				t.Errorf("SessionSpec() ModelChoice, PromptEngine = %+v, %q; want %+v, %q", spec.ModelChoice, spec.PromptEngine, choice, "claude")
			}
			if spec.Display.Anchor != render.AnchorOwnWindow || !spec.Display.Focus {
				t.Errorf("SessionSpec() Display = %+v; want a focused %s strand", spec.Display, render.AnchorOwnWindow)
			}
			if !slices.Equal(spec.Skills, tt.in.LanguageSkills) {
				t.Errorf("SessionSpec() Skills = %v; want %v", spec.Skills, tt.in.LanguageSkills)
			}
			for _, want := range append([]string{"`ly`", "`/hubs/loomyard`", "`" + tt.in.WorktreeRoot + "`", tt.wantPlace}, tt.wantAlso...) {
				if !strings.Contains(spec.Prompt, want) {
					t.Errorf("SessionSpec() Prompt lacks %q:\n%s", want, spec.Prompt)
				}
			}
			stencilkit.RequireEditDirectiveOf(t, stencilsDir, spec.Prompt, "claude", "codex")
		})
	}

	t.Run("RefusesAnEmptyRequiredInput", func(t *testing.T) {
		t.Parallel()
		clears := []struct {
			field string
			clear func(*operatoragent.Inputs)
		}{
			{"StencilsDir", func(in *operatoragent.Inputs) { in.StencilsDir = "" }},
			{"Name", func(in *operatoragent.Inputs) { in.Name = "" }},
			{"HubShortname", func(in *operatoragent.Inputs) { in.HubShortname = "" }},
			{"HubPath", func(in *operatoragent.Inputs) { in.HubPath = "" }},
			{"WorktreeRoot", func(in *operatoragent.Inputs) { in.WorktreeRoot = "" }},
		}
		for _, tt := range clears {
			in := task
			tt.clear(&in)
			_, err := operatoragent.SessionSpec(in, choice, "claude")
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Errorf("SessionSpec() with empty %s = _, %v; want an error naming %s", tt.field, err, tt.field)
			}
		}
	})
}
