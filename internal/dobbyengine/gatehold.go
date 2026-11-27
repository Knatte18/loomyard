// gatehold.go implements the gate-hold notice.
// On a probe tick it reads the held gate slots and posts one line for each hold of a driven run that is past its bound.
// The line names the holder's process tree and the marked processes outside it.

package dobbyengine

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
)

// gateHoldProcessMax is the most descendants, and the most marked processes outside the tree, one line names.
const gateHoldProcessMax = 20

// GateHolder is one held gate slot as dobby reads it.
type GateHolder struct {
	Slot int
	// Worktree is the directory the holding run works in.
	Worktree string
	// Site is the label of what holds the slot.
	Site string
	// Tier is the tier key of a tier run, empty for any other holder.
	Tier string
	// Marker is the `NAME=value` entry the holder's processes carry in their environment, empty when it records none.
	Marker string
	// PID is the holding process.
	PID int
	// Started is when the slot was acquired.
	Started time.Time
	// TierTimeout is the tier's gate.tiers.<tier>.timeout_min, zero for a holder with no tier or a tier with none.
	TierTimeout time.Duration
}

// GateHoldDeps is the seams of the gate-hold notice, told by the hub wiring.
// A nil Holders or a nil Post checks nothing.
type GateHoldDeps struct {
	// Holders reads the live holders with their tier timeouts.
	Holders func() ([]GateHolder, error)
	// TaskWorktree returns the task worktree path of the run slug.
	TaskWorktree func(slug string) (string, error)
	// Tree samples the process pid and every descendant of it.
	Tree func(pid int) ([]proc.ProcessSample, error)
	// MarkedPIDs returns the processes whose environment holds entry.
	MarkedPIDs func(entry string) ([]int, error)
	// Post posts one line about the run slug to the nearest live orch.
	Post func(ctx context.Context, slug, line string) error
}

// gateHolds notices each hold of a listed run that is past its bound, once per hold, on a probe tick.
// The bound runs on awake time, and the keys of the holds noticed are kept in the run's watch memory.
// A failed read or post is logged once per episode and tried again on the next probe tick.
func (d *daemon) gateHolds(ctx context.Context, now time.Time) {
	seams := d.deps.GateHolds
	if seams.Holders == nil || seams.Post == nil || !d.probeDue {
		return
	}
	cfg, err := d.deps.LoadConfig()
	if err != nil {
		d.gateHoldReads.Fail("config", "dobbyengine: the config does not load; no gate-hold check", "error", err)
		return
	}
	d.gateHoldReads.Recover("config", "dobbyengine: the config loads again for the gate-hold check")
	if cfg.GateHoldFactor == 0 {
		return
	}
	holders, err := seams.Holders()
	if err != nil {
		d.gateHoldReads.Fail("holders", "dobbyengine: the gate slot holders are unreadable; no gate-hold check", "error", err)
		return
	}
	d.gateHoldReads.Recover("holders", "dobbyengine: the gate slot holders read again")
	record, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		d.gateHoldReads.Fail("state", "dobbyengine: the state record is unreadable; no gate-hold check", "error", err)
		return
	}
	d.gateHoldReads.Recover("state", "dobbyengine: the state record reads again for the gate-hold check")

	// One process may hold several slots, which are one hold.
	holds := map[string][]GateHolder{}
	byPID := map[int][]GateHolder{}
	for _, holder := range holders {
		byPID[holder.PID] = append(byPID[holder.PID], holder)
	}
	for _, hold := range byPID {
		holds[gateHoldKey(hold)] = hold
	}

	for _, run := range d.listedRuns {
		worktree, err := seams.TaskWorktree(run.Slug)
		if err != nil {
			d.gateHoldReads.Fail(run.Slug+"|worktree", "dobbyengine: the run's task worktree does not resolve; no gate-hold check for it", "slug", run.Slug, "error", err)
			continue
		}
		d.gateHoldReads.Recover(run.Slug+"|worktree", "dobbyengine: the run's task worktree resolves again", "slug", run.Slug)

		noticed := record.Watches[run.Slug].GateHolds
		kept := slices.DeleteFunc(slices.Clone(noticed), func(key string) bool { return holds[key] == nil })
		for key, hold := range holds {
			if !underWorktree(hold[0].Worktree, worktree) || slices.Contains(kept, key) {
				continue
			}
			bound := gateHoldBound(cfg, hold)
			if pauserecord.AwakeSince(gateHoldStart(hold), now, record.Sleeps) < bound {
				continue
			}
			tree, marked := d.gateHoldProcesses(run.Slug, hold)
			if err := seams.Post(ctx, run.Slug, gateHoldLine(run.Slug, hold, bound, tree, marked)); err != nil {
				d.gateHoldPosts.Fail(run.Slug, "dobbyengine: a gate-hold notice did not go out", "slug", run.Slug, "error", err)
				continue
			}
			d.gateHoldPosts.Recover(run.Slug, "dobbyengine: the gate-hold notices go out again", "slug", run.Slug)
			kept = append(kept, key)
		}
		if slices.Equal(kept, noticed) {
			continue
		}
		err = UpdateState(d.geom.StatePath, func(s *State) error {
			if s.Watches == nil {
				s.Watches = map[string]RunWatchState{}
			}
			watch := s.Watches[run.Slug]
			watch.GateHolds = kept
			s.Watches[run.Slug] = watch
			return nil
		})
		if err != nil {
			d.gateHoldPosts.Fail(run.Slug+"|record", "dobbyengine: the noticed gate holds could not be recorded", "slug", run.Slug, "error", err)
			continue
		}
		d.gateHoldPosts.Recover(run.Slug+"|record", "dobbyengine: the noticed gate holds are recorded again", "slug", run.Slug)
	}
}

