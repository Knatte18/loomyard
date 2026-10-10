// discussiontable_test.go — untagged Tier-1 unit tests for DiscussionTable.
// Pure Go over an in-memory Config, a temp-dir modelspec registry and a seeded stencils directory;
// the seat prompts are composed through seatengine.Prompts, which starts nothing.

package loomengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

const discussionTableSlug = "add-json-flag"

// discussionTableLayout returns an anchored location under a temp hub whose worktree directory exists.
func discussionTableLayout(t *testing.T) *lyxcwd.Location {
	t.Helper()
	layout := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "repo", AnchorRel: "backend"}
	if err := os.MkdirAll(layout.AnchorPath(), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", layout.AnchorPath(), err)
	}
	return layout
}

// discussionTableGeometry returns the seat geometry for layout, naming a parent.
func discussionTableGeometry(layout *lyxcwd.Location, stencilsDir string) seatengine.Geometry {
	return seatengine.Geometry{
		WorktreeRoot: layout.WorktreePath(),
		AnchorPath:   layout.AnchorPath(),
		StencilsDir:  stencilsDir,
		ParentName:   "ly:orch",
		Shortname:    "ly",
		Slug:         discussionTableSlug,
	}
}

// discussionTableConfig returns a config with the first advisors entries of the two advisor model-specs.
func discussionTableConfig(advisors int, interactive bool) Config {
	return Config{
		Discussion:            "opus[effort=high]",
		DiscussionTimeoutMin:  480,
		DiscussionInteractive: interactive,
		DiscussionAdvisors:    ModelSpecList{"sonnet[low]", "opus[high]"}[:advisors],
	}
}

// discussionTableRegistry returns the built-in model registry.
func discussionTableRegistry(t *testing.T) modelspec.Registry {
	t.Helper()
	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}
	return reg
}

// unboundedAskOperator reports whether any sentence of prompt tells the agent to ask the operator without a prohibition word.
func unboundedAskOperator(prompt string) bool {
	for _, sentence := range strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool { return r == '.' || r == '\n' }) {
		asks := strings.Contains(sentence, "ask the operator") || strings.Contains(sentence, "ask the user")
		bound := slices.ContainsFunc([]string{"do not", "don't", "never", "must not", "may not", "before"}, func(word string) bool {
			return strings.Contains(sentence, word)
		})
		if asks && !bound {
			return true
		}
	}
	return false
}

