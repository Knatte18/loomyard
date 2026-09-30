// watch_test.go covers the watch verb and its production session adapter with a fake strandOps and no real reed.

package orchcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestRunnerSession_StrandAlive(t *testing.T) {
	t.Parallel()

	fake := &fakeStrands{strands: []reedengine.StrandStatus{
		{GUID: "live", Live: true},
		{GUID: "dead", Live: false},
	}}
	s := runnerSession{strands: fake}

	for _, tc := range []struct {
		guid string
		want bool
	}{{"live", true}, {"dead", false}, {"absent", false}} {
		got, err := s.StrandAlive(tc.guid)
		if err != nil {
			t.Fatalf("StrandAlive(%q): %v", tc.guid, err)
		}
		if got != tc.want {
			t.Errorf("StrandAlive(%q) = %v; want %v", tc.guid, got, tc.want)
		}
	}
}

func TestWatch_RefusesWithoutRecordedStrand(t *testing.T) {
	t.Parallel()

	c := newTestCLI(t, &fakeStrands{})
	code, env := runVerb(t, c.watchCmd())
	if code == 0 || env["ok"] != false {
		t.Errorf("watch = exit %d, %v; want a refusal", code, env)
	}
}

func TestWatch_ReportsAlreadyRunningWhenLockHeld(t *testing.T) {
	t.Parallel()

	c := newTestCLI(t, &fakeStrands{})
	if err := orchengine.SaveState(c.paths, orchengine.State{Strand: "g1"}); err != nil {
		t.Fatal(err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(c.paths.WatchLockPath)
	if err != nil || !acquired {
		t.Fatalf("hold watch lock: acquired=%v err=%v", acquired, err)
	}
	defer held.Release()

	code, env := runVerb(t, c.watchCmd())
	if code != 0 || env["ok"] != true || env["already_running"] != true {
		t.Errorf("watch = exit %d, %v; want a success envelope with already_running", code, env)
	}
}
