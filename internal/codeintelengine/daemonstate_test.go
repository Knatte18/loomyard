// daemonstate_test.go covers daemonstate.go's state-file round-trip,
// two-part staleness check, and its concurrent-readers-never-see-a-partial-
// write property, plus probe.go's readiness gate. It is untagged and
// offline: every sub-test uses t.TempDir() and in-memory fakes, except
// sub-test (3) below (TestDaemonStale_DeadPIDIsStale), which spawns a
// short-lived exec.Command child to obtain a confirmed-dead PID fixture,
// mirroring internal/proc/isalive_test.go's own technique. Its spawn trips
// the Test Tier Purity Invariant guard and is allowlisted at the file level
// in cmd/lyx/tierpurity_test.go's allowedSpawners map.

package codeintelengine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestDaemonState_WriteReadRoundTrip asserts writeDaemonState followed by
// readDaemonState preserves every field exactly.
func TestDaemonState_WriteReadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "daemon.json")
	want := daemonState{
		PID:             12345,
		Address:         "unix;/tmp/example.sock",
		ProtocolVersion: supervisedProtocolVersion,
		StartedAt:       "2026-07-29T06:00:00Z",
	}

	if err := writeDaemonState(path, want); err != nil {
		t.Fatalf("writeDaemonState() failed: %v", err)
	}

	got, found, err := readDaemonState(path)
	if err != nil {
		t.Fatalf("readDaemonState() failed: %v", err)
	}
	if !found {
		t.Fatal("readDaemonState() found = false; want true")
	}
	if got != want {
		t.Errorf("readDaemonState() = %+v; want %+v", got, want)
	}
}

// TestReadDaemonState_MissingFileIsNotAnError asserts readDaemonState on a
// non-existent path returns (daemonState{}, false, nil), not an error — the
// common case on a worktree's first EnsureServer call.
func TestReadDaemonState_MissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	got, found, err := readDaemonState(path)
	if err != nil {
		t.Fatalf("readDaemonState() on missing file returned error: %v", err)
	}
	if found {
		t.Error("readDaemonState() on missing file found = true; want false")
	}
	if got != (daemonState{}) {
		t.Errorf("readDaemonState() on missing file = %+v; want zero value", got)
	}
}

// spawnAndWaitForDeadPID spawns a short-lived child process, waits for it
// to exit, and returns its now-dead PID. This is the one spawn this file
// performs, mirroring internal/proc/isalive_test.go's own technique.
func spawnAndWaitForDeadPID(t *testing.T) int {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "exit", "0")
	} else {
		cmd = exec.Command("true")
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start() failed: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("cmd.Wait() failed: %v", err)
	}
	return pid
}

// TestDaemonStale_DeadPIDIsStale asserts daemonStale reports true when the
// recorded PID names a confirmed-dead process, even with a matching
// ProtocolVersion — either half of the two-part check alone is sufficient.
func TestDaemonStale_DeadPIDIsStale(t *testing.T) {
	s := daemonState{
		PID:             spawnAndWaitForDeadPID(t),
		ProtocolVersion: supervisedProtocolVersion,
	}
	if !daemonStale(s) {
		t.Errorf("daemonStale(%+v) = false; want true (dead PID)", s)
	}
}

// TestDaemonStale_MismatchedProtocolVersionIsStale asserts daemonStale
// reports true when PID is the test's own live PID but ProtocolVersion does
// not match supervisedProtocolVersion.
func TestDaemonStale_MismatchedProtocolVersionIsStale(t *testing.T) {
	s := daemonState{
		PID:             os.Getpid(),
		ProtocolVersion: "stale-version",
	}
	if !daemonStale(s) {
		t.Errorf("daemonStale(%+v) = false; want true (mismatched protocol version)", s)
	}
}

