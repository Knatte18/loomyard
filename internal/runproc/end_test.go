//go:build tier0

// end_test.go tests the end sequence over a scripted process table and a fake clock, so no row spawns a process or waits.

package runproc

import (
	"errors"
	"fmt"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

// world is a scripted process table with a fake clock and a recorder of the signals sent to it.
// A signalled process leaves the table unless a row makes it ignore SIGTERM, ignore both signals, or turn zombie on SIGTERM.
type world struct {
	start         time.Time
	clock         time.Time
	table         map[int]proc.StatEntry
	signals       []string
	ignoreTerm    map[int]bool
	immortal      map[int]bool
	zombieOnTerm  map[int]bool
	failSignal    map[int]bool
	afterTerm     func(w *world)
	onFirstSignal func(w *world)
	slept         bool
	statsErr      error
}

func entry(pid, ppid, pgid int, startTime string) proc.StatEntry {
	return proc.StatEntry{PID: pid, PPID: ppid, PGID: pgid, StartTime: startTime}
}

func pidSet(pids []int) map[int]bool {
	set := map[int]bool{}
	for _, pid := range pids {
		set[pid] = true
	}
	return set
}

func (w *world) react(pid int, kill bool) {
	current, found := w.table[pid]
	if !found || w.immortal[pid] || (!kill && w.ignoreTerm[pid]) {
		return
	}
	if !kill && w.zombieOnTerm[pid] {
		current.Zombie = true
		w.table[pid] = current
		return
	}
	delete(w.table, pid)
}

func (w *world) record(kind string, kill bool, id int) {
	signalName := "term"
	if kill {
		signalName = "kill"
	}
	if len(w.signals) == 0 && w.onFirstSignal != nil {
		defer w.onFirstSignal(w)
	}
	w.signals = append(w.signals, fmt.Sprintf("%s %s %d", signalName, kind, id))
}

func (w *world) ender(goos string, ancestors []int) ender {
	return ender{
		stats: func() ([]proc.StatEntry, error) {
			if w.statsErr != nil {
				return nil, w.statsErr
			}
			var entries []proc.StatEntry
			for _, current := range w.table {
				entries = append(entries, current)
			}
			slices.SortFunc(entries, func(a, b proc.StatEntry) int { return a.PID - b.PID })
			return entries, nil
		},
		startTime: func(pid int) (string, bool) {
			current, found := w.table[pid]
			return current.StartTime, found
		},
		isAlive: func(pid int) bool {
			_, found := w.table[pid]
			return found
		},
		isAncestor: func(pid int) bool { return slices.Contains(ancestors, pid) },
		signalProcess: func(pid int, kill bool) error {
			if _, found := w.table[pid]; !found {
				return syscall.ESRCH
			}
			if w.failSignal[pid] {
				return syscall.EPERM
			}
			w.record("pid", kill, pid)
			w.react(pid, kill)
			return nil
		},
		signalGroup: func(pgid int, kill bool) error {
			var members []int
			for pid, current := range w.table {
				if current.PGID == pgid {
					members = append(members, pid)
				}
			}
			if len(members) == 0 {
				return syscall.ESRCH
			}
			w.record("group", kill, pgid)
			for _, pid := range members {
				w.react(pid, kill)
			}
			return nil
		},
		sleep: func(d time.Duration) {
			w.clock = w.clock.Add(d)
			if !w.slept && len(w.signals) > 0 {
				w.slept = true
				if w.afterTerm != nil {
					w.afterTerm(w)
				}
			}
		},
		now:  func() time.Time { return w.clock },
		goos: goos,
	}
}

func TestEnd(t *testing.T) {
	t.Parallel()

	setStart := func(pid int, startTime string) func(w *world) {
		return func(w *world) {
			current := w.table[pid]
			current.StartTime = startTime
			w.table[pid] = current
		}
	}
	setGroup := func(pid, pgid int) func(w *world) {
		return func(w *world) {
			current := w.table[pid]
			current.PGID = pgid
			w.table[pid] = current
		}
	}

	tests := []struct {
		name          string
		table         []proc.StatEntry
		records       []Record
		process       *Record
		ancestors     []int
		spare         []string
		role          string
		caller        int
		ignoreTerm    []int
		immortal      []int
		zombieOnTerm  []int
		failSignal    []int
		afterTerm     func(w *world)
		onFirstSignal func(w *world, dir string)
		goos          string
		statsErr      error
		wantSignals   []string
		wantEnded     []Ended
		wantSurvivors []Ended
		wantSpent     time.Duration
	}{
		{
			name:        "a recorded leader ends with one group signal",
			table:       []proc.StatEntry{entry(10, 1, 10, "100"), entry(11, 10, 10, "110")},
			records:     []Record{{PID: 10, PGID: 10, StartTime: "100", Role: RoleLoop}},
			wantSignals: []string{"term group 10"},
			wantEnded:   []Ended{{10, RoleLoop}, {11, "descendant of loop"}},
		},
		{
			name:        "a group member that ignores SIGTERM after its leader exits gets the group SIGKILL",
			table:       []proc.StatEntry{entry(12, 1, 12, "120"), entry(13, 12, 12, "130")},
			records:     []Record{{PID: 12, PGID: 12, StartTime: "120", Role: RoleLoop}},
			ignoreTerm:  []int{13},
			wantSignals: []string{"term group 12", "kill group 12"},
			wantEnded:   []Ended{{12, RoleLoop}, {13, "descendant of loop"}},
			wantSpent:   endGrace,
		},
		{
			name:          "a group whose leader is gone still reports a member that survives SIGKILL",
			table:         []proc.StatEntry{{PID: 12, PPID: 1, PGID: 12, StartTime: "120", Zombie: true}, entry(14, 1, 12, "140")},
			records:       []Record{{PID: 12, PGID: 12, StartTime: "120", Role: RoleLoop}},
			immortal:      []int{14},
			wantSignals:   []string{"term group 12", "kill group 12"},
			wantSurvivors: []Ended{{14, "descendant of loop"}},
			wantSpent:     endGrace + killBound,
		},
		{
			name:          "a process whose signal call fails is reported when it stays alive",
			table:         []proc.StatEntry{entry(15, 1, 5, "150")},
			records:       []Record{{PID: 15, StartTime: "150", Role: RoleLoop}},
			failSignal:    []int{15},
			wantSurvivors: []Ended{{15, RoleLoop}},
			wantSpent:     endGrace + killBound,
		},
		{
			name:        "a non-leader ends with its descendants signalled by pid",
			table:       []proc.StatEntry{entry(20, 1, 5, "200"), entry(21, 20, 5, "210"), entry(22, 21, 5, "220")},
			records:     []Record{{PID: 20, StartTime: "200", Role: RoleLoop}},
			wantSignals: []string{"term pid 20", "term pid 21", "term pid 22"},
			wantEnded:   []Ended{{20, RoleLoop}, {21, "descendant of loop"}, {22, "descendant of loop"}},
		},
		{
			name:        "a descendant leading its own group gets a group signal",
			table:       []proc.StatEntry{entry(30, 1, 5, "300"), entry(31, 30, 31, "310"), entry(32, 31, 31, "320")},
			records:     []Record{{PID: 30, StartTime: "300", Role: RolePush}},
			wantSignals: []string{"term pid 30", "term group 31"},
			wantEnded:   []Ended{{30, RolePush}, {31, "descendant of push"}, {32, "descendant of push"}},
		},
		{
			name:        "the caller and an ancestor of it are not signalled",
			table:       []proc.StatEntry{entry(40, 1, 5, "400"), entry(41, 40, 5, "410"), entry(42, 1, 5, "420")},
			records:     []Record{{PID: 40, StartTime: "400", Role: RoleBatten}, {PID: 41, StartTime: "410", Role: RoleDriver}, {PID: 42, StartTime: "420", Role: RoleLoop}},
			ancestors:   []int{40},
			caller:      41,
			wantSignals: []string{"term pid 42"},
			wantEnded:   []Ended{{42, RoleLoop}},
		},
		{
			name:        "a spared role is left alone with the processes below it",
			table:       []proc.StatEntry{entry(66, 1, 5, "660"), entry(67, 1, 5, "670"), entry(68, 67, 5, "680")},
			records:     []Record{{PID: 66, StartTime: "660", Role: RoleBatten}, {PID: 67, StartTime: "670", Role: RoleDobby}},
			spare:       []string{RoleDobby},
			wantSignals: []string{"term pid 66"},
			wantEnded:   []Ended{{66, RoleBatten}},
		},
		{
			name:        "an owner ended with no spared role reaches its dobby and the processes below it",
			table:       []proc.StatEntry{entry(66, 1, 5, "660"), entry(67, 1, 5, "670"), entry(68, 67, 5, "680")},
			records:     []Record{{PID: 66, StartTime: "660", Role: RoleBatten}, {PID: 67, StartTime: "670", Role: RoleDobby}},
			wantSignals: []string{"term pid 66", "term pid 67", "term pid 68"},
			wantEnded:   []Ended{{66, RoleBatten}, {67, RoleDobby}, {68, "descendant of dobby"}},
		},
		{
			name:        "ending one role leaves the other roles and the processes below them alone",
			table:       []proc.StatEntry{entry(70, 1, 70, "700"), entry(71, 70, 70, "710"), entry(72, 1, 5, "720"), entry(73, 72, 5, "730"), entry(74, 1, 74, "740"), entry(75, 74, 74, "750")},
			records:     []Record{{PID: 70, PGID: 70, StartTime: "700", Role: RoleDriver}, {PID: 72, StartTime: "720", Role: RoleDobby}, {PID: 74, PGID: 74, StartTime: "740", Role: RoleDriver}},
			role:        RoleDobby,
			wantSignals: []string{"term pid 72", "term pid 73"},
			wantEnded:   []Ended{{72, RoleDobby}, {73, "descendant of dobby"}},
		},
		{
			name:    "a recorded pid whose start time differs is left alone",
			table:   []proc.StatEntry{entry(50, 1, 5, "999")},
			records: []Record{{PID: 50, StartTime: "500", Role: RoleLoop}},
		},
		{
			name:    "a record that appears during the first pass is ended by the second",
			table:   []proc.StatEntry{entry(60, 1, 5, "600")},
			records: []Record{{PID: 60, StartTime: "600", Role: RoleLoop}},
			onFirstSignal: func(w *world, dir string) {
				w.table[61] = entry(61, 1, 5, "610")
				if err := Register(dir, "slug", Record{PID: 61, StartTime: "610", Role: RoleSync}); err != nil {
					panic(err)
				}
			},
			wantSignals: []string{"term pid 60", "term pid 61"},
			wantEnded:   []Ended{{60, RoleLoop}, {61, RoleSync}},
		},
		{
			name:        "a process that ignores SIGTERM is killed after the grace",
			table:       []proc.StatEntry{entry(70, 1, 5, "700")},
			records:     []Record{{PID: 70, StartTime: "700", Role: RoleLoop}},
			ignoreTerm:  []int{70},
			wantSignals: []string{"term pid 70", "kill pid 70"},
			wantEnded:   []Ended{{70, RoleLoop}},
			wantSpent:   endGrace,
		},
		{
			name:          "a process that survives SIGKILL is named in a survivors error",
			table:         []proc.StatEntry{entry(80, 1, 5, "800")},
			records:       []Record{{PID: 80, StartTime: "800", Role: RoleDriver}},
			immortal:      []int{80},
			wantSignals:   []string{"term pid 80", "kill pid 80"},
			wantSurvivors: []Ended{{80, RoleDriver}},
			wantSpent:     endGrace + killBound,
		},
		{
			name:         "a process that reads zombie after SIGTERM counts as ended",
			table:        []proc.StatEntry{entry(90, 1, 5, "900")},
			records:      []Record{{PID: 90, StartTime: "900", Role: RoleLoop}},
			zombieOnTerm: []int{90},
			wantSignals:  []string{"term pid 90"},
			wantEnded:    []Ended{{90, RoleLoop}},
		},
		{
			name:        "a descendant whose pid is reused between SIGTERM and SIGKILL gets no SIGKILL",
			table:       []proc.StatEntry{entry(100, 1, 5, "1000"), entry(101, 100, 5, "1010")},
			records:     []Record{{PID: 100, StartTime: "1000", Role: RoleLoop}},
			ignoreTerm:  []int{101},
			afterTerm:   setStart(101, "9999"),
			wantSignals: []string{"term pid 100", "term pid 101"},
			wantEnded:   []Ended{{100, RoleLoop}, {101, "descendant of loop"}},
		},
		{
			name:        "a recorded non-leader whose pid is reused between SIGTERM and SIGKILL gets no SIGKILL",
			table:       []proc.StatEntry{entry(110, 1, 5, "1100")},
			records:     []Record{{PID: 110, StartTime: "1100", Role: RoleLoop}},
			ignoreTerm:  []int{110},
			afterTerm:   setStart(110, "9999"),
			wantSignals: []string{"term pid 110"},
			wantEnded:   []Ended{{110, RoleLoop}},
		},
		{
			name:    "a recorded leader whose group no live process reports gets no group signal",
			table:   []proc.StatEntry{entry(120, 1, 7, "1200")},
			records: []Record{{PID: 120, PGID: 120, StartTime: "1200", Role: RoleLoop}},
		},
		{
			name:    "a recorded leader whose group only an older process reports gets no group signal",
			table:   []proc.StatEntry{entry(130, 1, 7, "1300"), entry(131, 1, 130, "50")},
			records: []Record{{PID: 130, PGID: 130, StartTime: "1300", Role: RoleLoop}},
		},
		{
			name:          "a leader whose group empties between SIGTERM and SIGKILL gets no group SIGKILL",
			table:         []proc.StatEntry{entry(140, 1, 140, "1400")},
			records:       []Record{{PID: 140, PGID: 140, StartTime: "1400", Role: RoleLoop}},
			ignoreTerm:    []int{140},
			afterTerm:     setGroup(140, 7),
			wantSignals:   []string{"term group 140"},
			wantSurvivors: []Ended{{140, RoleLoop}},
			wantSpent:     endGrace + killBound,
		},
		{
			name:          "a descendant leader whose group empties between SIGTERM and SIGKILL gets no group SIGKILL",
			table:         []proc.StatEntry{entry(150, 1, 5, "1500"), entry(151, 150, 151, "1510")},
			records:       []Record{{PID: 150, StartTime: "1500", Role: RoleLoop}},
			ignoreTerm:    []int{151},
			afterTerm:     setGroup(151, 7),
			wantSignals:   []string{"term pid 150", "term group 151"},
			wantEnded:     []Ended{{150, RoleLoop}},
			wantSurvivors: []Ended{{151, "descendant of loop"}},
			wantSpent:     endGrace + killBound,
		},
		{
			name:    "Windows signals nothing",
			table:   []proc.StatEntry{entry(160, 1, 160, "1600")},
			records: []Record{{PID: 160, PGID: 160, StartTime: "1600", Role: RoleLoop}},
			goos:    "windows",
		},
		{
			name:     "an unsupported process listing signals nothing",
			table:    []proc.StatEntry{entry(165, 1, 5, "1650")},
			records:  []Record{{PID: 165, StartTime: "1650", Role: RoleLoop}},
			statsErr: errors.ErrUnsupported,
		},
		{
			name:    "EndProcess leaves a pid whose start time differs alone",
			table:   []proc.StatEntry{entry(170, 1, 5, "999")},
			process: &Record{PID: 170, StartTime: "1700", Role: "orphan"},
		},
		{
			name:        "EndProcess ends a matching pid by SIGTERM",
			table:       []proc.StatEntry{entry(171, 1, 5, "1710")},
			process:     &Record{PID: 171, StartTime: "1710", Role: "orphan"},
			wantSignals: []string{"term pid 171"},
			wantEnded:   []Ended{{171, "orphan"}},
		},
		{
			name:        "EndProcess kills a pid that ignores SIGTERM after the grace",
			table:       []proc.StatEntry{entry(172, 1, 5, "1720")},
			process:     &Record{PID: 172, StartTime: "1720", Role: "orphan"},
			ignoreTerm:  []int{172},
			wantSignals: []string{"term pid 172", "kill pid 172"},
			wantEnded:   []Ended{{172, "orphan"}},
			wantSpent:   endGrace,
		},
		{
			name:        "EndProcess sends no SIGKILL to a pid reused between the signals",
			table:       []proc.StatEntry{entry(173, 1, 5, "1730")},
			process:     &Record{PID: 173, StartTime: "1730", Role: "orphan"},
			ignoreTerm:  []int{173},
			afterTerm:   setStart(173, "9999"),
			wantSignals: []string{"term pid 173"},
			wantEnded:   []Ended{{173, "orphan"}},
		},
		{
			name:          "EndProcess returns a survivors error for a pid that survives SIGKILL",
			table:         []proc.StatEntry{entry(174, 1, 5, "1740")},
			process:       &Record{PID: 174, StartTime: "1740", Role: "orphan"},
			immortal:      []int{174},
			wantSignals:   []string{"term pid 174", "kill pid 174"},
			wantSurvivors: []Ended{{174, "orphan"}},
			wantSpent:     endGrace + killBound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			begin := time.Unix(1_000_000, 0)
			w := &world{
				start:        begin,
				clock:        begin,
				table:        map[int]proc.StatEntry{},
				ignoreTerm:   pidSet(tt.ignoreTerm),
				immortal:     pidSet(tt.immortal),
				zombieOnTerm: pidSet(tt.zombieOnTerm),
				failSignal:   pidSet(tt.failSignal),
				afterTerm:    tt.afterTerm,
				statsErr:     tt.statsErr,
			}
			for _, current := range tt.table {
				w.table[current.PID] = current
			}
			if tt.onFirstSignal != nil {
				w.onFirstSignal = func(w *world) { tt.onFirstSignal(w, dir) }
			}
			for _, record := range tt.records {
				if err := Register(dir, "slug", record); err != nil {
					t.Fatalf("Register(%+v): %v", record, err)
				}
			}

			e := w.ender(tt.goos, tt.ancestors)
			var ended []Ended
			var err error
			if tt.process != nil {
				ended, err = e.endProcess(tt.process.PID, tt.process.StartTime, tt.process.Role)
			} else {
				ends := endsEveryRoleBut(tt.spare)
				if tt.role != "" {
					ends = endsOnlyRole(tt.role)
				}
				ended, err = e.end(dir, "slug", tt.caller, ends)
			}

			var survivors *SurvivorsError
			switch {
			case len(tt.wantSurvivors) > 0:
				if !errors.As(err, &survivors) || !slices.Equal(survivors.Survivors, tt.wantSurvivors) {
					t.Errorf("end error = %v; want a SurvivorsError of %v", err, tt.wantSurvivors)
				}
			case err != nil:
				t.Errorf("end error = %v; want none", err)
			}
			if !slices.Equal(ended, tt.wantEnded) {
				t.Errorf("ended = %v; want %v", ended, tt.wantEnded)
			}
			if !slices.Equal(w.signals, tt.wantSignals) {
				t.Errorf("signals = %v; want %v", w.signals, tt.wantSignals)
			}
			if spent := w.clock.Sub(w.start); spent < tt.wantSpent {
				t.Errorf("clock advanced %v; want at least %v", spent, tt.wantSpent)
			}
		})
	}
}
