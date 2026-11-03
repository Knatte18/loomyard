// green_test.go pins the green end of the adopt check over fake seams and a temp subject directory: the lifts, the pair, the first passes, the resumed children, the fix task's removal and the try count.

package adoptcli

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// greenHoldTime is the pause time every hold entry of a green fixture carries.
var greenHoldTime = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

// greenFixture is a greenEnd over a temp adopt directory holding the subject `alpha` and a second subject `beta`, with every seam faked and recording its calls.
type greenFixture struct {
	t        *testing.T
	deps     greenDeps
	root     string
	dir      string
	otherDir string

	record     pauserecord.Record
	liftErrs   map[string]error
	lifted     []string
	beamUps    []string
	beamUpErr  error
	runRecords map[string]shedengine.Status
	started    []string
	envelopes  map[string]map[string]any
	tasks      []boardengine.Task
	parents    map[string]string
	children   map[string]shedengine.Status
	owners     map[string]string
	resumed    []string
	resumeErrs map[string]error
	statusErrs map[string]error

	cancelled      []string
	cancelErr      error
	removedEntries []string
	removeEntryErr error
	// afterPauseRead runs when the pause record is read, which is after the holds file was read.
	afterPauseRead func()
}

func newGreenFixture(t *testing.T) *greenFixture {
	t.Helper()
	root := t.TempDir()
	f := &greenFixture{
		t: t, root: root, dir: filepath.Join(root, "alpha"), otherDir: filepath.Join(root, "beta"),
		liftErrs: map[string]error{}, runRecords: map[string]shedengine.Status{}, envelopes: map[string]map[string]any{},
		parents: map[string]string{}, children: map[string]shedengine.Status{}, owners: map[string]string{}, resumeErrs: map[string]error{}, statusErrs: map[string]error{},
	}
	f.deps = greenDeps{
		Subject:  "alpha",
		Dir:      f.dir,
		Worktree: "alpha",
		Project:  "alpha",
		PauseRecord: func() (pauserecord.Record, error) {
			if f.afterPauseRead != nil {
				f.afterPauseRead()
			}
			return f.record, nil
		},
		Lift: func(ref tenderengine.EntryRef) error {
			if err := f.liftErrs[ref.Scope]; err != nil {
				return err
			}
			if ref.Caller != pauserecord.HoldCaller || !ref.PausedAt.Equal(greenHoldTime) {
				t.Errorf("Lift(%+v); want the hold caller and the entry's pause time", ref)
			}
			f.lifted = append(f.lifted, ref.Scope)
			return nil
		},
		BeamUp: func(worktree string) error {
			f.beamUps = append(f.beamUps, worktree)
			return f.beamUpErr
		},
		RunRecord: func(slug string) (shedengine.Status, bool, error) {
			status, found := f.runRecords[slug]
			return status, found, nil
		},
		StartFirstPass: func(slug string) (map[string]any, error) {
			f.started = append(f.started, slug)
			if envelope, refused := f.envelopes[slug]; refused {
				return envelope, nil
			}
			return map[string]any{"ok": true}, nil
		},
		Tasks:    func() ([]boardengine.Task, error) { return f.tasks, nil },
		ParentOf: func(slug string) (string, error) { return f.parents[slug], nil },
		ChildStatus: func(slug string) (shedengine.Status, bool, error) {
			if err := f.statusErrs[slug]; err != nil {
				return shedengine.Status{}, false, err
			}
			status, found := f.children[slug]
			return status, found, nil
		},
		ResumeChild: func(slug string) error {
			if err := f.resumeErrs[slug]; err != nil {
				return err
			}
			f.resumed = append(f.resumed, slug)
			return nil
		},
		Owner: func(slug string) (string, error) { return f.owners[slug], nil },
		FrontDeps: fronttask.Deps{
			Tasks: func() ([]boardengine.Task, error) { return f.tasks, nil },
			HasRun: func(slug string) (bool, error) {
				for _, task := range f.tasks {
					if task.Slug == slug {
						return boardengine.IsRunStatus(task.Status), nil
					}
				}
				return false, nil
			},
			Cancel: func(_ context.Context, slug string) error {
				f.cancelled = append(f.cancelled, slug)
				return f.cancelErr
			},
			RemoveEntry: func(slug string) error {
				if f.removeEntryErr != nil {
					return f.removeEntryErr
				}
				f.removedEntries = append(f.removedEntries, slug)
				return nil
			},
			RecordPath: filepath.Join(root, "front-records.json"),
		},
	}
	return f
}

