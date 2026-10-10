//go:build integration

// blobread_integration_test.go covers the history reads — FileAtRevision, FilesInDirAtRevision, PathRevisions, CommitsWithSubject, CommitsNotIn and IsAncestor's real-git reachability — against one real git repository with three commits built under t.TempDir(), reusing gitrepo_test.go's newRepo, writeFile, and commitAll fixture helpers.
// IsAncestor's argument-validation guard lives in the untagged ancestry_test.go, because a //go:build constraint applies per file, not per function.

package gitrepo_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// commitAllAt makes an empty commit in dir whose committer date is committerDate, which gitkit.Git cannot set without a process-global env change.
func commitAllAt(t *testing.T, dir, message, committerDate string) {
	t.Helper()

	cmd := exec.Command("git", "commit", "--allow-empty", "-m", message)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+committerDate, "GIT_AUTHOR_DATE="+committerDate)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit %q in %s: %v; output: %s", message, dir, err, out)
	}
}

// TestHistoryReads drives the history reads over one repository: commit A writes a.txt as "version one", commit B rewrites it as "version two" and commit C adds an unrelated file and a directory holding two files and a subdirectory;
// a second branch and a tag then point at commit C.
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
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "sub"), 0o755); err != nil {
		t.Fatalf("mkdir pkg/sub: %v", err)
	}
	writeFile(t, dir, "pkg/b.go", "package pkg")
	writeFile(t, dir, "pkg/a_test.go", "package pkg")
	writeFile(t, dir, "pkg/sub/c.go", "package sub")
	commitAll(t, dir, "commit C")
	shaC := requireCurrentSHA(t, repo)
	gitkit.Git(t, dir, "branch", "side", shaC)
	gitkit.Git(t, dir, "tag", "v1", shaC)

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
		// FilesInDirAtRevision lists only the regular files directly in the directory, sorted;
		// an absent directory is empty and a malformed revision is ErrInvalidSHA.
		{"FilesInDirAtRevision lists the files directly in a directory", func(t *testing.T) {
			got, err := repo.FilesInDirAtRevision(shaC, "pkg")
			if err != nil {
				t.Fatalf("FilesInDirAtRevision(C, pkg) error = %v; want nil", err)
			}
			if want := []string{"a_test.go", "b.go"}; !reflect.DeepEqual(got, want) {
				t.Errorf("FilesInDirAtRevision(C, pkg) = %v; want %v", got, want)
			}

			got, err = repo.FilesInDirAtRevision(shaB, "pkg")
			if err != nil {
				t.Fatalf("FilesInDirAtRevision(B, pkg) error = %v; want nil (absent directory)", err)
			}
			if len(got) != 0 {
				t.Errorf("FilesInDirAtRevision(B, pkg) = %v; want empty (pkg is absent at B)", got)
			}

			if _, err := repo.FilesInDirAtRevision("not-a-sha!!", "pkg"); !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Errorf("FilesInDirAtRevision(invalid SHA, pkg) error = %v; want ErrInvalidSHA", err)
			}
		}},
		// A commit reached by two branches and a tag is found once;
		// an absent subject finds nothing.
		{"CommitsWithSubject finds a commit once however many refs reach it", func(t *testing.T) {
			got, err := repo.CommitsWithSubject("commit C")
			if err != nil {
				t.Fatalf("CommitsWithSubject(commit C) error = %v; want nil", err)
			}
			if len(got) != 1 || got[0].SHA != shaC || got[0].Committed.IsZero() {
				t.Errorf("CommitsWithSubject(commit C) = %v; want one commit %s with a committer time", got, shaC)
			}

			got, err = repo.CommitsWithSubject("no such subject")
			if err != nil {
				t.Fatalf("CommitsWithSubject(absent) error = %v; want nil", err)
			}
			if len(got) != 0 {
				t.Errorf("CommitsWithSubject(absent) = %v; want none", got)
			}
		}},
		// HeadContains is true for HEAD's own commit and an earlier one, false for a commit only an unmerged side branch holds, a well-formed sha in no object store, and an unborn HEAD, and ErrInvalidSHA for a malformed sha.
		{"HeadContains answers whether HEAD's history holds a commit", func(t *testing.T) {
			sideDir, sideRepo := newRepo(t)
			writeFile(t, sideDir, "x.txt", "base")
			commitAll(t, sideDir, "base")
			gitkit.Git(t, sideDir, "checkout", "-b", "side")
			writeFile(t, sideDir, "x.txt", "side only")
			commitAll(t, sideDir, "side only")
			shaSide := requireCurrentSHA(t, sideRepo)
			gitkit.Git(t, sideDir, "checkout", "main")

			_, unbornRepo := newRepo(t)

			const absentSHA = "0123456789abcdef0123456789abcdef01234567"
			tests := []struct {
				name    string
				repo    *gitrepo.Repo
				sha     string
				want    bool
				wantErr error
			}{
				{"HEAD's own commit", repo, shaC, true, nil},
				{"an earlier commit", repo, shaA, true, nil},
				{"a commit only a side branch holds", sideRepo, shaSide, false, nil},
				{"a well-formed sha in no object store", repo, absentSHA, false, nil},
				{"a malformed sha", repo, "not-a-sha!!", false, gitrepo.ErrInvalidSHA},
				{"an unborn repository", unbornRepo, shaC, false, nil},
			}
			for _, tc := range tests {
				got, err := tc.repo.HeadContains(tc.sha)
				if !errors.Is(err, tc.wantErr) || got != tc.want {
					t.Errorf("%s: HeadContains() = (%v, %v); want (%v, %v)", tc.name, got, err, tc.want, tc.wantErr)
				}
			}
		}},
		// The target sits on a side branch, dated days after three old commits, and HEAD sits after the target;
		// deleting the oldest old commit's object means only a walk that stops at the bound can answer.
		// A second target under skewed descendants pins that the bound lets a walk through commits within the slack.
		{"HeadContains stops walking at the committer-time bound", func(t *testing.T) {
			boundDir, boundRepo := newRepo(t)
			commitAllAt(t, boundDir, "old one", "2024-01-01T12:00:00Z")
			oldestSHA := requireCurrentSHA(t, boundRepo)
			commitAllAt(t, boundDir, "old two", "2024-01-02T12:00:00Z")
			commitAllAt(t, boundDir, "old three", "2024-01-03T12:00:00Z")
			gitkit.Git(t, boundDir, "checkout", "-b", "side")
			commitAllAt(t, boundDir, "target", "2024-01-10T12:00:00Z")
			shaTarget := requireCurrentSHA(t, boundRepo)
			gitkit.Git(t, boundDir, "checkout", "main")
			commitAllAt(t, boundDir, "head", "2024-01-11T12:00:00Z")

			if err := os.Remove(filepath.Join(boundDir, ".git", "objects", oldestSHA[:2], oldestSHA[2:])); err != nil {
				t.Fatalf("delete the oldest commit's loose object: %v", err)
			}

			got, err := boundRepo.HeadContains(shaTarget)
			if err != nil || got {
				t.Errorf("HeadContains(target on a side branch) = (%v, %v); want (false, nil) without reading below the bound", got, err)
			}

			// Two descendants of a second target carry committer times hours before it, as a skewed clock writes them;
			// the walk goes on through them within the slack and finds the target.
			commitAllAt(t, boundDir, "skewed target", "2024-01-12T12:00:00Z")
			shaSkewedTarget := requireCurrentSHA(t, boundRepo)
			commitAllAt(t, boundDir, "skewed child", "2024-01-12T02:00:00Z")
			commitAllAt(t, boundDir, "skewed head", "2024-01-12T01:00:00Z")

			got, err = boundRepo.HeadContains(shaSkewedTarget)
			if err != nil || !got {
				t.Errorf("HeadContains(target under descendants 10 and 11 hours older) = (%v, %v); want (true, nil) within the slack", got, err)
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
