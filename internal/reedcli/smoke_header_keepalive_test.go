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

// TestSmokeHeaderBlockingKeepaliveDoesNotDeadlock checks that blockForever doesn't deadlock.
func TestSmokeHeaderBlockingKeepaliveDoesNotDeadlock(t *testing.T) {
	const childEnv = "REED_HEADER_KEEPALIVE_CHILD"

	if os.Getenv(childEnv) == "1" {
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

	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()

	select {
	case err := <-exited:
		t.Fatalf("keepalive child exited (%v); want it parked forever. Output:\n%s", err, output.String())
	case <-time.After(3 * time.Second):
		if strings.Contains(output.String(), "fatal error") {
			_ = child.Process.Kill()
			<-exited
			t.Fatalf("keepalive child printed a runtime fatal error while still tracked as running:\n%s", output.String())
		}
	}

	if err := child.Process.Kill(); err != nil {
		t.Fatalf("kill parked keepalive child: %v", err)
	}
	<-exited
}
