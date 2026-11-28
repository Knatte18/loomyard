// flow.go runs one window's placement, from recording the window to the command that follows it.

package kwinplace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// placementLockFileName is the lock that makes the placements of every hub on a machine take their turn.
const placementLockFileName = "kwinplace.lock"

// The production bounds of a placement.
const (
	defaultWindowWait = 10 * time.Second
	defaultParentWait = 180 * time.Second
	defaultLockWait   = 120 * time.Second
)

// windowQueryPause is the least time between one query for a new window ending and the next starting.
const windowQueryPause = time.Second

// Request is one window to place, with the facts of its hub that the placement reads.
type Request struct {
	// BoltDir is the hub's Bolt checkout, under which the remembered places are kept.
	BoltDir string
	// Worktree, Title and PID name the window.
	Worktree string
	Title    string
	PID      int
	// Prime reports that the window is the prime worktree's own.
	Prime bool
	// Parent is the name of the worktree whose window this one is placed from, "" only for the prime's own window.
	Parent string
	// Mode is the window placement mode in force: off, same or next.
	Mode string
	// MinTileWidth and MinTileHeight are the smallest zone a grid may have, both positive.
	MinTileWidth  int
	MinTileHeight int
	// Worktrees lists every worktree the hub has, as it stands when called.
	Worktrees func() ([]string, error)
}

// Mode returns the window placement mode in force: the task's own value unless it is empty, else the machine's.
func Mode(perTask, machine string) string {
	if perTask != beamsetting.WindowPlacementInherit {
		return perTask
	}
	return machine
}

// Flow places windows.
// Query, Apply and ReadTilingOutput are its calls into the screen session.
type Flow struct {
	Query            func(ctx context.Context, tilingOutput string, after time.Time, within time.Duration) (Snapshot, time.Time, error)
	Apply            func(ctx context.Context, order ApplyOrder) (ApplyReport, error)
	ReadTilingOutput func(ctx context.Context) (string, error)

	afterPlace  func(ctx context.Context) error
	now         func() time.Time
	sleep       func(ctx context.Context, d time.Duration)
	lockPath    string
	acquireLock func(path string) (*lock.FileLock, error)

	windowWait time.Duration
	parentWait time.Duration
	lockWait   time.Duration
}

// NewFlow returns the flow that places windows through runner and calls afterPlace once a window's placement has ended.
func NewFlow(runner *Runner, afterPlace func(ctx context.Context) error) Flow {
	return Flow{
		Query:            runner.Query,
		Apply:            runner.Apply,
		ReadTilingOutput: runner.ReadTilingOutput,
		afterPlace:       afterPlace,
		now:              time.Now,
		sleep:            sleepWithin,
		lockPath:         filepath.Join(runner.stateDir, placementLockFileName),
		acquireLock:      lock.AcquireWriteLock,
		windowWait:       defaultWindowWait,
		parentWait:       defaultParentWait,
		lockWait:         defaultLockWait,
	}
}

