// pushnotifier_integration_test.go proves the failed-push notice route over a real hub, with creel's daemon run in process on a fake delivery.

package orchcli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// startCreelDaemon runs creel's daemon in process for the hub, listening on a fake delivery that holds receiver as a live strand.
// The client NotifyPrime builds connects to it, so it spawns no daemon of its own.
func startCreelDaemon(t *testing.T, h *hubforge.Hub, receiver string) {
	t.Helper()
	root, err := lymtemp.ProcessRoot()
	if err != nil {
		t.Fatalf("ProcessRoot() = %v", err)
	}
	geometry, err := hubgeom.CreelGeometry(h.Path, root.Path())
	if err != nil {
		t.Fatalf("CreelGeometry() = %v", err)
	}
	delivery := creelfake.NewDelivery()
	delivery.SetStrands(creelfake.LiveStrand(receiver))
	// Busy, so a posted mail is not carried into the pane and marked read before the test drops or supersedes it.
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
		t.Fatalf("the daemon ended before it listened: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the daemon did not listen")
	}
}

// startHubOrchDaemon records a prime orch strand for the hub, runs creel's daemon in process with that strand live, and returns the strand's name.
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
	if err := orchengine.SaveState(PrimePaths(h.Location), orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	reedState := &reedengine.ReedState{Strands: []reedengine.Strand{{GUID: "g1", Name: receiver}}}
	if err := reedengine.SaveState(filepath.Join(h.Location.AnchorPath(), lymdirs.DotLymDirName), reedState); err != nil {
		t.Fatalf("reedengine.SaveState: %v", err)
	}
	startCreelDaemon(t, h, receiver)
	return receiver
}

// postedMails returns the mails the hub's creel daemon logged as posted.
func postedMails(t *testing.T, h *hubforge.Hub) []creelengine.LogEntry {
	t.Helper()
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
	return posted
}

// undeliveredEntries returns the log entry of each message the hub's creel daemon returned undelivered, by message id.
func undeliveredEntries(t *testing.T, h *hubforge.Hub) map[int64]creelengine.LogEntry {
	t.Helper()
	reader, err := creelcli.NewProgramClient(h.Location, "CHECK")
	if err != nil {
		t.Fatalf("NewProgramClient: %v", err)
	}
	entries, err := reader.Log(creelengine.LogFilter{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	undelivered := map[int64]creelengine.LogEntry{}
	for _, entry := range entries {
		if entry.Event == "undelivered" {
			undelivered[entry.ID] = entry
		}
	}
	return undelivered
}

// TestCreelNoticeTags drives the tags a notice carries through a real creel client and daemon:
// the topic supersedes the sender's earlier unread notice, and the run lets the teardown dropper drop it.
//
// It runs without t.Parallel: the push notifier and the strand environment variable are process-global.
func TestCreelNoticeTags(t *testing.T) {
	tiergate.Require(t, 1)

	// failedPushNotice records a push failure of the pair branch "task" over a hub whose prime orch is live.
	// It returns the hub, the orch's strand name and the id of the mail the notifier posted.
	failedPushNotice := func(t *testing.T) (*hubforge.Hub, string, int64) {
		t.Helper()
		t.Setenv(agentname.StrandNameEnv, "")
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		receiver := startHubOrchDaemon(t, h)
		t.Cleanup(fabricengine.InstallPushNotifier(func(string, fabricengine.PushNotice) (bool, error) { return false, nil }))
		InstallPushNotifier(lymcwd.WithCwd(context.Background(), fabricengine.BoardDir(h.Path)))
		failure := fabricengine.FailedPush{
			Repo: "code", RepoPath: h.PairCodeWorktree("task"), Branch: "task",
			Class: fabricengine.PushFailureTransport, Error: "connection refused",
		}
		if err := fabricengine.RecordPushFailure(h.Path, failure, time.Now()); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}
		posted := postedMails(t, h)
		if len(posted) != 1 {
			t.Fatalf("posted events = %+v; want exactly one", posted)
		}
		return h, receiver, posted[0].ID
	}

	t.Run("FailedPushNoticeIsSupersededByANewerPostOfItsTopic", func(t *testing.T) {
		h, receiver, first := failedPushNotice(t)
		fabric, err := creelcli.NewProgramClient(h.Location, fabricProgram)
		if err != nil {
			t.Fatalf("NewProgramClient: %v", err)
		}

		if _, err := fabric.PostTagged(receiver, "newer", creelengine.PostTags{Topic: "push/code/task"}); err != nil {
			t.Fatalf("PostTagged: %v", err)
		}

		if got := undeliveredEntries(t, h)[first].Reason; got != creelengine.SupersededReason {
			t.Errorf("undelivered reason of the failed-push notice #%d = %q; want %q", first, got, creelengine.SupersededReason)
		}
	})

	t.Run("FailedPushNoticeIsDroppedWithItsRun", func(t *testing.T) {
		h, _, first := failedPushNotice(t)

		dropped, err := dropRunMail(h.Location, "task")

		if err != nil || dropped != 1 {
			t.Fatalf("dropRunMail(task) = %d, %v; want 1, nil", dropped, err)
		}
		if got := undeliveredEntries(t, h)[first].Reason; got != creelengine.RunGoneReason {
			t.Errorf("undelivered reason of the failed-push notice #%d = %q; want %q", first, got, creelengine.RunGoneReason)
		}
		if again, err := dropRunMail(h.Location, "other"); err != nil || again != 0 {
			t.Errorf("dropRunMail(other) = %d, %v; want 0, nil", again, err)
		}
	})

	t.Run("NotifyParentMailIsDroppedWithThePairsSlug", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, "")
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, "task1")
		startHubOrchDaemon(t, h)
		pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("task1"))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}

		if posted, err := NotifyParent(pair, "CHECK", "a pair notice"); err != nil || !posted {
			t.Fatalf("NotifyParent() = %v, %v; want posted", posted, err)
		}

		if dropped, err := dropRunMail(h.Location, "task1"); err != nil || dropped != 1 {
			t.Errorf("dropRunMail(task1) = %d, %v; want the pair's notice dropped", dropped, err)
		}
	})
}

