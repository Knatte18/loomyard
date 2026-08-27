// export_test.go re-exports newPaired, the now-private warp/weft fields, and the destructive gate's
// unexported executors and ownership predicates, for the handful of package fabricengine_test files
// that need to drive them directly rather than through an application-level verb whose own
// higher-level checks would short-circuit before the gate is ever reached — the standard Go
// export_test.go idiom, per the export-test-shim decision.

package fabricengine

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// NewPairedFromPathsForTest re-exports newPaired for fabric_test.go's untagged unit test of the
// newPaired constructor itself, its one remaining consumer: it hands newPaired two empty directories
// and asserts the warp and weft fields come back non-nil.
// It is a constructor seam, not a fixture-pairing shim — it must never be used to assemble a test
// fixture's warp/weft pair.
// A test needing a real pair takes one from internal/hubforge instead.
var NewPairedFromPathsForTest = newPaired

// WarpForTest returns f's private warp field, for package fabricengine_test files that need
// warp-side gitrepo.Repo access no other exported accessor provides.
func WarpForTest(f *Fabric) *gitrepo.Repo {
	return f.warp
}

// WeftForTest returns f's private weft field, for package fabricengine_test files that need
// weft-side gitrepo.Repo access no other exported accessor provides.
func WeftForTest(f *Fabric) *gitrepo.Repo {
	return f.weft
}

// WarpProbeDirPrefixForTest re-exports warpProbeDirPrefix for package fabricengine_test files that
// assert on the probe's throwaway-clone directory naming without duplicating the literal.
const WarpProbeDirPrefixForTest = warpProbeDirPrefix

// ExcludePatternForTest re-exports excludePatternFor for package fabricengine_test files that
// assert on the anchored .git/info/exclude pattern without duplicating its spelling.
var ExcludePatternForTest = excludePatternFor

// MatchParentBranchForTest re-exports matchParentBranch for package fabricengine_test files that
// need to drive it directly with hand-built WorktreeEntry slices, without a hubforge fixture.
var MatchParentBranchForTest = matchParentBranch

// RemoveWarpWorktreeDirForTest re-exports removeWarpWorktreeDir for package fabricengine_test
// integration tests that need to drive the gate's registered-linked-worktree ownership kind
// directly against an arbitrary target, rather than through Topology.Remove — whose own top-level
// dirty check (remove.go) refuses an untracked warp worktree before removeWarpWorktreeDir's own
// git-refusal-triggered fallback is ever reached.
var RemoveWarpWorktreeDirForTest = removeWarpWorktreeDir

// TeardownHubForTest re-exports teardownHub for package fabricengine_test integration tests that
// need to drive it directly rather than through a full CloneHub run, whose hub path is always
// derived as a child of its own cwd argument and so can never itself be outside the operator-named
// parent.
// It mints the createdToken teardownHub requires by creating hubPath itself via createExclusiveDir
// (hubPath must not already exist), running seed against the freshly-created directory before
// teardownHub runs, if seed is non-nil.
// It passes a throwaway NewMutations("") recorder, since this seam has no verb-level recorder of its
// own and its callers assert nothing about the record.
func TeardownHubForTest(cwd, hubPath string, seed func(hubPath string) error, cause error) error {
	rec := NewMutations("")
	tok, err := createExclusiveDir(rec, hubPath)
	if err != nil {
		return err
	}
	if seed != nil {
		if err := seed(hubPath); err != nil {
			return err
		}
	}
	return teardownHub(rec, cwd, hubPath, tok, cause)
}

// LooksLikeHubForTest re-exports looksLikeHub for package fabricengine_test integration tests
// covering the gate's fabric-hub ownership kind.
var LooksLikeHubForTest = looksLikeHub

// IsWarpCheckoutForTest re-exports isAnyWorktreeOf (renamed from isWarpCheckout when
// ownedWeftCheckout started sharing its repo-agnostic predicate) for package fabricengine_test
// integration tests covering the gate's warp-checkout ownership kind, which ResetHard's own exported
// entry point cannot exercise with an arbitrary (repoDir, target) pair since it always calls it with
// both equal to the same f.warpPath. The exported seam name is unchanged, so
// destructivegaps_integration_test.go's TestOwnership_WarpCheckoutKind needs no edit.
var IsWarpCheckoutForTest = isAnyWorktreeOf

