// generation_test.go exercises PlanReviewSkippable at Tier 1 over a fake committed-file reader and temp directories.

package loomshed

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"testing"
)

var (
	proseCard  = reworkGenCard{"notes", "Prosa", "docs/notes.md"}
	sourceCard = reworkGenCard{"code", "Create", "internal/x/new.go"}
)

// commitRoundAt records round n as committed at HEAD with the given class and first_card; an empty class writes a classless record.
func (f *reworkFixture) commitRoundAt(n, first int, class string) {
	f.t.Helper()
	dir := filepath.Join(f.reworkDir, fmt.Sprintf("round-%d", n))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	data, _ := json.Marshal(roundRecord{PRNumber: 7, HeadSHA: reworkTestHead, RejectedAt: reworkTestRejectedAt, FirstCard: first, Class: class})
	f.committed[path.Join("rework", fmt.Sprintf("round-%d", n), "record.json")] = data
}

func (f *reworkFixture) skippable() (bool, error) {
	f.t.Helper()
	return PlanReviewSkippable(f.deps())
}

func TestPlanReviewSkippable(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *reworkFixture)
		want  bool
	}{
		{"exempt record over exempt live cards", func(f *reworkFixture) {
			f.writeGeneration(2, proseCard)
			f.commitWorkingPlan()
			f.commitRoundAt(1, 2, ReworkClassExempt)
		}, true},
		{"required record", func(f *reworkFixture) {
			f.writeGeneration(2, proseCard)
			f.commitWorkingPlan()
			f.commitRoundAt(1, 2, ReworkClassRequired)
		}, false},
		{"generation 0 has no round", func(f *reworkFixture) {
			f.writeGeneration(1, proseCard)
			f.commitWorkingPlan()
		}, false},
		{"round with a different first_card", func(f *reworkFixture) {
			f.writeGeneration(5, proseCard)
			f.commitWorkingPlan()
			f.commitRoundAt(1, 2, ReworkClassExempt)
		}, false},
		{"uncommitted working-tree edit that classifies required", func(f *reworkFixture) {
			f.writeGeneration(2, proseCard)
			f.commitWorkingPlan()
			f.commitRoundAt(1, 2, ReworkClassExempt)
			f.writeGeneration(2, sourceCard)
		}, false},
		{"exempt record over live cards that classify required", func(f *reworkFixture) {
			f.writeGeneration(2, sourceCard)
			f.commitWorkingPlan()
			f.commitRoundAt(1, 2, ReworkClassExempt)
		}, false},
		{"classless record", func(f *reworkFixture) {
			f.writeGeneration(2, proseCard)
			f.commitWorkingPlan()
			f.commitRoundAt(1, 2, "")
		}, false},
		{"only the highest matching round decides", func(f *reworkFixture) {
			f.writeGeneration(2, proseCard)
			f.commitWorkingPlan()
			f.commitRoundAt(1, 2, ReworkClassExempt)
			f.commitRoundAt(2, 2, ReworkClassRequired)
		}, false},
		{"a lower round with another first_card does not shadow the live one", func(f *reworkFixture) {
			f.writeGeneration(4, proseCard)
			f.commitWorkingPlan()
			f.commitRoundAt(1, 4, ReworkClassExempt)
			f.commitRoundAt(2, 7, ReworkClassRequired)
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newReworkFixture(t)
			tc.setup(f)
			got, err := f.skippable()
			if err != nil {
				t.Fatalf("PlanReviewSkippable: %v", err)
			}
			if got != tc.want {
				t.Errorf("PlanReviewSkippable = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlanReviewSkippable_UndecodableRecordErrors(t *testing.T) {
	f := newReworkFixture(t)
	f.writeGeneration(2, proseCard)
	f.commitWorkingPlan()
	f.commitRoundAt(1, 2, ReworkClassExempt)
	f.committed["rework/round-1/record.json"] = []byte("{not json")
	if got, err := f.skippable(); err == nil || got {
		t.Errorf("PlanReviewSkippable = %v, %v; want false and an error", got, err)
	}
}

func TestPlanReviewSkippable_UncommittedPlanErrors(t *testing.T) {
	f := newReworkFixture(t)
	f.committed = map[string][]byte{}
	if got, err := f.skippable(); err == nil || got {
		t.Errorf("PlanReviewSkippable = %v, %v; want false and an error", got, err)
	}
}

func TestArchivedWebsterDirs(t *testing.T) {
	reworkDir := t.TempDir()
	for _, n := range []string{"round-10", "round-2", "round-1"} {
		if err := os.MkdirAll(filepath.Join(reworkDir, n, "prior-generation", "webster"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(reworkDir, "round-3", "prior-generation", "plan"), 0o755); err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, n := range []string{"round-1", "round-2", "round-10"} {
		want = append(want, filepath.Join(reworkDir, n, "prior-generation", "webster"))
	}
	if got, err := ArchivedWebsterDirs(reworkDir); err != nil || !slices.Equal(got, want) {
		t.Errorf("ArchivedWebsterDirs = %v, %v; want %v, nil", got, err, want)
	}
	if got, err := ArchivedWebsterDirs(filepath.Join(reworkDir, "absent")); err != nil || len(got) != 0 {
		t.Errorf("ArchivedWebsterDirs over absent dir = %v, %v; want none, nil", got, err)
	}
}

// TestArchivedWebsterDirs_UnreadableDirErrors covers a rework directory that cannot be listed: an error, never a silently empty list.
func TestArchivedWebsterDirs_UnreadableDirErrors(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "rework")
	if err := os.WriteFile(notADir, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ArchivedWebsterDirs(notADir); err == nil {
		t.Errorf("ArchivedWebsterDirs = %v, nil; want an error", got)
	}
}
