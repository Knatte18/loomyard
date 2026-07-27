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
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestBuilderWeftPathspec_ExcludesRuntimeArtifacts proves the pathspec every
// builder weft commit stages under excludes both the advisory *.lock files and
// the pause flag, regardless of whether layout.RelPath prefixes the _lyx path.
func TestBuilderWeftPathspec_ExcludesRuntimeArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
	}{
		{name: "nested worktree (relPath set)", relPath: "wts/some-task"},
		{name: "weft-root worktree (relPath empty)", relPath: ""},
	}

	wantExcludes := []string{
		":(exclude)*.lock",
		":(exclude)*/builder/" + builderengine.PauseFlagName,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathspec := builderWeftPathspec(&hubgeometry.Layout{RelPath: tt.relPath})

			for _, want := range wantExcludes {
				if !containsString(pathspec, want) {
					t.Errorf("builderWeftPathspec(relPath=%q) = %v; want it to contain %q", tt.relPath, pathspec, want)
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
