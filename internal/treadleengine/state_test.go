// state_test.go tests the run-state file: loadOrInitState's classification and moveStaleArtifacts' renaming.
// It also round-trips runState through saveState and loadOrInitState.
// ProfileHash, DeriveRunID and ValidRunID belong to the caller, so this file does not test them.

package treadleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLoadOrInitState(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("fresh dir writes an initial state and starts at round 1", func(t *testing.T) {
		runDir := t.TempDir()

		got, info, err := loadOrInitState("gate", runDir, runDir, "hash-1", []int{5, 8, 10})
		if err != nil {
			t.Fatalf("loadOrInitState() = %v; want nil", err)
		}
		if !info.Fresh {
			t.Errorf("info.Fresh = false; want true")
		}
		if info.NextRound != 1 {
			t.Errorf("info.NextRound = %d; want 1", info.NextRound)
		}
		if got.ProfileHash != "hash-1" {
			t.Errorf("got.ProfileHash = %q; want %q", got.ProfileHash, "hash-1")
		}
		if !intSlicesEqual(got.RoundCaps, []int{5, 8, 10}) {
			t.Errorf("got.RoundCaps = %v; want %v", got.RoundCaps, []int{5, 8, 10})
		}

		path := filepath.Join(runDir, stateFileName)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("state.json not written: %v", err)
		}
	})

	// The resumed state is read back through state.ReadJSON,
	// so every field of the seeded rounds must survive the saveState/read cycle.
	// The first round sets every roundRecord field, including the *bool GatePassed.
	t.Run("unfinished state with matching hash resumes at the next round", func(t *testing.T) {
		runDir := t.TempDir()
		gatePassed := true
		seed := runState{
			ProfileHash: "hash-1",
			RoundCaps:   []int{5, 8, 10},
			Rounds: []roundRecord{
				{
					Round:           1,
					Attempts:        2,
					ShuttleOutcome:  "done",
					Verdict:         "BLOCKING",
					BlockingCount:   3,
					ReviewPath:      "round-1-review.md",
					FixerReportPath: "round-1-fixer-report.md",
					JudgePath:       "round-1-judge.md",
					HandoffPath:     "round-1-handoff.md",
					GatePath:        "round-1-gate.md",
					SeedPath:        "round-1-seed.md",
					JudgeVerdict:    "PROGRESSING",
					GatePassed:      &gatePassed,
					SessionID:       "session-abc",
				},
				{Round: 2, Attempts: 1, Verdict: "BLOCKING"},
			},
		}
		if err := saveState(runDir, runDir, seed); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}

		got, info, err := loadOrInitState("gate", runDir, runDir, "hash-1", []int{5, 8, 10})
		if err != nil {
			t.Fatalf("loadOrInitState() = %v; want nil", err)
		}
		if info.Fresh {
			t.Errorf("info.Fresh = true; want false")
		}
		if info.NextRound != 3 {
			t.Errorf("info.NextRound = %d; want 3", info.NextRound)
		}
		if !reflect.DeepEqual(got.Rounds, seed.Rounds) {
			t.Errorf("got.Rounds = %+v; want the seeded rounds %+v", got.Rounds, seed.Rounds)
		}
		if got.ProfileHash != seed.ProfileHash || !intSlicesEqual(got.RoundCaps, seed.RoundCaps) {
			t.Errorf("got = (%q, %v); want (%q, %v)", got.ProfileHash, got.RoundCaps, seed.ProfileHash, seed.RoundCaps)
		}
	})

	t.Run("unfinished state with a different hash fails loud", func(t *testing.T) {
		runDir := t.TempDir()
		seed := runState{ProfileHash: "old-hash", RoundCaps: []int{5, 8, 10}}
		if err := saveState(runDir, runDir, seed); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}

		_, _, err := loadOrInitState("gate", runDir, runDir, "new-hash", []int{5, 8, 10})
		if err == nil {
			t.Fatal("loadOrInitState() = nil; want an error")
		}
		wantSubstr := "started with a different profile"
		if !strings.Contains(err.Error(), wantSubstr) {
			t.Errorf("loadOrInitState() = %q; want substring %q", err.Error(), wantSubstr)
		}
	})

	t.Run("terminal state fails loud regardless of hash", func(t *testing.T) {
		runDir := t.TempDir()
		seed := runState{ProfileHash: "hash-1", RoundCaps: []int{5, 8, 10}, Outcome: "APPROVED"}
		if err := saveState(runDir, runDir, seed); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}

		_, _, err := loadOrInitState("gate", runDir, runDir, "hash-1", []int{5, 8, 10})
		if err == nil {
			t.Fatal("loadOrInitState() = nil; want an error")
		}
		wantSubstr := "already finished (APPROVED)"
		if !strings.Contains(err.Error(), wantSubstr) {
			t.Errorf("loadOrInitState() = %q; want substring %q", err.Error(), wantSubstr)
		}
	})

	t.Run("terminal state fails loud even with a mismatched hash", func(t *testing.T) {
		runDir := t.TempDir()
		seed := runState{ProfileHash: "old-hash", RoundCaps: []int{5, 8, 10}, Outcome: "STUCK", StuckReason: "hard-cap"}
		if err := saveState(runDir, runDir, seed); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}

		_, _, err := loadOrInitState("gate", runDir, runDir, "new-hash", []int{5, 8, 10})
		if err == nil {
			t.Fatal("loadOrInitState() = nil; want an error")
		}
		wantSubstr := "already finished (STUCK)"
		if !strings.Contains(err.Error(), wantSubstr) {
			t.Errorf("loadOrInitState() = %q; want substring %q", err.Error(), wantSubstr)
		}
	})

	// A block written before the handoffPath and seedPath fields existed must resume without migration.
	// Its rounds lack handoff coverage and carry no seed.
	// judgeReadSet's fallback and the PreRoundTargeting off-path handle those cases.
	// This writes the raw state.json bytes, bypassing saveState,
	// so the JSON has no "handoffPath" or "seedPath" keys at all.
	t.Run("legacy record without handoffPath or seedPath resumes cleanly", func(t *testing.T) {
		runDir := t.TempDir()
		legacyJSON := `{
			"profileHash": "hash-1",
			"roundCaps": [5, 8, 10],
			"rounds": [
				{
					"round": 1,
					"attempts": 1,
					"shuttleOutcome": "done",
					"verdict": "BLOCKING",
					"blockingCount": 1,
					"reviewPath": "round-1-review.md",
					"fixerReportPath": "round-1-fixer-report.md",
					"sessionId": "session-abc"
				}
			]
		}`
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v; want nil", err)
		}
		if err := os.WriteFile(filepath.Join(runDir, stateFileName), []byte(legacyJSON), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}

		got, info, err := loadOrInitState("gate", runDir, runDir, "hash-1", []int{5, 8, 10})
		if err != nil {
			t.Fatalf("loadOrInitState() = %v; want nil", err)
		}
		if info.Fresh {
			t.Errorf("info.Fresh = true; want false (a legacy record resumes, it is not fresh)")
		}
		if info.NextRound != 2 {
			t.Errorf("info.NextRound = %d; want 2", info.NextRound)
		}
		if len(got.Rounds) != 1 || got.Rounds[0].HandoffPath != "" || got.Rounds[0].SeedPath != "" {
			t.Errorf("got.Rounds = %+v; want one round with both HandoffPath and SeedPath empty", got.Rounds)
		}
	})
}

