//go:build integration

// recoverbatch_git_test.go exercises recover-batch's suspect-path evidence over a real scratch git repository:
// the flagged blob a failed batch recorded is looked up by git in the recovery report's head, at the start commit and in the worktree, so these tests need git's own object and ignore semantics.
// Every other RecoverBatch behavior is tested over a fakeGit in recoverbatch_test.go.

package websterengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// suspectScenario is the one scratch repository the suspect-evidence steps share.
// Its base commit holds base.txt alone.
type suspectScenario struct {
	repo string
	base string
}

func newSuspectScenario(t *testing.T) *suspectScenario {
	t.Helper()
	repo := newScratchRepo(t)
	base := gitkit.CommitFile(t, repo, "base.txt", "base", "base commit")
	return &suspectScenario{repo: repo, base: base}
}

// restart puts the repository back on its base commit with a clean tree, and returns a recover fixture over it.
// Its Git is nil, so webster asks the repository itself, and Head reads the repository's HEAD.
func (s *suspectScenario) restart(t *testing.T) *recoverFixture {
	t.Helper()
	gitkit.Git(t, s.repo, "reset", "--hard", s.base)
	gitkit.Git(t, s.repo, "clean", "-fdx")
	fx := newRecoverFixtureOver(t, s.repo, nil)
	fx.Head = func(t *testing.T) string { return gitkit.RevParse(t, s.repo, "HEAD") }
	return fx
}

// suspectRecovery seeds a failed batch 1 whose Master parent-write flagged internal/x.go holding "forged".
// internal/x.go is committed as "orig" (the batch's start commit) and the failed record carries the flagged blob.
// It returns the fixture, the start commit, and the flagged blob.
func (s *suspectScenario) suspectRecovery(t *testing.T) (*recoverFixture, string, string) {
	t.Helper()
	fx := s.restart(t)
	start := gitkit.CommitFile(t, fx.Worktree, "internal/x.go", "orig", "orig")
	if err := os.WriteFile(filepath.Join(fx.Worktree, "internal", "x.go"), []byte("forged"), 0o644); err != nil {
		t.Fatal(err)
	}
	blob := strings.TrimSpace(gitkit.Git(t, fx.Worktree, "hash-object", "internal/x.go"))
	gitkit.Git(t, fx.Worktree, "checkout", "--", "internal/x.go")
	fx.Deps.State.Batches[1] = &websterengine.BatchState{
		Slug: "json-flag", Kind: "fork", Terminal: true, Status: websterengine.DigestStatusFailed, StartSHA: start,
		SuspectPaths: []websterengine.SuspectPath{{Path: "internal/x.go", Blob: blob}},
	}
	return fx, start, blob
}

func recoverSuspect(t *testing.T, fx *recoverFixture) (*recoverDriveResult, error) {
	t.Helper()
	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	recoverAtReportHead(t, fx, clk)
	return driveRecoverBatch(fx.Deps, 1, time.Second, clk)
}

