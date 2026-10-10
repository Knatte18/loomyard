//go:build integration

// gitwrap_test.go exercises headSHA, dirty, otherWorktrees, reconcileReportHead and refuseMidMerge against real scratch git repositories built fresh under t.TempDir() for each test,
// reusing the package's existing hermetic TestMain (testmain_test.go) so these git spawns never inherit the operator's global gitconfig.

package websterengine

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// gitwrapNewScratchRepo is the in-package scratch-repo initialiser; the external test package keeps its own, since the two packages cannot share a helper.
func gitwrapNewScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gitkit.Git(t, dir, "init")
	gitkit.Git(t, dir, "config", "user.name", "Test User")
	gitkit.Git(t, dir, "config", "user.email", "test@example.com")

	return dir
}

// TestRepositoryProbes walks one scratch repository through the real-git probes: headSHA returns the
// commit just made, dirty is false on a clean tree and true once an untracked file appears, and
// otherWorktrees names, from either side, the one other worktree of the repository.
//
//testtiming:keep pins the real-git head, dirty and worktree-list probes' own return values, an untracked file counting as dirty, and both sides of a worktree pair; the covering verb tests observe them only through verb outcomes
func TestRepositoryProbes(t *testing.T) {
	t.Parallel()

	dir := gitwrapNewScratchRepo(t)
	want := gitkit.CommitFile(t, dir, "a.txt", "one", "first")

	t.Run("headSHA returns HEAD", func(t *testing.T) {
		got, err := headSHA(dir)
		if err != nil {
			t.Fatalf("headSHA() error = %v; want nil", err)
		}
		if got != want {
			t.Errorf("headSHA() = %q; want %q", got, want)
		}
	})

	t.Run("dirty is false right after a commit", func(t *testing.T) {
		isDirty, err := dirty(dir)
		if err != nil {
			t.Fatalf("dirty() error = %v; want nil", err)
		}
		if isDirty {
			t.Errorf("dirty() = true right after a commit with no other changes; want false")
		}
	})

	t.Run("otherWorktrees names the other side", func(t *testing.T) {
		added := filepath.Join(t.TempDir(), "added")
		gitkit.Git(t, dir, "worktree", "add", added)
		mainCanon, err := canonicalPath(dir)
		if err != nil {
			t.Fatal(err)
		}
		addedCanon, err := canonicalPath(added)
		if err != nil {
			t.Fatal(err)
		}
		for _, side := range []struct{ from, want string }{{dir, addedCanon}, {added, mainCanon}} {
			got, err := otherWorktrees(side.from)
			if err != nil {
				t.Fatalf("otherWorktrees(%s): %v", side.from, err)
			}
			if len(got) != 1 || got[0] != side.want {
				t.Errorf("otherWorktrees(%s) = %v; want [%s]", side.from, got, side.want)
			}
		}
	})

	t.Run("commitsNamedBy matches commit object names only", func(t *testing.T) {
		blob := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "HEAD:a.txt"))
		for _, tt := range []struct {
			name   string
			prefix string
			want   []string
		}{
			{"a prefix of HEAD names HEAD alone", want[:9], []string{want}},
			{"a prefix of a blob id names no commit", blob[:9], nil},
			{"a prefix naming no object names no commit", "0000000", nil},
		} {
			got, err := commitsNamedBy(dir, tt.prefix)
			if err != nil {
				t.Fatalf("%s: commitsNamedBy(%q) error = %v; want nil", tt.name, tt.prefix, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("%s: commitsNamedBy(%q) = %v; want %v", tt.name, tt.prefix, got, tt.want)
			}
		}
	})

	t.Run("nonMergeCommitsBetween sees a commit past the start, ignores merge-ins and refuses an absent start", func(t *testing.T) {
		repo := gitwrapNewScratchRepo(t)
		start := gitkit.CommitFile(t, repo, "a.txt", "one", "first")
		branch := strings.TrimSpace(gitkit.Git(t, repo, "branch", "--show-current"))
		gitkit.Git(t, repo, "switch", "-c", "side")
		gitkit.CommitFile(t, repo, "side.txt", "side", "side work")
		gitkit.Git(t, repo, "switch", branch)
		gitkit.Git(t, repo, "merge", "--no-ff", "-m", "merge side", "side")
		mergedHead := strings.TrimSpace(gitkit.Git(t, repo, "rev-parse", "HEAD"))
		ownHead := gitkit.CommitFile(t, repo, "b.txt", "two", "own work")

		for _, tt := range []struct {
			name    string
			base    string
			head    string
			want    bool
			wantErr bool
		}{
			{name: "a commit past the start is true", base: start, head: ownHead, want: true},
			{name: "a first-parent range of merge-ins only is false", base: start, head: mergedHead},
			{name: "an empty range is false", base: ownHead, head: ownHead},
			{name: "a start absent from the store is an error", base: strings.Repeat("0", 40), head: ownHead, wantErr: true},
		} {
			got, err := nonMergeCommitsBetween(repo, tt.base, tt.head)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("%s: nonMergeCommitsBetween() = (%v, %v); want (%v, error %v)", tt.name, got, err, tt.want, tt.wantErr)
			}
		}
	})

	t.Run("commitsFromBatchCheck keeps the commit lines", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			output  string
			want    []string
			wantErr string
		}{
			{name: "commit, blob and tree lines return the commit alone", output: "aaaa commit 230\nbbbb blob 12\ncccc tree 33\n", want: []string{"aaaa"}},
			{name: "a missing line names the object", output: "aaaa commit 230\nbbbb missing\n", wantErr: "bbbb"},
			{name: "a malformed line is an error", output: "aaaa commit\n", wantErr: "malformed"},
			{name: "empty output returns nothing", output: ""},
		} {
			got, err := commitsFromBatchCheck(tt.output)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("%s: error = %v; want one containing %q", tt.name, err, tt.wantErr)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s: error = %v; want nil", tt.name, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("%s: got %v; want %v", tt.name, got, tt.want)
			}
		}
	})

	// Runs last: the untracked file it writes leaves the tree dirty.
	t.Run("dirty is true with an untracked file", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write untracked file: %v", err)
		}
		isDirty, err := dirty(dir)
		if err != nil {
			t.Fatalf("dirty() error = %v; want nil", err)
		}
		if !isDirty {
			t.Errorf("dirty() = false with an untracked file present; want true")
		}
	})
}

