//go:build tier1

// notify_integration_test.go proves PrimeOrchStrand over a real hub: from a task worktree it names the prime's orch strand, and answers not recorded when the prime records none.
// It also proves the destination of a run's notice up a beam chain: a live beam orch, a stopped one, a pair forked from the prime and a destination with no strand recorded.

package orchcli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestNotifyPrime_QueuesInThePrimeOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		strand     string
		reedStrand string
		wantName   string
		wantRec    bool
	}{
		{"a GUID the reed state holds is named by the reed state", "g1", "g1", "tst:orch-held", true},
		{"no GUID recorded is not recorded", "", "", "", false},
		{"a GUID the reed state does not hold takes the fixed orch name", "g2", "g1", "tst:orch", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, "notify-task")
			task, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("notify-task"))
			if err != nil {
				t.Fatalf("ResolveWorktree task: %v", err)
			}
			if tt.strand != "" {
				if err := orchengine.SaveState(PrimePaths(h.Location), orchengine.State{Phase: orchengine.PhaseIdle, Strand: tt.strand}); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
			}
			if tt.reedStrand != "" {
				reedState := &reedengine.ReedState{Strands: []reedengine.Strand{{GUID: tt.reedStrand, Name: "tst:orch-held"}}}
				if err := reedengine.SaveState(filepath.Join(h.Location.AnchorPath(), lymdirs.DotLymDirName), reedState); err != nil {
					t.Fatalf("reedengine.SaveState: %v", err)
				}
			}

			primeOfTask, err := resolvePrime(task)
			if err != nil {
				t.Fatalf("resolvePrime(task): %v", err)
			}
			gotName, gotRecorded, err := PrimeOrchStrand(primeOfTask)
			if err != nil {
				t.Fatalf("PrimeOrchStrand() error = %v; want nil", err)
			}
			if gotName != tt.wantName || gotRecorded != tt.wantRec {
				t.Errorf("PrimeOrchStrand() = %q, %v; want %q, %v", gotName, gotRecorded, tt.wantName, tt.wantRec)
			}
		})
	}
}

func TestParentOrchStrand(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "m", fabricengine.AddOptions{SkipPush: true, Parent: "b"})
	hubforge.AddPairWith(t, h, "p", fabricengine.AddOptions{SkipPush: true})
	locate := func(slug string) *lymcwd.Location {
		t.Helper()
		location, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree(%q): %v", slug, err)
		}
		return location
	}
	beam, member, forkedFromPrime := locate("b"), locate("m"), locate("p")
	seed := func(location *lymcwd.Location, guid string) {
		t.Helper()
		if err := orchengine.SaveState(orchPaths(location), orchengine.State{Phase: orchengine.PhaseIdle, Strand: guid}); err != nil {
			t.Fatalf("SaveState(%s): %v", location.WorktreeName, err)
		}
	}
	seed(h.Location, "prime-guid")
	seed(beam, "beam-guid")
	beamLive := func(live bool) func(*lymcwd.Location) (bool, error) {
		return func(worktree *lymcwd.Location) (bool, error) { return live && worktree.WorktreeName == "b", nil }
	}
	beamOrch := hubforge.TestShortname + ":b:orch"
	primeOrch := hubforge.TestShortname + ":orch"

	t.Run("a live beam orch is the member's destination", func(t *testing.T) {
		name, recorded, err := parentOrchStrand(member, beamLive(true))
		if err != nil || !recorded || name != beamOrch {
			t.Errorf("parentOrchStrand(m) = (%q, %v, %v); want (%q, true, nil)", name, recorded, err, beamOrch)
		}
	})
	t.Run("a live beam orch is its own asker destination, where the pair walk starts above it", func(t *testing.T) {
		name, recorded, err := askerOrchStrand(beam, beamLive(true))
		if err != nil || !recorded || name != beamOrch {
			t.Errorf("askerOrchStrand(b) = (%q, %v, %v); want (%q, true, nil)", name, recorded, err, beamOrch)
		}
	})
	t.Run("a stopped beam orch leaves the prime's", func(t *testing.T) {
		name, recorded, err := parentOrchStrand(member, beamLive(false))
		if err != nil || !recorded || name != primeOrch {
			t.Errorf("parentOrchStrand(m) = (%q, %v, %v); want (%q, true, nil)", name, recorded, err, primeOrch)
		}
	})
	t.Run("a pair forked from the prime goes to the prime's", func(t *testing.T) {
		name, recorded, err := parentOrchStrand(forkedFromPrime, beamLive(true))
		if err != nil || !recorded || name != primeOrch {
			t.Errorf("parentOrchStrand(p) = (%q, %v, %v); want (%q, true, nil)", name, recorded, err, primeOrch)
		}
	})
	t.Run("a live beam orch is the member's parent orch name", func(t *testing.T) {
		parent, err := nearestLiveParent(member, beamLive(true))
		if want := (hubgeom.Parent{Name: beamOrch, Worktree: "b"}); err != nil || parent != want {
			t.Errorf("nearestLiveParent(m) = (%+v, %v); want (%+v, nil)", parent, err, want)
		}
	})
	t.Run("a stopped beam orch leaves the prime's as the member's parent orch name", func(t *testing.T) {
		parent, err := nearestLiveParent(member, beamLive(false))
		if want := (hubgeom.Parent{Name: primeOrch}); err != nil || parent != want {
			t.Errorf("nearestLiveParent(m) = (%+v, %v); want (%+v, nil)", parent, err, want)
		}
	})
	t.Run("the prime's lookup ignores a live beam orch", func(t *testing.T) {
		name, recorded, err := PrimeOrchStrand(member)
		if err != nil || !recorded || name != primeOrch {
			t.Errorf("PrimeOrchStrand(m) = (%q, %v, %v); want (%q, true, nil)", name, recorded, err, primeOrch)
		}
	})
	t.Run("NotifyOrchAt of a worktree whose orch state records no strand posts nothing", func(t *testing.T) {
		seed(beam, "")
		posted, err := NotifyOrchAt(beam, "batten", "[batten notice] x: landed")
		if err != nil || posted {
			t.Errorf("NotifyOrchAt(b) = (%v, %v); want (false, nil)", posted, err)
		}
	})
	t.Run("a destination whose orch state records no strand is not recorded", func(t *testing.T) {
		seed(beam, "")
		seed(h.Location, "")
		recorded, err := ParentOrchRecorded(member)
		if err != nil || recorded {
			t.Errorf("ParentOrchRecorded(m) = (%v, %v); want (false, nil)", recorded, err)
		}
	})
}
