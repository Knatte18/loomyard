// tierrun_memorycap_linux_test.go covers a tier run under its memory_mb where the user manager caps a scope's memory.
// The kernel kills the runaway command, and the tier reports a kill at its cap.

package gatecli

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunTierMemoryCapThatCannotBeSetRefusesWithItsWayForward(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	dir := t.TempDir()
	request := testRequest{root: dir, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay}
	run := tierRun{
		number:      1,
		tier:        gateslot.Tier{Command: "true", Counter: counterGoJSON, TimeoutMin: 1, MemoryMB: 64},
		invocations: []invocation{{dir: dir}},
	}

	// With only PATH, systemd-run cannot reach the user manager's bus, so the scope never runs the command.
	result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

	for _, want := range []string{"did not run under its gate.tiers.tier1.memory_mb of 64 MB", "remove gate.tiers.tier1.memory_mb"} {
		if !strings.Contains(result.refusal, want) {
			t.Errorf("runTier() refusal = %q; want it to hold %q", result.refusal, want)
		}
	}
}

func TestRunTierMemoryCapKillsARunawayCommandAndNamesTheCap(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}

	const memoryMB = 64
	dir := t.TempDir()
	var stderr bytes.Buffer
	request := testRequest{root: dir, stderr: &stderr, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay}
	// tail keeps a line with no newline in memory, and the zero bytes hold none, so it grows past the cap.
	// They reach it through a pipe, as tail seeks past a device file it can size.
	// head stops them at a fixed 256 MB, so a cap that does not hold fails the test and never the machine.
	run := tierRun{
		number: 1,
		tier: gateslot.Tier{
			Command: "head -c 268435456 /dev/zero | tail", Counter: counterGoJSON, TimeoutMin: 1, MemoryMB: memoryMB,
		},
		invocations: []invocation{{dir: dir}},
	}

	// The scope starts through the user manager's bus, which the whole environment names.
	result := runTier(context.Background(), io.Discard, run, os.Environ(), request)

	if result.refusal != "" {
		t.Fatalf("runTier() refusal = %q; want the tier to run under its cap", result.refusal)
	}
	capLine := tierMemoryCapLine("tier1", memoryMB)
	if result.record.ExitCode == 0 {
		t.Errorf("runTier() exit = 0; want a failing exit for a command the kernel killed at its cap")
	}
	if !result.memoryStopped {
		t.Errorf("runTier() memoryStopped = false; want it set, so the run is never rerun")
	}
	if !slices.Contains(result.causes, capLine) || !strings.Contains(stderr.String(), capLine) || !strings.Contains(string(result.log), capLine+"\n") {
		t.Errorf("runTier() causes = %q, stderr = %q, log = %q; want the memory cap line in each", result.causes, stderr.String(), result.log)
	}
}
