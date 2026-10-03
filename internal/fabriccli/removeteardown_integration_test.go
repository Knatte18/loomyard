//go:build integration

// removeteardown_integration_test.go covers `lyx fabric remove` going through the pairteardown composite: a slug with nothing left is a not-found error, a half-removed pair is finished with finished: true, and a pair with task-side dirt is refused without touching it.
//
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestRunCLI_RemoveNothingLeftIsNotFound(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	if code := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", "no-such-pair"}); code == 0 {
		t.Fatalf("remove of a slug with nothing left exited 0; output: %s", out.String())
	}
	envelope.RequireErr(t, out.String(), "not found")
}

func TestRunCLI_RemoveFinishesHalfRemovedPair(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	const slug = "cli-half-removed"
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairWarpWorktree(slug))

	var out bytes.Buffer
	if code := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", slug}); code != 0 {
		t.Fatalf("remove of a half-removed pair exited %d; want 0\noutput: %s", code, out.String())
	}
	env := envelope.Decode(t, out.String())
	if finished, _ := env.Raw["finished"].(bool); !finished {
		t.Errorf("finished = %v; want true\noutput: %s", env.Raw["finished"], out.String())
	}
	if _, present := env.Raw["session_ended"]; !present {
		t.Errorf("envelope has no \"session_ended\" key\noutput: %s", out.String())
	}
	if _, err := os.Stat(h.PairWeftSibling(slug)); !os.IsNotExist(err) {
		t.Errorf("sibling worktree still present after finishing the pair: %v", err)
	}
}

func TestRunCLI_RemoveTaskSideDirtRefusesWithoutTouchingThePair(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	const slug = "cli-task-dirty"
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	warpPath := h.PairWarpWorktree(slug)
	gitkit.CommitFile(t, warpPath, "tracked.md", "committed\n", "seed tracked file")
	if err := os.WriteFile(filepath.Join(warpPath, "tracked.md"), []byte("committed\nuncommitted\n"), 0o644); err != nil {
		t.Fatalf("dirty the task worktree: %v", err)
	}

	var out bytes.Buffer
	if code := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", slug}); code == 0 {
		t.Fatalf("remove of a task-side-dirty pair exited 0; output: %s", out.String())
	}
	envelope.RequireErr(t, out.String(), "")
	for _, path := range []string{warpPath, h.PairWeftSibling(slug)} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was touched by a refused remove: %v", path, err)
		}
	}
}
