//go:build integration

// ensure_integration_test.go covers Ensure against real hubforge hubs: the stale-build walk, the stamp gate, the mid-merge and incomplete-pair skips, the lock, concurrent callers, a pair registered mid-walk and the single-pair call.

package hubreconcile_test

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
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

// differingBatcherBudget is a batcher budget that differs from the repo default's, so a batcher.yaml holding it survives the strip.
const differingBatcherBudget = "budget: 450001"

// seedRetiredKey commits a batcher.yaml in the records worktree at recordsRoot: the template with the retired key in place of orientation, plus one key whose value differs from the repo default.
// It survives the sparse strip non-empty, which keeps retiredKeyPresent's read working after the walk.
func seedRetiredKey(t *testing.T, recordsRoot string) {
	t.Helper()

	module, _ := configreg.Lookup("batcher")
	content := strings.Replace(module.Template(), "orientation: 31400", retiredKey+": 52000", 1)
	content = strings.Replace(content, "budget: 450000", differingBatcherBudget, 1)
	if !strings.Contains(content, retiredKey) || !strings.Contains(content, differingBatcherBudget) {
		t.Fatalf("batcher template no longer carries the orientation and budget lines the fixture rewrites")
	}
	gitkit.CommitFile(t, recordsRoot, configengine.ConfigFileRel("batcher"), content, "fixture: retired key")
}

// differingLoomBounces is a loom review budget that differs from the repo default's, so a loom.yaml holding it survives the strip.
const differingLoomBounces = "review_max_bounces: 99"

