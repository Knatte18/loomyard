// mergestage_integration_test.go covers MergeStageResolved against real conflicted pairs.
// It checks that a path neither side lists is an error and that empty paths are a no-op.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestMergeStageResolved_PathNotConflictedOnEitherSide covers the caller-bug guard:
// a path listed by neither side's ConflictedFiles() is an error naming that path, never a silent skip.
func TestMergeStageResolved_PathNotConflictedOnEitherSide(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	_, f, _, _, _, _ := newMergeLayerFixture(t, &conflictLayer)

	if _, err := f.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}

	const bogusPath = "never-conflicted.txt"
	_, err := f.MergeStageResolved([]string{bogusPath})
	if err == nil {
		t.Fatal("MergeStageResolved([bogus path]) error = nil; want an error naming the unconflicted path")
	}
	if got := err.Error(); !strings.Contains(got, bogusPath) {
		t.Errorf("MergeStageResolved([bogus path]) error = %q; want it to name %q", got, bogusPath)
	}
}

// TestMergeStageResolved_EmptyPathsIsNoOp covers the empty-slice call: a nil error and an empty
// mutation record, with nothing staged.
func TestMergeStageResolved_EmptyPathsIsNoOp(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	_, f, _, _, _, _ := newMergePairFixture(t, ".")

	res, err := f.MergeStageResolved(nil)
	if err != nil {
		t.Fatalf("MergeStageResolved(nil) error = %v; want nil", err)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("MergeStageResolved(nil).Mutated().Len() = %d; want 0 (empty mutation record)", res.Mutated().Len())
	}

	res, err = f.MergeStageResolved([]string{})
	if err != nil {
		t.Fatalf("MergeStageResolved([]) error = %v; want nil", err)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("MergeStageResolved([]).Mutated().Len() = %d; want 0 (empty mutation record)", res.Mutated().Len())
	}
}
