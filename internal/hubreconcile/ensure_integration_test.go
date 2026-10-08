//go:build integration

// ensure_integration_test.go covers Ensure against real hubforge hubs: the stale-build walk, the stamp gate, the mid-merge and incomplete-pair skips, the lock, concurrent callers, a pair registered mid-walk and the single-pair call.

package hubreconcile_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lock"
)

const retiredKey = "master_base"

// geometryOf returns the walk geometry of h, listed from its prime.
func geometryOf(h *hubforge.Hub) hubreconcile.Geometry {
	return hubreconcile.Geometry{BoardDir: h.BoardDir(), WorktreePath: h.PrimeWorktree()}
}

// runningKey returns the build key the test binary would stamp.
func runningKey() string {
	return hubreconcile.BuildKey(buildvcs.Running(), configreg.Fingerprint())
}

// stampKey returns the build key the hub's stamp holds, and whether a stamp exists.
func stampKey(t *testing.T, geom hubreconcile.Geometry) (string, bool) {
	t.Helper()

	data, err := os.ReadFile(geom.StampPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	var s struct {
		BuildKey string `json:"build_key"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse stamp: %v", err)
	}
	return s.BuildKey, true
}

// writeCurrentStamp marks the hub as already reconciled by the test binary's build.
func writeCurrentStamp(t *testing.T, geom hubreconcile.Geometry) {
	t.Helper()

	data, err := json.Marshal(map[string]string{"build_key": runningKey()})
	if err != nil {
		t.Fatalf("marshal stamp: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(geom.StampPath()), 0o755); err != nil {
		t.Fatalf("mkdir stamp dir: %v", err)
	}
	if err := os.WriteFile(geom.StampPath(), data, 0o644); err != nil {
		t.Fatalf("write stamp: %v", err)
	}
}

// seedRetiredKey commits a batcher.yaml carrying a retired key in the records worktree at recordsRoot.
func seedRetiredKey(t *testing.T, recordsRoot string) {
	t.Helper()

	module, _ := configreg.Lookup("batcher")
	content := strings.Replace(module.Template(), "orientation: 31400", retiredKey+": 52000", 1)
	if !strings.Contains(content, retiredKey) {
		t.Fatalf("batcher template no longer carries the orientation line the fixture rewrites")
	}
	gitkit.CommitFile(t, recordsRoot, configengine.ConfigFileRel("batcher"), content, "fixture: retired key")
}

// retiredKeyPresent reports whether the batcher.yaml at codeRoot still carries the retired key.
func retiredKeyPresent(t *testing.T, codeRoot string) bool {
	t.Helper()

	data, err := os.ReadFile(configengine.ConfigFile(codeRoot, "batcher"))
	if err != nil {
		t.Fatalf("read batcher.yaml: %v", err)
	}
	return strings.Contains(string(data), retiredKey)
}

// commitCount returns the number of commits reachable from HEAD in dir.
func commitCount(t *testing.T, dir string) int {
	t.Helper()
	return gitkit.RevListCount(t, dir, "HEAD")
}

// assertClean fails when dir has uncommitted changes.
func assertClean(t *testing.T, dir string) {
	t.Helper()

	if status := gitkit.GitStatusPorcelain(t, dir); status != "" {
		t.Errorf("%s is dirty after Ensure:\n%s", dir, status)
	}
}

// markMergeRecord writes a fabric merge record into the gitdir of the records worktree and returns the func that clears it.
func markMergeRecord(t *testing.T, recordsDir string) (clear func()) {
	t.Helper()

	path := gitkit.Git(t, recordsDir, "rev-parse", "--path-format=absolute", "--git-path", "fabric-merge.json")
	if err := os.WriteFile(path, []byte(`{"verb":"merge-in","source":"other"}`), 0o644); err != nil {
		t.Fatalf("write merge record: %v", err)
	}
	return func() {
		if err := os.Remove(path); err != nil {
			t.Fatalf("clear merge record: %v", err)
		}
	}
}

// markForeignMerge leaves a MERGE_HEAD in the code worktree's gitdir and returns the func that clears it.
func markForeignMerge(t *testing.T, codeDir string) (clear func()) {
	t.Helper()

	head := gitkit.RevParse(t, codeDir, "HEAD")
	path := gitkit.Git(t, codeDir, "rev-parse", "--path-format=absolute", "--git-path", "MERGE_HEAD")
	if err := os.WriteFile(path, []byte(head+"\n"), 0o644); err != nil {
		t.Fatalf("write MERGE_HEAD: %v", err)
	}
	return func() {
		if err := os.Remove(path); err != nil {
			t.Fatalf("clear MERGE_HEAD: %v", err)
		}
	}
}

// newStaleHub builds a hub with a pair, both carrying the retired key committed on their records side, and no stamp.
func newStaleHub(t *testing.T, slug string) *hubforge.Hub {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, slug)
	seedRetiredKey(t, h.PrimeRecords())
	seedRetiredKey(t, h.PairRecordsSibling(slug))
	return h
}

func TestEnsure_StaleBuildReconcilesAndCommitsEveryWorktree(t *testing.T) {
	t.Parallel()

	h := newStaleHub(t, "pair-a")
	geom := geometryOf(h)
	pair := h.PairCodeWorktree("pair-a")
	primeBefore, pairBefore := commitCount(t, h.PrimeRecords()), commitCount(t, h.PairRecordsSibling("pair-a"))

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	for name, root := range map[string]string{"prime": h.PrimeWorktree(), "pair": pair} {
		if retiredKeyPresent(t, root) {
			t.Errorf("%s batcher.yaml still carries %s", name, retiredKey)
		}
	}
	assertClean(t, h.PrimeRecords())
	assertClean(t, h.PairRecordsSibling("pair-a"))
	primeAfter, pairAfter := commitCount(t, h.PrimeRecords()), commitCount(t, h.PairRecordsSibling("pair-a"))
	if primeAfter != primeBefore+1 || pairAfter != pairBefore+1 {
		t.Errorf("records commits: prime %d -> %d, pair %d -> %d; want one new commit each", primeBefore, primeAfter, pairBefore, pairAfter)
	}
	if key, found := stampKey(t, geom); !found || key != runningKey() {
		t.Errorf("stamp = (%q, %v); want the running key", key, found)
	}

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if got := commitCount(t, h.PrimeRecords()); got != primeAfter {
		t.Errorf("second Ensure landed a prime commit: %d -> %d", primeAfter, got)
	}
}

func TestEnsure_CurrentStampLeavesConfigAlone(t *testing.T) {
	t.Parallel()

	h := newStaleHub(t, "pair-a")
	geom := geometryOf(h)
	writeCurrentStamp(t, geom)
	before := commitCount(t, h.PrimeRecords())

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if !retiredKeyPresent(t, h.PrimeWorktree()) {
		t.Errorf("a current stamp still reconciled the prime")
	}
	if got := commitCount(t, h.PrimeRecords()); got != before {
		t.Errorf("prime commits %d -> %d; want none", before, got)
	}
}

func TestEnsure_MidMergePairIsLeftAloneAndRetried(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mark func(t *testing.T, h *hubforge.Hub) (clear func())
	}{
		{"fabric merge record", func(t *testing.T, h *hubforge.Hub) func() { return markMergeRecord(t, h.PairRecordsSibling("pair-a")) }},
		{"foreign MERGE_HEAD", func(t *testing.T, h *hubforge.Hub) func() { return markForeignMerge(t, h.PairCodeWorktree("pair-a")) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newStaleHub(t, "pair-a")
			geom := geometryOf(h)
			pair := h.PairCodeWorktree("pair-a")
			pairFile := configengine.ConfigFile(pair, "batcher")
			before, err := os.ReadFile(pairFile)
			if err != nil {
				t.Fatalf("read pair batcher.yaml: %v", err)
			}
			clear := tc.mark(t, h)

			if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
				t.Fatalf("Ensure: %v", err)
			}

			after, err := os.ReadFile(pairFile)
			if err != nil || string(after) != string(before) {
				t.Errorf("mid-merge pair file changed (err %v)", err)
			}
			if retiredKeyPresent(t, h.PrimeWorktree()) {
				t.Errorf("the prime was not reconciled while the pair was mid-merge")
			}
			if _, found := stampKey(t, geom); found {
				t.Errorf("stamp written while a worktree was skipped")
			}

			clear()
			if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
				t.Fatalf("Ensure after the merge state cleared: %v", err)
			}
			if retiredKeyPresent(t, pair) {
				t.Errorf("the pair was not reconciled once its merge state cleared")
			}
			if _, found := stampKey(t, geom); !found {
				t.Errorf("stamp absent after the retry")
			}
		})
	}
}

func TestEnsure_MergeBeginningBeforeCommitRestoresTheFiles(t *testing.T) {
	t.Parallel()

	h := newStaleHub(t, "pair-a")
	geom := geometryOf(h)
	pair := h.PairCodeWorktree("pair-a")
	pairFile := configengine.ConfigFile(pair, "batcher")
	before, err := os.ReadFile(pairFile)
	if err != nil {
		t.Fatalf("read pair batcher.yaml: %v", err)
	}
	hook := func(worktreePath string) {
		if worktreePath == pair {
			markMergeRecord(t, h.PairRecordsSibling("pair-a"))
		}
	}

	if err := hubreconcile.EnsureWithHooks(geom, hubreconcile.Options{}, nil, hook); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	after, err := os.ReadFile(pairFile)
	if err != nil || string(after) != string(before) {
		t.Errorf("hooked pair file not restored to its prior bytes (err %v)", err)
	}
	if status := gitkit.GitStatusPorcelain(t, h.PairRecordsSibling("pair-a")); status != "" {
		t.Errorf("hooked pair has uncommitted changes:\n%s", status)
	}
	if _, found := stampKey(t, geom); found {
		t.Errorf("stamp written although a worktree was skipped")
	}
}

func TestEnsure_UnparseablePairConfigNamesPairAndFile(t *testing.T) {
	t.Parallel()

	h := newStaleHub(t, "pair-a")
	geom := geometryOf(h)
	pair := h.PairCodeWorktree("pair-a")
	gitkit.CommitFile(t, h.PairRecordsSibling("pair-a"), configengine.ConfigFileRel("loom"), "a: [unclosed\n", "fixture: broken loom.yaml")

	err := hubreconcile.Ensure(geom, hubreconcile.Options{})

	var worktreeErr *hubreconcile.WorktreeError
	if !errors.As(err, &worktreeErr) {
		t.Fatalf("Ensure error = %v; want *WorktreeError", err)
	}
	if worktreeErr.Worktree != pair || worktreeErr.File != configengine.ConfigFile(pair, "loom") {
		t.Errorf("WorktreeError names (%q, %q); want the pair and its loom.yaml", worktreeErr.Worktree, worktreeErr.File)
	}
	if !strings.Contains(err.Error(), "lyx config loom") {
		t.Errorf("message %q does not name lyx config loom", err.Error())
	}
	if _, found := stampKey(t, geom); found {
		t.Errorf("stamp written after a failed walk")
	}

	module, _ := configreg.Lookup("loom")
	gitkit.CommitFile(t, h.PairRecordsSibling("pair-a"), configengine.ConfigFileRel("loom"), module.Template(), "fixture: fixed loom.yaml")
	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure after the fix: %v", err)
	}
	if _, found := stampKey(t, geom); !found {
		t.Errorf("stamp absent after the fixed walk")
	}
}

func TestEnsure_HeldLockTimesOutWithoutStamping(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	geom := geometryOf(h)
	if err := os.MkdirAll(filepath.Dir(geom.LockPath()), 0o755); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	held, err := lock.AcquireWriteLock(geom.LockPath())
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer func() { _ = held.Release() }()

	err = hubreconcile.Ensure(geom, hubreconcile.Options{LockWait: 50 * time.Millisecond})

	var timeoutErr *hubreconcile.LockTimeoutError
	if !errors.As(err, &timeoutErr) || timeoutErr.Path != geom.LockPath() {
		t.Fatalf("Ensure error = %v; want *LockTimeoutError for %s", err, geom.LockPath())
	}
	if _, found := stampKey(t, geom); found {
		t.Errorf("stamp written without the lock")
	}
}

func TestEnsure_ConcurrentCallsLandOneCommitPerBranch(t *testing.T) {
	t.Parallel()

	h := newStaleHub(t, "pair-a")
	geom := geometryOf(h)
	primeBefore, pairBefore := commitCount(t, h.PrimeRecords()), commitCount(t, h.PairRecordsSibling("pair-a"))

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
				t.Errorf("Ensure: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := commitCount(t, h.PrimeRecords()); got != primeBefore+1 {
		t.Errorf("prime records commits %d -> %d; want exactly one", primeBefore, got)
	}
	if got := commitCount(t, h.PairRecordsSibling("pair-a")); got != pairBefore+1 {
		t.Errorf("pair records commits %d -> %d; want exactly one", pairBefore, got)
	}
}

func TestEnsure_PairRegisteredDuringTheWalkIsReconciledBeforeTheStamp(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	geom := geometryOf(h)
	listings := 0
	hook := func() {
		listings++
		if listings != 1 {
			return
		}
		hubforge.AddPair(t, h, "late")
		seedRetiredKey(t, h.PairRecordsSibling("late"))
	}

	if err := hubreconcile.EnsureWithHooks(geom, hubreconcile.Options{}, hook, nil); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if retiredKeyPresent(t, h.PairCodeWorktree("late")) {
		t.Errorf("the late pair was not reconciled")
	}
	assertClean(t, h.PairRecordsSibling("late"))
	if _, found := stampKey(t, geom); !found {
		t.Errorf("stamp absent after the walk")
	}
}

func TestEnsure_PairDirectoryDeletedIsSkippedAsRemoved(t *testing.T) {
	t.Parallel()

	h := newStaleHub(t, "pair-a")
	geom := geometryOf(h)
	if err := os.RemoveAll(h.PairCodeWorktree("pair-a")); err != nil {
		t.Fatalf("remove pair: %v", err)
	}

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if _, found := stampKey(t, geom); !found {
		t.Errorf("stamp absent although the removed pair is not a failure")
	}
}

func TestEnsure_UnwiredPairIsLeftUnwrittenAndStampIsWritten(t *testing.T) {
	t.Parallel()

	h := newStaleHub(t, "pair-a")
	geom := geometryOf(h)
	junction := filepath.Join(h.PairCodeWorktree("pair-a"), "_lyx")
	if err := os.Remove(junction); err != nil {
		t.Fatalf("remove _lyx junction: %v", err)
	}
	before := commitCount(t, h.PairRecordsSibling("pair-a"))

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if _, err := os.Lstat(junction); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a path now exists at the unwired junction's place (Lstat err %v); Add's wiring would refuse", err)
	}
	if got := commitCount(t, h.PairRecordsSibling("pair-a")); got != before {
		t.Errorf("unwired pair records commits %d -> %d; want none", before, got)
	}
	if _, found := stampKey(t, geom); !found {
		t.Errorf("stamp absent although an incomplete pair is stamp-neutral")
	}
}

func TestEnsure_UnreachableBoardRemoteStillCommitsHubWideConfig(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	geom := geometryOf(h)
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\nretired_fixture_key: 1\n")
	gitkit.Git(t, h.BoardDir(), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	before := commitCount(t, h.BoardDir())

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if got := commitCount(t, h.BoardDir()); got != before+1 {
		t.Errorf("board commits %d -> %d; want the hub-wide change committed", before, got)
	}
}

func TestEnsure_PairCallReconcilesDespiteAFreshStamp(t *testing.T) {
	t.Parallel()

	h := newStaleHub(t, "pair-a")
	geom := geometryOf(h)
	writeCurrentStamp(t, geom)
	pair := filepath.Join(h.Path, "pair-a")

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{Pair: pair}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if retiredKeyPresent(t, pair) {
		t.Errorf("the pair still carries %s", retiredKey)
	}
	assertClean(t, h.PairRecordsSibling("pair-a"))
	if !retiredKeyPresent(t, h.PrimeWorktree()) {
		t.Errorf("the pair call reconciled the prime as well")
	}
}
