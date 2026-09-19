//go:build integration

// remoteenvelope_integration_test.go covers `--remote`'s CLI-level envelope and exit-code contract on
// `lyx fabric cleanup` and `lyx fabric remove`: the flag-matrix corner where --remote alone is still a
// dry run, the hand-built remove fields map actually carrying its new keys, cleanup's remote and local
// failure exit codes, the protected/unmanaged carve-outs staying exit-0, prune's untouched carve-out,
// and the no-origin skip reason exiting 0 symmetrically across both verbs. An exit code is observable
// only through the CLI seam, which is why none of these scenarios could be covered in batch 3's
// fabricengine-level tests.
//
// Package fabriccli_test, sharing the single TestMain in testmain_test.go and driving the real CLI
// through the same RunCLIIn seam envelopecontract_integration_test.go uses, with every hub built
// through hubforge.NewHub per the hubforge Fabric-Fixture Invariant.

package fabriccli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// remoteEnvelopeCreateOrphanBranch creates branch in the weft repo at weftRoot, pointed at HEAD, with
// no worktree checkout — the shape Cleanup treats as an orphan candidate once it also carries the
// "-weft" suffix WeftWarpSlug requires.
func remoteEnvelopeCreateOrphanBranch(t *testing.T, weftRoot, branch string) {
	t.Helper()
	gitkit.MustRun(t, weftRoot, "git", "branch", branch, "HEAD")
}

// remoteEnvelopePushBranch pushes branch from repoRoot to its configured origin remote.
func remoteEnvelopePushBranch(t *testing.T, repoRoot, branch string) {
	t.Helper()
	gitkit.MustRun(t, repoRoot, "git", "push", "origin", branch)
}

// remoteEnvelopeBreakOrigin points repoRoot's origin remote at a filesystem path that does not exist,
// so a push against it fails locally with no network involved.
func remoteEnvelopeBreakOrigin(t *testing.T, repoRoot string) {
	t.Helper()
	gitkit.MustRun(t, repoRoot, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-remote.git"))
}

// remoteEnvelopeRemoveOrigin removes repoRoot's origin remote entirely.
func remoteEnvelopeRemoveOrigin(t *testing.T, repoRoot string) {
	t.Helper()
	gitkit.MustRun(t, repoRoot, "git", "remote", "remove", "origin")
}

// remoteEnvelopeBranchExists reports whether branch exists at repoRoot.
func remoteEnvelopeBranchExists(t *testing.T, repoRoot, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}

// remoteEnvelopeDecode unmarshals out into a JSON envelope map, failing the test on decode error.
func remoteEnvelopeDecode(t *testing.T, out *bytes.Buffer) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v\noutput: %s", err, out.String())
	}
	return envelope
}

// TestRunCLI_CleanupRemoteAloneWithoutApplyDeletesNothing covers scenario 1: --remote alone on
// cleanup, without --apply, performs no deletion on either side and exits 0 — the flag-matrix corner
// an operator is most likely to get wrong, and the one the help text now promises explicitly.
func TestRunCLI_CleanupRemoteAloneWithoutApplyDeletesNothing(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}

	branch := fabricengine.WeftBranchName("cli-remote-no-apply")
	remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
	remoteEnvelopePushBranch(t, weftRoot, branch)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--remote"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(cleanup --remote) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	if !remoteEnvelopeBranchExists(t, weftRoot, branch) {
		t.Errorf("branch %q was removed locally by cleanup --remote with no --apply", branch)
	}
	if !remoteEnvelopeBranchExists(t, h.WeftBare, branch) {
		t.Errorf("branch %q was removed on the remote by cleanup --remote with no --apply", branch)
	}
}

// TestRunCLI_RemoveRemoteSuccessEnvelopeCarriesRemoteBranchDeleted covers scenario 2: remove
// --remote's success envelope carries remote_branch_deleted — the regression guard for the hand-built
// fields map, which would otherwise drop every new field silently while the struct still declared
// them. It asserts the key's presence, not only its value, so a dropped key fails rather than reading
// as false.
func TestRunCLI_RemoveRemoteSuccessEnvelopeCarriesRemoteBranchDeleted(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	const slug = "cli-remove-remote"

	var addOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &addOut, []string{"add", slug}); exitCode != 0 {
		t.Fatalf("RunCLI(add %s) = %d; want 0\noutput: %s", slug, exitCode, addOut.String())
	}

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", "--remote", slug})
	if exitCode != 0 {
		t.Fatalf("RunCLI(remove --remote %s) = %d; want 0\noutput: %s", slug, exitCode, out.String())
	}

	envelope := remoteEnvelopeDecode(t, &out)
	if _, present := envelope["remote_branch_deleted"]; !present {
		t.Errorf("envelope has no \"remote_branch_deleted\" key\noutput: %s", out.String())
	}
	if deleted, _ := envelope["remote_branch_deleted"].(bool); !deleted {
		t.Errorf("envelope remote_branch_deleted = %v; want true", envelope["remote_branch_deleted"])
	}
}

