// cleanupremove_integration_test.go drives `lym fabric cleanup`, `prune` and `remove` through the CLI.
// It covers the envelope and exit-code contract of `--remote`.
// It also covers the origin task-branch sweep, and `remove` through the pairteardown composite.
// Exit codes are observable only through the CLI seam, so these checks cannot live in fabricengine-level tests.
// The open-PR lookup is substituted through SetOpenPRHeadsForTest, so no network is involved.
// An archive/<slug>/* tag on the weft origin makes a task branch fabric-managed, and the weft orphan sweep keeps it.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// remoteEnvelopeCreateOrphanBranch creates branch in the weft repo at weftRoot, pointed at HEAD.
// It makes no worktree checkout, the shape Cleanup treats as an orphan candidate.
// The branch name also needs the "-weft" suffix WeftWarpSlug requires.
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

// landedManagedBranch pushes the prime's HEAD to the warp origin as branch, so all its work is on the default branch.
// It also gives branch an archive tag on the weft origin, which makes it fabric-managed.
func landedManagedBranch(t *testing.T, h *hubforge.Hub, branch string) {
	t.Helper()

	gitkit.MustRun(t, h.Location.WorktreePath(), "git", "push", "origin", "HEAD:refs/heads/"+branch)

	weftRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}
	tag := "archive/" + branch + "/tip"
	gitkit.MustRun(t, weftRoot, "git", "tag", tag, "HEAD")
	gitkit.MustRun(t, weftRoot, "git", "push", "origin", "refs/tags/"+tag)
}

// warpEntryOf returns the warp_entries element of env naming branch.
func warpEntryOf(t *testing.T, env envelope.Envelope, raw, branch string) map[string]any {
	t.Helper()

	entries, ok := env.Raw["warp_entries"].([]any)
	if !ok {
		t.Fatalf("envelope has no \"warp_entries\" array\noutput: %s", raw)
	}
	for _, e := range entries {
		if entry, isMap := e.(map[string]any); isMap && entry["branch"] == branch {
			return entry
		}
	}
	t.Fatalf("no warp_entries element for %q\noutput: %s", branch, raw)
	return nil
}

