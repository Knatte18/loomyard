//go:build integration

// delivery_test.go covers the hub delivery over a hubforge hub with a pair, reed states seeded on disk and the worktree wiring replaced by a stub.

package creelcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const pairSlug = hubforge.TemplatePairSlug

// stubOps answers each worktreeOps call as scripted.
type stubOps struct {
	status       reedengine.StatusResult
	statusErr    error
	idle         bool
	sendErr      error
	interruptErr error
}

func (s *stubOps) Status() (reedengine.StatusResult, error) { return s.status, s.statusErr }

func (s *stubOps) SessionIdle(string) (shuttleengine.IdleProbe, error) {
	return shuttleengine.IdleProbe{Idle: s.idle}, nil
}

func (s *stubOps) Send(string, string) error { return s.sendErr }

func (s *stubOps) Interrupt(string) error { return s.interruptErr }

// newStubbedDelivery returns the delivery of hub with its wiring answered by stubs keyed by worktree path.
func newStubbedDelivery(hub *hubforge.Hub, stubs map[string]*stubOps) *hubDelivery {
	delivery := newHubDelivery(hub.Path, nil)
	delivery.wire = func(location *lyxcwd.Location) (worktreeOps, error) {
		stub, ok := stubs[location.WorktreePath()]
		if !ok {
			return nil, fmt.Errorf("no stub for %s", location.WorktreePath())
		}
		return stub, nil
	}
	return delivery
}

// seedReedState persists the strands (name to guid) in the reed state of the worktree at path.
func seedReedState(t *testing.T, path string, strands map[string]string) {
	t.Helper()
	location, err := lyxcwd.ResolveWorktree(path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%q) = %v", path, err)
	}
	reedState := &reedengine.ReedState{}
	for name, guid := range strands {
		reedState.Strands = append(reedState.Strands, reedengine.Strand{Name: name, GUID: guid})
	}
	dotLyx := filepath.Join(location.AnchorPath(), lyxdirs.DotLyxDirName)
	if err := os.MkdirAll(dotLyx, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", dotLyx, err)
	}
	if err := reedengine.SaveState(dotLyx, reedState); err != nil {
		t.Fatalf("SaveState(%q) = %v", dotLyx, err)
	}
}

// pauseRecordPath is where the hub's pause record lives, told through the worktree at path.
func pauseRecordPath(t *testing.T, path string) string {
	t.Helper()
	location, err := lyxcwd.ResolveWorktree(path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%q) = %v", path, err)
	}
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		t.Fatalf("PauseGeometry() = %v", err)
	}
	return geometry.RecordPath
}

