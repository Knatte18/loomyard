// end.go implements the one end sequence of a run's processes: End for an owner's registered processes and EndProcess for one unregistered pid.

package runproc

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

const (
	// endGrace is the time in all that processes get to exit after SIGTERM before SIGKILL.
	endGrace = 5 * time.Second
	// killBound is the time processes get to disappear after SIGKILL before they count as survivors.
	killBound = 2 * time.Second
	// endPasses is how many times End re-lists an owner, so a process registered during the end is reached.
	endPasses = 3
	// pollInterval is the pause between the readings that wait for processes to disappear.
	pollInterval = 100 * time.Millisecond
)

// Ended is one process an end sequence signalled and found gone, or, inside a SurvivorsError, one process still alive.
type Ended struct {
	// PID is the process id.
	PID int
	// Role is the record's role, with a "descendant of " prefix for a process found below a record.
	Role string
}

// SurvivorsError reports processes that were still alive after SIGKILL and its bound.
type SurvivorsError struct {
	// Survivors lists each process still alive.
	Survivors []Ended
}

// Error names each survivor's pid and role.
func (e *SurvivorsError) Error() string {
	names := make([]string, len(e.Survivors))
	for i, survivor := range e.Survivors {
		names[i] = fmt.Sprintf("%d (%s)", survivor.PID, survivor.Role)
	}
	return "runproc: processes survived the end: " + strings.Join(names, ", ")
}

// End ends every live process registered under owner in dir, and the processes below them, except callerPID and its ancestors.
// Each is sent SIGTERM and, once the grace is spent, SIGKILL; a signal reaches a pid only while its start time matches the last reading, and a process group only while a live process still reports it.
// It returns each process it ended and, when some survive SIGKILL, a *SurvivorsError.
// On Windows, where no process or group can be told apart or signalled, it signals nothing.
func End(dir, owner string, callerPID int) ([]Ended, error) {
	return productionEnder().end(dir, owner, callerPID)
}

// EndSparing ends like End, leaving alone every live process registered under owner with a role in spare, and the processes below it.
// It serves an end of a session that is not a teardown of the owner's run.
func EndSparing(dir, owner string, callerPID int, spare ...string) ([]Ended, error) {
	return productionEnder().end(dir, owner, callerPID, spare...)
}

// EndProcess ends one process that is registered nowhere, by the same SIGTERM, grace, SIGKILL and bound as End, once its start time still matches startTime.
// It returns a *SurvivorsError when the process is alive after the bound.
func EndProcess(pid int, startTime, role string) ([]Ended, error) {
	return productionEnder().endProcess(pid, startTime, role)
}

// ender carries the seams of the end sequence, so a test drives it over a scripted process table and a fake clock.
type ender struct {
	stats         func() ([]proc.StatEntry, error)
	startTime     func(pid int) (string, bool)
	isAlive       func(pid int) bool
	isAncestor    func(pid int) bool
	signalProcess func(pid int, kill bool) error
	signalGroup   func(pgid int, kill bool) error
	sleep         func(time.Duration)
	now           func() time.Time
	goos          string
}

func productionEnder() ender {
	return ender{
		stats:         proc.ListStats,
		startTime:     proc.StartTime,
		isAlive:       proc.IsAlive,
		isAncestor:    proc.IsAncestor,
		signalProcess: proc.SignalProcess,
		signalGroup:   proc.SignalGroup,
		sleep:         time.Sleep,
		now:           time.Now,
		goos:          runtime.GOOS,
	}
}

// watched is a process an end sequence waits for, identified by pid and the start time read with it.
type watched struct {
	pid       int
	startTime string
	role      string
}

// unit is one signal an end sequence sends: to a pid, or to a group when group is set.
// members are the processes the signal is meant to end.
type unit struct {
	pid       int
	startTime string
	group     int
	members   []watched
}

