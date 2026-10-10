// seatfake_test.go holds the handle-shaped fake shuttle the engine tests drive, and the helpers that run an engine beside it.

package seatengine

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeWait bounds every wait a test makes on the engine's goroutines; it is only reached when a test is about to fail.
const fakeWait = 10 * time.Second

// fakeHandle is one started seat: its wait blocks until the test releases it or the engine stops it.
type fakeHandle struct {
	role string
	name string
	guid string

	mu         sync.Mutex
	sendErrs   []error
	alwaysBusy bool
	attempts   int
	stops      int
	stopErr    error

	sent   chan string
	ended  chan struct{}
	once   sync.Once
	result shuttleengine.Result
	err    error
}

func (h *fakeHandle) StrandGUID() string { return h.guid }
func (h *fakeHandle) StrandName() string { return h.name }
func (h *fakeHandle) RunDir() string     { return "/run/" + h.guid }

// release ends the seat's wait with result and err; only the first release counts.
func (h *fakeHandle) release(result shuttleengine.Result, err error) {
	h.once.Do(func() {
		h.result, h.err = result, err
		close(h.ended)
	})
}

func (h *fakeHandle) Wait() (shuttleengine.Result, error) {
	<-h.ended
	return h.result, h.err
}

// Send answers the scripted errors in order, then lands every line; alwaysBusy answers a busy session forever.
func (h *fakeHandle) Send(text string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attempts++
	if h.alwaysBusy {
		return shuttleengine.ErrSessionBusy
	}
	if len(h.sendErrs) > 0 {
		err := h.sendErrs[0]
		h.sendErrs = h.sendErrs[1:]
		if err != nil {
			return err
		}
	}
	h.sent <- text
	return nil
}

// Stop records the stop and, unless it is scripted to fail, ends the seat's wait as died.
func (h *fakeHandle) Stop() error {
	h.mu.Lock()
	h.stops++
	err := h.stopErr
	h.mu.Unlock()
	if err != nil {
		return err
	}
	h.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, nil)
	return nil
}

func (h *fakeHandle) stopCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stops
}

func (h *fakeHandle) attemptCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.attempts
}

// fakeShuttle is a Shuttle whose seats are fakeHandles, keyed by the role of the spec that started them.
// The script fields are set before the engine runs.
type fakeShuttle struct {
	t *testing.T

	// startErrs fails the start of the seat with that role.
	startErrs map[string]error
	// strandNames overrides the strand name a role's seat is started under.
	strandNames map[string]string
	// sendErrs, alwaysBusy and stopErrs script the handle started for that role.
	sendErrs   map[string][]error
	alwaysBusy map[string]bool
	stopErrs   map[string]error
	// holdStart blocks the start of a role until its channel is closed.
	holdStart map[string]chan struct{}
	// live holds the handles ProbeGated finds, and removedOnProbe the handles it stops and answers not found for.
	live           map[string]*fakeHandle
	removedOnProbe map[string]*fakeHandle
	probeErr       error

	started chan string

	mu      sync.Mutex
	handles map[string]*fakeHandle
	specs   map[string]shuttleengine.Spec
	gates   map[string]shuttleengine.GateSpec
	probed  []string
}

func newFakeShuttle(t *testing.T) *fakeShuttle {
	t.Helper()
	return &fakeShuttle{
		t:       t,
		started: make(chan string, 16),
		handles: make(map[string]*fakeHandle),
		specs:   make(map[string]shuttleengine.Spec),
		gates:   make(map[string]shuttleengine.GateSpec),
	}
}

// newHandle returns a handle for role, named and scripted from the shuttle's fields.
func (f *fakeShuttle) newHandle(role string) *fakeHandle {
	name := "ly:task:" + role
	if override, ok := f.strandNames[role]; ok {
		name = override
	}
	handle := &fakeHandle{
		role:       role,
		name:       name,
		guid:       "guid-" + role,
		sendErrs:   append([]error(nil), f.sendErrs[role]...),
		alwaysBusy: f.alwaysBusy[role],
		stopErr:    f.stopErrs[role],
		sent:       make(chan string, 16),
		ended:      make(chan struct{}),
	}
	// A seat left running when its test ends must not leak its goroutine.
	f.t.Cleanup(func() { handle.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, nil) })
	return handle
}

