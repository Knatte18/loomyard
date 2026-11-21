// ensure_integration_test.go covers Ensure against real hubforge hubs.
// It tests the stale-build walk, the stamp gate, and the skips for mid-merge and incomplete pairs.
// It also tests the lock, concurrent callers, a pair registered mid-walk, and the single-pair call.

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
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

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

// writeCurrentStamp marks the hub as reconciled by the test binary's build.
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

// configFilesOf returns the content of every file in codeRoot's config dir, keyed by file name.
func configFilesOf(t *testing.T, codeRoot string) map[string]string {
	t.Helper()

	dir := configengine.ConfigDir(codeRoot)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}
	}
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

// markMergeRecord writes a fabric merge record into the records worktree's gitdir and returns the func that clears it.
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

// staleHub is a hub with the pair pair-a and no build stamp.
var staleHub = &hubforge.Registration{
	Key:  "hubreconcile-stale",
	Base: hubforge.Shape{Anchor: ".", Pairs: []string{"pair-a"}},
}

// newStaleHub returns the test's own copy of the staleHub template.
func newStaleHub(t *testing.T) *hubforge.Hub {
	t.Helper()
	return hubforge.CopyRegistered(t, staleHub)
}

// newerHubWideKey is a fabric key the running template lacks, as a newer binary writes.
const newerHubWideKey = "newer_fixture_key: 1"

// newerWorktreeKey is a logger key the running template lacks.
// The test plants it in the pair's worktree file.
const newerWorktreeKey = "newer_worktree_key: 1"

// loomDefaultRel is the board-relative path of loom's repo default.
var loomDefaultRel = configengine.ConfigFileRel("loom")

// deleteBoardLoomDefault commits on the board the deletion of loom's repo default,
// so the hub-wide reconcile reseeds and commits it.
func deleteBoardLoomDefault(t *testing.T, h *hubforge.Hub) {
	t.Helper()

	gitkit.Git(t, h.BoardDir(), "rm", "-q", "--", filepath.ToSlash(loomDefaultRel))
	gitkit.Git(t, h.BoardDir(), "commit", "-q", "-m", "fixture: delete loom default")
}

// pushBoardFromSecondClone pushes the board's current branch to its origin.
// It then lands a commit on that origin from a second clone and returns the board's branch name.
// The board is left one commit behind its upstream.
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

// TestEnsure_StaleBuildReconcilesTheBoardAndKeepsNewerKeys does not call t.Parallel:
// it swaps the package-global detached board sync.
func TestEnsure_StaleBuildReconcilesTheBoardAndKeepsNewerKeys(t *testing.T) {
	tiergate.Require(t, 1)
	for _, tc := range []struct {
		name string
		run  staleRun
	}{
		{name: "board up to date"},
		{name: "board behind its upstream", run: staleRun{boardBehind: true}},
		{name: "loom default deleted beside a current stamp", run: staleRun{loomDefaultGone: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runStaleBuildReconcile(t, tc.run)
		})
	}
}

// staleRun sets the hub state a stale-build walk starts from.
type staleRun struct {
	// boardBehind leaves the board behind its upstream by one commit.
	boardBehind bool
	// loomDefaultGone deletes loom's committed repo default, then writes a stamp that names the running build.
	loomDefaultGone bool
}

// recordBoardSyncs replaces the detached board sync with a recorder of the board directories it was asked to sync.
// The original sync is restored when the test ends.
func recordBoardSyncs(t *testing.T) *[]string {
	t.Helper()

	var spawned []string
	t.Cleanup(hubreconcile.SetSpawnBoardSyncForTest(func(boardPath string) error {
		spawned = append(spawned, boardPath)
		return nil
	}))
	return &spawned
}

