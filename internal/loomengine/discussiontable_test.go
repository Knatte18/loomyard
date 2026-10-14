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

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/twilldirective"
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

// discussionTableDirs returns the config geometry the twill fan's burler.yaml is read through for layout.
func discussionTableDirs(layout *lyxcwd.Location) configengine.Dirs {
	return configengine.Dirs{Anchor: layout.AnchorPath()}
}

// discussionTableConfig returns a config with the first advisors entries of the two advisor model-specs, on the seats producer when advisors is positive and the single producer otherwise.
func discussionTableConfig(advisors int, interactive bool) Config {
	producer := DiscussionProducerSingle
	if advisors > 0 {
		producer = DiscussionProducerSeats
	}
	return Config{
		Discussion:            "opus[effort=high]",
		DiscussionTimeoutMin:  480,
		DiscussionInteractive: interactive,
		DiscussionProducer:    producer,
		DiscussionAdvisors:    ModelSpecList{"sonnet[low]", "opus[high]"}[:advisors],
	}
}

// discussionTableRegistry returns the built-in model registry.
func discussionTableRegistry(t *testing.T) modelspec.Registry {
	t.Helper()
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
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

				table, err := DiscussionTable(layout, discussionTableDirs(layout), stencilsDir, cfg, reg, discussionTableSlug)
				if err != nil {
					t.Fatalf("DiscussionTable(...) = _, %v; want nil error", err)
				}

				if table.RolePrefix != discussionRole || table.Segment != segmentcolor.Discussion || table.Timeout != 480*time.Minute || table.Interactive != interactive {
					t.Errorf("table prefix/segment/timeout/interactive = %q/%q/%s/%v; want %q/%q/480m0s/%v", table.RolePrefix, table.Segment, table.Timeout, table.Interactive, discussionRole, segmentcolor.Discussion, interactive)
				}
				if want := []string{"pattern_directive", "friction_directive", "twill_directive"}; !slices.Equal(table.Optional, want) {
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
				if role := seatengine.SeatRole(table.RolePrefix, chair.Name); chair.Name != "" || role != "discussion" || role != DiscussionWriterRole() {
					t.Errorf("lead name %q and role %q; want no name and the role discussion", chair.Name, role)
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
					if !strings.Contains(prompts[""], want) {
						t.Errorf("chair prompt does not contain %q", want)
					}
				}
				if strings.Contains(prompts[""], modeRules(interactive)) {
					t.Errorf("chair prompt carries the %v-mode text; want only the %v-mode text", interactive, !interactive)
				}
				advisorSentence := strings.TrimPrefix(discussionModeRules(true, true), modeRules(true))
				if has, want := strings.Contains(prompts[""], advisorSentence), !interactive && advisors > 0; has != want {
					t.Errorf("chair prompt carries the advisor sentence = %v; want %v", has, want)
				}
				for n := 1; n <= advisors; n++ {
					advisorPrompt := prompts[seatengine.AdvisorName(n)]
					for _, want := range []string{"`ly:" + discussionTableSlug + ":discussion`", DiscussionAdvisorNotes(layout, n), discussionTableSlug} {
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
				chairPrompt := prompts[""]
				if strings.Contains(strings.ToLower(chairPrompt), "twill") {
					t.Errorf("chair prompt carries twill text; want none")
				}
				if want := "- `" + DiscussionSupportLog(layout) + "`\n\nTwo narrow carve-outs"; !strings.Contains(chairPrompt, want) {
					t.Errorf("chair prompt does not hold the support-log bullet, one blank line, then the carve-outs")
				}
				if !strings.Contains(chairPrompt, "\n\n## Step 5") || strings.Contains(chairPrompt, "\n\n\n## Step 5") {
					t.Errorf("chair prompt does not hold exactly one blank line before Step 5")
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

				table, err := DiscussionTable(layout, discussionTableDirs(layout), stencilsDir, cfg, reg, discussionTableSlug)
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

		table, err := DiscussionTable(layout, discussionTableDirs(layout), stencilsDir, discussionTableConfig(1, false), reg, discussionTableSlug)
		if err != nil {
			t.Fatalf("DiscussionTable(...) = _, %v; want nil error", err)
		}
		prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
		if err != nil {
			t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
		}

		chairPrompt := prompts[""]
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

		table, err := DiscussionTable(layout, discussionTableDirs(layout), stencilsDir, discussionTableConfig(1, false), reg, discussionTableSlug)
		if err != nil {
			t.Fatalf("DiscussionTable(...) = _, %v; want nil error", err)
		}
		prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
		if err != nil {
			t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
		}
		if !strings.Contains(prompts[""], marker) {
			t.Errorf("chair prompt does not contain the on-disk-edited marker %q; want the edited stencil, not the embedded default", marker)
		}
	})

	t.Run("the friction note path reaches the lead prompt", func(t *testing.T) {
		t.Parallel()
		layout := discussionTableLayout(t)
		stencilsDir := newTestStencilsDir(t)
		cfg := discussionTableConfig(0, false)
		cfg.Friction = "sonnet[medium]"

		table, err := DiscussionTable(layout, discussionTableDirs(layout), stencilsDir, cfg, reg, discussionTableSlug)
		if err != nil {
			t.Fatalf("DiscussionTable(...) = _, %v; want nil error", err)
		}
		prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
		if err != nil {
			t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
		}
		notePath := friction.NotePath(LoomFrictionDir(layout), "Discussion-Write")
		if notePath == "" || !strings.Contains(prompts[""], notePath) {
			t.Errorf("lead prompt does not contain the friction note path %q", notePath)
		}
	})

	// advisors configured, so the producers that run no advisors are told apart from the seats producer.
	producers := []struct {
		name          string
		producer      string
		wantSeats     int
		wantForks     bool
		wantDirective bool
	}{
		{"single runs the lead alone", DiscussionProducerSingle, 1, false, false},
		{"twill runs the lead alone with lens forks", DiscussionProducerTwill, 1, true, true},
		{"seats runs the lead with its advisors", DiscussionProducerSeats, 3, false, false},
	}
	for _, tt := range producers {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			layout := discussionTableLayout(t)
			stencilsDir := newTestStencilsDir(t)
			cfg := discussionTableConfig(2, false)
			cfg.DiscussionProducer = tt.producer

			table, err := DiscussionTable(layout, discussionTableDirs(layout), stencilsDir, cfg, reg, discussionTableSlug)
			if err != nil {
				t.Fatalf("DiscussionTable(producer=%q) = _, %v; want nil error", tt.producer, err)
			}
			if len(table.Seats) != tt.wantSeats {
				t.Fatalf("DiscussionTable(producer=%q) has %d seats; want %d", tt.producer, len(table.Seats), tt.wantSeats)
			}
			for index, seat := range table.Seats {
				wantFork, wantDirective := index == 0 && tt.wantForks, index == 0 && tt.wantDirective
				if seat.ForkSubagents != wantFork {
					t.Errorf("seat %d ForkSubagents = %v; want %v", index, seat.ForkSubagents, wantFork)
				}
				if _, has := seat.Values[twilldirective.MarkerName]; has != wantDirective {
					t.Errorf("seat %d carries the twill directive = %v; want %v", index, has, wantDirective)
				}
			}
			prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
			if err != nil {
				t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
			}
			if has := strings.Contains(prompts[""], "## Lens review by forks"); has != tt.wantDirective {
				t.Errorf("lead prompt carries the twill block = %v; want %v", has, tt.wantDirective)
			}
		})
	}

	t.Run("twill lens review", func(t *testing.T) {
		t.Parallel()
		templateFan, err := burlerengine.ResolveFan(burlerengine.Config{}, twillFan)
		if err != nil {
			t.Fatalf("ResolveFan(zero Config, %q) = _, %v; want nil error", twillFan, err)
		}
		tests := []struct {
			name       string
			burlerYAML string
			// defaultYAML, when non-empty, is the burler.yaml of a temp board dir the load is told, so the fan lives only in the repo default.
			defaultYAML string
			wantLenses  []string
		}{
			{name: "template fan without burler.yaml", wantLenses: lensNames(templateFan)},
			{name: "burler.yaml redefines the fan", burlerYAML: "fans:\n  twill-discussion: [goal-scope]\n", wantLenses: []string{"goal-scope"}},
			{name: "the fan lives only in the repo default", defaultYAML: "fans:\n  twill-discussion: [goal-scope]\n", wantLenses: []string{"goal-scope"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				layout := discussionTableLayout(t)
				if tt.burlerYAML != "" {
					writeBurlerYAML(t, layout, tt.burlerYAML)
				}
				dirs := discussionTableDirs(layout)
				if tt.defaultYAML != "" {
					dirs.Board = t.TempDir()
					defaultPath := configengine.ConfigFile(dirs.Board, "burler")
					if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
						t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(defaultPath), err)
					}
					if err := os.WriteFile(defaultPath, []byte(tt.defaultYAML), 0o644); err != nil {
						t.Fatalf("WriteFile(%q) = %v; want nil", defaultPath, err)
					}
				}
				stencilsDir := newTestStencilsDir(t)
				cfg := discussionTableConfig(0, false)
				cfg.DiscussionProducer = DiscussionProducerTwill

				table, err := DiscussionTable(layout, dirs, stencilsDir, cfg, reg, discussionTableSlug)
				if err != nil {
					t.Fatalf("DiscussionTable(twill) = _, %v; want nil error", err)
				}
				prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
				if err != nil {
					t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
				}
				prompt := prompts[""]

				at := 0
				for _, lens := range tt.wantLenses {
					next := strings.Index(prompt[at:], "- "+lens+": ")
					if next < 0 {
						t.Fatalf("twill lead prompt lacks lens %q after offset %d; want the lenses %v in order", lens, at, tt.wantLenses)
					}
					at += next
				}
				if tt.burlerYAML != "" || tt.defaultYAML != "" {
					for _, dropped := range templateFan {
						if dropped.Name != "goal-scope" && strings.Contains(prompt, "- "+dropped.Name+": ") {
							t.Errorf("twill lead prompt carries lens %q; want only the burler.yaml fan %v", dropped.Name, tt.wantLenses)
						}
					}
				}

				for _, want := range []string{
					stencilstore.Path(stencilsDir, "loom-rubric-discussion-review"),
					`subagent_type: "fork"`,
					"runs `lyx loom validate-discussion` per `Step 6` until it passes",
					"6. Run `lyx loom validate-discussion` again until it passes",
					"per `Step 5`",
					"The writer wrote `" + DiscussionDecisionRecord(layout) + "` and `" + DiscussionSupportLog(layout) + "` and passed its self-check before spawning you.",
					"Once you have written both output files per Step 5 and `lyx loom validate-discussion` passes per Step 6, run this procedure once, before you end your turn:",
					"Fix the record, or reject the finding with a one-line reason.",
					"each with a severity (`BLOCKING`, `MEDIUM` or `LOW`), the section or decision it concerns",
				} {
					if !strings.Contains(prompt, want) {
						t.Errorf("twill lead prompt lacks %q", want)
					}
				}
				if strings.Contains(prompt, "lyx shuttle await") {
					t.Error("twill lead prompt carries a ready-marker wait; want the forks to wait on nothing")
				}

				modeRulesAt := strings.Index(prompt, modeRules(true))
				directiveAt := strings.Index(prompt, "## Lens review by forks")
				stepFiveAt := strings.Index(prompt, "## Step 5")
				if modeRulesAt < 0 || directiveAt < modeRulesAt || stepFiveAt < directiveAt {
					t.Errorf("mode rules at %d, directive at %d, Step 5 at %d; want the directive between the mode rules and Step 5", modeRulesAt, directiveAt, stepFiveAt)
				}
				if want := modeRules(true) + "\n\n## Lens review by forks"; !strings.Contains(prompt, want) {
					t.Errorf("twill lead prompt does not set the directive off as a paragraph after the mode rules")
				}
			})
		}
	})
}

