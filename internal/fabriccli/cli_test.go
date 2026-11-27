// cli_test.go covers the fabric CLI's clone and reconcile-backfill verbs against local bare remotes.
// It also holds the helpers the package's other integration files share:
// the cwd-fallback subprocess entry point, runFabric, and gitOutputCLI.

package fabriccli_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// lymFabricCLISubprocessCwdEnv is the env var that gates the re-exec entry point in init.
// When it is set, the process is a subprocess that the cwd-fallback step of TestCommand_CleanHubScenario spawns.
// That step starts it with exec.Command's Dir field set to a target directory.
// This exercises the Getwd fallback of clihelp.Execute for real, without any t.Chdir or os.Chdir call.
// This file is in the cwdmutation guard's subject set (cmd/lym/cwdmutation_test.go) and has no exemption.
const lymFabricCLISubprocessCwdEnv = "LYM_FABRICCLI_SUBPROCESS_CWD"

// init intercepts the subprocess-mode re-exec before testing.Main runs.
// The child's stdout then carries only the JSON output of clihelp.Execute.
// Any "=== RUN" or "--- PASS" line from go test would break the byte-for-byte comparison.
// The cwd-fallback step makes that comparison against the output of clihelp.ExecuteIn.
func init() {
	if os.Getenv(lymFabricCLISubprocessCwdEnv) == "" {
		return
	}
	var out bytes.Buffer
	code := clihelp.Execute(fabriccli.Command(), &out, []string{"pairs"})
	os.Stdout.Write(out.Bytes())
	os.Exit(code)
}

// makeCLICloneWarpBare creates a bare warp remote at <dir>/<name>.git.
// It is seeded with a README and a committed "backend" subdirectory.
// A clone with --subpath backend anchors at that subdirectory.
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

// makeCLICloneWeftBare creates a bare weft remote at <dir>/<name>.git with no commits.
// That is the state of a new weft repo before its first clone.
// Cloning it through the CLI exercises the orphan Bolt checkout path.
func makeCLICloneWeftBare(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	gitkit.MustRun(t, dir, "git", "init", "--bare", "-b", "main", bare)
	return bare
}

