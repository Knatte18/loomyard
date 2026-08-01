//go:build integration

// ancestry_integration_test.go covers IsAncestor's real-git reachability
// behaviour, split into its own tagged file from ancestry_test.go's untagged
// argument-validation guard because a //go:build constraint applies per
// file, not per function.

package gitrepo_test

import "testing"

// TestIsAncestor_Reachability builds a real repo where commit B descends
// directly from commit A and asserts IsAncestor's three answer shapes:
// IsAncestor(A, B) is (true, nil) since A is reachable from B; IsAncestor(B,
// A) is (false, nil) since B is not reachable from the earlier commit A —
// not-an-ancestor is a genuine answer, not an error; and a well-formed but
// absent SHA yields an error, since merge-base cannot classify a commit this
// repo has never seen.
func TestIsAncestor_Reachability(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "first")
	commitAll(t, dir, "commit A")
	shaA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	writeFile(t, dir, "b.txt", "second")
	commitAll(t, dir, "commit B")
	shaB, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

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
}
