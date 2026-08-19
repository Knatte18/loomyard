//go:build integration

// cli_test.go covers the fabric CLI cobra surface: no-arg listing of all 14 verbs,
// unknown-subcommand cobra error, the --weft-path push-only gate, pairs with a
// real hub built by hubforge.NewHub, commit --help's fixed-message/Warp-SHA-trailer prose,
// pull --help's both-sides/reconcile prose, and the WEFT_SKIP_PUSH env-to-SyncOptions
// mapping on push — this package exercises both the topology and content-sync verb
// families against the one fabric command tree.

package fabriccli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// lyxFabricCLISubprocessCwdEnv is the env var that gates this file's re-exec entry point below: when
// set, the process is not a normal `go test` run but a subprocess spawned by
// TestRunCLI_FallbackCwdMatchesInjectedCwd, deliberately standing in a target directory via
// exec.Command's Dir field so fabriccli.RunCLI's Getwd fallback can be observed for real, without any
// t.Chdir/os.Chdir call anywhere in this file — this file is on the cwdmutation guard's subject set in
// cmd/lyx/cwdmutation_test.go and carries no exemption.
const lyxFabricCLISubprocessCwdEnv = "LYX_FABRICCLI_SUBPROCESS_CWD"

// init intercepts the subprocess-mode re-exec before testing.Main ever runs, so the child process's
// stdout carries only fabriccli.RunCLI's own JSON output — never the "=== RUN"/"--- PASS" noise `go
// test` would otherwise interleave with it, which would defeat the byte-for-byte comparison
// TestRunCLI_FallbackCwdMatchesInjectedCwd performs against fabriccli.RunCLIIn's output.
func init() {
	if os.Getenv(lyxFabricCLISubprocessCwdEnv) == "" {
		return
	}
	var out bytes.Buffer
	code := fabriccli.RunCLI(&out, []string{"pairs"})
	os.Stdout.Write(out.Bytes())
	os.Exit(code)
}

// setupCLIRepo builds a real hub via hubforge.NewHub and overrides the repo-wide fabric.yaml at the
// board dir with a non-default branch_prefix so RunCLIIn's migrated topology-verb sites
// (LoadConfig(fabricengine.BoardDir(l.HubPath))) can be seen resolving a genuinely overridden value
// rather than the plain registered template hubforge.NewHub already materializes on its own.
// Returns the hub so callers drive fabriccli.RunCLIIn against h.PrimeWorktree() explicitly, rather
// than relying on the process cwd.
func setupCLIRepo(t *testing.T) *hubforge.Hub {
	t.Helper()
	h := hubforge.NewHub(t, ".")

	hubforge.SeedFabricConfig(t, h, "branch_prefix: wt-\npathspec: _lyx\n")
	return h
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

// TestRunCLI_NoArgs verifies that "lyx fabric" with no subcommand prints the subcommand listing
// naming all 14 verbs — no git repo is needed, since the bare group command is excluded from
// weft-verb PersistentPreRunE resolution.
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

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1 and emits a JSON error
// envelope with ok=false.
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	// "fabric" is not in weftVerbNames, so the PersistentPreRunE guard returns nil early, bypassing
	// all resolution -- no cwd setup is needed to reach this path, whatever the process cwd is.
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

// TestRunCLI_WeftPathPushOnly verifies that --weft-path with a non-push subcommand returns exit 1
// and the JSON error envelope {"ok":false,"error":"subcommand requires a worktree context"}.
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

// TestRunCLI_PairsReturnsPairsKey verifies that "fabric pairs" resolves the topology config from
// cwd and emits ok=true with a "pairs" key.
func TestRunCLI_PairsReturnsPairsKey(t *testing.T) {
	h := setupCLIRepo(t)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"pairs"})
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