// TestCommand_CloneEndToEnd drives "fabric clone --subpath backend <weft> <warp>" against a local two-repo fixture.
// It asserts the clone-does-everything contract:
// the JSON envelope, with the "warp" and "warp_binding_recorded" keys;
// the wired warp junctions;
// the anchor marker;
// repo-wide config and the warp-binding record committed onto weft:main;
// and per-worktree module config reconciliation.
// The test checks the CLI orchestration itself, not only the git-level clone that fabricengine.CloneHub performs.
func TestCommand_CloneEndToEnd(t *testing.T) {
	tiergate.Require(t, 1)
	fixtures := t.TempDir()
	warpBare := makeCLICloneWarpBare(t, fixtures, "clonecli-warp")
	weftBare := makeCLICloneWeftBare(t, fixtures, "clonecli-weft")

	cloneParent := t.TempDir()

	var out bytes.Buffer
	exitCode := clihelp.Execute(fabriccli.Command(), &out, []string{
		"clone", "--shortname", "tst", "--into", cloneParent, "--subpath", "backend",
		filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
	})
	if exitCode != 0 {
		t.Fatalf("Execute(clone --subpath backend) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := envelope.RequireOK(t, out.String())
	hubPath, _ := result.Raw["hub"].(string)
	if hubPath == "" {
		t.Fatalf("Execute(clone) output missing non-empty 'hub' key; got %v", result)
	}
	if anchor, _ := result.Raw["anchor"].(string); anchor != "backend" {
		t.Errorf("Execute(clone) anchor = %q; want %q", anchor, "backend")
	}
	if warp, _ := result.Raw["warp"].(string); warp != filepath.ToSlash(warpBare) {
		t.Errorf("Execute(clone) warp = %q; want %q", warp, filepath.ToSlash(warpBare))
	}
	if recorded, ok := result.Raw["warp_binding_recorded"].(bool); !ok || !recorded {
		t.Errorf("Execute(clone) warp_binding_recorded = %v; want true", result.Raw["warp_binding_recorded"])
	}

	// The prime warp worktree's structural junctions (_lym and .lym) must be wired.
	// .lym is structural (fabriclayout.StructuralNeverCommittedDirs), and every clone wires it regardless of pathspec.
	// So the check uses it, not an optional config-driven name.
	// An empty bare weft clone, with no commits and no fabric.yaml on weft:main, never wires such a name.
	primeCwd := filepath.Join(hubPath, "clonecli-warp", "backend")
	for _, name := range []string{lymdirs.LymDirName, lymdirs.DotLymDirName} {
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

	// The repo-wide fabric.yaml and warp binding must be committed onto weft:main, the board worktree.
	// Being present on disk is not enough.
	// The checks use git ls-files to test tracked-ness directly, not a blanket `git status --porcelain` check.
	// The .lym-anchor marker is tracked at the root of the prime's weft worktree and absent from the board.
	boardDir := fabriclayout.BoardDir(hubPath)
	for _, relPath := range []string{
		filepath.Join(lymdirs.LymDirName, "config", "fabric.yaml"),
		fabriclayout.WarpBindingFileName,
	} {
		tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", filepath.ToSlash(relPath)))
		if tracked == "" {
			t.Errorf("%s is not tracked on weft:main at %s", relPath, boardDir)
		}
	}
	primeWeft := weftname.SiblingPath(hubPath, "clonecli-warp")
	if tracked := strings.TrimSpace(gitOutputCLI(t, primeWeft, "ls-files", "--", lymcwd.AnchorFileName)); tracked != lymcwd.AnchorFileName {
		t.Errorf("prime weft tracks %q for the anchor marker; want %q", tracked, lymcwd.AnchorFileName)
	}
	if tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", lymcwd.AnchorFileName)); tracked != "" {
		t.Errorf("board tracks %q for the anchor marker; want none", tracked)
	}

	// A per-worktree module's repo default (e.g. "loom") must have been seeded on the board;
	// a hub-wide module's file (e.g. "board") has no per-worktree copy under the weft base.
	weftBase := filepath.Join(weftname.SiblingPath(hubPath, "clonecli-warp"), "backend")
	loomConfigPath := configengine.ConfigFile(boardDir, "loom")
	if _, err := os.Stat(loomConfigPath); err != nil {
		t.Errorf("loom repo default missing at %s: %v", loomConfigPath, err)
	}
	if _, err := os.Stat(configengine.ConfigFile(weftBase, "board")); !os.IsNotExist(err) {
		t.Errorf("per-worktree board config materialized under %s; want it only at the board dir (stat err = %v)", weftBase, err)
	}

	// The board is seeded from nothing, so its hub files are sparse and hold no template unit.
	boardRel, err := filepath.Rel(hubPath, boardDir)
	if err != nil {
		t.Fatalf("Rel(%s, %s): %v", hubPath, boardDir, err)
	}
	mutations, _ := result.Raw["mutations"].([]any)
	// The seed-only profiles file's template base is written and committed by the clone too.
	baseRel := filepath.ToSlash(configengine.TemplateBaseFileRel("profiles"))
	if tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", baseRel)); tracked != baseRel {
		t.Errorf("board tracks %q for the profiles template base; want %q", tracked, baseRel)
	}
	wantBaseTarget := filepath.ToSlash(boardRel) + "/" + baseRel
	baseRecorded := false
	for _, entry := range mutations {
		fields, _ := entry.(map[string]any)
		if fields["kind"] == string(fabriclayout.KindFileWritten) && fields["target"] == wantBaseTarget {
			baseRecorded = true
		}
	}
	if !baseRecorded {
		t.Errorf("mutation record holds no file_written entry for %q\nentries: %v", wantBaseTarget, mutations)
	}
	if fabricYAML, err := os.ReadFile(configengine.ConfigFile(boardDir, "fabric")); err != nil || string(fabricYAML) != configengine.SparseBaseHeader {
		t.Errorf("board fabric.yaml = %q, %v; want the header alone %q", fabricYAML, err, configengine.SparseBaseHeader)
	}
}

// TestCommand_CloneDestination asserts where "fabric clone" puts the hub and which anchor it echoes.
// With --into, the hub lands under that directory, not under the cwd given to clihelp.ExecuteIn.
// Without it, the hub lands at the resolved cwd, which is the --into default.
// With no --subpath, the anchor is the default root anchor ".".
func TestCommand_CloneDestination(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name     string
		useInto  bool
		shortKey string
	}{
		{name: "IntoFlagCreatesHubAtDirectory", useInto: true, shortKey: "intoflag"},
		{name: "WithoutIntoFlagUsesResolvedCwd", useInto: false, shortKey: "nointoflag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixtures := t.TempDir()
			warpBare := makeCLICloneWarpBare(t, fixtures, tt.shortKey+"-warp")
			weftBare := makeCLICloneWeftBare(t, fixtures, tt.shortKey+"-weft")

			callerCwd := t.TempDir()
			dest := t.TempDir()

			args := []string{"clone", "--shortname", "tst"}
			if tt.useInto {
				args = append(args, "--into", dest)
			}
			args = append(args, filepath.ToSlash(weftBare), filepath.ToSlash(warpBare))

			code, output := runFabric(t, callerCwd, args...)
			if code != 0 {
				t.Fatalf("ExecuteIn(%v) = %d; want 0\noutput: %s", args, code, output)
			}

			result := envelope.RequireOK(t, output)
			hubPath, _ := result.Raw["hub"].(string)
			if hubPath == "" {
				t.Fatalf("ExecuteIn(clone) output missing non-empty 'hub' key; got %v", result)
			}
			if anchor, _ := result.Raw["anchor"].(string); anchor != "." {
				t.Errorf("ExecuteIn(clone) anchor = %q; want %q", anchor, ".")
			}

			wantRoot, otherRoot := callerCwd, dest
			if tt.useInto {
				wantRoot, otherRoot = dest, callerCwd
			}
			if rel, err := filepath.Rel(wantRoot, hubPath); err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
				t.Errorf("hub %q was not created under %q", hubPath, wantRoot)
			}
			if rel, err := filepath.Rel(otherRoot, hubPath); err == nil && !strings.HasPrefix(rel, "..") {
				t.Errorf("hub %q was created under %q instead of %q", hubPath, otherRoot, wantRoot)
			}
		})
	}
}

