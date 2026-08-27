// wiring_commitstatus_test.go pins the per-transition status seam's three dispositions --
// commit-hard-errors, push-warns, skip-while-mid-merge -- and both ShedPaths fill sites. Every test
// here drives newCommitStatusSeam against injected commitStatusDeps stub closures, spawning no git
// and no process, so the file stays Tier 1 with no hub fixture.

package loomcli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestNewCommitStatusSeam_OrdinaryPath asserts that, with MergeActive false, Commit nil, and Push
// nil, the seam calls Commit exactly once and Push exactly once, in that order, and returns nil.
func TestNewCommitStatusSeam_OrdinaryPath(t *testing.T) {
	t.Parallel()

	var calls []string
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit: func(msg string) error {
			calls = append(calls, "commit")
			return nil
		},
		Push: func() error {
			calls = append(calls, "push")
			return nil
		},
	}

	seam := newCommitStatusSeam(deps)
	if err := seam("Discussion-Write", "running"); err != nil {
		t.Fatalf("seam(...) = %v; want nil", err)
	}

	want := []string{"commit", "push"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v; want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("calls[%d] = %q; want %q", i, calls[i], want[i])
		}
	}
}

// TestCommitStatusMessage renders exactly "loom: <producer> -> <state>" for a table of
// producer/state pairs, so a regression to a bare constant fails here.
func TestCommitStatusMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		producer string
		state    string
	}{
		{"DiscussionWriteRunning", "Discussion-Write", "running"},
		{"PlanBouncerStuck", "Plan-Bouncer", "stuck"},
		{"PublishDone", "Publish", "done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitStatusMessage(tt.producer, tt.state)
			want := fmt.Sprintf("loom: %s -> %s", tt.producer, tt.state)
			if got != want {
				t.Errorf("commitStatusMessage(%q, %q) = %q; want %q", tt.producer, tt.state, got, want)
			}
		})
	}
}

// TestNewCommitStatusSeam_CommitErrorPropagates asserts a Commit error propagates out of the seam
// unchanged, and that Push is never called.
func TestNewCommitStatusSeam_CommitErrorPropagates(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("commit failed")
	pushCalled := false
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) error { return commitErr },
		Push: func() error {
			pushCalled = true
			return nil
		},
	}

	seam := newCommitStatusSeam(deps)
	if err := seam("Discussion-Write", "running"); !errors.Is(err, commitErr) {
		t.Errorf("seam(...) = %v; want %v", err, commitErr)
	}
	if pushCalled {
		t.Error("Push was called; want it skipped when Commit errors")
	}
}

// TestNewCommitStatusSeam_PushErrorReturnsNil asserts a Push error returns nil from the seam, so a
// failed push never halts a run, while Commit still ran.
func TestNewCommitStatusSeam_PushErrorReturnsNil(t *testing.T) {
	t.Parallel()

	commitCalled := false
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit: func(msg string) error {
			commitCalled = true
			return nil
		},
		Push: func() error { return errors.New("push failed") },
	}

	seam := newCommitStatusSeam(deps)
	if err := seam("Discussion-Write", "running"); err != nil {
		t.Errorf("seam(...) = %v; want nil", err)
	}
	if !commitCalled {
		t.Error("Commit was not called; want it to have run before Push")
	}
}

// TestNewCommitStatusSeam_PushRejectedReturnsNil asserts gitrepo.ErrPushRejected specifically
// returns nil from the seam, since a rejection is the routine multi-machine case this feature
// creates.
func TestNewCommitStatusSeam_PushRejectedReturnsNil(t *testing.T) {
	t.Parallel()

	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit:      func(msg string) error { return nil },
		Push:        func() error { return gitrepo.ErrPushRejected },
	}

	seam := newCommitStatusSeam(deps)
	if err := seam("Discussion-Write", "running"); err != nil {
		t.Errorf("seam(...) = %v; want nil on a rejected push", err)
	}
}

// TestNewCommitStatusSeam_MergeActiveSkips asserts MergeActive reporting true skips both Commit and
// Push and returns nil, and that MergeActive returning a non-nil error does exactly the same --
// asserted in one test, since the equal-disposition property is the point.
func TestNewCommitStatusSeam_MergeActiveSkips(t *testing.T) {
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
			commitCalled := false
			pushCalled := false
			deps := commitStatusDeps{
				MergeActive: tt.mergeActive,
				Commit: func(msg string) error {
					commitCalled = true
					return nil
				},
				Push: func() error {
					pushCalled = true
					return nil
				},
			}

			seam := newCommitStatusSeam(deps)
			if err := seam("Discussion-Write", "running"); err != nil {
				t.Errorf("seam(...) = %v; want nil", err)
			}
			if commitCalled {
				t.Error("Commit was called; want it skipped while mid-merge")
			}
			if pushCalled {
				t.Error("Push was called; want it skipped while mid-merge")
			}
		})
	}
}

// TestWireStatusPathsOnly_CommitStatusFilled asserts wireStatusPathsOnly leaves
// c.shedPaths.CommitStatus non-nil, even though the read-only status/pause verbs it serves never
// call Run and so never invoke it -- filling it anyway keeps the two ShedPaths literals structurally
// identical, per wiring.go's own comment at that site.
func TestWireStatusPathsOnly_CommitStatusFilled(t *testing.T) {
	t.Parallel()

	location := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}

	c := &loomCLI{}
	c.wireStatusPathsOnly(location, location.AnchorPath())

	if c.shedPaths.CommitStatus == nil {
		t.Error("c.shedPaths.CommitStatus = nil; want a non-nil seam")
	}
}

// TestWire_CommitStatusFilled asserts wire() leaves c.shedPaths.CommitStatus non-nil. It drives
// wire() the way wiring_test.go's own hubLocation fixture already does, rather than building a
// second fixture idiom.
func TestWire_CommitStatusFilled(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.shedPaths.CommitStatus == nil {
		t.Error("c.shedPaths.CommitStatus = nil; want a non-nil seam")
	}
}
