// commitrecords_test.go drives commitRecordsVerb over stub commitStatusDeps closures, spawning no git.

package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// stubRecordsDeps builds commitStatusDeps from stub outcomes and returns pointers to its Commit and Push counts.
func stubRecordsDeps(active bool, probeErr, commitErr, pushErr error) (commitStatusDeps, *int, *int) {
	commits, pushes := 0, 0
	return commitStatusDeps{
		MergeActive: func() (bool, error) { return active, probeErr },
		Commit:      func(string) (bool, error) { commits++; return commitErr == nil, commitErr },
		Push:        func() error { pushes++; return pushErr },
	}, &commits, &pushes
}

// TestCommitRecordsVerb_Success asserts that a clean tree commits then pushes once each.
// A mid-merge tree commits and pushes nothing and reports a skipped reason.
// A push skipped under the pair's teardown lock is an ok envelope with push_skipped.
func TestCommitRecordsVerb_Success(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name                    string
		midMerge                bool
		wantCommits, wantPushes int
		wantOutput              []string
		pushErr                 error
	}{
		{"ordinary path commits then pushes", false, 1, 1, nil, nil},
		{"mid-merge skips", true, 0, 0, []string{`"committed":false`, "mid-merge"}, nil},
		{"push skipped under the teardown lock", false, 1, 1, []string{`"committed":true`, `"push_skipped"`}, fmt.Errorf("push: %w", fabricengine.ErrPairPushRunning)},
		{"push stopped by the pre-push check", false, 1, 1, []string{`"committed":true`, `"push_stopped"`, "owned by machine"}, fmt.Errorf("%w: the run is owned by machine neve", errPushStopped)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, commits, pushes := stubRecordsDeps(tt.midMerge, nil, nil, tt.pushErr)
			var out bytes.Buffer
			if code := commitRecordsVerb(&out, deps); code != 0 {
				t.Fatalf("exit = %d; want 0; output %s", code, out.String())
			}
			if *commits != tt.wantCommits || *pushes != tt.wantPushes {
				t.Errorf("Commit/Push calls = %d/%d; want %d/%d", *commits, *pushes, tt.wantCommits, tt.wantPushes)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output = %s; want it to contain %q", out.String(), want)
				}
			}
		})
	}
}

// TestCommitRecordsVerb_StoppedCommitPushesNothing asserts a commit the pre-push check stopped is an ok envelope:
// the envelope has committed false and push_stopped, and no push follows it.
func TestCommitRecordsVerb_StoppedCommitPushesNothing(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	deps, commits, pushes := stubRecordsDeps(false, nil, fmt.Errorf("%w: the run is owned by machine neve", errPushStopped), nil)
	var out bytes.Buffer

	code := commitRecordsVerb(&out, deps)

	if code != 0 {
		t.Fatalf("exit = %d; want 0; output %s", code, out.String())
	}
	for _, want := range []string{`"committed":false`, `"push_stopped"`, "owned by machine neve"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output = %s; want it to contain %q", out.String(), want)
		}
	}
	if *commits != 1 || *pushes != 0 {
		t.Errorf("Commit/Push calls = %d/%d; want 1/0", *commits, *pushes)
	}
}

// TestNewCommitStatusSeam_StoppedCommitStartsNoPush asserts a commit the pre-push check stopped:
// the status transition reports no hard error and starts no push.
func TestNewCommitStatusSeam_StoppedCommitStartsNoPush(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	pushes := 0
	seam := newCommitStatusSeam(commitStatusDeps{
		MergeActive: func() (bool, error) { return false, nil },
		Commit: func(string) (bool, error) {
			return false, fmt.Errorf("%w: the run is owned by machine neve", errPushStopped)
		},
		Push: func() error { pushes++; return nil },
	})

	if err := seam("Code-Review", "running"); err != nil {
		t.Errorf("seam() = %v; want nil", err)
	}
	if pushes != 0 {
		t.Errorf("Push calls = %d; want 0", pushes)
	}
}