// TestRunCLI_PairsReportsPollutionEntryWithRemedy pins the pollution JSON shape at the CLI
// boundary, not only at the fabricengine.Status engine boundary: with a file tracked directly
// under _lyx in the warp index, "fabric pairs" must still emit a pollution entry naming that
// path with a non-empty "remedy" key. The removed report-only marker key is gone from
// PollutionEntry as of this batch — Remedy == "" now carries the report-only signal on its own —
// so this test does not assert its absence directly; it asserts the surviving "remedy" key is
// present and non-empty instead, which is the shape that matters to a CLI consumer.
func TestRunCLI_PairsReportsPollutionEntryWithRemedy(t *testing.T) {
	// A real hub, not the warp-only setupCLIRepo fixture: Status bails out of the per-pair pollution
	// scan early when the weft sibling is missing, and hubforge.NewHub always builds a paired hub.
	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _lyx\n")

	// A real hub already wires _lyx as a junction onto the weft config. Pollution means an operator
	// replaced that junction with a real, tracked directory, so the junction must be removed first --
	// writing through it, as the old ungeometried fixture's plain mkdir did, would land the file on the
	// weft side instead of polluting the warp index this test means to exercise. The explicit "-f" on
	// the git add below is likewise new: WireJunctionsWith seeded _lyx into the warp's
	// .git/info/exclude, and git refuses to add an explicitly-named ignored path without it.
	warpLyxDir := filepath.Join(h.PrimeWorktree(), lyxdirs.LyxDirName)
	if err := fslink.Remove(warpLyxDir); err != nil {
		t.Fatalf("remove warp _lyx junction: %v", err)
	}
	if err := os.MkdirAll(warpLyxDir, 0o755); err != nil {
		t.Fatalf("mkdir warp _lyx dir: %v", err)
	}
	trackedFile := filepath.Join(warpLyxDir, "PATTERN.md")
	if err := os.WriteFile(trackedFile, []byte("# constraints\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "-f", "--", lyxdirs.LyxDirName)
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "-m", "accidentally track _lyx")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"pairs"})
	if exitCode != 0 {
		t.Errorf("RunCLI(pairs) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	pairs, ok := result["pairs"].([]any)
	if !ok || len(pairs) == 0 {
		t.Fatalf("RunCLI(pairs) 'pairs' = %v; want a non-empty array", result["pairs"])
	}

	var found map[string]any
	for _, p := range pairs {
		pair, ok := p.(map[string]any)
		if !ok {
			continue
		}
		pollution, ok := pair["pollution"].([]any)
		if !ok {
			continue
		}
		for _, e := range pollution {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if path, _ := entry["path"].(string); strings.Contains(path, "PATTERN.md") {
				found = entry
			}
		}
	}
	if found == nil {
		t.Fatalf("no pollution entry naming PATTERN.md found in %+v", result)
	}
	remedy, _ := found["remedy"].(string)
	if remedy == "" {
		t.Errorf("pollution entry %+v has empty/missing 'remedy'; want a non-empty git rm --cached remedy", found)
	}
}

// TestRunCLI_CommitHelp asserts that "fabric commit --help" output documents the fixed commit
// message and the Warp-SHA trailer,
// and does not advertise a --message flag that does not exist.
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

// TestRunCLI_PullHelp asserts that "fabric pull --help" documents the both-sides pull/reconcile
// behaviour — a help-accuracy assertion in the same style as TestRunCLI_CommitHelp, guarding
// against the CLI/Cobra Invariant's "stale help is review-blocking" obligation now that pull drives
// the unified Fabric.Pull instead of the weft-only PullWeft.
func TestRunCLI_PullHelp(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"pull", "--help"})

	if exitCode != 0 {
		t.Errorf("RunCLI(pull --help) = %d; want 0", exitCode)
	}

	got := out.String()

	if !strings.Contains(got, "Pulls both sides of the pair") {
		t.Errorf("pull --help output missing both-sides Long text; got:\n%s", got)
	}
	if !strings.Contains(got, "reconcile") {
		t.Errorf("pull --help output missing reconcile wording; got:\n%s", got)
	}
	if !strings.Contains(got, "rewrite") {
		t.Errorf("pull --help output missing warp-history-rewrite wording; got:\n%s", got)
	}
}

// TestRunCLI_PullShortNonEmpty asserts pullCmd's Short summary is non-empty and itself names the
// both-sides/reconcile behaviour, building the command tree via the fabriccli.Command() seam (the
// CLI/Cobra Invariant's "Short on every command" obligation, checked directly against pull's own
// Short rather than via --help output, where Long supersedes Short).
func TestRunCLI_PullShortNonEmpty(t *testing.T) {
	t.Parallel()

	root := fabriccli.Command()
	pull, _, err := root.Find([]string{"pull"})
	if err != nil {
		t.Fatalf("root.Find([pull]) error: %v", err)
	}

	if pull.Short == "" {
		t.Errorf("pullCmd.Short is empty; want a non-empty both-sides summary")
	}
	if !strings.Contains(pull.Short, "reconcil") {
		t.Errorf("pullCmd.Short = %q; want it to mention reconcile behaviour", pull.Short)
	}
}