func (f *fakeShuttle) StartGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (Handle, error) {
	if hold, ok := f.holdStart[spec.Role]; ok {
		<-hold
	}
	f.mu.Lock()
	f.specs[spec.Role] = spec
	f.gates[spec.Role] = gate
	f.mu.Unlock()
	if err := f.startErrs[spec.Role]; err != nil {
		f.started <- spec.Role
		return nil, err
	}
	handle := f.newHandle(spec.Role)
	f.mu.Lock()
	f.handles[spec.Role] = handle
	f.mu.Unlock()
	f.started <- spec.Role
	return handle, nil
}

func (f *fakeShuttle) ProbeGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (Handle, bool, error) {
	f.mu.Lock()
	f.probed = append(f.probed, spec.Role)
	f.gates[spec.Role] = gate
	f.mu.Unlock()
	if f.probeErr != nil {
		return nil, false, f.probeErr
	}
	if removed, ok := f.removedOnProbe[spec.Role]; ok {
		_ = removed.Stop()
		return nil, false, nil
	}
	if handle, ok := f.live[spec.Role]; ok {
		return handle, true, nil
	}
	return nil, false, nil
}

// handle returns the handle started for role.
func (f *fakeShuttle) handle(role string) *fakeHandle {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	handle, ok := f.handles[role]
	if !ok {
		f.t.Fatalf("no seat was started with the role %q", role)
	}
	return handle
}

// spec returns the spec the seat with role was started with.
func (f *fakeShuttle) spec(role string) shuttleengine.Spec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.specs[role]
}

// awaitStarts returns the roles of the next n start calls, in call order.
func (f *fakeShuttle) awaitStarts(n int) []string {
	f.t.Helper()
	roles := make([]string, 0, n)
	for len(roles) < n {
		select {
		case role := <-f.started:
			roles = append(roles, role)
		case <-time.After(fakeWait):
			f.t.Fatalf("only %d of %d seats started: %v", len(roles), n, roles)
		}
	}
	return roles
}

// receive returns the next line typed into the handle's session.
func receive(t *testing.T, handle *fakeHandle) string {
	t.Helper()
	select {
	case line := <-handle.sent:
		return line
	case <-time.After(fakeWait):
		t.Fatalf("no line was typed into %s", handle.name)
		return ""
	}
}

// engineEnd is what a run of the engine returned.
type engineEnd struct {
	result Result
	err    error
}

// runAsync runs table on e in the background and returns the channel its end arrives on.
func runAsync(e *Engine, table Table) <-chan engineEnd {
	ended := make(chan engineEnd, 1)
	go func() {
		result, err := e.Run(table)
		ended <- engineEnd{result, err}
	}()
	return ended
}

// await returns the end of a background run.
func await(t *testing.T, ended <-chan engineEnd) engineEnd {
	t.Helper()
	select {
	case end := <-ended:
		return end
	case <-time.After(fakeWait):
		t.Fatal("the run did not end")
		return engineEnd{}
	}
}

// engineFixture returns an engine over a fresh fake shuttle, whose stencils directory and worktree root are set up for table.
func engineFixture(t *testing.T, shuttle *fakeShuttle) (*Engine, Geometry) {
	t.Helper()
	geom := promptGeometry(t)
	geom.WorktreeRoot = t.TempDir()
	engine := New(shuttle, geom)
	engine.noticeRetry = time.Millisecond
	return engine, geom
}

// engineTable returns a table of one chair and the given number of advisors, whose files live under dir.
func engineTable(dir string, advisors int) Table {
	table := promptTable()
	table.Gate = shuttleengine.GateSpec{{Name: "chair-gate", Attempts: 1}}
	table.Seats = []Seat{{Name: RoleChair, Stencil: "seat-test-chair", Outputs: []string{filepath.Join(dir, "out.md")}}}
	for n := 1; n <= advisors; n++ {
		output := filepath.Join(dir, AdvisorName(n)+".md")
		table.Seats[0].Inputs = append(table.Seats[0].Inputs, output)
		table.Seats = append(table.Seats, Seat{Name: AdvisorName(n), Stencil: "seat-test-advisor", Outputs: []string{output}})
	}
	return table
}

// role returns the strand role the seat called seat takes in engineTable's tables.
func role(seat string) string { return SeatRole(promptTable().RolePrefix, seat) }