// TestDiscussionTable verifies the table DiscussionTable builds: its settings, seats and optional list for zero, one and two advisors in both modes, that it validates against a seeded stencils directory, and that the seats' composed prompts carry the paths, strand names and mode text they must.
func TestDiscussionTable(t *testing.T) {
	t.Parallel()
	reg := discussionTableRegistry(t)

	for _, advisors := range []int{0, 1, 2} {
		for _, interactive := range []bool{false, true} {
			name := map[bool]string{false: "autonomous", true: "interactive"}[interactive] + "/" + strings.Repeat("advisor,", advisors) + "chair"
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				layout := discussionTableLayout(t)
				stencilsDir := newTestStencilsDir(t)
				cfg := discussionTableConfig(advisors, interactive)

				table, err := DiscussionTable(layout, stencilsDir, cfg, reg, discussionTableSlug)
				if err != nil {
					t.Fatalf("DiscussionTable(...) = _, %v; want nil error", err)
				}

				if table.RolePrefix != discussionRole || table.Segment != segmentcolor.Discussion || table.Timeout != 480*time.Minute || table.Interactive != interactive {
					t.Errorf("table prefix/segment/timeout/interactive = %q/%q/%s/%v; want %q/%q/480m0s/%v", table.RolePrefix, table.Segment, table.Timeout, table.Interactive, discussionRole, segmentcolor.Discussion, interactive)
				}
				if want := []string{"pattern_directive", "friction_directive"}; !slices.Equal(table.Optional, want) {
					t.Errorf("table.Optional = %v; want %v", table.Optional, want)
				}
				if len(table.Seats) != 1+advisors {
					t.Fatalf("table has %d seats; want the chair and %d advisors", len(table.Seats), advisors)
				}

				chair := table.Chair()
				var wantNotes []string
				for n := 1; n <= advisors; n++ {
					wantNotes = append(wantNotes, DiscussionAdvisorNotes(layout, n))
				}
				if want := []string{DiscussionDecisionRecord(layout), DiscussionSupportLog(layout)}; !slices.Equal(chair.Outputs, want) {
					t.Errorf("chair outputs = %v; want %v", chair.Outputs, want)
				}
				if !slices.Equal(chair.Inputs, wantNotes) {
					t.Errorf("chair inputs = %v; want every advisor's notes file %v", chair.Inputs, wantNotes)
				}
				if !slices.Equal(chair.Skills, discussionSkills) || chair.Model == "" || chair.Effort != "high" || chair.Stencil != discussionChairStencil {
					t.Errorf("chair skills/model/effort/stencil = %v/%q/%q/%q; want %v, a model, high and %q", chair.Skills, chair.Model, chair.Effort, chair.Stencil, discussionSkills, discussionChairStencil)
				}
				for n := 1; n <= advisors; n++ {
					advisor := table.Seats[n]
					if advisor.Name != seatengine.AdvisorName(n) || advisor.Stencil != discussionAdvisorStencil || !slices.Equal(advisor.Outputs, []string{DiscussionAdvisorNotes(layout, n)}) || !slices.Equal(advisor.Skills, []string{"scribe:prose"}) || advisor.Model == "" {
						t.Errorf("advisor %d = %+v; want its stencil, its notes file as the only output, scribe:prose alone and a model", n, advisor)
					}
				}
				if advisors == 2 && table.Seats[1].Effort == table.Seats[2].Effort {
					t.Errorf("advisor efforts = %q and %q; want each advisor's own resolved effort", table.Seats[1].Effort, table.Seats[2].Effort)
				}

				if err := table.Validate(stencilsDir); err != nil {
					t.Fatalf("table.Validate() = %v; want nil", err)
				}
				prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
				if err != nil {
					t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
				}

				chairWants := []string{"`ly:orch`", "lyx board get", discussionTableSlug, DiscussionDecisionRecord(layout), DiscussionSupportLog(layout), modeRules(!interactive)}
				for n, notes := range wantNotes {
					chairWants = append(chairWants, "ly:"+discussionTableSlug+":discussion-"+seatengine.AdvisorName(n+1), notes)
				}
				for _, want := range chairWants {
					if !strings.Contains(prompts[seatengine.RoleChair], want) {
						t.Errorf("chair prompt does not contain %q", want)
					}
				}
				if strings.Contains(prompts[seatengine.RoleChair], modeRules(interactive)) {
					t.Errorf("chair prompt carries the %v-mode text; want only the %v-mode text", interactive, !interactive)
				}
				advisorSentence := strings.TrimPrefix(discussionModeRules(true, true), modeRules(true))
				if has, want := strings.Contains(prompts[seatengine.RoleChair], advisorSentence), !interactive && advisors > 0; has != want {
					t.Errorf("chair prompt carries the advisor sentence = %v; want %v", has, want)
				}
				for n := 1; n <= advisors; n++ {
					advisorPrompt := prompts[seatengine.AdvisorName(n)]
					for _, want := range []string{"`ly:" + discussionTableSlug + ":discussion-chair`", DiscussionAdvisorNotes(layout, n), discussionTableSlug} {
						if !strings.Contains(advisorPrompt, want) {
							t.Errorf("advisor %d prompt does not contain %q", n, want)
						}
					}
				}
				for seat, prompt := range prompts {
					if strings.Contains(prompt, "{{") || strings.Contains(prompt, "scribe:") || strings.Contains(prompt, "--auto") || unboundedAskOperator(prompt) {
						t.Errorf("%s prompt carries an unrendered marker, a skill name, --auto or an ask-the-operator sentence:\n%s", seat, prompt)
					}
				}
			})
		}
	}

	t.Run("without PATTERN.md or friction directive stencils", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			friction string
		}{
			{"friction off", ""},
			{"friction on but its directive stencil unreadable", "sonnet[medium]"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				layout := discussionTableLayout(t)
				stencilsDir := newMinimalStencilsDir(t)
				cfg := discussionTableConfig(1, false)
				cfg.Friction = tt.friction

				table, err := DiscussionTable(layout, stencilsDir, cfg, reg, discussionTableSlug)
				if err != nil {
					t.Fatalf("DiscussionTable(...) = _, %v; want nil error", err)
				}
				if err := table.Validate(stencilsDir); err != nil {
					t.Fatalf("table.Validate() = %v; want nil", err)
				}
				if _, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table); err != nil {
					t.Fatalf("seatengine.Prompts() = _, %v; want every prompt composed", err)
				}
			})
		}
	})

	t.Run("a planted PATTERN.md reaches the chair prompt before Step 1", func(t *testing.T) {
		t.Parallel()
		layout := discussionTableLayout(t)
		stencilsDir := newTestStencilsDir(t)
		if err := os.WriteFile(filepath.Join(layout.WorktreePath(), "PATTERN.md"), []byte("- PATTERN-sample: a rule\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
		}

		table, err := DiscussionTable(layout, stencilsDir, discussionTableConfig(1, false), reg, discussionTableSlug)
		if err != nil {
			t.Fatalf("DiscussionTable(...) = _, %v; want nil error", err)
		}
		prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
		if err != nil {
			t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
		}

		chairPrompt := prompts[seatengine.RoleChair]
		directiveAt, stepOneAt := strings.Index(chairPrompt, "- PATTERN-sample: a rule"), strings.Index(chairPrompt, "## Step 1")
		if !strings.Contains(chairPrompt, "check every design decision against these") || directiveAt < 0 || stepOneAt < 0 || directiveAt > stepOneAt {
			t.Errorf("chair prompt directive at %d, Step 1 at %d; want the designer directive before Step 1", directiveAt, stepOneAt)
		}
	})

	t.Run("an on-disk edit of the chair stencil reaches the chair prompt", func(t *testing.T) {
		t.Parallel()
		layout := discussionTableLayout(t)
		stencilsDir := newTestStencilsDir(t)
		const marker = "THE-CHAIR-STENCIL-WAS-EDITED-ON-DISK"
		edited := "<!-- banner -->\n\n" + marker + "\n{{template \"seat-directive-chair\"}}\n"
		if err := os.WriteFile(filepath.Join(stencilsDir, "loom", discussionChairStencil+".md"), []byte(edited), 0o644); err != nil {
			t.Fatalf("WriteFile(chair stencil) = %v; want nil", err)
		}

		table, err := DiscussionTable(layout, stencilsDir, discussionTableConfig(1, false), reg, discussionTableSlug)
		if err != nil {
			t.Fatalf("DiscussionTable(...) = _, %v; want nil error", err)
		}
		prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
		if err != nil {
			t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
		}
		if !strings.Contains(prompts[seatengine.RoleChair], marker) {
			t.Errorf("chair prompt does not contain the on-disk-edited marker %q; want the edited stencil, not the embedded default", marker)
		}
	})
}

