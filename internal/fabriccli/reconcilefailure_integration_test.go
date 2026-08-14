//go:build integration

// reconcilefailure_integration_test.go covers the one thing `lyx fabric reconcile`'s envelope must
// never get wrong: a pair it was asked to repair and could not must reach the caller as a FAILURE.
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
