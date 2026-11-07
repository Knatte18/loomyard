// failurereport_integration_test.go covers the failure report's reads of a real repository.
// It reads the changed files from the merge base with the pair's parent branch, and the lint steps a gate key lists.
// It is Tier 2: it builds a real hub and spawns git.

package gatecli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/patternlint"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestTouchedFiles(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// renameParent renames the parent branch the pair's origin record names, so the merge base cannot be read.
		renameParent bool
		want         []string
	}{
		{name: "MergeBaseWithTheParentListsCommittedAndUntrackedFiles", want: []string{"committed/c.go", "untracked/u.go"}},
		{name: "UnreadableMergeBaseFallsBackToTheFilesChangedSinceHEAD", renameParent: true, want: []string{"untracked/u.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
			pair := h.PairCodeWorktree(hubforge.TemplatePairSlug)
			for rel, content := range map[string]string{"committed/c.go": "package committed\n", "untracked/u.go": "package untracked\n"} {
				full := filepath.Join(pair, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			gitkit.Git(t, pair, "add", "committed")
			gitkit.Git(t, pair, "commit", "-q", "-m", "commit one file after the merge base")
			location, err := lymcwd.ResolveWorktree(pair)
			if err != nil {
				t.Fatal(err)
			}
			if tt.renameParent {
				gitkit.Git(t, h.PrimeWorktree(), "branch", "-m", "main", "main-renamed")
			}

			got := touchedFiles(context.Background(), location, pair, true)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("touchedFiles() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRunLints_PatternHeadroom(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()
	base, err := patternlint.OverviewAt(prime, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	head := bytes.Repeat([]byte("x"), len(base)+pattern.HeadroomBytes+500)
	if err := os.WriteFile(filepath.Join(prime, "PATTERN.md"), head, 0o644); err != nil {
		t.Fatal(err)
	}

	got := runLints(prime, []string{gateslot.LintPattern})
	message := fmt.Sprintf("PATTERN.md: overview grew to %d bytes from %d, past the %d-byte headroom line; cut %d bytes", len(head), len(base), pattern.HeadroomBytes, len(head)-max(len(base), pattern.HeadroomBytes))
	want := []failedRun{{
		label: "lint pattern", outcome: "1 finding", wayForward: patternlint.WayForward,
		failures: []runFailure{{lines: []string{message}}},
	}}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(failedRun{}, runFailure{})); diff != "" {
		t.Errorf("runLints() mismatch (-want +got):\n%s", diff)
	}
}
