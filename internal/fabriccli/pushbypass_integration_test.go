//go:build integration

// pushbypass_integration_test.go covers the two-sided --warp-path/--weft-path
// bypass push wired in card 5: RunCLI(&out, []string{"--warp-path", w,
// "--weft-path", f, "push"}) exercises exactly the synchronous bypass handler
// code the detached push child spawned by fabricengine.SpawnDetachedPush runs
// — a real forked child cannot itself be observed from a test binary (it would
// re-exec the test binary), so this synchronous seam is the deterministic proof
// that a supplied path is pushed. A --warp-path-only, non-push-verb case mirrors
// cli_test.go's TestRunCLI_WeftPathPushOnly for the existing --weft-path flag.

package fabriccli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// headSHA returns dir's HEAD commit SHA.
func headSHA(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// branchSHA returns branch's commit SHA as resolved in dir, distinct from headSHA because a real
// hub's weft bare carries more than one branch (the primary pair's own "main-weft" alongside
// weft:main's "main"), so the bare's own HEAD symref does not necessarily name the branch a caller
// means to check.
func branchSHA(t *testing.T, dir, branch string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "refs/heads/"+branch)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse refs/heads/%s in %s: %v", branch, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// TestRunCLI_BypassPushAdvancesBothUpstreams builds weft and warp repos with unpushed commits, then
// asserts that --warp-path/--weft-path bypass push exits 0 and both bare upstreams' HEAD matches
// their local checkout's HEAD.
func TestRunCLI_BypassPushAdvancesBothUpstreams(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	// Add one more commit on top of the weft side's already-pushed history, so the weft side has
	// something genuinely unpushed to push. This package's subject is push bypass, so the remote must
	// be a live push target: a real hub's warp and weft both have their own copied bare origin, the
	// same live-push substrate the old weft-only fixture used to provide, so nothing about the push
	// path needs compensating.
	placeholderFile := filepath.Join(h.PrimeWeft(), lyxdirs.LyxDirName, "placeholder")
	if err := os.WriteFile(placeholderFile, []byte("bypass push test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", ".")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "-q", "-m", "weft bypass push")

	// The prime pair's weft branch, so the bare-side assertion below checks that branch specifically
	// rather than the bare's own HEAD symref -- a real hub's weft bare also carries weft:main's own
	// "main" branch (the board checkout), and the bare's default HEAD may not name the prime pair's
	// branch.
	weftBranch := strings.TrimSpace(gitOutputCLI(t, h.PrimeWeft(), "rev-parse", "--abbrev-ref", "HEAD"))
	wantWeftSHA := headSHA(t, h.PrimeWeft())
	wantWarpSHA := headSHA(t, h.PrimeWorktree())

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{
		"--warp-path", h.PrimeWorktree(),
		"--weft-path", h.PrimeWeft(),
		"push",
	})
	if exitCode != 0 {
		t.Fatalf("RunCLI bypass push = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("RunCLI bypass push ok = %v; want true. Error: %v", result["ok"], result["error"])
	}

	if gotWeftSHA := branchSHA(t, h.WeftBare, weftBranch); gotWeftSHA != wantWeftSHA {
		t.Errorf("weft bare %s = %s; want %s (the unpushed commit was not pushed)", weftBranch, gotWeftSHA, wantWeftSHA)
	}
	if gotWarpSHA := headSHA(t, h.WarpBare); gotWarpSHA != wantWarpSHA {
		t.Errorf("warp bare HEAD = %s; want %s (the unpushed commit was not pushed)", gotWarpSHA, wantWarpSHA)
	}
}

// TestRunCLI_WarpPathPushOnly verifies that --warp-path with a non-push subcommand returns exit 1
// and a "subcommand requires a worktree context" error.
func TestRunCLI_WarpPathPushOnly(t *testing.T) {
	tmpDir := t.TempDir()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"--warp-path", tmpDir, "status"})

	if exitCode != 1 {
		t.Errorf("RunCLI --warp-path with non-push returned %d; want 1", exitCode)
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); ok {
		t.Errorf("ok should be false for error; got true")
	}
	if errMsg, ok := result["error"].(string); ok {
		if errMsg != "subcommand requires a worktree context" {
			t.Errorf("error message = %q; want %q", errMsg, "subcommand requires a worktree context")
		}
	} else {
		t.Errorf("error field missing or not a string")
	}
}
