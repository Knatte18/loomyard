//go:build smoke

// smoke_header_keepalive_test.go pins the header pane's keepalive mechanism:
// blockForever must park the process indefinitely rather than dying. The
// pre-fix implementation used `select {}`, which — with no other goroutines
// in the process — trips Go's runtime deadlock detector and kills the
// keepalive instantly with "fatal error: all goroutines are asleep -
// deadlock!", crashing every header pane right after it printed its text
// (observed live). The test re-executes its own test binary as a child that
// calls blockForever and asserts the child neither exits nor prints a fatal
// runtime error within the observation window; the old code dies within
// milliseconds, so a regression fails fast while the fixed code passes
// deterministically. Tagged smoke because untagged reedcli tests must not
// spawn processes (Test Tier Purity Invariant).

package reedcli

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestSmokeHeaderBlockingKeepaliveDoesNotDeadlock re-runs this test binary
// with reedHeaderKeepaliveChild set, which makes the child branch below call
// blockForever directly. The parent then watches the child: exiting at all —
// in particular with the runtime's deadlock fatal error — is the regression;
// still running at the end of the observation window is the pass.
func TestSmokeHeaderBlockingKeepaliveDoesNotDeadlock(t *testing.T) {
	const childEnv = "REED_HEADER_KEEPALIVE_CHILD"

	if os.Getenv(childEnv) == "1" {
		// Child branch: park exactly like the header pane's keepalive tail.
		// The parent kills this process; blockForever never returns.
		blockForever()
	}

	var output bytes.Buffer
	child := exec.Command(os.Args[0], "-test.run", "TestSmokeHeaderBlockingKeepaliveDoesNotDeadlock")
	child.Stdout = &output
	child.Stderr = &output
	child.Env = append(os.Environ(), childEnv+"=1")
	if err := child.Start(); err != nil {
		t.Fatalf("start keepalive child process: %v", err)
	}

	// Watch for the failure signature instead of sleeping once: the broken
	// keepalive dies within milliseconds, so a regression fails almost
	// immediately, while the fixed keepalive simply stays alive until the
	// deadline (which is the pass condition — the child is then killed).
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()

	select {
	case err := <-exited:
		// The child is already reaped by the Wait goroutine on this path.
		t.Fatalf("keepalive child exited (%v); want it parked forever. Output:\n%s", err, output.String())
	case <-time.After(3 * time.Second):
		if strings.Contains(output.String(), "fatal error") {
			_ = child.Process.Kill()
			<-exited
			t.Fatalf("keepalive child printed a runtime fatal error while still tracked as running:\n%s", output.String())
		}
	}

	// Pass path: kill AND reap the parked child — an unreaped child still
	// holds the test binary open on Windows and breaks go test's own
	// build-cache cleanup.
	if err := child.Process.Kill(); err != nil {
		t.Fatalf("kill parked keepalive child: %v", err)
	}
	<-exited
}
