// wiring_commitstatus_test.go pins the per-transition status seam's three dispositions --
// commit-hard-errors, push-warns, skip-while-mid-merge -- and both ShedPaths fill sites. Every test
// here drives newCommitStatusSeam against injected commitStatusDeps stub closures, spawning no git
// and no process, so the file stays Tier 1 with no hub fixture.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
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

// TestNewCommitStatusSeam_CommitFailsAfterMergeWentLive_TakesTheSkip pins the other end of the
// unlocked probe window. MergeActive answers false on the pre-commit probe and true on the
// post-failure re-probe -- the shape a real operator produces by starting a plain-git merge in the
// fabric sibling worktree between the two -- and the seam must absorb the resulting commit failure
// as the skip disposition rather than halting the run with it.
// Driven live during review: without the re-probe, a MERGE_HEAD landing in that window failed the
// path-scoped commit with git's "cannot do a partial commit during a merge" and killed the run.
func TestNewCommitStatusSeam_CommitFailsAfterMergeWentLive_TakesTheSkip(t *testing.T) {
	t.Parallel()

	probes := 0
	pushCalled := false
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) {
			probes++
			return probes > 1, nil
		},
		Commit: func(msg string) error {
			return errors.New("gitrepo: git commit: fatal: cannot do a partial commit during a merge")
		},
		Push: func() error {
			pushCalled = true
			return nil
		},
	}

	seam := newCommitStatusSeam(deps)
	if err := seam("Discussion-Write", "running"); err != nil {
		t.Errorf("seam(...) = %v; want nil -- a commit failure a live merge explains takes the skip disposition, never the halt", err)
	}
	if probes != 2 {
		t.Errorf("MergeActive called %d time(s); want exactly 2 -- once before the commit, once to explain its failure", probes)
	}
	if pushCalled {
		t.Error("Push was called; want it skipped -- nothing was committed to push")
	}
}

// TestNewCommitStatusSeam_CommitFailsAndReProbeFails_TakesTheSkip asserts that an unreadable
// merge-state re-probe resolves the same way the unreadable pre-commit probe does: warn and
// continue. An observation failure must not be the thing that halts an autonomous run.
func TestNewCommitStatusSeam_CommitFailsAndReProbeFails_TakesTheSkip(t *testing.T) {
	t.Parallel()

	probes := 0
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) {
			probes++
			if probes == 1 {
				return false, nil
			}
			return false, errors.New("probe unreadable")
		},
		Commit: func(msg string) error { return errors.New("commit failed") },
		Push:   func() error { return nil },
	}

	seam := newCommitStatusSeam(deps)
	if err := seam("Discussion-Write", "running"); err != nil {
		t.Errorf("seam(...) = %v; want nil -- an unreadable re-probe is the same untrustworthy-git-state category the skip exists for", err)
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

// TestWireLightweight_CommitStatusFilled asserts wireLightweight leaves c.shedPaths.CommitStatus
// non-nil, even though every verb on this lightweight path is read-only and so never invokes it --
// filling it anyway keeps the two ShedPaths literals structurally identical, per wiring.go's own
// comment at that site.
func TestWireLightweight_CommitStatusFilled(t *testing.T) {
	t.Parallel()

	location := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}

	c := &loomCLI{runID: shedrun.SelfRunID}
	c.wireLightweight(location, location.AnchorPath())

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

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.shedPaths.CommitStatus == nil {
		t.Error("c.shedPaths.CommitStatus = nil; want a non-nil seam")
	}
}