// runStaleBuildReconcile walks a stale hub and asserts that the board gets one reconcile commit.
// It also asserts that no worktree is committed.
// A board behind its upstream is pulled from upstream.
// The walk keeps a key the template lacks in the board's fabric.yaml and in the pair's logger.yaml.
// A deleted repo default is reseeded sparse and committed on the board.
func runStaleBuildReconcile(t *testing.T, run staleRun) {
	t.Helper()

	h := newStaleHub(t)
	geom := geometryOf(h)
	pair := h.PairCodeWorktree("pair-a")
	hubforge.SeedFabricConfig(t, h, newerHubWideKey+"\n")
	pairRecords := h.PairRecordsSibling("pair-a")
	gitkit.CommitFile(t, pairRecords, configengine.ConfigFileRel("logger"), newerWorktreeKey+"\n", "fixture: a key the template lacks")
	lensesDefault := configengine.ConfigFileRel("lenses")
	gitkit.Git(t, h.BoardDir(), "rm", "-q", lensesDefault)
	gitkit.Git(t, h.BoardDir(), "commit", "-q", "-m", "fixture: delete lenses default")
	// A hub with no lenses.yaml anywhere has no prime copy to seed the board's default from.
	for _, records := range []string{h.PrimeRecords(), pairRecords} {
		gitkit.Git(t, records, "rm", "-q", "--ignore-unmatch", "--", lensesDefault)
		gitkit.Git(t, records, "commit", "-q", "--allow-empty", "-m", "fixture: delete lenses file")
	}
	if run.loomDefaultGone {
		deleteBoardLoomDefault(t, h)
		writeCurrentStamp(t, geom)
	}
	var movedUpstream string
	if run.boardBehind {
		pushBoardFromSecondClone(t, h.BoardDir())
		movedUpstream = "moved-upstream.txt"
	}
	// The machine file sits under the board dir's never-tracked .lym, which no reconcile reads or writes.
	const machineBytes = "slots: 1\ntier_parallel:\n  tier0: 8\n"
	machineFile := configengine.MachineFile(h.BoardDir(), "gate")
	if err := os.MkdirAll(filepath.Dir(machineFile), 0o755); err != nil {
		t.Fatalf("create the board's .lym config dir: %v", err)
	}
	if err := os.WriteFile(machineFile, []byte(machineBytes), 0o644); err != nil {
		t.Fatalf("seed the machine file: %v", err)
	}
	primeBefore, pairBefore := commitCount(t, h.PrimeRecords()), commitCount(t, pairRecords)
	spawned := recordBoardSyncs(t)
	boardBranch := gitkit.CurrentBranch(t, h.BoardDir())
	originBoardBefore := gitkit.Git(t, gitkit.Git(t, h.BoardDir(), "remote", "get-url", "origin"), "for-each-ref", "refs/heads/"+boardBranch)

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if got, err := os.ReadFile(machineFile); err != nil || string(got) != machineBytes {
		t.Errorf("machine gate file = %q, %v after Ensure; want it unchanged", got, err)
	}
	if got, err := os.ReadFile(configengine.ConfigFile(h.BoardDir(), "fabric")); err != nil || !strings.Contains(string(got), newerHubWideKey) {
		t.Errorf("board fabric.yaml = %q, %v after Ensure; want the newer binary's %q kept", got, err, newerHubWideKey)
	}
	if got, err := os.ReadFile(configengine.ConfigFile(pairRecords, "logger")); err != nil || !strings.Contains(string(got), newerWorktreeKey) {
		t.Errorf("pair logger.yaml =%q, %v after Ensure; want the newer binary's %q kept", got, err, newerWorktreeKey)
	}

	if want := []string{h.BoardDir()}; !slices.Equal(*spawned, want) {
		t.Errorf("detached board syncs = %v; want one for the board %v after the hub-wide commit", *spawned, want)
	}
	if got := gitkit.Git(t, gitkit.Git(t, h.BoardDir(), "remote", "get-url", "origin"), "for-each-ref", "refs/heads/"+boardBranch); got != originBoardBefore {
		t.Errorf("origin board branch %q -> %q; want no synchronous push", originBoardBefore, got)
	}

	wantSubject := "lym: reconcile hub-wide config for build " + fabricengine.BinaryLabel()
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

	tracked := gitkit.LsFiles(t, h.BoardDir())
	if run.loomDefaultGone && !slices.Contains(tracked, filepath.ToSlash(loomDefaultRel)) {
		t.Errorf("the board does not track the reseeded %s", loomDefaultRel)
	}
	if !slices.Contains(tracked, lensesDefault) {
		t.Errorf("board tracks %v; want the reseeded %s", tracked, lensesDefault)
	}
	if got, err := os.ReadFile(filepath.Join(h.BoardDir(), lensesDefault)); err != nil || string(got) != configengine.SparseBaseHeader {
		t.Errorf("board lenses.yaml = %q, %v; want the reseeded default to hold only the sparse header", got, err)
	}
	assertClean(t, h.PrimeRecords())
	assertClean(t, pairRecords)
	if primeAfter, pairAfter := commitCount(t, h.PrimeRecords()), commitCount(t, pairRecords); primeAfter != primeBefore || pairAfter != pairBefore {
		t.Errorf("records commits: prime %d -> %d, pair %d -> %d; want none, since tracked worktree files are left as they are", primeBefore, primeAfter, pairBefore, pairAfter)
	}
	if key, found := stampKey(t, geom); !found || key != runningKey() {
		t.Errorf("stamp = (%q, %v); want the running key", key, found)
	}

	// A second walk lands no new commit and leaves every per-worktree file byte-identical.
	boardAfter := commitCount(t, h.BoardDir())
	filesBefore := map[string]map[string]string{"prime": configFilesOf(t, h.PrimeWorktree()), "pair": configFilesOf(t, pair)}
	if err := os.Remove(geom.StampPath()); err != nil {
		t.Fatalf("remove the stamp: %v", err)
	}
	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if got := commitCount(t, h.PrimeRecords()); got != primeBefore {
		t.Errorf("second walk landed a prime commit: %d -> %d", primeBefore, got)
	}
	if got := commitCount(t, pairRecords); got != pairBefore {
		t.Errorf("second walk landed a pair commit: %d -> %d", pairBefore, got)
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

// TestEnsure_CurrentStampLeavesConfigAlone does not call t.Parallel: it swaps the package-global detached board sync.
func TestEnsure_CurrentStampLeavesConfigAlone(t *testing.T) {
	tiergate.Require(t, 1)

	h := newStaleHub(t)
	geom := geometryOf(h)
	writeCurrentStamp(t, geom)
	spawned := recordBoardSyncs(t)
	boardBefore, primeBefore := commitCount(t, h.BoardDir()), commitCount(t, h.PrimeRecords())

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if len(*spawned) != 0 {
		t.Errorf("a current stamp still walked the hub: board syncs %v", *spawned)
	}
	if got := commitCount(t, h.BoardDir()); got != boardBefore {
		t.Errorf("board commits %d -> %d; want none", boardBefore, got)
	}
	if got := commitCount(t, h.PrimeRecords()); got != primeBefore {
		t.Errorf("prime commits %d -> %d; want none", primeBefore, got)
	}
}

func TestEnsure_MidMergePairIsLeftAloneAndRetried(t *testing.T) {
	tiergate.Require(t, 1)
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

			h := newStaleHub(t)
			geom := geometryOf(h)
			clear := tc.mark(t, h)

			if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
				t.Fatalf("Ensure: %v", err)
			}

			if _, found := stampKey(t, geom); found {
				t.Errorf("stamp written while a worktree was skipped")
			}

			clear()
			if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
				t.Fatalf("Ensure after the merge state cleared: %v", err)
			}
			if _, found := stampKey(t, geom); !found {
				t.Errorf("stamp absent after the retry")
			}
		})
	}
}