// gitwrapObjectNamedLike hashes candidate objects of kind with body from bodyFor(0), bodyFor(1), … and returns the first body whose object id starts with prefix,
// so a test can plant a blob and a tree sharing the commit's abbreviated name.
func gitwrapObjectNamedLike(t *testing.T, kind, prefix string, bodyFor func(attempt int) []byte) []byte {
	t.Helper()
	for attempt := 0; attempt < 5_000_000; attempt++ {
		body := bodyFor(attempt)
		sum := sha1.Sum(append([]byte(fmt.Sprintf("%s %d\x00", kind, len(body))), body...))
		if strings.HasPrefix(hex.EncodeToString(sum[:]), prefix) {
			return body
		}
	}
	t.Fatalf("no %s body hashes to prefix %s", kind, prefix)
	return nil
}

// TestCommitsNamedBy_OneBatchCheck pins that commitsNamedBy classifies every object a prefix names in a single `cat-file --batch-check` call, never one `cat-file -t` per object:
// a recording git first on PATH logs each invocation, over a prefix naming a commit, a blob and a tree.
// It sets PATH, process-global state, so it runs without t.Parallel.
func TestCommitsNamedBy_OneBatchCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the recording git is a shell script")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git not on PATH: %v", err)
	}

	dir := gitwrapNewScratchRepo(t)
	commit := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	prefix := commit[:4]

	blobBody := gitwrapObjectNamedLike(t, "blob", prefix, func(attempt int) []byte { return []byte(fmt.Sprintf("blob %d\n", attempt)) })
	blob, err := gitexec.RunStdin([]string{"hash-object", "-w", "--stdin"}, dir, string(blobBody))
	if err != nil {
		t.Fatalf("write blob: %v", err)
	}
	blob = strings.TrimSpace(blob)
	rawBlob, err := hex.DecodeString(blob)
	if err != nil {
		t.Fatalf("decode blob id %q: %v", blob, err)
	}
	treeBody := gitwrapObjectNamedLike(t, "tree", prefix, func(attempt int) []byte {
		return append([]byte(fmt.Sprintf("100644 f%d\x00", attempt)), rawBlob...)
	})
	entryName := strings.TrimPrefix(strings.SplitN(string(treeBody), "\x00", 2)[0], "100644 ")
	tree, err := gitexec.RunStdin([]string{"mktree"}, dir, fmt.Sprintf("100644 blob %s\t%s\n", blob, entryName))
	if err != nil {
		t.Fatalf("write tree: %v", err)
	}
	if strings.TrimSpace(tree)[:4] != prefix || blob[:4] != prefix {
		t.Fatalf("planted blob %s and tree %s do not share prefix %s", blob, strings.TrimSpace(tree), prefix)
	}

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "git-calls.log")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %q\nexec %q \"$@\"\n", logPath, realGit)
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("write recording git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := commitsNamedBy(dir, prefix)
	if err != nil {
		t.Fatalf("commitsNamedBy(%q) error = %v; want nil", prefix, err)
	}
	if !slices.Equal(got, []string{commit}) {
		t.Errorf("commitsNamedBy(%q) = %v; want [%s]", prefix, got, commit)
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	var batchChecks int
	for _, call := range strings.Split(strings.TrimSpace(string(logged)), "\n") {
		if strings.HasPrefix(call, "cat-file -t") {
			t.Errorf("per-object call %q; want one cat-file --batch-check", call)
		}
		if call == "cat-file --batch-check" {
			batchChecks++
		}
	}
	if batchChecks != 1 {
		t.Errorf("cat-file --batch-check calls = %d; want 1 in:\n%s", batchChecks, logged)
	}
}

// gitwrapParentBranch is the parent branch every reconcileReportHead test's run merges from.
const gitwrapParentBranch = "parent"

// gitwrapParent is the ParentBranchFunc naming gitwrapParentBranch.
func gitwrapParent() (string, error) { return gitwrapParentBranch, nil }

// gitwrapMergeSide checks out side (branching it off the current branch when it does not exist yet), commits one new file there,
// returns to the original branch and merges side with --no-ff, so a merge commit lands even without divergence.
// It returns the merge commit SHA and the side branch's own tip SHA.
func gitwrapMergeSide(t *testing.T, dir, side string) (mergeSHA, sideTip string) {
	t.Helper()
	base := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	baseHead := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "HEAD"))
	if _, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + side}, dir); err == nil && exitCode == 0 {
		gitkit.Git(t, dir, "checkout", side)
	} else {
		gitkit.Git(t, dir, "checkout", "-b", side)
	}
	sideTip = gitkit.CommitFile(t, dir, side+"-"+baseHead[:12]+".txt", side, side+" commit")
	gitkit.Git(t, dir, "checkout", base)
	gitkit.Git(t, dir, "merge", "--no-ff", "-m", "merge "+side, side)
	return strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "HEAD")), sideTip
}

