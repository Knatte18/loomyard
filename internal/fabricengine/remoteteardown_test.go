// remoteteardown_test.go covers the teardown child's seams that need no real hub:
// RunTeardownChild's hold of the teardown lock and its answer to the parent, and the notice of a failed remote step.

package fabricengine

import (
	"bufio"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordNotices installs a push notifier that records every notice it is given until the test ends.
// It returns a reader of the recorded notices.
// It changes process-global state, so the calling test must not be parallel.
func recordNotices(t *testing.T) (notices func() []PushNotice) {
	t.Helper()
	var got []PushNotice
	restore := InstallPushNotifier(func(hubPath string, notice PushNotice) (bool, error) {
		got = append(got, notice)
		return true, nil
	})
	t.Cleanup(restore)
	return func() []PushNotice { return slices.Clone(got) }
}

// TestNoticeRemoteTeardown covers the log line and the one creel notice of a failed remote step.
// Each carries the slug, the operation and the way forward.
// A job reports every failed step in one notice, and a kept branch or a clean outcome mails nothing.
// The rows install the process's push notifier, so neither the test nor its rows are parallel.
func TestNoticeRemoteTeardown(t *testing.T) {
	tiergate.Require(t, 0)
	const slug = "pair"
	worktree := t.TempDir()
	weftTip := strings.Repeat("a", 40)
	taskTip := strings.Repeat("b", 40)
	weftTagPush := "push " + originRemoteName + " refs/tags/" + ArchiveTagName(slug, weftTip)
	taskTagPush := "push " + originRemoteName + " refs/tags/" + ArchiveTagName(slug, taskTip)
	removeAgain := "`lym fabric remove " + slug + " --remote`"
	cleanupAgain := "`lym fabric cleanup --apply --remote`"
	discardAgain := "run the command that discarded the task branch again"
	removeJob := remoteTeardownJob{Kind: remoteTeardownRemove, Slug: slug, Worktree: worktree}
	cleanupJob := remoteTeardownJob{Kind: remoteTeardownCleanup, Slug: slug, Worktree: worktree}
	discardJob := remoteTeardownJob{Kind: remoteTeardownDiscard, Slug: slug, Worktree: worktree}
	withWeftTip := func(job remoteTeardownJob) remoteTeardownJob {
		job.WeftTip = weftTip
		return job
	}
	archiveFailure := remoteOutcome{err: errors.New("tag push refused")}
	weftDeleteFailure := remoteOutcome{weft: CleanupBranchEntry{RemoteError: "lease lost"}}
	taskDeleteFailure := remoteOutcome{taskKept: "ls-remote refused", taskFailed: true}

	tests := []struct {
		name string
		job  remoteTeardownJob
		out  remoteOutcome
		// weftRoot is the records repository root the notice may name; empty means it could not be found.
		weftRoot string
		// wantInLine are the texts the one notice holds; none means no notice at all.
		wantInLine []string
		wantAbsent []string
	}{
		{"RemoveArchiveFailureWithWeftTip", withWeftTip(removeJob), archiveFailure, "/weft", []string{slug, "the archive of its records", "tag push refused", "git -C /weft " + weftTagPush, "when origin still holds a branch of the pair", removeAgain}, nil},
		{"RemoveArchiveFailureWithWeftTipAndNoWeftRoot", withWeftTip(removeJob), archiveFailure, "", []string{slug, "the archive of its records", "tag push refused", "when origin still holds a branch of the pair", removeAgain}, []string{"refs/tags/", "git -C"}},
		{"RemoveArchiveFailureWithoutWeftTip", removeJob, archiveFailure, "/weft", []string{slug, "the archive of its records", "tag push refused", removeAgain}, []string{"refs/tags/"}},
		{"CleanupArchiveFailureWithWeftTip", withWeftTip(cleanupJob), archiveFailure, "/weft", []string{slug, weftTagPush, cleanupAgain}, []string{"lym fabric remove"}},
		{"RecordsRepositoryLookupFailure", withWeftTip(removeJob), remoteOutcome{err: errors.New("resolve weft repo root: no records repository"), weftRootErr: errors.New("resolve weft repo root: no records repository")}, "", []string{slug, "the lookup of its records repository", "no records repository", removeAgain}, []string{"the archive of its records", "refs/tags/"}},
		{"DiscardFailureWithTaskTip", func() remoteTeardownJob { job := discardJob; job.TaskTip = taskTip; return job }(), archiveFailure, "", []string{slug, "the archive and origin delete of its task branch", taskTagPush, discardAgain}, nil},
		{"DiscardFailureWithoutTaskTip", discardJob, archiveFailure, "", []string{slug, "the archive and origin delete of its task branch", discardAgain}, []string{"refs/tags/"}},
		{"WeftDeleteFailure", removeJob, weftDeleteFailure, "", []string{slug, "the delete of its records branch on origin", "lease lost", removeAgain}, nil},
		{"CleanupWeftDeleteFailure", cleanupJob, weftDeleteFailure, "", []string{slug, "lease lost", cleanupAgain}, []string{"lym fabric remove"}},
		{"TaskBranchDeleteFailure", removeJob, taskDeleteFailure, "", []string{slug, "the delete of its task branch on origin", "ls-remote refused", removeAgain}, nil},
		{"WeftAndTaskDeleteFailuresShareOneNotice", removeJob, remoteOutcome{weft: weftDeleteFailure.weft, taskKept: taskDeleteFailure.taskKept, taskFailed: true}, "",
			[]string{"the delete of its records branch on origin", "lease lost", "the delete of its task branch on origin", "ls-remote refused"}, nil},
		{"TaskBranchKeptIsNoFailure", removeJob, remoteOutcome{taskKept: "3 commit(s) not landed"}, "", nil, nil},
		{"WeftBranchKeptIsNoFailure", removeJob, remoteOutcome{weft: CleanupBranchEntry{RemoteKeptReason: "moved since the archive"}}, "", nil, nil},
		{"CleanOutcome", removeJob, remoteOutcome{}, "", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notices := recordNotices(t)

			noticeRemoteTeardown("hub", tt.job, tt.out, tt.weftRoot)

			got := notices()
			if len(tt.wantInLine) == 0 {
				if len(got) != 0 {
					t.Errorf("noticeRemoteTeardown() posted %q; want no notice", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("noticeRemoteTeardown() posted %d notices %q; want exactly one", len(got), got)
			}
			if got[0].Topic != "remote-teardown/"+slug || got[0].Run != "" {
				t.Errorf("notice topic %q run %q; want topic %q and no run", got[0].Topic, got[0].Run, "remote-teardown/"+slug)
			}
			for _, want := range tt.wantInLine {
				if !strings.Contains(got[0].Line, want) {
					t.Errorf("notice = %q; want it to hold %q", got[0].Line, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got[0].Line, absent) {
					t.Errorf("notice = %q; want it free of %q", got[0].Line, absent)
				}
			}
		})
	}

	t.Run("NoNotifierInstalled", func(t *testing.T) {
		previous := installedPushNotifier.Swap(nil)
		t.Cleanup(func() { installedPushNotifier.Store(previous) })

		noticeRemoteTeardown("hub", removeJob, archiveFailure, "")
	})
}

// TestSyncOptionsNotifier covers which notifier a push's options mail through:
// its own Notifier when set, otherwise the process's installed one.
// The rows install the process's push notifier, so neither the test nor its rows are parallel.
func TestSyncOptionsNotifier(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name          string
		setOwn        bool
		wantInstalled int
		wantOwn       int
	}{
		{"NilNotifierFallsBackToTheInstalledOne", false, 1, 0},
		{"OwnNotifierShadowsTheInstalledOne", true, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installed := recordNotices(t)
			var own []string
			options := SyncOptions{}
			if tt.setOwn {
				options.Notifier = func(hubPath string, notice PushNotice) (bool, error) {
					own = append(own, notice.Line)
					return true, nil
				}
			}

			notify := options.notifier()
			if notify == nil {
				t.Fatalf("SyncOptions.notifier() = nil; want a notifier")
			}
			if _, err := notify("hub", PushNotice{Line: "line"}); err != nil {
				t.Fatalf("notifier error = %v; want nil", err)
			}

			if got := len(installed()); got != tt.wantInstalled {
				t.Errorf("installed notifier mails = %d; want %d", got, tt.wantInstalled)
			}
			if len(own) != tt.wantOwn {
				t.Errorf("own notifier mails = %d; want %d", len(own), tt.wantOwn)
			}
		})
	}
}

// TestNoticeRemoteTeardownEnded covers the notice of a teardown that ended before its last step:
// it carries the slug, the reason and a way forward naming the command of every job kind.
// The rows install the process's push notifier, so neither the test nor its rows are parallel.
func TestNoticeRemoteTeardownEnded(t *testing.T) {
	tiergate.Require(t, 0)
	const slug = "pair"
	tests := []struct {
		name   string
		reason string
	}{
		{"EndedBySignal", "the child was ended by the signal terminated"},
		{"JobNotHandedOver", "its job could not be handed to the child: broken pipe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notices := recordNotices(t)

			NoticeRemoteTeardownEnded("hub", slug, tt.reason)

			got := notices()
			if len(got) != 1 {
				t.Fatalf("NoticeRemoteTeardownEnded() posted %d notices %q; want exactly one", len(got), got)
			}
			for _, want := range []string{slug, "ended before its last step", tt.reason, "`lym fabric remove " + slug + " --remote`", "`lym fabric cleanup --apply --remote`", "discarded the task branch"} {
				if !strings.Contains(got[0].Line, want) {
					t.Errorf("notice = %q; want it to hold %q", got[0].Line, want)
				}
			}
		})
	}
}

