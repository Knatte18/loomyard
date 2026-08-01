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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// setupCLIRepo creates a hub via lyxtest.CopyHostHub, changes into it, and writes a
// _lyx/config/fabric.yaml config at the repo-wide board dir so RunCLI's
// migrated topology-verb sites (LoadConfig(hubgeometry.BoardDir(l.Hub))) can
// resolve it. f.Hub is the fixture's host worktree root, i.e.
// hubgeometry.Layout.WorktreeRoot once resolved — the real hubgeometry Hub
// (WorktreeRoot's parent) is filepath.Dir(f.Hub), matching the established
// idiom in internal/boardcli's own cli_test.go/notes_test.go fixtures.
// Returns the hub path. Stays serial (no t.Parallel) because t.Chdir is
// required for RunCLI.
func setupCLIRepo(t *testing.T) string {
	t.Helper()
	f := lyxtest.CopyHostHub(t)
	t.Chdir(f.Hub)

	boardDir := hubgeometry.BoardDir(filepath.Dir(f.Hub))
	if err := os.MkdirAll(hubgeometry.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(hubgeometry.ConfigFile(boardDir, "fabric"), []byte("branch_prefix: wt-\npathspec: _lyx\n"), 0o644); err != nil {
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

	// Fabric config is a repo-wide fact read from the board dir (weft_verbs.go's
	// migrated PersistentPreRunE), not from the weft-prime fixture's own _lyx —
	// CopyPaired never materializes a _board dir, so seed it directly here.
	// fixture.Container is the real hubgeometry Hub (fixture.Hub's parent; see
	// CopyPaired's own doc comment), matching hubgeometry.Resolve(fixture.Hub).Hub.
	boardDir := hubgeometry.BoardDir(fixture.Container)
	if err := os.MkdirAll(hubgeometry.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("create board config dir: %v", err)
	}
	if err := os.WriteFile(hubgeometry.ConfigFile(boardDir, "fabric"), []byte(fabricengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("write board fabric.yaml: %v", err)
	}

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

// makeCLICloneHostBare creates a bare host remote at <dir>/<name>.git seeded
// with a README and a committed "backend" subdirectory, so a clone --subpath
// backend test has a real subpath to anchor at. This package has no existing
// two-repo clone fixture helper (CopyHostHub/CopyPaired both pre-materialize
// an already-paired hub, not raw clone sources), so it is built minimally
// inline, mirroring internal/fabricengine/clone_adopt_test.go's
// makeBareRemoteWithSubdir.
func makeCLICloneHostBare(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}
	lyxtest.MustRun(t, bare, "git", "init", "--bare")

	scratch := filepath.Join(dir, "scratch-"+name)
	if err := os.Mkdir(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	lyxtest.MustRun(t, scratch, "git", "init", "-b", "main")
	lyxtest.MustRun(t, scratch, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, scratch, "git", "config", "user.name", "Test")
	lyxtest.MustRun(t, scratch, "git", "remote", "add", "origin", filepath.ToSlash(bare))

	if err := os.WriteFile(filepath.Join(scratch, "README.md"), []byte("# "+name), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	backendDir := filepath.Join(scratch, "backend")
	if err := os.Mkdir(backendDir, 0o755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backendDir, "marker.txt"), []byte("backend content"), 0o644); err != nil {
		t.Fatalf("write backend marker: %v", err)
	}

	lyxtest.MustRun(t, scratch, "git", "add", "README.md", "backend")
	lyxtest.MustRun(t, scratch, "git", "commit", "-m", "init")
	lyxtest.MustRun(t, scratch, "git", "push", "-u", "origin", "main")

	return bare
}

// makeCLICloneWeftBare creates a genuinely empty (no commits) bare weft
// remote at <dir>/<name>.git — the state a brand-new weft repo is in before
// its first clone, exercising CloneHub's orphan _board path end-to-end
// through the CLI.
func makeCLICloneWeftBare(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	lyxtest.MustRun(t, dir, "git", "init", "--bare", "-b", "main", bare)
	return bare
}

// TestRunCLI_CloneEndToEnd drives "fabric clone --subpath backend <host>
// <weft>" against a local two-repo fixture and asserts the end-to-end
// clone-does-everything contract: the JSON envelope, the wired host
// junctions, the anchor marker and repo-wide config committed onto
// weft:main, and per-worktree module config reconciliation — i.e. that the
// card-16 CLI orchestration actually ran, not just fabricengine.CloneHub's
// own git-level clone.
func TestRunCLI_CloneEndToEnd(t *testing.T) {
	fixtures := t.TempDir()
	hostBare := makeCLICloneHostBare(t, fixtures, "clonecli-host")
	weftBare := makeCLICloneWeftBare(t, fixtures, "clonecli-weft")

	cloneParent := t.TempDir()
	t.Chdir(cloneParent)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{
		"clone", "--subpath", "backend",
		filepath.ToSlash(hostBare), filepath.ToSlash(weftBare),
	})
	if exitCode != 0 {
		t.Fatalf("RunCLI(clone --subpath backend) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("RunCLI(clone) ok = %v; want true; output: %s", result["ok"], out.String())
	}
	hubPath, _ := result["hub"].(string)
	if hubPath == "" {
		t.Fatalf("RunCLI(clone) output missing non-empty 'hub' key; got %v", result)
	}
	if anchor, _ := result["anchor"].(string); anchor != "backend" {
		t.Errorf("RunCLI(clone) anchor = %q; want %q", anchor, "backend")
	}

	// The prime host worktree's _lyx/_pattern junctions must be wired.
	primeCwd := filepath.Join(hubPath, "clonecli-host", "backend")
	for _, name := range []string{hubgeometry.LyxDirName, hubgeometry.PatternDirName} {
		link := filepath.Join(primeCwd, name)
		isLink, err := fslink.IsLink(link)
		if err != nil {
			t.Errorf("fslink.IsLink(%s): %v", link, err)
			continue
		}
		if !isLink {
			t.Errorf("%s is not wired as a junction after clone", link)
		}
	}

	// The .fabric-anchor marker and repo-wide fabric.yaml must be committed
	// onto weft:main (the board worktree), not merely present on disk. This
	// checks tracked-ness directly (git ls-files) rather than a blanket
	// `git status --porcelain` cleanliness assertion: CommitWeftAt/PushWeftAt
	// bypass ensureWeftLockDir's exclude-seeding (by design — see
	// weftgit.go's CommitWeftAt doc comment), so gitrepo's own
	// .gitrepo-push.lock can legitimately surface as untracked dirt on the
	// board worktree; that is a pre-existing gap in boardengine.Sync's own
	// identical CommitWeftAt/PushWeftAt pairing, not something this batch's
	// clone orchestration introduces or is responsible for fixing.
	boardDir := hubgeometry.BoardDir(hubPath)
	for _, relPath := range []string{
		hubgeometry.FabricAnchorName,
		filepath.Join(hubgeometry.LyxDirName, "config", "fabric.yaml"),
	} {
		tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", filepath.ToSlash(relPath)))
		if tracked == "" {
			t.Errorf("%s is not tracked on weft:main at %s", relPath, boardDir)
		}
	}

	// Per-worktree module configs (e.g. "board") must have been reconciled
	// on the weft side.
	weftBase := filepath.Join(hubgeometry.WeftSiblingPath(hubPath, "clonecli-host"), "backend")
	boardConfigPath := hubgeometry.ConfigFile(weftBase, "board")
	if _, err := os.Stat(boardConfigPath); err != nil {
		t.Errorf("per-worktree board config missing at %s: %v", boardConfigPath, err)
	}
}

// TestRunCLI_CloneDefaultSubpathAnchorsAtRoot asserts that "fabric clone" with
// no --subpath flag echoes anchor "." — the default root anchor.
func TestRunCLI_CloneDefaultSubpathAnchorsAtRoot(t *testing.T) {
	fixtures := t.TempDir()
	hostBare := makeCLICloneHostBare(t, fixtures, "clonecli-root-host")
	weftBare := makeCLICloneWeftBare(t, fixtures, "clonecli-root-weft")

	cloneParent := t.TempDir()
	t.Chdir(cloneParent)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{
		"clone", filepath.ToSlash(hostBare), filepath.ToSlash(weftBare),
	})
	if exitCode != 0 {
		t.Fatalf("RunCLI(clone) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("RunCLI(clone) ok = %v; want true; output: %s", result["ok"], out.String())
	}
	if anchor, _ := result["anchor"].(string); anchor != "." {
		t.Errorf("RunCLI(clone) anchor = %q; want %q", anchor, ".")
	}
}

// gitOutputCLI runs a git command in dir and returns its trimmed stdout,
// failing the test on any error — this package's capture-variant sibling of
// lyxtest.MustRun, which discards output. Named distinctly from
// internal/fabricengine's own gitOutput test helper since the two packages
// share no test code.
func gitOutputCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s in %s: %v", strings.Join(args, " "), dir, err)
	}
	return string(out)
}
