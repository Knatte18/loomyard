//go:build !windows

// handover_integration_test.go drives PushRun over a hub pair.
// The records origin of the pair holds another machine's takeover entry.

package pairteardown_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/takeoverkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// handOverFixture is a hub whose pair has local transitions on both sides and a registered sleeping process.
// Another machine's takeover entry for the pair's run sits on origin's records branch.
type handOverFixture struct {
	h        *hubforge.Hub
	slug     string
	pair     *lymcwd.Location
	entry    shedengine.Takeover
	sleeper  *exec.Cmd
	sleeperG chan struct{}
}

// newHandOverFixture builds the fixture for the run slug.
func newHandOverFixture(t *testing.T, slug string) *handOverFixture {
	t.Helper()
	return newHandOverFixtureWith(t, slug, true)
}

// newHandOverFixtureWith builds the fixture for the run slug.
// The takeover entry is on the pair's records branch only when plantTakeover is set.
func newHandOverFixtureWith(t *testing.T, slug string, plantTakeover bool) *handOverFixture {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	added := hubforge.AddPair(t, h, slug)
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	if _, err := fabricengine.PushPairAnchored(pair, fabricengine.EnvSyncOptions(), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
		t.Fatalf("priming push error = %v", err)
	}
	f := &handOverFixture{h: h, slug: slug, pair: pair, entry: shedengine.Takeover{Machine: "oslo-takeover-test", Subject: "sub", At: "2026-11-16T10:00:00Z"}}

	gitkit.CommitFile(t, h.PairCodeWorktree(slug), "code-file.txt", "code\n", "code transition")
	gitkit.CommitFile(t, h.PairRecordsSibling(slug), "records-file.txt", "records\n", "records transition")
	if plantTakeover {
		f.plantRunRecord(t, fabricengine.RecordsBranchName(added.Branch), slug, shedengine.Status{State: shedengine.StateRunning}, &f.entry)
	}

	sleeper := exec.Command("sleep", "300")
	sleeper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sleeper.Start(); err != nil {
		t.Fatalf("start the sleeping process: %v", err)
	}
	f.sleeper, f.sleeperG = sleeper, make(chan struct{})
	go func() {
		_ = sleeper.Wait()
		close(f.sleeperG)
	}()
	t.Cleanup(func() { _ = syscall.Kill(-sleeper.Process.Pid, syscall.SIGKILL) })
	if err := runproc.Register(fabricengine.RunProcDir(h.Path), slug, runproc.ChildRecord(sleeper.Process.Pid, runproc.RoleBatten, []string{"sleep", "300"})); err != nil {
		t.Fatalf("register the sleeping process: %v", err)
	}
	return f
}

// plantRunRecord commits the status file of the run slug on branch from a second clone of the records origin.
// It then pushes the commit.
// A non-nil takeover is written as the record's takeover entry.
func (f *handOverFixture) plantRunRecord(t *testing.T, branch, slug string, status shedengine.Status, takeover *shedengine.Takeover) {
	t.Helper()

	_, statusRel, err := shedrun.TreeRunFileRels(slug)
	if err != nil {
		t.Fatalf("TreeRunFileRels(%q): %v", slug, err)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal the status: %v", err)
	}
	statusPath := filepath.Join(t.TempDir(), "status.json")
	if err := os.WriteFile(statusPath, encoded, 0o644); err != nil {
		t.Fatalf("write the status: %v", err)
	}
	if takeover != nil {
		if err := takeoverkit.WriteTakeover(statusPath, *takeover); err != nil {
			t.Fatalf("WriteTakeover: %v", err)
		}
	}
	content, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read the status: %v", err)
	}
	other := filepath.Join(t.TempDir(), "other")
	gitkit.Git(t, filepath.Dir(other), "clone", "--branch", branch, f.h.RecordsBare, other)
	gitkit.CommitFile(t, other, statusRel, string(content), "run record of "+slug)
	gitkit.Git(t, other, "push", "origin", branch)
}

// writeLocalBattenRecord writes the local batten record of the fixture's run, holding a pause intent.
func (f *handOverFixture) writeLocalBattenRecord(t *testing.T) shedengine.Status {
	t.Helper()

	status := shedengine.Status{State: shedengine.StatePaused, PauseIntent: &shedengine.PauseIntent{By: "test", At: "2026-11-16T09:00:00Z"}}
	if err := state.WriteJSON(shedrun.StatusFile(f.h.Location, f.slug), shedrun.StatusLock(f.h.Location, f.slug), status); err != nil {
		t.Fatalf("write the local batten record: %v", err)
	}
	return status
}

