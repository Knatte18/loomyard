//go:build integration

// parity_test.go carries the differential-parity harness lifted from internal/gitnativepoc/harness_test.go into package gitrepo_test: fixture builders beyond newRepo/writeFile/commitAll (already defined in gitrepo_test.go and reused directly here), repo-shaping helpers the parity cases need, and comparison helpers that report an oracle-vs-implementation divergence with both values so a failing case is diagnosable without re-running it under a debugger.
// The parity cases built on this scaffolding follow the helpers; the linked-worktree parity cases live in gogit_test.go.

package gitrepo_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/gitrepo/internal/gitoracle"
)

// newEmptyRepoFixture builds a repo with an unborn HEAD via `git init -b main`.
func newEmptyRepoFixture(t *testing.T) (dir string) {
	t.Helper()

	dir = t.TempDir()
	gitkit.MustRun(t, dir, "git", "init", "-b", "main")
	return dir
}

// assertParitySHA fails the test unless oracle and impl — the SHAs returned
// by the CLI oracle and gitrepo's method for the same operation and fixture —
// are identical.
func assertParitySHA(t *testing.T, oracle, impl string) {
	t.Helper()

	if oracle != impl {
		t.Errorf("SHA parity mismatch: oracle (CLI) = %q; gitrepo = %q", oracle, impl)
	}
}

// assertParityBool fails the test unless oracle and impl — the boolean
// results returned by the CLI oracle and gitrepo's method for the same
// operation and fixture — agree.
func assertParityBool(t *testing.T, oracle, impl bool) {
	t.Helper()

	if oracle != impl {
		t.Errorf("bool parity mismatch: oracle (CLI) = %v; gitrepo = %v", oracle, impl)
	}
}

// assertParityFileList fails the test unless oracle and impl contain the same
// set of file paths, ignoring order — both sides are sorted before
// comparison, since neither ChangedFilesSince's godoc nor any consumer
// contracts on result order, only on the set of changed paths.
func assertParityFileList(t *testing.T, oracle, impl []string) {
	t.Helper()

	oracleSorted := append([]string(nil), oracle...)
	implSorted := append([]string(nil), impl...)
	sort.Strings(oracleSorted)
	sort.Strings(implSorted)

	if len(oracleSorted) != len(implSorted) {
		t.Errorf("file list parity mismatch: oracle (CLI) = %v; gitrepo = %v", oracleSorted, implSorted)
		return
	}
	for i := range oracleSorted {
		if oracleSorted[i] != implSorted[i] {
			t.Errorf("file list parity mismatch: oracle (CLI) = %v; gitrepo = %v", oracleSorted, implSorted)
			return
		}
	}
}

// assertParityErrClass fails the test unless oracleErr and implErr represent
// the same error class: both nil, or both non-nil with each side's error
// satisfying errors.Is against its OWN package's sentinel. The oracle and
// gitrepo define independent sentinel values for the same condition (the
// oracle must not import gitrepo's sentinels — doing so would reintroduce
// exactly the coupling the oracle exists to avoid), so a single shared target
// cannot bridge them; this is the cross-target comparison
// gitnativepoc/read_test.go's assertParityErrClassCrossTarget already
// established for the same reason.
func assertParityErrClass(t *testing.T, oracleErr, oracleTarget, implErr, implTarget error) {
	t.Helper()

	oracleIs := errors.Is(oracleErr, oracleTarget)
	implIs := errors.Is(implErr, implTarget)
	if oracleIs != implIs {
		t.Errorf("error class parity mismatch: oracle errors.Is(%v, %v) = %v; gitrepo errors.Is(%v, %v) = %v",
			oracleErr, oracleTarget, oracleIs, implErr, implTarget, implIs)
	}
}

// assertParityErrPresence fails the test unless oracleErr and implErr are
// either both nil or both non-nil — used where neither side defines a typed
// sentinel to compare against (e.g. CurrentBranch's detached-HEAD failure),
// so only error-vs-no-error agreement is meaningful.
func assertParityErrPresence(t *testing.T, oracleErr, implErr error) {
	t.Helper()

	if (oracleErr == nil) != (implErr == nil) {
		t.Errorf("error presence parity mismatch: oracle err = %v; gitrepo err = %v", oracleErr, implErr)
	}
}

