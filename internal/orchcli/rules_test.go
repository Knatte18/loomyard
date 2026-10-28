//go:build tier0

// rules_test.go covers writeHubRules over a fake Bolt: every refusal branch, the success envelope and the file left behind.

package orchcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// fakeRulesBolt is a rulesBolt that records its calls and models Bolt's unpushed commits.
type fakeRulesBolt struct {
	rulesPath string
	calls     []string

	// pushErrs is the queue Push draws its errors from; an empty queue means success.
	pushErrs []error
	// preWriteErr fails PullThenCommitWritten before the write runs.
	preWriteErr error
	// skip, with skipDetail, makes PullThenCommitWritten skip the write.
	skip       fabricengine.BoltSkip
	skipDetail string
	// commitErr fails PullThenCommitWritten after the write ran.
	commitErr error
	// upstreamMoved makes a pull skip as diverged while commits are unpushed.
	upstreamMoved bool
	// unpushed counts the commits not yet pushed.
	unpushed int
}

// Push pops the next queued error and, on success, clears the unpushed commits.
func (f *fakeRulesBolt) Push() error {
	f.calls = append(f.calls, "push")
	if len(f.pushErrs) > 0 {
		err := f.pushErrs[0]
		f.pushErrs = f.pushErrs[1:]
		if err != nil {
			return err
		}
	}
	f.unpushed = 0
	return nil
}

// PullThenCommitWritten stands in for the pull-first, the write and the commit, which it reports only when the file's bytes changed.
func (f *fakeRulesBolt) PullThenCommitWritten(write fabricengine.BoltWrite) (fabricengine.BoltWriteResult, error) {
	f.calls = append(f.calls, "pull")
	if f.preWriteErr != nil {
		return fabricengine.BoltWriteResult{}, f.preWriteErr
	}
	if f.skip != "" {
		return fabricengine.BoltWriteResult{Skipped: f.skip, SkipDetail: f.skipDetail}, nil
	}
	if f.upstreamMoved && f.unpushed > 0 {
		return fabricengine.BoltWriteResult{Skipped: fabricengine.BoltSkipDiverged, SkipDetail: "diverged"}, nil
	}
	before, readErr := os.ReadFile(f.rulesPath)
	if _, err := write.Write(); err != nil {
		return fabricengine.BoltWriteResult{}, err
	}
	if f.commitErr != nil {
		return fabricengine.BoltWriteResult{}, f.commitErr
	}
	after, err := os.ReadFile(f.rulesPath)
	if err != nil {
		return fabricengine.BoltWriteResult{}, err
	}
	committed := readErr != nil || !bytes.Equal(before, after)
	if committed {
		f.unpushed++
	}
	return fabricengine.BoltWriteResult{Committed: committed}, nil
}

