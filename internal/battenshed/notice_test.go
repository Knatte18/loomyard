// notice_test.go covers the notice step: the rendered line, each condition's one notice per episode, the quiet condition's guards, and a restart over the same scratch directory.

package battenshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// noticeHarness drives one producer whose status, driver liveness and status-file time a test sets between Calls.
type noticeHarness struct {
	t          *testing.T
	scratch    string
	statusPath string
	clock      *fakeClock
	status     shedengine.Status
	alive      bool
	notified   []string
	notifyErr  error
	quiet      time.Duration
}

func newNoticeHarness(t *testing.T) *noticeHarness {
	t.Helper()
	dir := t.TempDir()
	h := &noticeHarness{
		t:          t,
		scratch:    filepath.Join(dir, "scratch"),
		statusPath: filepath.Join(dir, "status.json"),
		clock:      &fakeClock{},
		status:     shedengine.Status{State: shedengine.StateRunning},
		alive:      true,
		quiet:      45 * time.Minute,
	}
	h.touch(h.clock.Now())
	return h
}

// touch sets the status file's modification time, creating the file when absent.
func (h *noticeHarness) touch(at time.Time) {
	h.t.Helper()
	if err := os.WriteFile(h.statusPath, []byte("{}"), 0o644); err != nil {
		h.t.Fatal(err)
	}
	if err := os.Chtimes(h.statusPath, at, at); err != nil {
		h.t.Fatal(err)
	}
}

// producer builds a fresh producer over the harness's scratch directory, so a second call to it models a restart.
func (h *noticeHarness) producer() shedengine.ShedProducer {
	deps := InnerRunDeps{
		Spawn:         func(context.Context) error { return nil },
		ResolveStatus: func() (string, string, error) { return h.statusPath, "", nil },
		ReadStatus: func(string, string) (shedengine.Status, bool, error) {
			return h.status, true, nil
		},
		Sleep:        h.clock.Sleep,
		Now:          h.clock.Now,
		ReadDecision: func() (ChildDecision, bool, error) { return ChildDecision{}, false, nil },
		DriverAlive:  func(context.Context) (bool, error) { return h.alive, nil },
		Notify: func(_ context.Context, line string) error {
			h.notified = append(h.notified, line)
			return h.notifyErr
		},
		NoticeQuiet: h.quiet,
		AttachDir:   func() (string, error) { return "/wt/task", nil },
	}
	return NewInnerRun("Run-Shed", "task", deps, time.Second, h.scratch, testGrace)
}

func (h *noticeHarness) call(p shedengine.ShedProducer) {
	h.t.Helper()
	if _, _, err := p.Call(context.Background()); err != nil {
		h.t.Fatalf("Call() error = %v; want nil", err)
	}
}

func TestRenderNotice_Shape(t *testing.T) {
	status := shedengine.Status{State: shedengine.StateFailed, Error: "boom\nsecond\x1b[0m line " + strings.Repeat("x", 300)}
	line := renderNotice("task", "child left running", status, "/wt/task")

	if !strings.HasPrefix(line, noticePrefix) {
		t.Errorf("notice = %q; want prefix %q", line, noticePrefix)
	}
	if strings.ContainsAny(line, "\n\r\x1b") {
		t.Errorf("notice = %q; want one line without control characters", line)
	}
	if !strings.HasSuffix(line, "cd /wt/task && lyx reed attach") {
		t.Errorf("notice = %q; want the attach command at the end", line)
	}
	for _, want := range []string{"task", "child left running", "failed", "boom second"} {
		if !strings.Contains(line, want) {
			t.Errorf("notice = %q; want it to contain %q", line, want)
		}
	}
	const head = "boom\nsecond\x1b[0m line "
	kept := strings.Repeat("x", noticeErrorMax-len([]rune(head)))
	if !strings.Contains(line, kept+";") {
		t.Errorf("notice = %q; want the error cut to %d characters", line, noticeErrorMax)
	}
}

func TestRenderNotice_NoErrorOmitsIt(t *testing.T) {
	line := renderNotice("task", "child left running", shedengine.Status{State: shedengine.StateDone}, "/wt")
	if strings.Contains(line, "error:") {
		t.Errorf("notice = %q; want no error clause", line)
	}
}

func TestNotice_StateChangeNotifiesOncePerEpisode(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()

	h.call(p)
	if len(h.notified) != 0 {
		t.Fatalf("notified = %v; want none for a live running child", h.notified)
	}

	h.status = shedengine.Status{State: shedengine.StateBlocked, Error: "stuck"}
	h.call(p)
	h.call(p)
	if len(h.notified) != 1 || !strings.Contains(h.notified[0], "blocked") {
		t.Fatalf("notified = %v; want exactly one blocked notice across two Calls", h.notified)
	}
}

