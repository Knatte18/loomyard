// sync_test.go asserts fabricSync's guard ordering: the WEFT_SKIP_GIT bypass short-circuits before
// fabricengine.Open's stat validation, while a non-bypass call surfaces Open's typed ErrMissingPath
// when the repo pair is absent.
// Pathspec-shape coverage now lives in sync_integration_test.go, which proves the exclude-file
// transients stay uncommitted through a real git repo rather than asserting a pathspec string shape
// against a since-deleted helper.

package buildercli

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestFabricSync_SkipGitBypassNeedsNoWeftWorktree proves the WEFT_SKIP_GIT bypass short-circuits
// before stat validation, so CI never needs the worktree on disk.
func TestFabricSync_SkipGitBypassNeedsNoWeftWorktree(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	t.Setenv("WEFT_SKIP_PUSH", "")

	// Neither side of the fabric repo pair exists on disk.
	hub := t.TempDir()
	layout := &lyxcwd.Location{HubPath: hub, WorktreeName: filepath.Base(filepath.Join(hub, "host")), AnchorRel: "."}

	committed, err := fabricSync(layout, "bypass probe")
	if err != nil {
		t.Fatalf("fabricSync() error = %v; want nil, the bypass must never touch the filesystem or git", err)
	}
	if committed {
		t.Error("fabricSync() committed = true; want false in bypass mode")
	}
}

// TestFabricSync_NonBypassValidatesPairPaths proves that without WEFT_SKIP_GIT, fabricSync
// validates the worktree pair exists.
func TestFabricSync_NonBypassValidatesPairPaths(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")

	hub := t.TempDir()
	layout := &lyxcwd.Location{HubPath: hub, WorktreeName: filepath.Base(filepath.Join(hub, "host")), AnchorRel: "."}

	committed, err := fabricSync(layout, "missing-pair probe")
	if committed {
		t.Error("fabricSync() committed = true; want false, no repo exists to commit to")
	}
	var missing *fabricengine.ErrMissingPath
	if !errors.As(err, &missing) {
		t.Fatalf("fabricSync() error = %v; want a *fabricengine.ErrMissingPath from Open's stat validation", err)
	}
}

// containsString reports whether haystack contains needle. Shared with
// sync_integration_test.go's exclude-file assertions.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