// requireProcessEnded fails the test unless the sleeping process exits.
func (f *handOverFixture) requireProcessEnded(t *testing.T) {
	t.Helper()

	select {
	case <-f.sleeperG:
	case <-time.After(30 * time.Second):
		t.Fatalf("the registered process %d is still running; want it ended by the hand-over", f.sleeper.Process.Pid)
	}
}

// requireHandedOver fails the test unless err wraps ErrHandedOver and names the entry's machine.
func (f *handOverFixture) requireHandedOver(t *testing.T, err error) {
	t.Helper()

	if !errors.Is(err, fabricengine.ErrHandedOver) || !strings.Contains(err.Error(), f.entry.Machine) {
		t.Fatalf("PushRun() error = %v; want one wrapping ErrHandedOver and naming %q", err, f.entry.Machine)
	}
}

// requireMark fails the test unless the hub holds this machine's hand-over mark naming the entry's machine.
func (f *handOverFixture) requireMark(t *testing.T) {
	t.Helper()

	mark, found, err := fabricengine.ReadHandOverMark(f.h.Path, f.slug)
	if err != nil || !found || mark.Machine != f.entry.Machine {
		t.Errorf("ReadHandOverMark() = %+v, %v, %v; want a mark naming %q", mark, found, err, f.entry.Machine)
	}
}

// captureNotices installs a push notifier that records each notice line, and returns the recorded lines.
// The notifier is process-wide, so a test calling it does not run in parallel.
func captureNotices(t *testing.T) *[]string {
	t.Helper()
	var lines []string
	t.Cleanup(fabricengine.InstallPushNotifier(func(_ string, notice fabricengine.PushNotice) (bool, error) {
		lines = append(lines, notice.Line)
		return true, nil
	}))
	return &lines
}