func TestEnsure_HubWideWalkRendersAndCommitsTheHeddleOverview(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	loomRel := filepath.ToSlash(loomDefaultRel)
	tests := []struct {
		name string
		// seed leaves the board's overview and heddle.json in the state the row walks, in board commits.
		seed func(t *testing.T, h *hubforge.Hub)
		// failedWriteFirst runs a walk whose board write fails first, and expects it to restore the overview.
		failedWriteFirst bool
		// renderFails marks a heddle.json that does not load, so the render fails and the walk goes on.
		renderFails bool
		// wantOverviewCommitted is whether the walk's board commit names the overview.
		wantOverviewCommitted bool
		// wantLoomCommitted is whether the walk's board commit names the loom default the seed deleted.
		wantLoomCommitted bool
	}{
		{
			name: "missing overview",
			seed: func(t *testing.T, h *hubforge.Hub) {
				gitkit.Git(t, h.BoardDir(), "rm", "-q", "--ignore-unmatch", "--", heddleengine.OverviewFileName)
				gitkit.Git(t, h.BoardDir(), "commit", "-q", "--allow-empty", "-m", "fixture: no heddle overview")
			},
			failedWriteFirst:      true,
			wantOverviewCommitted: true,
		},
		{
			name: "stale overview",
			seed: func(t *testing.T, h *hubforge.Hub) {
				gitkit.CommitFile(t, h.BoardDir(), heddleengine.OverviewFileName, "# Heddle\n\nstale\n", "fixture: stale heddle overview")
			},
			failedWriteFirst:      true,
			wantOverviewCommitted: true,
		},
		{
			name: "overview already current",
			seed: func(t *testing.T, h *hubforge.Hub) {
				if changed, err := heddleengine.RenderOverview(h.BoardDir()); err != nil {
					t.Fatalf("RenderOverview: %v", err)
				} else if changed {
					gitkit.Git(t, h.BoardDir(), "add", "--", heddleengine.OverviewFileName)
					gitkit.Git(t, h.BoardDir(), "commit", "-q", "-m", "fixture: current heddle overview")
				}
				deleteBoardLoomDefault(t, h)
			},
			wantLoomCommitted: true,
		},
		{
			name: "heddle store that does not load",
			seed: func(t *testing.T, h *hubforge.Hub) {
				gitkit.CommitFile(t, h.BoardDir(), heddleengine.FileName, "{not json", "fixture: broken heddle.json")
				deleteBoardLoomDefault(t, h)
			},
			renderFails:       true,
			wantLoomCommitted: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newStaleHub(t)
			geom := geometryOf(h)
			tc.seed(t, h)
			overview := filepath.Join(h.BoardDir(), heddleengine.OverviewFileName)
			before, beforeErr := os.ReadFile(overview)
			statusBefore := gitkit.GitStatusPorcelain(t, h.BoardDir())

			if tc.failedWriteFirst {
				if err := hubreconcile.EnsureFailingAfterHubWideWrite(geom, hubreconcile.Options{}, errors.New("fixture: failure after the hub-wide write")); err == nil {
					t.Fatalf("EnsureFailingAfterHubWideWrite = nil; want the fixture failure")
				}
				after, afterErr := os.ReadFile(overview)
				if string(after) != string(before) || errors.Is(afterErr, os.ErrNotExist) != errors.Is(beforeErr, os.ErrNotExist) {
					t.Errorf("overview after a failed board write = %q (err %v); want it restored to %q (err %v)", after, afterErr, before, beforeErr)
				}
				if got := gitkit.GitStatusPorcelain(t, h.BoardDir()); got != statusBefore {
					t.Errorf("board status after a failed board write = %q; want %q", got, statusBefore)
				}
			}

			if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
				t.Fatalf("Ensure: %v", err)
			}
			committed := strings.Split(gitkit.Git(t, h.BoardDir(), "log", "-1", "--name-only", "--format="), "\n")
			if got := slices.Contains(committed, heddleengine.OverviewFileName); got != tc.wantOverviewCommitted {
				t.Errorf("the walk's board commit holds %q; want %s committed: %v", committed, heddleengine.OverviewFileName, tc.wantOverviewCommitted)
			}
			if got := slices.Contains(committed, loomRel); got != tc.wantLoomCommitted {
				t.Errorf("the walk's board commit holds %q; want %s committed: %v", committed, loomRel, tc.wantLoomCommitted)
			}
			after, afterErr := os.ReadFile(overview)
			if tc.renderFails {
				if string(after) != string(before) || errors.Is(afterErr, os.ErrNotExist) != errors.Is(beforeErr, os.ErrNotExist) {
					t.Errorf("overview after a failed render = %q (err %v); want it left as %q (err %v)", after, afterErr, before, beforeErr)
				}
				if _, err := heddleengine.RenderOverview(h.BoardDir()); err == nil {
					t.Errorf("RenderOverview = nil error; want the broken heddle.json to still fail")
				}
			} else if changed, err := heddleengine.RenderOverview(h.BoardDir()); err != nil || changed {
				t.Errorf("RenderOverview after the walk = (%v, %v); want (false, nil), the walk having rendered what heddle holds", changed, err)
			}
			if got := gitkit.GitStatusPorcelain(t, h.BoardDir()); got != statusBefore {
				t.Errorf("board status after the walk = %q; want %q, the overview committed", got, statusBefore)
			}
		})
	}
}

