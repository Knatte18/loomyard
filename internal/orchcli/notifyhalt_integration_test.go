// notifyhalt_integration_test.go proves a run's halt notices carry the run's autonomy over a real hub.
// Creel's daemon runs in process on a fake delivery.

package orchcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeBeamAutonomy writes beam.autonomy under anchor as value.
func writeBeamAutonomy(t *testing.T, anchor, value string) {
	t.Helper()
	path := configengine.ConfigFile(anchor, "beam")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("autonomy: "+value+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestNotifyHalt_CarriesTheRunsAutonomy pins that a halt notice names the autonomy of the run it is about.
// The value is read when the notice is built.
// The cases are the task's own value, a hand edit of it, and a notice sent from the beam for the task.
// It runs without t.Parallel: the strand environment variable is process-global.
func TestNotifyHalt_CarriesTheRunsAutonomy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "halt-beam", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "halt-task", fabricengine.AddOptions{SkipPush: true, Parent: "halt-beam"})
	startHubOrchDaemon(t, h)
	beam, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("halt-beam"))
	if err != nil {
		t.Fatalf("ResolveWorktree(beam): %v", err)
	}
	task, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("halt-task"))
	if err != nil {
		t.Fatalf("ResolveWorktree(task): %v", err)
	}
	writeBeamAutonomy(t, beam.AnchorPath(), "false")
	writeBeamAutonomy(t, task.AnchorPath(), "true")

	if posted, err := NotifyParentHalt(task, "CHECK", "the run halted"); err != nil || !posted {
		t.Fatalf("NotifyParentHalt() = %v, %v; want posted", posted, err)
	}
	writeBeamAutonomy(t, task.AnchorPath(), "false")
	if posted, err := NotifyParentHalt(task, "CHECK", "the run halted again"); err != nil || !posted {
		t.Fatalf("NotifyParentHalt() after the hand edit = %v, %v; want posted", posted, err)
	}
	tags := creelengine.PostTags{Run: "halt-task"}
	if posted, err := NotifyRunFromHalt(beam, "halt-task", "CHECK", tags, "the run halted from the beam"); err != nil || !posted {
		t.Fatalf("NotifyRunFromHalt() = %v, %v; want posted", posted, err)
	}

	want := []string{
		"the run halted; autonomy: true",
		"the run halted again; autonomy: false",
		"the run halted from the beam; autonomy: false",
	}
	var got []string
	for _, mail := range postedMails(t, h) {
		got = append(got, mail.Subject)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("posted subjects = %q; want %q", got, want)
	}
}