// TestTerminalOutcome covers the three states a caller must tell apart before it writes a pause flag.
// A run dir with no state file is not terminal.
// A block with an empty Outcome is still in flight.
// A finished block reports its recorded Outcome with ok true.
func TestTerminalOutcome(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("no state file reports not terminal", func(t *testing.T) {
		runDir := t.TempDir()
		outcome, ok, err := TerminalOutcome(runDir, runDir)
		if err != nil {
			t.Fatalf("TerminalOutcome() error = %v; want nil", err)
		}
		if ok || outcome != "" {
			t.Errorf("TerminalOutcome() = (%q, %v); want (\"\", false) for a missing state file", outcome, ok)
		}
	})

	t.Run("in-flight block reports not terminal", func(t *testing.T) {
		runDir := t.TempDir()
		if err := saveState(runDir, runDir, runState{ProfileHash: "h", RoundCaps: []int{3}}); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}
		outcome, ok, err := TerminalOutcome(runDir, runDir)
		if err != nil {
			t.Fatalf("TerminalOutcome() error = %v; want nil", err)
		}
		if ok || outcome != "" {
			t.Errorf("TerminalOutcome() = (%q, %v); want (\"\", false) for an in-flight block", outcome, ok)
		}
	})

	t.Run("finished block reports its recorded outcome", func(t *testing.T) {
		runDir := t.TempDir()
		if err := saveState(runDir, runDir, runState{ProfileHash: "h", RoundCaps: []int{3}, Outcome: string(OutcomeStuck), StuckReason: string(StuckHardCap)}); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}
		outcome, ok, err := TerminalOutcome(runDir, runDir)
		if err != nil {
			t.Fatalf("TerminalOutcome() error = %v; want nil", err)
		}
		if !ok || outcome != OutcomeStuck {
			t.Errorf("TerminalOutcome() = (%q, %v); want (%q, true)", outcome, ok, OutcomeStuck)
		}
	})
}

