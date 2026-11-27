// hubpaths_test.go covers the hub path functions' literal joins and the hub-level name predicates.

package fabriclayout

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The HubScratchDir and StencilsDir rows together hold where the scratch tree sits.
// It is a sibling of the `_lym` component above the stencils directory, not nested under it.
func TestHubPaths(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	bolt := "/h/" + lymdirs.BoltDirName
	scratch := bolt + "/.lym"
	slash := filepath.FromSlash
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"BoltDir", BoltDir("/h"), slash(bolt)},
		{"BoardDir", BoardDir("/h"), slash(bolt)},
		{"StencilsDir", StencilsDir("/h"), slash(bolt + "/_lym/stencils")},
		{"SpecsDir", SpecsDir("/h"), slash(bolt + "/_lym/specs")},
		{"StencilsSubtreeRel", StencilsSubtreeRel(), "_lym/stencils"},
		{"SpecsSubtreeRel", SpecsSubtreeRel(), "_lym/specs"},
		{"HubScratchDir", HubScratchDir("/h"), slash(scratch)},
		{"AdoptScratchDir", AdoptScratchDir("/h"), slash(scratch + "/adopt")},
		{"ClaudeAccountsDir", ClaudeAccountsDir("/h"), slash(scratch + "/claude")},
		{"FrontTaskScratchDir", FrontTaskScratchDir("/h"), slash(scratch + "/fronttask")},
		{"HubLogsDir", HubLogsDir("/h"), slash(scratch + "/logs")},
		{"LogArchiveDir", LogArchiveDir("/h"), slash(scratch + "/log-archive")},
		{"RunProcDir", RunProcDir("/h"), slash(scratch + "/runproc")},
		{"HubPath", HubPath("/repos", "loomyard"), slash("/repos/loomyard-LYMHUB")},
		{"OrphanFirstSightPath", OrphanFirstSightPath("/h"), slash(scratch + "/orphan/first-sight.json")},
		{"TeardownLockPath", TeardownLockPath("/h", "s"), slash(scratch + "/teardown/s.lock")},
		{"HandOverMarkPath", HandOverMarkPath("/h", "s"), slash(scratch + "/hand-over/s.json")},
		{"RecordsLockDirPath", RecordsLockDirPath("/w"), slash("/w/.weft")},
		{"MoveRecordPath", MoveRecordPath("/w"), slash("/w/.weft/anchor-move.json")},
		{"OriginRecordRel", OriginRecordRel(), slash("_lym/fabric/origin.json")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("%s = %q; want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// Every geometry-owned hub-level name is reserved, and so are the caller's junctionNames.
// The geometry-owned names are the hub-structural ones and two directory sets.
// Those sets are StructuralCommittedDirs() and StructuralNeverCommittedDirs().
// Ordinary slugs and near-misses are not.
//
//testtiming:keep the reserved hub-entry predicate slug validation gates on, over every geometry-owned name and near-misses; coverage of its blocks by other tests does not show an assertion of this
func TestIsReservedHubName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		junctionNames []string
		want          bool
	}{
		{"lym dir", "_lym", nil, true},
		{"raddle dir", "_raddle", nil, false},
		{"board dir", BoltDirName, nil, true},
		{"portals dir", "_portals", nil, true},
		{"launchers dir", "_launchers", nil, true},
		{"ordinary slug", "my-task", nil, false},
		{"underscore-prefixed but unreserved", "_mytask", nil, false},
		{"compound near-miss", "_boltroom", nil, false},
		{"empty", "", nil, false},
		{"junction-only name", "_custom", []string{"_custom"}, true},
		{"unrelated to the junction names", "_other", []string{"_custom"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsReservedHubName(tt.input, tt.junctionNames); got != tt.want {
				t.Errorf("IsReservedHubName(%q, %v) = %v; want %v", tt.input, tt.junctionNames, got, tt.want)
			}
		})
	}

	// `.lym` is absent from this set although IsReservedHubName reserves it:
	// listing it would keep it from being wired into each worktree.
	wantReserved := []string{lymdirs.BoltDirName, "_portals", "_launchers"}
	if got := HubReservedNames(); !slices.Equal(got, wantReserved) {
		t.Errorf("HubReservedNames() = %v; want %v", got, wantReserved)
	}
}

func TestInWorktreeStaging(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"NestedUnderStagingDirectory", "/hub/" + WorktreeStagingPrefix + "a1b2c3/feature", true},
		{"StagingDirectoryItself", "/hub/" + WorktreeStagingPrefix + "a1b2c3", true},
		{"PrefixOnlyName", "/hub/" + strings.TrimSuffix(WorktreeStagingPrefix, "-") + "/feature", false},
		{"PrefixInsideAName", "/hub/not" + WorktreeStagingPrefix + "x/feature", false},
		{"UnrelatedPath", "/hub/feature", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := InWorktreeStaging(tt.path); got != tt.want {
				t.Errorf("InWorktreeStaging(%q) = %v; want %v", tt.path, got, tt.want)
			}
		})
	}
}
