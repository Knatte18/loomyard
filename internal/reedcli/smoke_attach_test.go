//go:build smoke

package reedcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// TestSmokeAttachRendersInsideHarnessPane drives the interactive terminal handover of `lyx reed attach` against a real ConPTY terminal.
func TestSmokeAttachRendersInsideHarnessPane(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	shellPath := harnessShellBinaryPath(t)
	lyxExe := buildLyxBinary(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"reed": reedengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	addStrand(t, smokeMarkerLaunchCmd("ATTACH-MARKER-ALPHA"), "--name", "amarker")
	reedSocket, session := socketAndSession(t)

	harness := fmt.Sprintf("lyx-attach-harness-%d", os.Getpid())
	if err := exec.Command(tmuxPath, "-L", harness, "new-session", "-d", "-s", "h", "-x", "140", "-y", "42",
		shellPath).Run(); err != nil {
		t.Fatalf("boot harness server: %v", err)
	}
	t.Cleanup(func() {
		reapHarnessServer(t, tmuxPath, harness)
	})
	deadline := time.Now().Add(30 * time.Second)
	for exec.Command(tmuxPath, "-L", harness, "has-session", "-t", "h").Run() != nil {
		if time.Now().After(deadline) {
			t.Fatal("harness session did not come up within 30s")
		}
		time.Sleep(100 * time.Millisecond)
	}

	harnessPane := harnessOnlyPaneID(t, tmuxPath, harness, "h")
	sendKeysLine(t, tmuxPath, harness, harnessPane, smokeAttachInvokeLine(lyxExe))
	pollPaneContains(t, tmuxPath, harness, harnessPane, "ATTACH-MARKER-ALPHA", 20*time.Second)

	if err := exec.Command(tmuxPath, "-L", harness, "send-keys", "-t", harnessPane, "C-b", "d").Run(); err != nil {
		t.Fatalf("send detach keys: %v", err)
	}
	pollPaneContains(t, tmuxPath, harness, harnessPane, "ATTACH-EXIT:0", 15*time.Second)

	if err := exec.Command(tmuxPath, "-L", reedSocket, "has-session", "-t", session).Run(); err != nil {
		t.Errorf("reed session %s gone after detach: %v", session, err)
	}
}
