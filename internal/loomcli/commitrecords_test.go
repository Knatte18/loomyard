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

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
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
	const agentRefusal = "loom: pr-continue: this call runs in the lym agent session lymhub:task:orch, and only the operator ends a pull request's review; " +
		"way forward: tell the operator the run's pull request waits for their review, which they end by approving or merging it on GitHub, " +
		"or by running lym loom pr-continue in the task worktree from a shell outside every lym session"
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
		if code := prContinueVerb(&out, "", record, lock); code != 0 || !strings.Contains(out.String(), url) {
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
		if code := prContinueVerb(&out, "", record, lock); code != 0 {
			t.Errorf("second prContinueVerb() = %d, %s; want 0", code, out.String())
		}
		after, err := os.Stat(record)
		if err != nil || !after.ModTime().Equal(before.ModTime()) {
			t.Errorf("second prContinueVerb rewrote the record (stat error %v); want it untouched", err)
		}
	})

	t.Run("ACallerWithAStrandNameIsRefusedAndWritesNothing", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		seed := pullrequest.Record{Number: 7, URL: url}
		if err := pullrequest.WriteRecord(record, lock, seed); err != nil {
			t.Fatalf("WriteRecord() error: %v", err)
		}
		var out bytes.Buffer
		if code := prContinueVerb(&out, "lymhub:task:orch", record, lock); code != 1 || !strings.Contains(out.String(), agentRefusal) {
			t.Errorf("prContinueVerb() = %d, %s; want 1 with %q", code, out.String(), agentRefusal)
		}
		got, _, err := pullrequest.ReadRecord(record, lock)
		if err != nil || !reflect.DeepEqual(got, seed) {
			t.Errorf("record after the refusal = %+v, %v; want the untouched %+v", got, err, seed)
		}
	})

	t.Run("ACallerWithAStrandNameIsRefusedBeforeTheRecordIsRead", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		var out bytes.Buffer
		if code := prContinueVerb(&out, "lymhub:task:orch", record, lock); code != 1 || !strings.Contains(out.String(), agentRefusal) {
			t.Errorf("prContinueVerb() = %d, %s; want 1 with %q", code, out.String(), agentRefusal)
		}
		if _, err := os.Stat(record); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat record after the refusal = %v; want it absent", err)
		}
	})

	t.Run("AnEndedReviewAnswersOkWithoutAWrite", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		seed := pullrequest.Record{Number: 7, URL: url, ReviewEnded: true}
		if err := pullrequest.WriteRecord(record, lock, seed); err != nil {
			t.Fatalf("WriteRecord() error: %v", err)
		}
		var out bytes.Buffer
		if code := prContinueVerb(&out, "", record, lock); code != 0 || !strings.Contains(out.String(), url) {
			t.Fatalf("prContinueVerb() = %d, %s; want 0 naming %s", code, out.String(), url)
		}
		got, _, err := pullrequest.ReadRecord(record, lock)
		if err != nil || !reflect.DeepEqual(got, seed) {
			t.Errorf("record after prContinueVerb = %+v, %v; want the untouched %+v", got, err, seed)
		}
	})

	t.Run("AbsentRecordIsRefused", func(t *testing.T) {
		t.Parallel()
		record, lock := newPaths(t)
		var out bytes.Buffer
		if code := prContinueVerb(&out, "", record, lock); code != 1 || !strings.Contains(out.String(), noRecordWayForward) {
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
		if code := prContinueVerb(&out, "", record, lock); code != 1 || !strings.Contains(out.String(), "decode failed") {
			t.Errorf("prContinueVerb() = %d, %s; want 1 carrying the decode error", code, out.String())
		}
	})
}

// TestPRContinueCmd_RefusesTheStrandItsEnvironmentNames does not call t.Parallel:
// it sets the strand name variable with t.Setenv.
func TestPRContinueCmd_RefusesTheStrandItsEnvironmentNames(t *testing.T) {
	tiergate.Require(t, 0)
	t.Setenv(agentname.StrandNameEnv, "lymhub:task:orch")
	c := &loomCLI{location: hubLocationAt(t, filepath.Join(t.TempDir(), "task"), "")}
	record, lock := shedrun.PullRequestRecord(c.location, shedrun.SelfRunID), shedrun.PullRequestRecordLock(c.location, shedrun.SelfRunID)
	seed := pullrequest.Record{Number: 7, URL: "u7"}
	if err := os.MkdirAll(filepath.Dir(record), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", filepath.Dir(record), err)
	}
	if err := pullrequest.WriteRecord(record, lock, seed); err != nil {
		t.Fatalf("WriteRecord() error: %v", err)
	}
	var out bytes.Buffer

	code := clihelp.Execute(c.prContinueCmd(), &out, []string{})

	if code != 1 || !strings.Contains(out.String(), "this call runs in the lym agent session lymhub:task:orch") {
		t.Errorf("pr-continue = %d, %s; want 1 with the agent refusal", code, out.String())
	}
	got, _, err := pullrequest.ReadRecord(record, lock)
	if err != nil || !reflect.DeepEqual(got, seed) {
		t.Errorf("record after the refusal = %+v, %v; want the untouched %+v", got, err, seed)
	}
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