// commitTemplate commits module's template as its config file in the records worktree at recordsRoot.
func commitTemplate(t *testing.T, recordsRoot, module string) {
	t.Helper()

	m, _ := configreg.Lookup(module)
	gitkit.CommitFile(t, recordsRoot, configengine.ConfigFileRel(module), m.Template(), "fixture: fixed "+module+".yaml")
}

func TestEnsure_FailedWalkNamesTheFileAndLeavesTheTreeAsItWas(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// breakConfig breaks one config file of the hub.
		// It returns the worktree the walk fails in, the repo holding the file, and the file.
		breakConfig func(t *testing.T, h *hubforge.Hub) (worktree, repo, file string)
		// module is the module whose "lym config" the message names; empty when the failure names no module.
		module string
		repair func(t *testing.T, h *hubforge.Hub)
		// restoresBoard pins that the failed walk leaves the board's config files at their prior bytes.
		restoresBoard bool
		// wantInMessage is text the error message must hold; empty skips the check.
		wantInMessage string
		// wantAbsentFromMessage is text the error message must not hold; empty skips the check.
		wantAbsentFromMessage string
		// ensure runs the walk; nil runs the default walk.
		ensure func(geom hubreconcile.Geometry) error
	}{
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
				if err := os.MkdirAll(path, 0o755); err != nil {
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
			name: "unparseable hub-wide config names the default-layer command",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				hubforge.SeedFabricConfig(t, h, "a: [unclosed\n")
				return h.BoardDir(), h.BoardDir(), configengine.ConfigFile(h.BoardDir(), "fabric")
			},
			module:        "fabric",
			wantInMessage: "lym config fabric --layer default",
			repair: func(t *testing.T, h *hubforge.Hub) {
				m, _ := configreg.Lookup("fabric")
				hubforge.SeedFabricConfig(t, h, m.Template())
			},
		},
		{
			name: "a failure after the hub-wide files and a template base were written restores the files and removes the base",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				// The profiles template base is dropped from the board, so the walk writes it before the failure.
				// The restore must remove it again.
				gitkit.Git(t, h.BoardDir(), "rm", "-q", "--ignore-unmatch", "--", filepath.ToSlash(configengine.TemplateBaseFileRel("profiles")))
				gitkit.Git(t, h.BoardDir(), "commit", "-q", "--allow-empty", "-m", "fixture: no profiles template base")
				return h.BoardDir(), h.BoardDir(), h.BoardDir()
			},
			restoresBoard: true,
			ensure: func(geom hubreconcile.Geometry) error {
				return hubreconcile.EnsureFailingAfterHubWideWrite(geom, hubreconcile.Options{}, errors.New("fixture: failure after the hub-wide write"))
			},
			repair: func(t *testing.T, h *hubforge.Hub) {},
		},
		{
			name: "a template base that is not a YAML mapping names the base and no config command",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				gitkit.CommitFile(t, h.BoardDir(), configengine.TemplateBaseFileRel("profiles"), "- not\n- a mapping\n", "fixture: corrupt profiles template base")
				return h.BoardDir(), h.BoardDir(), configengine.TemplateBaseFile(h.BoardDir(), "profiles")
			},
			wantInMessage:         "delete it",
			wantAbsentFromMessage: "lym config",
			repair: func(t *testing.T, h *hubforge.Hub) {
				gitkit.Git(t, h.BoardDir(), "rm", "-q", "--", filepath.ToSlash(configengine.TemplateBaseFileRel("profiles")))
				gitkit.Git(t, h.BoardDir(), "commit", "-q", "-m", "fixture: delete the corrupt profiles template base")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newStaleHub(t)
			geom := geometryOf(h)
			worktree, repo, file := tc.breakConfig(t, h)
			statusBefore := gitkit.GitStatusPorcelain(t, repo)
			boardFilesBefore := configFilesOf(t, h.BoardDir())

			var err error
			if tc.ensure != nil {
				err = tc.ensure(geom)
			} else {
				err = hubreconcile.Ensure(geom, hubreconcile.Options{})
			}

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
				if !strings.Contains(err.Error(), "lym config "+tc.module) {
					t.Errorf("message %q does not name lym config %s", err.Error(), tc.module)
				}
			}
			if tc.wantAbsentFromMessage != "" && strings.Contains(err.Error(), tc.wantAbsentFromMessage) {
				t.Errorf("message %q holds %q", err.Error(), tc.wantAbsentFromMessage)
			}
			if tc.wantInMessage != "" && !strings.Contains(err.Error(), tc.wantInMessage) {
				t.Errorf("message %q does not hold %q", err.Error(), tc.wantInMessage)
			}
			if tc.restoresBoard {
				if boardFilesAfter := configFilesOf(t, h.BoardDir()); !maps.Equal(boardFilesBefore, boardFilesAfter) {
					t.Errorf("the failed walk changed the board's config files:\nbefore: %v\nafter: %v", boardFilesBefore, boardFilesAfter)
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
		})
	}
}