func TestNotice_StatusChangeStartsNewEpisode(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()
	h.status = shedengine.Status{State: shedengine.StatePaused}
	h.call(p)

	h.touch(h.clock.Now().Add(time.Minute))
	h.call(p)
	if len(h.notified) != 2 {
		t.Fatalf("notified = %v; want a second notice once the status file changed", h.notified)
	}
}

func TestNotice_ReturnToRunningStartsNewEpisode(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()
	h.status = shedengine.Status{State: shedengine.StateAwaiting}
	h.call(p)

	h.status = shedengine.Status{State: shedengine.StateRunning}
	h.call(p)
	h.status = shedengine.Status{State: shedengine.StateAwaiting}
	h.call(p)
	if len(h.notified) != 2 {
		t.Fatalf("notified = %v; want one notice per awaiting episode", h.notified)
	}
}

func TestNotice_DeadDriverWhileRunning(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()
	h.alive = false

	h.call(p)
	h.call(p)
	if len(h.notified) != 1 || !strings.Contains(h.notified[0], "driver strand is dead") {
		t.Fatalf("notified = %v; want exactly one dead-driver notice", h.notified)
	}
}

func TestNotice_QuietOnlyForRunningChildWithLiveDriver(t *testing.T) {
	t.Run("FiresAfterTheWindow", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.clock.advance(44 * time.Minute)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none inside the quiet window", h.notified)
		}
		h.clock.advance(time.Minute)
		h.call(p)
		h.call(p)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "unchanged for 45m0s") {
			t.Fatalf("notified = %v; want exactly one quiet notice", h.notified)
		}
	})

	t.Run("NotForADeadDriver", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.alive = false
		h.clock.advance(time.Hour)
		h.call(p)
		if len(h.notified) != 1 || strings.Contains(h.notified[0], "unchanged") {
			t.Fatalf("notified = %v; want the dead-driver notice only", h.notified)
		}
	})

	t.Run("NotForAHaltedChild", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.status = shedengine.Status{State: shedengine.StatePaused}
		h.clock.advance(time.Hour)
		h.call(p)
		if len(h.notified) != 1 || strings.Contains(h.notified[0], "unchanged") {
			t.Fatalf("notified = %v; want the state-change notice only", h.notified)
		}
	})

	t.Run("ZeroWindowDisablesIt", func(t *testing.T) {
		h := newNoticeHarness(t)
		h.quiet = 0
		p := h.producer()
		h.clock.advance(24 * time.Hour)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none with the quiet window off", h.notified)
		}
	})
}

func TestNotice_RestartDoesNotRenotify(t *testing.T) {
	h := newNoticeHarness(t)
	h.status = shedengine.Status{State: shedengine.StateFailed}
	h.call(h.producer())
	h.call(h.producer())
	if len(h.notified) != 1 {
		t.Fatalf("notified = %v; want one notice across a restart", h.notified)
	}
}

func TestNotice_NotifyErrorDoesNotChangeOutcome(t *testing.T) {
	h := newNoticeHarness(t)
	h.notifyErr = errors.New("queue unavailable")
	h.status = shedengine.Status{State: shedengine.StateBlocked}
	p := h.producer()

	outcome, _, err := p.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = %v, %v; want Stuck, nil", outcome, err)
	}
	h.call(p)
	if len(h.notified) != 1 {
		t.Fatalf("notified = %v; want a failed delivery still counted as the episode's notice", h.notified)
	}
}

func TestNotice_NilNotifyRunsNoStep(t *testing.T) {
	h := newNoticeHarness(t)
	h.status = shedengine.Status{State: shedengine.StateBlocked}
	deps := InnerRunDeps{
		Spawn:         func(context.Context) error { return nil },
		ResolveStatus: func() (string, string, error) { return h.statusPath, "", nil },
		ReadStatus:    func(string, string) (shedengine.Status, bool, error) { return h.status, true, nil },
		Sleep:         h.clock.Sleep,
		Now:           h.clock.Now,
		ReadDecision:  func() (ChildDecision, bool, error) { return ChildDecision{}, false, nil },
		DriverAlive:   func(context.Context) (bool, error) { return true, nil },
	}
	p := NewInnerRun("Run-Shed", "task", deps, time.Second, h.scratch, testGrace)
	h.call(p)
	if _, err := os.Stat(noticeEpisodeFile(h.scratch, "Run-Shed")); err == nil {
		t.Errorf("episode marker exists; want no notice bookkeeping without a Notify")
	}
}
