// commitrecords_test.go drives commitRecordsVerb over stub commitStatusDeps closures, spawning no git.

package loomcli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// stubRecordsDeps builds commitStatusDeps from stub outcomes and returns pointers to its Commit and StartPush counts.
// StartPush reads pusherStarts.
func stubRecordsDeps(active bool, probeErr, commitErr error, pusherStarts bool) (commitStatusDeps, *int, *int) {
	commits, starts := 0, 0
	return commitStatusDeps{
		MergeActive: func() (bool, error) { return active, probeErr },
		Commit:      func(string) (bool, error) { commits++; return commitErr == nil, commitErr },
		StartPush:   func() bool { starts++; return pusherStarts },
	}, &commits, &starts
}

// TestCommitRecordsVerb_Success asserts that a clean tree commits then starts the pusher once each,
// and that the envelope carries the value StartPush returned.
// A mid-merge tree commits and starts nothing and reports a skipped reason.
func TestCommitRecordsVerb_Success(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name                    string
		midMerge                bool
		pusherStarts            bool
		wantCommits, wantStarts int
		wantOutput              []string
	}{
		{"commit then a pusher started", false, true, 1, 1, []string{`"committed":true`, `"pusher_started":true`}},
		{"commit then no pusher started", false, false, 1, 1, []string{`"committed":true`, `"pusher_started":false`}},
		{"mid-merge skips", true, true, 0, 0, []string{`"committed":false`, "mid-merge"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, commits, starts := stubRecordsDeps(tt.midMerge, nil, nil, tt.pusherStarts)
			var out bytes.Buffer
			if code := commitRecordsVerb(&out, deps); code != 0 {
				t.Fatalf("exit = %d; want 0; output %s", code, out.String())
			}
			if *commits != tt.wantCommits || *starts != tt.wantStarts {
				t.Errorf("Commit/StartPush calls = %d/%d; want %d/%d", *commits, *starts, tt.wantCommits, tt.wantStarts)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output = %s; want it to contain %q", out.String(), want)
				}
			}
		})
	}
}

func TestCommitRecordsVerb_ErrorEnvelopes(t *testing.T) {
	tiergate.Require(t, 0)
	boom := errors.New("boom")
	tests := []struct {
		name                string
		probeErr, commitErr error
		want                string
	}{
		{"probe", boom, nil, "probe merge state"},
		{"commit", nil, boom, "commit failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _, starts := stubRecordsDeps(false, tt.probeErr, tt.commitErr, true)
			var out bytes.Buffer
			if code := commitRecordsVerb(&out, deps); code == 0 {
				t.Fatalf("exit = 0; want non-zero; output %s", out.String())
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("output = %s; want it to contain %q", out.String(), tt.want)
			}
			if !strings.Contains(out.String(), "way forward: ") {
				t.Errorf("output = %s; want a way forward", out.String())
			}
			if *starts != 0 {
				t.Errorf("StartPush calls = %d; want none after a failure", *starts)
			}
			// The failure was transient: the re-run the message names succeeds.
			deps, _, _ = stubRecordsDeps(false, nil, nil, true)
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
		if _, err := burlerengine.AnswerParentAskOrUnreached(files, recorded.Number, "", now, nil); err != nil {
			t.Fatalf("AnswerParentAskOrUnreached() error: %v", err)
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