// TestRunTeardownChild_NoticesAnUnreadableJob covers a job the child cannot decode.
// The remote steps never run, the error is returned, and one notice names the slug and the way forward.
// It installs the process's push notifier, so it is not parallel.
func TestRunTeardownChild_NoticesAnUnreadableJob(t *testing.T) {
	tiergate.Require(t, 0)
	const slug = "pair"
	notices := recordNotices(t)

	var signal strings.Builder
	if err := RunTeardownChild(t.TempDir(), slug, false, time.Second, strings.NewReader("{not json"), &signal); err == nil {
		t.Fatalf("RunTeardownChild() error = nil; want the decode failure")
	}

	got := notices()
	if len(got) != 1 {
		t.Fatalf("notices = %q; want exactly one", got)
	}
	for _, want := range []string{slug, "reading its job", "way forward", "`lym fabric remove " + slug + " --remote`", "`lym fabric cleanup --apply --remote`", "discarded the task branch"} {
		if !strings.Contains(got[0].Line, want) {
			t.Errorf("notice = %q; want it to hold %q", got[0].Line, want)
		}
	}
}

// TestRunTeardownChild_HoldsTheLockUntilItsInputCloses covers the handover's lock.
// The child answers held and keeps the lock exclusive while the parent still writes.
// It releases the lock once the parent closes the input with no job.
func TestRunTeardownChild_HoldsTheLockUntilItsInputCloses(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const slug = "pair"
	hub := t.TempDir()
	jobReader, jobWriter := io.Pipe()
	signalReader, signalWriter := io.Pipe()

	done := make(chan error, 1)
	go func() {
		err := RunTeardownChild(hub, slug, false, time.Second, jobReader, signalWriter)
		_ = signalWriter.Close()
		done <- err
	}()

	answer, err := bufio.NewReader(signalReader).ReadString('\n')
	if err != nil || answer != teardownHeldSignal+"\n" {
		t.Fatalf("child answer = (%q, %v); want %q", answer, err, teardownHeldSignal)
	}
	if held, acquired, err := lock.TryAcquireReadLock(TeardownLockPath(hub, slug)); err != nil || acquired {
		if acquired {
			_ = held.Release()
		}
		t.Errorf("TryAcquireReadLock while the child holds the lock = (%v, %v); want (false, nil)", acquired, err)
	}

	_ = jobWriter.Close()
	if err := <-done; err != nil {
		t.Fatalf("RunTeardownChild() error = %v; want nil", err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(TeardownLockPath(hub, slug))
	if err != nil || !acquired {
		t.Fatalf("TryAcquireWriteLock after the child ended = (%v, %v); want (true, nil)", acquired, err)
	}
	_ = held.Release()
}

// TestRunTeardownChild_AnswersBusyOnAHeldLock covers a child that meets the lock already held.
// It answers busy, reads no job, and leaves the holder's lock alone.
func TestRunTeardownChild_AnswersBusyOnAHeldLock(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		try  bool
	}{
		{"OneAttempt", true},
		{"BoundedWait", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			const slug = "pair"
			hub := t.TempDir()
			if err := ensureTeardownLockDir(hub, slug); err != nil {
				t.Fatalf("ensureTeardownLockDir() error = %v; want nil", err)
			}
			held, acquired, err := lock.TryAcquireWriteLock(TeardownLockPath(hub, slug))
			if err != nil || !acquired {
				t.Fatalf("TryAcquireWriteLock() = (%v, %v); want (true, nil)", acquired, err)
			}
			t.Cleanup(func() { _ = held.Release() })

			var signal strings.Builder
			if err := RunTeardownChild(hub, slug, tt.try, 50*time.Millisecond, strings.NewReader(`{"kind":"remove"}`), &signal); err != nil {
				t.Fatalf("RunTeardownChild() error = %v; want nil", err)
			}
			if got := signal.String(); got != teardownBusySignal+"\n" {
				t.Errorf("child answer = %q; want %q", got, teardownBusySignal+"\n")
			}
		})
	}
}

// TestTeardownChildArgs covers the child's command line.
// The verb, the slug and the wait are always passed, and the try flag only when set.
func TestTeardownChildArgs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		try  bool
		want []string
	}{
		{"Wait", false, []string{"fabric", TeardownChildVerb, "--slug", "s", "--wait", "2s"}},
		{"Try", true, []string{"fabric", TeardownChildVerb, "--slug", "s", "--wait", "2s", "--try"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := teardownChildArgs("s", tt.try, 2*time.Second); !slices.Equal(got, tt.want) {
				t.Errorf("teardownChildArgs(s, %v, 2s) = %q; want %q", tt.try, got, tt.want)
			}
		})
	}
}