func TestReconcileReportHead_EqualIsFastPath(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	head := gitkit.CommitFile(t, dir, "a.txt", "one", "first")

	warning, err := reconcileReportHead(realGit{}, dir, head, "batch report x", gitwrapParent, 1)
	if err != nil || warning != "" {
		t.Fatalf("reconcileReportHead() = (%q, %v); want (\"\", nil)", warning, err)
	}
}

func TestReconcileReportHead_MergesOnTopAccepted(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	merge1, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)

	warning, err := reconcileReportHead(realGit{}, dir, report, "batch report x", gitwrapParent, 1)
	if err != nil {
		t.Fatalf("one merge: error = %v; want nil", err)
	}
	for _, want := range []string{"batch report x", report, merge1} {
		if !strings.Contains(warning, want) {
			t.Errorf("one merge: warning %q missing %q", warning, want)
		}
	}

	merge2, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)
	warning, err = reconcileReportHead(realGit{}, dir, report, "batch report x", gitwrapParent, 1)
	if err != nil {
		t.Fatalf("two merges: error = %v; want nil", err)
	}
	i2, i1 := strings.Index(warning, merge2), strings.Index(warning, merge1)
	if i2 < 0 || i1 < 0 || i2 > i1 {
		t.Errorf("two merges: warning %q must name %s then %s", warning, merge2, merge1)
	}
}