func TestCommitRecordsVerb_ErrorEnvelopes(t *testing.T) {
	tiergate.Require(t, 0)
	boom := errors.New("boom")
	rejected := fmt.Errorf("push side at /wt: %w", gitrepo.ErrPushRejected)
	const (
		rejectionWayForward = "way forward: merge the remote branch into the local branch in the worktree the error names"
		refusedVerbClause   = "a session lym refuses the verb from reports status: FAILED and the orch runs it"
		transientWayForward = "way forward: transient, run lym fabric push or re-run lym loom commit-records; " + refusedVerbClause
		pushFailedPrefix    = "the commit landed locally but the push failed: "
	)
	gone := &fabricengine.PushFailedError{
		Record: fabricengine.FailedPush{Repo: fabricengine.RepoRecords, RepoPath: "/hub/t-records", CodeWorktree: "/hub/t", Branch: "t-records", Class: fabricengine.PushFailureGone},
		Err:    fabricengine.ErrBranchGone,
	}
	rewound := &fabricengine.PushFailedError{
		Record: fabricengine.FailedPush{Repo: fabricengine.RepoCode, RepoPath: "/hub/t", Branch: "t", Class: fabricengine.PushFailureRewound, OriginHead: "abc123", Commits: []string{"def456"}},
		Err:    errors.New("push held back"),
	}
	auth := &fabricengine.PushFailedError{
		Record: fabricengine.FailedPush{Repo: fabricengine.RepoRecords, RepoPath: "/hub/t-records", CodeWorktree: "/hub/t", Branch: "t-records", Class: fabricengine.PushFailureAuth},
		Err:    gitrepo.ErrPushAuthRefused,
	}
	goneLine := "way forward: " + fabricengine.FailedPushWayForward(gone.Record)
	rewoundLine := "way forward: " + fabricengine.FailedPushWayForward(rewound.Record)
	authLine := "way forward: " + fabricengine.FailedPushWayForward(auth.Record)
	tests := []struct {
		name                       string
		probeErr, commitErr, pushE error
		want                       []string
		// once lists texts the output holds exactly once.
		once []string
	}{
		{"probe", boom, nil, nil, []string{"probe merge state"}, nil},
		{"commit", nil, boom, nil, []string{"commit failed"}, nil},
		{"push handed the run over", nil, nil, fmt.Errorf("hand over: %w: machine %q holds the run", fabricengine.ErrHandedOver, "oslo"), []string{"handed over to another machine", `machine \"oslo\" holds the run`, "way forward: drive the run on the machine that took it over"}, nil},
		{"push rejected", nil, nil, rejected, []string{pushFailedPrefix, rejectionWayForward, refusedVerbClause}, nil},
		{"push failed", nil, nil, boom, []string{pushFailedPrefix, transientWayForward}, nil},
		{"push lock busy", nil, nil, fmt.Errorf("push: %w", fabricengine.ErrPushLockBusy), []string{pushFailedPrefix, transientWayForward}, nil},
		{"rejected beside another failure", nil, nil, errors.Join(rejected, boom), []string{pushFailedPrefix, rejectionWayForward, "boom", "push rejected"}, nil},
		{"push gone", nil, nil, gone, []string{pushFailedPrefix}, []string{goneLine}},
		{"push rewound", nil, nil, rewound, []string{pushFailedPrefix}, []string{rewoundLine}},
		{"push auth", nil, nil, auth, []string{pushFailedPrefix}, []string{authLine}},
		{"both sides classified", nil, nil, errors.Join(gone, auth), []string{pushFailedPrefix}, []string{goneLine, authLine}},
		{"classified beside an unclassified failure", nil, nil, errors.Join(gone, boom), []string{pushFailedPrefix, "boom"}, []string{goneLine, "way forward: transient, run lym fabric push or re-run lym loom commit-records"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _, _ := stubRecordsDeps(false, tt.probeErr, tt.commitErr, tt.pushE)
			var out bytes.Buffer
			if code := commitRecordsVerb(&out, deps); code == 0 {
				t.Fatalf("exit = 0; want non-zero; output %s", out.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output = %s; want it to contain %q", out.String(), want)
				}
			}
			for _, want := range tt.once {
				if got := strings.Count(out.String(), want); got != 1 {
					t.Errorf("output = %s; want %q exactly once, got %d", out.String(), want, got)
				}
			}
			if strings.Contains(out.String(), "not pushed") {
				t.Errorf("output = %s; want no claim that the records were not pushed", out.String())
			}
			if !strings.Contains(out.String(), "way forward: ") {
				t.Errorf("output = %s; want a way forward", out.String())
			}
			// The failure was transient: the re-run the message names succeeds.
			deps, _, _ = stubRecordsDeps(false, nil, nil, nil)
			out.Reset()
			if code := commitRecordsVerb(&out, deps); code != 0 {
				t.Errorf("re-run exit = %d; want 0; output %s", code, out.String())
			}
		})
	}
}

func TestPRContinueVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const url = "https://github.com/o/r/pull/7"
	const noRecordWayForward = "way forward: wait until the PR-Review row has opened the pull request, then run lym loom pr-continue again"
	newPaths := func(t *testing.T) (record, lock string) {
		t.Helper()
		dir := t.TempDir()
		return filepath.Join(dir, "pull-request.json"), filepath.Join(dir, "pull-request.json.lock")
	}

	t.Run("SetsContinueAndNamesTheURL", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		if err := pullrequest.WriteRecord(record, lock, pullrequest.Record{Number: 7, URL: url}); err != nil {
			t.Fatalf("WriteRecord() error: %v", err)
		}
		var out bytes.Buffer
		if code := prContinueVerb(&out, record, lock, prContinueReview{}); code != 0 || !strings.Contains(out.String(), url) {
			t.Fatalf("prContinueVerb() = %d, %s; want 0 naming %s", code, out.String(), url)
		}
		got, _, err := pullrequest.ReadRecord(record, lock)
		if err != nil || !got.Continue || got.Number != 7 {
			t.Errorf("record after prContinueVerb = %+v, %v; want Continue set on pull request 7", got, err)
		}

		before, err := os.Stat(record)
		if err != nil {
			t.Fatalf("stat record: %v", err)
		}
		out.Reset()
		if code := prContinueVerb(&out, record, lock, prContinueReview{}); code != 0 {
			t.Errorf("second prContinueVerb() = %d, %s; want 0", code, out.String())
		}
		after, err := os.Stat(record)
		if err != nil || !after.ModTime().Equal(before.ModTime()) {
			t.Errorf("second prContinueVerb rewrote the record (stat error %v); want it untouched", err)
		}
	})

	t.Run("ARerunAfterAMailedGateFailureClearsTheMailedHead", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		if err := pullrequest.WriteRecord(record, lock, pullrequest.Record{Number: 7, URL: url, GatedHead: "h1", MailedHead: "h2", Continue: true}); err != nil {
			t.Fatalf("WriteRecord() error: %v", err)
		}
		var out bytes.Buffer
		if code := prContinueVerb(&out, record, lock, prContinueReview{}); code != 0 {
			t.Fatalf("prContinueVerb() = %d, %s; want 0", code, out.String())
		}
		got, _, err := pullrequest.ReadRecord(record, lock)
		want := pullrequest.Record{Number: 7, URL: url, GatedHead: "h1", Continue: true}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("record after prContinueVerb = %+v, %v; want %+v, the mailed head cleared", got, err, want)
		}
	})

	t.Run("AbsentRecordIsRefused", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		var out bytes.Buffer
		if code := prContinueVerb(&out, record, lock, prContinueReview{}); code != 1 || !strings.Contains(out.String(), noRecordWayForward) {
			t.Errorf("prContinueVerb() = %d, %s; want 1 with the way forward %q", code, out.String(), noRecordWayForward)
		}
	})

	t.Run("UndecodableRecordIsAnError", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		if err := os.WriteFile(record, []byte("not json"), 0o644); err != nil {
			t.Fatalf("write record: %v", err)
		}
		var out bytes.Buffer
		if code := prContinueVerb(&out, record, lock, prContinueReview{}); code != 1 || !strings.Contains(out.String(), "decode failed") {
			t.Errorf("prContinueVerb() = %d, %s; want 1 carrying the decode error", code, out.String())
		}
	})

	autonomous := true
	const head = "h2"
	tests := []struct {
		name         string
		standing     landingshed.PRReviewStanding
		mailed       string
		closed       bool
		wantCode     int
		wantOutput   []string
		wantAbsent   []string
		wantContinue bool
		wantMailed   string
	}{
		{name: "NoSatisfiedAnswerAndNoMailedHeadIsRefusedNamingAsk", standing: landingshed.PRReviewAskAgain, wantCode: 1,
			wantOutput: []string{"lym loom parent-review ask"}},
		{name: "AnAwaitingAskIsRefusedNamingAwait", standing: landingshed.PRReviewAwaiting, wantCode: 1,
			wantOutput: []string{"lym loom parent-review await"}},
		{name: "ASatisfiedAnswerAtAnOlderHeadIsRefused", standing: landingshed.PRReviewAskAgain, wantCode: 1,
			wantOutput: []string{"no satisfied answer from your parent stands at " + head}},
		{name: "ASatisfiedAnswerAtTheCurrentHeadSetsContinue", standing: landingshed.PRReviewSatisfied, mailed: "h1", wantCode: 0,
			wantOutput: []string{`"continue":true`}, wantContinue: true},
		{name: "AMailedHeadWithNoSatisfiedAnswerIsClearedAndTheReviewStaysOpen", standing: landingshed.PRReviewAskAgain, mailed: "h1", wantCode: 0,
			wantOutput: []string{`"continue":false`, "the review stays open"}},
		{name: "AClosedPullRequestIsRefusedNamingTheRowsHalt", standing: landingshed.PRReviewSatisfied, closed: true, wantCode: 1,
			wantOutput: []string{"the PR-Review row halts on its next wake", "your parent settles that halt"},
			wantAbsent: []string{"lym loom resume", "lym batten cancel"}},
	}
	for _, tt := range tests {
		t.Run("Autonomy_"+tt.name, func(t *testing.T) {
			t.Parallel()
			record, lock := newPaths(t)
			if err := pullrequest.WriteRecord(record, lock, pullrequest.Record{Number: 7, URL: url, Autonomy: &autonomous, MailedHead: tt.mailed}); err != nil {
				t.Fatalf("WriteRecord() error: %v", err)
			}
			review := prContinueReview{
				head:     func() (string, error) { return head, nil },
				standing: func(string) (landingshed.PRReviewStanding, error) { return tt.standing, nil },
				closed:   func(pullrequest.Record) (bool, error) { return tt.closed, nil },
			}
			var out bytes.Buffer
			if code := prContinueVerb(&out, record, lock, review); code != tt.wantCode {
				t.Errorf("prContinueVerb() = %d, %s; want %d", code, out.String(), tt.wantCode)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("prContinueVerb() output %s; want it to contain %q", out.String(), want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(out.String(), absent) {
					t.Errorf("prContinueVerb() output %s; want it to omit %q", out.String(), absent)
				}
			}
			got, _, err := pullrequest.ReadRecord(record, lock)
			if err != nil || got.Continue != tt.wantContinue || got.MailedHead != tt.wantMailed && tt.wantCode == 0 {
				t.Errorf("record after prContinueVerb = %+v, %v; want Continue %v and no mailed head", got, err, tt.wantContinue)
			}
		})
	}

	t.Run("AnAnswerTheParentRecordsEndsTheReview", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		if err := pullrequest.WriteRecord(record, lock, pullrequest.Record{Number: 7, URL: url, Autonomy: &autonomous}); err != nil {
			t.Fatalf("WriteRecord() error: %v", err)
		}
		now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
		files := burlerengine.ParentReviewFilesOf(t.TempDir())
		earlier := burlerengine.ParentAsk{Seed: "code-review", Tag: "[code-review request]", State: burlerengine.ParentAskTimeout}
		if _, err := burlerengine.RecordParentAsk(files, earlier); err != nil {
			t.Fatalf("RecordParentAsk() error: %v", err)
		}
		ask := burlerengine.ParentAsk{Seed: landingshed.PRReviewAskSeed, Tag: landingshed.PRReviewAskLineTag, Digest: head, State: burlerengine.ParentAskOpen, AskedAt: now, Deadline: now.Add(time.Hour)}
		recorded, err := burlerengine.RecordParentAsk(files, ask)
		if err != nil {
			t.Fatalf("RecordParentAsk() error: %v", err)
		}
		if _, err := burlerengine.AnswerParentAsk(files, recorded.Number, now, nil); err != nil {
			t.Fatalf("AnswerParentAsk() error: %v", err)
		}
		review := prContinueReview{
			head: func() (string, error) { return head, nil },
			standing: func(head string) (landingshed.PRReviewStanding, error) {
				return landingshed.ReadPRReviewStanding(files, head, now)
			},
			closed: func(pullrequest.Record) (bool, error) { return false, nil },
		}
		var out bytes.Buffer
		if code := prContinueVerb(&out, record, lock, review); code != 0 {
			t.Fatalf("prContinueVerb() = %d, %s; want 0", code, out.String())
		}
		if got, _, err := pullrequest.ReadRecord(record, lock); err != nil || !got.Continue {
			t.Errorf("record after prContinueVerb = %+v, %v; want Continue set", got, err)
		}
	})
}

