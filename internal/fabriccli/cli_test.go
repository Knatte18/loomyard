//go:build tier1

// cli_test.go covers the fabric CLI's clone and reconcile-backfill verbs against local bare remotes, and holds the helpers the package's other integration files share: the cwd-fallback subprocess entry point, runFabric and gitOutputCLI.

package fabriccli_test

import (
	"bytes"
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
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// lymFabricCLISubprocessCwdEnv is the env var that gates this file's re-exec entry point below: when set, the process is not a normal `go test` run but a subprocess spawned by the cwd-fallback step of TestRunCLI_CleanHubScenario, deliberately standing in a target directory via exec.Command's Dir field so fabriccli.RunCLI's Getwd fallback can be observed for real, without any t.Chdir/os.Chdir call anywhere in this file — this file is on the cwdmutation guard's subject set in cmd/lym/cwdmutation_test.go and carries no exemption.
const lymFabricCLISubprocessCwdEnv = "LYM_FABRICCLI_SUBPROCESS_CWD"

// init intercepts the subprocess-mode re-exec before testing.Main ever runs, so the child process's stdout carries only fabriccli.RunCLI's own JSON output — never the "=== RUN"/"--- PASS" noise `go test` would otherwise interleave with it, which would defeat the byte-for-byte comparison that step performs against fabriccli.RunCLIIn's output.
func init() {
	if os.Getenv(lymFabricCLISubprocessCwdEnv) == "" {
		return
	}
	var out bytes.Buffer
	code := fabriccli.RunCLI(&out, []string{"pairs"})
	os.Stdout.Write(out.Bytes())
	os.Exit(code)
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
		"clone", "--shortname", "tst", "--into", cloneParent, "--subpath", "backend",
		filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
	})
	if exitCode != 0 {
		t.Fatalf("RunCLI(clone --subpath backend) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	result := envelope.RequireOK(t, out.String())
	hubPath, _ := result.Raw["hub"].(string)
	if hubPath == "" {
		t.Fatalf("RunCLI(clone) output missing non-empty 'hub' key; got %v", result)
	}
	if anchor, _ := result.Raw["anchor"].(string); anchor != "backend" {
		t.Errorf("RunCLI(clone) anchor = %q; want %q", anchor, "backend")
	}
	if warp, _ := result.Raw["warp"].(string); warp != filepath.ToSlash(warpBare) {
		t.Errorf("RunCLI(clone) warp = %q; want %q", warp, filepath.ToSlash(warpBare))
	}
	if recorded, ok := result.Raw["warp_binding_recorded"].(bool); !ok || !recorded {
		t.Errorf("RunCLI(clone) warp_binding_recorded = %v; want true", result.Raw["warp_binding_recorded"])
	}

	// The prime warp worktree's structural junctions (_lym and .lym) must be
	// wired: .lym is structural (structuralNeverCommittedDirs) and is wired by
	// every clone regardless of pathspec, so it is checked here rather than
	// an optional config-driven name, which a genuinely empty bare weft
	// clone (zero commits, no pre-existing weft:main fabric.yaml) never wires.
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

	// The repo-wide fabric.yaml and warp binding must be committed onto weft:main (the board worktree), not merely present on disk.
	// The .lym-anchor marker is tracked at the root of the prime's weft worktree and absent from the board.
	// This checks tracked-ness directly (git ls-files) rather than a blanket `git status --porcelain` cleanliness assertion:
	// CommitWeftAt/PushWeftAt bypass ensureWeftLockDir's exclude-seeding (by design — see recordsgit.go's CommitWeftAt doc comment),
	// so gitrepo's own .gitrepo-push.lock can legitimately surface as untracked dirt on the board worktree;
	// that is a pre-existing gap in boardengine.Sync's own identical CommitWeftAt/PushWeftAt pairing, not something this batch's clone orchestration introduces or is responsible for fixing.
	boardDir := fabricengine.BoardDir(hubPath)
	for _, relPath := range []string{
		filepath.Join(lymdirs.LymDirName, "config", "fabric.yaml"),
		fabricengine.WarpBindingFileName,
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

	// The board is seeded from nothing, so the clone marks it sparse in its own board commit, and the sparse hub files hold no template unit.
	markerRel := filepath.ToSlash(configengine.BaseSparseMarkerRel())
	if tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", markerRel)); tracked != markerRel {
		t.Errorf("board tracks %q for the base-sparse marker; want %q", tracked, markerRel)
	}
	const cloneCommit = "fabric clone: record repo-wide config"
	if added := strings.TrimSpace(gitOutputCLI(t, boardDir, "log", "--diff-filter=A", "--format=%s", "--", markerRel)); added != cloneCommit {
		t.Errorf("commit adding the base-sparse marker = %q; want %q", added, cloneCommit)
	}
	boardRel, err := filepath.Rel(hubPath, boardDir)
	if err != nil {
		t.Fatalf("Rel(%s, %s): %v", hubPath, boardDir, err)
	}
	wantTarget := filepath.ToSlash(boardRel) + "/" + markerRel
	markerRecorded := false
	mutations, _ := result.Raw["mutations"].([]any)
	for _, entry := range mutations {
		fields, _ := entry.(map[string]any)
		if fields["kind"] == string(fabricengine.KindFileWritten) && fields["target"] == wantTarget {
			markerRecorded = true
		}
	}
	if !markerRecorded {
		t.Errorf("mutation record holds no file_written entry for %q\nentries: %v", wantTarget, mutations)
	}
	if fabricYAML, err := os.ReadFile(configengine.ConfigFile(boardDir, "fabric")); err != nil || string(fabricYAML) != configengine.SparseBaseHeader {
		t.Errorf("board fabric.yaml = %q, %v; want the header alone %q", fabricYAML, err, configengine.SparseBaseHeader)
	}

	// A pre-commit hook rejecting the marker fails the board commit, and the clone then leaves no untracked marker for a later walk to mistake for a stripped board.
	// The test sets the process environment to point git's hook path at the hook, so it does not run in parallel.
	hooks := t.TempDir()
	rejectMarker := "#!/bin/sh\nif git diff --cached --name-only | grep -qx '" + markerRel + "'; then\n  echo 'pre-commit hook rejected the base-sparse marker' >&2\n  exit 1\nfi\n"
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(rejectMarker), 0o755); err != nil {
		t.Fatalf("write the pre-commit hook: %v", err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", filepath.ToSlash(hooks))

	rejectedFixtures := t.TempDir()
	rejectedParent := t.TempDir()
	var rejectedOut bytes.Buffer
	rejectedCode := fabriccli.RunCLI(&rejectedOut, []string{
		"clone", "--shortname", "tst", "--into", rejectedParent,
		filepath.ToSlash(makeCLICloneWeftBare(t, rejectedFixtures, "rejected-weft")), filepath.ToSlash(makeCLICloneWarpBare(t, rejectedFixtures, "rejected-warp")),
	})
	if rejectedCode == 0 {
		t.Fatalf("RunCLI(clone) with a hook rejecting the marker = 0; want non-zero\noutput: %s", rejectedOut.String())
	}
	envelope.RequireErr(t, rejectedOut.String(), "pre-commit")
	rejectedHubs, err := filepath.Glob(filepath.Join(rejectedParent, "*"+fabricengine.HubSuffix))
	if err != nil || len(rejectedHubs) != 1 {
		t.Fatalf("hubs under %s = %v, %v; want exactly one", rejectedParent, rejectedHubs, err)
	}
	rejectedMarker := configengine.BaseSparseMarker(fabricengine.BoardDir(rejectedHubs[0]))
	if _, err := os.Stat(rejectedMarker); !os.IsNotExist(err) {
		t.Errorf("base-sparse marker stat err = %v after the rejected board commit; want the file absent", err)
	}
}

// TestRunCLI_CloneOfAnUnmigratedWeftLeavesTheMarkerToTheWalk clones a weft whose board already holds a full base file and whose prime holds a full loom.yaml at a former template value, with no marker anywhere.
// The clone writes no marker of its own, so the walk it runs strips both once and commits the board marker in its own board commit.
func TestRunCLI_CloneOfAnUnmigratedWeftLeavesTheMarkerToTheWalk(t *testing.T) {
	t.Parallel()

	fixtures := t.TempDir()
	warpBare := makeCLICloneWarpBare(t, fixtures, "unmigrated-warp")
	weftBare := makeCLICloneWeftBare(t, fixtures, "unmigrated-weft")

	seed := t.TempDir()
	gitkit.MustRun(t, seed, "git", "init", "-b", "main")
	gitkit.MustRun(t, seed, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, seed, "git", "config", "user.name", "Test")
	gitkit.MustRun(t, seed, "git", "remote", "add", "origin", filepath.ToSlash(weftBare))
	formerLoom := strings.Replace(loomengine.ConfigTemplate(), "discussion_lenses: twill-discussion", `discussion_lenses: ""`, 1)
	if formerLoom == loomengine.ConfigTemplate() {
		t.Fatal("the loom template holds no discussion_lenses: twill-discussion line to take its former value")
	}
	gitkit.CommitFile(t, seed, lymcwd.AnchorFileName, "backend\n", "record the anchor")
	gitkit.CommitFile(t, seed, filepath.Join(lymdirs.LymDirName, "config", "fabric.yaml"), fabricengine.ConfigTemplate(), "full board fabric.yaml")
	gitkit.CommitFile(t, seed, filepath.Join("backend", lymdirs.LymDirName, "config", "loom.yaml"), formerLoom, "full prime loom.yaml")
	gitkit.MustRun(t, seed, "git", "push", "-u", "origin", "main")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{
		"clone", "--shortname", "tst", "--into", t.TempDir(),
		filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
	})
	if exitCode != 0 {
		t.Fatalf("RunCLI(clone) = %d; want 0\noutput: %s", exitCode, out.String())
	}
	hubPath, _ := envelope.RequireOK(t, out.String()).Raw["hub"].(string)

	boardDir := fabricengine.BoardDir(hubPath)
	markerRel := filepath.ToSlash(configengine.BaseSparseMarkerRel())
	if tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", markerRel)); tracked != markerRel {
		t.Errorf("board tracks %q for the base-sparse marker; want %q", tracked, markerRel)
	}
	const walkCommit = "lym: reconcile hub-wide config for build"
	if added := strings.TrimSpace(gitOutputCLI(t, boardDir, "log", "--diff-filter=A", "--format=%s", "--", markerRel)); !strings.HasPrefix(added, walkCommit) {
		t.Errorf("commit adding the base-sparse marker = %q; want one starting %q", added, walkCommit)
	}
	if fabricYAML, err := os.ReadFile(configengine.ConfigFile(boardDir, "fabric")); err != nil || string(fabricYAML) != configengine.SparseBaseHeader {
		t.Errorf("board fabric.yaml = %q, %v; want it stripped to the header %q", fabricYAML, err, configengine.SparseBaseHeader)
	}

	primeCwd := filepath.Join(hubPath, "unmigrated-warp", "backend")
	// The strip leaves a full file that differs from the template in nothing else as an absent sparse file.
	primeLoom, err := os.ReadFile(configengine.ConfigFile(primeCwd, "loom"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read the prime's loom.yaml: %v", err)
	}
	if strings.Contains(string(primeLoom), "discussion_lenses") {
		t.Errorf("prime loom.yaml still holds discussion_lenses after the walk; got %q", primeLoom)
	}
	primeLocation, err := lymcwd.Resolve(primeCwd)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", primeCwd, err)
	}
	config, err := loomengine.LoadConfig(fabricengine.ConfigDirs(primeLocation), "loom")
	if err != nil {
		t.Fatalf("loomengine.LoadConfig over the prime: %v", err)
	}
	if config.DiscussionLenses != "twill-discussion" {
		t.Errorf("DiscussionLenses = %q; want %q", config.DiscussionLenses, "twill-discussion")
	}
}