// commitPreMigrationFiles commits, in the records worktree at recordsRoot, a full pre-migration per-worktree file set -- each per-worktree module's template, loom's with one key differing from the repo default -- and deletes the sparse marker, so the walk finds an unmigrated worktree with something to strip.
func commitPreMigrationFiles(t *testing.T, recordsRoot string) {
	t.Helper()

	for _, module := range configreg.Modules() {
		if module.HubWide {
			continue
		}
		content := module.Template()
		if module.Name == "loom" {
			content = strings.Replace(content, "review_max_bounces: 3", differingLoomBounces, 1)
			if !strings.Contains(content, differingLoomBounces) {
				t.Fatalf("loom template no longer carries the review_max_bounces line the fixture rewrites")
			}
		}
		path := filepath.Join(recordsRoot, configengine.ConfigFileRel(module.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	gitkit.Git(t, recordsRoot, "rm", "-q", filepath.ToSlash(configengine.SparseMarkerRel()))
	gitkit.Git(t, recordsRoot, "add", "-A")
	gitkit.Git(t, recordsRoot, "commit", "-q", "-m", "fixture: pre-migration per-worktree files")
}

// markerTracked reports whether the records worktree at recordsRoot tracks the sparse marker.
func markerTracked(t *testing.T, recordsRoot string) bool {
	t.Helper()
	return slices.Contains(gitkit.LsFiles(t, recordsRoot), filepath.ToSlash(configengine.SparseMarkerRel()))
}

// configFilesOf returns every file under codeRoot's config dir, relative path to bytes.
func configFilesOf(t *testing.T, codeRoot string) map[string]string {
	t.Helper()

	dir := configengine.ConfigDir(codeRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	files := map[string]string{}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = string(data)
	}
	return files
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

// newStaleHub builds a hub with a pair, both unmarked and carrying a full pre-migration file set with the retired key committed on their records side, and no stamp.
func newStaleHub(t *testing.T, slug string) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, slug)
	commitPreMigrationFiles(t, h.PrimeRecords())
	commitPreMigrationFiles(t, h.PairRecordsSibling(slug))
	seedRetiredKey(t, h.PrimeRecords())
	seedRetiredKey(t, h.PairRecordsSibling(slug))
	return h
}

// staleHubWideConfig is a fabric config carrying a key the registry no longer knows, so the hub-wide reconcile rewrites it.
const staleHubWideConfig = "branch_prefix: \"\"\nretired_fixture_key: 1\n"

// pushBoardFromSecondClone pushes the board's current branch to its origin, then lands a commit on that origin from a second clone, and returns the board's branch name.
// The board is left behind its upstream by exactly that commit.
func pushBoardFromSecondClone(t *testing.T, board string) string {
	t.Helper()

	branch := gitkit.CurrentBranch(t, board)
	origin := gitkit.Git(t, board, "remote", "get-url", "origin")
	gitkit.Git(t, board, "push", "-u", "origin", branch)
	clone := filepath.Join(t.TempDir(), "second")
	gitkit.Git(t, filepath.Dir(clone), "clone", "--branch", branch, origin, clone)
	gitkit.CommitFile(t, clone, "moved-upstream.txt", "moved\n", "fixture: upstream moves")
	gitkit.Git(t, clone, "push", "origin", branch)
	return branch
}

// It does not call t.Parallel: it swaps the package-global detached board sync.
func TestEnsure_StaleBuildReconcilesAndCommitsEveryWorktree(t *testing.T) {
	for _, tc := range []struct {
		name            string
		boardBehind     bool
		loomDefaultGone bool
	}{
		{name: "board up to date"},
		{name: "board behind its upstream", boardBehind: true},
		{name: "loom default deleted beside a current stamp", loomDefaultGone: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runStaleBuildReconcile(t, tc.boardBehind, tc.loomDefaultGone)
		})
	}
}

// recordBoardSyncs replaces the detached board sync with a recorder of the board directories it was asked to sync, and restores it when the test ends.
func recordBoardSyncs(t *testing.T) *[]string {
	t.Helper()

	var spawned []string
	t.Cleanup(hubreconcile.SetSpawnBoardSyncForTest(func(boardPath string) error {
		spawned = append(spawned, boardPath)
		return nil
	}))
	return &spawned
}

// runStaleBuildReconcile walks a stale hub and asserts every worktree and the board got one reconcile commit, with the board's pulled from upstream when boardBehind.
// With loomDefaultGone the stamp names the running build but loom's committed repo default is deleted, and the walk reseeds and commits it on the board; batcher's default stays intact so retiredKeyPresent keeps reading it.
func runStaleBuildReconcile(t *testing.T, boardBehind, loomDefaultGone bool) {
	t.Helper()

	h := newStaleHub(t, "pair-a")
	geom := geometryOf(h)
	pair := h.PairCodeWorktree("pair-a")
	hubforge.SeedFabricConfig(t, h, staleHubWideConfig)
	loomDefault := configengine.ConfigFileRel("loom")
	if loomDefaultGone {
		gitkit.Git(t, h.BoardDir(), "rm", "-q", loomDefault)
		gitkit.Git(t, h.BoardDir(), "commit", "-q", "-m", "fixture: delete loom default")
		writeCurrentStamp(t, geom)
	}
	var movedUpstream string
	if boardBehind {
		pushBoardFromSecondClone(t, h.BoardDir())
		movedUpstream = "moved-upstream.txt"
	}
	primeBefore, pairBefore := commitCount(t, h.PrimeRecords()), commitCount(t, h.PairRecordsSibling("pair-a"))
	spawned := recordBoardSyncs(t)
	boardBranch := gitkit.CurrentBranch(t, h.BoardDir())
	originBoardBefore := gitkit.Git(t, gitkit.Git(t, h.BoardDir(), "remote", "get-url", "origin"), "for-each-ref", "refs/heads/"+boardBranch)

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if want := []string{h.BoardDir()}; !slices.Equal(*spawned, want) {
		t.Errorf("detached board syncs = %v; want one for the board %v after the hub-wide commit", *spawned, want)
	}
	if got := gitkit.Git(t, gitkit.Git(t, h.BoardDir(), "remote", "get-url", "origin"), "for-each-ref", "refs/heads/"+boardBranch); got != originBoardBefore {
		t.Errorf("origin board branch %q -> %q; want no synchronous push", originBoardBefore, got)
	}

	wantSubject := "lyx: reconcile hub-wide config for build " + fabricengine.BinaryLabel()
	if got := gitkit.Git(t, h.BoardDir(), "log", "-1", "--format=%s"); got != wantSubject {
		t.Errorf("hub-wide commit subject = %q; want %q", got, wantSubject)
	}
	if movedUpstream != "" {
		if _, err := os.Stat(filepath.Join(h.BoardDir(), movedUpstream)); err != nil {
			t.Errorf("the pulled upstream file %s is missing: %v", movedUpstream, err)
		}
		if got := gitkit.Git(t, h.BoardDir(), "log", "-2", "--format=%s"); !strings.Contains(got, "fixture: upstream moves") {
			t.Errorf("hub-wide commit is not on top of the pulled upstream commit; log:\n%s", got)
		}
	}

	if loomDefaultGone && !slices.Contains(gitkit.LsFiles(t, h.BoardDir()), loomDefault) {
		t.Errorf("the board does not track the reseeded %s", loomDefault)
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

	// Each worktree is stripped and committed with its marker: loom.yaml keeps only its differing key, or is deleted when the reseeded default equals it.
	for name, records := range map[string]string{"prime": h.PrimeRecords(), "pair": h.PairRecordsSibling("pair-a")} {
		if !markerTracked(t, records) {
			t.Errorf("%s does not track the sparse marker after the walk", name)
		}
		loomFile := configengine.ConfigFile(records, "loom")
		data, err := os.ReadFile(loomFile)
		switch {
		case loomDefaultGone:
			if !os.IsNotExist(err) {
				t.Errorf("%s loom.yaml = %q (err %v); want it deleted, since the reseeded default equals it", name, data, err)
			}
		case err != nil || !strings.HasPrefix(string(data), differingLoomBounces) || strings.Count(string(data), "\n") != 1:
			t.Errorf("%s loom.yaml = %q (err %v); want only the %q line", name, data, err, differingLoomBounces)
		}
	}

	// A second walk strips nothing: it lands no new commit and leaves every per-worktree file and marker byte-identical.
	boardAfter := commitCount(t, h.BoardDir())
	filesBefore := map[string]map[string]string{"prime": configFilesOf(t, h.PrimeWorktree()), "pair": configFilesOf(t, pair)}
	if err := os.Remove(geom.StampPath()); err != nil {
		t.Fatalf("remove the stamp: %v", err)
	}
	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if got := commitCount(t, h.PrimeRecords()); got != primeAfter {
		t.Errorf("second walk landed a prime commit: %d -> %d", primeAfter, got)
	}
	if got := commitCount(t, h.PairRecordsSibling("pair-a")); got != pairAfter {
		t.Errorf("second walk landed a pair commit: %d -> %d", pairAfter, got)
	}
	if got := commitCount(t, h.BoardDir()); got != boardAfter {
		t.Errorf("second walk landed a board commit: %d -> %d", boardAfter, got)
	}
	filesAfter := map[string]map[string]string{"prime": configFilesOf(t, h.PrimeWorktree()), "pair": configFilesOf(t, pair)}
	for name, before := range filesBefore {
		if !maps.Equal(before, filesAfter[name]) {
			t.Errorf("second walk changed the %s config files:\nbefore: %v\nafter: %v", name, before, filesAfter[name])
		}
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

			if markerTracked(t, h.PairRecordsSibling("pair-a")) {
				t.Errorf("the skipped pair was marked sparse while mid-merge")
			}

			clear()
			if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
				t.Fatalf("Ensure after the merge state cleared: %v", err)
			}
			if retiredKeyPresent(t, pair) {
				t.Errorf("the pair was not reconciled once its merge state cleared")
			}
			if !markerTracked(t, h.PairRecordsSibling("pair-a")) {
				t.Errorf("the retry did not strip and mark the skipped pair")
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
	var clear func()
	hook := func(worktreePath string) {
		if worktreePath == pair {
			clear = markMergeRecord(t, h.PairRecordsSibling("pair-a"))
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
	if _, err := os.Stat(configengine.SparseMarker(pair)); !os.IsNotExist(err) {
		t.Errorf("the hooked pair keeps a sparse marker after the restore (stat err = %v)", err)
	}

	clear()
	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure after the merge state cleared: %v", err)
	}
	if !markerTracked(t, h.PairRecordsSibling("pair-a")) {
		t.Errorf("the next walk did not strip and mark the pair")
	}
	assertClean(t, h.PairRecordsSibling("pair-a"))
}

// commitTemplate commits module's template as its config file in the records worktree at recordsRoot.
func commitTemplate(t *testing.T, recordsRoot, module string) {
	t.Helper()

	m, _ := configreg.Lookup(module)
	gitkit.CommitFile(t, recordsRoot, configengine.ConfigFileRel(module), m.Template(), "fixture: fixed "+module+".yaml")
}

func TestEnsure_FailedWalkNamesTheFileAndLeavesTheTreeAsItWas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// breakConfig breaks one config file of h and returns the worktree the walk fails in, the repo holding the file, and the file.
		breakConfig func(t *testing.T, h *hubforge.Hub) (worktree, repo, file string)
		// module is the module whose "lyx config" the message names; empty when the failure names no module.
		module string
		repair func(t *testing.T, h *hubforge.Hub)
		// afterRepair, when set, checks the pair after the repaired walk.
		afterRepair func(t *testing.T, h *hubforge.Hub)
	}{
		{
			name: "a declared key move is undone with the failed walk and committed by the repaired one",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				records := h.PairRecordsSibling("pair-a")
				// The stale pair holds every module's template; without its claude.yaml the move has a file to create.
				gitkit.Git(t, records, "rm", "-q", filepath.ToSlash(configengine.ConfigFileRel("claude")))
				gitkit.Git(t, records, "commit", "-q", "-m", "fixture: pair without claude.yaml")
				gitkit.CommitFile(t, records, configengine.ConfigFileRel("shuttle"), "claude: /x\n", "fixture: shuttle.yaml with a movable key")
				gitkit.CommitFile(t, records, configengine.ConfigFileRel("loom"), "a: [unclosed\n", "fixture: broken loom.yaml")
				pair := h.PairCodeWorktree("pair-a")
				return pair, records, configengine.ConfigFile(pair, "loom")
			},
			module: "loom",
			repair: func(t *testing.T, h *hubforge.Hub) { commitTemplate(t, h.PairRecordsSibling("pair-a"), "loom") },
			afterRepair: func(t *testing.T, h *hubforge.Hub) {
				content, err := os.ReadFile(configengine.ConfigFile(h.PairCodeWorktree("pair-a"), "claude"))
				if err != nil || !strings.Contains(string(content), "binary: /x") {
					t.Errorf("claude.yaml = %q, %v; want the moved binary after the repaired walk", content, err)
				}
			},
		},
		{
			name: "unparseable pair config",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				gitkit.CommitFile(t, h.PairRecordsSibling("pair-a"), configengine.ConfigFileRel("loom"), "a: [unclosed\n", "fixture: broken loom.yaml")
				pair := h.PairCodeWorktree("pair-a")
				return pair, h.PairRecordsSibling("pair-a"), configengine.ConfigFile(pair, "loom")
			},
			module: "loom",
			repair: func(t *testing.T, h *hubforge.Hub) { commitTemplate(t, h.PairRecordsSibling("pair-a"), "loom") },
		},
		{
			name: "unreadable pair config",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				pair := h.PairCodeWorktree("pair-a")
				path := configengine.ConfigFile(pair, "loom")
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("remove loom.yaml: %v", err)
				}
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("mkdir at loom.yaml: %v", err)
				}
				return pair, h.PairRecordsSibling("pair-a"), path
			},
			repair: func(t *testing.T, h *hubforge.Hub) {
				if err := os.Remove(configengine.ConfigFile(h.PairCodeWorktree("pair-a"), "loom")); err != nil {
					t.Fatalf("remove the directory at loom.yaml: %v", err)
				}
				commitTemplate(t, h.PairRecordsSibling("pair-a"), "loom")
			},
		},
		{
			name: "unparseable hub-wide config",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				// A stale board.yaml, which precedes fabric.yaml in the registry, is written before the fabric failure.
				board, _ := configreg.Lookup("board")
				gitkit.CommitFile(t, h.BoardDir(), configengine.ConfigFileRel("board"), board.Template()+"retired_fixture_key: 1\n", "fixture: stale board.yaml")
				hubforge.SeedFabricConfig(t, h, "a: [unclosed\n")
				return h.BoardDir(), h.BoardDir(), configengine.ConfigFile(h.BoardDir(), "fabric")
			},
			module: "fabric",
			repair: func(t *testing.T, h *hubforge.Hub) {
				m, _ := configreg.Lookup("fabric")
				hubforge.SeedFabricConfig(t, h, m.Template())
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newStaleHub(t, "pair-a")
			geom := geometryOf(h)
			worktree, repo, file := tc.breakConfig(t, h)
			statusBefore := gitkit.GitStatusPorcelain(t, repo)

			err := hubreconcile.Ensure(geom, hubreconcile.Options{})

			var worktreeErr *hubreconcile.WorktreeError
			if !errors.As(err, &worktreeErr) {
				t.Fatalf("Ensure error = %v; want *WorktreeError", err)
			}
			if worktreeErr.Worktree != worktree {
				t.Errorf("WorktreeError names worktree %q; want %q", worktreeErr.Worktree, worktree)
			}
			if !strings.Contains(err.Error(), file) {
				t.Errorf("message %q does not name %s", err.Error(), file)
			}
			if tc.module != "" {
				if worktreeErr.File != file {
					t.Errorf("WorktreeError.File = %q; want %q", worktreeErr.File, file)
				}
				if !strings.Contains(err.Error(), "lyx config "+tc.module) {
					t.Errorf("message %q does not name lyx config %s", err.Error(), tc.module)
				}
			}
			if _, statErr := os.Stat(file); statErr != nil {
				t.Errorf("the failed file is gone after the walk: %v", statErr)
			}
			if status := gitkit.GitStatusPorcelain(t, repo); status != statusBefore {
				t.Errorf("%s status changed by the failed walk:\nbefore:\n%s\nafter:\n%s", repo, statusBefore, status)
			}
			if _, found := stampKey(t, geom); found {
				t.Errorf("stamp written after a failed walk")
			}

			tc.repair(t, h)
			if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
				t.Fatalf("Ensure after the repair: %v", err)
			}
			if _, found := stampKey(t, geom); !found {
				t.Errorf("stamp absent after the repaired walk")
			}
			assertClean(t, repo)
			if retiredKeyPresent(t, h.PairCodeWorktree("pair-a")) {
				t.Errorf("retired key still present in the pair after the repaired walk")
			}
			if tc.afterRepair != nil {
				tc.afterRepair(t, h)
			}
		})
	}
}