func TestMoveStaleArtifacts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("moves every existing artifact aside with .stale", func(t *testing.T) {
		runDir := t.TempDir()
		paths := artifactPaths(runDir, 3, 1)
		writeFile(t, paths.Review, "stale review")
		writeFile(t, paths.FixerReport, "stale fixer report")
		// Judge/Gate are left absent, as a round without a judge/gate
		// step would leave them.

		if err := moveStaleArtifacts("gate", runDir, 3, 1); err != nil {
			t.Fatalf("moveStaleArtifacts() = %v; want nil", err)
		}

		if fileExists(paths.Review) {
			t.Errorf("original review path %q still exists", paths.Review)
		}
		if !fileExists(paths.Review + staleSuffix) {
			t.Errorf("stale review path %q was not created", paths.Review+staleSuffix)
		}
		if !fileExists(paths.FixerReport + staleSuffix) {
			t.Errorf("stale fixer-report path %q was not created", paths.FixerReport+staleSuffix)
		}
	})

	t.Run("no-op when no artifacts exist", func(t *testing.T) {
		runDir := t.TempDir()
		if err := moveStaleArtifacts("gate", runDir, 5, 1); err != nil {
			t.Fatalf("moveStaleArtifacts() = %v; want nil", err)
		}
	})

	t.Run("a second stale collision gets a numeric suffix", func(t *testing.T) {
		runDir := t.TempDir()
		paths := artifactPaths(runDir, 3, 1)
		writeFile(t, paths.Review, "first stale review")
		if err := moveStaleIfExists("gate", paths.Review); err != nil {
			t.Fatalf("moveStaleIfExists() (first) = %v; want nil", err)
		}
		if !fileExists(paths.Review + staleSuffix) {
			t.Fatalf("first stale path %q was not created", paths.Review+staleSuffix)
		}

		// A fresh round re-run wrote the same round-3-review.md path again,
		// and it is now stale too, colliding with the already-.stale file.
		writeFile(t, paths.Review, "second stale review")
		if err := moveStaleIfExists("gate", paths.Review); err != nil {
			t.Fatalf("moveStaleIfExists() (second) = %v; want nil", err)
		}

		if fileExists(paths.Review) {
			t.Errorf("original review path %q still exists after second collision", paths.Review)
		}
		firstStale := paths.Review + staleSuffix
		secondStale := paths.Review + staleSuffix + ".2"
		if !fileExists(firstStale) {
			t.Errorf("first stale path %q was lost", firstStale)
		}
		if !fileExists(secondStale) {
			t.Errorf("second stale path %q was not created", secondStale)
		}
	})
}

func TestPauseFlag(t *testing.T) {
	tiergate.Require(t, 0)
	runDir := t.TempDir()
	flagPath := PauseFlagPath(runDir)
	if filepath.Dir(flagPath) != runDir {
		t.Errorf("PauseFlagPath(%q) = %q; want a file inside runDir", runDir, flagPath)
	}

	// clearPauseFlag must be a no-op when the flag is absent.
	if err := clearPauseFlag("gate", runDir); err != nil {
		t.Fatalf("clearPauseFlag() (absent) = %v; want nil", err)
	}

	writeFile(t, flagPath, "")
	if err := clearPauseFlag("gate", runDir); err != nil {
		t.Fatalf("clearPauseFlag() (present) = %v; want nil", err)
	}
	if fileExists(flagPath) {
		t.Errorf("pause flag %q still exists after clearPauseFlag", flagPath)
	}

	// A removal failure, other than the flag already being absent, must carry the calling engine's name.
	// Run returns this error verbatim, so an unprefixed message would reach the CLI envelope with no module label.
	// A non-empty directory at the flag path is the portable way to make os.Remove fail without changing permissions.
	writeFile(t, filepath.Join(flagPath, "occupant.txt"), "x")
	err := clearPauseFlag("tenter", runDir)
	if err == nil {
		t.Fatal("clearPauseFlag() (unremovable) = nil; want an error")
	}
	if !strings.HasPrefix(err.Error(), "tenter: ") {
		t.Errorf("clearPauseFlag() error = %q; want a \"tenter: \"-prefixed message", err.Error())
	}
}

// writeFile writes content to path, creating parent directories as needed and failing the test on any I/O error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
	}
}

// intSlicesEqual reports whether a and b contain the same ints in the same order.
// The helper is duplicated rather than shared across packages.
func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
