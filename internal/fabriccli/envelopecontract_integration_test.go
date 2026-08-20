//go:build integration

// envelopecontract_integration_test.go covers three envelope properties a fabric verb's JSON output
// must hold against real git, all of them found by driving the real CLI in R2's crucible round: a
// verb must not report success for work it failed to do, a verb must not report FAILURE for work a
// concurrent teardown removed out from under it, and a verb's array key must never be null.
// The first two are one rule seen from both sides and must always be read together.
//
// The first is `lyx fabric reconcile`: a pair it was asked to repair and could not must reach the
// caller as a FAILURE.
//
// Before R2's fix, runReconcile always exited through okWithRecord, so a reconcile that failed to
// re-point a junction still printed "ok":true with "partial":false and exited 0 — while carrying the
// real reason in pairs[].error, where a caller checking $? or reading "ok" never looks. Every scripted
// consumer (mill, an agent, a CI step) therefore read an unqualified success for a repair that did
// not happen.
//
// The failing state is induced through adoption's ONE remaining hard refusal — a warp-side real .lyx
// holding an entry that collides with a non-directory at the weft target — because that refusal is
// deterministic, needs no racing, and is the shape adoption deliberately still refuses after R2 made
// the dir/dir case merge.
//
// The second is `lyx fabric prune`, whose "entries" key rendered as null on a hub with nothing
// stale while every sibling verb emitted a real array in the same state.
//
// Package fabriccli_test, sharing the single TestMain in testmain_test.go, and building its hub via
// hubforge.NewHub like every other fabric hub fixture in the repo.

