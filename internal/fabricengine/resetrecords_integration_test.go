//go:build tier1

// resetrecords_integration_test.go covers ResetRecordsToParent over a hubforge hub: the reset of a forked pair's records to its parent's, its idempotence and the refusals that precede any commit.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// sortedLines returns the non-empty lines of out, sorted.
func sortedLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	slices.Sort(lines)
	return lines
}

func TestResetRecordsToParent(t *testing.T) {
	t.Parallel()
	const originPath = "_lyx/fabric/origin.json"

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	reset := func(slug string) (fabricengine.ResetRecordsResult, error) {
		return fabricengine.ResetRecordsToParent(fabricengine.NewMutations(h.Path), l, slug)
	}

	t.Run("replaces the records with the parent's and keeps the origin record", func(t *testing.T) {
		const slug = "forked"
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
		weft := h.PairRecordsSibling(slug)
		gitkit.CommitFile(t, weft, "_lyx/committed.txt", "committed\n", "a committed records file")
		if err := os.WriteFile(filepath.Join(weft, "_lyx", "uncommitted.txt"), []byte("uncommitted\n"), 0o644); err != nil {
			t.Fatalf("write the uncommitted records file: %v", err)
		}
		gitkit.CommitFile(t, h.PrimeRecords(), "_lyx/parent.txt", "parent\n", "a parent records file")
		parentTip := gitOut(t, h.PrimeRecords(), "rev-parse", "HEAD")
		origin, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found {
			t.Fatalf("ReadOriginFor(%q) = (%+v, %v, %v); want a record", slug, origin, found, err)
		}

		result, err := reset(slug)
		if err != nil {
			t.Fatalf("ResetRecordsToParent(%q): %v", slug, err)
		}
		head := gitOut(t, weft, "rev-parse", "HEAD")
		if !result.Committed || result.ResetSHA != head || result.PreCommitSHA == "" {
			t.Errorf("result = %+v; want a reset commit at HEAD %s and a pre-commit", result, head)
		}
		want := append(sortedLines(gitOut(t, h.PrimeRecords(), "ls-tree", "-r", "--name-only", parentTip)), originPath)
		slices.Sort(want)
		if got := sortedLines(gitOut(t, weft, "ls-files")); !slices.Equal(got, want) {
			t.Errorf("tracked files after the reset = %v; want the parent's plus the origin record %v", got, want)
		}
		for _, gone := range []string{"committed.txt", "uncommitted.txt"} {
			if _, err := os.Stat(filepath.Join(weft, "_lyx", gone)); !os.IsNotExist(err) {
				t.Errorf("_lyx/%s after the reset: stat error = %v; want it gone", gone, err)
			}
		}
		if _, err := os.Stat(filepath.Join(weft, "_lyx", "parent.txt")); err != nil {
			t.Errorf("_lyx/parent.txt after the reset: %v; want the parent's file", err)
		}
		// Both files stay reachable in the records history, the uncommitted one in the pre-commit.
		for _, kept := range []string{"committed.txt", "uncommitted.txt"} {
			gitOut(t, weft, "cat-file", "-e", result.PreCommitSHA+":_lyx/"+kept)
		}
		after, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found || after != origin {
			t.Errorf("origin record after the reset = (%+v, %v, %v); want %+v", after, found, err, origin)
		}

		second, err := reset(slug)
		if err != nil {
			t.Fatalf("second ResetRecordsToParent(%q): %v", slug, err)
		}
		if second.Committed || second.ResetSHA != "" {
			t.Errorf("second result = %+v; want no commit", second)
		}
		if got := gitOut(t, weft, "rev-parse", "HEAD"); got != head {
			t.Errorf("HEAD after the second reset = %s; want %s", got, head)
		}
	})

	t.Run("refuses before any commit", func(t *testing.T) {
		tests := []struct {
			name    string
			arrange func(t *testing.T, slug, weft string)
			want    []string
		}{
			{"UntrackedFileOutsideThePathspec", func(t *testing.T, slug, weft string) {
				if err := os.WriteFile(filepath.Join(weft, "stray.txt"), []byte("stray\n"), 0o644); err != nil {
					t.Fatalf("write the stray file: %v", err)
				}
			}, []string{"stray.txt", "lyx beam promote"}},
			{"MissingOriginRecord", func(t *testing.T, slug, weft string) {
				if err := os.Remove(filepath.Join(weft, originPath)); err != nil {
					t.Fatalf("delete the origin record: %v", err)
				}
			}, []string{"git checkout HEAD -- " + originPath}},
			{"MissingRecordsWorktree", func(t *testing.T, slug, weft string) {
				if err := os.RemoveAll(weft); err != nil {
					t.Fatalf("delete the records worktree: %v", err)
				}
			}, []string{"lyx fabric reconcile"}},
			{"RootPair", func(t *testing.T, slug, weft string) {
				if err := fabricengine.WriteOrigin(fabricengine.NewMutations(h.Path), l, slug, fabricengine.Origin{}); err != nil {
					t.Fatalf("rewrite the origin record: %v", err)
				}
			}, []string{"root pair", "lyx board upsert", "lyx beam create"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				slug := "refused-" + strings.ToLower(tt.name)
				hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
				weft := h.PairRecordsSibling(slug)
				before := gitOut(t, weft, "rev-parse", "HEAD")
				tt.arrange(t, slug, weft)

				result, err := reset(slug)
				if err == nil {
					t.Fatalf("ResetRecordsToParent(%q) = %+v; want a refusal", slug, result)
				}
				for _, want := range tt.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("ResetRecordsToParent(%q) error = %v; want it to contain %q", slug, err, want)
					}
				}
				if result.Committed || result.PreCommitSHA != "" {
					t.Errorf("result = %+v; want no commit", result)
				}
				if tt.name != "MissingRecordsWorktree" {
					if got := gitOut(t, weft, "rev-parse", "HEAD"); got != before {
						t.Errorf("records HEAD after the refusal = %s; want %s", got, before)
					}
				}
			})
		}
	})
}
