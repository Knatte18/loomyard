package loomcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shell"
)

func TestMustSpawnDriver(t *testing.T) {
	tests := []struct {
		name        string
		runLockHeld bool
		wantSpawn   bool
	}{
		{"LockHeld_NoSpawn", true, false},
		{"LockFree_Spawn", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustSpawnDriver(tt.runLockHeld)
			if got != tt.wantSpawn {
				t.Errorf("mustSpawnDriver(%v) = %v; want %v", tt.runLockHeld, got, tt.wantSpawn)
			}
		})
	}
}

// countingWait returns a wait seam that counts its own invocations, for use in place of a real
// sleep in awaitRunLock tests.
func countingWait(count *int) func() {
	return func() {
		*count++
	}
}

// stillRunning is the halted seam for every awaitRunLock test whose scenario is not about the
// halted arm: it reports the machine still running, which is what keeps those scenarios reaching
// the arm they are actually about.
func stillRunning() bool { return false }

func TestAwaitRunLock_ReadyOnLaterIteration(t *testing.T) {
	calls := 0
	lockHeld := func() (bool, error) {
		calls++
		return calls >= 3, nil
	}
	alive := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockReady {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockReady", got)
	}
	if waits != 2 {
		t.Errorf("awaitRunLock() waited %d times; want 2", waits)
	}
}

func TestAwaitRunLock_ChildDied(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	aliveCalls := 0
	alive := func() bool {
		aliveCalls++
		return aliveCalls < 2
	}
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockChildDied {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockChildDied", got)
	}
}

func TestAwaitRunLock_Deadline(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	alive := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 5)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockDeadline {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockDeadline", got)
	}
	if waits != 5 {
		t.Errorf("awaitRunLock() waited %d times; want 5", waits)
	}
}

func TestAwaitRunLock_LockSeamErrors(t *testing.T) {
	wantErr := errors.New("boom")
	lockHeld := func() (bool, error) { return false, wantErr }
	alive := func() bool { return true }
	waits := 0

	_, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 10)
	if !errors.Is(err, wantErr) {
		t.Errorf("awaitRunLock() error = %v; want %v", err, wantErr)
	}
	if waits != 0 {
		t.Errorf("awaitRunLock() waited %d times on immediate lock error; want 0", waits)
	}
}

func TestAwaitRunLock_ReadyBeforeAliveCheck_ChildAboutToExit(t *testing.T) {
	// The order is load-bearing: a child that took the lock and is about to exit must still be
	// reported ready, so lockHeld reporting true must win even when alive would report false.
	lockHeld := func() (bool, error) { return true, nil }
	alive := func() bool { return false }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockReady {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockReady even though alive() would report false", got)
	}
}

// TestAwaitRunLock_HaltedWhileChildStillAlive is the regression guard for the defect Tier 2
// introduced in `lyx loom run`'s handshake. shedengine.Run releases the run lock on return, and
// `lyx loom drive` then spends up to friction_timeout_min -- thirty minutes in the shipped template
// -- running the friction reflection agent, against a handshake budget of thirty seconds. Before the
// halted seam existed, that combination (lock free, child alive, machine finished) fell through to
// awaitRunLockDeadline, which dispositionForHandshake refuses: a healthy run was reported as
// "driver did not take the run lock" and the bootstrap skipped its own terminal handover. Reproduced
// live against a real hub before this test was written.
func TestAwaitRunLock_HaltedWhileChildStillAlive(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	alive := func() bool { return true }
	halted := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, halted, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockHalted {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockHalted -- a driver whose machine finished but whose process is still doing post-run bookkeeping is not a wedged spawn", got)
	}
	if waits != 0 {
		t.Errorf("awaitRunLock() waited %d times; want 0 -- the halt is observable on the first poll", waits)
	}
}

// TestAwaitRunLock_HaltedCheckedAfterAliveCheck pins the seam order: a child that is already gone
// reports child-died even when halted would also report true, so the more specific signal wins and
// the existing ChildDied scenario keeps its meaning.
func TestAwaitRunLock_HaltedCheckedAfterAliveCheck(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	alive := func() bool { return false }
	halted := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, halted, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockChildDied {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockChildDied even though halted() also reports true", got)
	}
}

// TestAwaitRunLock_StillRunningChildAliveStillReachesDeadline is the other half of the guard: the
// halted seam must not make the genuine refusal unreachable. A child that is alive, never takes the
// lock, and leaves the machine in running is still a wedged spawn and must still hit the deadline.
func TestAwaitRunLock_StillRunningChildAliveStillReachesDeadline(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	alive := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 4)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockDeadline {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockDeadline -- the halted arm must not swallow the wedged-spawn refusal", got)
	}
	if waits != 4 {
		t.Errorf("awaitRunLock() waited %d times; want 4", waits)
	}
}