// TestRunCLI_EnvMapToOption tests that the CLI edge properly maps WEFT_SKIP_PUSH to SyncOptions on
// the push verb.
// Stays serial (no t.Parallel): it calls t.Setenv("WEFT_SKIP_PUSH", "1") below, which panics under
// t.Parallel() exactly as t.Chdir did.
func TestRunCLI_EnvMapToOption(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	// Fabric config is a repo-wide fact read from the board dir (weft_verbs.go's migrated
	// PersistentPreRunE): fabriccli.CloneAndWire already materializes it with the plain registered
	// template as part of building h, so nothing further needs seeding here.

	// Modify a file in the weft config that would be committed.
	weftConfigFile := filepath.Join(h.WeftBase, lyxdirs.LyxDirName, "placeholder")
	if err := os.WriteFile(weftConfigFile, []byte("modified"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Set WEFT_SKIP_PUSH to prevent the actual push.
	t.Setenv("WEFT_SKIP_PUSH", "1")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"push"})

	if exitCode != 0 {
		t.Errorf("RunCLI push returned %d; want 0", exitCode)
		t.Logf("output: %s", out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("ok should be true; got false. Error: %v", result["error"])
	}
}

// TestRunCLI_SyncStillCommitsLyx_WhenRepoWidePathspecNamesOnlyPattern is the card-40 regression
// guard: with the repo-wide fabric.yaml's pathspec naming only "_extra" (a single non-_lyx name),
// "lyx fabric sync" must still commit _lyx content, because weft_verbs.go now
// builds its sync pathspec from fabricengine.PathspecNames — the routing set, which always contains
// "_lyx" structurally — never from a raw, unfiltered Config.Dirs() that would silently drop it.
// This is the single most breakage-prone edit in the whole task: a miss here is silent, not loud.
// Stays serial (no t.Parallel): it calls t.Setenv("WEFT_SKIP_PUSH", "1") below, which panics under
// t.Parallel() exactly as t.Chdir did.
func TestRunCLI_SyncStillCommitsLyx_WhenRepoWidePathspecNamesOnlyPattern(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	// Repo-wide fabric.yaml names only "_extra" -- a single non-_lyx name --
	// proving _lyx arrives from the routing set structurally, not from this
	// config.
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\n")

	weftConfigFile := filepath.Join(h.WeftBase, lyxdirs.LyxDirName, "placeholder")
	if err := os.WriteFile(weftConfigFile, []byte("modified for sync regression"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Prevent the detached push child from doing any real network work;
	// SpawnDetachedPush itself checks this env var before spawning.
	t.Setenv("WEFT_SKIP_PUSH", "1")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"sync"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(sync) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("RunCLI(sync) ok = %v; want true; output: %s", result["ok"], out.String())
	}

	tracked := strings.TrimSpace(gitOutputCLI(t, h.PrimeWeft(), "log", "-1", "--name-only", "--pretty=format:"))
	if !strings.Contains(tracked, filepath.ToSlash(filepath.Join(lyxdirs.LyxDirName, "placeholder"))) {
		t.Errorf("HEAD commit on %s does not touch %s; want the sync-built pathspec to still cover _lyx even though the repo-wide config names only _extra\nfiles: %s", h.PrimeWeft(), lyxdirs.LyxDirName, tracked)
	}
}

// TestRunCLI_CloneAcceptsOneOrTwoArgs verifies that "fabric clone" now accepts either one
// positional (<weft-url>, warp URL derived from the recorded binding) or two
// (<weft-url> <warp-url>), and rejects both zero and three positional arguments with exit 1 and the
// updated usage message — runCloneWithReset's len(args) != 1 && len(args) != 2 check runs before any
// git spawn, so a bare t.TempDir passed as RunCLIIn's cwd is sufficient with no fixture.
func TestRunCLI_CloneAcceptsOneOrTwoArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "ZeroArgs",
			args: []string{"clone"},
		},
		{
			name: "ThreeArgs",
			args: []string{"clone", "https://example.com/weft", "https://example.com/warp", "https://example.com/board"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			var out bytes.Buffer
			exitCode := fabriccli.RunCLIIn(tmpDir, &out, tt.args)

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

// makeCLICloneWarpBare creates a bare warp remote at <dir>/<name>.git seeded
// with a README and a committed "backend" subdirectory, so a clone --subpath
// backend test has a real subpath to anchor at. This package has no existing
// two-repo clone fixture helper (hubforge.NewHub pre-materializes an
// already-paired hub, not raw clone sources), so it is built minimally
// inline, mirroring internal/fabricengine/clone_adopt_test.go's
// makeBareRemoteWithSubdir.
func makeCLICloneWarpBare(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}
	gitkit.MustRun(t, bare, "git", "init", "--bare")

	scratch := filepath.Join(dir, "scratch-"+name)
	if err := os.Mkdir(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	gitkit.MustRun(t, scratch, "git", "init", "-b", "main")
	gitkit.MustRun(t, scratch, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, scratch, "git", "config", "user.name", "Test")
	gitkit.MustRun(t, scratch, "git", "remote", "add", "origin", filepath.ToSlash(bare))

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

	gitkit.MustRun(t, scratch, "git", "add", "README.md", "backend")
	gitkit.MustRun(t, scratch, "git", "commit", "-m", "init")
	gitkit.MustRun(t, scratch, "git", "push", "-u", "origin", "main")

	return bare
}

// makeCLICloneWeftBare creates a genuinely empty (no commits) bare weft
// remote at <dir>/<name>.git — the state a brand-new weft repo is in before
// its first clone, exercising CloneHub's orphan _board path end-to-end
// through the CLI.
func makeCLICloneWeftBare(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	gitkit.MustRun(t, dir, "git", "init", "--bare", "-b", "main", bare)
	return bare
}

// TestRunCLI_CloneEndToEnd drives "fabric clone --subpath backend <weft> <warp>" against a local
// two-repo fixture and asserts the end-to-end clone-does-everything contract: the JSON envelope
// (including the "warp" and "warp_binding_recorded" keys), the wired warp junctions, the anchor
// marker, repo-wide config, and warp-binding record committed onto weft:main, and per-worktree
// module config reconciliation — i.e.
// that the card-16 CLI orchestration actually ran, not just fabricengine.CloneHub's own git-level
// clone.
func TestRunCLI_CloneEndToEnd(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeCLICloneWarpBare(t, fixtures, "clonecli-warp")
	weftBare := makeCLICloneWeftBare(t, fixtures, "clonecli-weft")

	cloneParent := t.TempDir()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{
		"clone", "--into", cloneParent, "--subpath", "backend",
		filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
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
	if warp, _ := result["warp"].(string); warp != filepath.ToSlash(warpBare) {
		t.Errorf("RunCLI(clone) warp = %q; want %q", warp, filepath.ToSlash(warpBare))
	}
	if recorded, ok := result["warp_binding_recorded"].(bool); !ok || !recorded {
		t.Errorf("RunCLI(clone) warp_binding_recorded = %v; want true", result["warp_binding_recorded"])
	}

	// The prime warp worktree's structural junctions (_lyx and .lyx) must be
	// wired: .lyx is structural (structuralNeverCommittedDirs) and is wired by
	// every clone regardless of pathspec, so it is checked here rather than
	// an optional config-driven name, which a genuinely empty bare weft
	// clone (zero commits, no pre-existing weft:main fabric.yaml) never wires.
	primeCwd := filepath.Join(hubPath, "clonecli-warp", "backend")
	for _, name := range []string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName} {
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

	// The .lyx-anchor marker and repo-wide fabric.yaml must be committed
	// onto weft:main (the board worktree), not merely present on disk. This
	// checks tracked-ness directly (git ls-files) rather than a blanket
	// `git status --porcelain` cleanliness assertion: CommitWeftAt/PushWeftAt
	// bypass ensureWeftLockDir's exclude-seeding (by design — see
	// weftgit.go's CommitWeftAt doc comment), so gitrepo's own
	// .gitrepo-push.lock can legitimately surface as untracked dirt on the
	// board worktree; that is a pre-existing gap in boardengine.Sync's own
	// identical CommitWeftAt/PushWeftAt pairing, not something this batch's
	// clone orchestration introduces or is responsible for fixing.
	boardDir := fabricengine.BoardDir(hubPath)
	for _, relPath := range []string{
		lyxcwd.AnchorFileName,
		filepath.Join(lyxdirs.LyxDirName, "config", "fabric.yaml"),
		fabricengine.WarpBindingFileName,
	} {
		tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", filepath.ToSlash(relPath)))
		if tracked == "" {
			t.Errorf("%s is not tracked on weft:main at %s", relPath, boardDir)
		}
	}

	// Per-worktree module configs (e.g. "board") must have been reconciled
	// on the weft side.
	weftBase := filepath.Join(weftname.SiblingPath(hubPath, "clonecli-warp"), "backend")
	boardConfigPath := configengine.ConfigFile(weftBase, "board")
	if _, err := os.Stat(boardConfigPath); err != nil {
		t.Errorf("per-worktree board config missing at %s: %v", boardConfigPath, err)
	}
}

// TestRunCLI_CloneDefaultSubpathAnchorsAtRoot asserts that "fabric clone" with no --subpath flag
// echoes anchor "." — the default root anchor.
func TestRunCLI_CloneDefaultSubpathAnchorsAtRoot(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeCLICloneWarpBare(t, fixtures, "clonecli-root-warp")
	weftBare := makeCLICloneWeftBare(t, fixtures, "clonecli-root-weft")

	cloneParent := t.TempDir()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{
		"clone", "--into", cloneParent, filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
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

// TestRunCLI_CloneUnboundWeftErrorNamesTwoArgForm asserts that a one-positional clone against a
// local bare weft fixture carrying no recorded binding exits 1 through the output.Err envelope,
// with a message naming both the unbound-binding condition and the two-argument form's exact
// invocation as the remedy. makeCLICloneWeftBare's fixture is genuinely empty (no commits), which is
// the unborn-HEAD case the weft-candidate guard admits on its own, so --force-bootstrap is not
// needed here.
func TestRunCLI_CloneUnboundWeftErrorNamesTwoArgForm(t *testing.T) {
	fixtures := t.TempDir()
	weftBare := makeCLICloneWeftBare(t, fixtures, "clonecli-unbound-weft")

	cloneParent := t.TempDir()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"clone", "--into", cloneParent, filepath.ToSlash(weftBare)})
	if exitCode != 1 {
		t.Fatalf("RunCLI(clone <weft-url>) = %d; want 1\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); ok {
		t.Errorf("RunCLI(clone <weft-url>) ok = true; want false")
	}
	errMsg, _ := result["error"].(string)
	if !strings.Contains(errMsg, "has no recorded warp binding") {
		t.Errorf("RunCLI(clone <weft-url>) error = %q; want \"has no recorded warp binding\" substring", errMsg)
	}
	if !strings.Contains(errMsg, "lyx fabric clone <weft-url> <warp-url>") {
		t.Errorf("RunCLI(clone <weft-url>) error = %q; want \"lyx fabric clone <weft-url> <warp-url>\" substring", errMsg)
	}
}

// TestRunCLI_MergeHelp asserts "fabric merge --help" names all four of its own flags.
func TestRunCLI_MergeHelp(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"merge", "--help"})
	if exitCode != 0 {
		t.Errorf("RunCLI(merge --help) = %d; want 0", exitCode)
	}

	got := out.String()
	for _, flag := range []string{"--squash", "--continue", "--abort", "--message"} {
		if !strings.Contains(got, flag) {
			t.Errorf("merge --help output missing %q; got:\n%s", flag, got)
		}
	}
}

// TestRunCLI_MergeInHelp asserts "fabric merge-in --help" has a non-empty Short.
func TestRunCLI_MergeInHelp(t *testing.T) {
	t.Parallel()

	root := fabriccli.Command()
	mergeIn, _, err := root.Find([]string{"merge-in"})
	if err != nil {
		t.Fatalf("root.Find([merge-in]) error: %v", err)
	}
	if mergeIn.Short == "" {
		t.Errorf("merge-in.Short is empty; want a non-empty summary")
	}
}

// TestRunCLI_MergeContinueRejectsExtraArg asserts "merge --continue extra-arg" fails with a
// usage-shaped error envelope: the custom Args validator rejects a positional argument in
// --continue/--abort mode.
func TestRunCLI_MergeContinueRejectsExtraArg(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"merge", "--continue", "extra-arg"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(merge --continue extra-arg) = %d; want 1\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); ok {
		t.Errorf("RunCLI(merge --continue extra-arg) ok = true; want false")
	}
	errMsg, _ := result["error"].(string)
	if !strings.Contains(errMsg, "usage:") {
		t.Errorf("RunCLI(merge --continue extra-arg) error = %q; want a usage-shaped message", errMsg)
	}
}

// TestRunCLI_MergeContinueAndAbortTogetherFail asserts "merge --continue --abort" fails with a
// usage-shaped error envelope — the two flags are mutually exclusive. Driven against a real hub
// (rather than bare RunCLI) so PersistentPreRunE's own cwd resolution succeeds first and
// MarkFlagsMutuallyExclusive's own ValidateFlagGroups refusal is the only error the run produces —
// against an unresolvable cwd, PersistentPreRunE would already have written its own error envelope
// before ValidateFlagGroups ever runs, yielding two JSON lines instead of one.
func TestRunCLI_MergeContinueAndAbortTogetherFail(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"merge", "--continue", "--abort"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(merge --continue --abort) = %d; want 1\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); ok {
		t.Errorf("RunCLI(merge --continue --abort) ok = true; want false")
	}
	errMsg, _ := result["error"].(string)
	if errMsg == "" {
		t.Errorf("RunCLI(merge --continue --abort) error is empty; want a usage-shaped message naming the mutual exclusion")
	}
}

// gitOutputCLI runs a git command in dir and returns its trimmed stdout,
// failing the test on any error — this package's capture-variant sibling of
// gitkit.MustRun, which discards output. Named distinctly from
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

// TestRunCLI_ReconcileBacksFillsWarpBinding drives both the two-positional clone and reconcile
// through fabriccli.RunCLI so the commit-and-push half of the backfill is actually exercised — the
// record_failed value is set only by the handler and cannot be observed from an engine-only test. It
// builds the hub, deletes the recorded binding with plain git and commits that deletion (the board is
// already clean beforehand: the CLI clone commits the anchor and repo-wide config through Bolt as part
// of its normal run), then asserts reconcile exits 0, reports "recorded", and leaves the record
// tracked on the board worktree, with "pairs" still present and unchanged in shape.
func TestRunCLI_ReconcileBacksFillsWarpBinding(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeCLICloneWarpBare(t, fixtures, "reconcilecli-warp")
	weftBare := makeCLICloneWeftBare(t, fixtures, "reconcilecli-weft")

	cloneParent := t.TempDir()

	var cloneOut bytes.Buffer
	// No --force-bootstrap: makeCLICloneWeftBare's fixture is genuinely empty (no commits), which is
	// the unborn-HEAD case the weft-candidate guard admits on its own.
	exitCode := fabriccli.RunCLI(&cloneOut, []string{
		"clone", "--into", cloneParent, filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
	})
	if exitCode != 0 {
		t.Fatalf("RunCLI(clone) = %d; want 0\noutput: %s", exitCode, cloneOut.String())
	}
	cloneResult := decodeResult(t, &cloneOut)
	hubPath, _ := cloneResult["hub"].(string)
	if hubPath == "" {
		t.Fatalf("RunCLI(clone) output missing non-empty 'hub' key; got %v", cloneResult)
	}

	boardDir := fabricengine.BoardDir(hubPath)
	gitkit.MustRun(t, boardDir, "git", "rm", fabricengine.WarpBindingFileName)
	gitkit.MustRun(t, boardDir, "git", "commit", "-m", "test fixture: unbind hub")

	// Proving cwd is a per-call value and not a per-process one is exactly what this test exists to
	// demonstrate: this reconcile runs against a different directory than the clone above, passed
	// per-call rather than hoisted to a shared variable.
	var reconcileOut bytes.Buffer
	exitCode = fabriccli.RunCLIIn(filepath.Join(hubPath, "reconcilecli-warp"), &reconcileOut, []string{"reconcile"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(reconcile) = %d; want 0\noutput: %s", exitCode, reconcileOut.String())
	}

	result := decodeResult(t, &reconcileOut)
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("RunCLI(reconcile) ok = %v; want true; output: %s", result["ok"], reconcileOut.String())
	}
	if binding, _ := result["warp_binding"].(string); binding != string(fabricengine.WarpBindingOutcomeRecorded) {
		t.Errorf("RunCLI(reconcile) warp_binding = %q; want %q", binding, fabricengine.WarpBindingOutcomeRecorded)
	}
	if _, present := result["pairs"]; !present {
		t.Errorf("RunCLI(reconcile) output missing 'pairs' key; want it present and unchanged in shape")
	}

	tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", fabricengine.WarpBindingFileName))
	if tracked == "" {
		t.Errorf("%s is not tracked on weft:main at %s after the reconcile backfill", fabricengine.WarpBindingFileName, boardDir)
	}
}

// TestRunCLI_ReconcileBackfillFailureIsNonFatal points the weft remote at an unreachable path so the
// backfill's push fails after its commit succeeds, then asserts the envelope reports "record_failed"
// with a non-empty detail while the exit code stays 0 — a failed backfill commit or push is non-fatal,
// mirroring the board-junction precedent that a convenience repair may never downgrade a reconcile
// verdict. The exit-code assertion is the point of this test.
func TestRunCLI_ReconcileBackfillFailureIsNonFatal(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeCLICloneWarpBare(t, fixtures, "reconcilecli-fail-warp")
	weftBare := makeCLICloneWeftBare(t, fixtures, "reconcilecli-fail-weft")

	cloneParent := t.TempDir()

	var cloneOut bytes.Buffer
	exitCode := fabriccli.RunCLI(&cloneOut, []string{
		"clone", "--into", cloneParent, filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
	})
	if exitCode != 0 {
		t.Fatalf("RunCLI(clone) = %d; want 0\noutput: %s", exitCode, cloneOut.String())
	}
	cloneResult := decodeResult(t, &cloneOut)
	hubPath, _ := cloneResult["hub"].(string)
	if hubPath == "" {
		t.Fatalf("RunCLI(clone) output missing non-empty 'hub' key; got %v", cloneResult)
	}

	boardDir := fabricengine.BoardDir(hubPath)
	gitkit.MustRun(t, boardDir, "git", "rm", fabricengine.WarpBindingFileName)
	gitkit.MustRun(t, boardDir, "git", "commit", "-m", "test fixture: unbind hub")

	unreachable := filepath.ToSlash(filepath.Join(fixtures, "does-not-exist.git"))
	gitkit.MustRun(t, boardDir, "git", "remote", "set-url", "origin", unreachable)

	// Proving cwd is a per-call value and not a per-process one is exactly what this test exists to
	// demonstrate: this reconcile runs against a different directory than the clone above, passed
	// per-call rather than hoisted to a shared variable.
	var reconcileOut bytes.Buffer
	exitCode = fabriccli.RunCLIIn(filepath.Join(hubPath, "reconcilecli-fail-warp"), &reconcileOut, []string{"reconcile"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(reconcile) = %d; want 0 (a failed backfill push must be non-fatal)\noutput: %s", exitCode, reconcileOut.String())
	}

	result := decodeResult(t, &reconcileOut)
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("RunCLI(reconcile) ok = %v; want true; output: %s", result["ok"], reconcileOut.String())
	}
	if binding, _ := result["warp_binding"].(string); binding != string(fabricengine.WarpBindingOutcomeRecordFailed) {
		t.Errorf("RunCLI(reconcile) warp_binding = %q; want %q", binding, fabricengine.WarpBindingOutcomeRecordFailed)
	}
	if detail, _ := result["warp_binding_detail"].(string); detail == "" {
		t.Errorf("RunCLI(reconcile) warp_binding_detail is empty; want a non-empty push-failure message")
	}
}

// TestRunCLI_WeftSiblingNonAnchoredCwd_GetsWeftRefusal pins the refusal an operator sees from a
// weft sibling's NON-anchored directory on a subpath-anchored hub: the specific
// weft-sibling message, never the generic cwd-gate error — which names the weft's own anchored
// directory as the place to stand and thereby directs the operator deeper INTO the weft.
func TestRunCLI_WeftSiblingNonAnchoredCwd_GetsWeftRefusal(t *testing.T) {
	// "backend" is a subpath anchor, so the weft ROOT (h.PrimeWeft()) is not the anchored directory
	// (h.WeftBase) -- fabriccli.CloneAndWire records that anchor for real, so no hand-written anchor
	// marker is needed here.
	h := hubforge.NewHub(t, "backend")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWeft(), &out, []string{"pairs"})
	if exitCode == 0 {
		t.Fatalf("RunCLI(pairs) from weft sibling = 0; want a refusal\noutput: %s", out.String())
	}
	result := decodeResult(t, &out)
	errMsg, _ := result["error"].(string)
	if !strings.Contains(errMsg, "weft sibling of a pair") {
		t.Errorf("RunCLI(pairs) error = %q; want the specific weft-sibling refusal, not the generic gate error", errMsg)
	}
}