// IsRegisteredLinkedWorktreeInForTest re-exports isRegisteredLinkedWorktreeIn for package
// fabricengine_test integration tests covering the gate's registered-linked-worktree ownership
// kind against an arbitrary (repoDir, target) pair.
var IsRegisteredLinkedWorktreeInForTest = isRegisteredLinkedWorktreeIn

// WorktreeDirtyTrackedForTest re-exports worktreeDirty(scopeTracked, dir) for package
// fabricengine_test integration tests, since dirtyScope itself is unexported.
func WorktreeDirtyTrackedForTest(dir string) (dirty bool, detail string, err error) {
	return worktreeDirty(scopeTracked, dir)
}

// WorktreeDirtyAllForTest re-exports worktreeDirty(scopeAll, dir) for package fabricengine_test
// integration tests, since dirtyScope itself is unexported.
func WorktreeDirtyAllForTest(dir string) (dirty bool, detail string, err error) {
	return worktreeDirty(scopeAll, dir)
}

// DeleteBranchForTest re-exports the gate's deleteBranch executor, built from
// ownedManagedBranch(l, branchPrefix) and dirtyCheckedOutBranch(), for package fabricengine_test
// integration tests that need to drive the branch-ownership gate directly rather than through
// Cleanup's application-level orphan/liveness filtering — which never reaches the gate at all for
// the primary weft branch or a branch checked out at some worktree.
// It passes a throwaway NewMutations("") recorder, since this seam has no verb-level recorder of its
// own and its callers assert nothing about the record.
func DeleteBranchForTest(l *lyxcwd.Location, repoDir, branch, branchPrefix string, force bool) error {
	req := branchRequest{
		what:      "test delete branch",
		repoDir:   repoDir,
		branch:    branch,
		ownership: ownedManagedBranch(l, branchPrefix),
		dirtiness: dirtyCheckedOutBranch(),
		force:     force,
	}
	return deleteBranch(NewMutations(""), req)
}

// --- weft-fixture migration shim (fabricengine in-package weft batch) ---
//
// The functions and types below serve the nine package fabricengine_test files this batch relocates
// out of package fabricengine. Two different reasons put an entry here, and both are noted per entry:
//
//   - "production plumbing": the underlying identifier is defined in a non-test, non-relocating file
//     (or in a _test.go file that itself never relocates) and simply needs an exported seam.
//   - "relocated fixture helper": the underlying function used to live in one of the nine relocating
//     files, but a file OUTSIDE the nine — either a permanently package-fabricengine sibling test file,
//     or one of the nine itself migrating on an EARLIER card than the helper's original host file —
//     still needs it as a plain, unexported, same-package symbol at some point in the batch's history.
//     Moving the body here (and deleting the original) keeps that symbol resolvable under package
//     fabricengine for as long as anything package-fabricengine still calls it unqualified, while the
//     ForTest wrapper below it gives every migrated file a qualified path to the same logic.

// NewPlainWarpRepoForTest re-exports newPlainWarpRepo (relocated fixture helper, formerly
// index_integration_test.go): commitweftat_test.go (package fabricengine, never migrating) calls it
// unqualified, and several of the nine relocating files need it before
// index_integration_test.go's own migration card lands.
var NewPlainWarpRepoForTest = newPlainWarpRepo

// newPlainWarpRepo creates a minimal, isolated git repo at t.TempDir() on branch main with one
// commit — everything RecordCorrespondence/warpSeq needs from a warp repo, without any of fabric's
// own topology wiring (junctions, weft pairing), which the callers of this helper do not exercise.
func newPlainWarpRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitkit.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	gitkit.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("warp"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, dir, "git", "add", ".")
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

// NewFabricForTest re-exports newFabric (relocated fixture helper, formerly
// index_integration_test.go): several of the nine relocating files need it before
// index_integration_test.go's own migration card lands. Distinct from NewPairedFromPathsForTest
// above: newFabric fails the test on error rather than returning it, which every one of this
// helper's callers relies on.
var NewFabricForTest = newFabric