func TestFindStatusStrand(t *testing.T) {
	strands := []reedengine.StrandStatus{
		{GUID: "g1", Name: "loom-status-extra", PaneID: "%1"},
		{GUID: "g2", Name: statusStrandDisplayName, PaneID: "%2"},
	}

	tests := []struct {
		name      string
		strands   []reedengine.StrandStatus
		want      string
		wantFound bool
	}{
		{"Hit", strands, "loom-status", true},
		{"Missing", strands, "no-such-strand", false},
		{"ExactNameOverPrefix", strands, statusStrandDisplayName, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := findStatusStrand(tt.strands, tt.want)
			if found != tt.wantFound {
				t.Fatalf("findStatusStrand(..., %q) found = %v; want %v", tt.want, found, tt.wantFound)
			}
			if found && got.Name != tt.want {
				t.Errorf("findStatusStrand(..., %q) = %+v; want Name %q", tt.want, got, tt.want)
			}
		})
	}
}

func TestStatusStrandCmd(t *testing.T) {
	tests := []struct {
		name string
		sh   shell.Shell
		exe  string
		want string
	}{
		{"Posix", shell.Posix(), "/usr/local/bin/lyx", `/usr/local/bin/lyx loom status --watch`},
		{"Pwsh", shell.Pwsh(), `C:\lyx.exe`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusStrandCmd(tt.sh, tt.exe)
			wantPrefix := tt.sh.Invoke(tt.exe)
			if got == "" {
				t.Fatalf("statusStrandCmd(%q) = empty string", tt.exe)
			}
			if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
				t.Errorf("statusStrandCmd(%q) = %q; want prefix %q", tt.exe, got, wantPrefix)
			}
			for _, want := range []string{"loom", "status", "--watch"} {
				if !containsSubstring(got, want) {
					t.Errorf("statusStrandCmd(%q) = %q; want it to contain %q", tt.exe, got, want)
				}
			}
		})
	}
}

// containsSubstring is a tiny local helper so this test file adds no new import for a one-line
// substring check.
func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestDispositionForHandshake pins that only a deadline refuses the bootstrap.
// The ChildDied row is the original regression guard: the verb used to test
// `result != awaitRunLockReady`, which collapsed a driver that ran and finished into the same
// refusal as a wedged spawn, reported "driver did not take the run lock" for a run that had taken it
// and released it, and skipped the tmux handover entirely -- so the status strand showing the actual
// halt was the one place the operator was not put.
// The Halted row guards the same failure arriving through Tier 2's door: a driver whose machine has
// finished but whose process is still running the friction reflection agent is alive with the lock
// free, which used to land on the refusal below.
func TestDispositionForHandshake(t *testing.T) {
	tests := []struct {
		name   string
		result awaitRunLockResult
		want   handshakeDisposition
	}{
		{"Ready", awaitRunLockReady, handshakeProceed},
		{"ChildDied", awaitRunLockChildDied, handshakeProceed},
		{"Halted", awaitRunLockHalted, handshakeProceed},
		{"Deadline", awaitRunLockDeadline, handshakeRefuse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dispositionForHandshake(tt.result); got != tt.want {
				t.Errorf("dispositionForHandshake(%v) = %v; want %v", tt.result, got, tt.want)
			}
		})
	}
}

// TestResolveStatusStrandAction is the regression guard for a status pane that never came back.
// The DeadEntry row is the defect: the bootstrap used to decide by presence alone, and reed keeps
// tracking a strand whose pane is gone, so after any reed server restart -- a reboot, a crash, a
// kill-server, or reed's own zombie-boot force-reap -- every subsequent "lyx loom run" in that
// worktree saw the stale "loom-status" entry, reported "already there", and left the operator with
// no status read-out at all.
func TestResolveStatusStrandAction(t *testing.T) {
	tests := []struct {
		name     string
		strands  []reedengine.StrandStatus
		want     statusStrandAction
		wantGUID string
	}{
		{
			name:    "NoStrandsAtAll",
			strands: nil,
			want:    statusStrandAdd,
		},
		{
			name:    "OnlyOtherStrands",
			strands: []reedengine.StrandStatus{{GUID: "g1", Name: "discussion::g1", PaneID: "%2", Live: true}},
			want:    statusStrandAdd,
		},
		{
			name:     "LiveStatusStrand",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: true}},
			want:     statusStrandKeep,
			wantGUID: "g0",
		},
		{
			name:     "DeadEntryWithClearedPaneBinding",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "", Live: false}},
			want:     statusStrandReplace,
			wantGUID: "g0",
		},
		{
			name:     "DeadEntryWithADeadPane",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: false}},
			want:     statusStrandReplace,
			wantGUID: "g0",
		},
		{
			name: "LiveStatusStrandAmongOthers",
			strands: []reedengine.StrandStatus{
				{GUID: "g1", Name: "plan::g1", PaneID: "%3", Live: true},
				{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: true},
			},
			want:     statusStrandKeep,
			wantGUID: "g0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotGUID := resolveStatusStrandAction(tt.strands)
			if got != tt.want {
				t.Errorf("resolveStatusStrandAction(%+v) action = %v; want %v", tt.strands, got, tt.want)
			}
			if gotGUID != tt.wantGUID {
				t.Errorf("resolveStatusStrandAction(%+v) guid = %q; want %q", tt.strands, gotGUID, tt.wantGUID)
			}
		})
	}
}
