// publish_test.go holds the in-package fixtures the Merge-In and Finalize tests share: the recording resolver, the one Deps builder, the summary writer and the transport push error.

package landingshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// recordingResolver is the in-package fake standing in for the unexported resolver seam: it records
// whether Resolve was called and what source it was called with, and returns a scripted result/err.
type recordingResolver struct {
	called    bool
	gotSource string
	gotTip    string
	result    mergeresolve.Result
	err       error
}

func (r *recordingResolver) Resolve(ctx context.Context, source, tip string) (mergeresolve.Result, error) {
	r.called = true
	r.gotSource = source
	r.gotTip = tip
	return r.result, r.err
}

// newTestDeps is the package's one Deps builder, for Merge-In and Finalize tests alike: a base-branch list requiring a pull request, a well-formed final-summary artifact already written at DescriptionPath (Finalize's top-of-Call parse requires one), and a scratch dir under t.TempDir().
// Its parent lock seams are a temporary store and tip reads that equal a written merge record, so the lock-taking rows find the parent unmoved.
// A test needing a different variant mutates the returned value.
func newTestDeps(t *testing.T) Deps {
	t.Helper()
	summaryPath := summaryparser.Path(t.TempDir())
	writeSummary(t, summaryPath, "A landing title", "A landing body.")
	scratchDir := filepath.Join(t.TempDir(), "scratch")
	recordPath := MergeRecordPath(scratchDir)
	if err := WriteMergeRecord(recordPath, MergeRecord{ParentBranch: "main", MergedTip: testParentTip}); err != nil {
		t.Fatalf("write merge record: %v", err)
	}
	owner := parentlock.Owner{Slug: "me", Pair: "/p/me", RunID: "r1", PID: 222, StartTime: "t2"}
	return Deps{
		GateKey:         "finalize",
		WorktreeRoot:    t.TempDir(),
		TaskBranch:      "task-branch",
		ParentBranch:    "main",
		DescriptionPath: summaryPath,
		StencilsDir:     t.TempDir(),
		ScratchDir:      scratchDir,
		Config: Config{
			DeployBranches:     []string{"main"},
			Squash:             true,
			Conflict:           "sonnet",
			ConflictTimeoutMin: 30,
			EngineTrailers:     map[string]string{"claude": "Co-Authored-By: Test Author <test@example.com>"},
		},
		EnginesRun: func() ([]string, error) { return []string{"claude"}, nil },
		Lock: ParentLockDeps{
			Store:    &parentlock.Store{Dir: filepath.Join(t.TempDir(), "locks"), Alive: func(int, string) bool { return true }},
			Owner:    func() (parentlock.Owner, error) { return owner, nil },
			WaitPath: parentlock.WaitRecordPath(scratchDir),
		},
		ReadParentTips: func(bool) (fabricengine.ParentTips, error) {
			return fabricengine.ParentTips{Local: testParentTip, Upstream: testParentTip, Relation: fabricengine.ParentEqual}, nil
		},
		HoldsWork:       func(string, string) (bool, error) { return false, nil },
		TaskHead:        func() (string, error) { return "task-head", nil },
		MergeRecordPath: recordPath,
	}
}

// testParentTip is the parent tip newTestDeps's merge record names and its tip reads report.
const testParentTip = "parent-tip"

// writeSummary writes a well-formed summary artifact at path.
func writeSummary(t *testing.T, path, title, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(fmt.Sprintf("# %s\n\n%s\n", title, body)), 0o644); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
}

// transportPushErr is the error a push returns when the remote is unreachable: a *gitexec.GitError whose stderr shedtransient classifies as git-transport.
func transportPushErr() error {
	return fmt.Errorf("gitrepo: git push: %w", &gitexec.GitError{
		Args:     []string{"-c", "push.autoSetupRemote=true", "push"},
		ExitCode: 128,
		Stderr:   "fatal: unable to access '...': Failed to connect to 127.0.0.1 port 1",
	})
}