// TestRunCLI_CleanupRemoteFailureExitsNonZero covers scenario 3: a cleanup run where one entry
// carries a RemoteError exits non-zero, emits "ok":false and "partial":true, and still carries the
// full entries array with that entry's remote_error populated. Induced by pointing the weft repo's
// origin at a filesystem path that does not exist. Asserts the envelope carries no "refusal" key — a
// synthesised fmt.Errorf can never match RefusalOf.
func TestRunCLI_CleanupRemoteFailureExitsNonZero(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}

	branch := fabricengine.WeftBranchName("cli-cleanup-remote-fail")
	remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
	remoteEnvelopeBreakOrigin(t, weftRoot)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--apply", "--remote"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(cleanup --apply --remote) = %d; want 1\noutput: %s", exitCode, out.String())
	}

	envelope := remoteEnvelopeDecode(t, &out)
	if ok, _ := envelope["ok"].(bool); ok {
		t.Errorf("envelope ok = true; want false\noutput: %s", out.String())
	}
	if partial, _ := envelope["partial"].(bool); !partial {
		t.Errorf("envelope partial = %v; want true\noutput: %s", envelope["partial"], out.String())
	}
	if _, present := envelope["refusal"]; present {
		t.Errorf("envelope carries a \"refusal\" key; want none — a synthesised error is never a gate refusal\noutput: %s", out.String())
	}

	entries, ok := envelope["entries"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("envelope has no non-empty \"entries\" array\noutput: %s", out.String())
	}
	var sawRemoteError bool
	for _, raw := range entries {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		if entryBranch, _ := entry["branch"].(string); entryBranch == branch {
			if reason, _ := entry["remote_error"].(string); reason != "" {
				sawRemoteError = true
			}
		}
	}
	if !sawRemoteError {
		t.Errorf("no entry carries a \"remote_error\"; want the failing branch's own reason\noutput: %s", out.String())
	}
}

// TestRunCLI_CleanupAllProtectedEntriesExitZero covers scenario 4: a cleanup run whose entries are
// all Protected — here the primary weft branch (always present) plus an unmanaged legacy branch — and
// therefore carry no Error at all, still exits 0. Asserts Protected with an EMPTY Error: Cleanup never
// sets Error on a protected or unmanaged entry.
func TestRunCLI_CleanupAllProtectedEntriesExitZero(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}

	// No "-weft" suffix: unmanaged, reported but never deletable — WeftWarpSlug rejects it.
	const unmanagedBranch = "cli-cleanup-unmanaged-legacy"
	remoteEnvelopeCreateOrphanBranch(t, weftRoot, unmanagedBranch)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--apply"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(cleanup --apply) with only protected/unmanaged entries = %d; want 0\noutput: %s", exitCode, out.String())
	}

	envelope := remoteEnvelopeDecode(t, &out)
	entries, ok := envelope["entries"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("envelope has no non-empty \"entries\" array\noutput: %s", out.String())
	}
	var sawProtected bool
	for _, raw := range entries {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		protected, _ := entry["protected"].(bool)
		if !protected {
			t.Fatalf("entry %v is not protected; want every entry protected in this fixture\noutput: %s", entry, out.String())
		}
		sawProtected = true
		if errStr, _ := entry["error"].(string); errStr != "" {
			t.Errorf("protected entry %v carries a non-empty \"error\"; want empty — Cleanup never sets Error on a protected entry", entry)
		}
	}
	if !sawProtected {
		t.Errorf("no entry reported at all\noutput: %s", out.String())
	}
}

// TestRunCLI_CleanupLocalFailureExitsNonZero covers scenario 5: a cleanup run where one entry
// carries a local Error — the git branch -D itself failed — exits non-zero. This is the one
// existing-path change this task makes deliberately, and it needs its own pinned test because
// nothing else in the suite would notice the verdict flipping back. Induced by pre-creating the
// branch's own ref lock file, so `git branch -D` cannot acquire the lock it needs.
func TestRunCLI_CleanupLocalFailureExitsNonZero(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}

	branch := fabricengine.WeftBranchName("cli-cleanup-local-fail")
	remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)

	lockPath := filepath.Join(weftRoot, ".git", "refs", "heads", branch+".lock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("write ref lock %s: %v", lockPath, err)
	}
	t.Cleanup(func() { os.Remove(lockPath) })

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--apply"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(cleanup --apply) with a locked ref = %d; want 1\noutput: %s", exitCode, out.String())
	}

	envelope := remoteEnvelopeDecode(t, &out)
	if ok, _ := envelope["ok"].(bool); ok {
		t.Errorf("envelope ok = true; want false\noutput: %s", out.String())
	}

	entries, ok := envelope["entries"].([]any)
	if !ok {
		t.Fatalf("envelope has no \"entries\" array\noutput: %s", out.String())
	}
	var sawLocalError bool
	for _, raw := range entries {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		if b, _ := entry["branch"].(string); b == branch {
			if reason, _ := entry["error"].(string); reason != "" {
				sawLocalError = true
			}
		}
	}
	if !sawLocalError {
		t.Errorf("no entry for %q carries an \"error\"; want the local git branch -D failure reason\noutput: %s", branch, out.String())
	}
}

