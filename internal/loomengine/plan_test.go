// plan_test.go — untagged Tier-1 unit tests for PlanTable.
// Pure Go over an in-memory Config, a temp-dir modelspec registry and a seeded stencils directory;
// the lead's prompt is composed through seatengine.Prompts, which starts nothing.

package loomengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// planTableLayout returns a location under a temp hub, anchored under anchorRel, whose anchor directory exists.
func planTableLayout(t *testing.T, anchorRel string) *lyxcwd.Location {
	t.Helper()
	layout := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "repo", AnchorRel: anchorRel}
	if err := os.MkdirAll(layout.AnchorPath(), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", layout.AnchorPath(), err)
	}
	return layout
}

// planTableConfig returns a config whose plan_lenses key is lenses.
func planTableConfig(lenses string) Config {
	return Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120, PlanLenses: lenses}
}

// templatePlanFan is the fan the embedded burler template defines for the Plan-Write lead's lens forks.
const templatePlanFan = "twill-plan"

// TestPlanTable verifies PlanTable builds the lead the Plan-Write row needs, at an unanchored location and at a subpath-anchored one, with and without the plan_lenses key.
// Its lead carries the plan role, model, effort, version, outputs, skills, segment, timeout, interactivity and fork flag,
// and its prompt from seatengine.Prompts names the told paths.
// The anchored row has a non-"." AnchorRel so the anchor path and the worktree path are distinguishable strings:
// the plan paths must be rooted at layout.AnchorPath() and never layout.WorktreePath().
//
//testtiming:keep pins every lead field and the anchor-rooted plan paths, which TestProducerSpecs_SkillsAndParentDirective does not assert
func TestPlanTable(t *testing.T) {
	t.Parallel()
	reg := discussionTableRegistry(t)
	wantOverview := func(layout *lyxcwd.Location) string {
		return filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName, planparser.PlanDirName, "00-overview.md")
	}

	tests := []struct {
		name      string
		anchorRel string
		lenses    string
		wantForks bool
	}{
		{"no lens key unanchored", "", "", false},
		{"no lens key anchored under backend", "backend", "", false},
		{"lens key unanchored", "", templatePlanFan, true},
		{"lens key anchored under backend", "backend", templatePlanFan, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			layout := planTableLayout(t, tt.anchorRel)
			stencilsDir := newTestStencilsDir(t)
			specsDir := newTestSpecsDir(t)
			if !filepath.IsAbs(specsDir) {
				t.Fatalf("newTestSpecsDir(t) = %q; want an absolute path -- a deployed spec sits outside the agent's own worktree, so only an absolute spelling is reachable", specsDir)
			}
			cfg := planTableConfig(tt.lenses)
			dirs := configengine.Dirs{Anchor: layout.AnchorPath()}

			table, err := PlanTable(layout, dirs, stencilsDir, specsDir, cfg, reg, nil)
			if err != nil {
				t.Fatalf("PlanTable(...) = _, %v; want nil error", err)
			}
			if err := table.Validate(stencilsDir); err != nil {
				t.Fatalf("table.Validate() = %v; want nil", err)
			}
			if got, want := table.Values["relevant_path"], DiscussionRelevant(layout); got != want {
				t.Errorf("table.Values[\"relevant_path\"] = %q; want %q", got, want)
			}
			if len(table.Seats) != 1 {
				t.Fatalf("table has %d seats; want the lead alone", len(table.Seats))
			}

			lead := table.Chair()
			if role := seatengine.SeatRole(table.RolePrefix, lead.Name); lead.Name != "" || role != "plan" {
				t.Errorf("lead name %q and role %q; want no name and the role %q", lead.Name, role, "plan")
			}
			if lead.Engine != "claude" || lead.Model != "opus" || lead.Effort != "high" || lead.Version != "" {
				t.Errorf("lead engine/model/effort/version = %q/%q/%q/%q; want claude/opus/high/none", lead.Engine, lead.Model, lead.Effort, lead.Version)
			}
			if want := []string{wantOverview(layout)}; !slices.Equal(lead.Outputs, want) {
				t.Errorf("lead outputs = %v; want the overview %v", lead.Outputs, want)
			}
			if want := []string{"scribe:prose", "scribe:testing"}; !slices.Equal(lead.Skills, want) {
				t.Errorf("lead skills = %v; want %v", lead.Skills, want)
			}
			if table.Segment != segmentcolor.Plan {
				t.Errorf("table segment = %q; want %q", table.Segment, segmentcolor.Plan)
			}
			if table.Timeout != 120*time.Minute {
				t.Errorf("table timeout = %s; want 2h0m0s", table.Timeout)
			}
			if table.Interactive {
				t.Error("table interactive = true; want false")
			}
			if lead.ForkSubagents != tt.wantForks {
				t.Errorf("lead ForkSubagents = %v; want %v", lead.ForkSubagents, tt.wantForks)
			}
			if want := (shuttleengine.ModelKey{Module: "loom", Key: "plan"}); lead.ModelKey != want {
				t.Errorf("lead ModelKey = %+v; want %+v", lead.ModelKey, want)
			}
			if gotWake := slices.Contains(lead.Requires, shuttleengine.RequirementBackgroundWake); gotWake != (tt.lenses != "") {
				t.Errorf("lead requires background-wake = %v (Requires %v); want exactly when lenses are set (%q)", gotWake, lead.Requires, tt.lenses)
			}

			prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
			if err != nil {
				t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
			}
			prompt := prompts[""]

			wantPlanDir := filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName, planparser.PlanDirName)
			for _, want := range []string{"`ly:orch`", DiscussionDecisionRecord(layout), planparser.PlanDir(layout.AnchorPath()), planparser.PlanOverview(layout.AnchorPath()), wantPlanDir, specsDir} {
				if !strings.Contains(prompt, want) {
					t.Errorf("lead prompt does not contain %q", want)
				}
			}
			if tt.anchorRel != "" {
				wrongPlanDir := filepath.Join(layout.WorktreePath(), lyxdirs.LyxDirName, planparser.PlanDirName)
				if strings.Contains(prompt, wrongPlanDir) {
					t.Errorf("lead prompt contains the WorktreePath()-rooted plan dir %q; want only the AnchorPath()-rooted one", wrongPlanDir)
				}
			}
			if strings.Contains(prompt, "{{") {
				t.Error("lead prompt contains a leftover \"{{\" marker; want every marker filled")
			}
			if strings.Contains(prompt, "support-log.md") || strings.Contains(prompt, DiscussionSupportLog(layout)) {
				t.Error("lead prompt names the support log; the Plan producer must never read it")
			}
			if has := strings.Contains(prompt, "## Lens review by forks"); has != tt.wantForks {
				t.Errorf("lead prompt carries the twill block = %v; want %v", has, tt.wantForks)
			}
		})
	}

	t.Run("PATTERN.md is read from the worktree root", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			anchorRel string
			// patternAt is "worktree", "anchor" or "" for no PATTERN.md.
			patternAt     string
			wantDirective bool
		}{
			{"PATTERN.md at the worktree root", "", "worktree", true},
			{"PATTERN.md at the worktree root of an anchored location", "backend", "worktree", true},
			{"PATTERN.md under the anchor path alone is not read", "backend", "anchor", false},
			{"no PATTERN.md renders cleanly", "", "", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				layout := planTableLayout(t, tt.anchorRel)
				switch tt.patternAt {
				case "worktree":
					writeTestPattern(t, layout.WorktreePath())
				case "anchor":
					writeTestPattern(t, layout.AnchorPath())
				}
				stencilsDir := newTestStencilsDir(t)

				table, err := PlanTable(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, newTestSpecsDir(t), planTableConfig(""), reg, nil)
				if err != nil {
					t.Fatalf("PlanTable(...) = _, %v; want nil error", err)
				}
				prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
				if err != nil {
					t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
				}
				prompt := prompts[""]

				if !strings.Contains(prompt, "Edit or Write") {
					t.Errorf("lead prompt lacks the edit directive's \"Edit or Write\" sentence: %q", prompt)
				}
				directiveIdx := strings.Index(prompt, "## Constraints")
				if tt.wantDirective {
					if stepIdx := strings.Index(prompt, "## Step 1"); directiveIdx == -1 || stepIdx == -1 || directiveIdx >= stepIdx {
						t.Errorf("pattern_directive (idx %d) does not precede ## Step 1 (idx %d) in prompt: %q", directiveIdx, stepIdx, prompt)
					}
					return
				}
				if directiveIdx != -1 {
					t.Errorf("lead prompt contains a ## Constraints heading with no PATTERN.md at the worktree root: %q", prompt)
				}
				// Two adjacent optional markers (pattern_directive, friction_directive) both rendering empty produce one more blank line than a single empty marker would.
				if strings.Contains(prompt, "\n\n\n\n\n") {
					t.Errorf("lead prompt contains a stray blank-line block: %q", prompt)
				}
			})
		}
	})

	t.Run("the friction note path reaches the lead prompt, and an off friction renders nothing", func(t *testing.T) {
		t.Parallel()
		layout := planTableLayout(t, "")
		stencilsDir := newTestStencilsDir(t)
		notePath := friction.NotePath(LoomFrictionDir(layout), "Plan-Write")
		if notePath == "" {
			t.Fatal("friction.NotePath(LoomFrictionDir(layout), \"Plan-Write\") = \"\"; want a resolved path")
		}
		for _, enabled := range []bool{true, false} {
			cfg := planTableConfig("")
			if enabled {
				cfg.Friction = "sonnet[medium]"
			}
			table, err := PlanTable(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, newTestSpecsDir(t), cfg, reg, nil)
			if err != nil {
				t.Fatalf("PlanTable(friction=%v) = _, %v; want nil error", enabled, err)
			}
			prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
			if err != nil {
				t.Fatalf("seatengine.Prompts(friction=%v) = _, %v; want nil error", enabled, err)
			}
			if has := strings.Contains(prompts[""], notePath); has != enabled {
				t.Errorf("lead prompt carries the friction note path = %v; want %v", has, enabled)
			}
			if !enabled && strings.Contains(prompts[""], "Friction note") {
				t.Error("lead prompt contains friction directive content with friction off; want none")
			}
		}
	})

	t.Run("a friction directive stencil that cannot be read does not fail the table", func(t *testing.T) {
		t.Parallel()
		layout := planTableLayout(t, "")
		stencilsDir := newMinimalStencilsDir(t)
		cfg := planTableConfig("")
		cfg.Friction = "sonnet[medium]"

		table, err := PlanTable(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, newTestSpecsDir(t), cfg, reg, nil)
		if err != nil {
			t.Fatalf("PlanTable(...) = _, %v; want nil error", err)
		}
		if _, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table); err != nil {
			t.Fatalf("seatengine.Prompts() = _, %v; want every prompt composed", err)
		}
	})

	t.Run("lens review", func(t *testing.T) {
		t.Parallel()
		templateFan, err := burlerengine.ResolveFan(burlerengine.Config{}, templatePlanFan)
		if err != nil {
			t.Fatalf("ResolveFan(zero Config, %q) = _, %v; want nil error", templatePlanFan, err)
		}
		tests := []struct {
			name       string
			burlerYAML string
			wantLenses []string
		}{
			{name: "template fan without burler.yaml", wantLenses: lensNames(templateFan)},
			{name: "burler.yaml redefines the fan", burlerYAML: "fans:\n  twill-plan: [test-economy]\n", wantLenses: []string{"test-economy"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				layout := planTableLayout(t, "")
				if tt.burlerYAML != "" {
					writeBurlerYAML(t, layout, tt.burlerYAML)
				}
				stencilsDir := newTestStencilsDir(t)

				table, err := PlanTable(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, newTestSpecsDir(t), planTableConfig(templatePlanFan), reg, nil)
				if err != nil {
					t.Fatalf("PlanTable(twill) = _, %v; want nil error", err)
				}
				prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
				if err != nil {
					t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
				}
				prompt := prompts[""]
				if !table.Chair().ForkSubagents {
					t.Error("PlanTable(twill) lead ForkSubagents = false; want true")
				}

				at := 0
				for _, lens := range tt.wantLenses {
					next := strings.Index(prompt[at:], "- "+lens+": ")
					if next < 0 {
						t.Fatalf("twill lead prompt lacks lens %q after offset %d; want the lenses %v in order", lens, at, tt.wantLenses)
					}
					at += next
				}
				if tt.burlerYAML != "" {
					for _, dropped := range templateFan {
						if dropped.Name != "test-economy" && strings.Contains(prompt, "- "+dropped.Name+": ") {
							t.Errorf("twill lead prompt carries lens %q; want only the burler.yaml fan %v", dropped.Name, tt.wantLenses)
						}
					}
				}

				for _, want := range []string{
					stencilstore.Path(stencilsDir, "loom-rubric-plan-review"),
					filepath.Join(planparser.PlanDir(layout.AnchorPath()), planparser.LensReviewFileName),
					"`lyx loom validate-plan --lens-review`",
					"`Step 2`",
					"`lyx loom validate-plan` per `Step 5`",
					"`" + planparser.PlanOverview(layout.AnchorPath()) + "` last",
					"every file in `" + planparser.PlanDir(layout.AnchorPath()) + "`",
					"Report defects only, never a passed check.",
					"opened in this session, a `path:line` or glyph, a `PATTERN` entry, a test or caller the change would break, or the `lyx loom validate-plan` output, and what it shows",
					planTwillSeverityRule,
				} {
					if !strings.Contains(prompt, want) {
						t.Errorf("twill lead prompt lacks %q", want)
					}
				}
				for _, line := range strings.Split(planTwillSpawnPoint, "\n") {
					if !strings.Contains(prompt, line) {
						t.Errorf("twill lead prompt lacks the spawn point line %q", line)
					}
				}
				if strings.Contains(prompt, "died before its Agent call") {
					t.Error("twill lead prompt carries \"died before its Agent call\"; want the fork0 part instead of the relaunch part")
				}

				markerAt := 0
				for _, marker := range []string{"## Step 2", "## Lens review by forks", "Spawn one fork, fork0", "## Step 3"} {
					next := strings.Index(prompt[markerAt:], marker)
					if next < 0 {
						t.Fatalf("twill lead prompt lacks %q after offset %d; want Step 2, the lens review, fork0's spawn and Step 3 in order", marker, markerAt)
					}
					markerAt += next
				}
			})
		}
	})

	t.Run("the verify tiers reach the lead prompt", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			tiers []string
			// want is the text the prompt must hold; wantNone says the list sentence is followed directly by none.
			want     string
			wantNone bool
		}{
			{name: "two tiers render as consecutive bullets, verbatim and in order", tiers: []string{"`go build ./...` first", "then {{.x}}"}, want: "- `go build ./...` first\n- then {{.x}}"},
			{name: "no tiers render none", wantNone: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				layout := planTableLayout(t, "")
				stencilsDir := newTestStencilsDir(t)

				table, err := PlanTable(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, newTestSpecsDir(t), planTableConfig(""), reg, tt.tiers)
				if err != nil {
					t.Fatalf("PlanTable(...) = _, %v; want nil error", err)
				}
				prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
				if err != nil {
					t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
				}
				prompt := prompts[""]

				if tt.wantNone {
					if !strings.Contains(prompt, "this list gives:\nnone\n") {
						t.Error("lead prompt lacks none directly after the list sentence; want the empty list rendered as none")
					}
					if strings.Contains(prompt, "\n- \n") {
						t.Error("lead prompt holds a \"- \" line with empty content; want none instead")
					}
					return
				}
				if !strings.Contains(prompt, tt.want) {
					t.Errorf("lead prompt lacks %q; want the tiers as consecutive bullets", tt.want)
				}
			})
		}
	})

	t.Run("refusals", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			plan string
			// lenses is the plan_lenses value.
			lenses        string
			removeStencil bool
			// burlerYAML, when non-empty, is planted as the anchor's burler.yaml.
			burlerYAML   string
			wantErrNames []string
		}{
			{name: "unparsable plan model-spec", plan: "opus[effort", wantErrNames: []string{"plan role model-spec"}},
			{name: "missing stencil", plan: "opus[effort=high]", removeStencil: true, wantErrNames: []string{"loom-template-plan"}},
			{name: "lens fan with a burler.yaml that does not parse", plan: "opus[effort=high]", lenses: templatePlanFan, burlerYAML: "fans: [unterminated\n", wantErrNames: []string{"plan_lenses", "twill-plan", "burler.yaml", `set the key to ""`}},
			{name: "lens fan naming an undefined lens", plan: "opus[effort=high]", lenses: templatePlanFan, burlerYAML: "fans:\n  twill-plan:\n    - no-such-lens\n", wantErrNames: []string{"plan_lenses", "twill-plan", "no-such-lens", "burler.yaml", `set the key to ""`}},
			{name: "lens fan missing from burler.yaml and the template", plan: "opus[effort=high]", lenses: "no-such-fan", wantErrNames: []string{"plan_lenses", "no-such-fan", "burler.yaml", "define the fan and its lenses in burler.yaml", `set the key to ""`}},
			{name: "lens fan with no lenses", plan: "opus[effort=high]", lenses: templatePlanFan, burlerYAML: "fans:\n  twill-plan: []\n", wantErrNames: []string{"plan_lenses", "twill-plan", "burler.yaml", `set the key to ""`}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				layout := planTableLayout(t, "")
				if tt.burlerYAML != "" {
					writeBurlerYAML(t, layout, tt.burlerYAML)
				}
				stencilsDir := newTestStencilsDir(t)
				if tt.removeStencil {
					stencilkit.Remove(t, stencilsDir, planStencil)
				}
				cfg := planTableConfig(tt.lenses)
				cfg.Plan = tt.plan

				// A missing stencil is the seat engine's refusal: the table builder reads none, and Validate reads every seat's.
				table, err := PlanTable(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, newTestSpecsDir(t), cfg, reg, nil)
				if err == nil {
					err = table.Validate(stencilsDir)
				}
				if err == nil {
					t.Fatalf("PlanTable(Plan=%q) and its Validate = nil; want a refusal", tt.plan)
				}
				for _, want := range tt.wantErrNames {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("PlanTable(Plan=%q) error = %q; want it to name %q", tt.plan, err.Error(), want)
					}
				}
			})
		}
	})
}

// writeTestPattern writes a minimal PATTERN.md into dir.
func writeTestPattern(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "PATTERN.md"), []byte("# PATTERN\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
	}
}

// newTestSpecsDir returns a real, non-empty directory to pass as the specsDir parameter of PlanTable, ReworkTable and their spec predecessors, alongside newTestStencilsDir.
// A real directory rather than an empty-string placeholder is required: specs_dir is a required marker, so an empty value fails the table's validation.
func newTestSpecsDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}