// TestRunCLI_CloneDestination asserts where "fabric clone" puts the hub and which anchor it echoes:
// with --into the hub lands under that directory and not under the seam cwd RunCLIIn was given, and without it the hub lands at the resolved seam cwd, --into's default.
// With no --subpath the anchor is the default root anchor ".".
func TestRunCLI_CloneDestination(t *testing.T) {
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
				t.Fatalf("RunCLIIn(%v) = %d; want 0\noutput: %s", args, code, output)
			}

			result := envelope.RequireOK(t, output)
			hubPath, _ := result.Raw["hub"].(string)
			if hubPath == "" {
				t.Fatalf("RunCLIIn(clone) output missing non-empty 'hub' key; got %v", result)
			}
			if anchor, _ := result.Raw["anchor"].(string); anchor != "." {
				t.Errorf("RunCLIIn(clone) anchor = %q; want %q", anchor, ".")
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

// TestRunCLI_CloneEngineRefusalReachesEnvelopeUnchanged drives a one-positional clone against a bare weft carrying no recorded binding, and requires the envelope's error to equal the engine's own refusal text exactly.
// The refusal's wording is owned by fabricengine's TestCloneHub_UnboundWeftNamesTwoArgForm.
func TestRunCLI_CloneEngineRefusalReachesEnvelopeUnchanged(t *testing.T) {
	fixtures := t.TempDir()
	weftBare := filepath.ToSlash(makeCLICloneWeftBare(t, fixtures, "clonecli-unbound-weft"))

	_, engineErr := fabricengine.CloneHub(t.TempDir(), fabricengine.CloneOptions{WeftURL: weftBare})
	if !errors.Is(engineErr, fabricengine.ErrNoWarpBinding) {
		t.Fatalf("CloneHub(%s) error = %v; want errors.Is ErrNoWarpBinding", weftBare, engineErr)
	}

	var out bytes.Buffer
	exitCode := fabriccli.RunCLI(&out, []string{"clone", "--into", t.TempDir(), weftBare})
	if exitCode != 1 {
		t.Fatalf("RunCLI(clone <weft-url>) = %d; want 1\noutput: %s", exitCode, out.String())
	}

	result := envelope.RequireErr(t, out.String(), "")
	if result.Error != engineErr.Error() {
		t.Errorf("RunCLI(clone <weft-url>) error = %q; want the engine's %q", result.Error, engineErr.Error())
	}
}

// runFabric runs the fabric CLI through RunCLIIn from cwd and returns the exit code and the raw output.
func runFabric(t *testing.T, cwd string, args ...string) (int, string) {
	t.Helper()

	var out bytes.Buffer
	code := fabriccli.RunCLIIn(cwd, &out, args)
	return code, out.String()
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
		"clone", "--shortname", "tst", "--into", cloneParent, filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
	})
	if exitCode != 0 {
		t.Fatalf("RunCLI(clone) = %d; want 0\noutput: %s", exitCode, cloneOut.String())
	}
	cloneResult := envelope.Decode(t, cloneOut.String())
	hubPath, _ := cloneResult.Raw["hub"].(string)
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

	result := envelope.RequireOK(t, reconcileOut.String())
	if binding, _ := result.Raw["warp_binding"].(string); binding != string(fabricengine.WarpBindingOutcomeRecorded) {
		t.Errorf("RunCLI(reconcile) warp_binding = %q; want %q", binding, fabricengine.WarpBindingOutcomeRecorded)
	}
	if _, present := result.Raw["pairs"]; !present {
		t.Errorf("RunCLI(reconcile) output missing 'pairs' key; want it present and unchanged in shape")
	}

	tracked := strings.TrimSpace(gitOutputCLI(t, boardDir, "ls-files", "--", fabricengine.WarpBindingFileName))
	if tracked == "" {
		t.Errorf("%s is not tracked on weft:main at %s after the reconcile backfill", fabricengine.WarpBindingFileName, boardDir)
	}
}