func TestReconcileReportHead_ReportHeadIsMergeCommit(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	report, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)
	merge2, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)

	warning, err := reconcileReportHead(realGit{}, dir, report, "batch report x", gitwrapParent, 1)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	if !strings.Contains(warning, "("+merge2+")") {
		t.Errorf("warning %q; want the walked merges to be exactly (%s), without the report head", warning, merge2)
	}
}

func TestReconcileReportHead_Refusals(t *testing.T) {
	t.Parallel()

	t.Run("fast-forward onto side commits", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		base := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
		gitkit.Git(t, dir, "checkout", "-b", "side")
		gitkit.CommitFile(t, dir, "s.txt", "s", "side commit")
		gitkit.Git(t, dir, "checkout", base)
		gitkit.Git(t, dir, "merge", "--ff-only", "side")

		if _, err := reconcileReportHead(realGit{}, dir, report, "batch report x", gitwrapParent, 1); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})

	t.Run("non-merge commit after report head", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		head := gitkit.CommitFile(t, dir, "b.txt", "two", "second")

		_, err := reconcileReportHead(realGit{}, dir, report, "batch report x", gitwrapParent, 1)
		if err == nil {
			t.Fatal("error = nil; want refusal")
		}
		for _, want := range []string{"does not match the worktree's actual HEAD", report, head, "merge"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q missing %q", err, want)
			}
		}
	})

	t.Run("non-merge commit between merges", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		gitwrapMergeSide(t, dir, gitwrapParentBranch)
		gitkit.CommitFile(t, dir, "b.txt", "two", "second")
		head, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)

		_, err := reconcileReportHead(realGit{}, dir, report, "batch report x", gitwrapParent, 1)
		if err == nil {
			t.Fatal("error = nil; want refusal")
		}
		for _, want := range []string{"does not match the worktree's actual HEAD", report, head, "only merge commits"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q missing %q", err, want)
			}
		}
	})

	t.Run("report head only on merged-in side branch", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		_, sideTip := gitwrapMergeSide(t, dir, gitwrapParentBranch)

		if _, err := reconcileReportHead(realGit{}, dir, sideTip, "batch report x", gitwrapParent, 1); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})

	t.Run("all-zero report head", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		gitwrapMergeSide(t, dir, gitwrapParentBranch)

		zero := strings.Repeat("0", 40)
		if _, err := reconcileReportHead(realGit{}, dir, zero, "batch report x", gitwrapParent, 1); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})
}

// gitwrapParentCommit commits name=content on the parent branch (creating it off the current branch when absent) and returns to the current branch,
// returning the current branch's name.
func gitwrapParentCommit(t *testing.T, dir, name, content string) (base string) {
	t.Helper()
	base = strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	if _, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + gitwrapParentBranch}, dir); err == nil && exitCode == 0 {
		gitkit.Git(t, dir, "checkout", gitwrapParentBranch)
	} else {
		gitkit.Git(t, dir, "checkout", "-b", gitwrapParentBranch)
	}
	gitkit.CommitFile(t, dir, name, content, "parent: "+name)
	gitkit.Git(t, dir, "checkout", base)
	return base
}

