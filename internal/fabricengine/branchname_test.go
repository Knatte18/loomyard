// branchname_test.go — unit tests for WeftBranchName's uniform derivation.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestWeftBranchName covers the uniform <host>/<host>-weft scheme across the
// primary branch, a prefixed task branch, and a plain (empty-prefix) slug.
func TestWeftBranchName(t *testing.T) {
	tests := []struct {
		name       string
		hostBranch string
		want       string
	}{
		{"primary", "main", "main-weft"},
		{"prefixed_task_branch", "hanf/foo", "hanf/foo-weft"},
		{"empty_prefix_plain_slug", "foo", "foo-weft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fabricengine.WeftBranchName(tt.hostBranch)
			if got != tt.want {
				t.Errorf("WeftBranchName(%q) = %q; want %q", tt.hostBranch, got, tt.want)
			}
		})
	}
}

// TestWeftBranchName_RoundTripsWithWeftHostSlug asserts that
// hubgeometry.WeftHostSlug, the documented inverse, recovers the original host
// branch from every WeftBranchName output.
func TestWeftBranchName_RoundTripsWithWeftHostSlug(t *testing.T) {
	hostBranches := []string{"main", "hanf/foo", "foo"}
	for _, host := range hostBranches {
		weft := fabricengine.WeftBranchName(host)
		gotHost, ok := hubgeometry.WeftHostSlug(weft)
		if !ok {
			t.Errorf("WeftHostSlug(%q) ok = false; want true", weft)
			continue
		}
		if gotHost != host {
			t.Errorf("WeftHostSlug(%q) = %q; want %q", weft, gotHost, host)
		}
	}
}