// TestRunCLI_CleanupRemoveScenario runs the cleanup, prune and remove checks over one hub.
// It stays serial (no t.Parallel) because the steps that stub the open-PR lookup swap a package-level variable.
// That variable is process-global state.
// Steps run serially in this order, each tolerating the branches earlier steps leave behind.
// The all-protected step runs first, because it needs a hub with no deletable branch.
// The steps that break or remove the weft origin run last, because every later push would fail.
// The pair the no-origin remove needs is added before the origin is broken, since `add` pushes.
func TestRunCLI_CleanupRemoveScenario(t *testing.T) {
	tiergate.Require(t, 1)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	weftRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}

	const noOriginSlug = "cli-remove-no-origin"

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"CleanupAllProtectedEntriesExitZero", func(t *testing.T) {
			// A cleanup run whose entries are all Protected still exits 0.
			// Here they are the primary weft branch, always present, and an unmanaged legacy branch.
			// The test asserts Protected with an empty Error: Cleanup never sets Error on a protected or unmanaged entry.
			// The branch has no "-weft" suffix, so it is unmanaged, reported but never deletable: WeftWarpSlug rejects it.
			const unmanagedBranch = "cli-cleanup-unmanaged-legacy"
			remoteEnvelopeCreateOrphanBranch(t, weftRoot, unmanagedBranch)

			code, output := runFabric(t, h.PrimeWorktree(), "cleanup", "--apply")
			if code != 0 {
				t.Fatalf("RunCLI(cleanup --apply) with only protected/unmanaged entries = %d; want 0\noutput: %s", code, output)
			}

			env := envelope.Decode(t, output)
			entries, ok := env.Raw["entries"].([]any)
			if !ok || len(entries) == 0 {
				t.Fatalf("envelope has no non-empty \"entries\" array\noutput: %s", output)
			}
			var sawProtected bool
			for _, raw := range entries {
				entry, isMap := raw.(map[string]any)
				if !isMap {
					continue
				}
				protected, _ := entry["protected"].(bool)
				if !protected {
					t.Fatalf("entry %v is not protected; want every entry protected in this fixture\noutput: %s", entry, output)
				}
				sawProtected = true
				if errStr, _ := entry["error"].(string); errStr != "" {
					t.Errorf("protected entry %v carries a non-empty \"error\"; want empty — Cleanup never sets Error on a protected entry", entry)
				}
			}
			if !sawProtected {
				t.Errorf("no entry reported at all\noutput: %s", output)
			}
		}},
		{"PruneProtectedOrUnownedEntryStillExitsZero", func(t *testing.T) {
			// A prune run with a Protected or Unowned entry still exits 0.
			// It is a regression guard: prune's carve-out for these entries must not change.
			// An ordinary operator directory, not a git checkout and never fabric's, with a name WeftWarpSlug accepts.
			// Prune's orphan pass enumerates it as Unowned.
			unowned := filepath.Join(h.Path, "cli-prune-unowned-weft")
			if err := os.MkdirAll(unowned, 0o755); err != nil {
				t.Fatalf("create unowned hub directory: %v", err)
			}

			code, output := runFabric(t, h.PrimeWorktree(), "prune")
			if code != 0 {
				t.Fatalf("RunCLI(prune) with an unowned entry = %d; want 0\noutput: %s", code, output)
			}

			env := envelope.RequireOK(t, output)
			entries, ok := env.Raw["entries"].([]any)
			if !ok {
				t.Fatalf("envelope has no \"entries\" array\noutput: %s", output)
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
				t.Errorf("no entry reported \"unowned\":true\noutput: %s", output)
			}
		}},
		{"RemoveRemoteSuccessEnvelopeCarriesRemoteBranchDeleted", func(t *testing.T) {
			// remove --remote's success envelope carries remote_branch_deleted.
			// It guards the hand-built fields map, which would otherwise drop a new field silently.
			// The struct still declares that field, so nothing else would notice.
			// It asserts the key's presence, not only its value, so a dropped key fails rather than reading as false.
			const slug = "cli-remove-remote"

			if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
				t.Fatalf("RunCLI(add %s) = %d; want 0\noutput: %s", slug, code, output)
			}

			code, output := runFabric(t, h.PrimeWorktree(), "remove", "--remote", slug)
			if code != 0 {
				t.Fatalf("RunCLI(remove --remote %s) = %d; want 0\noutput: %s", slug, code, output)
			}

			env := envelope.Decode(t, output)
			if _, present := env.Raw["remote_branch_deleted"]; !present {
				t.Errorf("envelope has no \"remote_branch_deleted\" key\noutput: %s", output)
			}
			if deleted, _ := env.Raw["remote_branch_deleted"].(bool); !deleted {
				t.Errorf("envelope remote_branch_deleted = %v; want true", env.Raw["remote_branch_deleted"])
			}
		}},
		{"RemoveFinishesHalfRemovedPair", func(t *testing.T) {
			const slug = "cli-half-removed"
			hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})

			gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(slug))

			code, output := runFabric(t, h.PrimeWorktree(), "remove", slug)
			if code != 0 {
				t.Fatalf("remove of a half-removed pair exited %d; want 0\noutput: %s", code, output)
			}
			env := envelope.Decode(t, output)
			if finished, _ := env.Raw["finished"].(bool); !finished {
				t.Errorf("finished = %v; want true\noutput: %s", env.Raw["finished"], output)
			}
			if _, present := env.Raw["session_ended"]; !present {
				t.Errorf("envelope has no \"session_ended\" key\noutput: %s", output)
			}
			if _, err := os.Stat(h.PairRecordsSibling(slug)); !os.IsNotExist(err) {
				t.Errorf("sibling worktree still present after finishing the pair: %v", err)
			}
		}},
		{"RemoveTaskSideDirtRefusesWithoutTouchingThePair", func(t *testing.T) {
			const slug = "cli-task-dirty"
			hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

			warpPath := h.PairCodeWorktree(slug)
			gitkit.CommitFile(t, warpPath, "tracked.md", "committed\n", "seed tracked file")
			if err := os.WriteFile(filepath.Join(warpPath, "tracked.md"), []byte("committed\nuncommitted\n"), 0o644); err != nil {
				t.Fatalf("dirty the task worktree: %v", err)
			}

			code, output := runFabric(t, h.PrimeWorktree(), "remove", slug)
			if code == 0 {
				t.Fatalf("remove of a task-side-dirty pair exited 0; output: %s", output)
			}
			envelope.RequireErr(t, output, "")
			for _, path := range []string{warpPath, h.PairRecordsSibling(slug)} {
				if _, err := os.Stat(path); err != nil {
					t.Errorf("%s was touched by a refused remove: %v", path, err)
				}
			}
		}},
		{"CleanupRemoteAloneWithoutApplyDeletesNothing", func(t *testing.T) {
			// --remote alone on cleanup, without --apply, deletes nothing on either side and exits 0.
			// It is the flag combination an operator most likely gets wrong, and the help text states it explicitly.
			branch := fabricengine.RecordsBranchName("cli-remote-no-apply")
			remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
			remoteEnvelopePushBranch(t, weftRoot, branch)

			code, output := runFabric(t, h.PrimeWorktree(), "cleanup", "--remote")
			if code != 0 {
				t.Fatalf("RunCLI(cleanup --remote) = %d; want 0\noutput: %s", code, output)
			}

			if !gitkit.BranchExists(t, weftRoot, branch) {
				t.Errorf("branch %q was removed locally by cleanup --remote with no --apply", branch)
			}
			if !gitkit.BranchExists(t, h.RecordsBare, branch) {
				t.Errorf("branch %q was removed on the remote by cleanup --remote with no --apply", branch)
			}
		}},
		{"CleanupRemoteSkipsTaskBranchesWhenGitHubUnreachable", func(t *testing.T) {
			// An unreachable GitHub skips every task-branch deletion while the weft sweep still runs.
			landedManagedBranch(t, h, "gone-landed-unreachable")

			orphan := fabricengine.RecordsBranchName("weft-orphan")
			remoteEnvelopeCreateOrphanBranch(t, weftRoot, orphan)
			remoteEnvelopePushBranch(t, weftRoot, orphan)

			fabriccli.SetOpenPRHeadsForTest(t, func(context.Context, string) (map[string]bool, error) {
				return nil, errors.New("no GitHub token")
			})

			code, output := runFabric(t, h.PrimeWorktree(), "cleanup", "--apply", "--remote")
			if code != 0 {
				t.Fatalf("RunCLI(cleanup --apply --remote) = %d; want 0\noutput: %s", code, output)
			}

			env := envelope.Decode(t, output)
			reason, _ := env.Raw["warp_skipped_reason"].(string)
			if !strings.Contains(reason, "no GitHub token") {
				t.Errorf("warp_skipped_reason = %q; want it to name the cause", reason)
			}
			if entries, ok := env.Raw["warp_entries"].([]any); !ok || len(entries) != 0 {
				t.Errorf("warp_entries = %v; want an empty array\noutput: %s", env.Raw["warp_entries"], output)
			}
			if !gitkit.BranchExists(t, h.CodeBare, "gone-landed-unreachable") {
				t.Errorf("task branch gone-landed-unreachable was deleted from origin although GitHub was unreachable")
			}
			if gitkit.BranchExists(t, h.RecordsBare, orphan) {
				t.Errorf("weft orphan %q survives on the weft origin; the weft sweep must still run", orphan)
			}
		}},
		{"CleanupRemoteReportsOpenPRBranchAndApplyDeletesOnlyTheCandidate", func(t *testing.T) {
			// A reachable GitHub reports an open-PR branch as kept, and --apply deletes only the landed candidate.
			landedManagedBranch(t, h, "gone-landed")
			landedManagedBranch(t, h, "has-pr")

			fabriccli.SetOpenPRHeadsForTest(t, func(context.Context, string) (map[string]bool, error) {
				return map[string]bool{"has-pr": true}, nil
			})

			dryCode, dry := runFabric(t, h.PrimeWorktree(), "cleanup", "--remote")
			if dryCode != 0 {
				t.Fatalf("RunCLI(cleanup --remote) = %d; want 0\noutput: %s", dryCode, dry)
			}
			dryEnv := envelope.Decode(t, dry)
			if e := warpEntryOf(t, dryEnv, dry, "has-pr"); e["candidate"] != false || !strings.Contains(e["reason"].(string), "open pull request") {
				t.Errorf("has-pr entry = %v; want a non-candidate with the open-PR reason", e)
			}
			if e := warpEntryOf(t, dryEnv, dry, "gone-landed"); e["candidate"] != true || e["deleted"] != false {
				t.Errorf("gone-landed dry-run entry = %v; want a candidate, not deleted", e)
			}
			if !gitkit.BranchExists(t, h.CodeBare, "gone-landed") {
				t.Fatalf("dry run deleted gone-landed from origin")
			}

			appliedCode, applied := runFabric(t, h.PrimeWorktree(), "cleanup", "--apply", "--remote")
			if appliedCode != 0 {
				t.Fatalf("RunCLI(cleanup --apply --remote) = %d; want 0\noutput: %s", appliedCode, applied)
			}
			appliedEnv := envelope.Decode(t, applied)
			if e := warpEntryOf(t, appliedEnv, applied, "gone-landed"); e["deleted"] != true {
				t.Errorf("gone-landed apply entry = %v; want deleted", e)
			}
			if gitkit.BranchExists(t, h.CodeBare, "gone-landed") {
				t.Errorf("gone-landed still on origin after --apply")
			}
			if !gitkit.BranchExists(t, h.CodeBare, "has-pr") {
				t.Errorf("has-pr was deleted from origin although a pull request is open")
			}
		}},
		{"CleanupLocalFailureExitsNonZero", func(t *testing.T) {
			// A cleanup run where one entry carries a local Error — the git branch -D itself failed — exits non-zero.
			// It pins the verdict for a local failure, which nothing else in the suite would notice flipping back.
			// Induced by pre-creating the branch's own ref lock file, so `git branch -D` cannot acquire the lock it needs.
			branch := fabricengine.RecordsBranchName("cli-cleanup-local-fail")
			remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)

			lockPath := filepath.Join(weftRoot, ".git", "refs", "heads", branch+".lock")
			if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
				t.Fatalf("write ref lock %s: %v", lockPath, err)
			}
			t.Cleanup(func() { os.Remove(lockPath) })

			code, output := runFabric(t, h.PrimeWorktree(), "cleanup", "--apply")
			if code != 1 {
				t.Fatalf("RunCLI(cleanup --apply) with a locked ref = %d; want 1\noutput: %s", code, output)
			}

			env := envelope.Decode(t, output)
			if env.OK {
				t.Errorf("envelope ok = true; want false\noutput: %s", output)
			}

			entries, ok := env.Raw["entries"].([]any)
			if !ok {
				t.Fatalf("envelope has no \"entries\" array\noutput: %s", output)
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
				t.Errorf("no entry for %q carries an \"error\"; want the local git branch -D failure reason\noutput: %s", branch, output)
			}
		}},
		{"AddPairForTheNoOriginRemove", func(t *testing.T) {
			// Prepares RemoveNoOriginUnderRemoteExitsZero: `add` pushes, so it must run while the weft origin still works.
			if code, output := runFabric(t, h.PrimeWorktree(), "add", noOriginSlug); code != 0 {
				t.Fatalf("RunCLI(add %s) = %d; want 0\noutput: %s", noOriginSlug, code, output)
			}
		}},
		{"CleanupRemoteFailureExitsNonZero", func(t *testing.T) {
			// A cleanup run whose weft origin is unreachable exits non-zero and emits "ok":false.
			// The envelope still carries the full entries array, with the failing branch's own reason.
			// The weft origin is pointed at a filesystem path that does not exist.
			// The remote steps run after the local delete, so the archive tag push fails.
			// The branch is already deleted locally under its local tag,
			// and the envelope reports "partial":true with the reason in the entry's error.
			// The envelope carries no "refusal" key, since a synthesised fmt.Errorf never matches `RefusalOf`.
			branch := fabricengine.RecordsBranchName("cli-cleanup-remote-fail")
			remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
			remoteEnvelopeBreakOrigin(t, weftRoot)

			code, output := runFabric(t, h.PrimeWorktree(), "cleanup", "--apply", "--remote")
			if code != 1 {
				t.Fatalf("RunCLI(cleanup --apply --remote) = %d; want 1\noutput: %s", code, output)
			}

			env := envelope.Decode(t, output)
			if env.OK {
				t.Errorf("envelope ok = true; want false\noutput: %s", output)
			}
			if env.Partial == nil || !*env.Partial {
				t.Errorf("envelope partial = %v; want true — the local delete precedes the failed remote archive\noutput: %s", env.Partial, output)
			}
			if _, present := env.Raw["refusal"]; present {
				t.Errorf("envelope carries a \"refusal\" key; want none — a synthesised error is never a gate refusal\noutput: %s", output)
			}

			entries, ok := env.Raw["entries"].([]any)
			if !ok || len(entries) == 0 {
				t.Fatalf("envelope has no non-empty \"entries\" array\noutput: %s", output)
			}
			var sawRemoteError bool
			for _, raw := range entries {
				entry, isMap := raw.(map[string]any)
				if !isMap {
					continue
				}
				if entryBranch, _ := entry["branch"].(string); entryBranch == branch {
					if reason, _ := entry["error"].(string); strings.Contains(reason, "archive") {
						sawRemoteError = true
					}
				}
			}
			if !sawRemoteError {
				t.Errorf("no entry carries an archive \"error\"; want the failing branch's own reason\noutput: %s", output)
			}
		}},
		{"CleanupNoOriginUnderApplyAndRemoteExitsZero", func(t *testing.T) {
			// The no-origin path from the CLI on both verbs: a weft repo with its remote removed exits 0.
			// The reason is in remote_skipped_reason, and for cleanup there is no entries[].remote_error.
			// Both halves are needed.
			// An asymmetric exit code for one configuration across the two verbs is the defect this test guards against.
			// Only a CLI-level test can observe an exit code at all.
			branch := fabricengine.RecordsBranchName("cli-cleanup-no-origin")
			remoteEnvelopeCreateOrphanBranch(t, weftRoot, branch)
			remoteEnvelopeRemoveOrigin(t, weftRoot)

			code, output := runFabric(t, h.PrimeWorktree(), "cleanup", "--apply", "--remote")
			if code != 0 {
				t.Fatalf("RunCLI(cleanup --apply --remote) with no origin = %d; want 0\noutput: %s", code, output)
			}

			env := envelope.Decode(t, output)
			reason, _ := env.Raw["remote_skipped_reason"].(string)
			if reason == "" {
				t.Errorf("envelope remote_skipped_reason is empty; want a reason naming the missing origin remote\noutput: %s", output)
			}

			entries, ok := env.Raw["entries"].([]any)
			if !ok {
				t.Fatalf("envelope has no \"entries\" array\noutput: %s", output)
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
		}},
		{"RemoveNoOriginUnderRemoteExitsZero", func(t *testing.T) {
			// The remove half of the no-origin contract, relying on the previous step to remove the weft origin.
			code, output := runFabric(t, h.PrimeWorktree(), "remove", "--remote", noOriginSlug)
			if code != 0 {
				t.Fatalf("RunCLI(remove --remote %s) with no origin = %d; want 0\noutput: %s", noOriginSlug, code, output)
			}

			env := envelope.Decode(t, output)
			reason, _ := env.Raw["remote_skipped_reason"].(string)
			if reason == "" {
				t.Errorf("envelope remote_skipped_reason is empty; want a reason naming the missing origin remote\noutput: %s", output)
			}
			if remoteErr, _ := env.Raw["remote_branch_error"].(string); remoteErr != "" {
				t.Errorf("envelope remote_branch_error = %q; want empty when the pre-check itself skipped", remoteErr)
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}
