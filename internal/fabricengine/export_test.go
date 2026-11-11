// export_test.go exposes newPaired and the warp and weft fields.
// It also exposes the destructive gate's unexported executors and ownership predicates.
// Files in package fabricengine_test drive them directly, not through an application-level verb;
// that verb's own higher-level checks would short-circuit before the gate is reached.

package fabricengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// SetGCAutoThresholdForTest sets the loose-object estimate above which t's removal gc packs a store.
// It sets the threshold on the one Topology, so parallel tests inject per instance.
func SetGCAutoThresholdForTest(t *Topology, threshold int) {
	t.gcAutoThreshold = threshold
}

// GCAutoThresholdForTest returns the loose-object estimate above which t's removal gc packs a store.
func GCAutoThresholdForTest(t *Topology) int {
	return t.gcAutoThreshold
}

// SetScratchBeforeLinkHookForTest sets scratchBeforeLinkHook until the test ends.
// It changes process-global state, so the calling test must not be parallel.
func SetScratchBeforeLinkHookForTest(t *testing.T, hook func()) {
	t.Helper()

	scratchBeforeLinkHook = hook
	t.Cleanup(func() { scratchBeforeLinkHook = nil })
}

// SetAnchorFailHookForTest makes every anchor change t begins afterwards call hook around its weft commit or branch move.
// hook is called with afterCommit false before the step and true after it.
// An error it returns fails the change there.
// A nil hook restores production.
// It sets the hook on the one Topology, so parallel tests inject per instance.
func SetAnchorFailHookForTest(t *Topology, hook func(afterCommit bool) error) {
	t.anchor.fail = hook
}

// SetWeftHeadSHAForTest makes every anchor change t begins afterwards read the weft's commit after its move through fn.
// It sets the lookup on the one Topology, so parallel tests inject per instance.
func SetWeftHeadSHAForTest(t *Topology, fn func(weftPath string) (string, error)) {
	t.anchor.weftHead = fn
}

// SetScratchPathTrackedForTest replaces the scratch-path tracking check with fn until the test ends.
// It changes process-global state, so the calling test must not be parallel.
func SetScratchPathTrackedForTest(t *testing.T, fn func(worktreeDir, relPath string) (bool, error)) {
	t.Helper()

	scratchPathTracked = fn
	t.Cleanup(func() { scratchPathTracked = PathTracked })
}

// NewPairedFromPathsForTest exposes newPaired to the unit tests of the constructor itself.
// Such a test passes two empty directories and checks that the warp and weft fields come back non-nil.
// It is a constructor seam only and must never assemble a test fixture's warp/weft pair.
// A test that needs a real pair takes one from internal/hubforge instead.
var NewPairedFromPathsForTest = newPaired

// WarpForTest returns the warp-side gitrepo.Repo of f.
// It gives tests in package fabricengine_test direct access to that repository.
func WarpForTest(f *Fabric) *gitrepo.Repo {
	return f.code
}

// WeftForTest returns the weft-side gitrepo.Repo of f.
// It gives tests in package fabricengine_test direct access to that repository.
func WeftForTest(f *Fabric) *gitrepo.Repo {
	return f.records
}

// WarpProbeDirPrefixForTest is the prefix of the warp probe's throwaway-clone directory names.
// It lets tests in package fabricengine_test assert on those names without repeating the literal.
const WarpProbeDirPrefixForTest = warpProbeDirPrefix

// ExcludePatternForTest builds the anchored .git/info/exclude pattern for an anchor path and a name.
// It lets tests in package fabricengine_test assert on that pattern without repeating its spelling.
var ExcludePatternForTest = excludePatternFor

// IntegrateWeftForTest runs the integration of the weft at weftPath.
// It records into a fresh recorder for the directory that holds weftPath.
// Tests in package fabricengine_test call it to drive the integration directly.
func IntegrateWeftForTest(weftPath, upstream, target string, names []string, fallback string) (IntegrateOutcome, error) {
	return integrateWeft(NewMutations(filepath.Dir(weftPath)), weftPath, upstream, target, names, fallback)
}

// MoveLockPathForTest returns the path of the move lock for the weft at weftPath.
// It lets tests in package fabricengine_test hold that lock.
var MoveLockPathForTest = moveLockPath

// MatchParentBranchForTest finds the worktree path for a parent branch among a list of worktree entries.
// It lets tests in package fabricengine_test drive the match with hand-built WorktreeEntry slices.
// Such tests need no hubforge fixture.
var MatchParentBranchForTest = matchParentBranch