// sleepWithin sleeps for d, or until ctx ends.
func sleepWithin(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// flowRun is what one placement knows about its window.
type flowRun struct {
	req          Request
	placesPath   string
	tilingOutput string
	// seenOn is the one desktop the latest snapshot showed the window on, "" for none.
	seenOn string
	// recorded reports that the window's place has been written, which also settles its entry.
	recorded bool
	// lockBudget is what is left of the time the placement may spend waiting for the lock.
	lockBudget time.Duration
}

// Place records the window of req, places it, remembers where it landed and then calls the flow's afterPlace.
// Every way the placement can end leaves the window's entry settled.
// The one exception is an entry a newer window of the worktree has taken.
// Whatever kept the window from being placed is returned for the caller to log, joined with any error of afterPlace;
// the window itself stays open.
// A ctx that ends before afterPlace is started skips it, and one that ends during it reaches it through ctx.
func (f Flow) Place(ctx context.Context, req Request) error {
	run := &flowRun{req: req, placesPath: PlacesPath(req.BoltDir), lockBudget: f.lockWait}
	if err := RecordOpen(run.placesPath, req.Worktree, req.PID, req.Title, f.now()); err != nil {
		logger.Warn("kwinplace: window not recorded", "worktree", req.Worktree, "error", err)
	}
	warning := f.placeWindow(ctx, run)
	if !run.recorded {
		if err := Settle(run.placesPath, req.Worktree, req.PID, run.seenOn, f.now()); err != nil {
			logger.Warn("kwinplace: window's entry not settled", "worktree", req.Worktree, "error", err)
		}
	}
	if ctx.Err() != nil {
		return cmp.Or(warning, ctx.Err())
	}
	return errors.Join(warning, f.afterPlace(ctx))
}

// placeWindow waits for the window and for its parent's place, then places it under the placement lock.
func (f Flow) placeWindow(ctx context.Context, run *flowRun) error {
	tilingOutput, err := f.ReadTilingOutput(ctx)
	if err != nil {
		return fmt.Errorf("kwinplace: read the tiling screen: %w", err)
	}
	run.tilingOutput = tilingOutput
	snap, err := f.awaitWindow(ctx, run)
	if err != nil {
		return err
	}

	needsAnchor := run.needsAnchor(snap, ReadPlaces(run.placesPath))
	// One bound covers every wait for the parent, counted from the first.
	var parentDeadline time.Time
	for {
		if needsAnchor {
			if parentDeadline.IsZero() {
				parentDeadline = f.now().Add(f.parentWait)
			}
			f.awaitParent(ctx, run.placesPath, run.req.Parent, parentDeadline)
		}
		held, err := f.takeLock(ctx, run)
		if err != nil {
			return err
		}
		waitAgain, err := f.placeLocked(ctx, run, &needsAnchor, parentDeadline)
		if releaseErr := held.Release(); releaseErr != nil {
			logger.Warn("kwinplace: placement lock not released", "path", f.lockPath, "error", releaseErr)
		}
		if !waitAgain {
			return err
		}
	}
}

// awaitWindow queries until a snapshot shows the window, and returns that snapshot.
func (f Flow) awaitWindow(ctx context.Context, run *flowRun) (Snapshot, error) {
	// A shared report counts only if it ended after the previous query of this wait, or after the wait began.
	after := f.now()
	deadline := after.Add(f.windowWait)
	for first := true; ; first = false {
		remaining := deadline.Sub(f.now())
		if remaining <= 0 {
			return Snapshot{}, fmt.Errorf("kwinplace: window %q did not appear in KWin within %s", run.req.Title, f.windowWait)
		}
		snap, _, err := f.Query(ctx, run.tilingOutput, after, remaining)
		if err != nil {
			return Snapshot{}, err
		}
		run.see(snap)
		if first && !snap.PerDesktopTiling {
			return Snapshot{}, errors.New("kwinplace: window placement is unsupported: KWin does not tile each desktop on its own")
		}
		if _, found := windowIn(snap, run.req.PID, run.req.Title); found {
			return snap, nil
		}
		after = f.now()
		f.sleep(ctx, windowQueryPause)
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
	}
}

// takeLock takes the placement lock within what is left of the run's budget, and charges the wait to it.
func (f Flow) takeLock(ctx context.Context, run *flowRun) (*lock.FileLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	busy := fmt.Errorf("kwinplace: placement lock %s not free within %s of waiting", f.lockPath, f.lockWait)
	if run.lockBudget <= 0 {
		return nil, busy
	}
	started := f.now()
	held, taken, err := acquireWithin(ctx, f.lockPath, run.lockBudget, f.acquireLock)
	run.lockBudget -= f.now().Sub(started)
	if err != nil {
		return nil, fmt.Errorf("kwinplace: placement lock: %w", err)
	}
	if !taken {
		return nil, busy
	}
	return held, nil
}

// placeLocked plans, applies and records the placement from a fresh snapshot, for a caller holding the placement lock.
// It reports true, having done nothing, when the parent's place is not known and there is still time to wait for it.
// needsAnchor is set once the window turns out to need its parent's place.
func (f Flow) placeLocked(ctx context.Context, run *flowRun, needsAnchor *bool, parentDeadline time.Time) (bool, error) {
	snap, _, err := f.Query(ctx, run.tilingOutput, f.now(), defaultScriptLockWait)
	if err != nil {
		return false, err
	}
	run.see(snap)
	places := ReadPlaces(run.placesPath)
	req := run.req
	if req.Mode == beamsetting.WindowPlacementOff {
		return false, run.record(reportUpdateFrom(snap, run.tilingOutput, ApplyReport{}, places, req, Placement{}, false, f.now()))
	}

	*needsAnchor = *needsAnchor || run.needsAnchor(snap, places)
	anchor, fallback := snap.CurrentDesktop, false
	if *needsAnchor {
		parentDesktop, known := Anchor(snap, places, req.Parent)
		switch {
		case known && snap.hasDesktop(parentDesktop):
			anchor = parentDesktop
		case known:
			fallback = true
		case parentDeadline.IsZero() || f.now().Before(parentDeadline):
			return true, nil
		default:
			fallback = true
		}
	}

	plan := Plan(PlanInput{
		Snapshot:       snap,
		Places:         places,
		Mode:           req.Mode,
		MinTileWidth:   req.MinTileWidth,
		MinTileHeight:  req.MinTileHeight,
		TilingOutput:   run.tilingOutput,
		Worktree:       req.Worktree,
		PID:            req.PID,
		Title:          req.Title,
		Prime:          req.Prime,
		Anchor:         anchor,
		AnchorFallback: fallback,
	})
	if plan.Target == "" {
		return false, fmt.Errorf("kwinplace: window left where it is: %s", plan.Warning)
	}
	report, err := f.Apply(ctx, ApplyOrder{
		TilingOutput: run.tilingOutput,
		Desktop:      plan.Target,
		Rows:         plan.Grid.Rows,
		Cols:         plan.Grid.Cols,
		Zones:        plan.Zones,
		Placed:       plan.Placed,
	})
	if err != nil {
		return false, err
	}
	return false, run.record(reportUpdateFrom(snap, run.tilingOutput, report, places, req, plan, fallback, f.now()))
}

func (r *flowRun) see(snap Snapshot) {
	window, _ := windowIn(snap, r.req.PID, r.req.Title)
	r.seenOn = desktopOf(window)
}

// needsAnchor reports whether the window's desktop is counted from its parent's:
// it is placed, it is not the prime's own, and snap holds no home desktop of it.
func (r *flowRun) needsAnchor(snap Snapshot, places Places) bool {
	if r.req.Prime || r.req.Mode == beamsetting.WindowPlacementOff {
		return false
	}
	return !snap.hasDesktop(places[r.req.Worktree].Home)
}

func (r *flowRun) record(update ReportUpdate) error {
	if err := RecordReport(r.placesPath, update); err != nil {
		return err
	}
	r.recorded = true
	return nil
}

func windowIn(snap Snapshot, pid int, title string) (Window, bool) {
	i := slices.IndexFunc(snap.Windows, func(w Window) bool { return w.Matches(pid, title) })
	if i < 0 {
		return Window{}, false
	}
	return snap.Windows[i], true
}

// desktopOf returns the one desktop the window sits on, "" when it sits on several or on all.
func desktopOf(window Window) string {
	if len(window.Desktops) != 1 {
		return ""
	}
	return window.Desktops[0]
}

// reportUpdateFrom returns what a placement learned about the hub's windows.
// snap and places are what was read under the placement lock.
// A zero plan and report mean nothing was moved, and the placed window is then recorded where snap shows it.
func reportUpdateFrom(snap Snapshot, tilingOutput string, report ApplyReport, places Places, req Request, plan Placement, fallback bool, now time.Time) ReportUpdate {
	windows := slices.Clone(snap.Windows)
	for i, window := range windows {
		for _, applied := range report.Windows {
			if applied.InternalID == window.InternalID {
				windows[i].Desktops, windows[i].Frame = applied.Desktops, applied.Frame
			}
		}
	}

	onDesktop := map[string][]Window{}
	for _, window := range windows {
		if desktop := desktopOf(window); desktop != "" {
			onDesktop[desktop] = append(onDesktop[desktop], window)
		}
	}
	order := map[string]int{}
	for desktop, sitting := range onDesktop {
		tiles := snap.Tiles[desktop]
		if desktop == plan.Target {
			tiles = report.Tiles
		}
		for i, id := range CurrentOrder(tiles, sitting) {
			order[id] = i
		}
	}

	home := places[req.Worktree].Home
	update := ReportUpdate{
		Worktree:   req.Worktree,
		PID:        req.PID,
		Overflowed: plan.Overflowed,
		Fallback:   fallback,
		HomeGone:   home != "" && !snap.hasDesktop(home),
		Now:        now,
		Worktrees:  req.Worktrees,
	}
	if plan.Target != "" {
		update.Landed, update.Output = plan.Target, tilingOutput
	}
	remembered := slices.Sorted(maps.Keys(places))
	for _, window := range windows {
		worktree := req.Worktree
		if window.Matches(req.PID, req.Title) {
			if plan.Target == "" {
				update.Landed, update.Output = desktopOf(window), window.Output
			}
		} else {
			i := slices.IndexFunc(remembered, func(name string) bool {
				return name != req.Worktree && window.Matches(places[name].PID, places[name].Title)
			})
			if i < 0 {
				continue
			}
			worktree = remembered[i]
		}
		update.Windows = append(update.Windows, SeenWindow{Worktree: worktree, Desktop: desktopOf(window), Order: order[window.InternalID], Minimized: window.Minimized})
	}
	return update
}