// TestRunCLI_Reconcile_HealsMissingRepoWideConfig pins reconcile's self-healing of the repo-wide
// fabric.yaml: on a hub that has none, reconcile used to fail with "not initialized here; run
// \"lyx fabric reconcile\"" — prescribing the command that just failed — and must instead
// materialize the config and proceed.
func TestRunCLI_Reconcile_HealsMissingRepoWideConfig(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	// hubforge.NewHub always materializes a repo-wide fabric.yaml as part of building a real hub, so
	// the "missing config" state this test exists to heal must be produced by hand here -- an operator
	// deleting the file is exactly the scenario the healing logic guards against.
	cfgPath := configengine.ConfigFile(h.BoardDir(), "fabric")
	if err := os.Remove(cfgPath); err != nil {
		t.Fatalf("remove repo-wide fabric config: %v", err)
	}

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"reconcile"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(reconcile) = %d; want 0 (reconcile must heal the missing config, not report it)\noutput: %s", exitCode, out.String())
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("repo-wide fabric config not materialized at %s: %v", cfgPath, err)
	}
}

// TestRunCLI_ReadOnlyVerbsOmitMutationsKey asserts the four read-only verbs — list, pairs, status and
// diff — never carry a "mutations" key in their envelope: nothing was mutated, so the which-verbs
// scope decision is machine-held rather than a convention. All four are driven against one real,
// paired hub, since the weft-verb pair (status, diff) resolves its Fabric handle through the
// PersistentPreRunE that only a real hub satisfies.
func TestRunCLI_ReadOnlyVerbsOmitMutationsKey(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: \"\"\n")

	warpSHA := strings.TrimSpace(gitOutputCLI(t, h.PrimeWorktree(), "rev-parse", "HEAD"))

	tests := []struct {
		name string
		args []string
	}{
		{name: "List", args: []string{"list"}},
		{name: "Pairs", args: []string{"pairs"}},
		{name: "Status", args: []string{"status"}},
		{name: "Diff", args: []string{"diff", warpSHA}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, tt.args)
			if exitCode != 0 {
				t.Fatalf("RunCLI(%v) = %d; want 0\noutput: %s", tt.args, exitCode, out.String())
			}
			result := decodeResult(t, &out)
			if _, present := result["mutations"]; present {
				t.Errorf("RunCLI(%v) output has a 'mutations' key; want it absent from a read-only verb's envelope: %v", tt.args, result)
			}
			if _, present := result["partial"]; present {
				t.Errorf("RunCLI(%v) output has a 'partial' key; want it absent from a read-only verb's envelope: %v", tt.args, result)
			}
		})
	}
}

