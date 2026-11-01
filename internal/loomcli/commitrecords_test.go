//go:build tier0

// commitrecords_test.go drives commitRecordsVerb over stub commitStatusDeps closures, spawning no git.

package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/samplerengine"
)

// stubRecordsDeps builds commitStatusDeps counting Commit and Push calls.
func stubRecordsDeps(active bool, probeErr, commitErr, pushErr error) (commitStatusDeps, *int, *int) {
	commits, pushes := 0, 0
	return commitStatusDeps{
		MergeActive: func() (bool, error) { return active, probeErr },
		Commit:      func(string) (bool, error) { commits++; return commitErr == nil, commitErr },
		Push:        func() error { pushes++; return pushErr },
	}, &commits, &pushes
}

// TestCommitRecordsVerb_Success asserts a clean tree commits then pushes once each, and a mid-merge tree commits and pushes nothing and reports a skipped reason.
func TestCommitRecordsVerb_Success(t *testing.T) {
	tests := []struct {
		name                    string
		midMerge                bool
		wantCommits, wantPushes int
		wantOutput              []string
	}{
		{"ordinary path commits then pushes", false, 1, 1, nil},
		{"mid-merge skips", true, 0, 0, []string{`"committed":false`, "mid-merge"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, commits, pushes := stubRecordsDeps(tt.midMerge, nil, nil, nil)
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

func TestCommitRecordsVerb_ErrorEnvelopes(t *testing.T) {
	boom := errors.New("boom")
	rejected := fmt.Errorf("push side at /wt: %w", gitrepo.ErrPushRejected)
	const (
		rejectionWayForward = "way forward: merge the remote branch into the local branch in the worktree the error names"
		refusedVerbClause   = "a session lym refuses the verb from reports status: FAILED and the orch runs it"
		transientWayForward = "way forward: transient, run lym fabric push or re-run lym loom commit-records; " + refusedVerbClause
		pushFailedPrefix    = "the commit landed locally but the push failed: "
	)
	gone := &fabricengine.PushFailedError{
		Record: fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/t-records", CodeWorktree: "/hub/t", Branch: "t-records", Class: fabricengine.PushFailureGone},
		Err:    fabricengine.ErrBranchGone,
	}
	rewound := &fabricengine.PushFailedError{
		Record: fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/t", Branch: "t", Class: fabricengine.PushFailureRewound, OriginHead: "abc123", Commits: []string{"def456"}},
		Err:    errors.New("push held back"),
	}
	auth := &fabricengine.PushFailedError{
		Record: fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/t-records", CodeWorktree: "/hub/t", Branch: "t-records", Class: fabricengine.PushFailureAuth},
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
		if code := prContinueVerb(&out, record, lock); code != 0 || !strings.Contains(out.String(), url) {
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
		if code := prContinueVerb(&out, record, lock); code != 0 {
			t.Errorf("second prContinueVerb() = %d, %s; want 0", code, out.String())
		}
		after, err := os.Stat(record)
		if err != nil || !after.ModTime().Equal(before.ModTime()) {
			t.Errorf("second prContinueVerb rewrote the record (stat error %v); want it untouched", err)
		}
	})

	t.Run("AbsentRecordIsRefused", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		var out bytes.Buffer
		if code := prContinueVerb(&out, record, lock); code != 1 || !strings.Contains(out.String(), noRecordWayForward) {
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
		if code := prContinueVerb(&out, record, lock); code != 1 || !strings.Contains(out.String(), "decode failed") {
			t.Errorf("prContinueVerb() = %d, %s; want 1 carrying the decode error", code, out.String())
		}
	})
}

// TestSamplerDoneVerb pins the refusals of the sampler-done verb, each with its way forward and the record left unchanged, the done flag it writes at the row's entry stamp, and the error envelope of a seam that cannot be read.
func TestSamplerDoneVerb(t *testing.T) {
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
