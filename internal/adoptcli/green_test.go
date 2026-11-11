// green_test.go pins the green end of the adopt check over fake seams and a temp subject directory:
// the fix task's removal and the try count.

package adoptcli

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// greenFixture is a test fixture: a temp adopt directory holding the subject `alpha` and a second subject `beta`.
// Every seam is faked to record its calls.
type greenFixture struct {
	t        *testing.T
	deps     greenDeps
	root     string
	dir      string
	otherDir string

	tasks []boardengine.Task

	cancelled      []string
	cancelErr      error
	removedEntries []string
	removeEntryErr error
}

func newGreenFixture(t *testing.T) *greenFixture {
	t.Helper()
	root := t.TempDir()
	f := &greenFixture{t: t, root: root, dir: filepath.Join(root, "alpha"), otherDir: filepath.Join(root, "beta")}
	f.deps = greenDeps{
		Subject:  "alpha",
		Dir:      f.dir,
		Worktree: "alpha",
		Project:  "alpha",
		Tasks:    func() ([]boardengine.Task, error) { return f.tasks, nil },
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

// fixTask returns the front task of alpha's adopt check, with the given slug and status.
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