// TestCommand_CloneEngineRefusalReachesEnvelopeUnchanged drives a one-positional clone against a bare weft.
// That weft carries no recorded binding.
// The test requires the envelope's error to equal the engine's refusal text exactly.
func TestCommand_CloneEngineRefusalReachesEnvelopeUnchanged(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	fixtures := t.TempDir()
	weftBare := filepath.ToSlash(makeCLICloneWeftBare(t, fixtures, "clonecli-unbound-weft"))

	_, engineErr := fabricengine.CloneHub(t.TempDir(), fabriclayout.CloneOptions{WeftURL: weftBare})
	if !errors.Is(engineErr, fabricengine.ErrNoWarpBinding) {
		t.Fatalf("CloneHub(%s) error = %v; want errors.Is ErrNoWarpBinding", weftBare, engineErr)
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(fabriccli.Command(), &out, []string{"clone", "--into", t.TempDir(), weftBare})
	if exitCode != 1 {
		t.Fatalf("Execute(clone <weft-url>) = %d; want 1\noutput: %s", exitCode, out.String())
	}

	result := envelope.RequireErr(t, out.String(), "")
	if result.Error != engineErr.Error() {
		t.Errorf("Execute(clone <weft-url>) error = %q; want the engine's %q", result.Error, engineErr.Error())
	}
}

// runFabric runs the fabric CLI through clihelp.ExecuteIn from cwd and returns the exit code and the raw output.
func runFabric(t *testing.T, cwd string, args ...string) (int, string) {
	t.Helper()

	var out bytes.Buffer
	code := clihelp.ExecuteIn(fabriccli.Command(), cwd, &out, args)
	return code, out.String()
}

// gitOutputCLI runs a git command in dir and returns its stdout.
// It fails the test on any error.
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

// TestCommand_ReconcileLeavesAMissingWarpBindingUnrecorded drives the fabric command.
// It runs the two-positional clone and reconcile.
// The test builds the hub, deletes the recorded binding with plain git, and commits that deletion.
// The board is already clean beforehand.
// It then asserts that reconcile exits 0, reports "skipped", and leaves the binding file untracked.
// It also asserts that the hub-wide heal commits the deleted profiles template base again.
// The "pairs" data is still present and unchanged in shape.
//
//testtiming:keep the skipped warp_binding, the binding file left absent and the profiles template base tracked again after a reconcile; coverage of its blocks by other tests does not show an assertion of this
func TestCommand_ReconcileLeavesAMissingWarpBindingUnrecorded(t *testing.T) {
	tiergate.Require(t, 1)
	fixtures := t.TempDir()
	warpBare := makeCLICloneWarpBare(t, fixtures, "reconcilecli-warp")
	weftBare := makeCLICloneWeftBare(t, fixtures, "reconcilecli-weft")

	cloneParent := t.TempDir()

	var cloneOut bytes.Buffer
	// The clone runs without --force-bootstrap, because makeCLICloneWeftBare's fixture has no commits.
	// That is the unborn-HEAD case the weft-candidate guard admits on its own.
	exitCode := clihelp.Execute(fabriccli.Command(), &cloneOut, []string{
		"clone", "--shortname", "tst", "--into", cloneParent, filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
	})
	if exitCode != 0 {
		t.Fatalf("Execute(clone) = %d; want 0\noutput: %s", exitCode, cloneOut.String())
	}
	cloneResult := envelope.Decode(t, cloneOut.String())
	hubPath, _ := cloneResult.Raw["hub"].(string)
	if hubPath == "" {
		t.Fatalf("Execute(clone) output missing non-empty 'hub' key; got %v", cloneResult)
	}

	boardDir := fabriclayout.BoardDir(hubPath)
	baseRel := filepath.ToSlash(configengine.TemplateBaseFileRel("profiles"))
	gitkit.MustRun(t, boardDir, "git", "rm", fabriclayout.WarpBindingFileName, baseRel)
	gitkit.MustRun(t, boardDir, "git", "commit", "-m", "test fixture: unbind hub and drop the profiles template base")

	// cwd is a per-call value, passed here rather than hoisted to a shared variable.
	// This reconcile runs against a different directory than the clone above.
	var reconcileOut bytes.Buffer
	exitCode = clihelp.ExecuteIn(fabriccli.Command(), filepath.Join(hubPath, "reconcilecli-warp"), &reconcileOut, []string{"reconcile"})
	if exitCode != 0 {
		t.Fatalf("Execute(reconcile) = %d; want 0\noutput: %s", exitCode, reconcileOut.String())
	}

	result := envelope.RequireOK(t, reconcileOut.String())
	if binding, _ := result.Raw["warp_binding"].(string); binding != string(fabriclayout.WarpBindingOutcomeSkipped) {
		t.Errorf("Execute(reconcile) warp_binding = %q; want %q", binding, fabriclayout.WarpBindingOutcomeSkipped)
	}
	if detail, _ := result.Raw["warp_binding_detail"].(string); detail == "" {
		t.Errorf("Execute(reconcile) output holds no warp_binding_detail; want the detail of the unrecorded binding")
	}
	if _, present := result.Raw["pairs"]; !present {
		t.Errorf("Execute(reconcile) output missing 'pairs' key; want it present and unchanged in shape")
	}

	if tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", fabriclayout.WarpBindingFileName)); tracked != "" {
		t.Errorf("%s is tracked on weft:main at %s after the reconcile; want it left unrecorded", fabriclayout.WarpBindingFileName, boardDir)
	}
	if tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", baseRel)); tracked != baseRel {
		t.Errorf("board tracks %q for the profiles template base after the reconcile; want %q committed again", tracked, baseRel)
	}
}

