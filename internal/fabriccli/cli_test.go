//go:build integration

// cli_test.go covers the fabric CLI's clone and reconcile-backfill verbs against local bare remotes,
// and holds the helpers the package's other integration files share: the cwd-fallback subprocess
// entry point, runFabric and gitOutputCLI.

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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// lyxFabricCLISubprocessCwdEnv is the env var that gates this file's re-exec entry point below: when
// set, the process is not a normal `go test` run but a subprocess spawned by
// the cwd-fallback step of TestRunCLI_CleanHubScenario, deliberately standing in a target directory via
// exec.Command's Dir field so fabriccli.RunCLI's Getwd fallback can be observed for real, without any
// t.Chdir/os.Chdir call anywhere in this file — this file is on the cwdmutation guard's subject set in
// cmd/lyx/cwdmutation_test.go and carries no exemption.
const lyxFabricCLISubprocessCwdEnv = "LYX_FABRICCLI_SUBPROCESS_CWD"

// init intercepts the subprocess-mode re-exec before testing.Main ever runs, so the child process's
// stdout carries only fabriccli.RunCLI's own JSON output — never the "=== RUN"/"--- PASS" noise `go
// test` would otherwise interleave with it, which would defeat the byte-for-byte comparison
// that step performs against fabriccli.RunCLIIn's output.
func init() {
	if os.Getenv(lyxFabricCLISubprocessCwdEnv) == "" {
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

	// Per-worktree module configs (e.g. "loom") must have been reconciled
	// on the weft side; hub-wide ones (e.g. "board") live at the board dir only.
	weftBase := filepath.Join(weftname.SiblingPath(hubPath, "clonecli-warp"), "backend")
	loomConfigPath := configengine.ConfigFile(weftBase, "loom")
	if _, err := os.Stat(loomConfigPath); err != nil {
		t.Errorf("per-worktree loom config missing at %s: %v", loomConfigPath, err)
	}
	if _, err := os.Stat(configengine.ConfigFile(weftBase, "board")); !os.IsNotExist(err) {
		t.Errorf("per-worktree board config materialized under %s; want it only at the board dir (stat err = %v)", weftBase, err)
	}
}

// TestRunCLI_CloneDestination asserts where "fabric clone" puts the hub and which anchor it echoes:
// with --into the hub lands under that directory and not under the seam cwd RunCLIIn was given, and
// without it the hub lands at the resolved seam cwd, --into's default.
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

	result := envelope.RequireOK(t, reconcileOut.String())
	if binding, _ := result.Raw["warp_binding"].(string); binding != string(fabricengine.WarpBindingOutcomeRecordFailed) {
		t.Errorf("RunCLI(reconcile) warp_binding = %q; want %q", binding, fabricengine.WarpBindingOutcomeRecordFailed)
	}
	if detail, _ := result.Raw["warp_binding_detail"].(string); detail == "" {
		t.Errorf("RunCLI(reconcile) warp_binding_detail is empty; want a non-empty push-failure message")
	}
}