// end is the sequence behind End and EndSparing: a live record whose role is in spare is treated like the caller, left out of every signalled set with the processes below it.
func (e ender) end(dir, owner string, callerPID int, spare ...string) ([]Ended, error) {
	if e.goos == "windows" {
		return nil, nil
	}
	list := lister{startTime: e.startTime, isAlive: e.isAlive}
	attempted := map[string]bool{}
	var ended, survivors []Ended
	for pass := 0; pass < endPasses; pass++ {
		records, err := list.list(dir, owner)
		if err != nil {
			return ended, err
		}
		var todo []Record
		spared := map[int]bool{}
		for _, record := range records {
			if slices.Contains(spare, record.Role) {
				spared[record.PID] = true
			}
		}
		for _, record := range records {
			key := strconv.Itoa(record.PID) + "-" + record.StartTime
			if record.PID == callerPID || e.isAncestor(record.PID) || spared[record.PID] || attempted[key] {
				continue
			}
			attempted[key] = true
			todo = append(todo, record)
		}
		if len(todo) == 0 {
			break
		}
		stats, err := e.stats()
		if errors.Is(err, errors.ErrUnsupported) {
			return ended, nil
		}
		if err != nil {
			return ended, fmt.Errorf("runproc: read processes: %w", err)
		}
		skip := func(pid int) bool { return pid == callerPID || e.isAncestor(pid) || spared[pid] }
		passEnded, passSurvivors, err := e.runUnits(planRecords(todo, stats, skip), stats)
		ended = append(ended, passEnded...)
		for _, survivor := range passSurvivors {
			if !slices.Contains(survivors, survivor) {
				survivors = append(survivors, survivor)
			}
		}
		if err != nil {
			return ended, err
		}
	}
	logger.Info("runproc: ended the processes of an owner", "owner", owner, "ended", len(ended), "survivors", len(survivors))
	if len(survivors) > 0 {
		return ended, &SurvivorsError{Survivors: survivors}
	}
	return ended, nil
}

func (e ender) endProcess(pid int, startTime, role string) ([]Ended, error) {
	if e.goos == "windows" {
		return nil, nil
	}
	stats, err := e.stats()
	if errors.Is(err, errors.ErrUnsupported) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("runproc: read processes: %w", err)
	}
	target := watched{pid: pid, startTime: startTime, role: role}
	if !hasRunning(target, stats) {
		return nil, nil
	}
	ended, survivors, err := e.runUnits([]unit{{pid: pid, startTime: startTime, members: []watched{target}}}, stats)
	if err != nil {
		return ended, err
	}
	if len(survivors) > 0 {
		return ended, &SurvivorsError{Survivors: survivors}
	}
	return ended, nil
}

// runUnits sends SIGTERM to every unit that still holds against first and waits endGrace for the signalled members to go.
// It then sends SIGKILL to the units of those left, against a fresh reading, and waits killBound.
func (e ender) runUnits(units []unit, first []proc.StatEntry) (ended, survivors []Ended, err error) {
	var signalled []watched
	for _, u := range units {
		if e.signal(u, false, first) {
			signalled = append(signalled, u.members...)
		}
	}
	if len(signalled) == 0 {
		return nil, nil, nil
	}
	remaining, latest, err := e.await(signalled, endGrace)
	if err != nil {
		return nil, nil, err
	}
	if len(remaining) > 0 {
		for _, u := range units {
			if overlaps(u.members, remaining) {
				e.signal(u, true, latest)
			}
		}
		if remaining, _, err = e.await(remaining, killBound); err != nil {
			return nil, nil, err
		}
	}
	for _, member := range signalled {
		target := Ended{PID: member.pid, Role: member.role}
		if contains(remaining, member) {
			survivors = append(survivors, target)
		} else {
			ended = append(ended, target)
		}
	}
	return ended, survivors, nil
}

// signal sends SIGTERM, or SIGKILL when kill is set, to the unit when it still holds against stats, and reports whether a signal was attempted.
// A failed call still counts as attempted, so the unit's members are awaited and a member still running is reported.
func (e ender) signal(u unit, kill bool, stats []proc.StatEntry) bool {
	signalName := "SIGTERM"
	if kill {
		signalName = "SIGKILL"
	}
	var err error
	switch {
	case u.group != 0 && groupHolds(u, stats):
		err = e.signalGroup(u.group, kill)
		logger.Info("runproc: signalled a process group", "signal", signalName, "pgid", u.group, "err", err)
	case u.group == 0 && hasRunning(watched{pid: u.pid, startTime: u.startTime}, stats):
		err = e.signalProcess(u.pid, kill)
		logger.Info("runproc: signalled a process", "signal", signalName, "pid", u.pid, "err", err)
	default:
		return false
	}
	return true
}

// await polls stats until none of members is running or bound is spent, and returns the members still running with the last reading.
// A member is gone when its pid is absent, a zombie, or carries another start time.
func (e ender) await(members []watched, bound time.Duration) ([]watched, []proc.StatEntry, error) {
	deadline := e.now().Add(bound)
	for {
		stats, err := e.stats()
		if err != nil {
			return nil, nil, fmt.Errorf("runproc: read processes: %w", err)
		}
		var remaining []watched
		for _, member := range members {
			if hasRunning(member, stats) {
				remaining = append(remaining, member)
			}
		}
		if len(remaining) == 0 || !e.now().Before(deadline) {
			return remaining, stats, nil
		}
		e.sleep(pollInterval)
	}
}