// TestCommand_ReconcileBoardFailureFailsTheVerb runs reconcile over a cloned hub whose board commit or push fails.
// The failure is set at the hub-wide heal.
// Each row asserts a non-zero exit with the envelope kept:
// it still carries "pairs", the error names the failure, and the hub-wide detail names it too.
// The exit-code assertion is the point of this test.
func TestCommand_ReconcileBoardFailureFailsTheVerb(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// rejectPushes installs a pre-receive hook in the board's origin that rejects every push, while fetches still succeed.
	rejectPushes := func(t *testing.T, boardDir, _ string) {
		origin := strings.TrimSpace(gitOutputCLI(t, boardDir, "remote", "get-url", "origin"))
		writeFailingHook(t, filepath.Join(filepath.FromSlash(origin), "hooks"), "pre-receive")
	}
	// rejectCommits installs a pre-commit hook in the board's repository that rejects every commit.
	rejectCommits := func(t *testing.T, boardDir, _ string) {
		hooks := strings.TrimSpace(gitOutputCLI(t, boardDir, "rev-parse", "--path-format=absolute", "--git-path", "hooks"))
		writeFailingHook(t, hooks, "pre-commit")
	}

	tests := []struct {
		name string
		// staleHubConfig commits the deletion of the board's landing config, so the heal seeds it again.
		staleHubConfig      bool
		breakBoard          func(t *testing.T, boardDir, fixtures string)
		wantErr             string
		wantHubConfigDetail string
	}{
		{
			name:                "rejected push fails the healed config",
			staleHubConfig:      true,
			breakBoard:          rejectPushes,
			wantErr:             "hub-wide config committed but push failed",
			wantHubConfigDetail: "hub-wide config committed but push failed",
		},
		{
			name:                "rejected commit fails the healed config",
			staleHubConfig:      true,
			breakBoard:          rejectCommits,
			wantErr:             "pre-commit",
			wantHubConfigDetail: "pre-commit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixtures := t.TempDir()
			warpBare := makeCLICloneWarpBare(t, fixtures, "reconcilecli-fail-warp")
			weftBare := makeCLICloneWeftBare(t, fixtures, "reconcilecli-fail-weft")

			var cloneOut bytes.Buffer
			exitCode := clihelp.Execute(fabriccli.Command(), &cloneOut, []string{
				"clone", "--shortname", "tst", "--into", t.TempDir(), filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
			})
			if exitCode != 0 {
				t.Fatalf("Execute(clone) = %d; want 0\noutput: %s", exitCode, cloneOut.String())
			}
			cloneResult := envelope.Decode(t, cloneOut.String())
			hubPath, _ := cloneResult.Raw["hub"].(string)
			if hubPath == "" {
				t.Fatalf("Execute(clone) output missing non-empty 'hub' key; got %v", cloneResult)
			}

			boardDir := fabriclayout.BoardDir(hubPath)
			if tt.staleHubConfig {
				landingConfig := configengine.ConfigFile(boardDir, "landing")
				if err := os.Remove(landingConfig); err != nil {
					t.Fatalf("remove %s: %v", landingConfig, err)
				}
				gitkit.MustRun(t, boardDir, "git", "commit", "-am", "test fixture: hub-wide config deleted")
			}
			gitkit.MustRun(t, boardDir, "git", "push", "origin", "HEAD")
			tt.breakBoard(t, boardDir, fixtures)

			// cwd is passed per call, so this reconcile runs against a different directory than the clone above.
			var reconcileOut bytes.Buffer
			exitCode = clihelp.ExecuteIn(fabriccli.Command(), filepath.Join(hubPath, "reconcilecli-fail-warp"), &reconcileOut, []string{"reconcile"})
			if exitCode == 0 {
				t.Fatalf("Execute(reconcile) = 0; want non-zero (a failed board commit or push fails the verb)\noutput: %s", reconcileOut.String())
			}

			result := envelope.RequireErr(t, reconcileOut.String(), tt.wantErr)
			if _, ok := result.Raw["pairs"]; !ok {
				t.Errorf("Execute(reconcile) envelope lacks \"pairs\"; want the envelope kept on failure")
			}
			hubConfigDetail, _ := result.Raw["hub_config_detail"].(string)
			if (tt.wantHubConfigDetail == "") != (hubConfigDetail == "") || !strings.Contains(hubConfigDetail, tt.wantHubConfigDetail) {
				t.Errorf("Execute(reconcile) hub_config_detail = %q; want it to contain %q", hubConfigDetail, tt.wantHubConfigDetail)
			}
		})
	}
}

// writeFailingHook writes a git hook named name into hooksDir that exits non-zero.
func writeFailingHook(t *testing.T, hooksDir, name string) {
	t.Helper()

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", hooksDir, err)
	}
	hook := filepath.Join(hooksDir, name)
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho \"rejected by the test's "+name+" hook\" >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write %s: %v", hook, err)
	}
}