// TestStatusCommitPathspec asserts the pathspec names the reviews directory only when it holds a file:
// the empty segment directories recipe build creates would make the commit's pathspec match nothing and fail.
func TestStatusCommitPathspec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T, reviews string)
		wantReviews bool
	}{
		{"absent", func(t *testing.T, reviews string) {}, false},
		{"only empty segment directories", func(t *testing.T, reviews string) {
			if err := os.MkdirAll(filepath.Join(reviews, "plan"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"file in a segment directory", func(t *testing.T, reviews string) {
			seg := filepath.Join(reviews, "plan")
			if err := os.MkdirAll(seg, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(seg, "round-1-review.md"), []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"regular file at the reviews path", func(t *testing.T, reviews string) {
			if err := os.MkdirAll(filepath.Dir(reviews), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(reviews, []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			tt.setup(t, loomengine.LoomReviewsDir(location))
			want := []string{shedrun.StatusRel(location, shedrun.SelfRunID)}
			if tt.wantReviews {
				want = append(want, loomengine.LoomReviewsDirRel())
			}
			got := statusCommitPathspec(location, shedrun.SelfRunID)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("statusCommitPathspec() = %v; want %v", got, want)
			}
		})
	}
}

// TestStatusCommitPathspec_RunRecords asserts the loom durable directory and the drive-reports directory each appear exactly when they hold a file at any depth,
// including only a timestamped archive sibling, and are omitted when absent, empty, or holding only empty subdirectories.
func TestStatusCommitPathspec_RunRecords(t *testing.T) {
	t.Parallel()

	writeFile := func(t *testing.T, path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name      string
		setup     func(t *testing.T, loc *lyxcwd.Location)
		wantLoom  bool
		wantDrive bool
	}{
		{"absent", func(t *testing.T, loc *lyxcwd.Location) {}, false, false},
		{"empty directories", func(t *testing.T, loc *lyxcwd.Location) {
			for _, d := range []string{loomengine.LoomDurableDir(loc), shedrun.DriveReportsDir(loc, shedrun.SelfRunID)} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
		}, false, false},
		{"only empty subdirectories", func(t *testing.T, loc *lyxcwd.Location) {
			for _, d := range []string{loomengine.LoomFrictionDir(loc), filepath.Join(shedrun.DriveReportsDir(loc, shedrun.SelfRunID), "sub")} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
		}, false, false},
		{"friction note", func(t *testing.T, loc *lyxcwd.Location) {
			writeFile(t, filepath.Join(loomengine.LoomFrictionDir(loc), "note.md"))
		}, true, false},
		{"only an archive sibling", func(t *testing.T, loc *lyxcwd.Location) {
			writeFile(t, filepath.Join(loomengine.LoomDurableDir(loc), "friction-20260101T000000", "note.md"))
		}, true, false},
		{"drive report", func(t *testing.T, loc *lyxcwd.Location) {
			writeFile(t, filepath.Join(shedrun.DriveReportsDir(loc, shedrun.SelfRunID), "report.md"))
		}, false, true},
		{"both", func(t *testing.T, loc *lyxcwd.Location) {
			writeFile(t, filepath.Join(loomengine.LoomFrictionDir(loc), "note.md"))
			writeFile(t, filepath.Join(shedrun.DriveReportsDir(loc, shedrun.SelfRunID), "report.md"))
		}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			tt.setup(t, location)
			want := []string{shedrun.StatusRel(location, shedrun.SelfRunID)}
			if tt.wantLoom {
				want = append(want, loomengine.LoomDurableDirRel())
			}
			if tt.wantDrive {
				want = append(want, shedrun.DriveReportsRel(location, shedrun.SelfRunID))
			}
			got := statusCommitPathspec(location, shedrun.SelfRunID)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("statusCommitPathspec() = %v; want %v", got, want)
			}
		})
	}
}

// TestStatusCommitPathspec_PendingRejectionHoldsTheRound asserts a pending rejection leaves the reviews root and the loom durable directory out though each holds a file, whether the highest round is unclassed or classed but uncommitted, and that without a rejection today's pathspec stays.
func TestStatusCommitPathspec_PendingRejectionHoldsTheRound(t *testing.T) {
	t.Parallel()

	writeFile := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name        string
		record      string
		rejection   bool
		wantHoldDir bool
	}{
		{"pending rejection, unclassed round", `{"first_card":3}`, true, true},
		{"pending rejection, classed but uncommitted round", `{"first_card":3,"class":"exempt"}`, true, true},
		{"no pending rejection", `{"first_card":3,"class":"exempt"}`, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			writeFile(t, filepath.Join(loomengine.LoomReworkDir(location), "round-1", "record.json"), tt.record)
			writeFile(t, filepath.Join(loomengine.LoomReviewsDir(location), "plan", "round-1-review.md"), "x\n")
			writeFile(t, filepath.Join(shedrun.DriveReportsDir(location, shedrun.SelfRunID), "report.md"), "x\n")
			if tt.rejection {
				writeFile(t, loomengine.LoomRejectionPath(location), "{}\n")
			}

			want := []string{shedrun.StatusRel(location, shedrun.SelfRunID)}
			if !tt.wantHoldDir {
				want = append(want, loomengine.LoomReviewsDirRel(), loomengine.LoomDurableDirRel())
			}
			want = append(want, shedrun.DriveReportsRel(location, shedrun.SelfRunID))
			got := statusCommitPathspec(location, shedrun.SelfRunID)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("statusCommitPathspec() = %v; want %v", got, want)
			}
		})
	}
}
