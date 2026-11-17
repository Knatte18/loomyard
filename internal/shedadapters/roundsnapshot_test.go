// roundsnapshot_test.go tests the round snapshot record the review producer writes at a round's start.

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
// It also covers that a row with no snapshot records no target id.
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
	}{
		{name: "an accepted review leaves the scope, paths, target id and decision digests", withSnapshot: true},
		{name: "a second call keeps the first attempt's target id", withSnapshot: true, failFirst: true},
		{name: "a nil snapshot leaves no target id"},
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
			snapshot := TargetSnapshot{Scope: burlerengine.FixScopeOverlay, WorktreeRoot: root, Paths: []string{"target.txt"}, CacheDir: filepath.Join(root, "cache")}
			wantID, err := snapshot.ID()
			if err != nil {
				t.Fatalf("ID() = %v; want nil", err)
			}

			spec := newHalfSpec()
			spec.profile.Target = burlerengine.FileSet{Paths: []string{"target.txt"}}
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
			wantTargetID := ""
			if tt.withSnapshot {
				wantTargetID = wantID
			}
			if got.FixScope != "overlay" || !slices.Equal(got.TargetPaths, []string{"target.txt"}) || got.TargetID != wantTargetID {
				t.Errorf("record = %+v; want scope overlay, paths [target.txt] and target id %q", got, wantTargetID)
			}
			if !slices.Equal(got.AddedDecisions, wantDecisions) {
				t.Errorf("record added decisions = %+v; want %+v", got.AddedDecisions, wantDecisions)
			}
		})
	}
}
