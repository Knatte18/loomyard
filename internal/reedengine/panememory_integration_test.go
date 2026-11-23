// panememory_integration_test.go proves against a real tmux that a strand pane runs in a scope held to the memory cap.

package reedengine

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestAddStrand_ThePaneRunsInAScopeHeldToTheTemplatesMemoryCap(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	if !proc.MemoryCapCoversTree() {
		t.Skip("this host has no systemd user manager that caps a scope's memory")
	}
	e := newColdScratchEngine(t)

	s, err := e.AddStrand(AddSpec{NameOverride: "capped", Provider: "claude", Display: render.Display{Anchor: render.AnchorOwnWindow}})
	if err != nil {
		t.Fatalf("AddStrand = %v, want nil", err)
	}
	status, err := e.Status()
	if err != nil {
		t.Fatalf("Status = %v, want nil", err)
	}
	pid := 0
	for _, st := range status.Strands {
		if st.GUID == s.GUID {
			pid = st.PanePID
		}
	}
	if pid <= 0 {
		t.Fatalf("Status lists no live pane pid for the added strand: %+v", status.Strands)
	}
	listing, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cgroup")
	if err != nil {
		t.Fatal(err)
	}
	cgroup, found := "", false
	for _, line := range strings.Split(string(listing), "\n") {
		if cgroup, found = strings.CutPrefix(line, "0::"); found {
			break
		}
	}
	if !found {
		t.Fatalf("the pane shell's cgroup listing %q holds no cgroup v2 line", listing)
	}
	held, err := os.ReadFile(filepath.Join("/sys/fs/cgroup", cgroup, "memory.max"))
	want := strconv.Itoa(8192 << 20)
	if got := strings.TrimSpace(string(held)); err != nil || got != want {
		t.Errorf("memory.max of the pane's cgroup %s = %q, %v; want %s", cgroup, got, err, want)
	}
}
