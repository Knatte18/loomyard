package statuscommit

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeGate counts the admissions a seam takes and how many of them it still holds.
type fakeGate struct {
	admitted    int
	outstanding int
}

// admit takes one admission and returns its release.
func (g *fakeGate) admit() func() {
	g.admitted++
	g.outstanding++
	return func() { g.outstanding-- }
}

// TestNew_OrdinaryPath covers the path with no merge in progress.
// The seam commits with the message its prefix names, then runs afterCommit.
// It pushes only when the commit landed something, and it takes one admission per transition and releases it.
func TestNew_OrdinaryPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		prefix      string
		producer    string
		state       string
		committed   bool
		wantMessage string
		wantPush    bool
	}{
		{name: "loom", prefix: "loom", producer: "Discussion-Write", state: "running", committed: true, wantMessage: "loom: Discussion-Write -> running", wantPush: true},
		{name: "batten", prefix: "batten", producer: "Run-Shed", state: "running", committed: true, wantMessage: "batten: Run-Shed -> running", wantPush: true},
		{name: "NothingLanded", prefix: "loom", producer: "Discussion-Write", state: "running", committed: false, wantMessage: "loom: Discussion-Write -> running", wantPush: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls []string
			var gotMessage string
			var gate fakeGate
			deps := Deps{
				MergeActive: func() (bool, error) { return false, nil },
				Commit: func(msg string) (bool, error) {
					calls = append(calls, "commit")
					gotMessage = msg
					return tt.committed, nil
				},
				Push: func() error {
					calls = append(calls, "push")
					return nil
				},
				Admit: gate.admit,
			}
			afterCommit := func(producer, state string) {
				calls = append(calls, fmt.Sprintf("after:%s:%s", producer, state))
			}

			seam := New(deps, tt.prefix, tt.prefix+"cli", afterCommit)
			if err := seam(tt.producer, tt.state); err != nil {
				t.Fatalf("seam(...) = %v; want nil", err)
			}

			want := []string{"commit", fmt.Sprintf("after:%s:%s", tt.producer, tt.state)}
			if tt.wantPush {
				want = append(want, "push")
			}
			if !reflect.DeepEqual(calls, want) {
				t.Errorf("calls = %v; want %v", calls, want)
			}
			if gotMessage != tt.wantMessage {
				t.Errorf("Commit msg = %q; want %q", gotMessage, tt.wantMessage)
			}
			if gate.admitted != 1 || gate.outstanding != 0 {
				t.Errorf("admissions taken = %d, still held = %d; want 1 and 0", gate.admitted, gate.outstanding)
			}
		})
	}
}

//testtiming:keep pins Message's format for any prefix, which the commit-error test covering its blocks never reads
func TestMessage(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
	t.Parallel()

	commitErr := errors.New("commit failed")
	pushCalled := false
	afterCalled := false
	var gate fakeGate
	deps := Deps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) (bool, error) { return false, commitErr },
		Push:        func() error { pushCalled = true; return nil },
		Admit:       gate.admit,
	}

	seam := New(deps, "loom", "loomcli", func(string, string) { afterCalled = true })
	if err := seam("Discussion-Write", "running"); !errors.Is(err, commitErr) {
		t.Errorf("seam(...) = %v; want %v", err, commitErr)
	}
	if pushCalled {
		t.Error("Push was called; want it skipped when Commit errors")
	}
	if afterCalled {
		t.Error("afterCommit was called; want it skipped when Commit errors")
	}
	if gate.outstanding != 0 {
		t.Errorf("admissions still held = %d; want 0", gate.outstanding)
	}
}

// TestNew_CommitFailureExplainedByTheReProbeTakesTheSkip covers a commit failure that the second probe explains.
// The probe is unlocked, so a merge can go live between the first probe and the commit.
// Without the re-probe, a MERGE_HEAD landing in that window fails the path-scoped commit.
// Git refuses it with "cannot do a partial commit during a merge", and the run is killed.
// A re-probe that reports the merge live takes the skip disposition, and so does one that cannot read the state.
// Neither takes the halt: an unreadable re-probe is the same untrustworthy-git-state case the skip exists for.
func TestNew_CommitFailureExplainedByTheReProbeTakesTheSkip(t *testing.T) {
	tiergate.Require(t, 0)
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
			pushCalled := false
			deps := Deps{
				MergeActive: func() (bool, error) {
					probes++
					if probes == 1 {
						return false, nil
					}
					return tt.reProbe()
				},
				Commit: func(msg string) (bool, error) { return false, tt.commitErr },
				Push:   func() error { pushCalled = true; return nil },
			}

			seam := New(deps, "loom", "loomcli", nil)
			if err := seam("Discussion-Write", "running"); err != nil {
				t.Errorf("seam(...) = %v; want nil -- a commit failure the re-probe explains takes the skip disposition, never the halt", err)
			}
			if probes != 2 {
				t.Errorf("MergeActive called %d time(s); want exactly 2 -- once before the commit, once to explain its failure", probes)
			}
			if pushCalled {
				t.Error("Push was called; want it skipped -- nothing was committed to push")
			}
		})
	}
}

