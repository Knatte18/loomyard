//go:build integration

// blobread_integration_test.go covers FileAtRevision and PathRevisions against real git
// repositories built fresh under t.TempDir() for each test, reusing gitrepo_test.go's newRepo,
// writeFile, and commitAll fixture helpers.

package gitrepo_test

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// TestFileAtRevision_ReturnsExactStoredBytesAtOlderCommit asserts FileAtRevision returns a file's
// exact stored bytes at an older commit, unaffected by a later change to the working-tree copy.
func TestFileAtRevision_ReturnsExactStoredBytesAtOlderCommit(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "version one")
	commitAll(t, dir, "commit A")
	shaA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	writeFile(t, dir, "a.txt", "version two")
	commitAll(t, dir, "commit B")

	got, err := repo.FileAtRevision(shaA, "a.txt")
	if err != nil {
		t.Fatalf("FileAtRevision(%s, a.txt) error = %v; want nil", shaA, err)
	}
	if string(got) != "version one" {
		t.Errorf("FileAtRevision(%s, a.txt) = %q; want %q", shaA, got, "version one")
	}
}

// TestFileAtRevision_AbsentPathReturnsErrPathNotAtRevision asserts FileAtRevision returns
// ErrPathNotAtRevision for a path absent from that revision's tree, distinguishably from a
// malformed-revision error.
func TestFileAtRevision_AbsentPathReturnsErrPathNotAtRevision(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "commit A")
	sha, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	_, err = repo.FileAtRevision(sha, "missing.txt")
	if !errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		t.Errorf("FileAtRevision(%s, missing.txt) error = %v; want ErrPathNotAtRevision", sha, err)
	}
}

// TestFileAtRevision_InvalidSHAReturnsError asserts FileAtRevision returns an error for an invalid
// SHA, distinct from ErrPathNotAtRevision.
func TestFileAtRevision_InvalidSHAReturnsError(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "commit A")

	_, err := repo.FileAtRevision("not-a-sha!!", "a.txt")
	if err == nil {
		t.Fatal("FileAtRevision(invalid SHA, a.txt) error = nil; want an error")
	}
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		t.Errorf("FileAtRevision(invalid SHA, a.txt) error = ErrPathNotAtRevision; want a distinct invalid-SHA error")
	}
}

// TestPathRevisions_NewestFirstAndLimit asserts PathRevisions returns the touching commits
// newest-first and respects a positive limit.
func TestPathRevisions_NewestFirstAndLimit(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "version one")
	commitAll(t, dir, "commit A")
	shaA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	writeFile(t, dir, "a.txt", "version two")
	commitAll(t, dir, "commit B")
	shaB, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

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
}

// TestPathRevisions_NoHistoryReturnsEmptySlice asserts PathRevisions returns an empty slice with no
// error for a path that has no history.
func TestPathRevisions_NoHistoryReturnsEmptySlice(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "commit A")

	got, err := repo.PathRevisions("never-touched.txt", 0)
	if err != nil {
		t.Fatalf("PathRevisions(never-touched.txt, 0) error = %v; want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("PathRevisions(never-touched.txt, 0) = %v; want empty slice", got)
	}
}
