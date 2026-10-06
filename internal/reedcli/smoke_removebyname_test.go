//go:build tmux

package reedcli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
)

// TestSmokeRemoveByNameDetachedFromInsideStrand runs `lyx reed remove --name <n> --detach` from
// inside the strand it removes. The detached child must outlive the pane it kills, wait for the
// invoking process to exit, then remove the strand: reed.json stops listing it and the remaining
// pane is laid out at full height.
func TestSmokeRemoveByNameDetachedFromInsideStrand(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	lyxExe := lyxbin.Build(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	keeper := addStrand(t, smokeReapLaunchCmd(), "--name", "keeper")
	selfName := "selfrm"
	self := addStrand(t, smokeInvokeLine(lyxExe, "reed", "remove", "--name", selfName, "--detach"), "--name", selfName)
	socket, session := socketAndSession(t)

	deadline := time.Now().Add(60 * time.Second)
	for {
		out.Reset()
		if code := RunCLI(&out, []string{"status"}); code != 0 {
			t.Fatalf("status = %d; want 0, output: %s", code, out.String())
		}
		if _, listed := statusStrand(t, out.Bytes(), self); !listed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("strand %s still listed 60s after its own detached remove; status: %s", self, out.String())
		}
		time.Sleep(500 * time.Millisecond)
	}

	var status struct {
		Strands []struct {
			GUID string `json:"guid"`
			Live bool   `json:"live"`
		} `json:"strands"`
	}
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if len(status.Strands) != 1 || status.Strands[0].GUID != keeper || !status.Strands[0].Live {
		t.Fatalf("remaining strands = %+v; want only the live keeper %s", status.Strands, keeper)
	}

	// The remaining panes are laid out: no dead pane is left behind and every pane has height.
	lines := listPaneLines(t, tmuxPath, socket, session)
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) != 4 {
			t.Fatalf("unexpected list-panes row %q", l)
		}
		if f[1] != "0" {
			t.Errorf("dead pane left after removal: %q", l)
		}
		if f[3] == "0" {
			t.Errorf("pane has no height after re-layout: %q", l)
		}
	}
	if err := exec.Command(tmuxPath, "-L", socket, "has-session", "-t", session).Run(); err != nil {
		t.Errorf("reed session gone after self-removal: %v", err)
	}
}