func TestEnsure_HeldLockTimesOutWithoutStamping(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
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

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
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

// The logger's output is process-global, so this test does not call t.Parallel.
func TestEnsure_UnreachableBoardRemoteSkipsHubWideConfigAndLeavesStampAbsent(t *testing.T) {
	logs := logcapture.Capture(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	geom := geometryOf(h)
	hubforge.SeedFabricConfig(t, h, staleHubWideConfig)
	gitkit.Git(t, h.BoardDir(), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	before := commitCount(t, h.BoardDir())

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if got := commitCount(t, h.BoardDir()); got != before {
		t.Errorf("board commits %d -> %d; want nothing written while the remote is unreachable", before, got)
	}
	assertClean(t, h.BoardDir())
	if !strings.Contains(logs.String(), "fetch_failed") {
		t.Errorf("log lacks the fetch-failure Warn:\n%s", logs.String())
	}
	if _, found := stampKey(t, geom); found {
		t.Errorf("stamp written although the hub-wide config was skipped")
	}
}

// The detached board sync is package-global, so this test does not call t.Parallel.
func TestEnsure_BoardWithNoRemoteCommitsHubWideConfigAndWritesStamp(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	geom := geometryOf(h)
	hubforge.SeedFabricConfig(t, h, staleHubWideConfig)
	gitkit.Git(t, h.BoardDir(), "remote", "remove", "origin")
	before := commitCount(t, h.BoardDir())
	spawned := recordBoardSyncs(t)

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if got := commitCount(t, h.BoardDir()); got != before+1 {
		t.Errorf("board commits %d -> %d; want the hub-wide commit landed locally", before, got)
	}
	assertClean(t, h.BoardDir())
	if want := []string{h.BoardDir()}; !slices.Equal(*spawned, want) {
		t.Errorf("detached board syncs = %v; want %v", *spawned, want)
	}
	if key, found := stampKey(t, geom); !found || key != runningKey() {
		t.Errorf("stamp = %q (found %v); want the running key, since the walk never waits on the board's push", key, found)
	}
}

// unpushedHubWide is a hub whose board holds a hub-wide config commit origin has not taken, since the detached board sync is recorded and never run.
// origin is the records remote, hubWidePath the config file the commit changed and before the board's commit count ahead of that commit.
// The build stamp is absent, so the next Ensure walks the hub again.
type unpushedHubWide struct {
	hub         *hubforge.Hub
	geom        hubreconcile.Geometry
	board       string
	branch      string
	origin      string
	hubWidePath string
	before      int
}

// arrangeUnpushedHubWide builds an unpushedHubWide, after two Ensure runs, each of which stamps the build and asks for the board's detached sync.
// The rerun, with nothing new to commit, still asks for it, so an earlier unpushed commit is pushed too; the stamp is removed after each run.
func arrangeUnpushedHubWide(t *testing.T) *unpushedHubWide {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	geom := geometryOf(h)
	board := h.BoardDir()
	hubforge.SeedFabricConfig(t, h, staleHubWideConfig)
	branch := gitkit.CurrentBranch(t, board)
	gitkit.Git(t, board, "push", "-u", "origin", branch)
	origin := gitkit.Git(t, board, "remote", "get-url", "origin")
	before := commitCount(t, board)
	spawned := recordBoardSyncs(t)

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if got := commitCount(t, board); got != before+1 {
		t.Fatalf("board commits %d -> %d; want the hub-wide commit landed locally", before, got)
	}
	if key, found := stampKey(t, geom); !found || key != runningKey() {
		t.Fatalf("stamp = %q (found %v); want the running key although the hub-wide commit was not pushed", key, found)
	}
	changed := strings.Fields(gitkit.Git(t, board, "show", "--name-only", "--format=", "HEAD"))
	if len(changed) != 1 {
		t.Fatalf("the hub-wide commit changed %v; want one config file", changed)
	}
	if err := os.Remove(geom.StampPath()); err != nil {
		t.Fatalf("remove the stamp: %v", err)
	}

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("rerun Ensure: %v", err)
	}
	if got := commitCount(t, board); got != before+1 {
		t.Fatalf("rerun changed the board commits: %d -> %d; want no new commit", before+1, got)
	}
	if want := []string{board, board}; !slices.Equal(*spawned, want) {
		t.Fatalf("detached board syncs = %v; want one per run, the rerun's carrying the earlier commit", *spawned)
	}
	if err := os.Remove(geom.StampPath()); err != nil {
		t.Fatalf("remove the stamp: %v", err)
	}

	return &unpushedHubWide{hub: h, geom: geom, board: board, branch: branch, origin: origin, hubWidePath: changed[0], before: before}
}

// moveUpstream commits rel on origin's board branch from a second clone.
func (u *unpushedHubWide) moveUpstream(t *testing.T, rel, content string) {
	t.Helper()

	clone := filepath.Join(t.TempDir(), "second")
	gitkit.Git(t, filepath.Dir(clone), "clone", "--branch", u.branch, u.origin, clone)
	gitkit.CommitFile(t, clone, rel, content, "fixture: upstream moves")
	gitkit.Git(t, clone, "push", "origin", u.branch)
}

// The logger's output is process-global, so this test does not call t.Parallel.
func TestEnsure_UnpushedHubWideCommitOverMovedUpstreamLeavesStampAbsent(t *testing.T) {
	boardkit.InstallReplay(t)

	t.Run("an upstream moved on other paths is replayed under the commit and the stamp is written", func(t *testing.T) {
		u := arrangeUnpushedHubWide(t)
		u.moveUpstream(t, "moved-upstream.txt", "moved\n")

		if err := hubreconcile.Ensure(u.geom, hubreconcile.Options{}); err != nil {
			t.Fatalf("second Ensure: %v", err)
		}

		if got := commitCount(t, u.board); got != u.before+2 {
			t.Errorf("second Ensure left %d board commits, want the replayed hub-wide commit over the moved upstream (%d)", got, u.before+2)
		}
		if key, found := stampKey(t, u.geom); !found || key != runningKey() {
			t.Errorf("stamp = %q (found %v); want the running key once the hub-wide commit is replayed", key, found)
		}
	})

	t.Run("an upstream that changed the same config file skips with the stamp absent", func(t *testing.T) {
		logs := logcapture.Capture(t)
		u := arrangeUnpushedHubWide(t)
		u.moveUpstream(t, u.hubWidePath, "branch_prefix: \"upstream\"\n")

		if err := hubreconcile.Ensure(u.geom, hubreconcile.Options{}); err != nil {
			t.Fatalf("second Ensure: %v", err)
		}

		if got := commitCount(t, u.board); got != u.before+1 {
			t.Errorf("second Ensure changed the board commits: %d -> %d; want no new commit", u.before+1, got)
		}
		for _, want := range []string{u.hubWidePath, "lyx board sync"} {
			if !strings.Contains(logs.String(), want) {
				t.Errorf("log lacks the divergence Warn naming %q:\n%s", want, logs.String())
			}
		}
		if _, found := stampKey(t, u.geom); found {
			t.Errorf("stamp written although the hub-wide commit is still unpushed")
		}
	})
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