// RemoveWarpWorktreeDirForTest removes a warp worktree directory directly against an arbitrary target.
// Integration tests in package fabricengine_test use it to drive the gate's registered-linked-worktree ownership kind.
// The public remove path refuses an untracked warp worktree at its own dirty check.
// A test that goes through that path never reaches the fallback that runs when git refuses.
// It passes a zero leftoverSeam, which is the production behavior.
func RemoveWarpWorktreeDirForTest(rec *Mutations, l *lymcwd.Location, target string, force bool) error {
	return removeWarpWorktreeDir(rec, l, target, force, leftoverSeam{})
}

// TeardownHubForTest tears down the hub at hubPath, for integration tests in package fabricengine_test.
// A full clone always derives the hub path as a child of its working directory.
// It can therefore never produce a hub outside the operator-named parent.
// It creates hubPath itself to mint the token that teardown requires, so hubPath must not already exist.
// If seed is non-nil, it runs seed on the new directory before the teardown.
// It records into a throwaway recorder, since its callers assert nothing about the record.
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

// LooksLikeHubForTest reports whether hubPath looks like a fabric hub.
// It lets integration tests in package fabricengine_test cover the gate's fabric-hub ownership kind.
var LooksLikeHubForTest = looksLikeHub

// IsWarpCheckoutForTest reports whether target is a worktree of repoDir.
// It lets integration tests in package fabricengine_test cover the gate's warp-checkout ownership kind.
// The public reset entry point always passes the same path for both arguments.
// This seam is therefore needed to test a pair that differs.
var IsWarpCheckoutForTest = isAnyWorktreeOf

// IsRegisteredLinkedWorktreeInForTest exposes isRegisteredLinkedWorktreeIn for package fabricengine_test tests.
// It serves integration tests of the gate's registered-linked-worktree ownership kind.
// The check runs against an arbitrary (repoDir, target) pair.
var IsRegisteredLinkedWorktreeInForTest = isRegisteredLinkedWorktreeIn

// DeleteBranchForTest deletes branch in repoDir, subject to the branch-ownership gate.
// Integration tests in package fabricengine_test call it directly.
// The application-level orphan and liveness filter never reaches the gate for the primary weft branch.
// It never reaches the gate for a branch checked out at a worktree either.
// It records into a throwaway `NewMutations("")` recorder, since its callers assert nothing about the record.
func DeleteBranchForTest(l *lymcwd.Location, repoDir, branch string, force bool) error {
	req := branchRequest{
		what:      "test delete branch",
		repoDir:   repoDir,
		branch:    branch,
		ownership: ownedManagedBranch(l),
		dirtiness: dirtyCheckedOutBranch(),
		force:     force,
	}
	return deleteBranch(NewMutations(""), req)
}

// CheckRemoteBranchRequestForTest runs the remote-branch gate check for branch on remote.
// Integration tests in package fabricengine_test call it against a real hub.
// They cannot build a remoteBranchRequest themselves, since that type is unexported.
// An internal test file cannot import hubforge either.
// hubforge imports fabriccli, which imports fabricengine, so the import would close an import cycle.
// This seam lets the tests reach the gate.
// They still build their real-hub fixture through hubforge's factory function.
func CheckRemoteBranchRequestForTest(l *lymcwd.Location, repoDir, remote, branch string) error {
	req := remoteBranchRequest{
		what:      "test delete remote branch",
		repoDir:   repoDir,
		remote:    remote,
		branch:    branch,
		ownership: ownedManagedBranch(l),
		dirtiness: dirtyCheckedOutBranch(),
		force:     false,
	}
	return checkRemoteBranchRequest(req)
}

// DeleteArchivedWeftBranchForTest runs the remote-branch deletion for an archived weft branch.
// It uses pair-weft ownership and archived-on-remote dirtiness, and returns the recorded mutations.
// Package fabricengine_test integration tests use it, since they cannot build a remoteBranchRequest themselves.
func DeleteArchivedWeftBranchForTest(l *lymcwd.Location, repoDir, warpBranch, branch, archiveTag, leaseSHA string) (Mutations, error) {
	req := remoteBranchRequest{
		what:      "test delete archived weft branch",
		repoDir:   repoDir,
		remote:    originRemoteName,
		branch:    branch,
		ownership: ownedPairWeftBranch(l, warpBranch),
		dirtiness: dirtyArchivedOnRemote(archiveTag),
		leaseSHA:  leaseSHA,
	}
	rec := NewMutations("")
	_, err := deleteRemoteBranch(rec, req)
	return rec.Snapshot(), err
}