// TestRunCLI_Remove_RefusesDriftedPortalJunctionWithRefusalObject drives a real gate refusal through
// "fabric remove --force", reached via fabricengine.RefusalOf's own contract rather than a
// hand-rolled stub: the pair's portal link is hand-wired pointing at a directory OTHER than its real
// portal target, so removePortal's ownedWiredJunction ownership check refuses — its error propagates
// through Remove's %w-wrapped chain all the way to runRemove's errWithRecord call. Asserts the
// "refusal" object carries all four keys (check, what, target, reason), and that the flattened
// "error" string is still present alongside it.
//
// This is the repo's only positive assertion that the "refusal" object reaches an envelope
// (envelope_test.go asserts only its absence), re-homed here from the deleted _board junction's own
// unwire test onto the portal junction: removePortal builds
// ownedWiredJunction([]string{PortalLink(l, slug)}, portalTarget(l, slug)), structurally identical to
// the deleted unwireBoardLink's own gate, and Remove propagates the resulting *destructiveRefusal
// through surfaceRefusal.
func TestRunCLI_Remove_RefusesDriftedPortalJunctionWithRefusalObject(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: \"\"\n")

	const slug = "remove-refusal-portal"
	var addOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &addOut, []string{"add", slug}); exitCode != 0 {
		t.Fatalf("RunCLI(add) = %d; want 0\noutput: %s", exitCode, addOut.String())
	}

	// The correct portal link is already wired by "fabric add" above; drift it onto a WRONG target —
	// anywhere other than its real portal target — so removePortal's ownership check refuses.
	wrongTarget := t.TempDir()
	portalLink := h.PairPortalLink(slug)
	if err := fslink.Remove(portalLink); err != nil {
		t.Fatalf("remove correctly-wired portal link: %v", err)
	}
	if err := fslink.CreateDirLink(portalLink, wrongTarget); err != nil {
		t.Fatalf("create drifted portal link: %v", err)
	}

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", "--force", slug})
	if exitCode != 1 {
		t.Fatalf("RunCLI(remove) = %d; want 1 (a drifted portal junction must be refused)\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	if ok, _ := result["ok"].(bool); ok {
		t.Errorf("RunCLI(remove) ok = true; want false")
	}
	if errMsg, _ := result["error"].(string); errMsg == "" {
		t.Errorf("RunCLI(remove) output missing non-empty 'error'; want the flattened error string alongside the refusal object")
	}
	refusal, ok := result["refusal"].(map[string]any)
	if !ok {
		t.Fatalf("RunCLI(remove) output missing 'refusal' object; got %v", result)
	}
	for _, key := range []string{"check", "what", "target", "reason"} {
		val, present := refusal[key]
		if !present {
			t.Errorf("refusal object missing key %q: %v", key, refusal)
			continue
		}
		if s, _ := val.(string); s == "" {
			t.Errorf("refusal[%q] = %q; want a non-empty string", key, s)
		}
	}
	if check, _ := refusal["check"].(string); check != string(fabricengine.CheckOwnership) {
		t.Errorf("refusal[\"check\"] = %q; want %q", check, fabricengine.CheckOwnership)
	}
	if _, present := result["mutations"]; !present {
		t.Errorf("RunCLI(remove) output missing 'mutations' key on the failure path")
	}
}

