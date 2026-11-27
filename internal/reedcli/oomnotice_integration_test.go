// oomnotice_integration_test.go proves over a real hub that the watchdog's pane OOM notice reaches the parent orch.
// The strand, kill count and journal reads are faked.
// The post is the one engineOOMBoundaries builds.
// Creel's daemon runs in process on a fake delivery.

package reedcli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// startHubOrchDaemon records a prime orch strand for the hub and runs creel's daemon in process with that strand live.
// It returns the strand's name.
func startHubOrchDaemon(t *testing.T, h *hubforge.Hub) string {
	t.Helper()
	root, err := lymtemp.ProcessRoot()
	if err != nil {
		t.Fatalf("ProcessRoot() = %v", err)
	}
	geometry, err := hubgeom.CreelGeometry(h.Path, root.Path())
	if err != nil {
		t.Fatalf("CreelGeometry() = %v", err)
	}
	receiver := geometry.Shortname + ":orch-held"
	if err := orchengine.SaveState(orchcli.PrimePaths(h.Location), orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	reedState := &reedengine.ReedState{Strands: []reedengine.Strand{{GUID: "g1", Name: receiver}}}
	if err := reedengine.SaveState(filepath.Join(h.Location.AnchorPath(), lymdirs.DotLymDirName), reedState); err != nil {
		t.Fatalf("reedengine.SaveState: %v", err)
	}

	delivery := creelfake.NewDelivery()
	delivery.SetStrands(creelfake.LiveStrand(receiver))
	// Busy, so a posted mail is not carried into the pane and marked read before the test reads the log.
	delivery.SetIdle(receiver, false)
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
	return receiver
}

// TestReportPaneOOMKills_DeliversTheNoticeToTheParentOrchsCreelMailbox pins where a pane's OOM notice arrives.
// The notice goes through the post engineOOMBoundaries builds.
// It reaches the parent orch's creel mailbox as one mail with the notice line.
// It runs without t.Parallel: the strand environment variable is process-global.
func TestReportPaneOOMKills_DeliversTheNoticeToTheParentOrchsCreelMailbox(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "oom-task")
	receiver := startHubOrchDaemon(t, h)
	task, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("oom-task"))
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	// engineOOMBoundaries reads its engine only in strands, which this test replaces.
	boundaries := engineOOMBoundaries(nil, task)
	boundaries.strands = func() ([]reedengine.StrandStatus, error) {
		return []reedengine.StrandStatus{{Name: "ly:darn", PaneID: "%7", PanePID: 7}}, nil
	}
	boundaries.paneKills = func(int) (string, int, error) { return "/app.slice/lym-pane-7.scope", 1, nil }
	boundaries.journalKills = func(string) ([]proc.OOMKill, error) { return []proc.OOMKill{{PID: 11, Name: "script"}}, nil }

	if !reportPaneOOMKills(h.Path, "oom-task", boundaries, paneOOMKills{}) {
		t.Fatal("reportPaneOOMKills() = false; want the notice posted")
	}

	reader, err := creelcli.NewProgramClient(h.Location, "CHECK")
	if err != nil {
		t.Fatalf("NewProgramClient: %v", err)
	}
	entries, err := reader.Log(creelengine.LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	var posted []creelengine.LogEntry
	for _, entry := range entries {
		if entry.Event == "posted" {
			posted = append(posted, entry)
		}
	}
	const wantLine = "reed: the kernel's OOM killer ended script (pid 11) at the memory cap of strand ly:darn, pane %7, in worktree oom-task; the pane lives on"
	if len(posted) != 1 {
		t.Fatalf("posted mails = %+v; want exactly one", posted)
	}
	if posted[0].To != receiver || posted[0].Subject != wantLine {
		t.Errorf("posted mail = to %q subject %q; want to %q subject %q", posted[0].To, posted[0].Subject, receiver, wantLine)
	}
}