// hold lists slugs in the holds file of dir, and gives each of them a live hold entry when entry is set.
func (f *greenFixture) hold(dir string, entry bool, slugs ...string) {
	f.t.Helper()
	if err := appendHolds(dir, slugs); err != nil {
		f.t.Fatalf("appendHolds(%s): %v", dir, err)
	}
	if entry {
		for _, slug := range slugs {
			f.record.Live = append(f.record.Live, pauserecord.Entry{Scope: slug, PausedAt: greenHoldTime, GraceEnd: greenHoldTime, Caller: pauserecord.HoldCaller})
		}
	}
}

func (f *greenFixture) holds(dir string) []string {
	f.t.Helper()
	got, err := readHolds(dir)
	if err != nil {
		f.t.Fatalf("readHolds(%s): %v", dir, err)
	}
	return got
}

// fixTask returns the front task of alpha's adopt check with status.
func fixTask(slug string, status *string) boardengine.Task {
	return boardengine.Task{Slug: slug, Kind: boardengine.KindTask, Front: true, OnLand: fronttask.ActionAdoptCheck, Project: "alpha", Status: status}
}

func statusOf(text string) *string { return &text }

func mentions(lines []string, wants ...string) bool {
	return slices.ContainsFunc(lines, func(line string) bool {
		for _, want := range wants {
			if !strings.Contains(line, want) {
				return false
			}
		}
		return true
	})
}

