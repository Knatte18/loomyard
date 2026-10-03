//go:build integration

// addleftover_integration_test.go covers `lyx fabric add`'s envelope when a leftover remote weft branch from a plain remove blocks the re-add:
// a pre-flight failure, so a bare error carrying neither `mutations` nor `partial`.
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

func TestRunCLI_AddLeftoverWeftIsBarePreflightError(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	const slug = "leftover-slug"
	weftBranch := fabricengine.WeftBranchName(slug)

	var addOut bytes.Buffer
	if code := fabriccli.RunCLIIn(h.PrimeWorktree(), &addOut, []string{"add", slug}); code != 0 {
		t.Fatalf("first add exit = %d; output: %s", code, addOut.String())
	}
	var rmOut bytes.Buffer
	if code := fabriccli.RunCLIIn(h.PrimeWorktree(), &rmOut, []string{"remove", slug}); code != 0 {
		t.Fatalf("remove exit = %d; output: %s", code, rmOut.String())
	}

	clone := t.TempDir()
	gitkit.MustRun(t, clone, "git", "clone", "--quiet", h.WeftBare, ".")
	gitkit.MustRun(t, clone, "git", "checkout", "--quiet", weftBranch)
	if err := os.WriteFile(filepath.Join(clone, "leftover.txt"), []byte("leftover\n"), 0o644); err != nil {
		t.Fatalf("write leftover file: %v", err)
	}
	gitkit.MustRun(t, clone, "git", "add", "leftover.txt")
	gitkit.MustRun(t, clone, "git", "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "--quiet", "-m", "leftover work")
	gitkit.MustRun(t, clone, "git", "push", "--quiet", "origin", weftBranch)

	var out bytes.Buffer
	code := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"add", slug})
	if code == 0 {
		t.Fatalf("second add exit = 0; want non-zero; output: %s", out.String())
	}
	env := envelope.RequireErr(t, out.String(), weftBranch)
	if _, present := env.Raw["mutations"]; present {
		t.Errorf("envelope carries \"mutations\"; a pre-flight failure must not")
	}
	if env.Partial != nil {
		t.Errorf("envelope carries \"partial\"; a pre-flight failure must not")
	}
}
