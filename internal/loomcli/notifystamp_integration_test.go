// notifystamp_integration_test.go proves what loomcli's seams stamp on the notices they post, over a real hub:
// a halt notice carries the run's autonomy, and a verify-budget notice names the run.
// Creel's daemon runs in process on a fake delivery.

package loomcli

import (
	"context"
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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// startNoticeOrch records a prime orch strand for the hub and runs creel's daemon in process with that strand live.
// The daemon also lists each of others.
// The notice clients then connect to the daemon and spawn none of their own.
// It returns the daemon's fake delivery and the orch strand's name.
func startNoticeOrch(t *testing.T, h *hubforge.Hub, others ...creelengine.Strand) (*creelfake.Delivery, string) {
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
	delivery := creelfake.NewDelivery()
	delivery.SetStrands(append([]creelengine.Strand{creelfake.LiveStrand(receiver)}, others...)...)
	// Busy, so a posted mail is not carried into the pane before the test reads the log.
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
	return delivery, receiver
}

// postedEntries returns the log entry of each mail the hub's creel daemon logged as posted, oldest first.
func postedEntries(t *testing.T, h *hubforge.Hub) []creelengine.LogEntry {
	t.Helper()
	reader, err := creelcli.NewProgramClient(h.Location, "CHECK")
	if err != nil {
		t.Fatalf("NewProgramClient() = %v; want nil", err)
	}
	entries, err := reader.Log(creelengine.LogFilter{})
	if err != nil {
		t.Fatalf("Log() = %v; want nil", err)
	}
	var posted []creelengine.LogEntry
	for _, entry := range entries {
		if entry.Event == "posted" {
			posted = append(posted, entry)
		}
	}
	return posted
}

// postedSubjects returns the subject of each mail the hub's creel daemon logged as posted, oldest first.
func postedSubjects(t *testing.T, h *hubforge.Hub) []string {
	t.Helper()
	var subjects []string
	for _, entry := range postedEntries(t, h) {
		subjects = append(subjects, entry.Subject)
	}
	return subjects
}

// TestHaltNoticeSeams_CarryTheRunsAutonomy pins that the hand-back seam and the pre-push check post their lines stamped.
// The stamp is the autonomy the pair's beam.yaml holds when each line is built.
// It runs without t.Parallel: the strand environment variable is process-global.
func TestHaltNoticeSeams_CarryTheRunsAutonomy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")

	h := hubforge.CopyHub(t, pairHubShape)
	startNoticeOrch(t, h)
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(hubforge.TemplatePairSlug))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s) = %v; want nil", hubforge.TemplatePairSlug, err)
	}
	c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeTapestry}
	if err := c.wire(pair, pair.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}
	driverDeps, err := c.goDriverDepsFor(func() error { return nil })
	if err != nil {
		t.Fatalf("goDriverDepsFor() = %v; want nil", err)
	}
	handBack := driverDeps.HandBack.Notify
	prePush := newPrePushCheck(pair, shedrun.SelfRunID).notify

	tests := []struct {
		name     string
		autonomy string
		post     func(line string) error
	}{
		{"HandBackOn", "true", func(line string) error { _, err := handBack(sheddriver.Stop{}, line); return err }},
		{"HandBackOff", "false", func(line string) error { _, err := handBack(sheddriver.Stop{}, line); return err }},
		{"PrePushOn", "true", prePush},
		{"PrePushOff", "false", prePush},
	}
	var want []string
	for _, tt := range tests {
		writeConfigFile(t, configengine.ConfigFile(pair.AnchorPath(), "beam"), "autonomy: "+tt.autonomy+"\n")
		if err := tt.post(tt.name); err != nil {
			t.Fatalf("%s: post = %v; want nil", tt.name, err)
		}
		want = append(want, tt.name+"; autonomy: "+tt.autonomy)
	}

	if got := postedSubjects(t, h); !slices.Equal(got, want) {
		t.Errorf("posted subjects = %q; want %q", got, want)
	}
}

// TestVerifyBudgetNotice_NamesTheRun pins that a darn run's verify-budget notice opens with the run's slug.
// Several runs post to one orch, so a notice without the slug leaves the orch guessing which run it is about.
// It runs without t.Parallel: the strand environment variable is process-global.
func TestVerifyBudgetNotice_NamesTheRun(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")

	h := hubforge.CopyHub(t, pairHubShape)
	startNoticeOrch(t, h)
	seedDarnHubConfig(t, h.Path, "3")
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(hubforge.TemplatePairSlug))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s) = %v; want nil", hubforge.TemplatePairSlug, err)
	}
	c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
	_, budget, err := c.treeScopeSettings(pair)
	if err != nil {
		t.Fatalf("treeScopeSettings() = _, _, %v; want nil", err)
	}

	if err := budget.Notify("Darn-Write gate: the verify gate has run past its 30m0s budget"); err != nil {
		t.Fatalf("Notify() = %v; want nil", err)
	}

	want := []string{"loom " + hubforge.TemplatePairSlug + ": Darn-Write gate: the verify gate has run past its 30m0s budget"}
	if got := postedSubjects(t, h); !slices.Equal(got, want) {
		t.Errorf("posted subjects = %q; want %q", got, want)
	}
}