// TestRunCLI_FallbackCwdMatchesInjectedCwd proves fabriccli.RunCLI's process-cwd fallback and
// fabriccli.RunCLIIn's explicit-cwd branch agree on one read-only verb: it drives "pairs" through
// RunCLIIn(h.PrimeWorktree(), …) directly in this process, then drives the same verb through RunCLI
// in a genuinely separate OS process whose working directory is set to h.PrimeWorktree() via
// exec.Command's Dir field — never via a t.Chdir/os.Chdir call on this test binary's own process,
// which this file's entry on the cwdmutation guard's subject set bans outright. The subprocess is
// this very test binary, re-exec'd and intercepted by this file's package-level init before testing.Main
// runs, gated by lyxFabricCLISubprocessCwdEnv, mirroring the standard library's TestHelperProcess idiom.
func TestRunCLI_FallbackCwdMatchesInjectedCwd(t *testing.T) {
	h := setupCLIRepo(t)

	var injectedOut bytes.Buffer
	injectedCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &injectedOut, []string{"pairs"})
	if injectedCode != 0 {
		t.Fatalf("RunCLIIn(pairs) = %d; want 0\noutput: %s", injectedCode, injectedOut.String())
	}

	cmd := exec.Command(os.Args[0])
	cmd.Dir = h.PrimeWorktree()
	cmd.Env = append(os.Environ(), lyxFabricCLISubprocessCwdEnv+"=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	fallbackOut, runErr := cmd.Output()

	fallbackCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			t.Fatalf("subprocess RunCLI(pairs) failed to run: %v\nstderr: %s", runErr, stderr.String())
		}
		fallbackCode = exitErr.ExitCode()
	}

	if fallbackCode != injectedCode {
		t.Errorf("RunCLI (process-cwd fallback) exit code = %d; want %d (RunCLIIn's)\nstderr: %s", fallbackCode, injectedCode, stderr.String())
	}
	if string(fallbackOut) != injectedOut.String() {
		t.Errorf("RunCLI (process-cwd fallback) output = %q; want it identical to RunCLIIn(h.PrimeWorktree(), …) output %q", fallbackOut, injectedOut.String())
	}
}