// TestHandOverOwnedElsewhere drives HandOverOwnedElsewhere over a hub pair with commits origin lacks.
// The rows install the process's push notifier, so none of them runs in parallel.
func TestHandOverOwnedElsewhere(t *testing.T) {
	tiergate.Require(t, 1)

	const slug = "owned-elsewhere-run"
	handOver := func(f *handOverFixture) error {
		return pairteardown.HandOverOwnedElsewhere(f.h.Location, slug, f.entry.Machine)
	}
	archiveTagsOn := func(t *testing.T, bare string) string {
		t.Helper()
		return gitkit.Git(t, bare, "tag", "--list", fabricengine.OwnerArchiveTagPrefix+slug+"/*")
	}
	requirePairGone := func(t *testing.T, f *handOverFixture) {
		t.Helper()
		for _, path := range []string{f.h.PairCodeWorktree(slug), f.h.PairRecordsSibling(slug)} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("stat %s error = %v; want the pair gone", path, err)
			}
		}
	}

	t.Run("OwedCommitsAndArchiveTagsReachOriginTheProcessEndsAndThePairIsGone", func(t *testing.T) {
		f := newHandOverFixtureWith(t, slug, false)
		f.writeLocalBattenRecord(t)
		// Only an uncommitted file leaves a tip origin lacks once the owed commits are pushed, so each side holds one to archive.
		for _, dir := range []string{f.h.PairCodeWorktree(slug), f.h.PairRecordsSibling(slug)} {
			if err := os.WriteFile(filepath.Join(dir, "uncommitted.txt"), []byte("uncommitted\n"), 0o644); err != nil {
				t.Fatalf("write an uncommitted file in %s: %v", dir, err)
			}
		}
		lines := captureNotices(t)
		codeBranch := gitkit.CurrentBranch(t, f.h.PairCodeWorktree(slug))
		codeHead := gitkit.RevParse(t, f.h.PairCodeWorktree(slug), "HEAD")
		recordsHead := gitkit.RevParse(t, f.h.PairRecordsSibling(slug), "HEAD")

		if err := handOver(f); err != nil {
			t.Fatalf("HandOverOwnedElsewhere() error = %v; want nil", err)
		}

		if got := gitkit.RevParse(t, f.h.RecordsBare, "refs/heads/"+fabricengine.RecordsBranchName(codeBranch)); got != recordsHead {
			t.Errorf("origin records tip = %s; want the local commit %s pushed first", got, recordsHead)
		}
		if got := gitkit.RevParse(t, f.h.CodeBare, "refs/heads/"+codeBranch); got != codeHead {
			t.Errorf("origin code tip = %s; want the local commit %s, with the origin branch standing", got, codeHead)
		}
		for name, bare := range map[string]string{"code": f.h.CodeBare, "records": f.h.RecordsBare} {
			if tags := archiveTagsOn(t, bare); tags == "" {
				t.Errorf("owner archive tags on the %s origin = none; want one for the uncommitted files", name)
			}
		}
		f.requireProcessEnded(t)
		if len(*lines) != 1 || !strings.Contains((*lines)[0], f.entry.Machine) || !strings.Contains((*lines)[0], "pause intent") {
			t.Errorf("notices = %q; want one naming %q and the pause intent", *lines, f.entry.Machine)
		}
		requirePairGone(t, f)
		f.requireMark(t)
	})

	t.Run("ARunAtLandIsHandedOverTheSameWay", func(t *testing.T) {
		f := newHandOverFixtureWith(t, slug, false)
		writeStatus(t, f.h.Location, slug, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Land"})

		if err := handOver(f); err != nil {
			t.Fatalf("HandOverOwnedElsewhere() error = %v; want nil", err)
		}

		f.requireProcessEnded(t)
		requirePairGone(t, f)
		f.requireMark(t)
	})

	t.Run("ATakeoverEntryOnOriginPushesNothingFirstAndAnswersNil", func(t *testing.T) {
		f := newHandOverFixtureWith(t, slug, false)
		primeBranch := gitkit.CurrentBranch(t, f.h.PrimeRecords())
		gitkit.Git(t, f.h.PrimeRecords(), "push", "--set-upstream", "origin", primeBranch)
		f.plantRunRecord(t, primeBranch, slug, shedengine.Status{State: shedengine.StateRunning}, &f.entry)
		originRecords := "refs/heads/" + fabricengine.RecordsBranchName(gitkit.CurrentBranch(t, f.h.PairCodeWorktree(slug)))
		before := gitkit.RevParse(t, f.h.RecordsBare, originRecords)

		if err := handOver(f); err != nil {
			t.Fatalf("HandOverOwnedElsewhere() error = %v; want nil", err)
		}

		if got := gitkit.RevParse(t, f.h.RecordsBare, originRecords); got != before {
			t.Errorf("origin records tip = %s; want it unchanged at %s, nothing pushed first", got, before)
		}
		f.requireProcessEnded(t)
		f.requireMark(t)
	})

	t.Run("AFailedPushReturnsItsErrorAndWritesNoArchiveTagAndNoMark", func(t *testing.T) {
		f := newHandOverFixtureWith(t, slug, false)
		hook := filepath.Join(f.h.RecordsBare, "hooks", "pre-receive")
		if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
			t.Fatalf("create the hooks directory: %v", err)
		}
		if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			t.Fatalf("write the pre-receive hook: %v", err)
		}

		err := handOver(f)

		if err == nil || !strings.Contains(err.Error(), "push its owed commits") {
			t.Errorf("HandOverOwnedElsewhere() error = %v; want one naming the push step", err)
		}
		for name, bare := range map[string]string{"code": f.h.CodeBare, "records": f.h.RecordsBare} {
			if tags := archiveTagsOn(t, bare); tags != "" {
				t.Errorf("owner archive tags on the %s origin = %q; want none", name, tags)
			}
		}
		if mark, found, markErr := fabricengine.ReadHandOverMark(f.h.Path, slug); markErr != nil || found {
			t.Errorf("ReadHandOverMark() = %+v, %v, %v; want no mark", mark, found, markErr)
		}
	})
}

