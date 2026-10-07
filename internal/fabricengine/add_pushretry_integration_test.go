//go:build integration

// add_pushretry_integration_test.go proves Add survives a transiently refused weft push: the runner refuses it once with a synthetic "(failed)" rejection and then pushes for real.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

func TestAdd_RetriesATransientlyRefusedWeftPush(t *testing.T) {
	t.Parallel()

	const slug = "retry-weft-push"
	weftBranch := fabricengine.WeftBranchName(slug)
	h := hubforge.NewHub(t, ".")
	refused := false
	fabricengine.SetPushSeamForTest(h.Topology, func(args []string, cwd string) (string, error) {
		if args[len(args)-1] == weftBranch && !refused {
			refused = true
			return "", &gitexec.GitError{Args: args, Dir: cwd, ExitCode: 1, Stderr: " ! [remote rejected] " + weftBranch + " -> " + weftBranch + " (failed)\n"}
		}
		return gitexec.Run(args, cwd)
	}, func(time.Duration) {})

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	if !refused {
		t.Fatalf("the weft push was never refused")
	}
	if !gitkit.BranchExists(t, h.PrimeWorktree(), slug) || !gitkit.BranchExists(t, h.PrimeWeft(), weftBranch) {
		t.Errorf("a local branch is missing after the retried Add")
	}
	if !gitkit.BranchExists(t, h.WarpBare, slug) || !gitkit.BranchExists(t, h.WeftBare, weftBranch) {
		t.Errorf("a branch is missing on origin after the retried Add")
	}
}