// TestRunCLI_CloneIntoFlagCreatesHubAtDirectory asserts that "fabric clone --into <dir>" creates the
// new hub under <dir>, not under the seam cwd RunCLIIn was given — the --into flag names a genuine
// clone destination distinct from the lookup cwd every other topology verb resolves against.
func TestRunCLI_CloneIntoFlagCreatesHubAtDirectory(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeCLICloneWarpBare(t, fixtures, "intoflag-warp")
	weftBare := makeCLICloneWeftBare(t, fixtures, "intoflag-weft")

	callerCwd := t.TempDir()
	dest := t.TempDir()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(callerCwd, &out, []string{
		"clone", "--into", dest,
		filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
	})
	if exitCode != 0 {
		t.Fatalf("RunCLIIn(clone --into) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	hubPath, _ := result["hub"].(string)
	if hubPath == "" {
		t.Fatalf("RunCLIIn(clone --into) output missing non-empty 'hub' key; got %v", result)
	}

	if rel, err := filepath.Rel(dest, hubPath); err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("hub %q was not created under --into destination %q", hubPath, dest)
	}
	if rel, err := filepath.Rel(callerCwd, hubPath); err == nil && !strings.HasPrefix(rel, "..") {
		t.Errorf("hub %q was created under the caller's cwd %q instead of --into destination %q", hubPath, callerCwd, dest)
	}
}

// TestRunCLI_CloneWithoutIntoFlagUsesResolvedCwd asserts that "fabric clone" with no --into flag
// still creates the hub at the resolved seam cwd — --into's default, which is otherwise untested now
// that the five clone-destination sites migrated onto --into with an explicit value.
func TestRunCLI_CloneWithoutIntoFlagUsesResolvedCwd(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeCLICloneWarpBare(t, fixtures, "nointoflag-warp")
	weftBare := makeCLICloneWeftBare(t, fixtures, "nointoflag-weft")

	cwd := t.TempDir()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(cwd, &out, []string{
		"clone", filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
	})
	if exitCode != 0 {
		t.Fatalf("RunCLIIn(clone) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := decodeResult(t, &out)
	hubPath, _ := result["hub"].(string)
	if hubPath == "" {
		t.Fatalf("RunCLIIn(clone) output missing non-empty 'hub' key; got %v", result)
	}

	rel, err := filepath.Rel(cwd, hubPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Errorf("hub %q was not created under the seam cwd %q when --into was omitted", hubPath, cwd)
	}
}
