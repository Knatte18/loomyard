// watch_test.go covers the watch verb and its production session adapter with a fake strandOps and no real reed.

package orchcli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// TestWatcherIndexSource pins which binary the watch verb's watcher reads its index from: the one the orch strand recorded, through the CLI's index runner, and a refusal naming the way forward when none usable is recorded.
func TestWatcherIndexSource(t *testing.T) {
	t.Parallel()

	recorded := filepath.Join(t.TempDir(), "lyx")
	if err := os.WriteFile(recorded, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		strands []reedengine.StrandStatus
		// wantRunBin is the path the runner is called with, empty when the source must refuse before running it.
		wantRunBin string
		wantErr    string
	}{
		{"runs the recorded binary", []reedengine.StrandStatus{{GUID: "g1", Name: "ly:orch", LyxBin: recorded}}, recorded, ""},
		{"ignores a task strand's binary", []reedengine.StrandStatus{{GUID: "g0", Name: "ly:task:driver", LyxBin: "/elsewhere/lyx"}, {GUID: "g1", Name: "ly:orch", LyxBin: recorded}}, recorded, ""},
		{"refuses without an orch strand", []reedengine.StrandStatus{{GUID: "g0", Name: "ly:task:driver", LyxBin: recorded}}, "", "lyx orch stop"},
		{"refuses a strand recorded before the field existed", []reedengine.StrandStatus{{GUID: "g1", Name: "ly:orch"}}, "", "lyx orch stop"},
		{"refuses a recorded path that is gone", []reedengine.StrandStatus{{GUID: "g1", Name: "ly:orch", LyxBin: filepath.Join(t.TempDir(), "gone")}}, "", "lyx orch stop"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var ranWith []string
			c := newTestCLI(t, &fakeStrands{strands: tc.strands})
			c.indexRunner = func(bin string) (string, error) {
				ranWith = append(ranWith, bin)
				return fakeRunnerIndex, nil
			}
			got, err := c.watcherIndexSource()()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "lyx orch start") {
					t.Fatalf("source() = %q, %v; want an error naming %q and `lyx orch start`", got, err, tc.wantErr)
				}
			} else if err != nil || got != fakeRunnerIndex {
				t.Fatalf("source() = %q, %v; want the runner's output", got, err)
			}
			if want := []string{tc.wantRunBin}; tc.wantRunBin == "" && len(ranWith) != 0 || tc.wantRunBin != "" && !slices.Equal(ranWith, want) {
				t.Errorf("runner called with %v, want %v", ranWith, tc.wantRunBin)
			}
		})
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
