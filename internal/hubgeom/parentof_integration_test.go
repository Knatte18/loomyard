//go:build tier1

// parentof_integration_test.go drives ParentOf over one real hub from hubforge:
// pairs forked from the prime and from another pair, a record naming a parent pair that is gone, a record the prime's own branch or worktree name reads through, and the board's beam chain for a slug with no pair.

package hubgeom_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestParentOf pins that the origin record decides the parent whenever a pair has one and the local board decides only for a slug with no pair.
// A pair forked from the prime reads the prime, and one forked from pair b reads b, also when b's pair is gone from this machine.
// A record with no parent worktree reads the prime though its board entry is a beam's member, and so does a record naming the prime's branch under another worktree name.
// A slug with no pair reads the beam its board entry's project chain reaches, and a pair forked from the prime reads the prime though its board entry is a member of a beam.
func TestParentOf(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	const (
		fromPrime  = "po-from-prime"
		beamPair   = "po-b"
		fromBeam   = "po-from-b"
		goneParent = "po-gone-parent"
		noParent   = "po-no-parent"
		otherName  = "po-other-name"
		member     = "po-member"
		unspawned  = "po-unspawned"
	)
	hubforge.AddPairWith(t, h, fromPrime, fabricengine.AddOptions{SkipPush: true})
	hubforge.AddPairWith(t, h, beamPair, fabricengine.AddOptions{SkipPush: true})
	beamLocation, err := lyxcwd.Resolve(fabricengine.WorktreePath(l, beamPair))
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(beam pair): %v", err)
	}
	if _, err := h.Topology.Add(beamLocation, fromBeam, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q) from the beam pair: %v", fromBeam, err)
	}
	hubforge.AddPairWith(t, h, goneParent, fabricengine.AddOptions{SkipPush: true})
	hubforge.AddPairWith(t, h, noParent, fabricengine.AddOptions{SkipPush: true})
	hubforge.AddPairWith(t, h, otherName, fabricengine.AddOptions{SkipPush: true})
	hubforge.AddPairWith(t, h, member, fabricengine.AddOptions{SkipPush: true})

	primeRecord, found, err := fabricengine.ReadOriginFor(l, fromPrime)
	if err != nil || !found {
		t.Fatalf("ReadOriginFor(%q) = found %v, error %v; want a record", fromPrime, found, err)
	}
	rewrites := map[string]fabricengine.Origin{
		goneParent: {ParentBranch: "gone-branch", ParentWorktree: "po-removed"},
		noParent:   {ParentBranch: "gone-branch"},
		otherName:  {ParentBranch: primeRecord.ParentBranch, ParentWorktree: "po-renamed-prime"},
	}
	for slug, origin := range rewrites {
		if err := fabricengine.WriteOrigin(nil, l, slug, origin); err != nil {
			t.Fatalf("WriteOrigin(%q): %v", slug, err)
		}
	}

	board := `{"version":1,"entries":[` +
		`{"id":0,"slug":"po-beam","title":"Beam","kind":"project","labels":["bug"],"beam":true},` +
		`{"id":1,"slug":"` + unspawned + `","title":"Unspawned","kind":"task","labels":["bug"],"project":"po-beam"},` +
		`{"id":2,"slug":"` + member + `","title":"Member","kind":"task","labels":["bug"],"project":"po-beam"},` +
		`{"id":3,"slug":"` + noParent + `","title":"No parent","kind":"task","labels":["bug"],"project":"po-beam"}]}`
	if err := os.WriteFile(filepath.Join(h.BoardDir(), "board.json"), []byte(board), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}

	tests := []struct {
		name string
		slug string
		want string
	}{
		{"PairForkedFromThePrime", fromPrime, ""},
		{"PairForkedFromAnotherPair", fromBeam, beamPair},
		{"RecordNamingARemovedParentPair", goneParent, "po-removed"},
		{"RecordWithNoParentWorktree", noParent, ""},
		{"RecordNamingThePrimesBranchUnderAnotherWorktreeName", otherName, ""},
		{"SlugWithNoPairReadsTheBoardsBeam", unspawned, "po-beam"},
		{"PairForkedFromThePrimeReadsTheRecordOverTheBoard", member, ""},
		{"SlugWithNoPairAndNoEntry", "po-ghost", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := hubgeom.ParentOf(l, tt.slug)
			if err != nil {
				t.Fatalf("ParentOf(%q) error = %v; want nil", tt.slug, err)
			}
			if got != tt.want {
				t.Errorf("ParentOf(%q) = %q; want %q", tt.slug, got, tt.want)
			}
		})
	}
}