// DeleteArchivedTaskBranchForTest runs the archived task-branch deletion and reports which copies it deleted.
// Package fabricengine_test tests pass a tag or lease that the discard's own archive step never produces.
func DeleteArchivedTaskBranchForTest(repoDir, warpBranch, localTag, originTag, leaseTip string) (localDeleted, originDeleted bool, err error) {
	return deleteArchivedTaskBranch(NewMutations(""), repoDir, warpBranch, localTag, originTag, leaseTip)
}

// DeleteRemoteWithArchivedTipForTest runs the remote-branch deletion for a pair-warp branch.
// The dirtiness is a local-only archived tip, which the remote gate must refuse.
func DeleteRemoteWithArchivedTipForTest(repoDir, warpBranch, archiveTag, leaseSHA string) error {
	_, err := deleteRemoteBranch(NewMutations(""), remoteBranchRequest{
		what:      "test delete remote branch under a local dirtiness kind",
		repoDir:   repoDir,
		remote:    originRemoteName,
		branch:    warpBranch,
		ownership: ownedPairWarpBranch(warpBranch, ""),
		dirtiness: dirtyArchivedTip(archiveTag),
		leaseSHA:  leaseSHA,
	})
	return err
}

// UpdateRemoteBranchWithLeaseForTest updates the remote branch for f's pair warp checkout.
// It takes a caller-chosen lease SHA and returns the recorded mutations.
// Package fabricengine_test tests use it to present a lease that is stale against origin.
// `ResetPairCode` fetches and updates with no window between them, so a stale lease cannot arise there.
func UpdateRemoteBranchWithLeaseForTest(f *Fabric, sha, parentBranch, leaseSHA string) (Mutations, error) {
	branch, err := f.code.CurrentBranch()
	if err != nil {
		return Mutations{}, err
	}
	rec := NewMutations("")
	err = updateRemoteBranch(rec, remoteBranchUpdateRequest{
		pathReq: pathRequest{
			what:      "test update remote task branch",
			container: filepath.Dir(f.warpPath),
			target:    f.warpPath,
			ownership: ownedPairWarpCheckout(f.warpPath, f.weftPath, parentBranch),
			dirtiness: dirtyTrackedExcept(nil),
			force:     false,
		},
		repo:         f.code,
		remote:       originRemoteName,
		branch:       branch,
		parentBranch: parentBranch,
		sha:          sha,
		leaseSHA:     leaseSHA,
	})
	return rec.Snapshot(), err
}

// The entries below expose unexported identifiers to package fabricengine_test.
// Some alias production identifiers; others alias fixture helpers whose bodies this file defines.

// NewFabricForTest re-exports newFabric to tests outside package fabricengine.
// newFabric fails the test on error rather than returning it, so callers always get a valid pair.
var NewFabricForTest = newFabric

// newFabric wraps newPaired and fails the test on error instead of returning it; callers expect a valid pair.
func newFabric(t *testing.T, warpPath, weftPath string) *Fabric {
	t.Helper()

	f, err := newPaired(warpPath, weftPath)
	if err != nil {
		t.Fatalf("newPaired(%q, %q) error = %v", warpPath, weftPath, err)
	}
	return f
}

// SeedFabricConfigForTest re-exports seedFabricConfig to tests outside package fabricengine.
var SeedFabricConfigForTest = seedFabricConfig

// seedFabricConfig writes the repo-wide fabric config that Fabric.Commit's classifier reads.
// The classifier reads it from the `weft:main` base at BoardDir(hub).
// warpPath is a plain git checkout in a temporary directory, so its parent directory stands in for the hub.
// The Bolt checkout is not itself a git repository, so the file is written directly without a git add or commit step.
func seedFabricConfig(t *testing.T, warpPath string) {
	t.Helper()

	boardDir := BoardDir(filepath.Dir(warpPath))
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte("branch_prefix: \"\"\npathspec: _lym _extra\n"), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// WriteWarpFileForTest re-exports writeWarpFile to tests outside package fabricengine.
var WriteWarpFileForTest = writeWarpFile

// writeWarpFile overwrites name in the warp repo at warpPath without staging or committing it.
// Tests use it to dirty a warp-side file before calling Fabric.Commit.
func writeWarpFile(t *testing.T, warpPath, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(warpPath, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// pushCall records the arguments of one spawnDetachedPushFn invocation.
// Its fields are exported so that a test in another package can read recorded calls.
type pushCall struct {
	WarpPath     string
	WeftPath     string
	PauseSlug    string
	TeardownSlug string
	ParentLocked bool
	WorkDir      string
}

// pushRecorder collects spawnDetachedPushFn invocations under a mutex.
//
// Fabric.Commit fires the push seam after releasing the commit lock.
// Concurrent Commit calls therefore reach the recorder from several goroutines at once.
// An unguarded append to calls would be a data race that fails the package under -race.
type pushRecorder struct {
	mu    sync.Mutex
	calls []pushCall
	// Err is returned by the swapped push seam, nil unless a test sets it.
	Err error
}

func (r *pushRecorder) record(req PushSpawn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, pushCall{
		WarpPath:     req.WarpPath,
		WeftPath:     req.WeftPath,
		PauseSlug:    req.PauseSlug,
		TeardownSlug: req.TeardownSlug,
		ParentLocked: req.ParentLocked,
		WorkDir:      req.WorkDir,
	})
}

// Reset forgets everything recorded so far.
func (r *pushRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = nil
}

// Calls returns a copy of the recorded invocations, safe to call while other goroutines still record.
func (r *pushRecorder) Calls() []pushCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]pushCall(nil), r.calls...)
}

