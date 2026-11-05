// retiredmove_test.go covers MoveOffRetired over a temp beam directory and told seams that log their calls in order: what it carries, removes, writes and posts, and that a retried move repeats safely.

package pawlengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	moveHead       = "0123abc"
	moveScratchURL = "https://github.com/o/r/pull/7"
	moveRetired    = "PR-Review"
	moveTarget     = "Beam-Await"
)

// moveWorld is a beam directory holding the records a move off a retired row removes or carries, and the ordered record of the seams' calls.
type moveWorld struct {
	paths                                          pawlengine.Paths
	landingPath, landingLock, readyPath, readyLock string
	scratchPath                                    string

	// open and openErr are what the pull request state seam answers.
	open    bool
	openErr error
	// commitErr fails the commit seam, and notifyErr the notice.
	commitErr, notifyErr error

	events  []string
	notices []string
	// removedAtCommit holds whether every record a move removes was gone when the commit seam ran.
	removedAtCommit []bool
}

// newMoveWorld returns a beam directory holding a ready record, a land request, a dobby-stop marker, a reopen request and a scratch pull request record, whose pull request GitHub reports open.
func newMoveWorld(t *testing.T) *moveWorld {
	t.Helper()
	dir := t.TempDir()
	w := &moveWorld{
		paths: pawlengine.Paths{
			LandRequest:     filepath.Join(dir, "beam", pawlengine.LandRequestFileName),
			ReopenRequest:   filepath.Join(dir, "beam", pawlengine.ReopenRequestFileName),
			DobbyStopMarker: filepath.Join(dir, "beam", pawlengine.DobbyStopMarkerName),
		},
		landingPath: filepath.Join(dir, "landing", "beam-landing.json"),
		landingLock: filepath.Join(dir, "locks", "beam-landing.json.lock"),
		readyPath:   filepath.Join(dir, "landing", "beam-ready.json"),
		readyLock:   filepath.Join(dir, "locks", "beam-ready.json.lock"),
		scratchPath: filepath.Join(dir, "scratch", "pull-request.json"),
		open:        true,
	}
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	if err := pawlengine.WriteReady(w.readyPath, w.readyLock, pawlengine.ReadyRecord{Slug: testSlug, SetAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := pawlengine.WriteLandRequest(w.paths, pawlengine.LandRequest{RunID: testRunID, RequestedAt: at}); err != nil {
		t.Fatal(err)
	}
	touch(t, w.paths.DobbyStopMarker)
	touch(t, w.paths.ReopenRequest)
	touch(t, w.scratchPath)
	return w
}

// removable lists the records a move removes, bar the scratch record.
func (w *moveWorld) removable() []string {
	return []string{w.readyPath, w.paths.LandRequest, w.paths.DobbyStopMarker, w.paths.ReopenRequest}
}

func (w *moveWorld) deps() pawlengine.RetiredMoveDeps {
	return pawlengine.RetiredMoveDeps{
		Slug:            testSlug,
		Paths:           w.paths,
		LandingPath:     w.landingPath,
		LandingLockPath: w.landingLock,
		ReadyPath:       w.readyPath,
		ReadyLockPath:   w.readyLock,
		EndSession: func() error {
			w.events = append(w.events, "end-session")
			return nil
		},
		ScratchPullRequest: func() (int, string, bool, error) {
			return 7, moveScratchURL, exists(w.scratchPath), nil
		},
		PullRequestOpen: func(number int) (bool, error) {
			w.events = append(w.events, "pull-request-open")
			return w.open, w.openErr
		},
		BeamHead: func() (string, error) { return moveHead, nil },
		RemoveScratch: func() error {
			w.events = append(w.events, "remove-scratch")
			if err := os.Remove(w.scratchPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		},
		Commit: func() error {
			w.events = append(w.events, "commit")
			gone := true
			for _, path := range w.removable() {
				gone = gone && !exists(path)
			}
			w.removedAtCommit = append(w.removedAtCommit, gone)
			return w.commitErr
		},
		SetStatus: func(producer string) error {
			w.events = append(w.events, "status:"+producer)
			return nil
		},
		Notify: func(line string) error {
			w.events = append(w.events, "notify")
			w.notices = append(w.notices, line)
			return w.notifyErr
		},
	}
}

func TestMoveOffRetired(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("the scratch pull request is carried unless GitHub reports it closed, and the rest of the move happens either way", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name        string
			scratch     bool
			open        bool
			openErr     error
			wantCarried bool
		}{
			{"OpenPullRequestIsCarried", true, true, nil, true},
			{"UnreadableStateIsCarried", true, false, errors.New("github unreachable"), true},
			{"ClosedPullRequestIsNotCarried", true, false, nil, false},
			{"NoScratchRecordCarriesNothing", false, true, nil, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				w := newMoveWorld(t)
				w.open, w.openErr = tt.open, tt.openErr
				if !tt.scratch {
					if err := os.Remove(w.scratchPath); err != nil {
						t.Fatal(err)
					}
				}

				if err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, false); err != nil {
					t.Fatalf("MoveOffRetired() = %v; want nil", err)
				}

				record, found, err := pawlengine.ReadLanding(w.landingPath, w.landingLock, testSlug)
				if err != nil {
					t.Fatal(err)
				}
				switch {
				case tt.wantCarried && (!found || record.PullRequest == nil || *record.PullRequest != pawlengine.LandingPullRequest{Number: 7, URL: moveScratchURL, PushedHead: moveHead, AnnounceHead: moveHead}):
					t.Errorf("landing record = %+v, %v; want the pull request carried with the beam head as pushed and announce head", record, found)
				case !tt.wantCarried && found && record.PullRequest != nil:
					t.Errorf("landing record = %+v; want no pull request carried", record)
				}
				if exists(w.scratchPath) {
					t.Error("the scratch pull request record stands; want it removed")
				}
				for _, path := range w.removable() {
					if exists(path) {
						t.Errorf("%s stands; want it removed", filepath.Base(path))
					}
				}
				if slices.Equal(w.removedAtCommit, []bool{true}) == false {
					t.Errorf("records gone at each commit = %v; want one commit after every removal", w.removedAtCommit)
				}
				if got := countOf(w.events, "end-session"); got != 1 {
					t.Errorf("EndSession called %d times; want once", got)
				}
				if want := []string{"status:" + moveTarget, "notify"}; !slices.Equal(w.events[len(w.events)-2:], want) {
					t.Errorf("last events = %v; want the status write and then the notice %v", w.events[len(w.events)-2:], want)
				}
				if len(w.notices) != 1 {
					t.Fatalf("notices = %q; want one", w.notices)
				}
				notice := w.notices[0]
				for _, want := range []string{"lym beam start " + testSlug, moveRetired, moveTarget, "lym beam describe", "lym beam ready"} {
					if !strings.Contains(notice, want) {
						t.Errorf("notice = %q; want it to carry %q", notice, want)
					}
				}
				if got := strings.Count(notice, "lym loom resume"); got != 1 {
					t.Errorf("notice names `lym loom resume` %d times; want once", got)
				}
				if got := strings.Contains(notice, moveScratchURL); got != tt.wantCarried {
					t.Errorf("notice names the pull request = %v; want %v", got, tt.wantCarried)
				}
			})
		}
	})

	t.Run("a resumed move names the resume twice", func(t *testing.T) {
		t.Parallel()
		w := newMoveWorld(t)

		if err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, true); err != nil {
			t.Fatalf("MoveOffRetired() = %v; want nil", err)
		}

		if got := strings.Count(w.notices[0], "lym loom resume"); got != 2 {
			t.Errorf("notice = %q; want `lym loom resume` twice, got %d", w.notices[0], got)
		}
	})

	t.Run("a failing commit returns its step before the status is written", func(t *testing.T) {
		t.Parallel()
		w := newMoveWorld(t)
		w.commitErr = errors.New("index locked")

		err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, false)

		if !errors.Is(err, w.commitErr) || !strings.Contains(err.Error(), "commit the landing directory") {
			t.Fatalf("MoveOffRetired() = %v; want the commit's error naming its step", err)
		}
		if slices.ContainsFunc(w.events, func(event string) bool { return strings.HasPrefix(event, "status:") || event == "notify" }) {
			t.Errorf("events = %v; want no status write or notice after the failed commit", w.events)
		}
	})

	t.Run("a move repeated after a failed commit names the pull request the landing record holds", func(t *testing.T) {
		t.Parallel()
		w := newMoveWorld(t)
		w.commitErr = errors.New("index locked")
		if err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, false); err == nil {
			t.Fatal("first MoveOffRetired() = nil; want the commit's error")
		}
		w.commitErr = nil

		if err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, false); err != nil {
			t.Fatalf("second MoveOffRetired() = %v; want nil", err)
		}

		if len(w.notices) != 1 || !strings.Contains(w.notices[0], moveScratchURL) {
			t.Errorf("notices = %q; want one naming the pull request %s", w.notices, moveScratchURL)
		}
	})

	t.Run("a repeated move leaves a landing record pull request GitHub reports closed out of the notice", func(t *testing.T) {
		t.Parallel()
		w := newMoveWorld(t)
		w.commitErr = errors.New("index locked")
		if err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, false); err == nil {
			t.Fatal("first MoveOffRetired() = nil; want the commit's error")
		}
		w.commitErr = nil
		w.open = false

		if err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, false); err != nil {
			t.Fatalf("second MoveOffRetired() = %v; want nil", err)
		}

		if len(w.notices) != 1 || strings.Contains(w.notices[0], moveScratchURL) {
			t.Errorf("notices = %q; want one that does not name the closed pull request %s", w.notices, moveScratchURL)
		}
	})

	t.Run("a failed notice is only logged", func(t *testing.T) {
		t.Parallel()
		w := newMoveWorld(t)
		w.notifyErr = errors.New("no orch strand is recorded")

		if err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, false); err != nil {
			t.Errorf("MoveOffRetired() = %v; want nil", err)
		}
	})

	t.Run("a second move after a full one changes nothing on disk", func(t *testing.T) {
		t.Parallel()
		w := newMoveWorld(t)
		if err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, false); err != nil {
			t.Fatalf("first MoveOffRetired() = %v; want nil", err)
		}
		landing, err := os.ReadFile(w.landingPath)
		if err != nil {
			t.Fatal(err)
		}

		if err := pawlengine.MoveOffRetired(w.deps(), moveRetired, moveTarget, false); err != nil {
			t.Fatalf("second MoveOffRetired() = %v; want nil", err)
		}

		after, err := os.ReadFile(w.landingPath)
		if err != nil || string(after) != string(landing) {
			t.Errorf("landing record after the second move = %q, %v; want it unchanged %q", after, err, landing)
		}
		for _, path := range append(w.removable(), w.scratchPath) {
			if exists(path) {
				t.Errorf("%s stands after the second move; want it still removed", filepath.Base(path))
			}
		}
	})

	t.Run("a nil seam is refused naming the field", func(t *testing.T) {
		t.Parallel()
		w := newMoveWorld(t)
		deps := w.deps()
		deps.Commit = nil

		err := pawlengine.MoveOffRetired(deps, moveRetired, moveTarget, false)

		if err == nil || !strings.Contains(err.Error(), "RetiredMoveDeps.Commit") {
			t.Errorf("MoveOffRetired() = %v; want a refusal naming RetiredMoveDeps.Commit", err)
		}
		if len(w.events) != 0 {
			t.Errorf("events = %v; want none before the refusal", w.events)
		}
	})
}

// countOf returns how many of events equal event.
func countOf(events []string, event string) int {
	count := 0
	for _, got := range events {
		if got == event {
			count++
		}
	}
	return count
}