// TestReconcileReportHead_UncleanParentMergesRefused proves a merge commit is walked over only when it is a clean merge of the run's parent branch:
// every other two-or-more-parent shape is refused with the parent-merge refusal and its way forward.
func TestReconcileReportHead_UncleanParentMergesRefused(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		parent ParentBranchFunc
		// move builds the moved HEAD on top of the report head in dir.
		move       func(t *testing.T, dir string)
		wantReason string
	}{
		{
			name:   "evil merge carrying an extra edit",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				gitwrapParentCommit(t, dir, "p.txt", "p")
				gitkit.Git(t, dir, "merge", "--no-ff", "--no-commit", gitwrapParentBranch)
				if err := os.WriteFile(filepath.Join(dir, "smuggled.txt"), []byte("unaudited"), 0o644); err != nil {
					t.Fatalf("write smuggled file: %v", err)
				}
				gitkit.Git(t, dir, "add", "smuggled.txt")
				gitkit.Git(t, dir, "commit", "--no-edit")
			},
			wantReason: "carries changes beyond a clean merge",
		},
		{
			name:   "merge of a non-parent branch",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				gitwrapParentCommit(t, dir, "p.txt", "p")
				gitwrapMergeSide(t, dir, "other")
			},
			wantReason: "not on the run's parent branch",
		},
		{
			name:   "hand-made two-parent commit with an arbitrary tree",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				gitwrapParentCommit(t, dir, "p.txt", "p")
				gitkit.CommitFile(t, dir, "scratch.txt", "arbitrary", "scratch")
				tree := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "HEAD^{tree}"))
				gitkit.Git(t, dir, "reset", "--hard", "HEAD~1")
				forged := strings.TrimSpace(gitkit.Git(t, dir, "commit-tree", tree, "-p", "HEAD", "-p", gitwrapParentBranch, "-m", "forged merge"))
				gitkit.Git(t, dir, "reset", "--hard", forged)
			},
			wantReason: "carries changes beyond a clean merge",
		},
		{
			name:   "hand-resolved conflicting parent merge",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				gitwrapParentCommit(t, dir, "a.txt", "parent side")
				gitkit.CommitFile(t, dir, "a.txt", "our side", "our side")
				if _, _, exitCode, err := gitexec.RunGit([]string{"merge", "--no-ff", gitwrapParentBranch}, dir); err != nil || exitCode == 0 {
					t.Fatalf("expected a conflicting merge; exit %d err %v", exitCode, err)
				}
				if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("resolved"), 0o644); err != nil {
					t.Fatalf("resolve conflict: %v", err)
				}
				gitkit.Git(t, dir, "add", "a.txt")
				gitkit.Git(t, dir, "commit", "--no-edit")
			},
			wantReason: "do not merge cleanly",
		},
		{
			name:   "octopus merge",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				// Two independent lines, both merged into the parent branch, so each octopus head is reachable from its tip.
				base := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
				for _, line := range []string{"line-a", "line-b"} {
					gitkit.Git(t, dir, "checkout", "-b", line, base)
					gitkit.CommitFile(t, dir, line+".txt", line, line+" commit")
				}
				gitkit.Git(t, dir, "checkout", "-b", gitwrapParentBranch, "line-a")
				gitkit.Git(t, dir, "merge", "--no-ff", "-m", "parent takes line-b", "line-b")
				gitkit.Git(t, dir, "checkout", base)
				gitkit.Git(t, dir, "merge", "--no-ff", "-m", "octopus", "line-a", "line-b")
			},
			wantReason: "has 3 parents",
		},
		{
			name:   "no known parent branch",
			parent: nil,
			move: func(t *testing.T, dir string) {
				gitwrapMergeSide(t, dir, gitwrapParentBranch)
			},
			wantReason: "no known parent branch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := gitwrapNewScratchRepo(t)
			report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
			tc.move(t, dir)
			head := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "HEAD"))
			if head == report {
				t.Fatal("move left HEAD at the report head")
			}

			_, err := reconcileReportHead(realGit{}, dir, report, "batch report x", tc.parent, 1)
			if err == nil {
				t.Fatal("error = nil; want refusal")
			}
			for _, want := range []string{"does not match the worktree's actual HEAD", report, head, "merge commit " + head + " does not qualify", tc.wantReason, "way forward: 1) lyx webster reset --to report-head --batch 01"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err, want)
				}
			}
		})
	}
}

