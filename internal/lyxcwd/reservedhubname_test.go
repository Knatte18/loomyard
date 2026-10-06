// reservedhubname_test.go pins that two names the hub once reserved are no longer reserved.
// BoardDir, HubPath and IsReservedHubName live in internal/fabricengine; their full coverage is in
// fabricengine/junctionnames_test.go, and this file keeps only the inverse-of-the-old-truth rows.

package lyxcwd_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestIsReservedHubName_RetiredNamesAreNotReserved pins the inverse of the old truth: _pattern has
// collapsed into _lyx, and raddle has converged on an anchor-level `_lyx/raddle/` design with no
// hub-level presence, so a worktree slug named "_pattern" or "_raddle" is no longer refused on
// reserved-name grounds.
// nil is passed for junctionNames rather than a fixture naming the slug: with pathspec empty the
// production call site supplies no junction names at all, and injecting the slug through
// junctionNames would make the assertion trivially reserved again and prove nothing.
func TestIsReservedHubName_RetiredNamesAreNotReserved(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"_pattern", "_raddle"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := fabricengine.IsReservedHubName(name, nil); got {
				t.Errorf("IsReservedHubName(%q, nil) = %v; want false", name, got)
			}
		})
	}
}