func TestHubDelivery(t *testing.T) {
	t.Parallel()

	newHub := func(t *testing.T) (*hubforge.Hub, string, string) {
		hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
		return hub, hub.PrimeWorktree(), hub.PairCodeWorktree(pairSlug)
	}

	t.Run("Strands lists the prime and the pair with slugs, liveness and pane pids", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		seedReedState(t, prime, map[string]string{"ly:orch": "guid-orch"})
		seedReedState(t, pair, map[string]string{"ly:pair:impl": "guid-impl", "ly:pair:dormant": "guid-dormant"})
		delivery := newStubbedDelivery(hub, map[string]*stubOps{
			prime: {status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "guid-orch", Live: true, PanePID: 11}}}},
			pair:  {status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "guid-impl", Live: true, PanePID: 22}, {GUID: "guid-dormant"}}}},
		})

		got, err := delivery.Strands()
		if err != nil {
			t.Fatalf("Strands() = %v", err)
		}
		want := []creelengine.Strand{
			{Name: "ly:orch", GUID: "guid-orch", Worktree: prime, Live: true, PanePID: 11},
			{Name: "ly:pair:dormant", GUID: "guid-dormant", Worktree: pair, Slug: pairSlug},
			{Name: "ly:pair:impl", GUID: "guid-impl", Worktree: pair, Slug: pairSlug, Live: true, PanePID: 22},
		}
		sortStrands(got)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("Strands() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Strands reads a worktree without a session as dormant and fails on another status error", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		seedReedState(t, pair, map[string]string{"ly:pair:impl": "guid-impl"})
		stubs := map[string]*stubOps{
			prime: {},
			pair:  {statusErr: fmt.Errorf("status: %w", reedengine.ErrNoSession)},
		}
		delivery := newStubbedDelivery(hub, stubs)

		got, err := delivery.Strands()
		if err != nil || len(got) != 1 || got[0].Live {
			t.Fatalf("Strands() with no session = (%+v, %v); want one dormant strand", got, err)
		}

		stubs[pair].statusErr = errors.New("tmux exploded")
		_, err = delivery.Strands()
		if err == nil || !strings.Contains(err.Error(), pair) {
			t.Errorf("Strands() with a failing status error = %v; want one naming %s", err, pair)
		}
	})

	t.Run("Strands fails naming the file on a corrupt reed state", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		seedReedState(t, pair, map[string]string{"ly:pair:impl": "guid-impl"})
		corrupt := filepath.Join(pair, lyxdirs.DotLyxDirName, "reed.json")
		if err := os.WriteFile(corrupt, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", corrupt, err)
		}
		delivery := newStubbedDelivery(hub, map[string]*stubOps{prime: {}, pair: {}})

		_, err := delivery.Strands()
		if err == nil || !strings.Contains(err.Error(), "reed.json") {
			t.Errorf("Strands() with a corrupt reed.json = %v; want one naming the file", err)
		}
	})

	t.Run("Strands fails naming a worktree that does not resolve", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		// An empty .git directory keeps git listing the worktree as live while it no longer resolves.
		gitEntry := filepath.Join(pair, ".git")
		if err := os.Remove(gitEntry); err != nil {
			t.Fatalf("Remove(%q) = %v", gitEntry, err)
		}
		if err := os.Mkdir(gitEntry, 0o755); err != nil {
			t.Fatalf("Mkdir(%q) = %v", gitEntry, err)
		}
		delivery := newStubbedDelivery(hub, map[string]*stubOps{prime: {}, pair: {}})

		_, err := delivery.Strands()
		if err == nil || !strings.Contains(err.Error(), pair) {
			t.Errorf("Strands() with an unresolvable worktree = %v; want one naming %s", err, pair)
		}
	})

	t.Run("Send and Interrupt map shuttle's sentinels onto creel's", func(t *testing.T) {
		t.Parallel()
		hub, prime, _ := newHub(t)
		other := errors.New("boom")
		tests := []struct {
			name string
			in   error
			want error
		}{
			{"busy", fmt.Errorf("wrapped: %w", shuttleengine.ErrSessionBusy), creelengine.ErrBusy},
			{"not landed", fmt.Errorf("wrapped: %w", shuttleengine.ErrSubmissionNotLanded), creelengine.ErrNotLanded},
			{"no running turn", fmt.Errorf("wrapped: %w", shuttleengine.ErrNoRunningTurn), creelengine.ErrNoRunningTurn},
			{"other", other, other},
			{"nil", nil, nil},
		}
		strand := creelengine.Strand{Name: "ly:orch", GUID: "guid-orch", Worktree: prime}
		for _, tt := range tests {
			stub := &stubOps{sendErr: tt.in, interruptErr: tt.in}
			delivery := newStubbedDelivery(hub, map[string]*stubOps{prime: stub})
			if got := delivery.Send(strand, "hello"); got != tt.want {
				t.Errorf("Send() with %s = %v; want %v", tt.name, got, tt.want)
			}
			if got := delivery.Interrupt(strand); got != tt.want {
				t.Errorf("Interrupt() with %s = %v; want %v", tt.name, got, tt.want)
			}
		}
	})

	t.Run("Idle reports the stub's probe", func(t *testing.T) {
		t.Parallel()
		hub, prime, _ := newHub(t)
		strand := creelengine.Strand{Name: "ly:orch", GUID: "guid-orch", Worktree: prime}
		for _, idle := range []bool{true, false} {
			delivery := newStubbedDelivery(hub, map[string]*stubOps{prime: {idle: idle}})
			if got, err := delivery.Idle(strand); err != nil || got != idle {
				t.Errorf("Idle() = (%v, %v); want (%v, nil)", got, err, idle)
			}
		}
	})

	t.Run("Paused follows the pause record's scope", func(t *testing.T) {
		t.Parallel()
		hub, prime, pair := newHub(t)
		recordPath := pauseRecordPath(t, prime)
		primeStrand := creelengine.Strand{Name: "ly:orch", GUID: "g1", Worktree: prime}
		pairStrand := creelengine.Strand{Name: "ly:pair:impl", GUID: "g2", Worktree: pair, Slug: pairSlug}
		delivery := newStubbedDelivery(hub, nil)

		pause := func(scope string) {
			t.Helper()
			_, err := pauserecord.Update(recordPath, time.Now(), func(record *pauserecord.Record) error {
				record.Live = []pauserecord.Entry{{Scope: scope, PausedAt: time.Now().Add(-time.Minute)}}
				return nil
			})
			if err != nil {
				t.Fatalf("Update() = %v", err)
			}
		}
		tests := []struct {
			name                  string
			scope                 string
			wantPrime, wantPaired bool
		}{
			{"hub entry", pauserecord.ScopeHub, true, true},
			{"pair entry", pairSlug, false, true},
		}
		for _, tt := range tests {
			pause(tt.scope)
			if got, err := delivery.Paused(primeStrand); err != nil || got != tt.wantPrime {
				t.Errorf("Paused(prime strand) under a %s = (%v, %v); want (%v, nil)", tt.name, got, err, tt.wantPrime)
			}
			if got, err := delivery.Paused(pairStrand); err != nil || got != tt.wantPaired {
				t.Errorf("Paused(pair strand) under a %s = (%v, %v); want (%v, nil)", tt.name, got, err, tt.wantPaired)
			}
		}

		if err := os.WriteFile(recordPath, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", recordPath, err)
		}
		if _, err := delivery.Paused(primeStrand); err == nil {
			t.Error("Paused() on an undecodable pause record = nil error; want one")
		}
	})
}

// sortStrands orders strands by name, since the seeded states hold them in map order.
func sortStrands(strands []creelengine.Strand) {
	sort.Slice(strands, func(i, j int) bool { return strands[i].Name < strands[j].Name })
}
