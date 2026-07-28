//go:build integration

// fixtures_test.go builds the linked-worktree and junction test fixtures the
// package gitrepo_test parity/oracle coverage of later batches (batches 2-4)
// needs: a shared-common-dir topology no standalone `git init` repo can
// produce, and the one this checkout, and every fabricengine worktree,
// actually runs in — see .scratch/gogit-worktree-probe-report.md. This file
// only adds fixture builders; the existing fixtures in gitrepo_test.go
// (newRepo, writeFile, commitAll, runGit, firstCommitSHA, ...) are untouched
// and reused directly, since both files share package gitrepo_test.
//
// Note this file's builders are unreachable from gogit_test.go's coverage in
// this same batch: gogit_test.go is package gitrepo (the internal package,
// needed to reach Repo's unexported goGit/lookupObjectRetrying), a
// structurally different Go package from this file's package gitrepo_test,
// even though both live in this directory and compile into one test binary.
// gogit_test.go therefore builds its own equivalent, smaller fixture rather
// than importing these.

package gitrepo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// linkedWorktreeFixture is a git checkout with a linked worktree built via
// `git worktree add`, deliberately arranged so the main and linked
// worktrees disagree on every axis a coincidence could otherwise hide
// behind: different branch ("main" vs "feature") and different HEAD commit.
// refs/loomyard/snapshot/*-style refs and every other shared-namespace ref
// live in the common dir the two worktrees share, while HEAD is
// per-worktree — exactly the split that a wrong open (git.PlainOpen without
// EnableDotGitCommonDir) surfaces as a wrong value rather than a clean
// error, per the probe report.
//
// Not yet used within this batch — its first caller lands with batch 2's
// parity/oracle test files; golangci-lint's default (untagged) build sees no
// caller either way, matching gitnativepoc/read.go's identical hasUnpushed
// precedent.
//
//nolint:unused // first exercised by batch 2's parity/oracle test files; only reachable under the //go:build integration tag either way
type linkedWorktreeFixture struct {
	// mainDir/mainRepo is the primary worktree, left on branch "main".
	mainDir  string
	mainRepo *gitrepo.Repo
	// mainSHA is main's HEAD commit once the fixture is fully built.
	mainSHA string

	// linkedDir/linkedRepo is the linked worktree under test, on branch
	// "feature" with its own HEAD commit — deliberately different from
	// mainSHA. For a fixture built by newLinkedWorktreeFixtureViaJunction,
	// linkedDir is the junction path, not the worktree's real path.
	linkedDir  string
	linkedRepo *gitrepo.Repo
	// linkedSHA is linked's HEAD commit once the fixture is fully built.
	linkedSHA string

	// sharedCommitSHA is a commit that exists before the two worktrees'
	// branches diverge, so it is reachable from either one — used to
	// exercise an object lookup that must succeed regardless of which
	// worktree's handle performs it.
	sharedCommitSHA string
}

// newLinkedWorktreeFixture builds a main worktree on branch "main", then adds
// a linked worktree via `git worktree add -b feature` and advances it with
// its own separate commit — so the two worktrees end up on different
// branches at different HEAD commits, never coincidentally equal. Both
// worktrees are reachable directly at their own filesystem path.
//
// Not yet used within this batch — see linkedWorktreeFixture's doc.
//
//nolint:unused // first exercised by batch 2's parity/oracle test files; only reachable under the //go:build integration tag either way
func newLinkedWorktreeFixture(t *testing.T) *linkedWorktreeFixture {
	t.Helper()

	container := t.TempDir()
	mainDir := filepath.Join(container, "main")
	if err := os.Mkdir(mainDir, 0o755); err != nil {
		t.Fatalf("mkdir main worktree dir: %v", err)
	}
	lyxtest.MustRun(t, mainDir, "git", "init", "-b", "main")
	mainRepo := gitrepo.New(mainDir)

	writeFile(t, mainDir, "shared.txt", "shared base content\n")
	commitAll(t, mainDir, "base commit")
	sharedCommitSHA, err := mainRepo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (base) error = %v", err)
	}

	writeFile(t, mainDir, "main-only.txt", "main advances\n")
	commitAll(t, mainDir, "main commit 2")
	mainSHA, err := mainRepo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (main) error = %v", err)
	}

	linkedDir := filepath.Join(container, "linked")
	lyxtest.MustRun(t, mainDir, "git", "worktree", "add", "-b", "feature", linkedDir)
	linkedRepo := gitrepo.New(linkedDir)

	writeFile(t, linkedDir, "linked-only.txt", "linked advances differently\n")
	commitAll(t, linkedDir, "feature commit")
	linkedSHA, err := linkedRepo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() (linked) error = %v", err)
	}

	if linkedSHA == mainSHA {
		t.Fatalf("fixture setup produced equal HEAD SHAs for main and linked (%s); want them to differ", mainSHA)
	}

	return &linkedWorktreeFixture{
		mainDir:         mainDir,
		mainRepo:        mainRepo,
		mainSHA:         mainSHA,
		linkedDir:       linkedDir,
		linkedRepo:      linkedRepo,
		linkedSHA:       linkedSHA,
		sharedCommitSHA: sharedCommitSHA,
	}
}

// newLinkedWorktreeFixtureViaJunction builds a linkedWorktreeFixture exactly
// as newLinkedWorktreeFixture does, then creates a directory link
// (internal/fslink.CreateDirLink — a junction on Windows, a symlink
// elsewhere) pointing at the linked worktree, and returns a fixture whose
// linkedDir/linkedRepo address the worktree only through that link, never
// through its direct path — the same indirection internal/fslink uses to
// reach fabricengine's worktrees in production. It skips the calling test
// cleanly (t.Skipf) when link creation is unavailable on this platform,
// rather than failing: link creation itself is not under test here.
//
// Not yet used within this batch — see linkedWorktreeFixture's doc.
//
//nolint:unused // first exercised by batch 2's parity/oracle test files; only reachable under the //go:build integration tag either way
func newLinkedWorktreeFixtureViaJunction(t *testing.T) *linkedWorktreeFixture {
	t.Helper()

	base := newLinkedWorktreeFixture(t)

	junctionPath := filepath.Join(t.TempDir(), "via-junction")
	if err := fslink.CreateDirLink(junctionPath, base.linkedDir); err != nil {
		t.Skipf("directory link creation unavailable on this platform: %v", err)
	}

	return &linkedWorktreeFixture{
		mainDir:         base.mainDir,
		mainRepo:        base.mainRepo,
		mainSHA:         base.mainSHA,
		linkedDir:       junctionPath,
		linkedRepo:      gitrepo.New(junctionPath),
		linkedSHA:       base.linkedSHA,
		sharedCommitSHA: base.sharedCommitSHA,
	}
}