// assertParityString fails the test unless oracle and impl are identical
// strings — the general string-equality comparison CurrentBranch's parity
// cases use. assertParitySHA exists separately for the SHA-shaped case so its
// failure message names the value it is actually comparing.
func assertParityString(t *testing.T, oracle, impl string) {
	t.Helper()

	if oracle != impl {
		t.Errorf("string parity mismatch: oracle (CLI) = %q; gitrepo = %q", oracle, impl)
	}
}

// resolveRevOrFatal resolves rev to its object SHA via `git rev-parse`,
// failing the test outright on any failure — used by fixtures that need a
// tree or blob SHA (HEAD^{tree}, HEAD:<path>) rather than a commit SHA, which
// no fixture builder in this package hands back directly.
func resolveRevOrFatal(t *testing.T, dir, rev string) string {
	t.Helper()

	stdout, stderr, code, err := runGit(t, dir, "rev-parse", rev)
	if err != nil {
		t.Fatalf("git rev-parse %s error = %v", rev, err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse %s exited %d: %s", rev, code, stderr)
	}
	return strings.TrimSpace(stdout)
}

// TestParity drives the oracle and gitrepo through one committed repository, asserting they agree on CurrentSHA, SHAExists, ChangedFilesSince and CurrentBranch.
// The steps run serially in one order and share the repository's state: the file-list steps each take their own base commit and add commits on main, and the detached-HEAD step runs before the orphan step because both leave HEAD off main.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestParity(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		// Trivially true while gitrepo.CurrentSHA is CLI-backed;
		// the value of this case is established before the method flips onto go-git.
		{"CurrentSHA agrees on a committed repo", func(t *testing.T) {
			oracleSHA, oracleErr := gitoracle.CurrentSHA(t, dir)
			if oracleErr != nil {
				t.Fatalf("gitoracle.CurrentSHA() error = %v", oracleErr)
			}
			implSHA, implErr := repo.CurrentSHA()
			if implErr != nil {
				t.Fatalf("CurrentSHA() error = %v", implErr)
			}

			assertParitySHA(t, oracleSHA, implSHA)
		}},
		// A well-formed-but-absent SHA and a non-hex string both fold into false without either side treating the lookup itself as a failure worth surfacing.
		// A tree or blob SHA — a real, valid-hex object name, just not a commit — is false too, never true: the `^{commit}` peel is what makes this so, and it is exactly what the missing and non-hex rows cannot distinguish, since neither of those SHAs resolves to any object at all.
		{"SHAExists agrees on committed, missing, non-hex, tree and blob shas", func(t *testing.T) {
			tests := []struct {
				name string
				sha  string
			}{
				{"CommittedSHA", requireCurrentSHA(t, repo)},
				{"MissingSHA", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
				{"NonHexSHA", "not-a-sha!!"},
				{"TreeSHA", resolveRevOrFatal(t, dir, "HEAD^{tree}")},
				{"BlobSHA", resolveRevOrFatal(t, dir, "HEAD:a.txt")},
			}
			for _, tt := range tests {
				assertParityBool(t, gitoracle.SHAExists(t, dir, tt.sha), repo.SHAExists(tt.sha))
			}
			if !repo.SHAExists(tests[0].sha) {
				t.Error("SHAExists(committed sha) = false; want true")
			}
			for _, tt := range tests[1:] {
				if repo.SHAExists(tt.sha) {
					t.Errorf("SHAExists(%s %q) = true; want false", tt.name, tt.sha)
				}
			}
		}},
		{"CurrentBranch agrees on an ordinary branch", func(t *testing.T) {
			oracleBranch, oracleErr := gitoracle.CurrentBranch(t, dir)
			if oracleErr != nil {
				t.Fatalf("gitoracle.CurrentBranch() error = %v", oracleErr)
			}
			implBranch, implErr := repo.CurrentBranch()
			if implErr != nil {
				t.Fatalf("CurrentBranch() error = %v", implErr)
			}
			assertParityString(t, oracleBranch, implBranch)
			if implBranch != "main" {
				t.Errorf("CurrentBranch() = %q, want %q", implBranch, "main")
			}
		}},
		// The linked worktree's git dir sits under the primary's common dir, so the two reads differ there and agree on the primary.
		{"GitDir, CommonDir and Toplevel agree on the primary and on a linked worktree", func(t *testing.T) {
			linkedDir := filepath.Join(t.TempDir(), "linked")
			gitkit.MustRun(t, dir, "git", "worktree", "add", "-b", "linked-geometry", linkedDir)

			for _, checkout := range []struct {
				name string
				dir  string
			}{{"primary", dir}, {"linked", linkedDir}} {
				handle := gitrepo.New(checkout.dir)
				reads := []struct {
					name   string
					oracle func(testing.TB, string) (string, error)
					impl   func() (string, error)
				}{
					{"GitDir", gitoracle.GitDir, handle.GitDir},
					{"CommonDir", gitoracle.CommonDir, handle.CommonDir},
					{"Toplevel", gitoracle.Toplevel, handle.Toplevel},
				}
				for _, read := range reads {
					oracleValue, oracleErr := read.oracle(t, checkout.dir)
					if oracleErr != nil {
						t.Fatalf("gitoracle.%s() on the %s checkout error = %v", read.name, checkout.name, oracleErr)
					}
					implValue, implErr := read.impl()
					if implErr != nil {
						t.Fatalf("%s() on the %s checkout error = %v", read.name, checkout.name, implErr)
					}
					assertParityString(t, oracleValue, implValue)
				}
			}

			linked := gitrepo.New(linkedDir)
			linkedGitDir, _ := linked.GitDir()
			linkedCommonDir, _ := linked.CommonDir()
			if linkedGitDir == linkedCommonDir {
				t.Errorf("linked worktree GitDir() = CommonDir() = %q; want them to differ", linkedGitDir)
			}
		}},
		// Each side returns its own ErrInvalidSHA-class sentinel before either ever resolves or diffs anything.
		{"ChangedFilesSince rejects a non-hex sha", func(t *testing.T) {
			if _, err := repo.ChangedFilesSince("not-a-sha!!"); !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Errorf("ChangedFilesSince(non-hex) error = %v, want gitrepo.ErrInvalidSHA", err)
			}
		}},
		// Both return a non-ASCII filename verbatim — the on-disk literal, never core.quotePath's C-quoted escape form — and agree with each other.
		{"ChangedFilesSince returns a non-ASCII path verbatim on both sides", func(t *testing.T) {
			since := requireCurrentSHA(t, repo)
			const filename = "å.txt"
			writeFile(t, dir, filename, "berries")
			commitAll(t, dir, "add non-ascii filename")

			oracleFiles, oracleErr := gitoracle.ChangedFilesSince(t, dir, since)
			if oracleErr != nil {
				t.Fatalf("gitoracle.ChangedFilesSince() error = %v", oracleErr)
			}
			if !slices.Contains(oracleFiles, filename) {
				t.Fatalf("gitoracle.ChangedFilesSince() = %v, want it to contain verbatim %q", oracleFiles, filename)
			}

			implFiles, implErr := repo.ChangedFilesSince(since)
			if implErr != nil {
				t.Fatalf("ChangedFilesSince() error = %v", implErr)
			}
			if !slices.Contains(implFiles, filename) {
				t.Fatalf("ChangedFilesSince() = %v, want it to contain verbatim %q", implFiles, filename)
			}

			assertParityFileList(t, oracleFiles, implFiles)
		}},
		// Both report a pure rename (identical content, the case git's default rename detection folds into one entry) as its old path (deleted) and new path (added) separately, never folded into one entry, exercising the --no-renames convention both the oracle and the implementation must apply; and agree with each other.
		{"ChangedFilesSince reports both sides of a rename on both sides", func(t *testing.T) {
			const oldName, newName = "old.txt", "new.txt"
			writeFile(t, dir, oldName, "content that stays identical")
			commitAll(t, dir, "add old.txt")
			since := requireCurrentSHA(t, repo)
			gitkit.MustRun(t, dir, "git", "mv", oldName, newName)
			gitkit.MustRun(t, dir, "git", "commit", "-m", "rename")

			oracleFiles, oracleErr := gitoracle.ChangedFilesSince(t, dir, since)
			if oracleErr != nil {
				t.Fatalf("gitoracle.ChangedFilesSince() error = %v", oracleErr)
			}
			implFiles, implErr := repo.ChangedFilesSince(since)
			if implErr != nil {
				t.Fatalf("ChangedFilesSince() error = %v", implErr)
			}

			for _, files := range [][]string{oracleFiles, implFiles} {
				if !slices.Contains(files, oldName) {
					t.Errorf("ChangedFilesSince() = %v, want it to contain the deleted old path %q", files, oldName)
				}
				if !slices.Contains(files, newName) {
					t.Errorf("ChangedFilesSince() = %v, want it to contain the added new path %q", files, newName)
				}
			}
			assertParityFileList(t, oracleFiles, implFiles)
		}},
		// A detached HEAD must be an error, never an empty string — a caller never mistakes "no branch captured" for a legitimate branch name.
		{"CurrentBranch agrees on a detached HEAD", func(t *testing.T) {
			gitkit.MustRun(t, dir, "git", "checkout", "--detach", requireCurrentSHA(t, repo))

			_, oracleErr := gitoracle.CurrentBranch(t, dir)
			_, implErr := repo.CurrentBranch()
			assertParityErrPresence(t, oracleErr, implErr)
			if implErr == nil {
				t.Error("CurrentBranch() on detached HEAD error = nil, want non-nil")
			}
		}},
		// Relies on the detached HEAD the previous step leaves: an orphan branch is started from it.
		{"CurrentBranch agrees on an orphan branch", func(t *testing.T) {
			gitkit.MustRun(t, dir, "git", "checkout", "--orphan", "orphan-branch")
			gitkit.MustRun(t, dir, "git", "rm", "-rf", "--cached", ".")
			gitkit.CommitFile(t, dir, "orphan.txt", "unrelated root", "orphan root")

			oracleBranch, oracleErr := gitoracle.CurrentBranch(t, dir)
			if oracleErr != nil {
				t.Fatalf("gitoracle.CurrentBranch() on orphan branch error = %v, want nil", oracleErr)
			}
			implBranch, implErr := repo.CurrentBranch()
			if implErr != nil {
				t.Fatalf("CurrentBranch() on orphan branch error = %v, want nil", implErr)
			}
			assertParityString(t, oracleBranch, implBranch)
			if implBranch != "orphan-branch" {
				t.Errorf("CurrentBranch() on orphan branch = %q, want %q", implBranch, "orphan-branch")
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}

// TestParity_UnbornHEAD asserts the oracle and gitrepo agree on an unborn HEAD.
// CurrentSHA maps git's ambiguous-HEAD stderr shape to each side's own sentinel (gitoracle.ErrNoCommits and gitrepo.ErrNoCommits), so the cross-target class comparison — never a raw string comparison, since the two sides never produce byte-identical errors — is what proves agreement.
// CurrentBranch succeeds on an unborn HEAD and prints the branch name even with no commit yet.
func TestParity_UnbornHEAD(t *testing.T) {
	t.Parallel()

	dir := newEmptyRepoFixture(t)
	repo := gitrepo.New(dir)

	_, oracleErr := gitoracle.CurrentSHA(t, dir)
	_, implErr := repo.CurrentSHA()

	assertParityErrClass(t, oracleErr, gitoracle.ErrNoCommits, implErr, gitrepo.ErrNoCommits)
	if !errors.Is(oracleErr, gitoracle.ErrNoCommits) {
		t.Errorf("gitoracle.CurrentSHA() on unborn HEAD error = %v, want gitoracle.ErrNoCommits", oracleErr)
	}
	if !errors.Is(implErr, gitrepo.ErrNoCommits) {
		t.Errorf("CurrentSHA() on unborn HEAD error = %v, want gitrepo.ErrNoCommits", implErr)
	}

	oracleBranch, oracleErr := gitoracle.CurrentBranch(t, dir)
	if oracleErr != nil {
		t.Fatalf("gitoracle.CurrentBranch() on unborn HEAD error = %v, want nil", oracleErr)
	}
	implBranch, implErr := repo.CurrentBranch()
	if implErr != nil {
		t.Fatalf("CurrentBranch() on unborn HEAD error = %v, want nil", implErr)
	}
	assertParityString(t, oracleBranch, implBranch)
	if implBranch != "main" {
		t.Errorf("CurrentBranch() on unborn HEAD = %q, want %q", implBranch, "main")
	}
}

// forcePackIndexFreeze forces repo's go-git handle to build (and freeze) its internal packfile index against whatever packs exist on disk at the moment of the call, by driving one object-lookup miss through the exported SHAExists.
// It is the shared setup step every "hard variant" mixed-backend case below builds on:
// a subsequent commit-then-repack sequence produces a packfile-only object the frozen index predates, so a later read can only resolve it correctly by going through the whole-read reindex retry of readGoGit.
func forcePackIndexFreeze(t *testing.T, repo *gitrepo.Repo) {
	t.Helper()

	if repo.SHAExists("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef") {
		t.Fatal("SHAExists(fabricated sha) = true; want false (sanity check for the freeze helper)")
	}
}

// TestMixedBackend_PreWarmedHandleSeesCLICommit asserts the trailing r.CurrentSHA() call of StageAndCommit and of its wildcard sibling StageAllAndCommit — a go-git ref read — sees the commit its own preceding `git commit` call just wrote, even when the Repo's go-git handle was warmed (opened and cached) before that commit landed.
// This is the call-granular boundary's central mixed-backend site: a stale answer here would hand a wrong SHA to any caller that records the return value as the checkout's new baseline.
// The rows run serially against one repository and one warmed handle.
func TestMixedBackend_PreWarmedHandleSeesCLICommit(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	if _, err := repo.CurrentSHA(); err != nil {
		t.Fatalf("CurrentSHA() (warm handle) error = %v", err)
	}

	tests := []struct {
		name   string
		commit func() (string, bool, error)
	}{
		{"StageAndCommit", func() (string, bool, error) { return repo.StageAndCommit("commit a", []string{"a.txt"}) }},
		{"StageAllAndCommit", func() (string, bool, error) { return repo.StageAllAndCommit("commit all") }},
	}
	for i, tt := range tests {
		if !t.Run(tt.name, func(t *testing.T) {
			writeFile(t, dir, "a.txt", fmt.Sprintf("changed %d", i))
			sha, committed, err := tt.commit()
			if err != nil {
				t.Fatalf("%s() error = %v; want nil", tt.name, err)
			}
			if !committed {
				t.Fatalf("%s() committed = false; want true", tt.name)
			}

			want := resolveRevOrFatal(t, dir, "HEAD")
			if sha != want {
				t.Errorf("%s() sha = %q; want %q (the commit its own CLI write just created)", tt.name, sha, want)
			}
		}) {
			return
		}
	}
}

// TestMixedBackend_RepackBetweenCommitAndRead is the #468 regression, the hard variant of the mixed-backend interop cases:
// each row's go-git handle is frozen (via forcePackIndexFreeze) against a pack-less on-disk state, a second commit then lands, and `git repack -d` with `git prune-packed` leaves it packfile-only BEFORE the row's one read — the shape Push's own pull --rebase retry can produce in production.
// Every row builds its own repository and Repo: the first row's retry reindexes the handle it uses, so a shared fixture would let every later row pass even if its method bypassed the whole-read retry.
// SHAExists' failure-swallowing posture means its row would fail silently (report false forever), never loudly.
func TestMixedBackend_RepackBetweenCommitAndRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// check makes the row's one read against a frozen, now-stale handle, after dirtying the worktree if the read is about it.
		check func(t *testing.T, dir string, repo *gitrepo.Repo, initSHA, secondSHA string)
	}{
		{"SHAExists", func(t *testing.T, dir string, repo *gitrepo.Repo, initSHA, secondSHA string) {
			if !repo.SHAExists(secondSHA) {
				t.Errorf("SHAExists(%q) after repack = false; want true", secondSHA)
			}
		}},
		{"UntrackedFiles", func(t *testing.T, dir string, repo *gitrepo.Repo, initSHA, secondSHA string) {
			writeFile(t, dir, "untracked.txt", "new")
			got, err := repo.UntrackedFiles()
			if err != nil {
				t.Fatalf("UntrackedFiles() error = %v; want nil", err)
			}
			requireSameFiles(t, "UntrackedFiles()", got, "untracked.txt")
		}},
		{"WorktreeChangedFiles", func(t *testing.T, dir string, repo *gitrepo.Repo, initSHA, secondSHA string) {
			writeFile(t, dir, "a.txt", "changed")
			got, err := repo.WorktreeChangedFiles()
			if err != nil {
				t.Fatalf("WorktreeChangedFiles() error = %v; want nil", err)
			}
			requireSameFiles(t, "WorktreeChangedFiles()", got, "a.txt")
		}},
		{"ChangedFilesSince", func(t *testing.T, dir string, repo *gitrepo.Repo, initSHA, secondSHA string) {
			got, err := repo.ChangedFilesSince(initSHA)
			if err != nil {
				t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
			}
			requireSameFiles(t, "ChangedFilesSince()", got, "b.txt")
		}},
		{"HeadContains", func(t *testing.T, dir string, repo *gitrepo.Repo, initSHA, secondSHA string) {
			got, err := repo.HeadContains(secondSHA)
			if err != nil || !got {
				t.Errorf("HeadContains(%q) = (%v, %v); want (true, nil)", secondSHA, got, err)
			}
		}},
		{"PathRevisions", func(t *testing.T, dir string, repo *gitrepo.Repo, initSHA, secondSHA string) {
			got, err := repo.PathRevisions("b.txt", 0)
			if err != nil || !slices.Equal(got, []string{secondSHA}) {
				t.Errorf("PathRevisions(b.txt) = (%v, %v); want ([%s], nil)", got, err, secondSHA)
			}
		}},
		{"CommitParents", func(t *testing.T, dir string, repo *gitrepo.Repo, initSHA, secondSHA string) {
			got, err := repo.CommitParents(secondSHA)
			if err != nil || !slices.Equal(got, []string{initSHA}) {
				t.Errorf("CommitParents(%q) = (%v, %v); want ([%s], nil)", secondSHA, got, err, initSHA)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir, repo := newRepo(t)
			writeFile(t, dir, "a.txt", "initial")
			commitAll(t, dir, "init")
			initSHA := resolveRevOrFatal(t, dir, "HEAD")

			forcePackIndexFreeze(t, repo)

			writeFile(t, dir, "b.txt", "second")
			commitAll(t, dir, "second commit")
			secondSHA := resolveRevOrFatal(t, dir, "HEAD")

			gitkit.MustRun(t, dir, "git", "repack", "-d")
			gitkit.MustRun(t, dir, "git", "prune-packed")

			tc.check(t, dir, repo, initSHA, secondSHA)
		})
	}
}

// TestSHAExists_MixedBackend_CrossInstanceReindexSeesWriteFromOtherRepo pins the fingerprint gate
// against a per-instance counter gate: repoA's go-git index is frozen against the pack-less on-disk
// state, then a SEPARATE gitrepo.New value on the identical path builds and repacks a new commit —
// a distinct *Repo, so repoA's own call count is never touched by any of it — and repoA's own
// SHAExists must still resolve the new, now-packed commit.
// A per-*Repo call counter would never observe this write at all, since it only ever counts repoA's
// own calls;
// the pack fingerprint is shared, on-disk ground truth every *Repo addressing the same checkout can
// observe regardless of which one performed the write.
// This is the case that fails under a counter gate and passes under the fingerprint gate, so it
// pins the design rather than merely restating it.
func TestSHAExists_MixedBackend_CrossInstanceReindexSeesWriteFromOtherRepo(t *testing.T) {
	t.Parallel()

	dir, repoA := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	forcePackIndexFreeze(t, repoA)

	// A genuinely separate *Repo value on the same path — repoA's own handle
	// is never touched by anything below.
	repoB := gitrepo.New(dir)
	writeFile(t, dir, "b.txt", "from repoB")
	commitAll(t, dir, "commit via repoB's path")
	sha, err := repoB.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (repoB) error = %v", err)
	}
	gitkit.MustRun(t, dir, "git", "gc")

	if !repoA.SHAExists(sha) {
		t.Errorf("repoA.SHAExists(%q) = false; want true (repoA's fingerprint-gated reindex must see a write+repack made through a separate *Repo)", sha)
	}
}
