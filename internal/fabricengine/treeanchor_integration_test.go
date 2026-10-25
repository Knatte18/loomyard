//go:build tier1

// treeanchor_integration_test.go pins the one rule that derives the anchor of a branch recording none:
// the committed marker, else the shallowest directory holding a wired name, else the caller's fallback.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestTreeAnchor covers each answer of the rule over a plain repository holding the listed files.
func TestTreeAnchor(t *testing.T) {
	t.Parallel()

	const fallback = "the-fallback"
	wired := []string{"_lyx", ".lyx"}

	tests := []struct {
		name    string
		files   map[string]string
		want    string
		wantErr bool
	}{
		{"CommittedMarkerWins", map[string]string{lyxcwd.AnchorFileName: "sub\n", "_lyx/config.yaml": "x"}, "sub", false},
		{"WiredNameAtTheRoot", map[string]string{"_lyx/config.yaml": "x"}, ".", false},
		{"TieAtOneDepthGoesToTheSmallerPath", map[string]string{"b/x/_lyx/config.yaml": "x", "a/y/_lyx/config.yaml": "x"}, filepath.FromSlash("a/y"), false},
		{"ShallowerWiredNameWins", map[string]string{"a/b/_lyx/config.yaml": "x", "d/.lyx/state": "x"}, "d", false},
		{"NoWiredNameAnswersTheFallback", map[string]string{"docs/readme": "x"}, fallback, false},
		{"BlankCommittedMarkerCountsAsNone", map[string]string{lyxcwd.AnchorFileName: " \n", "sub/_lyx/config.yaml": "x"}, "sub", false},
		{"UnusableCommittedMarkerIsAnError", map[string]string{lyxcwd.AnchorFileName: "../out\n"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := fabricengine.NewPlainWarpRepoForTest(t)
			for rel, content := range tt.files {
				gitkit.CommitFile(t, repo, rel, content, "add "+rel)
			}

			got, err := fabricengine.TreeAnchorForTest(repo, "HEAD", wired, fallback)
			if (err != nil) != tt.wantErr {
				t.Fatalf("TreeAnchor(HEAD) error = %v; want error: %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("TreeAnchor(HEAD) = %q; want %q", got, tt.want)
			}
		})
	}
}