// newFabric wraps newPaired, failing the test on error rather than returning it — every caller of
// this helper expects a valid pair.
func newFabric(t *testing.T, warpPath, weftPath string) *Fabric {
	t.Helper()

	f, err := newPaired(warpPath, weftPath)
	if err != nil {
		t.Fatalf("newPaired(%q, %q) error = %v", warpPath, weftPath, err)
	}
	return f
}

// SeedFabricConfigForTest re-exports seedFabricConfig (relocated fixture helper, formerly
// commit_integration_test.go): diff_integration_test.go and syncweft_integration_test.go need it
// before their own migration cards land, and newCommitFixture below needs it permanently since it
// stays package fabricengine.
var SeedFabricConfigForTest = seedFabricConfig

// seedFabricConfig materializes the repo-wide fabric.yaml Fabric.Commit's classify step now reads via
// RepoWiredNames — the `weft:main` base at BoardDir(Hub) — so its resolved pathspec is what
// Fabric.Commit's classifier needs. warpPath is a bare t.TempDir() plain-git checkout (newPlainWarpRepo
// or a sibling unborn-warp fixture, not a real hub tree), so lyxcwd.ResolveWorktree(warpPath)'s
// HubPath — warpPath's parent directory — stands in for the hub; _board is not itself a git
// repository, so the file is written directly with no git add/commit step.
func seedFabricConfig(t *testing.T, warpPath string) {
	t.Helper()

	boardDir := BoardDir(filepath.Dir(warpPath))
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte("branch_prefix: \"\"\npathspec: _lyx _extra\n"), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// WriteWarpFileForTest re-exports writeWarpFile (relocated fixture helper, formerly
// commit_integration_test.go): commit_gating_integration_test.go, committed_lyxonly_integration_test.go,
// commit_partial_integration_test.go and commit_lock_integration_test.go (package fabricengine, never
// migrating) call it unqualified.
var WriteWarpFileForTest = writeWarpFile

// writeWarpFile overwrites (without staging or committing) name inside the warp repo at warpPath —
// the standard way a caller of this helper dirties a warp-side file before calling Fabric.Commit.
func writeWarpFile(t *testing.T, warpPath, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(warpPath, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// pushCall records one spawnDetachedPushFn invocation's arguments. WarpPath and WeftPath are
// exported (unlike the type name) so a caller outside this package can read the value
// SwapPushRecorderForTest returns' recorded calls' fields without this package needing to export the
// pushCall/pushRecorder names themselves — Go permits reading exported fields of an unexported type
// obtained from an exported function, and holding such a value via `:=` type inference, without ever
// needing to spell the unexported type name.
type pushCall struct {
	WarpPath string
	WeftPath string
}

// pushRecorder collects spawnDetachedPushFn invocations under a mutex.
//
// The mutex is not decoration: Fabric.Commit fires the push seam AFTER releasing the commit lock,
// deliberately, so a test driving concurrent Commit calls (commit_lock_integration_test.go) reaches
// the recorder from several goroutines at once. An unguarded slice append there is a real data race
// that fails the whole package under -race.
type pushRecorder struct {
	mu    sync.Mutex
	calls []pushCall
}

// record appends one invocation.
func (r *pushRecorder) record(warpPath, weftPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, pushCall{WarpPath: warpPath, WeftPath: weftPath})
}

// Calls returns a copy of everything recorded so far, safe to read while other goroutines are still
// recording.
func (r *pushRecorder) Calls() []pushCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]pushCall(nil), r.calls...)
}

// SwapPushRecorderForTest re-exports swapPushRecorder (relocated fixture helper, formerly
// commit_integration_test.go): commit_gating_integration_test.go, committed_lyxonly_integration_test.go,
// commit_partial_integration_test.go and commit_lock_integration_test.go (package fabricengine, never
// migrating) call it unqualified; the nine relocating files call the exported form.
var SwapPushRecorderForTest = swapPushRecorder

