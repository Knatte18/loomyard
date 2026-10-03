// wiring_commitstatus_test.go pins loom's side of the per-transition status seam -- its board status write and both ShedPaths fill sites -- and the status pathspec.
// The seam's three dispositions -- commit-hard-errors, push-warns, skip-while-mid-merge -- live in internal/statuscommit and are pinned there.
// Every test here drives newCommitStatusSeam against injected commitStatusDeps stub closures, spawning no git
// and no process, so the file stays Tier 1 with no hub fixture.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

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

// TestNewCommitStatusSeam_BoardStatus asserts the seam writes "<state> · <producer>" to the board only when it changed,
// writes nothing for a finished run, retries a failed write on the next transition, and still commits when the board write fails.
func TestNewCommitStatusSeam_BoardStatus(t *testing.T) {
	t.Parallel()

	var written []string
	commits := 0
	failNext := false
	deps := commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit: func(string) error {
			commits++
			return nil
		},
		Push: func() error { return nil },
		SetBoardStatus: func(status string) error {
			if failNext {
				failNext = false
				return errors.New("board locked")
			}
			written = append(written, status)
			return nil
		},
	}

	seam := newCommitStatusSeam(deps)
	steps := []struct {
		producer, state string
		fail            bool
	}{
		{"Plan-Write", "running", false},
		{"Plan-Write", "running", false},
		{"Plan-Review", "running", true},
		{"Plan-Review", "running", false},
		{"PR-Gate", "awaiting", false},
		{"Friction-Reflect", "done", false},
	}
	for _, st := range steps {
		failNext = st.fail
		if err := seam(st.producer, st.state); err != nil {
			t.Fatalf("seam(%q, %q) = %v; want nil", st.producer, st.state, err)
		}
	}

	want := []string{"running · Plan-Write", "running · Plan-Review", "awaiting · PR-Gate"}
	if !reflect.DeepEqual(written, want) {
		t.Errorf("board writes = %q; want %q", written, want)
	}
	if commits != len(steps) {
		t.Errorf("commits = %d; want %d", commits, len(steps))
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
