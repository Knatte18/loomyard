// roundsnapshot_test.go tests the round snapshot record the review producer writes at a round's start.
// It also tests that the review producer drops the record's rerun when it retires the round.

package shedadapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBurlerReviewProducer_Call_RecordsRoundSnapshot covers what the record holds after an accepted review.
// It also covers that a failed first attempt's record survives an edited target.
// It also covers that a row with no snapshot, or whose target matches no file, records no target id.
func TestBurlerReviewProducer_Call_RecordsRoundSnapshot(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		firstEntry  = "### Added after Discussion (parent, 2026-11-17): First call\n\nDecision: cache per run.\n"
		secondEntry = "### Added after Discussion (parent, 2026-11-17): Second call\n\nDecision: no retry.\n"
	)
	digestOf := func(text string) string {
		sum := sha256.Sum256([]byte(text))
		return hex.EncodeToString(sum[:])
	}
	wantDecisions := []addedDecisionDigest{
		{Title: "First call", Digest: digestOf("### Added after Discussion (parent, 2026-11-17): First call\n\nDecision: cache per run.")},
		{Title: "Second call", Digest: digestOf("### Added after Discussion (parent, 2026-11-17): Second call\n\nDecision: no retry.")},
	}

	tests := []struct {
		name string
		// withSnapshot gives the producer an overlay snapshot over the temp target.
		withSnapshot bool
		// failFirst makes the first Call fail through its failure exit, and the target is edited before the second.
		failFirst bool
		// missingTarget points the snapshot and the profile at a file that does not exist.
		missingTarget bool
	}{
		{name: "an accepted review leaves the scope, paths, target id and decision digests", withSnapshot: true},
		{name: "a second call keeps the first attempt's target id", withSnapshot: true, failFirst: true},
		{name: "a nil snapshot leaves no target id"},
		{name: "a target that matches no file leaves no target id", withSnapshot: true, missingTarget: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("one\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(target) = %v; want nil", err)
			}
			recordPath := filepath.Join(root, "decisions.md")
			if err := os.WriteFile(recordPath, []byte("# Record\n\n## Decisions\n\n"+firstEntry+"\n"+secondEntry), 0o644); err != nil {
				t.Fatalf("WriteFile(record) = %v; want nil", err)
			}
			targetPaths := []string{"target.txt"}
			if tt.missingTarget {
				targetPaths = []string{"absent.txt"}
			}
			snapshot := TargetSnapshot{Scope: burlerengine.FixScopeOverlay, WorktreeRoot: root, Paths: targetPaths, CacheDir: filepath.Join(root, "cache")}
			wantID := ""
			if tt.withSnapshot && !tt.missingTarget {
				id, err := snapshot.ID()
				if err != nil {
					t.Fatalf("ID() = %v; want nil", err)
				}
				wantID = id
			}

			spec := newHalfSpec()
			spec.profile.Target = burlerengine.FileSet{Paths: targetPaths}
			spec.decisionRecordPath = recordPath
			if tt.withSnapshot {
				spec.snapshot = &snapshot
			}
			runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d1")}}
			if tt.failFirst {
				runner = &shedfake.BurlerHalfRunner{
					ReviewResults: []burlerengine.ReviewResult{{}, reviewDone("d1")},
					ReviewErrs:    []error{errors.New("runner failed"), nil},
				}
			}
			runDir := filepath.Join(root, "run")
			p := newReviewProducer(t, runDir, runner, spec)

			if tt.failFirst {
				if _, _, err := p.Call(context.Background()); err == nil {
					t.Fatal("first Call() error = nil; want the runner's failure")
				}
				if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("edited\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(edited target) = %v; want nil", err)
				}
			}
			shedfake.RequireOutcome(t, p, shedengine.Stuck)

			got, found, err := readRoundSnapshot(runDir, 1)
			if err != nil || !found {
				t.Fatalf("readRoundSnapshot() = %v, %v; want the record", found, err)
			}
			if got.FixScope != "overlay" || !slices.Equal(got.TargetPaths, targetPaths) || got.TargetID != wantID {
				t.Errorf("record = %+v; want scope overlay, paths %v and target id %q", got, targetPaths, wantID)
			}
			if !slices.Equal(got.AddedDecisions, wantDecisions) {
				t.Errorf("record added decisions = %+v; want %+v", got.AddedDecisions, wantDecisions)
			}
		})
	}
}

// TestBurlerReviewProducer_Call_FailureExitDropsTheRoundsRerunRecord proves a rerun is dropped.
// The review producer's failure exit retires the round's review.
// The rerun a fixer left in the round's snapshot record goes with it.
func TestBurlerReviewProducer_Call_FailureExitDropsTheRoundsRerunRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	runDir := filepath.Join(t.TempDir(), "run")
	runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{{}}, ReviewErrs: []error{errors.New("runner failed")}}
	runner.DuringReview = func(int) {
		planted := &fixerRerunRecord{ReviewDigest: "d1", Reason: "the round halted: x", Report: "old.md"}
		if err := updateRoundSnapshot(runDir, 1, func(rec *roundSnapshot, _ bool) { rec.FixerRerun = planted }); err != nil {
			t.Errorf("updateRoundSnapshot: %v", err)
		}
	}
	p := newReviewProducer(t, runDir, runner, newHalfSpec())

	if _, _, err := p.Call(context.Background()); err == nil {
		t.Fatal("Call() error = nil; want the runner's failure")
	}

	snapshot, found, err := readRoundSnapshot(runDir, 1)
	if err != nil || !found || snapshot.FixerRerun != nil {
		t.Errorf("round 1 snapshot = %+v, found %v, %v; want the record kept with no rerun", snapshot, found, err)
	}
}
