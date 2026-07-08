// state_test.go table-drives loadOrInitState's fresh/resume/hash-mismatch/
// terminal classification, exercises moveStaleArtifacts' renaming
// (including the double-.stale collision case), and round-trips a runState
// through saveState/loadOrInitState to check the persisted shape survives.

package perchengine

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

		got, info, err := loadOrInitState(runDir, "hash-1", []int{5, 8, 10})
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

		// The initial state must actually be persisted, not just returned in
		// memory — a second read should see the same file.
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
		if err := saveState(runDir, seed); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}

		got, info, err := loadOrInitState(runDir, "hash-1", []int{5, 8, 10})
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
		if err := saveState(runDir, seed); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}

		_, _, err := loadOrInitState(runDir, "new-hash", []int{5, 8, 10})
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
		if err := saveState(runDir, seed); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}

		_, _, err := loadOrInitState(runDir, "hash-1", []int{5, 8, 10})
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
		if err := saveState(runDir, seed); err != nil {
			t.Fatalf("saveState() = %v; want nil", err)
		}

		_, _, err := loadOrInitState(runDir, "new-hash", []int{5, 8, 10})
		if err == nil {
			t.Fatal("loadOrInitState() = nil; want an error")
		}
		wantSubstr := "already finished (STUCK)"
		if !strings.Contains(err.Error(), wantSubstr) {
			t.Errorf("loadOrInitState() = %q; want substring %q", err.Error(), wantSubstr)
		}
	})
}

// TestSaveState_ReadJSONRoundTrip round-trips a runState through saveState
// and a direct state.ReadJSON read, checking every field survives the
// write/read cycle.
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
				GatePath:        "round-1-gate.md",
				TriagePath:      "",
				JudgeVerdict:    "PROGRESSING",
				GatePassed:      &gatePassed,
				SessionID:       "session-abc",
			},
		},
		Outcome:     "",
		StuckReason: "",
	}

	if err := saveState(runDir, want); err != nil {
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
		gotRound.GatePath != wantRound.GatePath || gotRound.TriagePath != wantRound.TriagePath ||
		gotRound.JudgeVerdict != wantRound.JudgeVerdict || gotRound.SessionID != wantRound.SessionID {
		t.Errorf("Rounds[0] = %+v; want %+v", gotRound, wantRound)
	}
	if gotRound.GatePassed == nil || *gotRound.GatePassed != *wantRound.GatePassed {
		t.Errorf("Rounds[0].GatePassed = %v; want %v", gotRound.GatePassed, *wantRound.GatePassed)
	}
}

func TestMoveStaleArtifacts(t *testing.T) {
	t.Run("moves every existing artifact aside with .stale", func(t *testing.T) {
		runDir := t.TempDir()
		paths := artifactPaths(runDir, 3, 1)
		writeFile(t, paths.Review, "stale review")
		writeFile(t, paths.FixerReport, "stale fixer report")
		// Judge/Gate/Triage are left absent, as a round without a judge/gate/
		// triage step would leave them.

		if err := moveStaleArtifacts(runDir, 3, 1); err != nil {
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
		if err := moveStaleArtifacts(runDir, 5, 1); err != nil {
			t.Fatalf("moveStaleArtifacts() = %v; want nil", err)
		}
	})

	t.Run("a second stale collision gets a numeric suffix", func(t *testing.T) {
		runDir := t.TempDir()
		paths := artifactPaths(runDir, 3, 1)
		writeFile(t, paths.Review, "first stale review")
		if err := moveStaleIfExists(paths.Review); err != nil {
			t.Fatalf("moveStaleIfExists() (first) = %v; want nil", err)
		}
		if !fileExists(paths.Review + staleSuffix) {
			t.Fatalf("first stale path %q was not created", paths.Review+staleSuffix)
		}

		// A fresh round re-run wrote the same round-3-review.md path again,
		// and it is now stale too, colliding with the already-.stale file.
		writeFile(t, paths.Review, "second stale review")
		if err := moveStaleIfExists(paths.Review); err != nil {
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

func TestProfileHash(t *testing.T) {
	p1 := Profile{Rubric: "a", RoundCaps: []int{5, 8, 10}, JudgeModel: "haiku"}
	p2 := Profile{Rubric: "a", RoundCaps: []int{5, 8, 10}, JudgeModel: "haiku"}
	p3 := Profile{Rubric: "b", RoundCaps: []int{5, 8, 10}, JudgeModel: "haiku"}

	h1, err := ProfileHash(p1)
	if err != nil {
		t.Fatalf("ProfileHash(p1) = %v; want nil", err)
	}
	h2, err := ProfileHash(p2)
	if err != nil {
		t.Fatalf("ProfileHash(p2) = %v; want nil", err)
	}
	h3, err := ProfileHash(p3)
	if err != nil {
		t.Fatalf("ProfileHash(p3) = %v; want nil", err)
	}

	if h1 != h2 {
		t.Errorf("ProfileHash(p1) = %q; ProfileHash(p2) = %q; want equal for identical profiles", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("ProfileHash(p1) = ProfileHash(p3) = %q; want different for differing profiles", h1)
	}
	if len(h1) != 64 {
		t.Errorf("len(ProfileHash(p1)) = %d; want 64 (sha256 hex)", len(h1))
	}
}

func TestDeriveRunID(t *testing.T) {
	tests := []struct {
		name        string
		profilePath string
		hash        string
		want        string
	}{
		{
			name:        "simple basename",
			profilePath: filepath.Join("profiles", "code-review.yaml"),
			hash:        "abcdef0123456789",
			want:        "code-review-abcdef01",
		},
		{
			name:        "spaces and mixed case sanitize to dashes",
			profilePath: filepath.Join("profiles", "My Profile.yml"),
			hash:        "0011223344556677",
			want:        "my-profile-00112233",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveRunID(tt.profilePath, tt.hash)
			if got != tt.want {
				t.Errorf("DeriveRunID(%q, %q) = %q; want %q", tt.profilePath, tt.hash, got, tt.want)
			}
		})
	}
}

func TestPauseFlag(t *testing.T) {
	runDir := t.TempDir()
	flagPath := PauseFlagPath(runDir)
	if filepath.Dir(flagPath) != runDir {
		t.Errorf("PauseFlagPath(%q) = %q; want a file inside runDir", runDir, flagPath)
	}

	// clearPauseFlag must be a no-op when the flag is absent.
	if err := clearPauseFlag(runDir); err != nil {
		t.Fatalf("clearPauseFlag() (absent) = %v; want nil", err)
	}

	writeFile(t, flagPath, "")
	if err := clearPauseFlag(runDir); err != nil {
		t.Fatalf("clearPauseFlag() (present) = %v; want nil", err)
	}
	if fileExists(flagPath) {
		t.Errorf("pause flag %q still exists after clearPauseFlag", flagPath)
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