// TestDaemonStale_LivePIDAndCurrentVersionIsNotStale asserts daemonStale
// reports false when both halves of the check pass: a live PID and a
// matching protocol version.
func TestDaemonStale_LivePIDAndCurrentVersionIsNotStale(t *testing.T) {
	s := daemonState{
		PID:             os.Getpid(),
		ProtocolVersion: supervisedProtocolVersion,
	}
	if daemonStale(s) {
		t.Errorf("daemonStale(%+v) = true; want false (live PID, current version)", s)
	}
}

// TestWriteDaemonState_ConcurrentReadersNeverSeeAPartialWrite runs one
// writer goroutine calling writeDaemonState in a loop alongside N reader
// goroutines calling readDaemonState concurrently on the same path, and
// asserts every read either finds no file yet (found=false) or parses
// cleanly — never a truncated-JSON unmarshal error. This is exactly the
// failure mode the temp-file-then-rename write sequence exists to prevent.
// The iteration count is kept small and bounded so this stays fast enough
// for the default go test run.
func TestWriteDaemonState_ConcurrentReadersNeverSeeAPartialWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.json")

	const writes = 50
	const readers = 8

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(stop)
		for i := 0; i < writes; i++ {
			s := daemonState{
				PID:             i,
				Address:         "unix;/tmp/example.sock",
				ProtocolVersion: supervisedProtocolVersion,
				StartedAt:       "2026-07-29T06:00:00Z",
			}
			if err := writeDaemonState(path, s); err != nil {
				t.Errorf("writeDaemonState() failed: %v", err)
				return
			}
		}
	}()

	var readerWG sync.WaitGroup
	for r := 0; r < readers; r++ {
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			for {
				_, _, err := readDaemonState(path)
				if err != nil {
					t.Errorf("readDaemonState() returned an error during concurrent writes (partial-write leak): %v", err)
					return
				}
				select {
				case <-stop:
					return
				default:
				}
			}
		}()
	}

	wg.Wait()
	readerWG.Wait()
}

// TestProbe_EmptyWorkspaceSymbolResultReturnsNil asserts probe returns nil
// when the fake server answers workspace/symbol with an empty array.
func TestProbe_EmptyWorkspaceSymbolResultReturnsNil(t *testing.T) {
	clientTransport, serverTransport := newPipeTransportPair()
	defer clientTransport.Close()
	defer serverTransport.Close()

	client := newLSPClientFromRW(clientTransport)
	server := newFakeServer(serverTransport)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req, ok := server.readMessage(t)
		if !ok {
			return
		}
		if req.Method != "workspace/symbol" {
			t.Errorf("fakeServer: got request method %q; want %q", req.Method, "workspace/symbol")
			return
		}
		server.respond(t, req.ID, []map[string]any{})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := probe(ctx, client, 5*time.Second); err != nil {
		t.Fatalf("probe() returned unexpected error: %v", err)
	}
	<-done
}

// TestProbe_NoResponseReturnsErrServerTimeout asserts probe returns an
// ErrServerTimeout-satisfying error once its own short timeout expires,
// when the fake server never responds.
func TestProbe_NoResponseReturnsErrServerTimeout(t *testing.T) {
	clientTransport, serverTransport := newPipeTransportPair()
	defer clientTransport.Close()
	defer serverTransport.Close()

	client := newLSPClientFromRW(clientTransport)
	server := newFakeServer(serverTransport)

	// The fake server reads the workspace/symbol request off the pipe (an
	// io.Pipe Write blocks until something Reads it, so the request must be
	// drained for writeMessage to return at all) but deliberately never
	// answers it — probe's own short timeout, not a response, is what must
	// bound this test's runtime. done confirms the read completed before
	// this test function returns, so the goroutine never touches t after
	// the test has finished.
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.readMessage(t)
	}()

	ctx := context.Background()
	err := probe(ctx, client, 50*time.Millisecond)
	if err == nil {
		t.Fatal("probe() with an unresponsive server returned nil error; want ErrServerTimeout")
	}
	if !errors.Is(err, ErrServerTimeoutSentinel) {
		t.Errorf("probe() with an unresponsive server err = %v; want errors.Is(err, ErrServerTimeoutSentinel)", err)
	}
	<-done
}
