// noticestamp_integration_test.go proves batten's notices about a run carry the run's autonomy, over a real hub.
// Creel's daemon runs in process over the hub's own delivery, whose reed reads a recorded tmux.

package battencli

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxrec"
)

// startNoticeOrch records a prime orch strand for the hub in reed's state.
// It also runs creel's daemon in process over the hub's own delivery.
// Its reed reads the hub's session over a recorded tmux on which no session runs,
// so the strand reads not live and a posted mail waits unread in the log the test reads.
// The notice clients then connect to the daemon and spawn none of their own.
func startNoticeOrch(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	root, err := lymtemp.ProcessRoot()
	if err != nil {
		t.Fatalf("ProcessRoot() = %v; want nil", err)
	}
	geometry, err := hubgeom.CreelGeometry(h.Path, root.Path())
	if err != nil {
		t.Fatalf("CreelGeometry() = %v; want nil", err)
	}
	receiver := geometry.Shortname + ":orch-held"
	if err := orchengine.SaveState(orchcli.PrimePaths(h.Location), orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1"}); err != nil {
		t.Fatalf("orchengine.SaveState() = %v; want nil", err)
	}
	reedState := &reedengine.ReedState{Strands: []reedengine.Strand{{GUID: "g1", Name: receiver}}}
	if err := reedengine.SaveState(filepath.Join(h.Location.AnchorPath(), lymdirs.DotLymDirName), reedState); err != nil {
		t.Fatalf("reedengine.SaveState() = %v; want nil", err)
	}
	reedGeometry, err := hubgeom.ReedGeometry(h.Location)
	if err != nil {
		t.Fatalf("ReedGeometry() = %v; want nil", err)
	}
	delivery := creelcli.NewHubDelivery(h.Path, tmuxrec.PlayOutside(t, "session-not-started", reedGeometry.SessionName))
	epoch := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- creelengine.Run(ctx, creelengine.Options{
			Geometry: geometry, Delivery: delivery, Clock: creelfake.NewClock(epoch), PollClock: creelfake.NewClock(epoch),
			ParentPID: proc.ParentPID, Ready: func() { close(ready) },
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("the creel daemon ended before it listened: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the creel daemon did not listen")
	}
}

// postedSubjects returns the subject of each mail the hub's creel daemon logged as posted, oldest first.
func postedSubjects(t *testing.T, h *hubforge.Hub) []string {
	t.Helper()
	reader, err := creelcli.NewProgramClient(h.Location, "CHECK")
	if err != nil {
		t.Fatalf("NewProgramClient() = %v; want nil", err)
	}
	entries, err := reader.Log(creelengine.LogFilter{})
	if err != nil {
		t.Fatalf("Log() = %v; want nil", err)
	}
	var subjects []string
	for _, entry := range entries {
		if entry.Event == "posted" {
			subjects = append(subjects, entry.Subject)
		}
	}
	return subjects
}

// writeRunAutonomy writes beam.autonomy under anchor as value.
func writeRunAutonomy(t *testing.T, anchor, value string) {
	t.Helper()
	path := configengine.ConfigFile(anchor, "beam")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("autonomy: "+value+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestNotifyRunParent_CarriesTheRunsAutonomy pins that batten's run notice is stamped.
// It covers a present pair and an absent one.
// The present pair "p" holds its own autonomy true over the prime's false.
// The absent run "gone" has no worktree, so its autonomy is the prime's false.
// It runs without t.Parallel: the strand environment variable is process-global.
func TestNotifyRunParent_CarriesTheRunsAutonomy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")

	h := noticeHub(t)
	startNoticeOrch(t, h)
	writeRunAutonomy(t, h.Location.AnchorPath(), "false")
	writeRunAutonomy(t, resolvePair(t, h, "p").AnchorPath(), "true")

	for _, run := range []string{"p", "gone"} {
		if posted, err := notifyRunParent(h.Location, run, "notice of "+run); err != nil || !posted {
			t.Fatalf("notifyRunParent(%q) = %v, %v; want posted", run, posted, err)
		}
	}

	want := []string{"notice of p; autonomy: true", "notice of gone; autonomy: false"}
	if got := postedSubjects(t, h); !slices.Equal(got, want) {
		t.Errorf("posted subjects = %q; want %q", got, want)
	}
}
