// discussion_test.go — untagged Tier-1 unit tests for DiscussionSpec.
// Pure Go over an in-memory Config and a temp-dir modelspec registry;
// no live hub, reed, or network involved.

package loomengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// TestDiscussionSpec verifies DiscussionSpec's field mapping for both autonomous values.
//
//testtiming:keep pins OutputFiles, Interactive, Role, Effort and Timeout for both modes, which TestProducerSpecs_SkillsAndParentDirective does not assert
func TestDiscussionSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}

	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	wantOutputFiles := []string{
		filepath.Join(worktreeRoot, "_lyx", "discussion", "decision-record.md"),
		filepath.Join(worktreeRoot, "_lyx", "discussion", "support-log.md"),
	}
	wantTimeout := 480 * time.Minute

	tests := []struct {
		name            string
		producer        string
		autonomous      bool
		wantInteractive bool
		wantForks       bool
	}{
		{"Interactive", DiscussionProducerSingle, false, true, false},
		{"Autonomous", DiscussionProducerSingle, true, false, false},
		{"Twill autonomous", DiscussionProducerTwill, true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cfg
			cfg.DiscussionProducer = tt.producer
			spec, err := DiscussionSpec(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, newTestStencilsDir(t), "", cfg, reg, "add-json-flag", tt.autonomous)
			if err != nil {
				t.Fatalf("DiscussionSpec(..., autonomous=%v) = _, %v; want nil error", tt.autonomous, err)
			}
			if spec.ForkSubagents != tt.wantForks {
				t.Errorf("DiscussionSpec(producer=%q).ForkSubagents = %v; want %v", tt.producer, spec.ForkSubagents, tt.wantForks)
			}

			if len(spec.OutputFiles) != len(wantOutputFiles) {
				t.Fatalf("DiscussionSpec(...).OutputFiles = %v; want %v", spec.OutputFiles, wantOutputFiles)
			}
			for i, want := range wantOutputFiles {
				if spec.OutputFiles[i] != want {
					t.Errorf("DiscussionSpec(...).OutputFiles[%d] = %q; want %q", i, spec.OutputFiles[i], want)
				}
			}
			if spec.Interactive != tt.wantInteractive {
				t.Errorf("DiscussionSpec(..., autonomous=%v).Interactive = %v; want %v", tt.autonomous, spec.Interactive, tt.wantInteractive)
			}
			if spec.Role != "discussion" {
				t.Errorf("DiscussionSpec(...).Role = %q; want %q", spec.Role, "discussion")
			}
			if spec.Segment != segmentcolor.Discussion {
				t.Errorf("DiscussionSpec(...).Segment = %q; want %q", spec.Segment, segmentcolor.Discussion)
			}
			if spec.Model == "" {
				t.Error("DiscussionSpec(...).Model = \"\"; want non-empty")
			}
			if spec.Effort != "high" {
				t.Errorf("DiscussionSpec(...).Effort = %q; want %q", spec.Effort, "high")
			}
			if spec.Timeout != wantTimeout {
				t.Errorf("DiscussionSpec(...).Timeout = %s; want %s", spec.Timeout, wantTimeout)
			}
			if spec.Prompt == "" {
				t.Error("DiscussionSpec(...).Prompt = \"\"; want non-empty")
			}
		})
	}
}

// TestDiscussionSpec_Refuses verifies DiscussionSpec rejects an empty slug, and, when stencilsDir does not exist, returns an error naming the missing stencil rather than silently falling back to the embedded default, per the missing-board-is-a-hard-error Shared Decision.
func TestDiscussionSpec_Refuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		slug       string
		missingDir bool
		// burlerYAML, when non-empty, runs the twill producer against a temp-dir location whose anchor holds this burler.yaml.
		burlerYAML   string
		wantErrNames []string
	}{
		{name: "empty slug", slug: ""},
		// The parent directive renders before the discussion stencil is read, so it is the first stencil the error names.
		{name: "missing stencils directory", slug: "add-json-flag", missingDir: true, wantErrNames: []string{"parent-directive-none"}},
		{name: "twill burler.yaml does not parse", slug: "add-json-flag", burlerYAML: "fans: [unterminated\n", wantErrNames: []string{"twill-discussion", "burler.yaml"}},
		{name: "twill fan names an undefined lens", slug: "add-json-flag", burlerYAML: "fans:\n  twill-discussion:\n    - no-such-lens\n", wantErrNames: []string{"twill-discussion", "burler.yaml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktreeRoot := filepath.Join("home", "user", "repo")
			layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
			cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}
			if tt.burlerYAML != "" {
				layout = &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "repo"}
				writeBurlerYAML(t, layout, tt.burlerYAML)
				cfg.DiscussionProducer = DiscussionProducerTwill
			}
			reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}
			stencilsDir := newTestStencilsDir(t)
			if tt.missingDir {
				stencilsDir = filepath.Join(t.TempDir(), "does-not-exist")
			}

			_, err = DiscussionSpec(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, "", cfg, reg, tt.slug, false)
			if err == nil {
				t.Fatalf("DiscussionSpec(slug=%q, missingDir=%v) = _, nil; want non-nil error", tt.slug, tt.missingDir)
			}
			for _, want := range tt.wantErrNames {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("DiscussionSpec(slug=%q, missingDir=%v) error = %q; want it to name %q", tt.slug, tt.missingDir, err.Error(), want)
				}
			}
		})
	}
}

