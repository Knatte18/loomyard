// weft_test.go asserts builderWeftPathspec's exclusion set: the machine-local
// runtime artifacts (advisory *.lock files and the pause flag) must be excluded
// from every builder weft commit so they never leak into durable weft history
// or materialize on another machine's weft pull.

package buildercli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
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

// containsString reports whether haystack contains needle.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