// TestDiscussionTable_Refuses verifies DiscussionTable rejects an empty slug, an unreadable designer directive stencil (the first stencil it reads, when PATTERN.md is active), an unparsable chair model, and an advisor entry the registry does not define, naming the key, the index and the way forward.
func TestDiscussionTable_Refuses(t *testing.T) {
	t.Parallel()
	reg := discussionTableRegistry(t)
	tests := []struct {
		name        string
		slug        string
		missingDir  bool
		mutate      func(*Config)
		wantErrHold []string
	}{
		{name: "empty slug", slug: "", wantErrHold: []string{"slug must not be empty"}},
		{name: "missing stencils directory", slug: discussionTableSlug, missingDir: true, wantErrHold: []string{"pattern-directive-designer"}},
		{name: "unparsable chair model", slug: discussionTableSlug, mutate: func(c *Config) { c.Discussion = "opus[effort" }, wantErrHold: []string{"discussion role model-spec"}},
		{name: "advisor the registry does not define", slug: discussionTableSlug, mutate: func(c *Config) { c.DiscussionAdvisors = ModelSpecList{"sonnet[low]", "ghost"} }, wantErrHold: []string{"discussion_advisors", "entry 2", "ghost", "set the entry to a model-spec the registry defines"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			layout := discussionTableLayout(t)
			stencilsDir := newTestStencilsDir(t)
			if tt.missingDir {
				stencilsDir = filepath.Join(t.TempDir(), "does-not-exist")
				if err := os.WriteFile(filepath.Join(layout.WorktreePath(), "PATTERN.md"), []byte("- PATTERN-sample: a rule\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
				}
			}
			cfg := discussionTableConfig(1, false)
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			_, err := DiscussionTable(layout, stencilsDir, cfg, reg, tt.slug)
			if err == nil {
				t.Fatalf("DiscussionTable(slug=%q) = _, nil; want a refusal", tt.slug)
			}
			for _, want := range tt.wantErrHold {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("DiscussionTable(slug=%q) error = %q; want it to contain %q", tt.slug, err.Error(), want)
				}
			}
		})
	}
}
