//go:build integration

// configchanges_integration_test.go proves ReadConfigChanges reports a task's changes to the per-worktree config files it is told about, read from the task's weft branch since its fork point.
//
// Package fabricengine_test to reuse hubforge.NewHub and the add_rollback_adopt_test.go helper mustRecordsRepoRoot;
// it shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestReadConfigChanges forks a pair from the prime, commits on the pair's weft branch and calls ReadConfigChanges against the prime's branch.
func TestReadConfigChanges(t *testing.T) {
	t.Parallel()

	loomRel := configengine.ConfigFileRel("loom")
	boardRel := configengine.ConfigFileRel("board")

	tests := []struct {
		name   string
		anchor string
		// rels is the anchor-relative file set handed to ReadConfigChanges.
		rels []string
		// taskFiles are anchor-relative files committed on the pair's weft branch, one commit each.
		taskFiles []string
		// outsideAnchorFile is a repository-root-relative file also committed on the pair's weft branch.
		outsideAnchorFile string
		// parentFileAfterFork is an anchor-relative file committed on the parent's weft branch after the fork.
		parentFileAfterFork string
		parentBranch        string
		// wantFiles are anchor-relative paths the call reports.
		wantFiles []string
		wantErr   bool
		// wantZero expects the zero ConfigChanges, with no git run.
		wantZero bool
	}{
		{
			name:         "an empty file set returns the zero value without reading either branch",
			anchor:       ".",
			parentBranch: "ghost",
			wantZero:     true,
		},
		{
			name:         "a listed config file changed is reported",
			anchor:       ".",
			rels:         []string{loomRel},
			taskFiles:    []string{loomRel},
			parentBranch: "main",
			wantFiles:    []string{loomRel},
		},
		{
			name:         "a config file not listed is not reported",
			anchor:       ".",
			rels:         []string{loomRel},
			taskFiles:    []string{boardRel},
			parentBranch: "main",
		},
		{
			name:         "no change reports no files",
			anchor:       ".",
			rels:         []string{loomRel},
			parentBranch: "main",
		},
		{
			name:                "a commit on the parent after the fork does not appear",
			anchor:              ".",
			rels:                []string{loomRel},
			parentFileAfterFork: loomRel,
			parentBranch:        "main",
		},
		{
			name:         "a parent whose weft branch is missing is an error naming both branches",
			anchor:       ".",
			rels:         []string{loomRel},
			taskFiles:    []string{loomRel},
			parentBranch: "ghost",
			wantErr:      true,
		},
		{
			name:              "a subpath anchor scopes the pathspec under the anchor",
			anchor:            "backend",
			rels:              []string{loomRel},
			taskFiles:         []string{loomRel},
			outsideAnchorFile: loomRel,
			parentBranch:      "main",
			wantFiles:         []string{loomRel},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			const slug = "feature"
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: tc.anchor})
			l := h.Location
			weftRoot := mustRecordsRepoRoot(t, l)
			forkPoint := gitkit.RevParse(t, weftRoot, fabricengine.RecordsBranchName("main"))

			hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

			pairWeft := h.PairRecordsSibling(slug)
			for _, rel := range tc.taskFiles {
				gitkit.CommitFile(t, pairWeft, filepath.Join(l.AnchorRel, rel), "task change", "task change")
			}
			if tc.outsideAnchorFile != "" {
				gitkit.CommitFile(t, pairWeft, tc.outsideAnchorFile, "outside the anchor", "outside the anchor")
			}
			if tc.parentFileAfterFork != "" {
				gitkit.CommitFile(t, weftRoot, filepath.Join(l.AnchorRel, tc.parentFileAfterFork), "parent change", "parent change")
			}

			got, err := fabricengine.ReadConfigChanges(l, slug, tc.parentBranch, tc.rels)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ReadConfigChanges() = %+v, nil; want an error", got)
				}
				for _, branch := range []string{fabricengine.RecordsBranchName(slug), fabricengine.RecordsBranchName(tc.parentBranch)} {
					if !strings.Contains(err.Error(), branch) {
						t.Errorf("ReadConfigChanges() error = %q; want it to name %q", err, branch)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadConfigChanges() error = %v", err)
			}

			if tc.wantZero {
				if len(got.Files) != 0 || got.Base != "" || got.Tip != "" {
					t.Errorf("ReadConfigChanges() = %+v; want the zero value", got)
				}
				return
			}

			var want []string
			for _, rel := range tc.wantFiles {
				want = append(want, filepath.ToSlash(filepath.Join(l.AnchorRel, rel)))
			}
			if !slices.Equal(got.Files, want) {
				t.Errorf("Files = %v; want %v", got.Files, want)
			}
			if got.Base != forkPoint {
				t.Errorf("Base = %s; want the fork point %s", got.Base, forkPoint)
			}
			if wantTip := gitkit.RevParse(t, weftRoot, fabricengine.RecordsBranchName(slug)); got.Tip != wantTip {
				t.Errorf("Tip = %s; want the task branch tip %s", got.Tip, wantTip)
			}
		})
	}
}
