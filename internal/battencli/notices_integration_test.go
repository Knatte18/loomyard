//go:build tier1

// notices_integration_test.go pins where batten's notices about a run go, over a hubforge hub with a beam: the walk's start for a present or absent pair, and whether its destination records an orch strand.

package battencli

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// noticeHub returns a hub with a recorded shortname, a marked beam "b" with a member pair "m" and a pair "p" forked from the prime.
func noticeHub(t *testing.T) *hubforge.Hub {
	t.Helper()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	if err := fabricengine.WriteShortname(h.BoardDir(), "lx"); err != nil {
		t.Fatalf("record the shortname: %v", err)
	}
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "m", fabricengine.AddOptions{SkipPush: true, Parent: "b"})
	hubforge.AddPairWith(t, h, "p", fabricengine.AddOptions{SkipPush: true})
	return h
}

// recordOrchStrand records a strand in the orch state of location, with no reed session behind it, so the orch reads recorded but never live.
func recordOrchStrand(t *testing.T, location *lymcwd.Location) {
	t.Helper()
	if err := orchengine.SaveState(orchcli.PrimePaths(location), orchengine.State{Strand: "guid-" + location.WorktreeName, Phase: orchengine.PhaseIdle}); err != nil {
		t.Fatalf("record the orch strand of %q: %v", location.WorktreeName, err)
	}
}

func TestNoticeSite(t *testing.T) {
	t.Parallel()

	h := noticeHub(t)
	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}
	pair := resolvePair(t, h, "p")
	resolverErr := errors.New("origin record unreadable")

	tests := []struct {
		name       string
		pair       func() (*lymcwd.Location, bool, error)
		parent     string
		parentErr  error
		wantSite   string
		wantIsPair bool
		wantErr    error
	}{
		{"PresentPairIsTheSite", func() (*lymcwd.Location, bool, error) { return pair, true, nil }, "", nil, "p", true, nil},
		{"AbsentPairOfAPresentBeamStartsAtTheBeam", func() (*lymcwd.Location, bool, error) { return nil, false, nil }, "b", nil, "b", false, nil},
		{"AbsentPairWhoseBeamIsAbsentHereStartsAtThePrime", func() (*lymcwd.Location, bool, error) { return nil, false, nil }, "gone-beam", nil, primeName, false, nil},
		{"TaskOfThePrimeStartsAtThePrime", func() (*lymcwd.Location, bool, error) { return nil, false, nil }, "", nil, primeName, false, nil},
		{"ResolverErrorIsReturned", func() (*lymcwd.Location, bool, error) { return nil, false, nil }, "", resolverErr, "", false, resolverErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site, isPair, err := noticeSite(h.Location, "m", tt.pair, func(string) (string, error) { return tt.parent, tt.parentErr })

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("noticeSite() error = %v; want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if site.WorktreeName != tt.wantSite || isPair != tt.wantIsPair {
				t.Errorf("noticeSite() = (%q, %v); want (%q, %v)", site.WorktreeName, isPair, tt.wantSite, tt.wantIsPair)
			}
		})
	}
}

func TestNoticesReachDriversParent(t *testing.T) {
	t.Parallel()

	h := noticeHub(t)
	// An absent member of the beam: its code worktree is gone while its records and origin record remain.
	hubforge.AddPairWith(t, h, "gone-member", fabricengine.AddOptions{SkipPush: true, Parent: "b"})
	if err := os.RemoveAll(h.PairCodeWorktree("gone-member")); err != nil {
		t.Fatalf("remove the member's worktree: %v", err)
	}
	beam := resolvePair(t, h, "b")

	// The beam's orch records a strand with no session behind it, so it never reads live and the walk passes it.
	recordOrchStrand(t, beam)

	tests := []struct {
		name        string
		primeStrand bool
		slug        string
		want        bool
	}{
		{"PairForkedFromThePrimeReachesThePrimesOrch", true, "p", true},
		{"MemberOfABeamWithNoLiveOrchReachesThePrimesOrch", true, "m", true},
		{"PairForkedFromThePrimeWithNoOrchRecordedIsUnwatched", false, "p", false},
		{"MemberOfABeamWithNoOrchRecordedAnywhereIsUnwatched", false, "m", false},
		{"AbsentMemberWithItsBeamPresentFollowsTheSameWalk", true, "gone-member", true},
		{"AbsentMemberWithNoOrchRecordedIsUnwatched", false, "gone-member", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The prime's state is shared, so these rows run in order.
			statePath := orchcli.PrimePaths(h.Location).StatePath
			if tt.primeStrand {
				recordOrchStrand(t, h.Location)
			} else if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
				t.Fatalf("clear the prime's orch state: %v", err)
			}

			got, err := runParentRecorded(h.Location, tt.slug)
			if err != nil {
				t.Fatalf("runParentRecorded(%q) = %v", tt.slug, err)
			}
			if got != tt.want {
				t.Errorf("runParentRecorded(%q) = %v; want %v", tt.slug, got, tt.want)
			}

			if tt.slug == "gone-member" {
				return
			}
			pair := resolvePair(t, h, tt.slug)
			reached, err := noticesReachDriversParent(h.Location, pair)
			if err != nil {
				t.Fatalf("noticesReachDriversParent(%q) = %v", tt.slug, err)
			}
			if reached != tt.want {
				t.Errorf("noticesReachDriversParent(%q) = %v; want %v", tt.slug, reached, tt.want)
			}
		})
	}
}
