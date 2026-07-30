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
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// headSHA returns dir's (or a bare repo's) HEAD commit SHA.
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

// TestRunCLI_BypassPushAdvancesBothUpstreams builds a weft repo with an
// unpushed commit (lyxtest.CopyWeft, whose fixture ships with upstream
// tracking already established, plus one more commit made on top) and a warp
// repo whose lyxtest.CopyHostHub fixture already carries an unpushed "init"
// commit against an empty bare origin — reused directly per the batch's
// decision, since PushCoalesced's -c push.autoSetupRemote=true makes the
// first push against a no-upstream checkout succeed and establish tracking.
// It asserts RunCLI's --warp-path/--weft-path bypass push exits 0 and that
// both bare upstreams' HEAD now matches their local checkout's HEAD.
func TestRunCLI_BypassPushAdvancesBothUpstreams(t *testing.T) {
	weftFixture := lyxtest.CopyWeft(t)
	warpFixture := lyxtest.CopyHostHub(t)

	// Add one more commit on top of the weft fixture's already-pushed "init"
	// commit, so the weft side has something genuinely unpushed to push.
	configPath := filepath.Join(weftFixture.WeftPath, "_lyx", "config.yaml")
	if err := os.WriteFile(configPath, []byte("bypass push test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "add", ".")
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "commit", "-q", "-m", "weft bypass push")

	wantWeftSHA := headSHA(t, weftFixture.WeftPath)
	wantWarpSHA := headSHA(t, warpFixture.Hub)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{
		"--warp-path", warpFixture.Hub,
		"--weft-path", weftFixture.WeftPath,
		"push",
	})
	if exitCode != 0 {
		t.Fatalf("RunCLI bypass push = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("RunCLI bypass push ok = %v; want true. Error: %v", result["ok"], result["error"])
	}

	if gotWeftSHA := headSHA(t, weftFixture.Bare); gotWeftSHA != wantWeftSHA {
		t.Errorf("weft bare HEAD = %s; want %s (the unpushed commit was not pushed)", gotWeftSHA, wantWeftSHA)
	}
	if gotWarpSHA := headSHA(t, warpFixture.Bare); gotWarpSHA != wantWarpSHA {
		t.Errorf("warp bare HEAD = %s; want %s (the unpushed commit was not pushed)", gotWarpSHA, wantWarpSHA)
	}
}

// TestRunCLI_WarpPathPushOnly verifies that --warp-path with a non-push
// subcommand returns exit 1 and the JSON error envelope
// {"ok":false,"error":"subcommand requires a worktree context"}, mirroring
// cli_test.go's TestRunCLI_WeftPathPushOnly for the sibling --weft-path flag.
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
