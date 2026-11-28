// prreview_mail_integration_test.go pins how the PR-Review row mails a refused head to its session.
// The row's wired mail seam posts to a real creel daemon, which runs in process on a fake delivery.
// The wired seams that read config, git and the run's records read a real task pair.
// The session runs over real reed on a recorded tmux, and the gate runs a failing command in that pair.

package loomcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/prkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxrec"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// reviewWakes is how many wakes a PR-Review mail row's wait runs before its context is cancelled.
const reviewWakes = 3

// refusedMail is the mail of the pr-review gate refusing the task head, whose failed run wrote log.
func refusedMail(head, log string) string {
	return fmt.Sprintf("the pr-review gate refused task head %s of pull request #7, so it is not pushed: "+
		"the pr-review gate failed at tier 1; output: %s; fix it in a new commit"+
		"; when the failure lies outside the change, such as a file another process held open, "+
		"lym loom pr-continue gates this head again, but it also ends the review once a head passes the gate, "+
		"so run it only on the operator's word", head, log)
}

// TestPRReviewGateFailureMail pins the mail of a refused head over the wakes of the PR-Review wait.
// A refusal is mailed to the session once per head, and a cleared mailed head is gated and mailed again.
// A mail creel refuses halts the row and records no mailed head.
// No row pushes the refused head, moves the record's gated head or takes a parent lock,
// and every sleep of the wait lasts from a second to a minute.
// It runs without t.Parallel: the strand and git environment variables are process-global.
func TestPRReviewGateFailureMail(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")
	// The pair's origin is a GitHub URL, so a git command that reaches for it must fail at once and ask for no credentials.
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("GIT_ASKPASS", "")

	h := hubforge.CopyHub(t, pairHubShape)
	const slug = hubforge.TemplatePairSlug
	once := hubforge.TestShortname + ":pr-once"
	again := hubforge.TestShortname + ":pr-again"
	startNoticeOrch(t, h, creelfake.LiveStrand(once), creelfake.LiveStrand(again))
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s) = %v; want nil", slug, err)
	}
	mode := []yamlengine.KV{{Key: dobbyengine.PullRequestKey, Value: dobbyengine.PullRequestReview}}
	if _, err := configengine.Set(fabricengine.ConfigDirs(pair), dobbyengine.ConfigSpec(), configengine.LayerWorktree, mode); err != nil {
		t.Fatalf("configengine.Set(beam) = %v; want nil", err)
	}
	gitkit.Git(t, pair.WorktreePath(), "remote", "set-url", "origin", "https://github.com/acme/widgets.git")
	wired := landingDeps(pair, shedrun.RecipeTapestry, websterengine.Geometry{}, slug, "main",
		false, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(pair), parentLockDeps(pair, shedrun.SelfRunID, "", ""))
	// The row reads whether the task branch holds a commit its parent branch lacks.
	head := gitkit.CommitFile(t, pair.WorktreePath(), "task.txt", "task work\n", "task: work")

	tests := []struct {
		name string
		// strand is the session strand the record names, which creel lists or not.
		strand string
		// clearAtSleep clears the record's mailed head at that 1-based sleep; 0 clears nothing.
		clearAtSleep int
		// wantGates is how many times the gate runs; each run writes the next verify log.
		wantGates int
		// wantMailed says whether creel takes each run's mail, and the record then names the task head as mailed.
		wantMailed bool
		wantHalt   bool
	}{
		{name: "ARefusalIsMailedToTheSessionOncePerHead", strand: once, wantGates: 1, wantMailed: true},
		{name: "AClearedMailedHeadIsGatedAndMailedAgain", strand: again, clearAtSleep: 1, wantGates: 2, wantMailed: true},
		{name: "AMailCreelRefusesHaltsAndRecordsNoMailedHead", strand: hubforge.TestShortname + ":pr-gone", wantGates: 1, wantHalt: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, gates, pushes := prReviewMailDeps(t, pair, wired, tt.strand)
			pr := &deps.PullRequest
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
			sleeps := 0
			pr.Now = func() time.Time { return now }
			pr.Sleep = func(ctx context.Context, d time.Duration) error {
				sleeps++
				if d < time.Second || d > time.Minute {
					t.Errorf("sleep %d lasts %s; want from a second to a minute", sleeps, d)
				}
				now = now.Add(d)
				if sleeps == tt.clearAtSleep {
					err := pullrequest.UpdateRecord(pr.RecordPath, pr.RecordLockPath, func(r *pullrequest.Record) error {
						r.MailedHead = ""
						return nil
					})
					if err != nil {
						t.Errorf("clear the mailed head: %v", err)
					}
				}
				if sleeps >= reviewWakes {
					cancel()
				}
				return ctx.Err()
			}

			outcome, pointer, err := landingshed.NewPRReview("PR-Review", deps).Call(ctx)

			var logs []string
			for n := 1; n <= tt.wantGates; n++ {
				logs = append(logs, filepath.Join(deps.VerifyDir, fmt.Sprintf("verify-%d.log", n)))
			}
			if tt.wantHalt {
				wantReason := "the pr-review gate refused task head " + head + " of pull request #7, committed in the task worktree while the review waited: " +
					"the pr-review gate failed at tier 1; output: " + logs[0] + "; fix it in a new commit; " +
					"way forward: commit a fix in the task worktree, or leave the head as it is when the failure lies outside the change, " +
					"then run lym loom resume, which gates the task head again before the row pushes it"
				if err != nil || outcome != shedengine.Stuck || !pointer.Halt || pointer.Reason != wantReason {
					t.Errorf("Call() = %q, halt %v, error %v, reason:\n%s\nwant Stuck with halt and reason:\n%s", outcome, pointer.Halt, err, pointer.Reason, wantReason)
				}
			} else if !errors.Is(err, context.Canceled) {
				t.Errorf("Call() = %q, %+v, error %v; want the wait cancelled after %d wakes", outcome, pointer, err, reviewWakes)
			}
			if *gates != tt.wantGates {
				t.Errorf("the gate ran %d times; want %d", *gates, tt.wantGates)
			}
			if *pushes != 0 {
				t.Errorf("the task branch was pushed %d times; want no push of a refused head", *pushes)
			}
			if held, err := deps.Lock.Store.List(); err != nil || len(held) != 0 {
				t.Errorf("parent locks = %+v, error %v; want none, since the wait takes no parent lock", held, err)
			}
			var wantMails []string
			wantMailedHead := ""
			if tt.wantMailed {
				for _, log := range logs {
					wantMails = append(wantMails, refusedMail(head, log))
				}
				wantMailedHead = head
			}
			var mails []string
			for _, entry := range postedEntries(t, h) {
				if entry.To == tt.strand {
					mails = append(mails, entry.Subject)
				}
			}
			if diff := cmp.Diff(wantMails, mails); diff != "" {
				t.Errorf("mails posted to %s mismatch (-want +got):\n%s", tt.strand, diff)
			}
			wantRecord := pullrequest.Record{
				Number: 7, URL: "https://github.com/acme/widgets/pull/7", Base: "fixture-parent", Head: "task-branch", GatedHead: "h1",
				StrandGUID: tmuxrec.GUIDPrefix + "1", StrandName: tt.strand, SessionID: tmuxrec.SessionID, MailedHead: wantMailedHead,
			}
			rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
			if err != nil || !found {
				t.Fatalf("ReadRecord() found %v, error %v; want the record", found, err)
			}
			if diff := cmp.Diff(wantRecord, rec); diff != "" {
				t.Errorf("record mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// prReviewMailDeps returns the deps of a PR-Review row that gates in pair's worktree.
// The row mails, and reads config, git and the run's records, through the seams and record paths of wired.
// The gate runs a command that fails; the second result counts its runs, and the third the pushes of the task branch.
// The pull request is open on GitHub, and its record names a live session on strand that has not gated the task head.
func prReviewMailDeps(t *testing.T, pair *lymcwd.Location, wired landingshed.Deps, strand string) (landingshed.Deps, *int, *int) {
	t.Helper()
	deps := envkit.LandingDeps(t, t.TempDir())
	gates, pushes := 0, 0
	deps.GateKey = "pr-review"
	deps.WorktreeRoot = pair.WorktreePath()
	deps.VerifyDir = t.TempDir()
	deps.VerifyHold = nil
	deps.VerifyCommand = func(string) (string, error) {
		gates++
		return "exit 1", nil
	}
	deps.TaskHead = wired.TaskHead
	deps.DriftVerdict = wired.DriftVerdict
	if err := os.MkdirAll(filepath.Dir(deps.DescriptionPath), 0o755); err != nil {
		t.Fatalf("create the description's directory: %v", err)
	}
	if err := os.WriteFile(deps.DescriptionPath, []byte("# Change title\n\nChange body\n"), 0o644); err != nil {
		t.Fatalf("write the change description: %v", err)
	}

	pr := &deps.PullRequest
	pr.MailAgent = wired.PullRequest.MailAgent
	pr.Mode = wired.PullRequest.Mode
	pr.Origin = wired.PullRequest.Origin
	pr.DirtyPaths = wired.PullRequest.DirtyPaths
	pr.CommitsMissing = wired.PullRequest.CommitsMissing
	pr.TaskHasCommits = wired.PullRequest.TaskHasCommits
	pr.PauseCovered = wired.PullRequest.PauseCovered
	pr.RecordPath = wired.PullRequest.RecordPath
	pr.RecordLockPath = wired.PullRequest.RecordLockPath
	pr.RecordChanges = wired.PullRequest.RecordChanges
	push := pr.PushTask
	pr.PushTask = func() error {
		pushes++
		return push()
	}
	api := &prkit.API{}
	api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateOpen}})
	pr.API = func(string) (pullrequest.API, error) { return api, nil }

	session, replay := tmuxrec.PlaySession(t, "session-start-stop")
	t.Cleanup(func() {
		if unanswered := replay.Unanswered(); len(unanswered) > 0 {
			t.Errorf("recording session-start-stop holds no answer to %q", unanswered)
		}
	})
	status := session.Reed.Status
	registerDir := filepath.Join(t.TempDir(), "runproc")
	pr.Sessions = session.Runner
	deps.EnginesRun = session.Runner.EnginesRun
	pr.SessionLive = reedengine.SessionLiveness(status)
	pr.RegisterSession = reedengine.SessionRegistrar(status, func(pid int) error {
		return runproc.Register(registerDir, "fixture", runproc.ChildRecord(pid, runproc.RoleSession, nil))
	})
	started, err := session.Runner.StartSession(tmuxrec.SessionSpec(""))
	if err != nil {
		t.Fatalf("seed the session: StartSession() error = %v", err)
	}
	rec := pullrequest.Record{
		Number: 7, URL: "https://github.com/acme/widgets/pull/7", Base: deps.ParentBranch, Head: deps.TaskBranch, GatedHead: "h1",
		StrandGUID: started.StrandGUID, StrandName: strand, SessionID: started.SessionID,
	}
	if err := pullrequest.WriteRecord(pr.RecordPath, pr.RecordLockPath, rec); err != nil {
		t.Fatalf("write the pull request record: %v", err)
	}
	return deps, &gates, &pushes
}
