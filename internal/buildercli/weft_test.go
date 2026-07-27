// weft_test.go asserts builderWeftPathspec's exclusion set — the machine-local
// runtime artifacts (advisory *.lock files and the pause flag) must be excluded
// from every builder weft commit so they never leak into durable weft history
// or materialize on another machine's weft pull — and weftCommit's guard
// ordering: the WEFT_SKIP_GIT bypass short-circuits before fabricengine.New's
// stat validation, while a non-bypass call surfaces New's typed
// ErrMissingPath when the host/weft pair is absent.

package buildercli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestBuilderWeftPathspec_ExcludesRuntimeArtifacts proves the pathspec every
// builder weft commit stages under excludes both the advisory *.lock files
// and the pause flag, at each layout.RelPath shape -- and that every
// exclusion is ANCHORED under the scoped _lyx base rather than spelled with a
// leading wildcard. The anchoring is not cosmetic: git treats a leading-"*"
// pattern with no further wildcard as a one-star pathspec that
// false-positive-matches the intermediate directories leading to a
// multi-segment positive pathspec, which prunes the whole subtree and turns
// the weft commit into a silent no-op. Note this test can only prove the
// SHAPE of the pathspec; that real git honours it is proved by
// weft_integration_test.go's TestWeftCommit_CommitsAtEveryRelPathDepth.
func TestBuilderWeftPathspec_ExcludesRuntimeArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		base    string
	}{
		{name: "nested worktree (relPath set)", relPath: "wts/some-task", base: "wts/some-task/_lyx"},
		{name: "worktree root (relPath dot)", relPath: ".", base: "_lyx"},
		{name: "weft-root worktree (relPath empty)", relPath: "", base: "_lyx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathspec := builderWeftPathspec(&hubgeometry.Layout{RelPath: tt.relPath})

			wantExcludes := []string{
				":(exclude)" + tt.base + "/*.lock",
				":(exclude)" + tt.base + "/builder/" + builderengine.PauseFlagName,
			}
			for _, want := range wantExcludes {
				if !containsString(pathspec, want) {
					t.Errorf("builderWeftPathspec(relPath=%q) = %v; want it to contain %q", tt.relPath, pathspec, want)
				}
			}

			for _, entry := range pathspec {
				if strings.HasPrefix(entry, ":(exclude)*") {
					t.Errorf("builderWeftPathspec(relPath=%q) has unanchored exclusion %q; every exclusion must be anchored under %q", tt.relPath, entry, tt.base)
				}
			}
		})
	}
}

// TestWeftCommit_SkipGitBypassNeedsNoWeftWorktree pins the guard ordering
// weftCommit's own block comment documents: with WEFT_SKIP_GIT=1 the bypass
// must short-circuit BEFORE fabricengine.New's stat-based path validation,
// so the CI/test bypass never requires a weft worktree (or even the host
// worktree) to exist on disk. A regression hoisting New above the guard
// turns every bypassed CI run into an ErrMissingPath failure.
func TestWeftCommit_SkipGitBypassNeedsNoWeftWorktree(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	t.Setenv("WEFT_SKIP_PUSH", "")

	// Neither the host worktree nor its -weft sibling exists on disk.
	hub := t.TempDir()
	layout := &hubgeometry.Layout{
		Hub:          hub,
		WorktreeRoot: filepath.Join(hub, "host"),
		Cwd:          filepath.Join(hub, "host"),
		RelPath:      ".",
	}

	committed, err := weftCommit(layout, "bypass probe")
	if err != nil {
		t.Fatalf("weftCommit() error = %v; want nil, the bypass must never touch the filesystem or git", err)
	}
	if committed {
		t.Error("weftCommit() committed = true; want false in bypass mode")
	}
}

// TestWeftCommit_NonBypassValidatesPairPaths proves the counterpart of the
// bypass test above: without WEFT_SKIP_GIT, weftCommit constructs the
// fabric handle and surfaces fabricengine's typed ErrMissingPath when the
// pair is absent -- evidence New runs, and runs only in non-bypass mode.
func TestWeftCommit_NonBypassValidatesPairPaths(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")

	hub := t.TempDir()
	layout := &hubgeometry.Layout{
		Hub:          hub,
		WorktreeRoot: filepath.Join(hub, "host"),
		Cwd:          filepath.Join(hub, "host"),
		RelPath:      ".",
	}

	committed, err := weftCommit(layout, "missing-pair probe")
	if committed {
		t.Error("weftCommit() committed = true; want false, no repo exists to commit to")
	}
	var missing *fabricengine.ErrMissingPath
	if !errors.As(err, &missing) {
		t.Fatalf("weftCommit() error = %v; want a *fabricengine.ErrMissingPath from New's stat validation", err)
	}
}

// containsString reports whether haystack contains needle.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