func TestGreenEnd(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("lifts the standing holds of its subject and leaves the holds appended meanwhile and another subject's", func(t *testing.T) {
		t.Parallel()
		f := newGreenFixture(t)
		f.hold(f.dir, true, "t1", "t2")
		f.hold(f.otherDir, true, "x1")
		f.afterPauseRead = func() { f.hold(f.dir, true, "t3") }

		if lines := greenEnd(f.deps); len(lines) != 0 {
			t.Errorf("greenEnd() = %q; want no line", lines)
		}
		if !slices.Equal(f.lifted, []string{"t1", "t2"}) {
			t.Errorf("lifted = %q; want t1, t2", f.lifted)
		}
		if got := f.holds(f.dir); !slices.Equal(got, []string{"t3"}) {
			t.Errorf("holds of alpha = %q; want only t3, appended after the read", got)
		}
		if got := f.holds(f.otherDir); !slices.Equal(got, []string{"x1"}) {
			t.Errorf("holds of beta = %q; want x1 left", got)
		}
	})

	t.Run("a hold lifted by hand meanwhile is skipped and its run is not started", func(t *testing.T) {
		t.Parallel()
		f := newGreenFixture(t)
		f.hold(f.dir, true, "t1")
		f.hold(f.dir, false, "t2")
		f.runRecords["t1"] = shedengine.Status{State: shedengine.StatePaused}
		f.runRecords["t2"] = shedengine.Status{State: shedengine.StatePaused}

		if lines := greenEnd(f.deps); len(lines) != 0 {
			t.Errorf("greenEnd() = %q; want no line", lines)
		}
		if !slices.Equal(f.lifted, []string{"t1"}) || !slices.Equal(f.started, []string{"t1"}) {
			t.Errorf("lifted = %q, started = %q; want only t1", f.lifted, f.started)
		}
		if got := f.holds(f.dir); len(got) != 0 {
			t.Errorf("holds = %q; want both listed slugs dropped", got)
		}
	})

	t.Run("a failed lift names the hand resume and the other steps go on", func(t *testing.T) {
		t.Parallel()
		f := newGreenFixture(t)
		f.hold(f.dir, true, "t1", "t2")
		f.liftErrs["t1"] = errors.New("pause record locked")

		lines := greenEnd(f.deps)
		if !mentions(lines, "lym tender resume t1", "pause record locked") {
			t.Errorf("greenEnd() = %q; want a line naming `lym tender resume t1`", lines)
		}
		if !slices.Equal(f.lifted, []string{"t2"}) || !slices.Equal(f.beamUps, []string{"alpha"}) {
			t.Errorf("lifted = %q, beam ups = %q; want t2 lifted and the pair brought up", f.lifted, f.beamUps)
		}
	})

	t.Run("a failed pair bring-up names lym beam start", func(t *testing.T) {
		t.Parallel()
		f := newGreenFixture(t)
		f.beamUpErr = errors.New("no tmux")

		if lines := greenEnd(f.deps); !mentions(lines, "lym beam start alpha", "no tmux") {
			t.Errorf("greenEnd() = %q; want a line naming `lym beam start alpha`", lines)
		}
	})

	t.Run("starts a first pass per lifted run with a record and none for a released task or a done run, a refusal naming the hand pass", func(t *testing.T) {
		t.Parallel()
		f := newGreenFixture(t)
		f.hold(f.dir, true, "run", "released", "finished", "refused")
		f.runRecords["run"] = shedengine.Status{State: shedengine.StatePaused}
		f.runRecords["finished"] = shedengine.Status{State: shedengine.StateDone}
		f.runRecords["refused"] = shedengine.Status{State: shedengine.StatePaused}
		f.envelopes["refused"] = map[string]any{"ok": false, "error": "run status covers it"}

		lines := greenEnd(f.deps)
		if want := []string{"run", "refused"}; !slices.Equal(f.started, want) {
			t.Errorf("started = %q; want %q", f.started, want)
		}
		if len(lines) != 1 || !mentions(lines, "lym batten run refused", "run status covers it") {
			t.Errorf("greenEnd() = %q; want one line naming `lym batten run refused`", lines)
		}
	})

	t.Run("resumes the children of its subject halted at Finalize's owner check that this machine owns", func(t *testing.T) {
		t.Parallel()
		ownerHalt := shedengine.Status{State: shedengine.StateBlocked, Error: landingshed.OwnerHaltPrefix + "the parent is owned elsewhere"}
		tests := []struct {
			name        string
			subject     string
			child       string
			status      shedengine.Status
			parent      string
			owner       string
			wantResumed bool
		}{
			{"HaltedUnderTheSubject", "alpha", "c1", ownerHalt, "alpha", "", true},
			{"HaltedForAnotherReason", "alpha", "c1", shedengine.Status{State: shedengine.StateBlocked, Error: "tests failed"}, "alpha", "", false},
			{"NotBlocked", "alpha", "c1", shedengine.Status{State: shedengine.StateRunning}, "alpha", "", false},
			{"UnderAnotherBeam", "alpha", "c1", ownerHalt, "beta", "", false},
			{"UnderThePrimeWhileTheSubjectIsABeam", "alpha", "c1", ownerHalt, "", "", false},
			{"OwnedByAnotherMachine", "alpha", "c1", ownerHalt, "alpha", "otherbox", false},
			{"UnderThePrimeForRoot", "root", "c1", ownerHalt, "", "", true},
			{"UnderABeamForRoot", "root", "c1", ownerHalt, "alpha", "", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newGreenFixture(t)
				f.deps.Subject = tt.subject
				f.tasks = []boardengine.Task{{Slug: tt.child, Kind: boardengine.KindTask}}
				f.children[tt.child] = tt.status
				f.parents[tt.child] = tt.parent
				f.owners[tt.child] = tt.owner

				if lines := greenEnd(f.deps); len(lines) != 0 {
					t.Errorf("greenEnd() = %q; want no line", lines)
				}
				if got := slices.Contains(f.resumed, tt.child); got != tt.wantResumed {
					t.Errorf("resumed = %q; want %s resumed: %v", f.resumed, tt.child, tt.wantResumed)
				}
			})
		}

		t.Run("AFailedStatusReadOfAnotherSubjectsChildAddsNoLine", func(t *testing.T) {
			t.Parallel()
			f := newGreenFixture(t)
			f.tasks = []boardengine.Task{{Slug: "c1", Kind: boardengine.KindTask}, {Slug: "c2", Kind: boardengine.KindTask}}
			f.parents["c1"] = "beta"
			f.statusErrs["c1"] = errors.New("status unreadable")
			f.parents["c2"] = "alpha"
			f.statusErrs["c2"] = errors.New("own status unreadable")

			lines := greenEnd(f.deps)
			if mentions(lines, "c1") {
				t.Errorf("greenEnd() = %q; want no line naming c1, a child of another subject", lines)
			}
			if !mentions(lines, "status of c2", "own status unreadable") {
				t.Errorf("greenEnd() = %q; want a line for the unreadable status of c2, a child of this subject", lines)
			}
		})

		t.Run("AFailedResumeNamesLymLoomResume", func(t *testing.T) {
			t.Parallel()
			f := newGreenFixture(t)
			f.tasks = []boardengine.Task{{Slug: "c1", Kind: boardengine.KindTask}}
			f.children["c1"] = ownerHalt
			f.parents["c1"] = "alpha"
			f.resumeErrs["c1"] = errors.New("exit 1")

			if lines := greenEnd(f.deps); !mentions(lines, "`lym loom resume` in the pair of c1", "exit 1") {
				t.Errorf("greenEnd() = %q; want a line naming `lym loom resume` in c1's pair", lines)
			}
		})
	})

	t.Run("removes the fix task in whatever status it stands, ending a running one first", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			status     *string
			wantCancel bool
		}{
			{"Draft", statusOf("draft"), false},
			{"ReleasedThatNeverStarted", nil, false},
			{"Running", statusOf(boardengine.RunStatus("running", "Code-Write")), true},
			{"Halted", statusOf(boardengine.RunStatus("blocked", "Code-Write")), true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newGreenFixture(t)
				f.tasks = []boardengine.Task{fixTask("adopt-fix-alpha-t1", tt.status)}

				if lines := greenEnd(f.deps); len(lines) != 0 {
					t.Errorf("greenEnd() = %q; want no line", lines)
				}
				if !slices.Equal(f.removedEntries, []string{"adopt-fix-alpha-t1"}) || (len(f.cancelled) == 1) != tt.wantCancel {
					t.Errorf("removed = %q, cancelled = %q; want the entry removed, cancelled: %v", f.removedEntries, f.cancelled, tt.wantCancel)
				}
			})
		}

		t.Run("ADoneFixTaskAndAnotherProjectsAreLeft", func(t *testing.T) {
			t.Parallel()
			f := newGreenFixture(t)
			other := fixTask("adopt-fix-beta-t1", nil)
			other.Project = "beta"
			f.tasks = []boardengine.Task{fixTask("adopt-fix-alpha-t1", statusOf("done")), other}

			if lines := greenEnd(f.deps); len(lines) != 0 || len(f.removedEntries) != 0 {
				t.Errorf("greenEnd() = %q, removed = %q; want nothing removed", lines, f.removedEntries)
			}
		})

		t.Run("AFailedRemovalNamesItsHandRemoval", func(t *testing.T) {
			t.Parallel()
			f := newGreenFixture(t)
			f.tasks = []boardengine.Task{fixTask("adopt-fix-alpha-t1", nil)}
			f.removeEntryErr = errors.New("board locked")

			if lines := greenEnd(f.deps); !mentions(lines, "adopt-fix-alpha-t1", "board locked", "`lym board remove '{\"slug\":\"adopt-fix-alpha-t1\"}'`") {
				t.Errorf("greenEnd() = %q; want a line with the step's error and its hand removal", lines)
			}
		})
	})

	t.Run("writes the try count to 0", func(t *testing.T) {
		t.Parallel()
		f := newGreenFixture(t)
		if err := writeTries(f.dir, 2); err != nil {
			t.Fatal(err)
		}
		if err := writeTries(f.otherDir, 2); err != nil {
			t.Fatal(err)
		}

		greenEnd(f.deps)
		if got, err := readTries(f.dir); err != nil || got != 0 {
			t.Errorf("readTries(alpha) = %d, %v; want 0", got, err)
		}
		if got, err := readTries(f.otherDir); err != nil || got != 2 {
			t.Errorf("readTries(beta) = %d, %v; want its 2 left", got, err)
		}
	})
}
