// world_test.go declares fakeWorld, the in-memory hub the verb and daemon tests drive the engine over: paths under a temp directory, a wall and an awake clock, and a scripted fake of every seam.

package hibernateengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// statusReading is what one TenderStatus call answers besides the pause record's live entries.
type statusReading struct {
	sessions []tenderengine.SessionStatus
	netOps   []tenderengine.NetOpReport
	holds    []tenderengine.HeldBranch
	err      error
}

// fakeWorld is one test's hub.
// Every field the seams touch is guarded by mu, and a hook runs outside it, so a hook may call back into the world.
type fakeWorld struct {
	t           *testing.T
	mu          sync.Mutex
	requestPath string
	recordPath  string

	// wall and awake both advance on Sleep; pendingSleep makes the next Sleep move wall an hour past awake.
	wall         time.Time
	awake        time.Duration
	pendingSleep bool

	caller    string
	goos      string
	systemctl bool

	// nextPID is the pid the next Spawn hands out; spawned holds every pid handed out.
	nextPID  int
	spawnErr error
	spawned  []int

	selfPID   int
	selfStart string
	alive     map[int]string
	killed    []int
	// onKill runs inside Kill before the process is recorded dead.
	onKill func(pid int)

	// runs holds each slug's readings, consumed per call with the last repeated; runsMissing reports a slug as having no status.
	runs        map[string][]RunStatus
	runsMissing map[string]bool
	runSlugs    []string

	// landLock holds the land lock readings, consumed per call with the last repeated; no reading is not held.
	landLock []bool

	// stillBusy, refused, resumedDuringGrace and pauseErr are what the fake pause reports.
	stillBusy          []string
	refused            []tenderengine.SessionNote
	resumedDuringGrace bool
	pauseErr           error
	// onPause runs after the entry is added and before Added is called.
	onPause func()
	pauses  []tenderengine.PauseRequest

	// readings holds the status readings by call index, the last repeating; none reads an idle hub.
	readings    []statusReading
	statusCalls int
	// onStatus runs at the start of each status call with the call's index.
	onStatus func(call int)

	liftErr error

	sleeps  []time.Duration
	onSleep func()

	hibernateCalls int
	hibernateErr   error
	notices        []string
}

// newFakeWorld builds a Linux hub with systemctl on PATH, an external caller, and the clocks at a fixed instant.
func newFakeWorld(t *testing.T) *fakeWorld {
	t.Helper()
	root := t.TempDir()
	return &fakeWorld{
		t:           t,
		requestPath: filepath.Join(root, "hibernate", "request.json"),
		recordPath:  filepath.Join(root, "pause", pauserecord.RecordFileName),
		wall:        time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		goos:        "linux",
		systemctl:   true,
		nextPID:     1000,
		selfPID:     4242,
		selfStart:   "self-start",
		alive:       map[int]string{},
		runs:        map[string][]RunStatus{},
		runsMissing: map[string]bool{},
	}
}

func (w *fakeWorld) now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wall
}

func (w *fakeWorld) sleep(d time.Duration) {
	w.mu.Lock()
	w.sleeps = append(w.sleeps, d)
	w.wall = w.wall.Add(d)
	w.awake += d
	if w.pendingSleep {
		w.wall = w.wall.Add(time.Hour)
		w.pendingSleep = false
	}
	hook := w.onSleep
	w.mu.Unlock()
	if hook != nil {
		hook()
	}
}

func (w *fakeWorld) spawn() (int, string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.spawnErr != nil {
		return 0, "", w.spawnErr
	}
	pid := w.nextPID
	w.nextPID++
	start := fmt.Sprintf("start-%d", pid)
	w.alive[pid] = start
	w.spawned = append(w.spawned, pid)
	return pid, start, nil
}

func (w *fakeWorld) kill(pid int) error {
	w.mu.Lock()
	hook := w.onKill
	w.mu.Unlock()
	if hook != nil {
		hook(pid)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.killed = append(w.killed, pid)
	delete(w.alive, pid)
	return nil
}

func (w *fakeWorld) isAlive(pid int, start string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	got, ok := w.alive[pid]
	return ok && got == start
}

func (w *fakeWorld) runStatus(slug string) (RunStatus, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	readings := w.runs[slug]
	if w.runsMissing[slug] || len(readings) == 0 {
		return RunStatus{}, false, nil
	}
	if len(readings) > 1 {
		w.runs[slug] = readings[1:]
	}
	return readings[0], true, nil
}

func (w *fakeWorld) listRunSlugs() ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.runSlugs != nil {
		return append([]string(nil), w.runSlugs...), nil
	}
	var slugs []string
	for slug := range w.runs {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return slugs, nil
}

func (w *fakeWorld) landLockHeld() (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.landLock) == 0 {
		return false, nil
	}
	held := w.landLock[0]
	if len(w.landLock) > 1 {
		w.landLock = w.landLock[1:]
	}
	return held, nil
}