// The logger's output is process-global, so this test does not call t.Parallel.
func TestEnsureOrWarn_WarnsWithTheWayForward(t *testing.T) {
	tiergate.Require(t, 1)
	const wayForwardText = "start verb to retry the reconcile"

	tests := []struct {
		name string
		// setup breaks the hub.
		// It returns the call's options, the file or lock the Warn must name, and a repair that fixes the cause.
		// A nil repair skips the second call.
		setup func(t *testing.T, h *hubforge.Hub, geom hubreconcile.Geometry) (opts hubreconcile.Options, wantNamed string, repair func())
	}{
		{
			name: "broken config is warned about, then reconciled once fixed",
			setup: func(t *testing.T, h *hubforge.Hub, geom hubreconcile.Geometry) (hubreconcile.Options, string, func()) {
				gitkit.CommitFile(t, h.PairRecordsSibling("pair-a"), configengine.ConfigFileRel("loom"), "a: [unclosed\n", "fixture: broken loom.yaml")
				return hubreconcile.Options{}, configengine.ConfigFile(h.PairCodeWorktree("pair-a"), "loom"), func() { commitTemplate(t, h.PairRecordsSibling("pair-a"), "loom") }
			},
		},
		{
			name: "held lock is warned about",
			setup: func(t *testing.T, h *hubforge.Hub, geom hubreconcile.Geometry) (hubreconcile.Options, string, func()) {
				if err := os.MkdirAll(filepath.Dir(geom.LockPath()), 0o755); err != nil {
					t.Fatalf("mkdir lock dir: %v", err)
				}
				held, err := lock.AcquireWriteLock(geom.LockPath())
				if err != nil {
					t.Fatalf("hold lock: %v", err)
				}
				t.Cleanup(func() { _ = held.Release() })
				return hubreconcile.Options{LockWait: 50 * time.Millisecond}, geom.LockPath(), nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newStaleHub(t)
			logs := logcapture.Capture(t)
			geom := geometryOf(h)
			opts, wantNamed, repair := tt.setup(t, h, geom)

			hubreconcile.EnsureOrWarn(geom, opts)

			for _, want := range []string{wantNamed, wayForwardText, "`lym beam start`"} {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("EnsureOrWarn log lacks %q:\n%s", want, logs.String())
				}
			}
			if _, found := stampKey(t, geom); found {
				t.Errorf("stamp written after a failed reconcile")
			}
			if repair == nil {
				return
			}

			repair()
			logs.Reset()
			hubreconcile.EnsureOrWarn(geom, opts)

			if strings.Contains(logs.String(), wayForwardText) {
				t.Errorf("EnsureOrWarn warned after the repair:\n%s", logs.String())
			}
			if _, found := stampKey(t, geom); !found {
				t.Errorf("stamp absent after the repaired reconcile")
			}
		})
	}
}