// gateHoldProcesses samples the holder's tree and the marked processes outside it.
// It returns nothing at all when either cannot be read, so the line names the tree as unreadable.
func (d *daemon) gateHoldProcesses(slug string, hold []GateHolder) (tree, marked []proc.ProcessSample) {
	seams := d.deps.GateHolds
	unreadable := func(err error) {
		d.gateHoldReads.Fail(slug+"|processes", "dobbyengine: the gate holder's processes are unreadable", "slug", slug, "pid", hold[0].PID, "error", err)
	}
	tree, err := seams.Tree(hold[0].PID)
	if err != nil {
		unreadable(err)
		return nil, nil
	}
	if marker := gateHoldMarker(hold); marker != "" {
		pids, err := seams.MarkedPIDs(marker)
		if err != nil {
			unreadable(err)
			return nil, nil
		}
		for _, pid := range pids {
			if slices.ContainsFunc(tree, func(sample proc.ProcessSample) bool { return sample.PID == pid }) {
				continue
			}
			own, err := seams.Tree(pid)
			if err != nil {
				unreadable(err)
				return nil, nil
			}
			// A process that ended since the marker scan has no entry.
			if i := slices.IndexFunc(own, func(sample proc.ProcessSample) bool { return sample.PID == pid }); i >= 0 {
				marked = append(marked, own[i])
			}
		}
	}
	d.gateHoldReads.Recover(slug+"|processes", "dobbyengine: the gate holder's processes read again", "slug", slug)
	return tree, marked
}

// gateHoldStart returns the earliest acquire time among the slots of hold.
func gateHoldStart(hold []GateHolder) time.Time {
	start := hold[0].Started
	for _, holder := range hold[1:] {
		if holder.Started.Before(start) {
			start = holder.Started
		}
	}
	return start
}

// gateHoldKey tells one hold from another: another process, or a later start of the same one, is a new hold.
func gateHoldKey(hold []GateHolder) string {
	return strconv.Itoa(hold[0].PID) + "|" + strconv.FormatInt(gateHoldStart(hold).UnixNano(), 10)
}

// gateHoldBound returns how long hold may stand: the factor times its largest tier timeout,
// or the configured minutes when no slot of it has one.
func gateHoldBound(cfg Config, hold []GateHolder) time.Duration {
	var timeout time.Duration
	for _, holder := range hold {
		timeout = max(timeout, holder.TierTimeout)
	}
	if timeout <= 0 {
		return time.Duration(cfg.GateHoldMin) * time.Minute
	}
	bound := cfg.GateHoldFactor * float64(timeout)
	// A product past the type's range converts to a negative bound, which every hold is past.
	if bound >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return time.Duration(bound)
}

// gateHoldMarker returns the first environment marker a slot of hold records, empty when none does.
func gateHoldMarker(hold []GateHolder) string {
	for _, holder := range hold {
		if holder.Marker != "" {
			return holder.Marker
		}
	}
	return ""
}

// gateHoldLine renders the gate-hold notice of the run slug.
// The tree argument is the holder's process tree, empty when it could not be read.
// The marked argument is the processes outside that tree that carry the holder's marker.
// Each process list is ordered by CPU time, most first, and cut at gateHoldProcessMax.
func gateHoldLine(slug string, hold []GateHolder, bound time.Duration, tree []proc.ProcessSample, marked []proc.ProcessSample) string {
	slots := make([]int, len(hold))
	for i, holder := range hold {
		slots[i] = holder.Slot
	}
	slices.Sort(slots)
	slotNames := make([]string, len(slots))
	for i, slot := range slots {
		slotNames[i] = strconv.Itoa(slot)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: a gate slot is held past its bound: slots %s; tier %s; site %s; started %s; bound %s; holder pid %d",
		runNoticePrefix, slug, strings.Join(slotNames, ","), cmp.Or(hold[0].Tier, "none"), hold[0].Site,
		gateHoldStart(hold).UTC().Format(time.RFC3339), bound, hold[0].PID)
	if len(tree) == 0 {
		b.WriteString("; process tree unreadable")
		return b.String()
	}

	var descendants []proc.ProcessSample
	for _, sample := range tree {
		if sample.PID == hold[0].PID {
			fmt.Fprintf(&b, " %s cpu %s", sample.Path, sample.CPU)
			continue
		}
		descendants = append(descendants, sample)
	}
	writeProcesses := func(lead string, samples []proc.ProcessSample) {
		ordered := slices.Clone(samples)
		slices.SortStableFunc(ordered, func(a, b proc.ProcessSample) int { return cmp.Compare(b.CPU, a.CPU) })
		for _, sample := range ordered[:min(len(ordered), gateHoldProcessMax)] {
			fmt.Fprintf(&b, "; %s %d %s cpu %s", lead, sample.PID, sample.Path, sample.CPU)
		}
		if rest := len(ordered) - gateHoldProcessMax; rest > 0 {
			fmt.Fprintf(&b, "; and %d more", rest)
		}
	}
	writeProcesses("child pid", descendants)
	writeProcesses("outside the tree, carrying "+gateHoldMarker(hold)+": pid", marked)
	return b.String()
}

// underWorktree reports whether path is worktree or lies inside it, compared on whole path segments.
func underWorktree(path, worktree string) bool {
	path, worktree = filepath.Clean(path), filepath.Clean(worktree)
	return path == worktree || strings.HasPrefix(path, worktree+string(filepath.Separator))
}