// TestDiscussionTable_Refuses verifies DiscussionTable rejects an empty slug, an unreadable designer directive stencil (the first stencil it reads, when PATTERN.md is active), an unparsable chair model, an advisor entry the registry does not define, and, under twill, a burler.yaml that does not parse or a fan that does not resolve, naming the key, the index or the fan and the way forward.
func TestDiscussionTable_Refuses(t *testing.T) {
	t.Parallel()
	reg := discussionTableRegistry(t)
	tests := []struct {
		name       string
		slug       string
		missingDir bool
		mutate     func(*Config)
		// burlerYAML, when non-empty, is planted as the anchor's burler.yaml.
		burlerYAML  string
		wantErrHold []string
	}{
		{name: "empty slug", slug: "", wantErrHold: []string{"slug must not be empty"}},
		{name: "missing stencils directory", slug: discussionTableSlug, missingDir: true, wantErrHold: []string{"pattern-directive-designer"}},
		{name: "unparsable chair model", slug: discussionTableSlug, mutate: func(c *Config) { c.Discussion = "opus[effort" }, wantErrHold: []string{"discussion role model-spec"}},
		{name: "advisor the registry does not define", slug: discussionTableSlug, mutate: func(c *Config) { c.DiscussionAdvisors = ModelSpecList{"sonnet[low]", "ghost"} }, wantErrHold: []string{"discussion_advisors", "entry 2", "ghost", "set the entry to a model-spec the registry defines"}},
		{name: "twill burler.yaml does not parse", slug: discussionTableSlug, mutate: func(c *Config) { c.DiscussionProducer = DiscussionProducerTwill }, burlerYAML: "fans: [unterminated\n", wantErrHold: []string{"twill-discussion", "burler.yaml"}},
		{name: "twill fan names an undefined lens", slug: discussionTableSlug, mutate: func(c *Config) { c.DiscussionProducer = DiscussionProducerTwill }, burlerYAML: "fans:\n  twill-discussion:\n    - no-such-lens\n", wantErrHold: []string{"twill-discussion", "burler.yaml"}},
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
			if tt.burlerYAML != "" {
				writeBurlerYAML(t, layout, tt.burlerYAML)
			}
			cfg := discussionTableConfig(1, false)
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			_, err := DiscussionTable(layout, discussionTableDirs(layout), stencilsDir, cfg, reg, tt.slug)
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