// TestRecoverBatch_SuspectEvidence runs every suspect-evidence case over one scratch repository, one step per case.
// Each step starts by restarting the repository from its base commit, so no step relies on another's state.
func TestRecoverBatch_SuspectEvidence(t *testing.T) {
	t.Parallel()
	s := newSuspectScenario(t)

	t.Run("fails when the suspect content survives", func(t *testing.T) {
		fx, start, _ := s.suspectRecovery(t)
		clk := &recoverFakeClock{now: time.Unix(0, 0)}
		if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
			t.Fatal(err)
		}
		gitkit.CommitFile(t, fx.Worktree, "internal/x.go", "forged", "strand keeps forged")
		head := gitkit.RevParse(t, fx.Worktree, "HEAD")
		writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
		_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
		if !errors.Is(err, websterengine.ErrBatchFailed) {
			t.Fatalf("error = %v; want ErrBatchFailed", err)
		}
		if !strings.Contains(err.Error(), "internal/x.go") || !strings.Contains(err.Error(), "revert it to "+start) {
			t.Errorf("error = %q; want the path and the revert instruction", err.Error())
		}
		bs := fx.Deps.State.Batches[1]
		if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed || len(bs.SuspectPaths) != 1 || bs.SuspectPaths[0].Blob == "" {
			t.Errorf("record = %+v; want terminal failed with SuspectPaths kept", bs)
		}
	})

	// A re-failed recovery keeps the blob the audit first flagged:
	// a strand that deletes the committed flagged file without committing the delete fails, and a later strand that restores it is failed again.
	t.Run("a re-failed recovery keeps the flagged blob", func(t *testing.T) {
		fx, start, blob := s.suspectRecovery(t)
		clk := &recoverFakeClock{now: time.Unix(0, 0)}
		if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
			t.Fatal(err)
		}
		gitkit.CommitFile(t, fx.Worktree, "internal/x.go", "forged", "strand keeps forged")
		if err := os.Remove(filepath.Join(fx.Worktree, "internal", "x.go")); err != nil {
			t.Fatal(err)
		}
		head := gitkit.RevParse(t, fx.Worktree, "HEAD")
		writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
		if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); !errors.Is(err, websterengine.ErrBatchFailed) {
			t.Fatalf("first recovery error = %v; want ErrBatchFailed", err)
		}
		if got := fx.Deps.State.Batches[1].SuspectPaths; len(got) != 1 || got[0].Blob != blob {
			t.Fatalf("SuspectPaths after re-fail = %+v; want the flagged blob %s kept", got, blob)
		}

		if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, fx.Worktree, "checkout", "--", "internal/x.go")
		gitkit.CommitFile(t, fx.Worktree, "other.txt", "o", "other work")
		head = gitkit.RevParse(t, fx.Worktree, "HEAD")
		writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
		_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
		if !errors.Is(err, websterengine.ErrBatchFailed) || !strings.Contains(err.Error(), "revert it to "+start) {
			t.Fatalf("second recovery error = %v; want ErrBatchFailed naming the revert to %s", err, start)
		}
	})

	t.Run("fails on an uncommitted suspect path", func(t *testing.T) {
		fx, _, _ := s.suspectRecovery(t)
		if err := os.WriteFile(filepath.Join(fx.Worktree, "internal", "x.go"), []byte("forged"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitkit.CommitFile(t, fx.Worktree, "other.txt", "o", "other work")
		head := gitkit.RevParse(t, fx.Worktree, "HEAD")
		_, err := recoverSuspect(t, fx)
		if !errors.Is(err, websterengine.ErrBatchFailed) {
			t.Fatalf("error = %v; want ErrBatchFailed", err)
		}
		if !strings.Contains(err.Error(), "internal/x.go") || !strings.Contains(err.Error(), head) {
			t.Errorf("error = %q; want the path and the head %s", err.Error(), head)
		}
	})

	t.Run("passes when the suspect path is reverted", func(t *testing.T) {
		fx, _, _ := s.suspectRecovery(t)
		gitkit.CommitFile(t, fx.Worktree, "other.txt", "o", "other work")
		result, err := recoverSuspect(t, fx)
		if err != nil {
			t.Fatalf("error = %v; want nil", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("Digest = %+v; want done", result.Digest)
		}
	})

	t.Run("passes when the suspect path is re-derived", func(t *testing.T) {
		fx, _, _ := s.suspectRecovery(t)
		gitkit.CommitFile(t, fx.Worktree, "internal/x.go", "derived", "strand re-derives")
		result, err := recoverSuspect(t, fx)
		if err != nil {
			t.Fatalf("error = %v; want nil", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("Digest = %+v; want done", result.Digest)
		}
	})

	t.Run("fails when the suspect content moved", func(t *testing.T) {
		fx, _, _ := s.suspectRecovery(t)
		clk := &recoverFakeClock{now: time.Unix(0, 0)}
		if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
			t.Fatal(err)
		}
		gitkit.CommitFile(t, fx.Worktree, "internal/x.go", "forged", "strand keeps forged")
		gitkit.Git(t, fx.Worktree, "mv", "internal/x.go", "internal/y.go")
		gitkit.Git(t, fx.Worktree, "commit", "-m", "strand moves forged")
		head := gitkit.RevParse(t, fx.Worktree, "HEAD")
		writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
		_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
		if !errors.Is(err, websterengine.ErrBatchFailed) {
			t.Fatalf("error = %v; want ErrBatchFailed", err)
		}
		if !strings.Contains(err.Error(), "internal/x.go") || !strings.Contains(err.Error(), "internal/y.go") {
			t.Errorf("error = %q; want both the suspect path and the moved path", err.Error())
		}
	})

	// The blob search still runs when the strand moves the flagged file and leaves an ignored file at the suspect path.
	t.Run("fails when the suspect path is untracked and the content moved", func(t *testing.T) {
		fx, _, _ := s.suspectRecovery(t)
		clk := &recoverFakeClock{now: time.Unix(0, 0)}
		if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
			t.Fatal(err)
		}
		gitkit.CommitFile(t, fx.Worktree, "internal/x.go", "forged", "strand keeps forged")
		gitkit.Git(t, fx.Worktree, "mv", "internal/x.go", "internal/y.go")
		if err := os.WriteFile(filepath.Join(fx.Worktree, ".gitignore"), []byte("internal/x.go\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, fx.Worktree, "add", ".gitignore")
		gitkit.Git(t, fx.Worktree, "commit", "-m", "strand moves forged and ignores x")
		if err := os.WriteFile(filepath.Join(fx.Worktree, "internal", "x.go"), []byte("ignored"), 0o644); err != nil {
			t.Fatal(err)
		}
		head := gitkit.RevParse(t, fx.Worktree, "HEAD")
		writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
		_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
		if !errors.Is(err, websterengine.ErrBatchFailed) {
			t.Fatalf("error = %v; want ErrBatchFailed", err)
		}
		if !strings.Contains(err.Error(), "internal/y.go") {
			t.Errorf("error = %q; want the moved path named", err.Error())
		}
	})

	t.Run("passes when the start commit held the same content elsewhere", func(t *testing.T) {
		fx := s.restart(t)
		gitkit.CommitFile(t, fx.Worktree, "internal/z.go", "forged", "z holds forged")
		start := gitkit.CommitFile(t, fx.Worktree, "internal/x.go", "orig", "orig")
		blob := strings.TrimSpace(gitkit.Git(t, fx.Worktree, "rev-parse", start+":internal/z.go"))
		fx.Deps.State.Batches[1] = &websterengine.BatchState{
			Slug: "json-flag", Kind: "fork", Terminal: true, Status: websterengine.DigestStatusFailed, StartSHA: start,
			SuspectPaths: []websterengine.SuspectPath{{Path: "internal/x.go", Blob: blob}},
		}
		gitkit.CommitFile(t, fx.Worktree, "other.txt", "o", "other work")
		result, err := recoverSuspect(t, fx)
		if err != nil {
			t.Fatalf("error = %v; want nil", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("Digest = %+v; want done", result.Digest)
		}
	})

	// The recovery record keeps the failed record's suspect paths and its fork transcripts,
	// so the run-exit audit still maps the original fork to its own batch report.
	t.Run("the recovery record carries the suspect paths and transcripts", func(t *testing.T) {
		fx, _, blob := s.suspectRecovery(t)
		fx.Deps.State.Batches[1].ForkTranscripts = []string{"subagents/f1.jsonl"}
		clk := &recoverFakeClock{now: time.Unix(0, 0)}
		if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
			t.Fatal(err)
		}
		bs := fx.Deps.State.Batches[1]
		if bs.Kind != "recovery" || len(bs.SuspectPaths) != 1 || bs.SuspectPaths[0].Path != "internal/x.go" || bs.SuspectPaths[0].Blob != blob {
			t.Errorf("recovery record = %+v; want SuspectPaths carried", bs)
		}
		if !slices.Equal(bs.ForkTranscripts, []string{"subagents/f1.jsonl"}) {
			t.Errorf("recovery record ForkTranscripts = %v; want the failed record's transcripts carried", bs.ForkTranscripts)
		}
	})
}

// TestRecoverBatch_UncommittedPathsPrompt spawns a recovery over a real dirty worktree and asserts the prompt groups each uncommitted path by whether some session of the run wrote it.
// The run-written paths are a file and a file inside a directory git collapses; the foreign path no session wrote.
func TestRecoverBatch_UncommittedPathsPrompt(t *testing.T) {
	t.Parallel()
	s := newSuspectScenario(t)
	fx := s.restart(t)
	for path, content := range map[string]string{"mine.txt": "m", "newdir/inside.go": "i", "foreign.txt": "f"} {
		writeWorktreeFile(t, fx.Worktree, path, content)
	}
	fx.Deps.State.MasterSessionID = "s1"
	fx.Deps.State.Batches[1] = &websterengine.BatchState{Slug: "json-flag", Kind: "fork", Terminal: true, Status: websterengine.DigestStatusFailed, StartSHA: s.base}
	fx.Engine.AuditForksFn = func(string, string) (shuttleengine.ForkAudit, error) {
		return shuttleengine.ForkAudit{ParentWriteEvents: []shuttleengine.WriteEvent{
			{Path: filepath.Join(fx.Worktree, "mine.txt"), Succeeded: true},
			{Path: filepath.Join(fx.Worktree, "newdir", "inside.go"), Succeeded: true},
			{Path: filepath.Join(fx.Worktree, "foreign.txt")},
		}}, nil
	}

	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
		t.Fatal(err)
	}
	want := "Written by this run:\n- mine.txt\n- newdir/\n\nNot written by this run:\n- foreign.txt"
	if !strings.Contains(fx.Engine.LastPrompt, "What the worktree holds\n\n"+want+"\n") {
		t.Errorf("recovery prompt does not hold the grouped uncommitted paths %q; got:\n%s", want, fx.Engine.LastPrompt)
	}
}