// writeBurlerYAML plants content as the burler.yaml of layout's anchor, creating its _lyx/config/ directory.
func writeBurlerYAML(t *testing.T, layout *lyxcwd.Location, content string) {
	t.Helper()
	path := configengine.ConfigFile(layout.AnchorPath(), "burler")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestDiscussionSpec_Twill verifies the twill producer's rendered prompt: the template fan's lenses in order, the rubric path, the fork instruction, the host's spawn point, self-check and step names and no ready-marker wait, with the directive between the mode rules and Step 5, and that a burler.yaml redefining the fan replaces the lenses.
func TestDiscussionSpec_Twill(t *testing.T) {
	t.Parallel()
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480, DiscussionProducer: DiscussionProducerTwill}

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
			layout := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "repo"}
			if tt.burlerYAML != "" {
				writeBurlerYAML(t, layout, tt.burlerYAML)
			}
			dirs := configengine.Dirs{Anchor: layout.AnchorPath()}
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

			spec, err := DiscussionSpec(layout, dirs, stencilsDir, "", cfg, reg, "add-json-flag", true)
			if err != nil {
				t.Fatalf("DiscussionSpec(twill) = _, %v; want nil error", err)
			}
			prompt := spec.Prompt

			at := 0
			for _, lens := range tt.wantLenses {
				next := strings.Index(prompt[at:], "- "+lens+": ")
				if next < 0 {
					t.Fatalf("DiscussionSpec(twill).Prompt lacks lens %q after offset %d; want the lenses %v in order", lens, at, tt.wantLenses)
				}
				at += next
			}
			if tt.burlerYAML != "" || tt.defaultYAML != "" {
				for _, dropped := range templateFan {
					if dropped.Name != "goal-scope" && strings.Contains(prompt, "- "+dropped.Name+": ") {
						t.Errorf("DiscussionSpec(twill).Prompt carries lens %q; want only the burler.yaml fan %v", dropped.Name, tt.wantLenses)
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
					t.Errorf("DiscussionSpec(twill).Prompt lacks %q", want)
				}
			}
			if strings.Contains(prompt, "lyx shuttle await") {
				t.Error("DiscussionSpec(twill).Prompt carries a ready-marker wait; want the forks to wait on nothing")
			}

			modeRulesAt := strings.Index(prompt, "This session is running autonomously")
			directiveAt := strings.Index(prompt, "## Lens review by forks")
			stepFiveAt := strings.Index(prompt, "## Step 5")
			if modeRulesAt < 0 || directiveAt < modeRulesAt || stepFiveAt < directiveAt {
				t.Errorf("mode rules at %d, directive at %d, Step 5 at %d; want the directive between the mode rules and Step 5", modeRulesAt, directiveAt, stepFiveAt)
			}
		})
	}
}

// lensNames returns the names of lenses, in order.
func lensNames(lenses []burlerengine.Lens) []string {
	names := make([]string, len(lenses))
	for i, lens := range lenses {
		names[i] = lens.Name
	}
	return names
}

// TestDiscussionSpec_ReadsStencilAtCallTime verifies DiscussionSpec's prompt reflects an on-disk edit to the stencils directory rather than the embedded default, per the runtime-read-not-embed Shared Decision.
//
//testtiming:keep pins that an on-disk edit of the stencil reaches the prompt, which the covering spec test never edits
func TestDiscussionSpec_ReadsStencilAtCallTime(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}

	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	stencilsDir := newTestStencilsDir(t)
	const marker = "THIS-BODY-WAS-EDITED-ON-DISK"
	discussionPath := filepath.Join(stencilsDir, "loom", "loom-template-discussion.md")
	edited := "<!-- banner --> \n\n" + marker + "\n\n" +
		"{{.slug}} {{.decision_record_path}} {{.support_log_path}} {{.mode_rules}} lyx board get"
	if err := os.WriteFile(discussionPath, []byte(edited), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", discussionPath, err)
	}

	spec, err := DiscussionSpec(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, stencilsDir, "", cfg, reg, "add-json-flag", false)
	if err != nil {
		t.Fatalf("DiscussionSpec(...) = _, %v; want nil error", err)
	}
	if !strings.Contains(spec.Prompt, marker) {
		t.Errorf("DiscussionSpec(...).Prompt does not contain the on-disk-edited marker %q; want the modified stencil, not the embedded default, to reach the composed prompt", marker)
	}
}

