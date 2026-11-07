// checkoutstate_integration_test.go covers CheckoutStates and RecordsPathsChangedSince against a wired hub fixture.
//
// The package's single TestMain lives in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckoutStates runs the checkout-state and weft-change rows over one hub, in order.
// Its rows dirty the same two checkouts and run in sequence, so none is parallel.
func TestCheckoutStates(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weft := fabricengine.RecordsWorktree(l)

	t.Run("CleanPairReadsHeadsAndNoPaths", func(t *testing.T) {
		code, records, err := fabricengine.CheckoutStates(l)
		if err != nil {
			t.Fatalf("CheckoutStates error = %v", err)
		}
		if code.Root != l.WorktreePath() || code.Head != gitkit.RevParse(t, l.WorktreePath(), "HEAD") {
			t.Errorf("code state = %+v; want root %s and the warp HEAD", code, l.WorktreePath())
		}
		if records.Root != weft || records.Head != gitkit.RevParse(t, weft, "HEAD") {
			t.Errorf("records state = %+v; want root %s and the weft HEAD", records, weft)
		}
		for name, list := range map[string][]string{
			"code.Changed": code.Changed, "code.Untracked": code.Untracked,
			"records.Changed": records.Changed, "records.Untracked": records.Untracked,
		} {
			if list == nil || len(list) != 0 {
				t.Errorf("%s = %#v; want an empty, non-nil list", name, list)
			}
		}
	})

	t.Run("DirtyFilesAreListedPerSide", func(t *testing.T) {
		tracked := gitkit.LsFiles(t, l.WorktreePath())[0]
		if err := os.WriteFile(filepath.Join(l.WorktreePath(), filepath.FromSlash(tracked)), []byte("modified\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(weft, "checkout-probe.txt"), []byte("new\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		// A file the weft's ignore rules cover reads in neither list.
		exclude := gitkit.Git(t, weft, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
		existing, err := os.ReadFile(exclude)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if err := os.WriteFile(exclude, append(existing, []byte("\nignored-probe.txt\n")...), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(weft, "ignored-probe.txt"), []byte("ignored\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		code, records, err := fabricengine.CheckoutStates(l)
		if err != nil {
			t.Fatalf("CheckoutStates error = %v", err)
		}
		if !slices.Equal(code.Changed, []string{tracked}) || len(code.Untracked) != 0 {
			t.Errorf("code state = %+v; want Changed [%s] and no untracked path", code, tracked)
		}
		if len(records.Changed) != 0 || !slices.Equal(records.Untracked, []string{"checkout-probe.txt"}) {
			t.Errorf("records state = %+v; want no changed path and Untracked [checkout-probe.txt]", records)
		}
	})

	t.Run("RecordsPathsChangedSinceNamesTheCommittedFile", func(t *testing.T) {
		before := gitkit.RevParse(t, weft, "HEAD")
		const rel = "_lym/checkout-probe/committed.txt"
		writeAnchored(t, l, rel, "committed")
		rec := fabricengine.NewMutations(l.HubPath)
		if _, committed, err := fabricengine.CommitAnchoredPaths(rec, l, []string{rel}, "probe", fabricengine.SyncOptions{}); err != nil || !committed {
			t.Fatalf("CommitAnchoredPaths committed = %v, err = %v", committed, err)
		}

		tests := []struct {
			name string
			rev  string
			want []string
		}{
			{"EarlierHead", before, []string{rel}},
			{"CurrentHead", gitkit.RevParse(t, weft, "HEAD"), []string{}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := fabricengine.RecordsPathsChangedSince(l, tt.rev)
				if err != nil {
					t.Fatalf("RecordsPathsChangedSince(%s) error = %v", tt.rev, err)
				}
				if got == nil || !slices.Equal(got, tt.want) {
					t.Errorf("RecordsPathsChangedSince(%s) = %#v; want %#v", tt.rev, got, tt.want)
				}
			})
		}
	})
}