// swapPushRecorder replaces spawnDetachedPushFn with a no-op recorder for the duration of the test,
// restoring the original on cleanup — the push-invocation-seam-for-tests Shared Decision. Callers of
// this helper must not use t.Parallel().
func swapPushRecorder(t *testing.T) *pushRecorder {
	t.Helper()

	recorder := &pushRecorder{}
	original := spawnDetachedPushFn
	spawnDetachedPushFn = func(warpPath, weftPath string) error {
		recorder.record(warpPath, weftPath)
		return nil
	}
	t.Cleanup(func() { spawnDetachedPushFn = original })
	return recorder
}

// NewCommitFixtureForTest re-exports newCommitFixture (relocated fixture helper, formerly
// commit_integration_test.go): commit_gating_integration_test.go, committed_lyxonly_integration_test.go,
// commit_partial_integration_test.go and commit_lock_integration_test.go (package fabricengine, never
// migrating) call it unqualified; the nine relocating files call the exported form.
var NewCommitFixtureForTest = newCommitFixture

// newCommitFixture builds a fresh warp/weft pair with the fabric config seeded, returning the Fabric
// handle and both repo paths.
func newCommitFixture(t *testing.T) (f *Fabric, warpPath, weftPath string) {
	t.Helper()

	warpPath = newPlainWarpRepo(t)
	weftPath = newPlainWeftRepo(t)
	seedFabricConfig(t, warpPath)
	f = newFabric(t, warpPath, weftPath)
	return f, warpPath, weftPath
}

// newPlainWeftRepo creates a minimal, isolated git repo at t.TempDir() on branch main with a single
// tracked _lyx/config.yaml file and one commit — the weft-side sibling of newPlainWarpRepo, replacing
// gitkit's own retired weft-only template for newCommitFixture's four in-package callers, which
// cannot import the hubforge package.
func newPlainWeftRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitkit.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	gitkit.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, dir, "git", "config", "user.name", "Test")

	lyxDir := filepath.Join(dir, lyxdirs.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lyxDir, "config.yaml"), []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gitkit.MustRun(t, dir, "git", "add", ".")
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

// WriteWeftConfigContentForTest re-exports writeWeftConfigContent (relocated fixture helper, formerly
// syncweft_integration_test.go): commit_partial_integration_test.go, commit_gating_integration_test.go,
// committed_lyxonly_integration_test.go and commit_lock_integration_test.go (package fabricengine,
// never migrating) call it unqualified, and commit_integration_test.go/diff_integration_test.go need it
// before syncweft_integration_test.go's own migration card lands.
var WriteWeftConfigContentForTest = writeWeftConfigContent

