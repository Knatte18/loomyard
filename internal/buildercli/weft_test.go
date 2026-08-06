// weft_test.go asserts weftCommit's guard ordering: the WEFT_SKIP_GIT bypass short-circuits before fabricengine.New's stat validation, while a non-bypass call surfaces New's typed ErrMissingPath when the host/weft pair is absent.
// Pathspec-shape coverage now lives in weft_integration_test.go, which proves the exclude-file transients stay uncommitted through a real git repo rather than asserting a pathspec string shape against a since-deleted helper.

package buildercli

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestWeftCommit_SkipGitBypassNeedsNoWeftWorktree proves the WEFT_SKIP_GIT bypass short-circuits before stat validation, so CI never needs the worktree on disk.
func TestWeftCommit_SkipGitBypassNeedsNoWeftWorktree(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	t.Setenv("WEFT_SKIP_PUSH", "")

	// Neither the host worktree nor its -weft sibling exists on disk.
	hub := t.TempDir()
	layout := &lyxcwd.Location{HubPath: hub, WorktreeName: filepath.Base(filepath.Join(hub, "host")), AnchorRel: "."}

	committed, err := weftCommit(layout, "bypass probe")
	if err != nil {
		t.Fatalf("weftCommit() error = %v; want nil, the bypass must never touch the filesystem or git", err)
	}
	if committed {
		t.Error("weftCommit() committed = true; want false in bypass mode")
	}
}

// TestWeftCommit_NonBypassValidatesPairPaths proves that without WEFT_SKIP_GIT, weftCommit validates the worktree pair exists.
func TestWeftCommit_NonBypassValidatesPairPaths(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")

	hub := t.TempDir()
	layout := &lyxcwd.Location{HubPath: hub, WorktreeName: filepath.Base(filepath.Join(hub, "host")), AnchorRel: "."}

	committed, err := weftCommit(layout, "missing-pair probe")
	if committed {
		t.Error("weftCommit() committed = true; want false, no repo exists to commit to")
	}
	var missing *fabricengine.ErrMissingPath
	if !errors.As(err, &missing) {
		t.Fatalf("weftCommit() error = %v; want a *fabricengine.ErrMissingPath from New's stat validation", err)
	}
}

// containsString reports whether haystack contains needle. Shared with
// weft_integration_test.go's exclude-file assertions.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