// TestSamplerDoneVerb pins the refusals of the sampler-done verb.
// Each refusal names its way forward and leaves the record unchanged.
// The verb writes the done flag at the row's entry stamp, and a seam that cannot be read yields the error envelope.
func TestSamplerDoneVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const row = "Sampler-Session"
	readFailure := errors.New("seam unavailable")
	withStrand := &samplerengine.Record{StrandGUID: "guid", StrandName: "hub:slug:sampler"}
	tests := []struct {
		name          string
		hasRow        bool
		current       string
		record        *samplerengine.Record
		corruptRecord bool
		dirty         []string
		stamp         int
		rowErr        error
		currentErr    error
		stampErr      error
		dirtyErr      error
		repeat        bool
		wantCode      int
		wantOutput    []string
		wantDoneStamp int
	}{
		{name: "a recipe with no session row is refused", hasRow: false, current: "Describe", record: withStrand, wantCode: 1,
			wantOutput: []string{"has no Sampler-Session row", "way forward: none is needed"}},
		{name: "a run at another row is refused naming it", hasRow: true, current: "Describe", record: withStrand, wantCode: 1,
			wantOutput: []string{"current row is Describe", "way forward: run lym loom sampler-done while the run stands at Sampler-Session"}},
		{name: "a run with no status file is refused", hasRow: true, current: "", record: withStrand, wantCode: 1,
			wantOutput: []string{"current row is none", "way forward"}},
		{name: "an absent record is refused", hasRow: true, current: row, record: nil, wantCode: 1,
			wantOutput: []string{"way forward: wait until the Sampler-Session row has started its session"}},
		{name: "a record with no strand is refused", hasRow: true, current: row, record: &samplerengine.Record{DoneStamp: 0}, wantCode: 1,
			wantOutput: []string{"way forward: wait until the Sampler-Session row has started its session", "lym loom resume", "lym batten cancel"}},
		{name: "a dirty worktree is refused naming the path", hasRow: true, current: row, record: withStrand, dirty: []string{"notes/draft.md"}, stamp: 3, wantCode: 1,
			wantOutput: []string{"notes/draft.md", "way forward: commit the work"}},
		{name: "a clean worktree writes the done flag at the entry stamp", hasRow: true, current: row, record: withStrand, stamp: 3, wantCode: 0,
			wantOutput: []string{`"done":true`, `"stamp":3`}, wantDoneStamp: 3},
		{name: "a repeat call writes the same stamp again", hasRow: true, current: row, record: withStrand, stamp: 3, repeat: true, wantCode: 0,
			wantOutput: []string{`"stamp":3`}, wantDoneStamp: 3},
		{name: "a row read failure is an error envelope", hasRow: true, current: row, record: withStrand, rowErr: readFailure, wantCode: 1,
			wantOutput: []string{"seam unavailable", "run lym loom sampler-done again"}},
		{name: "a current row read failure is an error envelope", hasRow: true, current: row, record: withStrand, currentErr: readFailure, wantCode: 1,
			wantOutput: []string{"seam unavailable", "run lym loom sampler-done again"}},
		{name: "an undecodable record is an error envelope", hasRow: true, current: row, corruptRecord: true, wantCode: 1,
			wantOutput: []string{"read the sampler record", "run lym loom sampler-done again"}},
		{name: "a stamp read failure is an error envelope", hasRow: true, current: row, record: withStrand, stampErr: readFailure, wantCode: 1,
			wantOutput: []string{"seam unavailable", "run lym loom sampler-done again"}},
		{name: "a dirty paths read failure is an error envelope", hasRow: true, current: row, record: withStrand, dirtyErr: readFailure, wantCode: 1,
			wantOutput: []string{"seam unavailable", "run lym loom sampler-done again"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			recordPath, lockPath := filepath.Join(dir, "sampler.json"), filepath.Join(dir, "sampler.json.lock")
			switch {
			case tt.corruptRecord:
				if err := os.WriteFile(recordPath, []byte("not json"), 0o644); err != nil {
					t.Fatalf("write record: %v", err)
				}
			case tt.record != nil:
				seed := *tt.record
				if err := samplerengine.UpdateRecord(recordPath, lockPath, func(stored *samplerengine.Record) { *stored = seed }); err != nil {
					t.Fatalf("seed the record: %v", err)
				}
			}
			deps := samplerDoneDeps{
				row:            func() (string, bool, error) { return row, tt.hasRow, tt.rowErr },
				currentRow:     func() (string, error) { return tt.current, tt.currentErr },
				recordPath:     recordPath,
				recordLockPath: lockPath,
				stamp:          func() (int, error) { return tt.stamp, tt.stampErr },
				dirty:          func() ([]string, error) { return tt.dirty, tt.dirtyErr },
			}

			var out bytes.Buffer
			code := samplerDoneVerb(&out, deps)
			if tt.repeat {
				out.Reset()
				code = samplerDoneVerb(&out, deps)
			}
			if code != tt.wantCode {
				t.Errorf("samplerDoneVerb() = %d, %s; want exit %d", code, out.String(), tt.wantCode)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("samplerDoneVerb() output = %s; want it to contain %q", out.String(), want)
				}
			}
			if tt.corruptRecord {
				return
			}
			got, found, err := samplerengine.ReadRecord(recordPath, lockPath)
			if err != nil || found != (tt.record != nil) {
				t.Fatalf("ReadRecord() = %+v, %v, %v; want found %v, nil", got, found, err, tt.record != nil)
			}
			if got.DoneStamp != tt.wantDoneStamp {
				t.Errorf("record DoneStamp = %d; want %d", got.DoneStamp, tt.wantDoneStamp)
			}
		})
	}
}