// TestPushRun_HandsOverOnATakeover drives PushRun against another machine's takeover entry.
// The rows install the process's push notifier, so none of them runs in parallel.
func TestPushRun_HandsOverOnATakeover(t *testing.T) {
	tiergate.Require(t, 1)

	const slug = "hand-over-run"
	run := func(f *handOverFixture, insidePair bool) error {
		_, err := pairteardown.PushRun(f.h.Location, f.pair, slug, pairteardown.RunPush{LockWait: fabricengine.LockWaitUnbounded, InsidePair: insidePair})
		return err
	}
	notices := captureNotices

	t.Run("OutsideThePairTheProcessEndsThePairIsGoneAndTheMarkStands", func(t *testing.T) {
		f := newHandOverFixture(t, slug)

		err := run(f, false)

		f.requireHandedOver(t, err)
		f.requireProcessEnded(t)
		for _, path := range []string{f.h.PairCodeWorktree(slug), f.h.PairRecordsSibling(slug)} {
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Errorf("stat %s error = %v; want the pair gone", path, statErr)
			}
		}
		f.requireMark(t)
	})

	t.Run("InsideThePairItStaysAndTheErrorNamesThePairLeftForTheParent", func(t *testing.T) {
		f := newHandOverFixture(t, slug)

		err := run(f, true)

		f.requireHandedOver(t, err)
		if !strings.Contains(err.Error(), "left standing") {
			t.Errorf("PushRun() error = %v; want it to say the pair is left standing for the parent's dobby", err)
		}
		f.requireProcessEnded(t)
		if _, statErr := os.Stat(f.h.PairCodeWorktree(slug)); statErr != nil {
			t.Errorf("stat the pair error = %v; want the pair left standing", statErr)
		}
		f.requireMark(t)

		originRecords := gitkit.RevParse(t, f.h.RecordsBare, "refs/heads/"+fabricengine.RecordsBranchName(gitkit.CurrentBranch(t, f.h.PairCodeWorktree(slug))))
		err = run(f, true)
		f.requireHandedOver(t, err)
		if got := gitkit.RevParse(t, f.h.RecordsBare, "refs/heads/"+fabricengine.RecordsBranchName(gitkit.CurrentBranch(t, f.h.PairCodeWorktree(slug)))); got != originRecords {
			t.Errorf("origin records tip = %s after the second PushRun; want it unchanged at %s", got, originRecords)
		}
	})

	t.Run("ALocalPauseIntentOriginLacksIsNamedInTheNotice", func(t *testing.T) {
		f := newHandOverFixture(t, slug)
		gitkit.Git(t, f.h.PrimeRecords(), "push", "--set-upstream", "origin", gitkit.CurrentBranch(t, f.h.PrimeRecords()))
		f.writeLocalBattenRecord(t)
		lines := notices(t)

		f.requireHandedOver(t, run(f, true))

		if len(*lines) != 1 || !strings.Contains((*lines)[0], "pause intent") {
			t.Errorf("notices = %q; want one naming the pause intent", *lines)
		}
	})

	t.Run("AStalePrimeTrackingRefNamesNoIntentOriginAlreadyHolds", func(t *testing.T) {
		f := newHandOverFixture(t, slug)
		primeBranch := gitkit.CurrentBranch(t, f.h.PrimeRecords())
		gitkit.Git(t, f.h.PrimeRecords(), "push", "--set-upstream", "origin", primeBranch)
		local := f.writeLocalBattenRecord(t)
		f.plantRunRecord(t, primeBranch, slug, local, nil)
		lines := notices(t)

		f.requireHandedOver(t, run(f, true))

		if len(*lines) != 1 || strings.Contains((*lines)[0], "pause intent") {
			t.Errorf("notices = %q; want one naming no intent, since origin holds the pause intent", *lines)
		}
	})

	// primeAhead builds the fixture whose prime records hold unpushed commits of two batten runs, interleaved.
	// The run slug is taken over on origin, and origin's prime records branch moved past the local tip.
	// It returns the fixture, the prime's branch and the prime's local tip before any push.
	const keptSlug = "kept-run"
	primeAhead := func(t *testing.T) (*handOverFixture, string, string) {
		t.Helper()
		boardkit.InstallReplay(t)
		f := newHandOverFixtureWith(t, slug, false)
		primeRecords := f.h.PrimeRecords()
		branch := gitkit.CurrentBranch(t, primeRecords)
		gitkit.Git(t, primeRecords, "push", "--set-upstream", "origin", branch)
		for i, runSlug := range []string{keptSlug, slug, keptSlug} {
			_, statusRel, err := shedrun.TreeRunFileRels(runSlug)
			if err != nil {
				t.Fatalf("TreeRunFileRels(%q): %v", runSlug, err)
			}
			message := fmt.Sprintf("transition %d of %s\n\n%s: %s", i, runSlug, fabricengine.TransitionTrailerKey, runSlug)
			gitkit.CommitFile(t, primeRecords, path.Join(path.Dir(statusRel), fmt.Sprintf("transition-%d.txt", i)), "x\n", message)
		}
		head := gitkit.RevParse(t, primeRecords, "HEAD")
		f.plantRunRecord(t, branch, slug, shedengine.Status{State: shedengine.StateRunning}, &f.entry)
		return f, branch, head
	}
	runPrime := func(f *handOverFixture, named string) error {
		_, err := pairteardown.PushRun(f.h.Location, f.h.Location, named, pairteardown.RunPush{LockWait: fabricengine.LockWaitUnbounded})
		return err
	}
	// originTransitions returns the subjects of the transition commits on origin's prime records branch, oldest first.
	originTransitions := func(t *testing.T, f *handOverFixture, branch string) []string {
		t.Helper()
		var subjects []string
		for _, subject := range strings.Split(gitkit.Git(t, f.h.RecordsBare, "log", "--reverse", "--format=%s", "refs/heads/"+branch), "\n") {
			if strings.HasPrefix(subject, "transition ") {
				subjects = append(subjects, subject)
			}
		}
		return subjects
	}
	wantKeptOnly := []string{"transition 0 of " + keptSlug, "transition 2 of " + keptSlug}
	requirePairGone := func(t *testing.T, f *handOverFixture) {
		t.Helper()
		for _, dir := range []string{f.h.PairCodeWorktree(slug), f.h.PairRecordsSibling(slug)} {
			if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
				t.Errorf("stat %s error = %v; want the pair gone", dir, statErr)
			}
		}
	}

	t.Run("APrimePushHandsTheTakenOverRunOverAndLandsTheOthersCommitsInOrder", func(t *testing.T) {
		f, branch, primeHead := primeAhead(t)
		captureNotices(t)

		if err := runPrime(f, ""); err != nil {
			t.Fatalf("PushRun(prime) error = %v; want nil", err)
		}

		f.requireMark(t)
		f.requireProcessEnded(t)
		requirePairGone(t, f)
		holdsPrimeTip := false
		for _, tag := range strings.Fields(gitkit.Git(t, f.h.RecordsBare, "tag", "--list", fabricengine.OwnerArchiveTagPrefix+slug+"/*")) {
			if gitkit.RevParse(t, f.h.RecordsBare, tag+"^{commit}") == primeHead {
				holdsPrimeTip = true
			}
		}
		if !holdsPrimeTip {
			t.Errorf("owner archive tags of %q on origin hold none at the prime's tip %s", slug, primeHead)
		}
		if got := originTransitions(t, f, branch); !slices.Equal(got, wantKeptOnly) {
			t.Errorf("origin transition commits = %q; want %q, none of the taken-over run's", got, wantKeptOnly)
		}
	})

	t.Run("APrimePushNamingTheTakenOverRunEndsWithErrHandedOver", func(t *testing.T) {
		f, branch, _ := primeAhead(t)
		captureNotices(t)

		err := runPrime(f, slug)

		f.requireHandedOver(t, err)
		if got := originTransitions(t, f, branch); !slices.Equal(got, wantKeptOnly) {
			t.Errorf("origin transition commits = %q; want %q, none of the taken-over run's", got, wantKeptOnly)
		}
	})

	t.Run("AFailedArchiveOwesThePassAndTheNextPushHandsTheRunOver", func(t *testing.T) {
		f, branch, _ := primeAhead(t)
		captureNotices(t)
		hook := filepath.Join(f.h.RecordsBare, "hooks", "pre-receive")
		if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
			t.Fatalf("create the hooks directory: %v", err)
		}
		refuseTags := "#!/bin/sh\nwhile read old new ref; do case \"$ref\" in refs/tags/*) exit 1;; esac; done\n"
		if err := os.WriteFile(hook, []byte(refuseTags), 0o755); err != nil {
			t.Fatalf("write the pre-receive hook: %v", err)
		}
		before := gitkit.RevParse(t, f.h.RecordsBare, "refs/heads/"+branch)

		err := runPrime(f, "")

		if !errors.Is(err, fabricengine.ErrTakeoverPassOwed) || !fabricengine.IsPushOwed(err) {
			t.Fatalf("PushRun(prime) error = %v; want one wrapping ErrTakeoverPassOwed that IsPushOwed reads owed", err)
		}
		if mark, found, markErr := fabricengine.ReadHandOverMark(f.h.Path, slug); markErr != nil || found {
			t.Errorf("ReadHandOverMark() = %+v, %v, %v; want no mark", mark, found, markErr)
		}
		if got := gitkit.RevParse(t, f.h.RecordsBare, "refs/heads/"+branch); got != before {
			t.Errorf("origin prime records tip = %s; want it unchanged at %s", got, before)
		}

		if err := os.Remove(hook); err != nil {
			t.Fatalf("remove the pre-receive hook: %v", err)
		}
		if err := runPrime(f, ""); err != nil {
			t.Fatalf("second PushRun(prime) error = %v; want nil", err)
		}
		f.requireMark(t)
		if got := originTransitions(t, f, branch); !slices.Equal(got, wantKeptOnly) {
			t.Errorf("origin transition commits = %q; want %q, none of the taken-over run's", got, wantKeptOnly)
		}
	})
}