func TestWriteHubRules(t *testing.T) {
	t.Parallel()

	str := func(s string) *string { return &s }
	conflictRecord := fabricengine.FailedPush{
		Repo: "bolt", RepoPath: "/hub/_board", Branch: "main",
		Class: fabricengine.PushFailureNotFastForward, Paths: []string{"board.json"}, Commits: []string{"abc1234"},
	}
	rebaseFailed := &fabricengine.PushFailedError{Record: conflictRecord, Err: fmt.Errorf("push: %w", gitrepo.ErrPushRejected)}
	conflictWayForward := fabricengine.FailedPushWayForward(conflictRecord)
	tests := []struct {
		name string
		// prior is the rules file before the call; nil means absent.
		prior *string
		// draft is the draft file's content; nil means no draft file exists.
		draft *string
		// from is the --from value; "{abs}" stands for the draft's absolute path.
		from     string
		readOnly bool
		fake     fakeRulesBolt

		wantExit int
		// wantFields is whether the envelope carries path, committed and unpushed.
		wantFields    bool
		wantCommitted bool
		wantUnpushed  bool
		wantErr       []string
		// wantFile is the rules file afterwards; nil means absent.
		wantFile        *string
		wantCalls       []string
		wantFakePending int
	}{
		{
			name: "NewFileUnderAbsentDirectory", draft: str("rules\n"), from: "draft.md",
			wantFields: true, wantCommitted: true, wantFile: str("rules\n"),
			wantCalls: []string{"push", "pull", "push"},
		},
		{
			name: "EmptyDraftCommitsEmptyFile", draft: str(""), from: "{abs}",
			wantFields: true, wantCommitted: true, wantFile: str(""),
			wantCalls: []string{"push", "pull", "push"},
		},
		{
			name: "EmptyFromRefused", from: "", prior: str("old"),
			wantExit: 1, wantErr: []string{"needs --from"}, wantFile: str("old"),
		},
		{
			name: "MissingFromFileRefused", from: "missing.md", prior: str("old"),
			wantExit: 1, wantErr: []string{"--from", "missing.md"}, wantFile: str("old"),
		},
		{
			name: "FirstPushFailure", draft: str("new"), from: "draft.md", prior: str("old"),
			fake:     fakeRulesBolt{pushErrs: []error{errors.New("boom")}},
			wantExit: 1, wantFields: true, wantUnpushed: true, wantErr: []string{"boom", "nothing was written"},
			wantFile: str("old"), wantCalls: []string{"push"},
		},
		{
			name: "FirstPushRebaseConflict", draft: str("new"), from: "draft.md", prior: str("old"),
			fake:     fakeRulesBolt{pushErrs: []error{rebaseFailed}},
			wantExit: 1, wantFields: true, wantUnpushed: true, wantErr: []string{conflictWayForward, "then re-run lym orch rules --from draft.md"},
			wantFile: str("old"), wantCalls: []string{"push"},
		},
		{
			name: "PreWriteError", draft: str("new"), from: "draft.md", prior: str("old"),
			fake:     fakeRulesBolt{preWriteErr: errors.New("board lock held")},
			wantExit: 1, wantFields: true, wantErr: []string{"board lock held", "unchanged, nothing written"},
			wantFile: str("old"), wantCalls: []string{"push", "pull"},
		},
		{
			name: "PullSkip", draft: str("new"), from: "draft.md", prior: str("old"),
			fake:     fakeRulesBolt{skip: fabricengine.BoltSkipDirty, skipDetail: "run lym board sync first"},
			wantExit: 1, wantFields: true, wantErr: []string{"run lym board sync first"},
			wantFile: str("old"), wantCalls: []string{"push", "pull"},
		},
		{
			name: "FailedWriteKeepsPriorContent", draft: str("new"), from: "draft.md", prior: str("old"), readOnly: true,
			wantExit: 1, wantFields: true, wantErr: []string{"unchanged"},
			wantFile: str("old"), wantCalls: []string{"push", "pull"},
		},
		{
			name: "FailedCommitKeepsNewContent", draft: str("new"), from: "draft.md", prior: str("old"),
			fake:     fakeRulesBolt{commitErr: errors.New("commit failed")},
			wantExit: 1, wantFields: true, wantErr: []string{"commit failed", "lym orch rules --from draft.md"},
			wantFile: str("new"), wantCalls: []string{"push", "pull"},
		},
		{
			name: "LastPushFailure", draft: str("new"), from: "draft.md", prior: str("old"),
			fake:     fakeRulesBolt{pushErrs: []error{nil, errors.New("boom")}},
			wantExit: 1, wantFields: true, wantCommitted: true, wantUnpushed: true, wantErr: []string{"boom", "committed"},
			wantFile: str("new"), wantCalls: []string{"push", "pull", "push"}, wantFakePending: 1,
		},
		{
			name: "LastPushRebaseConflict", draft: str("new"), from: "draft.md", prior: str("old"),
			fake:     fakeRulesBolt{pushErrs: []error{nil, rebaseFailed}},
			wantExit: 1, wantFields: true, wantCommitted: true, wantUnpushed: true, wantErr: []string{conflictWayForward, "then re-run lym orch rules --from draft.md"},
			wantFile: str("new"), wantCalls: []string{"push", "pull", "push"}, wantFakePending: 1,
		},
		{
			name: "RerunAfterFailedPushCommitsNothing", draft: str("same"), from: "draft.md", prior: str("same"),
			fake:       fakeRulesBolt{unpushed: 1, upstreamMoved: true},
			wantFields: true, wantFile: str("same"), wantCalls: []string{"push", "pull", "push"},
		},
	}
	for _, tt := range tests {
		// A read-only directory does not stop a write on Windows, so Windows generates no subtest for a read-only row.
		if tt.readOnly && runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			rulesPath := filepath.Join(dir, "board", rulesRel())
			if tt.prior != nil {
				if err := os.MkdirAll(filepath.Dir(rulesPath), 0o755); err != nil {
					t.Fatalf("mkdir rules dir: %v", err)
				}
				if err := os.WriteFile(rulesPath, []byte(*tt.prior), 0o644); err != nil {
					t.Fatalf("seed rules file: %v", err)
				}
			}
			draftPath := filepath.Join(dir, "draft.md")
			if tt.draft != nil {
				if err := os.WriteFile(draftPath, []byte(*tt.draft), 0o644); err != nil {
					t.Fatalf("write draft: %v", err)
				}
			}
			if tt.readOnly {
				if os.Geteuid() == 0 {
					t.Skip("a read-only directory does not stop a write as root")
				}
				if err := os.Chmod(filepath.Dir(rulesPath), 0o500); err != nil {
					t.Fatalf("chmod rules dir: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(filepath.Dir(rulesPath), 0o755) })
			}
			from := strings.ReplaceAll(tt.from, "{abs}", draftPath)
			fake := tt.fake
			fake.rulesPath = rulesPath

			var out bytes.Buffer
			code := writeHubRules(&out, dir, from, rulesPath, &fake)

			if code != tt.wantExit {
				t.Fatalf("writeHubRules() exit = %d; want %d; output: %s", code, tt.wantExit, out.String())
			}
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("envelope %q is not JSON: %v", out.String(), err)
			}
			if envelope["ok"] != (tt.wantExit == 0) {
				t.Errorf("envelope ok = %v; want %v", envelope["ok"], tt.wantExit == 0)
			}
			for _, want := range tt.wantErr {
				if got, _ := envelope["error"].(string); !strings.Contains(got, want) {
					t.Errorf("envelope error = %q; want it to contain %q", got, want)
				}
			}
			if got, _ := envelope["error"].(string); strings.Contains(got, "git pull --rebase") {
				t.Errorf("envelope error = %q; want no `git pull --rebase`, which no push path runs", got)
			}
			if tt.wantFields {
				if envelope["path"] != rulesPath || envelope["committed"] != tt.wantCommitted || envelope["unpushed"] != tt.wantUnpushed {
					t.Errorf("envelope fields = %v; want path %q, committed %v, unpushed %v", envelope, rulesPath, tt.wantCommitted, tt.wantUnpushed)
				}
			} else if _, has := envelope["unpushed"]; has {
				t.Errorf("envelope %v carries unpushed; want none before the draft is read", envelope)
			}
			got, err := os.ReadFile(rulesPath)
			switch {
			case tt.wantFile == nil && err == nil:
				t.Errorf("rules file exists with %q; want it absent", got)
			case tt.wantFile != nil && (err != nil || string(got) != *tt.wantFile):
				t.Errorf("rules file = %q (err %v); want %q", got, err, *tt.wantFile)
			}
			if !slices.Equal(fake.calls, tt.wantCalls) {
				t.Errorf("Bolt calls = %v; want %v", fake.calls, tt.wantCalls)
			}
			if fake.unpushed != tt.wantFakePending {
				t.Errorf("unpushed commits = %d; want %d", fake.unpushed, tt.wantFakePending)
			}
		})
	}
}
