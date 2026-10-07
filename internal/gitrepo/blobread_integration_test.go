//go:build integration

// blobread_integration_test.go covers the history reads — FileAtRevision, PathRevisions, CommitsNotIn and IsAncestor's real-git reachability — against one real git repository with three commits built under t.TempDir(), reusing gitrepo_test.go's newRepo, writeFile, and commitAll fixture helpers.
// IsAncestor's argument-validation guard lives in the untagged ancestry_test.go, because a //go:build constraint applies per file, not per function.

package gitrepo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// TestHistoryReads drives the history reads over one repository: commit A writes a.txt as "version one", commit B rewrites it as "version two" and commit C adds an unrelated file.
// Every step only reads, so the steps run serially in one order over the shared repository and depend on nothing an earlier step does.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestHistoryReads(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "version one")
	commitAll(t, dir, "commit A")
	shaA := requireCurrentSHA(t, repo)
	writeFile(t, dir, "a.txt", "version two")
	commitAll(t, dir, "commit B")
	shaB := requireCurrentSHA(t, repo)
	writeFile(t, dir, "other.txt", "unrelated")
	commitAll(t, dir, "commit C")
	shaC := requireCurrentSHA(t, repo)

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		// IsAncestor answers (true, nil) when an ancestor, (false, nil) when not, and an error for an absent SHA.
		{"IsAncestor answers reachability", func(t *testing.T) {
			got, err := repo.IsAncestor(shaA, shaB)
			if err != nil {
				t.Fatalf("IsAncestor(A, B) error = %v; want nil", err)
			}
			if !got {
				t.Errorf("IsAncestor(A, B) = %v; want true (A is an ancestor of B)", got)
			}

			got, err = repo.IsAncestor(shaB, shaA)
			if err != nil {
				t.Fatalf("IsAncestor(B, A) error = %v; want nil", err)
			}
			if got {
				t.Errorf("IsAncestor(B, A) = %v; want false (B is not an ancestor of A)", got)
			}

			const absentSHA = "0123456789abcdef0123456789abcdef01234567"
			if _, err := repo.IsAncestor(absentSHA, shaB); err == nil {
				t.Fatal("IsAncestor(absent SHA, B) error = nil; want an error (merge-base cannot classify an unknown commit)")
			}
		}},
		// CommitsNotIn lists the commits past the base newest first, nothing when the tip is behind the base, and errors on an unknown object.
		{"CommitsNotIn lists the commits past a base newest first", func(t *testing.T) {
			got, err := repo.CommitsNotIn(shaC, shaA)
			if err != nil {
				t.Fatalf("CommitsNotIn(C, A) error = %v; want nil", err)
			}
			if want := []string{shaC, shaB}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
				t.Errorf("CommitsNotIn(C, A) = %v; want %v (newest first)", got, want)
			}

			got, err = repo.CommitsNotIn(shaA, shaC)
			if err != nil {
				t.Fatalf("CommitsNotIn(A, C) error = %v; want nil", err)
			}
			if len(got) != 0 {
				t.Errorf("CommitsNotIn(A, C) = %v; want none (A is an ancestor of C)", got)
			}

			const absentSHA = "0123456789abcdef0123456789abcdef01234567"
			_, err = repo.CommitsNotIn(absentSHA, shaA)
			if err == nil {
				t.Fatal("CommitsNotIn(absent SHA, A) error = nil; want an error")
			}
			if !strings.Contains(err.Error(), absentSHA) {
				t.Errorf("CommitsNotIn(absent SHA, A) error = %v; want it to name %s", err, absentSHA)
			}
		}},
		// FileAtRevision returns a file's exact stored bytes at an older commit, unaffected by a later change to the working-tree copy.
		{"FileAtRevision returns the exact bytes at an older commit", func(t *testing.T) {
			got, err := repo.FileAtRevision(shaA, "a.txt")
			if err != nil {
				t.Fatalf("FileAtRevision(%s, a.txt) error = %v; want nil", shaA, err)
			}
			if string(got) != "version one" {
				t.Errorf("FileAtRevision(%s, a.txt) = %q; want %q", shaA, got, "version one")
			}
		}},
		// ErrPathNotAtRevision for a path absent from that revision's tree is distinguishable from a malformed-revision error.
		{"FileAtRevision distinguishes an absent path from an invalid sha", func(t *testing.T) {
			_, err := repo.FileAtRevision(shaB, "missing.txt")
			if !errors.Is(err, gitrepo.ErrPathNotAtRevision) {
				t.Errorf("FileAtRevision(%s, missing.txt) error = %v; want ErrPathNotAtRevision", shaB, err)
			}

			_, err = repo.FileAtRevision("not-a-sha!!", "a.txt")
			if err == nil {
				t.Fatal("FileAtRevision(invalid SHA, a.txt) error = nil; want an error")
			}
			if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
				t.Errorf("FileAtRevision(invalid SHA, a.txt) error = ErrPathNotAtRevision; want a distinct invalid-SHA error")
			}
		}},
		{"PathRevisions returns the touching commits newest first and respects a limit", func(t *testing.T) {
			got, err := repo.PathRevisions("a.txt", 0)
			if err != nil {
				t.Fatalf("PathRevisions(a.txt, 0) error = %v; want nil", err)
			}
			want := []string{shaB, shaA}
			if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
				t.Errorf("PathRevisions(a.txt, 0) = %v; want %v (newest first)", got, want)
			}

			limited, err := repo.PathRevisions("a.txt", 1)
			if err != nil {
				t.Fatalf("PathRevisions(a.txt, 1) error = %v; want nil", err)
			}
			if len(limited) != 1 || limited[0] != shaB {
				t.Errorf("PathRevisions(a.txt, 1) = %v; want [%s]", limited, shaB)
			}
		}},
		{"PathRevisions returns an empty slice for a path with no history", func(t *testing.T) {
			got, err := repo.PathRevisions("never-touched.txt", 0)
			if err != nil {
				t.Fatalf("PathRevisions(never-touched.txt, 0) error = %v; want nil", err)
			}
			if len(got) != 0 {
				t.Errorf("PathRevisions(never-touched.txt, 0) = %v; want empty slice", got)
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}