// pause adds a hub entry with the PauseCaller label unless one is live, runs onPause, then calls Added.
func (w *fakeWorld) pause(req tenderengine.PauseRequest) (tenderengine.PauseReport, error) {
	w.mu.Lock()
	w.pauses = append(w.pauses, req)
	pauseErr, hook := w.pauseErr, w.onPause
	report := tenderengine.PauseReport{
		Scope:              pauserecord.ScopeHub,
		StillBusy:          append([]string{}, w.stillBusy...),
		Refused:            append([]tenderengine.SessionNote{}, w.refused...),
		ResumedDuringGrace: w.resumedDuringGrace,
	}
	w.mu.Unlock()
	if pauseErr != nil {
		return report, pauseErr
	}

	now := w.now()
	var entry pauserecord.Entry
	_, err := pauserecord.Update(w.recordPath, now, func(record *pauserecord.Record) error {
		for _, live := range record.Live {
			if live.Scope == pauserecord.ScopeHub {
				report.AlreadyPaused = true
				return nil
			}
		}
		entry = pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: now, GraceEnd: now.Add(req.Grace), Caller: PauseCaller, Sessions: []pauserecord.Session{}}
		record.Live = append(record.Live, entry)
		return nil
	})
	if err != nil {
		return report, err
	}
	if hook != nil {
		hook()
	}
	if !report.AlreadyPaused && req.Added != nil {
		if err := req.Added(entry); err != nil {
			return report, fmt.Errorf("pause: report the added entry: %w", err)
		}
	}
	return report, nil
}

func (w *fakeWorld) tenderStatus() (tenderengine.StatusReport, error) {
	w.mu.Lock()
	call := w.statusCalls
	w.statusCalls++
	hook := w.onStatus
	w.mu.Unlock()
	if hook != nil {
		hook(call)
	}
	var reading statusReading
	w.mu.Lock()
	if len(w.readings) > 0 {
		reading = w.readings[min(call, len(w.readings)-1)]
	}
	w.mu.Unlock()
	if reading.err != nil {
		return tenderengine.StatusReport{}, reading.err
	}
	record, err := pauserecord.Read(w.recordPath)
	if err != nil {
		return tenderengine.StatusReport{}, err
	}
	report := tenderengine.StatusReport{Live: record.Live, Sessions: reading.sessions, NetOps: reading.netOps}
	report.Holds.Held = reading.holds
	return report, nil
}

// lift lifts by identity through the real tender engine over the world's record, unless a lift error is set.
func (w *fakeWorld) lift(ref tenderengine.EntryRef) (tenderengine.ResumeReport, error) {
	w.mu.Lock()
	liftErr := w.liftErr
	w.mu.Unlock()
	if liftErr != nil {
		return tenderengine.ResumeReport{}, liftErr
	}
	return tenderengine.Lift(tenderengine.Deps{RecordPath: w.recordPath, Now: w.now, Sleep: w.sleep}, ref)
}

func (w *fakeWorld) hibernate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hibernateCalls++
	if w.hibernateErr != nil {
		return w.hibernateErr
	}
	w.pendingSleep = true
	return nil
}

func (w *fakeWorld) notify(line string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.notices = append(w.notices, line)
	return nil
}

func (w *fakeWorld) awakeClock() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.awake
}

func (w *fakeWorld) deps() Deps {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Deps{
		RequestPath: w.requestPath,
		Caller:      w.caller,
		GOOS:        w.goos,
		LookPath: func(file string) (string, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			if !w.systemctl {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + file, nil
		},
		Spawn:        w.spawn,
		SelfPID:      w.selfPID,
		SelfStart:    w.selfStart,
		Alive:        w.isAlive,
		Kill:         w.kill,
		RunStatus:    w.runStatus,
		RunSlugs:     w.listRunSlugs,
		LandLockHeld: w.landLockHeld,
		Pause:        w.pause,
		TenderStatus: w.tenderStatus,
		Lift:         w.lift,
		Hibernate:    w.hibernate,
		Notify:       w.notify,
		Now:          w.now,
		Sleep:        w.sleep,
		Awake:        w.awakeClock,
	}
}

// ownRequest is a waiting request whose daemon is the world's own process.
func (w *fakeWorld) ownRequest(trigger Trigger, slug string) Request {
	return Request{Trigger: trigger, Slug: slug, Requester: requesterExternal, RequestedAt: w.now(), DaemonPID: w.selfPID, DaemonStart: w.selfStart, Phase: PhaseWaiting}
}

// seed writes r as the request file.
func (w *fakeWorld) seed(r Request) {
	w.t.Helper()
	err := updateRequest(w.requestPath, func(Request, bool) (Request, error) { return r, nil })
	if err != nil {
		w.t.Fatal(err)
	}
}

// request reads the request file, which the test expects to exist.
func (w *fakeWorld) request() Request {
	w.t.Helper()
	r, found, err := readRequest(w.requestPath)
	if err != nil || !found {
		w.t.Fatalf("read the request: found %v, err %v", found, err)
	}
	return r
}

// liveEntries lists the pause record's live entries.
func (w *fakeWorld) liveEntries() []pauserecord.Entry {
	w.t.Helper()
	record, err := pauserecord.Read(w.recordPath)
	if err != nil {
		w.t.Fatal(err)
	}
	return record.Live
}

// addHubEntry adds a live hub entry as another caller would have, paused at the world's wall clock.
func (w *fakeWorld) addHubEntry(caller string) pauserecord.Entry {
	w.t.Helper()
	now := w.now()
	entry := pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: now, GraceEnd: now, Caller: caller, Sessions: []pauserecord.Session{}}
	_, err := pauserecord.Update(w.recordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, entry)
		return nil
	})
	if err != nil {
		w.t.Fatal(err)
	}
	return entry
}
