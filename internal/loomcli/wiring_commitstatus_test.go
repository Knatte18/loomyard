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
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// selfStatusRel is the status file's relative path for the self run.
func selfStatusRel(location *lyxcwd.Location) string {
	return shedrun.StatusRel(location, shedrun.SelfRunID)
}

// TestNewCommitStatusSeam_OrdinaryPath asserts that, with MergeActive false, Commit nil, and Push
// nil, the seam calls Commit exactly once and Push exactly once, in that order, and returns nil.
//
//testtiming:keep pins the seam with no board-status writer calling commit then push once each, in that order; the board-status test always installs a writer
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

// TestStatusCommitPathspec asserts the pathspec always names the run's status file, and names each of the reviews directory, the loom durable directory and the drive-reports directory only when it holds a file at any depth:
// empty directories, segment directories and subdirectories would make the commit's pathspec match nothing and fail, and an archive sibling alone still counts.
// A pending rejection holds the reviews root and the loom durable directory out though each holds a file, whether the highest rework round is unclassed or classed but uncommitted.
//
//testtiming:keep pins the status commit pathspec naming the status file always and the reviews, loom durable and drive-reports directories only once they hold a file, and a pending rejection holding the round out; the covering integration test commits one fixed layout
func TestStatusCommitPathspec(t *testing.T) {
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
		setup       func(t *testing.T, loc *lyxcwd.Location)
		wantReviews bool
		wantLoom    bool
		wantDrive   bool
	}{
		{name: "nothing on disk", setup: func(t *testing.T, loc *lyxcwd.Location) {}},
		{
			name: "only empty segment directories",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				if err := os.MkdirAll(filepath.Join(loomengine.LoomReviewsDir(loc), "plan"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "file in a segment directory",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				writeFile(t, filepath.Join(loomengine.LoomReviewsDir(loc), "plan", "round-1-review.md"), "x\n")
			},
			wantReviews: true,
		},
		{
			name: "regular file at the reviews path",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				writeFile(t, loomengine.LoomReviewsDir(loc), "x\n")
			},
		},
		{
			name: "empty directories",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				for _, d := range []string{loomengine.LoomDurableDir(loc), shedrun.DriveReportsDir(loc, shedrun.SelfRunID)} {
					if err := os.MkdirAll(d, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "only empty subdirectories",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				for _, d := range []string{loomengine.LoomFrictionDir(loc), filepath.Join(shedrun.DriveReportsDir(loc, shedrun.SelfRunID), "sub")} {
					if err := os.MkdirAll(d, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "friction note",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				writeFile(t, filepath.Join(loomengine.LoomFrictionDir(loc), "note.md"), "x\n")
			},
			wantLoom: true,
		},
		{
			name: "only an archive sibling",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				writeFile(t, filepath.Join(loomengine.LoomDurableDir(loc), "friction-20260101T000000", "note.md"), "x\n")
			},
			wantLoom: true,
		},
		{
			name: "drive report",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				writeFile(t, filepath.Join(shedrun.DriveReportsDir(loc, shedrun.SelfRunID), "report.md"), "x\n")
			},
			wantDrive: true,
		},
		{
			name: "friction note and drive report",
			setup: func(t *testing.T, loc *lyxcwd.Location) {
				writeFile(t, filepath.Join(loomengine.LoomFrictionDir(loc), "note.md"), "x\n")
				writeFile(t, filepath.Join(shedrun.DriveReportsDir(loc, shedrun.SelfRunID), "report.md"), "x\n")
			},
			wantLoom:  true,
			wantDrive: true,
		},
		{
			name:  "pending rejection, unclassed round",
			setup: pendingRejectionSetup(writeFile, `{"first_card":3}`, true),
			// The held directories are left out though each holds a file.
			wantDrive: true,
		},
		{
			name:      "pending rejection, classed but uncommitted round",
			setup:     pendingRejectionSetup(writeFile, `{"first_card":3,"class":"exempt"}`, true),
			wantDrive: true,
		},
		{
			name:        "no pending rejection",
			setup:       pendingRejectionSetup(writeFile, `{"first_card":3,"class":"exempt"}`, false),
			wantReviews: true,
			wantLoom:    true,
			wantDrive:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			tt.setup(t, location)
			want := []string{selfStatusRel(location)}
			if tt.wantReviews {
				want = append(want, loomengine.LoomReviewsDirRel())
			}
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

// pendingRejectionSetup returns a setup that writes a rework round record, a review file and a drive report,
// and a pending rejection when rejection is true.
func pendingRejectionSetup(writeFile func(t *testing.T, path, content string), record string, rejection bool) func(t *testing.T, loc *lyxcwd.Location) {
	return func(t *testing.T, loc *lyxcwd.Location) {
		writeFile(t, filepath.Join(loomengine.LoomReworkDir(loc), "round-1", "record.json"), record)
		writeFile(t, filepath.Join(loomengine.LoomReviewsDir(loc), "plan", "round-1-review.md"), "x\n")
		writeFile(t, filepath.Join(shedrun.DriveReportsDir(loc, shedrun.SelfRunID), "report.md"), "x\n")
		if rejection {
			writeFile(t, loomengine.LoomRejectionPath(loc), "{}\n")
		}
	}
}

// TestPlanCommitPathspec asserts the plan commit always names the plan directory, and adds webster's durable directory only when it holds a file or Plan-Write's rotation archived a run record under the plan:
// the moved-from deletions and the archive then land in one commit, while an empty or absent directory would make the pathspec match nothing and fail.
func TestPlanCommitPathspec(t *testing.T) {
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
		name        string
		setup       func(t *testing.T, anchorPath string)
		wantWebster bool
	}{
		{name: "no webster directory", setup: func(t *testing.T, anchorPath string) {}},
		{
			name: "empty webster and archive directories",
			setup: func(t *testing.T, anchorPath string) {
				for _, d := range []string{websterengine.Dir(anchorPath), filepath.Join(planparser.PlanDir(anchorPath), planparser.ArchiveDirName("20260101T000000Z", ""), "webster")} {
					if err := os.MkdirAll(d, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "webster directory holding a file",
			setup: func(t *testing.T, anchorPath string) {
				writeFile(t, filepath.Join(websterengine.Dir(anchorPath), "state.json"))
			},
			wantWebster: true,
		},
		{
			name: "record moved into the archive with the webster directory empty",
			setup: func(t *testing.T, anchorPath string) {
				if err := os.MkdirAll(websterengine.Dir(anchorPath), 0o755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(planparser.PlanDir(anchorPath), planparser.ArchiveDirName("20260101T000000Z", ""), "webster", "state.json"))
			},
			wantWebster: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			tt.setup(t, location.AnchorPath())
			want := []string{planparser.PlanDirRel()}
			if tt.wantWebster {
				want = append(want, websterengine.DirRel())
			}
			if got := planCommitPathspec(location); !reflect.DeepEqual(got, want) {
				t.Errorf("planCommitPathspec() = %v; want %v", got, want)
			}
		})
	}
}