// planRecords builds the signals that end records and everything below them in one reading.
// A leader's group is signalled as a whole, a process outside a group already signalled is signalled by pid, and one that leads its own group through that group.
// A group unit's members are its leader and every other live process of the group, so each is awaited and escalated; skip names the processes never watched.
func planRecords(records []Record, stats []proc.StatEntry, skip func(pid int) bool) []unit {
	byPID := map[int]proc.StatEntry{}
	children := map[int][]proc.StatEntry{}
	for _, entry := range stats {
		byPID[entry.PID] = entry
		children[entry.PPID] = append(children[entry.PPID], entry)
	}

	var units []unit
	groups := map[int]bool{}
	claimed := map[int]bool{}
	running := map[int]bool{}
	recorded := map[int]bool{}
	for _, record := range records {
		recorded[record.PID] = true
	}
	for _, record := range records {
		member := watched{pid: record.PID, startTime: record.StartTime, role: record.Role}
		running[record.PID] = hasRunning(member, stats)
		var members []watched
		if running[record.PID] {
			claimed[record.PID] = true
			members = []watched{member}
		}
		switch {
		case record.PGID > 1:
			if !groups[record.PGID] {
				members = append(members, groupMembers(record.PGID, "descendant of "+record.Role, stats, claimed, recorded, skip)...)
			}
			groups[record.PGID] = true
			units = append(units, unit{pid: record.PID, startTime: record.StartTime, group: record.PGID, members: members})
		case running[record.PID]:
			units = append(units, unit{pid: record.PID, startTime: record.StartTime, members: members})
		}
	}

	visited := map[int]bool{}
	for _, record := range records {
		if !running[record.PID] {
			continue
		}
		role := "descendant of " + record.Role
		pending := []int{record.PID}
		for len(pending) > 0 {
			parent := pending[0]
			pending = pending[1:]
			for _, child := range children[parent] {
				if visited[child.PID] {
					continue
				}
				visited[child.PID] = true
				pending = append(pending, child.PID)
				if child.Zombie || claimed[child.PID] || groups[child.PGID] {
					continue
				}
				claimed[child.PID] = true
				member := watched{pid: child.PID, startTime: child.StartTime, role: role}
				next := unit{pid: child.PID, startTime: child.StartTime, members: []watched{member}}
				if child.PGID == child.PID && child.PGID > 1 {
					next.group = child.PGID
					groups[child.PGID] = true
					next.members = append(next.members, groupMembers(child.PGID, role, stats, claimed, recorded, skip)...)
				}
				units = append(units, next)
			}
		}
	}
	return units
}

// groupMembers returns the live processes of group that no unit claims yet, bar recorded pids, which each form their own unit, and skipped pids.
// It marks the returned processes claimed.
func groupMembers(group int, role string, stats []proc.StatEntry, claimed, recorded map[int]bool, skip func(pid int) bool) []watched {
	var members []watched
	for _, entry := range stats {
		if entry.PGID != group || entry.Zombie || claimed[entry.PID] || recorded[entry.PID] || skip(entry.PID) {
			continue
		}
		claimed[entry.PID] = true
		members = append(members, watched{pid: entry.PID, startTime: entry.StartTime, role: role})
	}
	return members
}

// hasRunning reports whether stats holds member's pid, with its start time, and not as a zombie.
func hasRunning(member watched, stats []proc.StatEntry) bool {
	for _, entry := range stats {
		if entry.PID == member.pid && entry.StartTime == member.startTime && !entry.Zombie {
			return true
		}
	}
	return false
}

// groupHolds reports whether a live process other than a reused pid still reports the unit's group with a start time at or after the unit's.
// Linux keeps a group id unassigned while any member lives, so a match is the recorded group.
func groupHolds(u unit, stats []proc.StatEntry) bool {
	unitStart, err := strconv.ParseUint(u.startTime, 10, 64)
	if err != nil {
		return false
	}
	for _, entry := range stats {
		if entry.PGID != u.group || entry.Zombie || (entry.PID == u.pid && entry.StartTime != u.startTime) {
			continue
		}
		if start, err := strconv.ParseUint(entry.StartTime, 10, 64); err == nil && start >= unitStart {
			return true
		}
	}
	return false
}

func overlaps(members, remaining []watched) bool {
	for _, member := range members {
		if contains(remaining, member) {
			return true
		}
	}
	return false
}

func contains(members []watched, member watched) bool {
	for _, candidate := range members {
		if candidate.pid == member.pid && candidate.startTime == member.startTime {
			return true
		}
	}
	return false
}