// TestDiscussionSpec_PatternDirective proves DiscussionSpec injects the designer directive when PATTERN.md exists at the worktree root, and renders none without it.
// It uses a non-"." AnchorRel, so the root the directive reads is told apart from the anchor path.
//
//testtiming:keep pins the designer directive text and its place before Step 1 for the discussion prompt, which the covering tests do not assert
func TestDiscussionSpec_PatternDirective(t *testing.T) {
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	tests := []struct {
		name        string
		plantAtRoot bool
	}{
		{"PATTERN.md at the worktree root", true},
		{"no PATTERN.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "repo", AnchorRel: "backend"}
			if err := os.MkdirAll(layout.AnchorPath(), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", layout.AnchorPath(), err)
			}
			if tt.plantAtRoot {
				if err := os.WriteFile(filepath.Join(layout.WorktreePath(), "PATTERN.md"), []byte("- PATTERN-sample: a rule\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
				}
			}

			spec, err := DiscussionSpec(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, newTestStencilsDir(t), "", cfg, reg, "add-json-flag", false)
			if err != nil {
				t.Fatalf("DiscussionSpec(...) = _, %v; want nil error", err)
			}

			hasDirective := strings.Contains(spec.Prompt, "check every design decision against these") &&
				strings.Contains(spec.Prompt, "- PATTERN-sample: a rule")
			if hasDirective != tt.plantAtRoot {
				t.Errorf("DiscussionSpec(...).Prompt carries the designer directive = %v; want %v", hasDirective, tt.plantAtRoot)
			}
			if tt.plantAtRoot {
				directiveAt := strings.Index(spec.Prompt, "## Constraints")
				stepOneAt := strings.Index(spec.Prompt, "## Step 1")
				if directiveAt < 0 || directiveAt > stepOneAt {
					t.Errorf("designer directive at %d, Step 1 at %d; want the directive before Step 1", directiveAt, stepOneAt)
				}
			}
		})
	}
}

// TestProducerSpecs_SkillsAndParentDirective verifies each loom producer spec carries its role's skills in order, names the told parent in its prompt, and renders the no-parent variant for an empty name.
// The operator-ban line is absent only for the interactive Discussion.
func TestProducerSpecs_SkillsAndParentDirective(t *testing.T) {
	t.Parallel()

	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Discussion: "opus[effort=high]", Plan: "opus[effort=high]"}
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	const operatorBan = "A question to the operator in your pane is never the way forward."
	build := map[string]func(parent string) (shuttleengine.Spec, error){
		"discussion-interactive": func(parent string) (shuttleengine.Spec, error) {
			return DiscussionSpec(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, newTestStencilsDir(t), parent, cfg, reg, "add-json-flag", false)
		},
		"discussion-autonomous": func(parent string) (shuttleengine.Spec, error) {
			return DiscussionSpec(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, newTestStencilsDir(t), parent, cfg, reg, "add-json-flag", true)
		},
		"plan": func(parent string) (shuttleengine.Spec, error) {
			return PlanSpec(layout, configengine.Dirs{Anchor: layout.AnchorPath()}, newTestStencilsDir(t), newTestSpecsDir(t), parent, cfg, reg)
		},
		"rework": func(parent string) (shuttleengine.Spec, error) {
			return ReworkSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), parent, cfg, reg, 4, "prior")
		},
	}
	wantSkills := map[string][]string{
		"discussion-interactive": {"scribe:prose", "scribe:conversation"},
		"discussion-autonomous":  {"scribe:prose", "scribe:conversation"},
		"plan":                   {"scribe:prose", "scribe:testing"},
		"rework":                 {"scribe:prose", "scribe:testing"},
	}

	for name, mk := range build {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			withParent, err := mk("shortname:orch")
			if err != nil {
				t.Fatalf("spec with parent = _, %v; want nil error", err)
			}
			if !reflect.DeepEqual(withParent.Skills, wantSkills[name]) {
				t.Errorf("Skills = %v; want %v", withParent.Skills, wantSkills[name])
			}
			if !strings.Contains(withParent.Prompt, "`shortname:orch`") {
				t.Errorf("Prompt does not name the told parent %q", "shortname:orch")
			}
			wantBan := name != "discussion-interactive"
			if got := strings.Contains(withParent.Prompt, operatorBan); got != wantBan {
				t.Errorf("Prompt carries the operator-ban line = %v; want %v", got, wantBan)
			}

			noParent, err := mk("")
			if err != nil {
				t.Fatalf("spec without parent = _, %v; want nil error", err)
			}
			if !strings.Contains(noParent.Prompt, "No parent is recorded for this run.") {
				t.Error("Prompt does not carry the no-parent variant for an empty parent name")
			}
			if strings.Contains(noParent.Prompt, "{{") {
				t.Error("Prompt contains an unrendered marker")
			}
		})
	}
}