// TestNew_PushErrorReturnsNil covers a push that fails with an error that is not owed.
// The seam returns the error, so the run halts at the transition, and it neither retries nor waits.
// The name is the one the earlier warn-and-continue disposition had.
func TestNew_PushErrorReturnsNil(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	pushErr := errors.New("push rejected")
	tests := []struct {
		name string
		owed func(error) bool
	}{
		{"OwedReadsFalse", func(error) bool { return false }},
		{"OwedNil", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			commits, pushes := 0, 0
			var gate fakeGate
			deps := Deps{
				MergeActive: func() (bool, error) { return false, nil },
				Commit:      func(msg string) (bool, error) { commits++; return true, nil },
				Push:        func() error { pushes++; return pushErr },
				Admit:       gate.admit,
				Owed:        tt.owed,
				Sleep:       func(time.Duration) { t.Error("Sleep was called; want no wait for a push that is not owed") },
			}

			seam := New(deps, "loom", "loomcli", nil)
			if err := seam("Discussion-Write", "running"); !errors.Is(err, pushErr) {
				t.Errorf("seam(...) = %v; want %v -- a push that is not owed halts the run", err, pushErr)
			}
			if commits != 1 || pushes != 1 {
				t.Errorf("Commit and Push called %d and %d time(s); want 1 and 1 -- the transition is not retried", commits, pushes)
			}
			if gate.outstanding != 0 {
				t.Errorf("admissions still held = %d; want 0", gate.outstanding)
			}
		})
	}
}

// TestNew_OwedPushRetriesBehindTheGate covers a push that is owed before it lands.
// The first push runs under the admission taken before the commit, so a pause cannot hold it.
// Every retry runs with the admission released, so the gate holds it while a pause stands.
// The wait before a retry starts at a second, doubles, and stops doubling at owedRetryCeiling.
func TestNew_OwedPushRetriesBehindTheGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	owedErr := errors.New("origin unreachable")
	tests := []struct {
		name       string
		owedPushes int
		wantSleeps []time.Duration
	}{
		{"LandsAfterTwoOwedPushes", 2, []time.Duration{time.Second, 2 * time.Second}},
		{"StopsDoublingAtTheCeiling", 8, []time.Duration{
			time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
			16 * time.Second, 32 * time.Second, owedRetryCeiling, owedRetryCeiling,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gate fakeGate
			var heldAtPush []int
			var sleeps []time.Duration
			deps := Deps{
				MergeActive: func() (bool, error) { return false, nil },
				Commit:      func(msg string) (bool, error) { return true, nil },
				Push: func() error {
					heldAtPush = append(heldAtPush, gate.outstanding)
					if len(heldAtPush) <= tt.owedPushes {
						return owedErr
					}
					return nil
				},
				Admit: gate.admit,
				Owed:  func(err error) bool { return errors.Is(err, owedErr) },
				Sleep: func(wait time.Duration) { sleeps = append(sleeps, wait) },
			}

			seam := New(deps, "loom", "loomcli", nil)
			if err := seam("Discussion-Write", "running"); err != nil {
				t.Fatalf("seam(...) = %v; want nil once the push landed", err)
			}

			wantHeld := make([]int, tt.owedPushes+1)
			wantHeld[0] = 1
			if !reflect.DeepEqual(heldAtPush, wantHeld) {
				t.Errorf("admissions held at each push = %v; want %v -- only the first push runs admitted", heldAtPush, wantHeld)
			}
			if !reflect.DeepEqual(sleeps, tt.wantSleeps) {
				t.Errorf("waits before the retries = %v; want %v", sleeps, tt.wantSleeps)
			}
			if gate.admitted != 1 || gate.outstanding != 0 {
				t.Errorf("admissions taken = %d, still held = %d; want 1 and 0", gate.admitted, gate.outstanding)
			}
		})
	}
}

// TestNew_NilPushPanics pins the wiring guard: a seam with no push would leave every transition unpublished.
func TestNew_NilPushPanics(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("New(Deps{}, ...) returned; want a panic for a nil Push")
		}
	}()
	New(Deps{MergeActive: func() (bool, error) { return false, nil }}, "loom", "loomcli", nil)
}

func TestNew_MergeActiveSkips(t *testing.T) {
	tiergate.Require(t, 0)
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
			var gate fakeGate
			deps := Deps{
				MergeActive: tt.mergeActive,
				Commit:      func(msg string) (bool, error) { called = append(called, "commit"); return true, nil },
				Push:        func() error { called = append(called, "push"); return nil },
				Admit:       gate.admit,
			}

			seam := New(deps, "loom", "loomcli", func(string, string) { called = append(called, "after") })
			if err := seam("Discussion-Write", "running"); err != nil {
				t.Errorf("seam(...) = %v; want nil", err)
			}
			if len(called) != 0 {
				t.Errorf("calls = %v; want none while mid-merge", called)
			}
			if gate.admitted != 0 {
				t.Errorf("admissions taken = %d; want none while mid-merge", gate.admitted)
			}
		})
	}
}