// TestRunCLI_ReconcileBoardFailureFailsTheVerb runs reconcile over a cloned hub whose board commit or push fails, at the hub-wide heal, the warp-binding record or both.
// Each row asserts a non-zero exit with the envelope kept:
// it still carries "pairs", the error names the first failure, and each side's detail names its own failure.
// The exit-code assertion is the point of this test.
func TestRunCLI_ReconcileBoardFailureFailsTheVerb(t *testing.T) {
	t.Parallel()

	// unreachableRemote points the board's origin at a path that does not exist, so every fetch and push fails.
	unreachableRemote := func(t *testing.T, boardDir, fixtures string) {
		unreachable := filepath.ToSlash(filepath.Join(fixtures, "does-not-exist.git"))
		gitkit.MustRun(t, boardDir, "git", "remote", "set-url", "origin", unreachable)
	}
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
		// staleHubConfig commits a retired key into the board's fabric config, so the heal rewrites it.
		staleHubConfig bool
		// unbind commits the warp-binding record's removal, so reconcile records it again.
		unbind               bool
		breakBoard           func(t *testing.T, boardDir, fixtures string)
		wantErr              string
		wantHubConfigDetail  string
		wantWarpBindingError string
	}{
		{
			name:                 "unreachable remote skips the heal and fails the recorded binding's push",
			unbind:               true,
			breakBoard:           unreachableRemote,
			wantErr:              "warp binding record failed",
			wantHubConfigDetail:  "fetching origin",
			wantWarpBindingError: "commit succeeded but push failed",
		},
		{
			name:                 "rejected push fails the healed config and the present binding",
			staleHubConfig:       true,
			breakBoard:           rejectPushes,
			wantErr:              "hub-wide config committed but push failed",
			wantHubConfigDetail:  "hub-wide config committed but push failed",
			wantWarpBindingError: "a previously committed warp binding record could not be pushed",
		},
		{
			name:                "rejected commit fails the healed config",
			staleHubConfig:      true,
			breakBoard:          rejectCommits,
			wantErr:             "pre-commit",
			wantHubConfigDetail: "pre-commit",
		},
		{
			name:                 "rejected commit fails the recorded binding",
			unbind:               true,
			breakBoard:           rejectCommits,
			wantErr:              "warp binding record failed",
			wantWarpBindingError: "pre-commit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixtures := t.TempDir()
			warpBare := makeCLICloneWarpBare(t, fixtures, "reconcilecli-fail-warp")
			weftBare := makeCLICloneWeftBare(t, fixtures, "reconcilecli-fail-weft")

			var cloneOut bytes.Buffer
			exitCode := fabriccli.RunCLI(&cloneOut, []string{
				"clone", "--shortname", "tst", "--into", t.TempDir(), filepath.ToSlash(weftBare), filepath.ToSlash(warpBare),
			})
			if exitCode != 0 {
				t.Fatalf("RunCLI(clone) = %d; want 0\noutput: %s", exitCode, cloneOut.String())
			}
			cloneResult := envelope.Decode(t, cloneOut.String())
			hubPath, _ := cloneResult.Raw["hub"].(string)
			if hubPath == "" {
				t.Fatalf("RunCLI(clone) output missing non-empty 'hub' key; got %v", cloneResult)
			}

			boardDir := fabricengine.BoardDir(hubPath)
			if tt.staleHubConfig {
				fabricConfig := configengine.ConfigFile(boardDir, "fabric")
				current, err := os.ReadFile(fabricConfig)
				if err != nil {
					t.Fatalf("read %s: %v", fabricConfig, err)
				}
				if err := os.WriteFile(fabricConfig, append(current, []byte("retired_fixture_key: 1\n")...), 0o644); err != nil {
					t.Fatalf("write %s: %v", fabricConfig, err)
				}
				gitkit.MustRun(t, boardDir, "git", "commit", "-am", "test fixture: stale hub-wide config")
			}
			if tt.unbind {
				gitkit.MustRun(t, boardDir, "git", "rm", fabricengine.WarpBindingFileName)
				gitkit.MustRun(t, boardDir, "git", "commit", "-m", "test fixture: unbind hub")
			}
			gitkit.MustRun(t, boardDir, "git", "push", "origin", "HEAD")
			tt.breakBoard(t, boardDir, fixtures)

			// The reconcile runs against a different directory than the clone above, passed per-call, so cwd is a per-call value and not a per-process one.
			var reconcileOut bytes.Buffer
			exitCode = fabriccli.RunCLIIn(filepath.Join(hubPath, "reconcilecli-fail-warp"), &reconcileOut, []string{"reconcile"})
			if exitCode == 0 {
				t.Fatalf("RunCLI(reconcile) = 0; want non-zero (a failed board commit or push fails the verb)\noutput: %s", reconcileOut.String())
			}

			result := envelope.RequireErr(t, reconcileOut.String(), tt.wantErr)
			if _, ok := result.Raw["pairs"]; !ok {
				t.Errorf("RunCLI(reconcile) envelope lacks \"pairs\"; want the envelope kept on failure")
			}
			hubConfigDetail, _ := result.Raw["hub_config_detail"].(string)
			if (tt.wantHubConfigDetail == "") != (hubConfigDetail == "") || !strings.Contains(hubConfigDetail, tt.wantHubConfigDetail) {
				t.Errorf("RunCLI(reconcile) hub_config_detail = %q; want it to contain %q", hubConfigDetail, tt.wantHubConfigDetail)
			}
			binding, _ := result.Raw["warp_binding"].(string)
			bindingDetail, _ := result.Raw["warp_binding_detail"].(string)
			if tt.wantWarpBindingError == "" {
				if binding == string(fabricengine.WarpBindingOutcomeRecordFailed) {
					t.Errorf("RunCLI(reconcile) warp_binding = %q, detail %q; want the binding not failed", binding, bindingDetail)
				}
				return
			}
			if binding != string(fabricengine.WarpBindingOutcomeRecordFailed) || !strings.Contains(bindingDetail, tt.wantWarpBindingError) {
				t.Errorf("RunCLI(reconcile) warp_binding = %q, detail %q; want %q naming %q", binding, bindingDetail, fabricengine.WarpBindingOutcomeRecordFailed, tt.wantWarpBindingError)
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
