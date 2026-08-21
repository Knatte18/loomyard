// state_test.go table-drives loadOrInitState's fresh/resume/hash-mismatch/ terminal classification,
// exercises moveStaleArtifacts' renaming (including the double-.stale collision case), and
// round-trips a runState through saveState/loadOrInitState to check the persisted shape survives.
// ProfileHash/DeriveRunID/ValidRunID are a caller's own functions, not this package's, so their
// tests belong with that caller.

package treadleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

func TestLoadOrInitState(t *testing.T) {
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

		// Verify the initial state is actually persisted.
		path := filepath.Join(runDir, stateFileName)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("state.json not written: %v", err)
		}
	})

	t.Run("unfinished state with matching hash resumes at the next round", func(t *testing.T) {
		runDir := t.TempDir()
		seed := runState{
			ProfileHash: "hash-1",
			RoundCaps:   []int{5, 8, 10},
			Rounds: []roundRecord{
				{Round: 1, Attempts: 1, Verdict: "BLOCKING"},
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
		if len(got.Rounds) != 2 {
			t.Errorf("len(got.Rounds) = %d; want 2", len(got.Rounds))
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

	// A block written by a binary that predates handoffPath/seedPath (the
	// additive state-json-compatibility fields — see roundRecord's doc) must
	// resume with zero migration: its rounds simply lack handoff coverage
	// and never carry a seed, which judgeReadSet's fallback and the
	// PreRoundTargeting off-path already handle respectively. This writes
	// the raw state.json bytes directly (bypassing saveState/roundRecord) so
	// the JSON genuinely has no "handoffPath"/"seedPath" keys at all, rather
	// than merely empty ones.
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

// TestSaveState_ReadJSONRoundTrip round-trips a runState through saveState and a direct
// state.ReadJSON read, checking every field survives the write/read cycle.
func TestSaveState_ReadJSONRoundTrip(t *testing.T) {
	runDir := t.TempDir()
	gatePassed := true
	want := runState{
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
				TriagePath:      "",
				SeedPath:        "round-1-seed.md",
				JudgeVerdict:    "PROGRESSING",
				GatePassed:      &gatePassed,
				SessionID:       "session-abc",
			},
		},
		Outcome:     "",
		StuckReason: "",
	}

	if err := saveState(runDir, runDir, want); err != nil {
		t.Fatalf("saveState() = %v; want nil", err)
	}

	path := filepath.Join(runDir, stateFileName)
	lockPath := path + ".lock"
	got, found, err := state.ReadJSON[runState](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() = %v; want nil", err)
	}
	if !found {
		t.Fatal("ReadJSON() found = false; want true")
	}

	if got.ProfileHash != want.ProfileHash {
		t.Errorf("ProfileHash = %q; want %q", got.ProfileHash, want.ProfileHash)
	}
	if !intSlicesEqual(got.RoundCaps, want.RoundCaps) {
		t.Errorf("RoundCaps = %v; want %v", got.RoundCaps, want.RoundCaps)
	}
	if len(got.Rounds) != 1 {
		t.Fatalf("len(Rounds) = %d; want 1", len(got.Rounds))
	}
	gotRound := got.Rounds[0]
	wantRound := want.Rounds[0]
	if gotRound.Round != wantRound.Round || gotRound.Attempts != wantRound.Attempts ||
		gotRound.ShuttleOutcome != wantRound.ShuttleOutcome || gotRound.Verdict != wantRound.Verdict ||
		gotRound.BlockingCount != wantRound.BlockingCount || gotRound.ReviewPath != wantRound.ReviewPath ||
		gotRound.FixerReportPath != wantRound.FixerReportPath || gotRound.JudgePath != wantRound.JudgePath ||
		gotRound.HandoffPath != wantRound.HandoffPath ||
		gotRound.GatePath != wantRound.GatePath || gotRound.TriagePath != wantRound.TriagePath ||
		gotRound.SeedPath != wantRound.SeedPath ||
		gotRound.JudgeVerdict != wantRound.JudgeVerdict || gotRound.SessionID != wantRound.SessionID {
		t.Errorf("Rounds[0] = %+v; want %+v", gotRound, wantRound)
	}
	if gotRound.GatePassed == nil || *gotRound.GatePassed != *wantRound.GatePassed {
		t.Errorf("Rounds[0].GatePassed = %v; want %v", gotRound.GatePassed, *wantRound.GatePassed)
	}
}

// TestTerminalOutcome covers the three states a caller's own pause verb (e.g.
// a pause verb) must distinguish before writing a pause flag: no state file at all (a run dir that
// never started a block), an in-flight block (empty Outcome), and a finished block, whose recorded
// Outcome is reported with ok true.
func TestTerminalOutcome(t *testing.T) {
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
	t.Run("moves every existing artifact aside with .stale", func(t *testing.T) {
		runDir := t.TempDir()
		paths := artifactPaths(runDir, 3, 1)
		writeFile(t, paths.Review, "stale review")
		writeFile(t, paths.FixerReport, "stale fixer report")
		// Judge/Gate/Triage are left absent, as a round without a judge/gate/
		// triage step would leave them.

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

	// A removal that fails for any reason other than "already absent" must
	// still carry the calling engine's name: Run returns this error verbatim,
	// so an unprefixed message would surface in a caller's CLI envelope as the
	// only diagnostic with no module label. A non-empty directory at the flag
	// path is the portable way to make os.Remove fail without touching
	// permissions.
	writeFile(t, filepath.Join(flagPath, "occupant.txt"), "x")
	err := clearPauseFlag("tenter", runDir)
	if err == nil {
		t.Fatal("clearPauseFlag() (unremovable) = nil; want an error")
	}
	if !strings.HasPrefix(err.Error(), "tenter: ") {
		t.Errorf("clearPauseFlag() error = %q; want a \"tenter: \"-prefixed message", err.Error())
	}
}

// writeFile writes content to path, creating parent directories as needed
// and failing the test on any I/O error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
	}
}

// intSlicesEqual reports whether a and b contain the same ints in the same
// order. A few-line local copy rather than a shared helper, per the
// mechanical-package-split helper-fallout clause: a
// handful of lines, duplicated verbatim rather than shared across packages.
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