// TestReconcileReportHead_ParentOnlyOnOrigin proves a merge of the parent branch's origin remote-tracking tip is accepted when no local parent branch exists.
func TestReconcileReportHead_ParentOnlyOnOrigin(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	merge, sideTip := gitwrapMergeSide(t, dir, gitwrapParentBranch)
	gitkit.Git(t, dir, "update-ref", "refs/remotes/origin/"+gitwrapParentBranch, sideTip)
	gitkit.Git(t, dir, "branch", "-D", gitwrapParentBranch)

	warning, err := reconcileReportHead(realGit{}, dir, report, "batch report x", gitwrapParent, 1)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	if !strings.Contains(warning, "("+merge+")") {
		t.Errorf("warning %q; want the walked merge (%s)", warning, merge)
	}
}

// gitwrapConflictingMerge sets up a conflict on file c.txt between the current branch and a side branch, leaving the merge in progress in dir.
func gitwrapConflictingMerge(t *testing.T, dir string) {
	t.Helper()
	base := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	gitkit.CommitFile(t, dir, "c.txt", "base", "add c")
	gitkit.Git(t, dir, "checkout", "-b", "conflict-side")
	gitkit.CommitFile(t, dir, "c.txt", "side", "side c")
	gitkit.Git(t, dir, "checkout", base)
	gitkit.CommitFile(t, dir, "c.txt", "main", "main c")
	if _, _, exitCode, err := gitexec.RunGit([]string{"merge", "conflict-side"}, dir); err != nil || exitCode == 0 {
		t.Fatalf("expected a conflicting merge; exit %d err %v", exitCode, err)
	}
}

func TestRefuseMidMerge(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	gitkit.CommitFile(t, dir, "a.txt", "one", "first")

	if err := refuseMidMerge(realGit{}, dir); err != nil {
		t.Fatalf("clean repo: error = %v; want nil", err)
	}

	gitwrapConflictingMerge(t, dir)
	err := refuseMidMerge(realGit{}, dir)
	if err == nil {
		t.Fatal("mid-merge: error = nil; want refusal")
	}
	for _, want := range []string{"lyx fabric merge --continue", "lyx fabric merge --abort", "git merge --continue", "git merge --abort", "a session lyx refuses the verb from reports status: FAILED and the orch runs it"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}

	gitkit.Git(t, dir, "merge", "--abort")
	if err := refuseMidMerge(realGit{}, dir); err != nil {
		t.Fatalf("after abort: error = %v; want nil", err)
	}
}

func TestRefuseMidMerge_LinkedWorktree(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	linked := filepath.Join(t.TempDir(), "linked")
	gitkit.Git(t, dir, "worktree", "add", "-b", "linked-branch", linked)
	gitkit.Git(t, linked, "config", "user.name", "Test User")
	gitkit.Git(t, linked, "config", "user.email", "test@example.com")

	gitwrapConflictingMerge(t, linked)
	err := refuseMidMerge(realGit{}, linked)
	if err == nil || !strings.Contains(err.Error(), "merge in progress") {
		t.Fatalf("linked worktree: error = %v; want merge-in-progress refusal", err)
	}
}
