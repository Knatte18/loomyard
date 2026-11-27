// recordscommit_onerepository_integration_test.go covers records commits on a one-repository worktree's own branch.
// It covers the path-limited commit beside staged code, the empty snapshot commit and the prime's branch refusal.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeUnder writes content to the file at rel under dir, creating its directories.
func writeUnder(t *testing.T, dir, rel, content string) {
	t.Helper()

	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRecordsCommit_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.NewOneRepoHub(t, "backend")
	pairPath := hubforge.AddPairWith(t, h, "pair", fabricengine.AddOptions{SkipPush: true}).Path
	pair, err := lymcwd.NamedWorktree(h.Path, "pair")
	if err != nil {
		t.Fatalf("NamedWorktree: %v", err)
	}
	rec := fabricengine.NewMutations(h.Path)
	// The rows share the hub and run in order, since the last two change the prime.

	t.Run("CommitsOnlyRecordsAndLeavesStagedCodeStaged", func(t *testing.T) {
		writeUnder(t, pairPath, "backend/code.txt", "code\n")
		gitkit.Git(t, pairPath, "add", "backend/code.txt")
		writeUnder(t, pairPath, "backend/_lym/status.txt", "{}\n")
		headBefore := gitkit.RevParse(t, pairPath, "HEAD")

		_, committed, err := fabricengine.CommitAnchoredPathsWithTrailers(rec, pair, []string{"_lym/status.txt"}, "record status", nil, fabricengine.SyncOptions{})
		if err != nil || !committed {
			t.Fatalf("CommitAnchoredPathsWithTrailers = committed %v, %v; want a commit", committed, err)
		}

		if got := gitkit.Git(t, pairPath, "show", "--name-only", "--format=", "HEAD"); got != "backend/_lym/status.txt" {
			t.Errorf("commit changed %q; want the status file alone", got)
		}
		if got := gitkit.Git(t, pairPath, "diff", "--cached", "--name-only"); got != "backend/code.txt" {
			t.Errorf("staged paths = %q; want the code file still staged", got)
		}
		if message := gitkit.Git(t, pairPath, "log", "-1", "--format=%B"); !strings.Contains(message, fabricengine.WarpSHATrailerKey+": "+headBefore) {
			t.Errorf("commit message = %q; want a %s trailer naming %s", message, fabricengine.WarpSHATrailerKey, headBefore)
		}
	})

	t.Run("EmptyRecordsCommitWithCodeStagedLands", func(t *testing.T) {
		writeUnder(t, pairPath, "backend/code2.txt", "more code\n")
		gitkit.Git(t, pairPath, "add", "backend/code2.txt")
		headBefore := gitkit.RevParse(t, pairPath, "HEAD")
		f, err := fabricengine.Open(pair)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}

		res, err := f.Commit(nil, "baseline", []string{"snap1"}, fabricengine.SyncOptions{SkipPush: true})
		if err != nil || !res.WeftCommitted {
			t.Fatalf("Commit = %+v, %v; want an empty records commit", res, err)
		}

		if gitkit.RevParse(t, pairPath, "HEAD") == headBefore {
			t.Errorf("HEAD did not move; want the empty commit")
		}
		if message := gitkit.Git(t, pairPath, "log", "-1", "--format=%B"); !strings.Contains(message, "snap1") {
			t.Errorf("commit message = %q; want the tag", message)
		}
		if got := gitkit.Git(t, pairPath, "diff", "--cached", "--name-only"); !strings.Contains(got, "backend/code2.txt") {
			t.Errorf("staged paths = %q; want the code file still staged", got)
		}
	})

	t.Run("HeldIndexLockReleasedWithinTheBoundLetsTheCommitLand", func(t *testing.T) {
		writeUnder(t, pairPath, "backend/_lym/locked.txt", "locked\n")
		lockPath := gitkit.Git(t, pairPath, "rev-parse", "--path-format=absolute", "--git-path", "index.lock")
		if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
			t.Fatalf("hold the index lock: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(lockPath) })
		headBefore := gitkit.RevParse(t, pairPath, "HEAD")
		type commitResult struct {
			committed bool
			err       error
		}
		done := make(chan commitResult, 1)
		go func() {
			_, committed, err := fabricengine.CommitAnchoredPaths(rec, pair, []string{"_lym/locked.txt"}, "locked", fabricengine.SyncOptions{})
			done <- commitResult{committed, err}
		}()

		select {
		case early := <-done:
			t.Fatalf("CommitAnchoredPaths returned (committed %v, %v) while the index lock was held; want it to wait", early.committed, early.err)
		case <-time.After(300 * time.Millisecond):
		}
		if err := os.Remove(lockPath); err != nil {
			t.Fatalf("release the index lock: %v", err)
		}
		landed := <-done

		if landed.err != nil || !landed.committed {
			t.Fatalf("CommitAnchoredPaths = committed %v, %v; want the commit to land once the lock is released", landed.committed, landed.err)
		}
		if gitkit.RevParse(t, pairPath, "HEAD") == headBefore {
			t.Errorf("HEAD did not move; want the records commit")
		}
	})

	prime := h.PrimeWorktree()
	t.Run("PrimeOffItsBranchRefuses", func(t *testing.T) {
		recorded := gitkit.CurrentBranch(t, prime)
		gitkit.Git(t, prime, "switch", "-q", "-c", "off-branch")
		defer gitkit.Git(t, prime, "switch", "-q", recorded)
		writeUnder(t, prime, "backend/_lym/edit.txt", "edit\n")
		headBefore := gitkit.RevParse(t, prime, "HEAD")

		_, _, err := fabricengine.CommitAnchoredPaths(rec, h.Location, []string{"_lym/edit.txt"}, "edit", fabricengine.SyncOptions{})

		var refusal *fabricengine.ErrPrimeBranch
		if !errors.As(err, &refusal) || !fabricengine.IsPrimeBranchRefusal(err) {
			t.Fatalf("CommitAnchoredPaths error = %v; want a prime-branch refusal", err)
		}
		if message := err.Error(); !strings.Contains(message, recorded) || !strings.Contains(message, "off-branch") {
			t.Errorf("refusal = %q; want both branches named", message)
		}
		if _, statErr := os.Stat(filepath.Join(prime, "backend/_lym/edit.txt")); statErr != nil {
			t.Errorf("the records edit is gone from disk: %v; want it left uncommitted", statErr)
		}
		if gitkit.RevParse(t, prime, "HEAD") != headBefore {
			t.Errorf("HEAD moved; want no commit")
		}
	})

	t.Run("PrimeWithoutTheKeyRefuses", func(t *testing.T) {
		gitkit.Git(t, prime, "config", "--unset", fabricengine.PrimeBranchConfigKey)

		_, _, err := fabricengine.CommitAnchoredPaths(rec, h.Location, []string{"_lym/edit.txt"}, "edit", fabricengine.SyncOptions{})

		var refusal *fabricengine.ErrPrimeBranch
		if !errors.As(err, &refusal) {
			t.Fatalf("CommitAnchoredPaths error = %v; want a prime-branch refusal", err)
		}
		if !strings.Contains(err.Error(), fabricengine.PrimeBranchConfigKey) {
			t.Errorf("refusal = %q; want the key named", err.Error())
		}
	})

	t.Run("TwoRepositoryPrimeIsNotRefused", func(t *testing.T) {
		twoRepository := hubforge.NewHub(t, ".")
		writeUnder(t, fabricengine.RecordsWorktree(twoRepository.Location), "_lym/probe.txt", "probe\n")

		_, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(twoRepository.Path), twoRepository.Location, []string{"_lym/probe.txt"}, "probe", fabricengine.SyncOptions{})

		if err != nil || !committed {
			t.Errorf("CommitAnchoredPaths = committed %v, %v; want a commit with no %s key", committed, err, fabricengine.PrimeBranchConfigKey)
		}
	})
}
