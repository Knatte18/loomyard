// discussion_test.go — untagged Tier-1 unit tests for DiscussionSpec.
// Pure Go over an in-memory Config and a temp-dir modelspec registry;
// no live hub, reed, or network involved.

package loomengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// TestDiscussionSpec verifies DiscussionSpec's field mapping for both autonomous values.
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
			spec, err := DiscussionSpec(layout, newTestStencilsDir(t), cfg, reg, "add-json-flag", tt.autonomous)
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

// TestDiscussionSpec_EmptySlug verifies an empty slug is rejected.
func TestDiscussionSpec_EmptySlug(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	if _, err := DiscussionSpec(layout, newTestStencilsDir(t), cfg, reg, "", false); err == nil {
		t.Fatal("DiscussionSpec(..., slug=\"\", ...) = _, nil; want non-nil error")
	}
}

// TestDiscussionSpec_ReadsStencilAtCallTime verifies DiscussionSpec's prompt reflects an on-disk edit
// to the stencils directory rather than the embedded default, per the runtime-read-not-embed Shared
// Decision.
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

	spec, err := DiscussionSpec(layout, stencilsDir, cfg, reg, "add-json-flag", false)
	if err != nil {
		t.Fatalf("DiscussionSpec(...) = _, %v; want nil error", err)
	}
	if !strings.Contains(spec.Prompt, marker) {
		t.Errorf("DiscussionSpec(...).Prompt does not contain the on-disk-edited marker %q; want the modified stencil, not the embedded default, to reach the composed prompt", marker)
	}
}

// TestDiscussionSpec_MissingStencilsDirIsHardError verifies DiscussionSpec returns an error naming
// the missing stencil, rather than silently falling back to the embedded default, when stencilsDir
// does not exist, per the missing-board-is-a-hard-error Shared Decision.
func TestDiscussionSpec_MissingStencilsDirIsHardError(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	missingStencilsDir := filepath.Join(t.TempDir(), "does-not-exist")
	_, err = DiscussionSpec(layout, missingStencilsDir, cfg, reg, "add-json-flag", false)
	if err == nil {
		t.Fatal("DiscussionSpec(..., stencilsDir=<missing>, ...) = _, nil; want non-nil error")
	}
	if !strings.Contains(err.Error(), "loom-template-discussion") {
		t.Errorf("DiscussionSpec(..., stencilsDir=<missing>, ...) error = %q; want it to name the missing stencil %q", err.Error(), "loom-template-discussion")
	}
}