// writeWeftConfigContent overwrites the tracked _lyx/config.yaml file newPlainWeftRepo (and a real
// hub's weft worktree alike) ships with, without staging or committing — the standard way a caller
// of this helper dirties a weft worktree's pathspec-covered content before calling
// CommitWeft/Fabric.Commit.
func writeWeftConfigContent(t *testing.T, weftPath, content string) {
	t.Helper()

	configPath := filepath.Join(weftPath, "_lyx", "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// WeftGitDirForTest re-exports f.weftGitDir (production plumbing: index.go), for
// index_integration_test.go's direct assertion on the per-worktree gitdir the correspondence index
// is scoped to.
func WeftGitDirForTest(f *Fabric) (string, error) {
	return f.weftGitDir()
}

// CorrIndexPathForTest re-exports f.corrIndexPath (production plumbing: index.go), for the several
// relocating files that load the correspondence index directly rather than through a Fabric method.
func CorrIndexPathForTest(f *Fabric) (string, error) {
	return f.corrIndexPath()
}

// CommitWeftForTest re-exports f.commitWeft (production plumbing: weftgit.go), for the several
// relocating files that drive the weft-only commit path directly rather than through Fabric.Commit,
// whose own classify-and-dispatch step would never reach it in isolation.
func CommitWeftForTest(f *Fabric, pathspec []string, message string, opts SyncOptions, snapshotTags ...string) (sha string, committed bool, err error) {
	return f.commitWeft(pathspec, message, opts, snapshotTags...)
}

// SnapshotWarpSHAForTest re-exports f.snapshotWarpSHA (production plumbing: snapshot.go), for the
// relocating files covering its newest-tagged-commit-wins, tag-isolation and staleness behavior.
func SnapshotWarpSHAForTest(f *Fabric, tag string) (string, error) {
	return f.snapshotWarpSHA(tag)
}

// WarpPathForTest re-exports f's private warpPath field, for pull_integration_test.go's direct
// assertion against the warp worktree's own path, distinct from WarpForTest's *gitrepo.Repo handle.
func WarpPathForTest(f *Fabric) string {
	return f.warpPath
}

// AppendWarpSHATrailerForTest re-exports appendWarpSHATrailer (production plumbing: trailer.go), for
// relocating files that hand-craft a commit message carrying a Warp-SHA trailer without going through
// CommitWeft.
var AppendWarpSHATrailerForTest = appendWarpSHATrailer

// AppendSnapshotTrailersForTest re-exports appendSnapshotTrailers (production plumbing: trailer.go),
// for snapshot_integration_test.go's hand-crafted, back-dated commit fixture, which cannot go through
// CommitWeft because it needs a caller-supplied GIT_COMMITTER_DATE.
var AppendSnapshotTrailersForTest = appendSnapshotTrailers

// ParseWarpSHATrailerForTest re-exports parseWarpSHATrailer (production plumbing: trailer.go), for
// commit_integration_test.go's assertions on a landed commit's Warp-SHA trailer.
var ParseWarpSHATrailerForTest = parseWarpSHATrailer

// WeftLockDirNameForTest re-exports weftLockDirName (production plumbing: weftgit.go), for
// diff_integration_test.go's assertion that Status never surfaces fabric's own git-excluded lock
// directory.
const WeftLockDirNameForTest = weftLockDirName

// CorrIndexForTest re-exports corrIndex's type identity (production plumbing: corrindex.go), for
// relocating files that load and inspect the correspondence index directly.
type CorrIndexForTest = corrIndex

// LoadCorrIndexForTest re-exports loadCorrIndex (production plumbing: corrindex.go), for relocating
// files that load the correspondence index directly rather than through a Fabric method.
var LoadCorrIndexForTest = loadCorrIndex

// CorrIndexEntriesForTest re-exports ix.entries (production plumbing: corrindex.go), for relocating
// files comparing an incrementally-built index against a RebuildIndex-reconstructed one. The returned
// []corrEntry is usable via type inference without this package needing to export corrEntry's own
// name, since corrEntry's fields are already exported.
func CorrIndexEntriesForTest(ix *CorrIndexForTest) []corrEntry {
	return ix.entries()
}

// CorrIndexExactForTest re-exports ix.exact (production plumbing: corrindex.go), for relocating files
// asserting on a specific warp SHA's recorded correspondence entry.
func CorrIndexExactForTest(ix *CorrIndexForTest, warpSHA string) (corrEntry, bool) {
	return ix.exact(warpSHA)
}

// PathspecNamesForTest re-exports pathspecNames (production plumbing: junctionnames.go), for
// weftgit_pathspec_integration_test.go's assertion on the real, resolved default routing set.
var PathspecNamesForTest = pathspecNames

// PartialCommitErrorWeftCommittedForTest re-exports e's private weftCommitted field (production
// plumbing: commit.go), for commit_integration_test.go's assertion distinguishing a landed-but-
// unrecorded weft commit from a weft commit that failed entirely — deliberately not part of
// PartialCommitError's public contract (only Error()'s rendered message is), but this file's own
// assertion needs the raw bool.
func PartialCommitErrorWeftCommittedForTest(e *PartialCommitError) bool {
	return e.weftCommitted
}

// HookSentinelForTest re-exports hookSentinel (production plumbing: hook.go), for hook_test.go's
// assertions on the fabric-managed hook marker after its package flip to fabricengine_test.
const HookSentinelForTest = hookSentinel

// WarpLayoutForForTest re-exports warpLayoutFor (production plumbing: warplayout.go), for
// warplayout_test.go's direct comparison of its spawn-free hub-sibling fast path against the
// spawning lyxcwd.ResolveWorktree fallback after its package flip to fabricengine_test.
var WarpLayoutForForTest = warpLayoutFor

// MergeStateForTest mirrors mergeState's fields (production plumbing: mergestate.go), for
// mergestate_integration_test.go's roundtrip assertions — the unexported record type itself is
// never exported, per this batch's owner-set carve-out, so tests build and compare this exported
// mirror instead.
type MergeStateForTest struct {
	Verb          string
	Source        string
	Squash        bool
	Message       string
	WarpStart     string
	WeftStart     string
	WarpSource    string
	WeftSource    string
	WarpOutcome   string
	WeftOutcome   string
	WarpCommitted string
	WeftCommitted string
	StartedAt     time.Time
}

// toMergeState converts a MergeStateForTest into the package-private mergeState saveMergeState
// expects.
func (m MergeStateForTest) toMergeState() *mergeState {
	return &mergeState{
		Verb:          m.Verb,
		Source:        m.Source,
		Squash:        m.Squash,
		Message:       m.Message,
		WarpStart:     m.WarpStart,
		WeftStart:     m.WeftStart,
		WarpSource:    m.WarpSource,
		WeftSource:    m.WeftSource,
		WarpOutcome:   m.WarpOutcome,
		WeftOutcome:   m.WeftOutcome,
		WarpCommitted: m.WarpCommitted,
		WeftCommitted: m.WeftCommitted,
		StartedAt:     m.StartedAt,
	}
}

// fromMergeState converts a *mergeState (or nil) into (MergeStateForTest, found).
func fromMergeState(st *mergeState) (MergeStateForTest, bool) {
	if st == nil {
		return MergeStateForTest{}, false
	}
	return MergeStateForTest{
		Verb:          st.Verb,
		Source:        st.Source,
		Squash:        st.Squash,
		Message:       st.Message,
		WarpStart:     st.WarpStart,
		WeftStart:     st.WeftStart,
		WarpSource:    st.WarpSource,
		WeftSource:    st.WeftSource,
		WarpOutcome:   st.WarpOutcome,
		WeftOutcome:   st.WeftOutcome,
		WarpCommitted: st.WarpCommitted,
		WeftCommitted: st.WeftCommitted,
		StartedAt:     st.StartedAt,
	}, true
}

// SaveMergeStateForTest re-exports f.saveMergeState (production plumbing: mergestate.go), taking
// the exported mirror type so callers outside this package never need the unexported mergeState
// name.
func SaveMergeStateForTest(f *Fabric, st MergeStateForTest) error {
	return f.saveMergeState(st.toMergeState())
}

// LoadMergeStateForTest re-exports f.loadMergeState (production plumbing: mergestate.go), returning
// the exported mirror type and a found bool in place of a nil-or-populated pointer.
func LoadMergeStateForTest(f *Fabric) (MergeStateForTest, bool, error) {
	st, err := f.loadMergeState()
	if err != nil {
		return MergeStateForTest{}, false, err
	}
	got, found := fromMergeState(st)
	return got, found, nil
}

// DeleteMergeStateForTest re-exports f.deleteMergeState (production plumbing: mergestate.go).
func DeleteMergeStateForTest(f *Fabric) error {
	return f.deleteMergeState()
}

// MergeRecordExistsForTest re-exports f.mergeRecordExists (production plumbing: mergestate.go).
func MergeRecordExistsForTest(f *Fabric) (bool, error) {
	return f.mergeRecordExists()
}

// ForeignMergeStatePresentForTest re-exports f.foreignMergeStatePresent (production plumbing:
// mergestate.go).
func ForeignMergeStatePresentForTest(f *Fabric) (bool, error) {
	return f.foreignMergeStatePresent()
}

// MergeStatePathForTest re-exports f.mergeStatePath (production plumbing: mergestate.go), for
// mergestate_integration_test.go's assertion that the record lands at <weft gitdir>/fabric-merge.json.
func MergeStatePathForTest(f *Fabric) (string, error) {
	return f.mergeStatePath()
}

// ResetMergeSidesForTest re-exports f.resetMergeSides (production plumbing: destroy.go), for
// mergestate_integration_test.go's direct assertion on the gated warp-only merge-abort reset —
// driven without going through a merge verb, none of which exist yet in this batch.
func ResetMergeSidesForTest(f *Fabric, rec *Mutations, warpSHA string) error {
	return f.resetMergeSides(rec, warpSHA)
}