// SwapPushRecorderForTest re-exports swapPushRecorder to tests outside package fabricengine.
var SwapPushRecorderForTest = swapPushRecorder

// swapPushRecorder replaces spawnDetachedPushFn with a recorder that captures each push instead of spawning it.
// The swap lasts for the duration of the test and is undone on cleanup.
// Tests that call it must not use t.Parallel().
func swapPushRecorder(t *testing.T) *pushRecorder {
	t.Helper()

	recorder := &pushRecorder{}
	original := spawnDetachedPushFn
	spawnDetachedPushFn = func(req PushSpawn) error {
		recorder.record(req)
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		return recorder.Err
	}
	t.Cleanup(func() { spawnDetachedPushFn = original })
	return recorder
}

// TreeAnchorForTest exposes treeAnchor, the rule that derives the anchor of a branch recording none.
// A table test pins that rule through it.
var TreeAnchorForTest = treeAnchor

// ShowFileForTest exposes showFile, the read of a file's bytes at a revision.
var ShowFileForTest = showFile

// WriteWeftConfigContentForTest exposes writeWeftConfigContent to tests outside the package.
var WriteWeftConfigContentForTest = writeWeftConfigContent

// writeWeftConfigContent overwrites the tracked _lym/config.yaml in weftPath without staging or committing.
// It leaves the weft worktree dirty in its pathspec-covered content, the state a commit test starts from.
func writeWeftConfigContent(t *testing.T, weftPath, content string) {
	t.Helper()

	configPath := filepath.Join(weftPath, "_lym", "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// WeftGitDirForTest returns the per-worktree git directory that the correspondence index is scoped to.
// It exposes f.weftGitDir so tests can assert on that directory directly.
func WeftGitDirForTest(f *Fabric) (string, error) {
	return f.weftGitDir()
}

// CorrIndexPathForTest returns the path of the correspondence index.
// Tests use it to load the index directly, without going through a Fabric method.
func CorrIndexPathForTest(f *Fabric) (string, error) {
	return f.corrIndexPath()
}

// CommitWeftForTest exposes commitWeft, the weft-only commit path, to tests outside the package.
// Tests call it to reach that path directly;
// the classify-and-dispatch step in front of it never reaches it in isolation.
func CommitWeftForTest(f *Fabric, pathspec []string, message string, opts SyncOptions, snapshotTags ...string) (sha string, committed bool, err error) {
	return f.commitWeft(pathspec, message, opts, snapshotTags...)
}

// SnapshotWarpSHAForTest exposes f.snapshotWarpSHA to tests outside the package.
// Tests use it to check that the newest tagged commit wins, along with tag isolation and staleness.
func SnapshotWarpSHAForTest(f *Fabric, tag string) (string, error) {
	return f.snapshotWarpSHA(tag)
}

// WarpPathForTest returns the path of the warp worktree itself, not a repository handle.
// It exposes f's warpPath field so tests can assert against that path directly.
func WarpPathForTest(f *Fabric) string {
	return f.warpPath
}

// AppendWarpSHATrailerForTest exposes appendWarpSHATrailer, which appends a Warp-SHA trailer to a commit message.
// Tests use it to hand-craft such a commit message, bypassing the normal commit path.
var AppendWarpSHATrailerForTest = appendWarpSHATrailer

// AppendSnapshotTrailersForTest exposes appendSnapshotTrailers, which appends snapshot trailers to a commit message.
// Tests use it to build a back-dated commit fixture.
// The normal commit path cannot take a caller-supplied GIT_COMMITTER_DATE, which the fixture needs.
var AppendSnapshotTrailersForTest = appendSnapshotTrailers

// ParseWarpSHATrailerForTest exposes parseWarpSHATrailer, which reads the Warp-SHA trailer of a commit message.
// Tests use it to assert on the trailer of a landed commit.
var ParseWarpSHATrailerForTest = parseWarpSHATrailer

// WeftLockDirNameForTest exposes weftLockDirName, the name of the lock directory that git excludes.
// Tests use it to assert that Status never reports that directory.
const WeftLockDirNameForTest = weftLockDirName

// CorrIndexForTest is an alias of corrIndex, for tests that load and inspect the correspondence index directly.
type CorrIndexForTest = corrIndex

// LoadCorrIndexForTest exposes loadCorrIndex, which loads the correspondence index.
// Tests use it to load the index directly, without going through a Fabric method.
var LoadCorrIndexForTest = loadCorrIndex

// CorrIndexEntriesForTest exposes `ix.entries` to tests.
// It lets a test compare an incrementally built index with one rebuilt by RebuildIndex.
// The result is a []corrEntry. Its element type stays unexported.
// Tests use it through type inference, since corrEntry's fields are exported.
func CorrIndexEntriesForTest(ix *CorrIndexForTest) []corrEntry {
	return ix.entries()
}

// CorrIndexExactForTest exposes `ix.exact` to tests.
// It lets a test assert on the correspondence entry recorded for a warp SHA.
func CorrIndexExactForTest(ix *CorrIndexForTest, warpSHA string) (corrEntry, bool) {
	return ix.exact(warpSHA)
}

// PathspecNamesForTest exposes `pathspecNames` to tests.
// It lets a test assert on the real, resolved default routing set.
var PathspecNamesForTest = pathspecNames

// PartialCommitErrorWeftCommittedForTest reports whether the weft commit landed.
// It reads the private `weftCommitted` field of `e`.
// Tests use it to tell a landed but unrecorded commit from one that failed entirely.
// The field is outside PartialCommitError's public contract, which covers only the rendered Error() message.
func PartialCommitErrorWeftCommittedForTest(e *PartialCommitError) bool {
	return e.weftCommitted
}

// HookSentinelForTest exposes `hookSentinel` to tests.
// It lets a test assert on the fabric-managed hook marker.
const HookSentinelForTest = hookSentinel

// WarpLayoutForForTest exposes `warpLayoutFor` to tests.
// It lets a test compare the spawn-free hub-sibling fast path with the fallback.
// The fallback calls `lymcwd.ResolveWorktree`, which spawns a process.
var WarpLayoutForForTest = warpLayoutFor

// MergeStateForTest mirrors the fields of the unexported `mergeState` record.
// Tests round-trip this exported mirror, since the record type itself stays unexported.
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

// toMergeState converts m into the unexported `mergeState` record, so the package can save it.
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

// fromMergeState converts st into a MergeStateForTest, and reports whether st is non-nil.
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

// SaveMergeStateForTest exposes `f.saveMergeState`, taking the exported MergeStateForTest mirror.
// Callers need not name the unexported `mergeState` record.
func SaveMergeStateForTest(f *Fabric, st MergeStateForTest) error {
	return f.saveMergeState(st.toMergeState())
}

// LoadMergeStateForTest exposes `f.loadMergeState`, returning the exported MergeStateForTest mirror.
// It returns a found bool in place of a nil-or-populated pointer.
func LoadMergeStateForTest(f *Fabric) (MergeStateForTest, bool, error) {
	st, err := f.loadMergeState()
	if err != nil {
		return MergeStateForTest{}, false, err
	}
	got, found := fromMergeState(st)
	return got, found, nil
}

// DeleteMergeStateForTest exposes `f.deleteMergeState` to tests.
func DeleteMergeStateForTest(f *Fabric) error {
	return f.deleteMergeState()
}

// MergeRecordExistsForTest exposes `f.mergeRecordExists` to tests.
func MergeRecordExistsForTest(f *Fabric) (bool, error) {
	return f.mergeRecordExists()
}

// ForeignMergeStatePresentForTest exposes `f.foreignMergeStatePresent` to tests.
func ForeignMergeStatePresentForTest(f *Fabric) (bool, error) {
	return f.foreignMergeStatePresent()
}

// MergeStatePathForTest exposes `f.mergeStatePath`.
// It lets a test assert that the merge record lands at `<weft gitdir>/fabric-merge.json`.
func MergeStatePathForTest(f *Fabric) (string, error) {
	return f.mergeStatePath()
}

// ResetMergeSidesForTest exposes `f.resetMergeSides`.
// It lets a test drive the gated warp-only merge-abort reset directly, without going through a merge verb.
func ResetMergeSidesForTest(f *Fabric, rec *Mutations, warpSHA string) error {
	return f.resetMergeSides(rec, warpSHA)
}

// ArchiveWeftTipForTest runs `archiveWeftTip` without a branch push or a sleep seam.
// It returns the local tip's tag, the skipped reason and the error.
// Tests in the external test package use it, since they cannot read the unexported fields of `weftArchive`.
func ArchiveWeftTipForTest(rec *Mutations, l *lymcwd.Location, slug, weftBranch string) (tag, skippedReason string, err error) {
	archive, skippedReason, err := archiveWeftTip(rec, l, slug, weftBranch, "", false, nil)
	return archive.localTag, skippedReason, err
}

// SetArchiveTagPushHookForTest sets `archiveTagPushHook` to hook and restores nil in t.Cleanup.
// It mutates a package variable, so only a non-parallel test may call it.
func SetArchiveTagPushHookForTest(t testing.TB, hook func(tagRef string) error) {
	t.Helper()

	archiveTagPushHook = hook
	t.Cleanup(func() { archiveTagPushHook = nil })
}

// SetArchiveSleepForTest replaces the real sleep between t's archive-tag push attempts with sleep.
// The seam sits on each Topology, so parallel tests inject their own sleep per instance.
func SetArchiveSleepForTest(t *Topology, sleep func(time.Duration)) {
	t.archiveSleep = sleep
}

// SetAddBeforeWeftReplaceHookForTest sets addBeforeWeftReplaceHook to fn and restores the previous value in t.Cleanup.
// It mutates a package var, so it is usable only from a non-parallel test.
func SetAddBeforeWeftReplaceHookForTest(t testing.TB, fn func()) {
	t.Helper()

	original := addBeforeWeftReplaceHook
	addBeforeWeftReplaceHook = fn
	t.Cleanup(func() { addBeforeWeftReplaceHook = original })
}

// SetAddFailAfterWarpHookForTest sets addFailAfterWarpHook to hook and restores the previous value in t.Cleanup.
// It mutates a package var, so it is usable only from a non-parallel test.
func SetAddFailAfterWarpHookForTest(t testing.TB, hook func() error) {
	t.Helper()

	original := addFailAfterWarpHook
	addFailAfterWarpHook = hook
	t.Cleanup(func() { addFailAfterWarpHook = original })
}

// DeleteTaskBranchAtTipForTest exposes the gated task-branch deletion to package fabricengine_test integration tests.
// They hand it a tip other than the one currently on origin, to prove the lease holds.
// It passes a throwaway NewMutations("") recorder, since its callers assert nothing about the record.
func DeleteTaskBranchAtTipForTest(l *lymcwd.Location, warpBranch, parentBranch, tip string) (deleted bool, keptReason string) {
	deleted, keptReason, _ = deleteTaskBranchAtTip(NewMutations(""), l, warpBranch, parentBranch, tip)
	return deleted, keptReason
}

// CommitPendingRecordsForTest exposes the pending-record commit to package fabricengine_test integration tests.
// They drive a sibling location that Remove would refuse or tear down before the commit is observable on its own.
// It passes a throwaway NewMutations("") recorder, since its callers assert nothing about the record.
func CommitPendingRecordsForTest(l *lymcwd.Location, slug, warpBranch string) (committed bool, err error) {
	return commitPendingRecords(NewMutations(""), l, slug, warpBranch, false)
}

// CleanupRemoteWarpWithHookForTest runs the cleanup and calls afterEnumerate between enumeration and the first deletion,
// so an integration test can move a branch tip after it was observed and prove the lease holds.
func CleanupRemoteWarpWithHookForTest(t *Topology, l *lymcwd.Location, apply bool, openPRHeads map[string]bool, afterEnumerate func()) (RemoteWarpCleanupResult, error) {
	return t.cleanupRemoteWarp(l, apply, openPRHeads, afterEnumerate)
}

// SetLeftoverSeamForTest makes t's Remove finish a directory git deregistered but left on disk.
// It uses remove in place of the contained removal and sleep in place of a real sleep.
// A nil argument keeps the production behaviour for that part.
// It sets the seam on the one Topology, so parallel tests inject per instance.
func SetLeftoverSeamForTest(t *Topology, remove func(container, target string) (removed, wasDir bool, err error), sleep func(delay time.Duration)) {
	t.leftover.remove = remove
	t.leftover.sleep = sleep
}

// SetLeftoverRefillForTest makes t's Remove call refill with each path that `git worktree remove` exited cleanly on.
// It runs before Remove looks for a directory left on disk, so a test can play a late writer refilling it.
// It sets the seam on the one Topology, so parallel tests inject per instance.
func SetLeftoverRefillForTest(t *Topology, refill func(target string)) {
	t.leftover.refill = refill
}

// SetTeardownWaitForTest shortens how long t's Remove waits for the pair's teardown lock.
// It sets the wait on the one Topology, so parallel tests inject per instance.
func SetTeardownWaitForTest(t *Topology, wait time.Duration) {
	t.teardownWait = wait
}

// SetCleanupAfterEnumerateForTest makes t's Cleanup call hook once per orphan.
// It calls hook right before the orphan's slug's teardown lock is taken.
// It sets the hook on the one Topology, so parallel tests inject per instance.
func SetCleanupAfterEnumerateForTest(t *Topology, hook func()) {
	t.cleanupAfterEnumerate = hook
}

// SetTeardownHooksForTest sets the two teardown hooks until the test ends and restores nil on cleanup.
// A nil argument leaves that hook unset.
// It changes process-global state, so the calling test must not be parallel.
func SetTeardownHooksForTest(t *testing.T, heldByPush, afterRemoveTeardown func()) {
	t.Helper()

	detachedPushHeldHook = heldByPush
	removeAfterTeardownHook = afterRemoveTeardown
	t.Cleanup(func() {
		detachedPushHeldHook = nil
		removeAfterTeardownHook = nil
	})
}

// SetHousekeepHookForTest sets the hook that runs before a Remove's store housekeeping.
// It returns the function that restores the previous one.
// It changes process-global state, so the calling test must not be parallel.
func SetHousekeepHookForTest(hook func(slug string)) (restore func()) {
	previous := housekeepHook
	housekeepHook = hook
	return func() { housekeepHook = previous }
}

// SetRemoveAfterLockHookForTest sets the hook that runs inside a removal right after it takes the teardown lock.
// An error from the hook fails the removal.
// It returns the function that restores the previous hook.
// It changes process-global state, so the calling test must not be parallel.
func SetRemoveAfterLockHookForTest(hook func() error) (restore func()) {
	previous := removeAfterLockHook
	removeAfterLockHook = hook
	return func() { removeAfterLockHook = previous }
}

// SetRemoveBeforeWeftDeleteHookForTest sets the hook that runs before each leased delete of a weft branch on origin.
// It restores nil in t.Cleanup.
// It mutates a package var, so it is usable only from a non-parallel test.
func SetRemoveBeforeWeftDeleteHookForTest(t testing.TB, hook func()) {
	t.Helper()

	removeBeforeWeftDeleteHook = hook
	t.Cleanup(func() { removeBeforeWeftDeleteHook = nil })
}

// DropSeedCommitsForTest exposes the board seed-commit drop's gate to package fabricengine_test integration tests.
// They drive it directly.
var DropSeedCommitsForTest = dropSeedCommits

// BoardDropRequestForTest exposes the board drop's gate request builder to package fabricengine_test files.
// They build the request without naming the unexported request type.
var BoardDropRequestForTest = boardDropRequest

// lockWaitWaiters maps a weft write lock path to the channel its test waits on.
var lockWaitWaiters sync.Map

// installLockWaitDispatch installs the lock-wait hook once for the process.
var installLockWaitDispatch sync.Once

// AwaitWeftLockWaitForTest registers lockPath and returns a channel.
// The channel closes when a merge verb is about to wait for the weft write lock at that path.
// Only the registered path is signalled, so parallel tests each wait for their own verb.
func AwaitWeftLockWaitForTest(t *testing.T, lockPath string) <-chan struct{} {
	t.Helper()

	installLockWaitDispatch.Do(func() {
		hook := func(path string) {
			if waiter, ok := lockWaitWaiters.LoadAndDelete(path); ok {
				close(waiter.(chan struct{}))
			}
		}
		lockWaitHook.Store(&hook)
	})
	signal := make(chan struct{})
	lockWaitWaiters.Store(lockPath, signal)
	t.Cleanup(func() { lockWaitWaiters.Delete(lockPath) })
	return signal
}

// RegenerateHubWorkspaceWithinForTest regenerates the hub workspace with the lock wait the caller passes.
// A test passes a short wait, so it does not sit out the production wait.
func RegenerateHubWorkspaceWithinForTest(rec *Mutations, l *lymcwd.Location, wait time.Duration) (string, error) {
	return regenerateHubWorkspace(rec, l, wait)
}

// SetHubWorkspaceBeforeLockHookForTest sets the hook that runs just before the hub workspace lock is taken.
// The hook is reset to nil when the test ends.
// It changes process-global state, so the calling test must not be parallel.
func SetHubWorkspaceBeforeLockHookForTest(t *testing.T, hook func()) {
	t.Helper()

	hubWorkspaceBeforeLockHook = hook
	t.Cleanup(func() { hubWorkspaceBeforeLockHook = nil })
}

// ReplayWeftForTest replays the weft of worktree against its configured upstream as last fetched.
// The replay records into a throwaway recorder.
func ReplayWeftForTest(l *lymcwd.Location, worktree string, ended map[string]string) (ReplayOutcome, error) {
	upstream, err := gitrepo.New(worktree).UpstreamSHA()
	if err != nil {
		return ReplayOutcome{}, err
	}
	return replayWeft(NewMutations(l.HubPath), l, worktree, upstream, EndedSet(ended))
}

// ReplayBoltForTest replays the Bolt board in boardDir against its configured upstream as last fetched.
// The replay records into a throwaway recorder.
func ReplayBoltForTest(l *lymcwd.Location, boardDir string, ended map[string]string) (ReplayOutcome, error) {
	upstream, err := gitrepo.New(boardDir).UpstreamSHA()
	if err != nil {
		return ReplayOutcome{}, err
	}
	return replayBolt(NewMutations(l.HubPath), l, boardDir, upstream, EndedSet(ended))
}

// WithoutBoltReplayersForTest empties the registry of Bolt replayers.
// The returned function restores the set it held.
// It changes process-global state, so the calling test must not be parallel.
func WithoutBoltReplayersForTest() (restore func()) {
	previous := boltReplayerSet.Swap(&[]BoltReplayer{})
	return func() { boltReplayerSet.Store(previous) }
}

// ReadEndedSetForTest reads the ended set for slugs and returns it as a plain map[string]string.
func ReadEndedSetForTest(recordsRoot, boltRef string, slugs []string, localCreated map[string]string) (map[string]string, error) {
	ended, err := readEndedSet(recordsRoot, boltRef, slugs, localCreated)
	return map[string]string(ended), err
}

// RecordsConflictWayForwardForTest is recordsConflictWayForward.
// That function produces the way-forward line a records conflict names, so a test can assert its exact text.
var RecordsConflictWayForwardForTest = recordsConflictWayForward

// PushStepForTest is pushStep, the push step a way-forward line ends with.
var PushStepForTest = pushStep

// RewoundWayForwardForTest is rewoundWayForward.
// That function produces the way-forward line a rewound task branch names, so a test can assert its exact text.
var RewoundWayForwardForTest = rewoundWayForward

// CodeDivergedWayForwardForTest is codeDivergedWayForward.
// That function produces the way-forward line a diverged code branch names, so a test can assert its exact text.
var CodeDivergedWayForwardForTest = codeDivergedWayForward

// StopWayForwardForTest is BranchOutcome.stopWayForward, the repair step a stopped branch names.
func StopWayForwardForTest(b BranchOutcome) string {
	return b.stopWayForward()
}

// WeftDirtyStopForTest is weftDirtyStop for a Fabric holding only warpPath and weftPath.
func WeftDirtyStopForTest(warpPath, weftPath, branch string, paths []string) *WeftStopError {
	return (&Fabric{warpPath: warpPath, weftPath: weftPath}).weftDirtyStop(branch, paths, errors.New("dirty"))
}

// EntryFactsForTest is entryFacts, the facts an entry is classified from, including OwnedElsewhere.
type EntryFactsForTest = entryFacts

// ClassifyEntryForTest is classifyEntry, which judges an entry from its facts.
// A test drives its precedence as table rows.
var ClassifyEntryForTest = classifyEntry

// RecordPushFailureNotifyingForTest exposes recordPushFailure.
// It writes a failed-push record and mails it through the notifier the test passes, never the installed one.
var RecordPushFailureNotifyingForTest = recordPushFailure

// HoldHubPauseForTest installs a Defer gate for slug over a temp pause record that holds one live hub entry.
// The gate stays installed until the test ends.
// It changes process-global state, so the calling test must not be parallel.
func HoldHubPauseForTest(t *testing.T, slug string) {
	t.Helper()

	recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
	now := time.Now()
	if _, err := pauserecord.Update(recordPath, now, func(r *pauserecord.Record) error {
		r.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, PausedAt: now}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{
		Geometry: pauserecord.Geometry{RecordPath: recordPath, Slug: slug},
		Defer:    true,
		Notify:   func(string) {},
		Sleep:    func(context.Context, time.Duration) error { return errors.New("the test's pause is never lifted") },
	}))
}

// SetBeforeWireForTest makes the Add call on t run hook once the new pair's weft worktree exists.
// The hook runs before the pair's junctions are wired.
// It sets the hook on the one Topology, so parallel tests inject per instance.
func SetBeforeWireForTest(t *Topology, hook func()) {
	t.beforeWire = hook
}
