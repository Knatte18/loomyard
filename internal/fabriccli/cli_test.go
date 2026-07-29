//go:build integration

// cli_test.go covers the fabric CLI cobra surface: no-arg listing of all 14 verbs,
// unknown-subcommand cobra error, the --weft-path push-only gate, pairs with a
// minimal topology fixture, commit --help's fixed-message/Warp-SHA-trailer prose,
// and the WEFT_SKIP_PUSH env-to-SyncOptions mapping on push — this package
// exercises both the topology and content-sync verb families against the one
// fabric command tree.

package fabriccli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// setupCLIRepo creates a hub via lyxtest.CopyHostHub, changes into it, and writes a
// _lyx/config/fabric.yaml config so RunCLI can resolve topology-verb configuration
// from the cwd. Returns the hub path. Stays serial (no t.Parallel) because t.Chdir
// is required for RunCLI.
func setupCLIRepo(t *testing.T) string {
	t.Helper()
	f := lyxtest.CopyHostHub(t)
	t.Chdir(f.Hub)

	if err := os.MkdirAll(hubgeometry.ConfigDir(f.Hub), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(hubgeometry.ConfigFile(f.Hub, "fabric"), []byte("branch_prefix: wt-\npathspec: _lyx\n"), 0o644); err != nil {
		t.Fatalf("write fabric.yaml: %v", err)
	}
	return f.Hub
}

// decodeResult parses RunCLI's JSON output into a generic map.
func decodeResult(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON output: %v\noutput: %s", err, buf.String())
	}
	return result
}

// TestRunCLI_NoArgs verifies that "lyx fabric" with no subcommand prints the
// subcommand listing naming all 14 verbs — no git repo is needed, since the bare
// group command is excluded from weft-verb PersistentPreRunE resolution.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{})

	if exitCode != 0 {
		t.Errorf("RunCLI() = %d; want 0 for no-arg listing", exitCode)
	}

	got := out.String()
	wantVerbs := []string{
		"clone", "add", "list", "remove", "checkout",
		"pairs", "reconcile", "prune", "cleanup",
		"status", "commit", "push", "pull", "sync",
	}
	for _, verb := range wantVerbs {
		if !strings.Contains(got, verb) {
			t.Errorf("RunCLI() no-arg output missing verb %q; got:\n%s", verb, got)
		}
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1 and
// emits a JSON error envelope with ok=false.
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	// A temp dir is sufficient: "fabric" is not in weftVerbNames, so the
	// PersistentPreRunE guard returns nil early, bypassing all resolution.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"unknown"})

	if exitCode != 1 {
		t.Errorf("RunCLI with unknown subcommand returned %d; want 1", exitCode)
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); ok {
		t.Errorf("RunCLI(unknown) ok = true; want false")
	}
	if errMsg, _ := result["error"].(string); !strings.Contains(errMsg, "unknown") {
		t.Errorf("RunCLI(unknown) error = %q; want \"unknown\" substring", errMsg)
	}
}

// TestRunCLI_WeftPathPushOnly verifies that --weft-path with a non-push subcommand
// returns exit 1 and the JSON error envelope {"ok":false,"error":"subcommand requires
// a worktree context"}.
func TestRunCLI_WeftPathPushOnly(t *testing.T) {
	tmpDir := t.TempDir()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"--weft-path", tmpDir, "status"})

	if exitCode != 1 {
		t.Errorf("RunCLI --weft-path with non-push returned %d; want 1", exitCode)
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

// TestRunCLI_PairsReturnsPairsKey verifies that "fabric pairs" resolves the
// topology config from cwd and emits ok=true with a "pairs" key.
func TestRunCLI_PairsReturnsPairsKey(t *testing.T) {
	setupCLIRepo(t)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"pairs"})
	if exitCode != 0 {
		t.Errorf("RunCLI(pairs) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("RunCLI(pairs) ok = %v; want true", result["ok"])
	}
	if _, hasPairs := result["pairs"]; !hasPairs {
		t.Errorf("RunCLI(pairs) output missing 'pairs' key; got %v", result)
	}
}

// TestRunCLI_CommitHelp asserts that "fabric commit --help" output documents the
// fixed commit message and the Warp-SHA trailer, and does not advertise a
// --message flag that does not exist.
func TestRunCLI_CommitHelp(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"commit", "--help"})

	if exitCode != 0 {
		t.Errorf("RunCLI(commit --help) = %d; want 0", exitCode)
	}

	got := out.String()

	if !strings.Contains(got, "weft sync") {
		t.Errorf("commit --help output missing fixed message string %q; got:\n%s", "weft sync", got)
	}
	if !strings.Contains(got, "Warp-SHA") {
		t.Errorf("commit --help output missing %q trailer wording; got:\n%s", "Warp-SHA", got)
	}
	if strings.Contains(got, "--message") {
		t.Errorf("commit --help output unexpectedly contains --message flag; got:\n%s", got)
	}
}

// TestRunCLI_EnvMapToOption tests that the CLI edge properly maps WEFT_SKIP_PUSH to
// SyncOptions on the push verb. This is a serial test because it exercises the
// cwd-based push command which reads the current directory.
func TestRunCLI_EnvMapToOption(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)

	// Seed the weft-prime fixture with the fabric config template needed for RunCLI.
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})

	// Change to the hub directory so hubgeometry.Resolve can locate the repo from cwd;
	// t.Chdir restores the original cwd automatically after the test.
	t.Chdir(fixture.Hub)

	// Modify a file in the weft config that would be committed.
	weftConfigFile := filepath.Join(fixture.WeftPrime, hubgeometry.LyxDirName, "placeholder")
	if err := os.WriteFile(weftConfigFile, []byte("modified"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Set WEFT_SKIP_PUSH to prevent the actual push.
	t.Setenv("WEFT_SKIP_PUSH", "1")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"push"})

	if exitCode != 0 {
		t.Errorf("RunCLI push returned %d; want 0", exitCode)
		t.Logf("output: %s", out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("ok should be true; got false. Error: %v", result["error"])
	}
}

// TestRunCLI_CloneRequiresExactlyTwoArgs verifies that "fabric clone" rejects both
// too few (1) and too many (3, the old <host-url> <weft-url> <board-url> form this
// task removed) positional arguments with exit 1 and the updated usage message —
// runCloneWithReset's len(args) != 2 check runs before any git spawn, so a
// t.TempDir + t.Chdir is sufficient with no fixture.
func TestRunCLI_CloneRequiresExactlyTwoArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "OneArg",
			args: []string{"clone", "https://example.com/host"},
		},
		{
			name: "ThreeArgs",
			args: []string{"clone", "https://example.com/host", "https://example.com/weft", "https://example.com/board"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Chdir(tmpDir)

			var out bytes.Buffer
			exitCode := fabriccli.RunCLI(&out, tt.args)

			if exitCode != 1 {
				t.Errorf("RunCLI(%v) = %d; want 1", tt.args, exitCode)
			}

			result := decodeResult(t, &out)
			if ok, _ := result["ok"].(bool); ok {
				t.Errorf("RunCLI(%v) ok = true; want false", tt.args)
			}
			errMsg, _ := result["error"].(string)
			if !strings.Contains(errMsg, "usage: lyx fabric clone") {
				t.Errorf("RunCLI(%v) error = %q; want \"usage: lyx fabric clone\" substring", tt.args, errMsg)
			}
		})
	}
}