package fabriccli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestRunCLI_ReconcileReportsAFailedPairAsAFailure asserts the full envelope contract on the
// pair-failure path: exit 1, "ok":false, a non-empty "error", and — the half a bare non-zero exit
// would lose — the per-pair "pairs" report still present, so a caller learns WHICH pair failed.
func TestRunCLI_ReconcileReportsAFailedPairAsAFailure(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	l := h.Location
	warpWorktree := l.WorktreePath()
	warpDotLyx := filepath.Join(warpWorktree, l.AnchorRel, lyxdirs.DotLyxDirName)
	weftDotLyx := filepath.Join(h.PrimeWeft(), l.AnchorRel, lyxdirs.DotLyxDirName)

	// Tear the wired junction down to a real directory, then plant a collision adoption still
	// refuses: the same name is a FILE on the warp side and a DIRECTORY on the weft side.
	if err := fslink.Remove(warpDotLyx); err != nil {
		t.Fatalf("remove wired .lyx junction: %v", err)
	}
	if err := os.MkdirAll(warpDotLyx, 0o755); err != nil {
		t.Fatalf("mkdir real warp .lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(warpDotLyx, "logs"), []byte("a file, not a directory"), 0o644); err != nil {
		t.Fatalf("seed warp-side colliding file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(weftDotLyx, "logs"), 0o755); err != nil {
		t.Fatalf("mkdir weft-side colliding directory: %v", err)
	}

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(warpWorktree, &out, []string{"reconcile"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(reconcile) with an unrepairable pair = %d; want 1\noutput: %s", exitCode, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode reconcile envelope: %v\noutput: %s", err, out.String())
	}

	if ok, _ := envelope["ok"].(bool); ok {
		t.Errorf("envelope ok = true; want false when a pair failed to repair\noutput: %s", out.String())
	}
	if msg, _ := envelope["error"].(string); msg == "" {
		t.Errorf("envelope carries no \"error\" string; want one naming the failed repair\noutput: %s", out.String())
	}

	// The per-pair report must survive the failure path, and the failing pair must still carry its
	// own reason — an exit code alone tells a caller nothing about which pair to go fix.
	pairs, ok := envelope["pairs"].([]any)
	if !ok || len(pairs) == 0 {
		t.Fatalf("envelope has no non-empty \"pairs\" array on the failure path\noutput: %s", out.String())
	}
	var sawPairError bool
	for _, raw := range pairs {
		pair, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		if reason, _ := pair["error"].(string); reason != "" {
			sawPairError = true
		}
	}
	if !sawPairError {
		t.Errorf("no pair in the envelope carries an \"error\"; want the failing pair's own reason\noutput: %s", out.String())
	}

	// The fixed key set holds on this path too.
	if _, present := envelope["mutations"]; !present {
		t.Errorf("envelope is missing the always-present \"mutations\" key\noutput: %s", out.String())
	}
	if _, present := envelope["partial"]; !present {
		t.Errorf("envelope is missing the always-present \"partial\" key\noutput: %s", out.String())
	}
}

// TestRunCLI_PruneEmitsAnEmptyArrayNotNull pins the never-null rule for `prune`'s own array key.
//
// PruneResult.Entries was left to append from a nil slice, so a hub with nothing stale emitted
// "entries":null while `cleanup` and `reconcile` both emitted a real array in the same
// nothing-to-report state — making prune the one fabric verb whose array a consumer had to
// special-case against null, the exact asymmetry envelope.go's rule for "mutations" exists to
// prevent.
func TestRunCLI_PruneEmitsAnEmptyArrayNotNull(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	if exitCode := fabriccli.RunCLIIn(h.Location.WorktreePath(), &out, []string{"prune"}); exitCode != 0 {
		t.Fatalf("RunCLI(prune) = %d; want 0\noutput: %s", exitCode, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode prune envelope: %v\noutput: %s", err, out.String())
	}

	raw, present := envelope["entries"]
	if !present {
		t.Fatalf("prune envelope has no \"entries\" key\noutput: %s", out.String())
	}
	if raw == nil {
		t.Fatalf("prune envelope has \"entries\":null; want an empty array\noutput: %s", out.String())
	}
	entries, isArray := raw.([]any)
	if !isArray {
		t.Fatalf("prune \"entries\" is %T; want a JSON array\noutput: %s", raw, out.String())
	}
	if len(entries) != 0 {
		t.Errorf("prune \"entries\" has %d element(s) on a hub with nothing stale; want 0", len(entries))
	}
}

// TestRunCLI_ReconcileDoesNotFailOnAPairThatVanishedMidWalk is the counter-assertion to
// TestRunCLI_ReconcileReportsAFailedPairAsAFailure, and the two must be read together.
//
// Once a per-pair error drives reconcile's exit code, the verb becomes sensitive to a race it cannot
// avoid: `git worktree list` is read once, before the per-pair loop, so a concurrent
// `lyx fabric remove`/`prune` can delete a pair between the enumeration and the iteration that
// reaches it. Without the vanished-mid-walk verdict, that ordinary teardown would fail every
// enclosing reconcile — turning an honest exit code into a noisy one and, worse, training a caller to
// ignore it.
//
// Deleting the directory while leaving git's registration in place is exactly the state the race
// produces, and needs no actual concurrency to construct, so this stays deterministic.
func TestRunCLI_ReconcileDoesNotFailOnAPairThatVanishedMidWalk(t *testing.T) {
	const slug = "vanished-cli"
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, slug)

	if err := os.RemoveAll(h.PairWarpWorktree(slug)); err != nil {
		t.Fatalf("remove warp worktree directory (leaving git's registration behind): %v", err)
	}

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.Location.WorktreePath(), &out, []string{"reconcile"})
	if exitCode != 0 {
		t.Fatalf("RunCLI(reconcile) with a pair that vanished mid-walk = %d; want 0 — a concurrent teardown is not a reconcile failure\noutput: %s", exitCode, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode reconcile envelope: %v\noutput: %s", err, out.String())
	}
	if ok, _ := envelope["ok"].(bool); !ok {
		t.Errorf("envelope ok = false; want true\noutput: %s", out.String())
	}

	pairs, isArray := envelope["pairs"].([]any)
	if !isArray {
		t.Fatalf("envelope has no \"pairs\" array\noutput: %s", out.String())
	}
	var sawVanished bool
	for _, raw := range pairs {
		pair, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		if action, _ := pair["action"].(string); action == string(fabricengine.ReconcileActionVanishedMidWalk) {
			sawVanished = true
			if reason, _ := pair["error"].(string); reason != "" {
				t.Errorf("vanished pair carries error %q; want none — nothing failed to reconcile", reason)
			}
		}
	}
	if !sawVanished {
		t.Errorf("no pair reported %q\noutput: %s", fabricengine.ReconcileActionVanishedMidWalk, out.String())
	}
}

// TestRunCLI_MergeConflictEnvelopeKeepsPartialFalse asserts the conflict envelope's fixed key
// contract against real git: exit 1, "ok":false, a "mutations" array present, a "partial" key present
// and the literal false, and a non-empty "conflicts" array — the property the "conflict envelope
// never goes through errWithRecordFields" Shared Decision exists to pin.
func TestRunCLI_MergeConflictEnvelopeKeepsPartialFalse(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	// Diverge warp's "conflict.txt" on both the current branch and a "feature" branch, so merging
	// "feature" into the current branch conflicts. commitOnBranchCLI/commitOnCurrentBranchCLI are
	// merge_cli_integration_test.go's own fixture helpers, in the same package.
	commitOnCurrentBranchCLI(t, h.PrimeWorktree(), "conflict.txt", "seed content\n", "seed conflict.txt")
	commitOnBranchCLI(t, h.PrimeWorktree(), "feature", "conflict.txt", "branch content\n", "diverge conflict.txt on feature")
	commitOnCurrentBranchCLI(t, h.PrimeWorktree(), "conflict.txt", "current content\n", "diverge conflict.txt on current")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"merge-in", "feature"})
	if exitCode != 1 {
		t.Fatalf("RunCLI(merge-in feature) = %d; want 1\noutput: %s", exitCode, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode merge-in conflict envelope: %v\noutput: %s", err, out.String())
	}

	if ok, _ := envelope["ok"].(bool); ok {
		t.Errorf("envelope ok = true; want false on a conflict\noutput: %s", out.String())
	}
	if _, present := envelope["mutations"]; !present {
		t.Errorf("envelope is missing the always-present \"mutations\" key\noutput: %s", out.String())
	}
	partial, present := envelope["partial"]
	if !present {
		t.Errorf("envelope is missing the always-present \"partial\" key\noutput: %s", out.String())
	}
	if partial != false {
		t.Errorf("envelope partial = %v; want the literal false\noutput: %s", partial, out.String())
	}
	conflicts, ok := envelope["conflicts"].([]any)
	if !ok || len(conflicts) == 0 {
		t.Fatalf("envelope has no non-empty \"conflicts\" array\noutput: %s", out.String())
	}
}