// TestInstallPushNotifier covers the install from the board worktree and from outside a hub.
//
// It runs without t.Parallel: the push notifier and the strand environment variable are process-global.
func TestInstallPushNotifier(t *testing.T) {
	tiergate.Require(t, 1)
	t.Run("InstalledFromTheBoardWorktreeAFailureMailsTheOrchAsFabric", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, "")
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		receiver := startHubOrchDaemon(t, h)
		t.Cleanup(fabricengine.InstallPushNotifier(func(string, fabricengine.PushNotice) (bool, error) { return false, nil }))

		InstallPushNotifier(lymcwd.WithCwd(context.Background(), fabricengine.BoardDir(h.Path)))
		failure := fabricengine.FailedPush{
			Repo: "code", RepoPath: h.PrimeWorktree(), Branch: "main", Prime: true,
			Class: fabricengine.PushFailureTransport, Error: "connection refused",
		}
		if err := fabricengine.RecordPushFailure(h.Path, failure, time.Now()); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}

		wantLine := fmt.Sprintf("a push of %s branch %s failed (%s): %s; way forward: %s", failure.Repo, failure.Branch, failure.Class, failure.Error, fabricengine.FailedPushWayForward(failure))
		posted := postedMails(t, h)
		if len(posted) != 1 {
			t.Fatalf("posted events = %+v; want exactly one", posted)
		}
		if posted[0].To != receiver || !strings.HasSuffix(posted[0].From, ":FABRIC") || posted[0].Subject != wantLine {
			t.Errorf("posted mail = from %q to %q subject %q; want from <shortname>:FABRIC to %q subject %q", posted[0].From, posted[0].To, posted[0].Subject, receiver, wantLine)
		}
		records, err := fabricengine.ListFailedPushes(h.Path)
		if err != nil || len(records) != 1 || !records[0].Notified {
			t.Errorf("ListFailedPushes() = %+v, %v; want one notified record", records, err)
		}
	})

	t.Run("OutsideAHubInstallsNothing", func(t *testing.T) {
		calls := 0
		t.Cleanup(fabricengine.InstallPushNotifier(func(string, fabricengine.PushNotice) (bool, error) {
			calls++
			return true, nil
		}))

		InstallPushNotifier(lymcwd.WithCwd(context.Background(), t.TempDir()))
		failure := fabricengine.FailedPush{Repo: "code", Branch: "main", Class: fabricengine.PushFailureTransport}
		if err := fabricengine.RecordPushFailure(t.TempDir(), failure, time.Now()); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}

		if calls != 1 {
			t.Errorf("sentinel notifier calls = %d; want 1, so the install changed nothing", calls)
		}
	})
}
