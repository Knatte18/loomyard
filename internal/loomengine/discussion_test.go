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

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestDiscussionSpec verifies DiscussionSpec's field mapping for both autonomous values.
//
//testtiming:keep pins OutputFiles, Interactive, AwaitOperator, Role, Effort and Timeout for both modes, which TestProducerSpecs_SkillsAndParentDirective does not assert
func TestDiscussionSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	wantOutputFiles := []string{
		filepath.Join(worktreeRoot, "_lyx", "discussion", "decision-record.md"),
		filepath.Join(worktreeRoot, "_lyx", "discussion", "support-log.md"),
	}
	wantTimeout := 480 * time.Minute

	tests := []struct {
		name              string
		autonomous        bool
		wantInteractive   bool
		wantAwaitOperator bool
	}{
		{"Interactive", false, true, true},
		{"Autonomous", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := DiscussionSpec(layout, newTestStencilsDir(t), "", cfg, reg, "add-json-flag", tt.autonomous)
			if err != nil {
				t.Fatalf("DiscussionSpec(..., autonomous=%v) = _, %v; want nil error", tt.autonomous, err)
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
			if spec.AwaitOperator != tt.wantAwaitOperator {
				t.Errorf("DiscussionSpec(..., autonomous=%v).AwaitOperator = %v; want %v", tt.autonomous, spec.AwaitOperator, tt.wantAwaitOperator)
			}
			if spec.Role != "discussion" {
				t.Errorf("DiscussionSpec(...).Role = %q; want %q", spec.Role, "discussion")
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

// TestDiscussionSpec_Refuses verifies DiscussionSpec rejects an empty slug, and, when stencilsDir does not exist,
// returns an error naming the missing stencil rather than silently falling back to the embedded default,
// per the missing-board-is-a-hard-error Shared Decision.
func TestDiscussionSpec_Refuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		slug         string
		missingDir   bool
		wantErrNames string
	}{
		{name: "empty slug", slug: ""},
		// The parent directive renders before the discussion stencil is read, so it is the first stencil the error names.
		{name: "missing stencils directory", slug: "add-json-flag", missingDir: true, wantErrNames: "parent-directive-none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktreeRoot := filepath.Join("home", "user", "repo")
			layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
			cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}
			reg, err := modelspec.LoadRegistry(t.TempDir())
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}
			stencilsDir := newTestStencilsDir(t)
			if tt.missingDir {
				stencilsDir = filepath.Join(t.TempDir(), "does-not-exist")
			}

			_, err = DiscussionSpec(layout, stencilsDir, "", cfg, reg, tt.slug, false)
			if err == nil {
				t.Fatalf("DiscussionSpec(slug=%q, missingDir=%v) = _, nil; want non-nil error", tt.slug, tt.missingDir)
			}
			if !strings.Contains(err.Error(), tt.wantErrNames) {
				t.Errorf("DiscussionSpec(slug=%q, missingDir=%v) error = %q; want it to name %q", tt.slug, tt.missingDir, err.Error(), tt.wantErrNames)
			}
		})
	}
}

// TestDiscussionSpec_ReadsStencilAtCallTime verifies DiscussionSpec's prompt reflects an on-disk edit
// to the stencils directory rather than the embedded default, per the runtime-read-not-embed Shared
// Decision.
//
//testtiming:keep pins that an on-disk edit of the stencil reaches the prompt, which the covering spec test never edits
func TestDiscussionSpec_ReadsStencilAtCallTime(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}

	reg, err := modelspec.LoadRegistry(t.TempDir())
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

	spec, err := DiscussionSpec(layout, stencilsDir, "", cfg, reg, "add-json-flag", false)
	if err != nil {
		t.Fatalf("DiscussionSpec(...) = _, %v; want nil error", err)
	}
	if !strings.Contains(spec.Prompt, marker) {
		t.Errorf("DiscussionSpec(...).Prompt does not contain the on-disk-edited marker %q; want the modified stencil, not the embedded default, to reach the composed prompt", marker)
	}
}

// TestDiscussionSpec_PatternDirective proves DiscussionSpec injects the designer directive when
// PATTERN.md exists at the worktree root, and renders none without it.
// It uses a non-"." AnchorRel, so the root the directive reads is told apart from the anchor path.
//
//testtiming:keep pins the designer directive text and its place before Step 1 for the discussion prompt, which the covering tests do not assert
func TestDiscussionSpec_PatternDirective(t *testing.T) {
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}
	reg, err := modelspec.LoadRegistry(t.TempDir())
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

			spec, err := DiscussionSpec(layout, newTestStencilsDir(t), "", cfg, reg, "add-json-flag", false)
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
	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	const operatorBan = "A question to the operator in your pane is never the way forward."
	build := map[string]func(parent string) (shuttleengine.Spec, error){
		"discussion-interactive": func(parent string) (shuttleengine.Spec, error) {
			return DiscussionSpec(layout, newTestStencilsDir(t), parent, cfg, reg, "add-json-flag", false)
		},
		"discussion-autonomous": func(parent string) (shuttleengine.Spec, error) {
			return DiscussionSpec(layout, newTestStencilsDir(t), parent, cfg, reg, "add-json-flag", true)
		},
		"plan": func(parent string) (shuttleengine.Spec, error) {
			return PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), parent, cfg, reg)
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
