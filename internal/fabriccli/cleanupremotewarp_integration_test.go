//go:build integration

// cleanupremotewarp_integration_test.go covers `lyx fabric cleanup --remote`'s origin task-branch sweep through the CLI:
// an unreachable GitHub skips every task-branch deletion while the weft sweep still runs,
// and a reachable one reports an open-PR branch as kept while --apply deletes only the landed candidate.
//
// The open-PR lookup is substituted through SetOpenPRHeadsForTest, so no network is involved.
// A task branch is made fabric-managed by an archive/<slug>/* tag on the weft origin, which, unlike a weft branch, the weft orphan sweep does not delete.
// Package fabriccli_test; shares the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// landedManagedBranch pushes the prime's HEAD to the warp origin as branch, so all its work is on the default branch,
// and gives it an archive tag on the weft origin, which makes it fabric-managed.
func landedManagedBranch(t *testing.T, h *hubforge.Hub, branch string) {
	t.Helper()

	gitkit.MustRun(t, h.Location.WorktreePath(), "git", "push", "origin", "HEAD:refs/heads/"+branch)

	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
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

func TestRunCLI_CleanupRemoteSkipsTaskBranchesWhenGitHubUnreachable(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	landedManagedBranch(t, h, "gone-landed")

	weftRoot, err := fabricengine.WeftRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("WeftRepoRoot: %v", err)
	}
	orphan := fabricengine.WeftBranchName("weft-orphan")
	remoteEnvelopeCreateOrphanBranch(t, weftRoot, orphan)
	remoteEnvelopePushBranch(t, weftRoot, orphan)

	fabriccli.SetOpenPRHeadsForTest(t, func(context.Context, string) (map[string]bool, error) {
		return nil, errors.New("no GitHub token")
	})

	var out bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"cleanup", "--apply", "--remote"}); exitCode != 0 {
		t.Fatalf("RunCLI(cleanup --apply --remote) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	env := envelope.Decode(t, out.String())
	reason, _ := env.Raw["warp_skipped_reason"].(string)
	if !strings.Contains(reason, "no GitHub token") {
		t.Errorf("warp_skipped_reason = %q; want it to name the cause", reason)
	}
	if entries, ok := env.Raw["warp_entries"].([]any); !ok || len(entries) != 0 {
		t.Errorf("warp_entries = %v; want an empty array\noutput: %s", env.Raw["warp_entries"], out.String())
	}
	if !gitkit.BranchExists(t, h.WarpBare, "gone-landed") {
		t.Errorf("task branch gone-landed was deleted from origin although GitHub was unreachable")
	}
	if gitkit.BranchExists(t, h.WeftBare, orphan) {
		t.Errorf("weft orphan %q survives on the weft origin; the weft sweep must still run", orphan)
	}
}

func TestRunCLI_CleanupRemoteReportsOpenPRBranchAndApplyDeletesOnlyTheCandidate(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	landedManagedBranch(t, h, "gone-landed")
	landedManagedBranch(t, h, "has-pr")

	fabriccli.SetOpenPRHeadsForTest(t, func(context.Context, string) (map[string]bool, error) {
		return map[string]bool{"has-pr": true}, nil
	})

	var dry bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &dry, []string{"cleanup", "--remote"}); exitCode != 0 {
		t.Fatalf("RunCLI(cleanup --remote) = %d; want 0\noutput: %s", exitCode, dry.String())
	}
	dryEnv := envelope.Decode(t, dry.String())
	if e := warpEntryOf(t, dryEnv, dry.String(), "has-pr"); e["candidate"] != false || !strings.Contains(e["reason"].(string), "open pull request") {
		t.Errorf("has-pr entry = %v; want a non-candidate with the open-PR reason", e)
	}
	if e := warpEntryOf(t, dryEnv, dry.String(), "gone-landed"); e["candidate"] != true || e["deleted"] != false {
		t.Errorf("gone-landed dry-run entry = %v; want a candidate, not deleted", e)
	}
	if !gitkit.BranchExists(t, h.WarpBare, "gone-landed") {
		t.Fatalf("dry run deleted gone-landed from origin")
	}

	var applied bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &applied, []string{"cleanup", "--apply", "--remote"}); exitCode != 0 {
		t.Fatalf("RunCLI(cleanup --apply --remote) = %d; want 0\noutput: %s", exitCode, applied.String())
	}
	appliedEnv := envelope.Decode(t, applied.String())
	if e := warpEntryOf(t, appliedEnv, applied.String(), "gone-landed"); e["deleted"] != true {
		t.Errorf("gone-landed apply entry = %v; want deleted", e)
	}
	if gitkit.BranchExists(t, h.WarpBare, "gone-landed") {
		t.Errorf("gone-landed still on origin after --apply")
	}
	if !gitkit.BranchExists(t, h.WarpBare, "has-pr") {
		t.Errorf("has-pr was deleted from origin although a pull request is open")
	}
}
