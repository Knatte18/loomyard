// discussion_test.go — untagged Tier-1 unit tests for DiscussionSpec. Pure
// Go over an in-memory Config and a temp-dir modelspec registry; no live
// hub, mux, or network involved.

package loomengine

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// TestDiscussionSpec verifies DiscussionSpec's field mapping for both
// autonomous values against a hand-built Layout, an in-memory Config, and
// the built-in modelspec registry (no models.yaml present).
func TestDiscussionSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
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
		name            string
		autonomous      bool
		wantInteractive bool
	}{
		{"Interactive", false, true},
		{"Autonomous", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := DiscussionSpec(layout, cfg, reg, "add-json-flag", tt.autonomous)
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

// TestDiscussionSpec_EmptySlug verifies an empty slug is rejected with a
// non-nil error rather than silently producing a Spec with no board-read
// target.
func TestDiscussionSpec_EmptySlug(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	cfg := Config{Discussion: "opus[effort=high]", DiscussionTimeoutMin: 480}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	if _, err := DiscussionSpec(layout, cfg, reg, "", false); err == nil {
		t.Fatal("DiscussionSpec(..., slug=\"\", ...) = _, nil; want non-nil error")
	}
}