// TestRunCLI_PruneProtectedOrUnownedEntryStillExitsZero covers scenario 6: a prune run with a
// Protected or Unowned entry still exits 0 — the regression guard that prune's carve-out survived
// untouched while cleanup left it.
func TestRunCLI_PruneProtectedOrUnownedEntryStillExitsZero(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	// An ordinary operator directory that is not a git checkout at all and was never fabric's, named
	// so WeftWarpSlug accepts it and prune's orphan pass enumerates it as Unowned.
	unowned := filepath.Join(h.Path, "cli-prune-unowned-weft")
	if err := os.MkdirAll(unowned, 0o755); err != nil {
		t.Fatalf("create unowned hub directory: %v", err)
	}

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"prune"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(prune) with an unowned entry = %d; want 0\noutput: %s", exitCode, out.String())
	}

	envelope := remoteEnvelopeDecode(t, &out)
	if ok, _ := envelope["ok"].(bool); !ok {
		t.Errorf("envelope ok = false; want true\noutput: %s", out.String())
	}

	entries, ok := envelope["entries"].([]any)
	if !ok {
		t.Fatalf("envelope has no \"entries\" array\noutput: %s", out.String())
	}
	var sawUnowned bool
	for _, raw := range entries {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		if unownedFlag, _ := entry["unowned"].(bool); unownedFlag {
			sawUnowned = true
		}
	}
	if !sawUnowned {
		t.Errorf("no entry reported \"unowned\":true\noutput: %s", out.String())
	}
}

// TestRunCLI_CleanupNoOriginUnderApplyAndRemoteExitsZero and
// TestRunCLI_RemoveNoOriginUnderRemoteExitsZero together cover the no-origin path from the CLI on
// both verbs: a weft repo with its remote removed exits 0 with the reason in remote_skipped_reason
// and, for cleanup, no entries[].remote_error. Both halves are needed — an asymmetric exit code for
// one configuration state across the two verbs is the defect the
// remote-failure-non-fatal-in-engine-fatal-in-cli Shared Decision exists to prevent, and only a
// CLI-level test can observe an exit code at all.
func TestRunCLI_CleanupNoOriginUnderApplyAndRemoteExitsZero(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}

	branch := fabricengine.WeftBranchName("cli-cleanup-no-origin")
	remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
	remoteEnvelopeRemoveOrigin(t, weftRoot)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--apply", "--remote"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(cleanup --apply --remote) with no origin = %d; want 0\noutput: %s", exitCode, out.String())
	}

	envelope := remoteEnvelopeDecode(t, &out)
	reason, _ := envelope["remote_skipped_reason"].(string)
	if reason == "" {
		t.Errorf("envelope remote_skipped_reason is empty; want a reason naming the missing origin remote\noutput: %s", out.String())
	}

	entries, ok := envelope["entries"].([]any)
	if !ok {
		t.Fatalf("envelope has no \"entries\" array\noutput: %s", out.String())
	}
	for _, raw := range entries {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		if reason, _ := entry["remote_error"].(string); reason != "" {
			t.Errorf("entry %v carries a \"remote_error\"; want empty — the reason lives on the verb-level field, not here", entry)
		}
	}
}

// TestRunCLI_RemoveNoOriginUnderRemoteExitsZero is the second half described above, for remove.
func TestRunCLI_RemoveNoOriginUnderRemoteExitsZero(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}
	const slug = "cli-remove-no-origin"

	var addOut bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &addOut, []string{"add", slug}); exitCode != 0 {
		t.Fatalf("RunCLI(add %s) = %d; want 0\noutput: %s", slug, exitCode, addOut.String())
	}
	remoteEnvelopeRemoveOrigin(t, weftRoot)

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", "--remote", slug})
	if exitCode != 0 {
		t.Fatalf("RunCLI(remove --remote %s) with no origin = %d; want 0\noutput: %s", slug, exitCode, out.String())
	}

	envelope := remoteEnvelopeDecode(t, &out)
	reason, _ := envelope["remote_skipped_reason"].(string)
	if reason == "" {
		t.Errorf("envelope remote_skipped_reason is empty; want a reason naming the missing origin remote\noutput: %s", out.String())
	}
	if remoteErr, _ := envelope["remote_branch_error"].(string); remoteErr != "" {
		t.Errorf("envelope remote_branch_error = %q; want empty when the pre-check itself skipped", remoteErr)
	}
}
