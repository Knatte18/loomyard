//go:build tier0

package statuscommit

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// TestNew_OrdinaryPath covers the path with no merge in progress: the seam commits with the message its prefix names, then runs afterCommit, then spawns the push only when the commit landed something.
func TestNew_OrdinaryPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		prefix      string
		producer    string
		state       string
		committed   bool
		wantMessage string
		wantSpawn   bool
	}{
		{name: "loom", prefix: "loom", producer: "Discussion-Write", state: "running", committed: true, wantMessage: "loom: Discussion-Write -> running", wantSpawn: true},
		{name: "batten", prefix: "batten", producer: "Run-Shed", state: "running", committed: true, wantMessage: "batten: Run-Shed -> running", wantSpawn: true},
		{name: "NothingLanded", prefix: "loom", producer: "Discussion-Write", state: "running", committed: false, wantMessage: "loom: Discussion-Write -> running", wantSpawn: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls []string
			var gotMessage string
			deps := Deps{
				MergeActive: func() (bool, error) { return false, nil },
				Commit: func(msg string) (bool, error) {
					calls = append(calls, "commit")
					gotMessage = msg
					return tt.committed, nil
				},
				Spawn: func() error {
					calls = append(calls, "spawn")
					return nil
				},
			}
			afterCommit := func(producer, state string) {
				calls = append(calls, fmt.Sprintf("after:%s:%s", producer, state))
			}

			seam := New(deps, tt.prefix, tt.prefix+"cli", afterCommit)
			if err := seam(tt.producer, tt.state); err != nil {
				t.Fatalf("seam(...) = %v; want nil", err)
			}

			want := []string{"commit", fmt.Sprintf("after:%s:%s", tt.producer, tt.state)}
			if tt.wantSpawn {
				want = append(want, "spawn")
			}
			if !reflect.DeepEqual(calls, want) {
				t.Errorf("calls = %v; want %v", calls, want)
			}
			if gotMessage != tt.wantMessage {
				t.Errorf("Commit msg = %q; want %q", gotMessage, tt.wantMessage)
			}
		})
	}
}

//testtiming:keep pins Message's format for any prefix, which the commit-error test covering its blocks never reads
func TestMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefix   string
		producer string
		state    string
		want     string
	}{
		{"LoomRunning", "loom", "Discussion-Write", "running", "loom: Discussion-Write -> running"},
		{"BattenPaused", "batten", "Run-Shed", "paused", "batten: Run-Shed -> paused"},
		{"AnyPrefix", "other", "Publish", "done", "other: Publish -> done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Message(tt.prefix, tt.producer, tt.state); got != tt.want {
				t.Errorf("Message(%q, %q, %q) = %q; want %q", tt.prefix, tt.producer, tt.state, got, tt.want)
			}
		})
	}
}

func TestNew_CommitErrorPropagates(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("commit failed")
	spawnCalled := false
	afterCalled := false
	deps := Deps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) (bool, error) { return false, commitErr },
		Spawn:       func() error { spawnCalled = true; return nil },
	}

	seam := New(deps, "loom", "loomcli", func(string, string) { afterCalled = true })
	if err := seam("Discussion-Write", "running"); !errors.Is(err, commitErr) {
		t.Errorf("seam(...) = %v; want %v", err, commitErr)
	}
	if spawnCalled {
		t.Error("Spawn was called; want it skipped when Commit errors")
	}
	if afterCalled {
		t.Error("afterCommit was called; want it skipped when Commit errors")
	}
}

// TestNew_CommitFailureExplainedByTheReProbeTakesTheSkip covers a commit failure the second probe explains: the probe is unlocked, so a merge can go live between the first probe and the commit.
// Driven live during review: without the re-probe, a MERGE_HEAD landing in that window failed the path-scoped commit with git's "cannot do a partial commit during a merge" and killed the run.
// Both a re-probe that reports the merge live and one that cannot read the state take the skip disposition, never the halt: an unreadable re-probe is the same untrustworthy-git-state category the skip exists for.
func TestNew_CommitFailureExplainedByTheReProbeTakesTheSkip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		commitErr error
		reProbe   func() (bool, error)
	}{
		{
			name:      "merge went live",
			commitErr: errors.New("gitrepo: git commit: fatal: cannot do a partial commit during a merge"),
			reProbe:   func() (bool, error) { return true, nil },
		},
		{
			name:      "re-probe fails",
			commitErr: errors.New("commit failed"),
			reProbe:   func() (bool, error) { return false, errors.New("probe unreadable") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			probes := 0
			spawnCalled := false
			deps := Deps{
				MergeActive: func() (bool, error) {
					probes++
					if probes == 1 {
						return false, nil
					}
					return tt.reProbe()
				},
				Commit: func(msg string) (bool, error) { return false, tt.commitErr },
				Spawn:  func() error { spawnCalled = true; return nil },
			}

			seam := New(deps, "loom", "loomcli", nil)
			if err := seam("Discussion-Write", "running"); err != nil {
				t.Errorf("seam(...) = %v; want nil -- a commit failure the re-probe explains takes the skip disposition, never the halt", err)
			}
			if probes != 2 {
				t.Errorf("MergeActive called %d time(s); want exactly 2 -- once before the commit, once to explain its failure", probes)
			}
			if spawnCalled {
				t.Error("Spawn was called; want it skipped -- nothing was committed to push")
			}
		})
	}
}

func TestNew_PushErrorReturnsNil(t *testing.T) {
	t.Parallel()

	commits := 0
	deps := Deps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) (bool, error) { commits++; return true, nil },
		Spawn:       func() error { return errors.New("spawn failed") },
	}

	seam := New(deps, "loom", "loomcli", nil)
	if err := seam("Discussion-Write", "running"); err != nil {
		t.Errorf("seam(...) = %v; want nil -- a push that fails to spawn never halts a run", err)
	}
	if commits != 1 {
		t.Errorf("Commit called %d time(s); want exactly 1 -- the transition is not retried", commits)
	}
}

func TestNew_MergeActiveSkips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mergeActive func() (bool, error)
	}{
		{"ReportsTrue", func() (bool, error) { return true, nil }},
		{"ProbeErrors", func() (bool, error) { return false, errors.New("probe unreadable") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var called []string
			deps := Deps{
				MergeActive: tt.mergeActive,
				Commit:      func(msg string) (bool, error) { called = append(called, "commit"); return true, nil },
				Spawn:       func() error { called = append(called, "spawn"); return nil },
			}

			seam := New(deps, "loom", "loomcli", func(string, string) { called = append(called, "after") })
			if err := seam("Discussion-Write", "running"); err != nil {
				t.Errorf("seam(...) = %v; want nil", err)
			}
			if len(called) != 0 {
				t.Errorf("calls = %v; want none while mid-merge", called)
			}
		})
	}
}