func TestEnsure_ConcurrentCallsLandOneBoardCommit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := newStaleHub(t)
	geom := geometryOf(h)
	deleteBoardLoomDefault(t, h)
	boardBefore := commitCount(t, h.BoardDir())

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

	if got := commitCount(t, h.BoardDir()); got != boardBefore+1 {
		t.Errorf("board commits %d -> %d; want exactly one", boardBefore, got)
	}
}

// TestEnsure_PairRegisteredDuringTheWalkIsReconciledBeforeTheStamp does not call t.Parallel:
// it installs the process-wide worktree seam and swaps the package-global detached board sync.
func TestEnsure_PairRegisteredDuringTheWalkIsReconciledBeforeTheStamp(t *testing.T) {
	tiergate.Require(t, 1)

	recordBoardSyncs(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	geom := geometryOf(h)
	t.Cleanup(fabricengine.InstallWorktreeSeam(heddleengine.Seam{}))
	renderedFiles := []string{"CLAUDE.local.md", "AGENTS.override.md"}
	listings := 0
	hook := func() {
		listings++
		if listings != 1 {
			return
		}
		hubforge.AddPair(t, h, "late")
		for _, name := range renderedFiles {
			if err := os.Remove(filepath.Join(h.PairCodeWorktree("late"), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("remove %s from the late pair: %v", name, err)
			}
		}
	}

	if err := hubreconcile.EnsureWithHooks(geom, hubreconcile.Options{}, hook, nil); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	for _, name := range renderedFiles {
		content, err := os.ReadFile(filepath.Join(h.PairCodeWorktree("late"), name))
		if err != nil || !strings.HasPrefix(string(content), heddleengine.Marker) {
			t.Errorf("%s of the late pair = %q, %v; want a file rendered by the walk", name, content, err)
		}
	}
	assertClean(t, h.PairRecordsSibling("late"))
	if _, found := stampKey(t, geom); !found {
		t.Errorf("stamp absent after the walk")
	}
}

func TestEnsure_PairDirectoryDeletedIsSkippedAsRemoved(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := newStaleHub(t)
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
	tiergate.Require(t, 1)
	t.Parallel()

	h := newStaleHub(t)
	geom := geometryOf(h)
	junction := filepath.Join(h.PairCodeWorktree("pair-a"), "_lym")
	if err := os.Remove(junction); err != nil {
		t.Fatalf("remove _lym junction: %v", err)
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
	tiergate.Require(t, 1)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	logs := logcapture.Capture(t)
	geom := geometryOf(h)
	deleteBoardLoomDefault(t, h)
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
	tiergate.Require(t, 1)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	geom := geometryOf(h)
	deleteBoardLoomDefault(t, h)
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

// The installed notifier and the detached board sync are process-global, so this test does not call t.Parallel.
func TestEnsure_CarriesATemplateChangeIntoASeedOnlyConfigFile(t *testing.T) {
	tiergate.Require(t, 1)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	geom := geometryOf(h)
	const lightDarn = "  darn:\n    loom: {code_max_bounces: 1, code_hard_max_bounces: 2, code_reviewer_model: \"opus[medium]\"}\n"
	const lightDarnSolo = "  darn:\n    loom: {code_max_bounces: 1, code_hard_max_bounces: 2, code_reviewer_fan: \"\", code_reviewer_model: \"opus[medium]\"}\n"
	const solidDarn = "  darn:\n    loom: {code_max_bounces: 2, code_hard_max_bounces: 3, code_reviewer_model: \"opus[medium]\"}\n"
	const solidDarnBase = "  darn:\n    loom: {code_max_bounces: 3, code_hard_max_bounces: 4, code_reviewer_model: \"opus[medium]\"}\n"
	const solidDarnTuned = "  darn:\n    loom: {code_max_bounces: 5, code_hard_max_bounces: 4, code_reviewer_model: \"opus[medium]\"}\n"
	const solidDarnFollowed = "  darn:\n    loom: {code_max_bounces: 5, code_hard_max_bounces: 3, code_reviewer_model: \"opus[medium]\"}\n"
	olderTemplate := strings.Replace(strings.Replace(runprofile.ConfigTemplate(), lightDarn, lightDarnSolo, 1), solidDarn, solidDarnBase, 1)
	hubProfiles := strings.Replace(olderTemplate, solidDarnBase, solidDarnTuned, 1)
	gitkit.CommitFile(t, h.BoardDir(), configengine.ConfigFileRel("profiles"), hubProfiles, "fixture: profiles seeded from an older template, with solid's darn rounds tuned")
	gitkit.CommitFile(t, h.BoardDir(), configengine.TemplateBaseFileRel("profiles"), olderTemplate, "fixture: the older template as the profiles' base")
	gitkit.Git(t, h.BoardDir(), "remote", "remove", "origin")
	recordBoardSyncs(t)
	var mailed []string
	t.Cleanup(hubreconcile.InstallNotifier(func(_, line string) (bool, error) {
		mailed = append(mailed, line)
		return true, nil
	}))

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	got, err := os.ReadFile(configengine.ConfigFile(h.BoardDir(), "profiles"))
	if err != nil {
		t.Fatalf("read the board's profiles.yaml: %v", err)
	}
	if strings.Contains(string(got), lightDarnSolo) || !strings.Contains(string(got), lightDarn) {
		t.Errorf("board profiles.yaml = %q; want light's darn block to follow the template and drop its solo fan", got)
	}
	if !strings.Contains(string(got), solidDarnFollowed) {
		t.Errorf("board profiles.yaml = %q; want the operator's tuned solid code_max_bounces kept and the untouched code_hard_max_bounces updated", got)
	}
	if base, err := os.ReadFile(configengine.TemplateBaseFile(h.BoardDir(), "profiles")); err != nil || !strings.HasSuffix(string(base), runprofile.ConfigTemplate()) {
		t.Errorf("board profiles.template-base = %q, %v; want it to end with the running template", base, err)
	}
	committed := gitkit.Git(t, h.BoardDir(), "log", "-1", "--name-only", "--format=")
	for _, rel := range []string{configengine.ConfigFileRel("profiles"), configengine.TemplateBaseFileRel("profiles")} {
		if !strings.Contains(committed, filepath.ToSlash(rel)) {
			t.Errorf("the walk's board commit holds %q; want it to carry %s", committed, rel)
		}
	}
	assertClean(t, h.BoardDir())
	for _, want := range []string{
		"profiles.yaml removed light.darn.loom.code_reviewer_fan",
		"profiles.yaml updated solid.darn.loom.code_hard_max_bounces: 4 -> 3",
		"profiles.yaml kept the hub's solid.darn.loom.code_max_bounces: file 5, template 2",
	} {
		if !strings.Contains(strings.Join(mailed, "\n"), want) {
			t.Errorf("orch mail = %q; want a reconcile notice naming %q", mailed, want)
		}
	}

	// The first walk records the template in the base, so a second walk has nothing to carry or report and mails nothing.
	mailedByFirstWalk := len(mailed)
	if err := os.Remove(geom.StampPath()); err != nil {
		t.Fatalf("remove the stamp: %v", err)
	}
	if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if len(mailed) != mailedByFirstWalk {
		t.Errorf("orch mail after the second walk = %q; want nothing beyond the first walk's", mailed[mailedByFirstWalk:])
	}
}

// unpushedHubWide is a hub whose board holds a hub-wide config commit that origin has not taken.
// The detached board sync is recorded and never run, so the commit stays unpushed.
// origin is the records remote, and hubWidePath is the config file the commit changed.
// before counts the board's commits before that commit.
// The build stamp is absent, so the next walk of the hub walks it again.
type unpushedHubWide struct {
	hub         *hubforge.Hub
	geom        hubreconcile.Geometry
	board       string
	branch      string
	origin      string
	hubWidePath string
	before      int
}

// arrangeUnpushedHubWide brings the fresh hub h to an unpushedHubWide by running the walk twice.
// Each run stamps the build and asks for the board's detached sync.
// The second run, with nothing new to commit, still asks for the sync, so an earlier unpushed commit is pushed too.
// The stamp is removed after each run.
func arrangeUnpushedHubWide(t *testing.T, h *hubforge.Hub) *unpushedHubWide {
	t.Helper()

	geom := geometryOf(h)
	board := h.BoardDir()
	deleteBoardLoomDefault(t, h)
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
	// The first walk of a hub also commits each seed-only module's template base, which is no config file.
	var changed []string
	for _, path := range strings.Fields(gitkit.Git(t, board, "show", "--name-only", "--format=", "HEAD")) {
		if !strings.HasSuffix(path, ".template-base") {
			changed = append(changed, path)
		}
	}
	if len(changed) != 1 {
		t.Fatalf("the hub-wide commit changed %v besides the template bases; want one config file", changed)
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
	tiergate.Require(t, 1)
	// The copies are taken before the Bolt replayer is installed, so no template build runs under it.
	replayedHub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	divergedHub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	boardkit.InstallReplay(t)

	t.Run("an upstream moved on other paths is replayed under the commit and the stamp is written", func(t *testing.T) {
		u := arrangeUnpushedHubWide(t, replayedHub)
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
		u := arrangeUnpushedHubWide(t, divergedHub)
		u.moveUpstream(t, u.hubWidePath, "fixer_timeout_min: 7\n")

		if err := hubreconcile.Ensure(u.geom, hubreconcile.Options{}); err != nil {
			t.Fatalf("second Ensure: %v", err)
		}

		if got := commitCount(t, u.board); got != u.before+1 {
			t.Errorf("second Ensure changed the board commits: %d -> %d; want no new commit", u.before+1, got)
		}
		for _, want := range []string{u.hubWidePath, "lym board sync"} {
			if !strings.Contains(logs.String(), want) {
				t.Errorf("log lacks the divergence Warn naming %q:\n%s", want, logs.String())
			}
		}
		if _, found := stampKey(t, u.geom); found {
			t.Errorf("stamp written although the hub-wide commit is still unpushed")
		}
	})
}

// refusingSeam is a worktree seam whose render of a made worktree always fails.
type refusingSeam struct{}

func (refusingSeam) Made(fabricengine.WorktreeEvent) error {
	return errors.New("the seam refuses")
}

func (refusingSeam) TookIn(fabricengine.WorktreeEvent) error { return nil }

// TestEnsure_RendersTheHeddleFileTargetsOfEveryWalkedWorktree does not call t.Parallel:
// it installs the process-wide worktree seam and swaps the package-global detached board sync.
func TestEnsure_RendersTheHeddleFileTargetsOfEveryWalkedWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	tests := []struct {
		name string
		seam fabricengine.WorktreeSeam
		// pair is the pair directory under the hub that Options.Pair names, empty for the hub-wide walk.
		pair string
		// rendered returns the worktree roots whose anchors end up holding the rendered files.
		rendered func(h *hubforge.Hub) []string
	}{
		{
			name:     "StaleWalk",
			seam:     heddleengine.Seam{},
			rendered: func(h *hubforge.Hub) []string { return []string{h.PrimeWorktree(), h.PairCodeWorktree("pair-a")} },
		},
		{
			name:     "PairCall",
			seam:     heddleengine.Seam{},
			pair:     "pair-a",
			rendered: func(h *hubforge.Hub) []string { return []string{h.PairCodeWorktree("pair-a")} },
		},
		{
			name:     "FailingSeam",
			seam:     refusingSeam{},
			rendered: func(*hubforge.Hub) []string { return nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordBoardSyncs(t)
			h := newStaleHub(t)
			geom := geometryOf(h)
			worktrees, err := fabricengine.CodeWorktrees(h.PrimeWorktree())
			if err != nil {
				t.Fatalf("CodeWorktrees() error = %v", err)
			}
			renderedFiles := []string{"CLAUDE.local.md", "AGENTS.override.md"}
			for _, w := range worktrees {
				for _, name := range renderedFiles {
					if err := os.Remove(filepath.Join(w.Anchor, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("remove %s from %s: %v", name, w.Anchor, err)
					}
				}
			}
			t.Cleanup(fabricengine.InstallWorktreeSeam(tt.seam))
			options := hubreconcile.Options{}
			if tt.pair != "" {
				options.Pair = filepath.Join(h.Path, tt.pair)
			}

			if err := hubreconcile.Ensure(geom, options); err != nil {
				t.Fatalf("Ensure() error = %v; want nil whatever the seam does", err)
			}

			rendered := tt.rendered(h)
			for _, w := range worktrees {
				for _, name := range renderedFiles {
					content, err := os.ReadFile(filepath.Join(w.Anchor, name))
					if slices.Contains(rendered, w.Path) {
						if err != nil || !strings.HasPrefix(string(content), heddleengine.Marker) {
							t.Errorf("%s of %s = %q, %v; want a file rendered by heddle", name, w.Path, content, err)
						}
					} else if !errors.Is(err, os.ErrNotExist) {
						t.Errorf("%s of %s = %q, %v; want it absent", name, w.Path, content, err)
					}
				}
			}
			key, found := stampKey(t, geom)
			if wantStamp := tt.pair == ""; wantStamp != (found && key == runningKey()) {
				t.Errorf("stamp = %q (found %v); want the running key written: %v", key, found, wantStamp)
			}
		})
	}
}
