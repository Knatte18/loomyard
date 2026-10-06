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

func TestWatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// recorded saves a strand in state and holds the watch lock, as a running watcher would.
		recorded bool
		// wantOK is the envelope's ok value; a refusal is a non-zero exit.
		wantOK bool
		// wantAlreadyRunning is whether the success envelope carries already_running.
		wantAlreadyRunning bool
	}{
		{"refuses without a recorded strand", false, false, false},
		{"reports already running when the lock is held", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := newTestCLI(t, &fakeStrands{})
			if tc.recorded {
				if err := orchengine.SaveState(c.paths, orchengine.State{Strand: "g1"}); err != nil {
					t.Fatal(err)
				}
				held, acquired, err := lock.TryAcquireWriteLock(c.paths.WatchLockPath)
				if err != nil || !acquired {
					t.Fatalf("hold watch lock: acquired=%v err=%v", acquired, err)
				}
				defer held.Release()
			}

			code, env := runVerb(t, c.watchCmd())
			if (code == 0) != tc.wantOK || env["ok"] != tc.wantOK {
				t.Errorf("watch = exit %d, %v; want ok %v", code, env, tc.wantOK)
			}
			if tc.wantAlreadyRunning && env["already_running"] != true {
				t.Errorf("watch = %v; want already_running", env)
			}
		})
	}
}
