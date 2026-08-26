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

func TestAwaitRunLock_ReadyOnLaterIteration(t *testing.T) {
	calls := 0
	lockHeld := func() (bool, error) {
		calls++
		return calls >= 3, nil
	}
	alive := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, countingWait(&waits), 10)
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

	got, err := awaitRunLock(lockHeld, alive, countingWait(&waits), 10)
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

	got, err := awaitRunLock(lockHeld, alive, countingWait(&waits), 5)
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

	_, err := awaitRunLock(lockHeld, alive, countingWait(&waits), 10)
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

	got, err := awaitRunLock(lockHeld, alive, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockReady {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockReady even though alive() would report false", got)
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
// The ChildDied row is the regression guard: the verb used to test `result != awaitRunLockReady`,
// which collapsed a driver that ran and finished into the same refusal as a wedged spawn, reported
// "driver did not take the run lock" for a run that had taken it and released it, and skipped the
// tmux handover entirely -- so the status strand showing the actual halt was the one place the
// operator was not put.
func TestDispositionForHandshake(t *testing.T) {
	tests := []struct {
		name   string
		result awaitRunLockResult
		want   handshakeDisposition
	}{
		{"Ready", awaitRunLockReady, handshakeProceed},
		{"ChildDied", awaitRunLockChildDied, handshakeProceed},
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

func TestAttachArgv(t *testing.T) {
	got := attachArgv("my-socket", "my-session")
	want := []string{"-L", "my-socket", "attach-session", "-t", "=my-session"}
	if len(got) != len(want) {
		t.Fatalf("attachArgv() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("attachArgv()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
