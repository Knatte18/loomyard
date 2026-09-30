//go:build integration

// mergein_webster_integration_test.go drives "lyx fabric merge-in" through fabriccli.RunCLIIn against a
// real hubforge pair while webster state sits under the worktree's anchor,
// asserting the optional "warnings" key that reports a mid-run parent merge and never blocks it.

package fabriccli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// seedWebsterState writes state.json, plus outcome.yaml when outcome is non-empty, under the
// hub's webster directory and commits it on the prime weft, so the pair is clean when the weft feature branch is cut.
func seedWebsterState(t *testing.T, h *hubforge.Hub, outcome string) {
	t.Helper()

	dir := websterengine.Dir(h.Location.AnchorPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(state.json): %v", err)
	}
	if outcome != "" {
		if err := os.WriteFile(filepath.Join(dir, "outcome.yaml"), []byte(outcome), 0o644); err != nil {
			t.Fatalf("WriteFile(outcome.yaml): %v", err)
		}
	}
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "-q", "-m", "seed webster state")
}

// runMergeIn runs "merge-in feature" from the prime worktree and returns the exit code and decoded envelope.
func runMergeIn(t *testing.T, h *hubforge.Hub) (int, map[string]any) {
	t.Helper()

	var out bytes.Buffer
	exitCode := fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"merge-in", "feature"})
	return exitCode, decodeResult(t, &out)
}

// divergeCleanly gives feature and the current branch each a commit on a different file.
func divergeCleanly(t *testing.T, h *hubforge.Hub) {
	t.Helper()

	commitOnBranchCLI(t, h.PrimeWorktree(), "feature", "feature.txt", "feature\n", "feature: add file")
	commitOnCurrentBranchCLI(t, h.PrimeWorktree(), "main-side.txt", "main\n", "main: add file")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")
}

func assertOneWebsterWarning(t *testing.T, envelope map[string]any) {
	t.Helper()

	raw, present := envelope["warnings"]
	if !present {
		t.Fatalf("envelope has no warnings key: %v", envelope)
	}
	warnings, ok := raw.([]any)
	if !ok || len(warnings) != 1 {
		t.Fatalf("warnings = %v; want exactly one entry", raw)
	}
	if s, _ := warnings[0].(string); s == "" {
		t.Errorf("warnings[0] = %v; want a non-empty string", warnings[0])
	}
}

func assertNoWarnings(t *testing.T, envelope map[string]any) {
	t.Helper()

	if _, present := envelope["warnings"]; present {
		t.Errorf("envelope carries a warnings key = %v; want none", envelope["warnings"])
	}
}

func TestMergeIn_WarnsWhileWebsterInFlight(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	seedWebsterState(t, h, "")
	divergeCleanly(t, h)

	exitCode, envelope := runMergeIn(t, h)
	if exitCode != 0 {
		t.Fatalf("merge-in = %d; want 0\nenvelope: %v", exitCode, envelope)
	}
	if committed, _ := envelope["committed"].(bool); !committed {
		t.Errorf("committed = %v; want true", envelope["committed"])
	}
	assertOneWebsterWarning(t, envelope)
}

func TestMergeIn_ConflictWarnsWhileWebsterInFlight(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	seedWebsterState(t, h, "")
	setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	exitCode, envelope := runMergeIn(t, h)
	if exitCode != 1 {
		t.Fatalf("merge-in = %d; want 1\nenvelope: %v", exitCode, envelope)
	}
	if conflicts, _ := envelope["conflicts"].([]any); len(conflicts) == 0 {
		t.Errorf("conflicts = %v; want a non-empty array", envelope["conflicts"])
	}
	assertOneWebsterWarning(t, envelope)
}

func TestMergeIn_NoWarningsWithoutWebsterState(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	divergeCleanly(t, h)

	exitCode, envelope := runMergeIn(t, h)
	if exitCode != 0 {
		t.Fatalf("merge-in = %d; want 0\nenvelope: %v", exitCode, envelope)
	}
	assertNoWarnings(t, envelope)
}

func TestMergeIn_NoWarningsWhenWebsterDone(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	seedWebsterState(t, h, "outcome: done\nstuck_reason: null\nbatches_done: 3\n")
	divergeCleanly(t, h)

	exitCode, envelope := runMergeIn(t, h)
	if exitCode != 0 {
		t.Fatalf("merge-in = %d; want 0\nenvelope: %v", exitCode, envelope)
	}
	assertNoWarnings(t, envelope)
}

func TestMergeIn_NoWarningsWhenAlreadyUpToDate(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	seedWebsterState(t, h, "")
	branchAtCurrentHEADCLI(t, h.PrimeWorktree(), "feature")
	branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")

	exitCode, envelope := runMergeIn(t, h)
	if exitCode != 0 {
		t.Fatalf("merge-in = %d; want 0\nenvelope: %v", exitCode, envelope)
	}
	if upToDate, _ := envelope["already_up_to_date"].(bool); !upToDate {
		t.Errorf("already_up_to_date = %v; want true", envelope["already_up_to_date"])
	}
	assertNoWarnings(t, envelope)
}

func TestMergeIn_MalformedOutcomeDegradesToNoWarning(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	seedWebsterState(t, h, "outcome: [unterminated\n")
	divergeCleanly(t, h)

	exitCode, envelope := runMergeIn(t, h)
	if exitCode != 0 {
		t.Fatalf("merge-in = %d; want 0\nenvelope: %v", exitCode, envelope)
	}
	if committed, _ := envelope["committed"].(bool); !committed {
		t.Errorf("committed = %v; want true", envelope["committed"])
	}
	assertNoWarnings(t, envelope)
}
