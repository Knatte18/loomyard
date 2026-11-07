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
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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

// writeCurrentStamp marks the hub as reconciled by the test binary's build.
// It writes the stamp and, when the board lacks the base-sparse marker, commits that marker first.
// The walk treats a stamp on an unmarked board as stale.
func writeCurrentStamp(t *testing.T, geom hubreconcile.Geometry) {
	t.Helper()

	if _, err := os.Stat(configengine.BaseSparseMarker(geom.BoardDir)); errors.Is(err, os.ErrNotExist) {
		gitkit.CommitFile(t, geom.BoardDir, configengine.BaseSparseMarkerRel(), "# the board's base files are sparse\n", "fixture: board marker")
	}

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

// differingBatcherBudget is a batcher budget that differs from the repo default's,
// so a batcher.yaml holding it survives the strip.
const differingBatcherBudget = "budget: 450001"

// seedRetiredKey commits a batcher.yaml in the records worktree at recordsRoot.
// The file is the template with the retired key in place of orientation.
// It also sets one key to a value that differs from the repo default.
// It survives the sparse strip non-empty, so the test can read the retired key after the walk.
func seedRetiredKey(t testing.TB, recordsRoot string) {
	t.Helper()

	module, _ := configreg.Lookup("batcher")
	content := strings.Replace(module.Template(), "orientation: 31400", retiredKey+": 52000", 1)
	content = strings.Replace(content, "budget: 200000", differingBatcherBudget, 1)
	if !strings.Contains(content, retiredKey) || !strings.Contains(content, differingBatcherBudget) {
		t.Fatalf("batcher template no longer carries the orientation and budget lines the fixture rewrites")
	}
	gitkit.CommitFile(t, recordsRoot, configengine.ConfigFileRel("batcher"), content, "fixture: retired key")
}

// differingLoomBounces is a loom review budget that differs from the repo default's,
// so a loom.yaml holding it survives the strip.
const differingLoomBounces = "review_max_bounces: 99"

// replaceOnce returns content with the first old replaced by replacement, failing the test when content holds no old.
func replaceOnce(t testing.TB, module, content, old, replacement string) string {
	t.Helper()

	if !strings.Contains(content, old) {
		t.Fatalf("%s template no longer carries the %q text the fixture rewrites", module, old)
	}
	return strings.Replace(content, old, replacement, 1)
}

// formerUnits sets each top-level scalar unit of template to the given YAML value, keeping its line comment.
// It fails the test on a unit the template lacks.
func formerUnits(t *testing.T, module, template string, former map[string]string) string {
	t.Helper()

	lines := strings.Split(template, "\n")
	for key, value := range former {
		found := false
		for i, line := range lines {
			if !strings.HasPrefix(line, key+": ") {
				continue
			}
			found = true
			comment := ""
			if at := strings.Index(line, "  #"); at >= 0 {
				comment = line[at:]
			}
			lines[i] = key + ": " + value + comment
		}
		if !found {
			t.Fatalf("%s template no longer carries the %s unit the fixture rewrites", module, key)
		}
	}
	return strings.Join(lines, "\n")
}

// loomFormerUnits returns loom's tuned units at their former template values, leaving the plan perch's units aside.
func loomFormerUnits() map[string]string {
	former := map[string]string{
		"code_reviewer_model":    "sonnet[high]",
		"driver":                 "sonnet[medium]",
		"discussion_max_bounces": "~",
		"code_max_bounces":       "~",
		"code_auto_accept":       "false",
	}
	for _, unit := range []string{"discussion_lenses", "discussion_reviewer_model", "code_fixer_model"} {
		former[unit] = `""`
	}
	for _, segment := range []string{"discussion", "code"} {
		former[segment+"_reviewer_fan"] = `""`
		former[segment+"_fixer_fan"] = `""`
	}
	return former
}

// tapestryFormerUnits returns tapestry's tuned units at their former template values.
func tapestryFormerUnits() map[string]string {
	former := map[string]string{}
	for _, unit := range []string{"plan_lenses", "plan_reviewer_model", "plan_reviewer_fan", "plan_fixer_fan"} {
		former[unit] = `""`
	}
	return former
}

// preMigrationBase returns module's base file as a hub held it before the tuned defaults.
// It is the module's current template with every tuned unit at its former value.
func preMigrationBase(t *testing.T, module configreg.Module) string {
	t.Helper()

	template := module.Template()
	switch module.Name {
	case "loom":
		return formerUnits(t, module.Name, template, loomFormerUnits())
	case "tapestry":
		return formerUnits(t, module.Name, template, tapestryFormerUnits())
	case "shed":
		return formerUnits(t, module.Name, template, map[string]string{"driver_kind": "llm"})
	case "darn":
		return replaceOnce(t, module.Name, template, "verify_attempts: 3 ", "verify_attempts: 5 ")
	case "claude":
		var kept []string
		found, inRoles := false, false
		for _, line := range strings.Split(template, "\n") {
			switch {
			case strings.HasPrefix(line, "prompt_cache_ttl_roles:"):
				found, inRoles = true, true
				kept = append(kept, "prompt_cache_ttl_roles: {driver: 1h, webster: 1h}")
			case inRoles && strings.HasPrefix(line, " "):
			default:
				inRoles = false
				kept = append(kept, line)
			}
		}
		if !found {
			t.Fatalf("claude template no longer carries the prompt_cache_ttl_roles map the fixture rewrites")
		}
		return strings.Join(kept, "\n")
	case "orch":
		return formerUnits(t, module.Name, template, map[string]string{"soft_threshold_tokens": "300000", "threshold_tokens": "400000"})
	case "batcher":
		return replaceOnce(t, module.Name, template, "budget: 200000", "budget: 450000")
	}
	return template
}

// commitPreMigrationBase commits every non-seed-only module's base file on the board at boardDir.
// Each file is the base a hub held before the tuned defaults.
// It also deletes the board marker, so the walk finds an unmigrated board with something to strip.
func commitPreMigrationBase(t *testing.T, boardDir string) {
	t.Helper()

	for _, module := range configreg.Modules() {
		if module.SeedOnly {
			continue
		}
		path := configengine.ConfigFile(boardDir, module.Name)
		if err := os.WriteFile(path, []byte(preMigrationBase(t, module)), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	gitkit.Git(t, boardDir, "rm", "-q", "--ignore-unmatch", "--", filepath.ToSlash(configengine.BaseSparseMarkerRel()))
	removeReinterpretationMarkers(t, boardDir)
	gitkit.Git(t, boardDir, "add", "-A")
	gitkit.Git(t, boardDir, "commit", "-q", "-m", "fixture: pre-migration board base files")
}

// removeReinterpretationMarkers stages the deletion of every reinterpretation marker the board at boardDir tracks,
// so the board is one no registered reinterpretation has been applied to.
// The forged hub arrives with them, written by the walk that cloned it.
func removeReinterpretationMarkers(t *testing.T, boardDir string) {
	t.Helper()

	for _, module := range configreg.Modules() {
		for _, change := range module.Reinterpretations {
			gitkit.Git(t, boardDir, "rm", "-q", "--ignore-unmatch", "--", filepath.ToSlash(configengine.ReinterpretedMarkerRel(change.ID)))
		}
	}
}

// commitRetiredKeys appends the key each module retired to the board's darn, gate and landing base files.
// It commits them.
func commitRetiredKeys(t *testing.T, boardDir string) {
	t.Helper()

	for module, line := range map[string]string{"darn": "verify: make verify\n", "gate": "go_parallel: 8\n", "landing": "publish_verify: x\n"} {
		path := configengine.ConfigFile(boardDir, module)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := os.WriteFile(path, append(data, line...), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	gitkit.Git(t, boardDir, "add", "-A")
	gitkit.Git(t, boardDir, "commit", "-q", "-m", "fixture: retired keys in the board base files")
}

// commitPreMigrationFiles commits, in the records worktree at recordsRoot, a full pre-migration per-worktree file set.
// Every file is its module's template, except loom's and shed's.
// Loom's file differs from the repo default in one key and holds its former discussion_lenses.
// Shed's file holds its former driver_kind.
// It also deletes the sparse marker, so the walk finds an unmigrated worktree with something to strip.
func commitPreMigrationFiles(t testing.TB, recordsRoot string) {
	t.Helper()

	for _, module := range configreg.Modules() {
		if module.HubWide {
			continue
		}
		content := module.Template()
		switch module.Name {
		case "loom":
			content = strings.Replace(content, "review_max_bounces: 3", differingLoomBounces, 1)
			if !strings.Contains(content, differingLoomBounces) {
				t.Fatalf("loom template no longer carries the review_max_bounces line the fixture rewrites")
			}
			content = replaceOnce(t, module.Name, content, "discussion_lenses: twill-discussion", `discussion_lenses: ""`)
		case "shed":
			content = replaceOnce(t, module.Name, content, "driver_kind: go", "driver_kind: llm")
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

// configFilesOf returns the content of every file in codeRoot's config dir, keyed by file name.
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

// staleHub is a hub with the pair pair-a whose records worktrees lack the sparse marker.
// Each records worktree holds a full pre-migration file set with the retired key committed, and the hub has no stamp.
// The build commits only in the records worktrees, so the board stays at its upstream tip.
var staleHub = &hubforge.Registration{
	Key:  "hubreconcile-stale",
	Base: hubforge.Shape{Anchor: ".", Pairs: []string{"pair-a"}},
	Build: func(tb testing.TB, h *hubforge.Hub) {
		commitPreMigrationFiles(tb, h.PrimeRecords())
		commitPreMigrationFiles(tb, h.PairRecordsSibling("pair-a"))
		seedRetiredKey(tb, h.PrimeRecords())
		seedRetiredKey(tb, h.PairRecordsSibling("pair-a"))
	},
}

// newStaleHub returns the test's own copy of the staleHub template.
func newStaleHub(t *testing.T) *hubforge.Hub {
	t.Helper()
	return hubforge.CopyRegistered(t, staleHub)
}

// newerHubWideKey is a fabric key the running template lacks and no retired key covers, as a newer binary writes.
const newerHubWideKey = "newer_fixture_key: 1"

// newerWorktreeKey is a logger key the running template lacks and no retired key covers.
// The test plants it in the pair's worktree file.
const newerWorktreeKey = "newer_worktree_key: 1"

// seedRetiredHubWideKey commits on the board a landing.yaml holding only a key landing retired,
// so the hub-wide reconcile rewrites it.
func seedRetiredHubWideKey(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	gitkit.CommitFile(t, h.BoardDir(), configengine.ConfigFileRel("landing"), "publish_verify: x\n", "fixture: retired landing key")
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

// TestEnsure_StaleBuildReconcilesAndCommitsEveryWorktree does not call t.Parallel:
// it swaps the package-global detached board sync.
func TestEnsure_StaleBuildReconcilesAndCommitsEveryWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	for _, tc := range []struct {
		name string
		run  staleRun
	}{
		{name: "board up to date"},
		{name: "board behind its upstream", run: staleRun{boardBehind: true}},
		{name: "loom default deleted beside a current stamp", run: staleRun{loomDefaultGone: true}},
		{name: "unmigrated board is stripped once and marked", run: staleRun{unmigratedBoard: true}},
		{name: "unmigrated board behind its upstream is stripped and marked over the pulled commit", run: staleRun{boardBehind: true, unmigratedBoard: true}},
		{name: "unmigrated board holding retired keys is stripped of them and of its template-equal units", run: staleRun{unmigratedBoard: true, retiredKeys: true}},
		{name: "marker removed beside a current stamp strips the board again", run: staleRun{markerRemoved: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runStaleBuildReconcile(t, tc.run)
		})
	}
}

// staleRun sets the hub state a stale-build walk starts from, and the checks it makes beyond the common reconcile.
type staleRun struct {
	// boardBehind leaves the board behind its upstream by one commit.
	boardBehind bool
	// loomDefaultGone deletes loom's committed repo default, then writes a stamp that names the running build.
	loomDefaultGone bool
	// unmigratedBoard commits every board base file at its pre-migration values and deletes the board marker before the walk.
	unmigratedBoard bool
	// retiredKeys sets, on an unmigrated board, a key the module retired in the board's darn, gate and landing base files.
	// Each such key sits beside template-equal units.
	retiredKeys bool
	// markerRemoved removes the board marker by a board commit after the second walk.
	// The stamp still names the running build, and the walk that follows runs again.
	markerRemoved bool
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

// tunedGenericMarker is the text of a hub's tuned generic lens.
const tunedGenericMarker = "tuned generic prose"

// todaysBurlerYAML is a board burler.yaml in the hub shape that follows the migration.
// It holds a tuned generic lens and the seven-lens twill-discussion fan.
const todaysBurlerYAML = `lenses:
  generic: |
    ` + tunedGenericMarker + `
fans:
  twill-discussion: [generic, correctness, error-handling, test-gaps, security, performance, api-design]
`

// runStaleBuildReconcile walks a stale hub and asserts that every worktree and the board get one reconcile commit.
// A board behind its upstream is pulled from upstream.
// The walk keeps a key the template lacks in the board's fabric.yaml and in the pair's logger.yaml.
// When loom's committed repo default is deleted and the stamp names the running build:
// the walk reseeds and commits that default on the board.
// batcher's default stays intact so the retired-key check keeps reading it.
func runStaleBuildReconcile(t *testing.T, run staleRun) {
	t.Helper()
	boardBehind, loomDefaultGone := run.boardBehind, run.loomDefaultGone

	h := newStaleHub(t)
	geom := geometryOf(h)
	pair := h.PairCodeWorktree("pair-a")
	var modelsBefore []byte
	if run.unmigratedBoard {
		var err error
		if modelsBefore, err = os.ReadFile(configengine.ConfigFile(h.BoardDir(), "models")); err != nil {
			t.Fatalf("read the board's models.yaml: %v", err)
		}
		commitPreMigrationBase(t, h.BoardDir())
		if run.retiredKeys {
			commitRetiredKeys(t, h.BoardDir())
		}
	}
	hubforge.SeedFabricConfig(t, h, newerHubWideKey+"\n")
	pairRecords := h.PairRecordsSibling("pair-a")
	pairLogger, err := os.ReadFile(configengine.ConfigFile(pairRecords, "logger"))
	if err != nil {
		t.Fatalf("read the pair's logger.yaml: %v", err)
	}
	gitkit.CommitFile(t, pairRecords, configengine.ConfigFileRel("logger"), string(pairLogger)+newerWorktreeKey+"\n", "fixture: a key the template lacks")
	loomDefault := configengine.ConfigFileRel("loom")
	lensesDefault := configengine.ConfigFileRel("lenses")
	burlerDefault := configengine.ConfigFileRel("burler")
	gitkit.Git(t, h.BoardDir(), "rm", "-q", lensesDefault)
	gitkit.Git(t, h.BoardDir(), "commit", "-q", "-m", "fixture: delete lenses default")
	gitkit.CommitFile(t, h.BoardDir(), burlerDefault, todaysBurlerYAML, "fixture: today's burler.yaml")
	// A hub in the post-migration shape has no lenses.yaml anywhere,
	// so the board's moved values are not shadowed by a seeded prime copy.
	for _, records := range []string{h.PrimeRecords(), h.PairRecordsSibling("pair-a")} {
		gitkit.Git(t, records, "rm", "-q", lensesDefault)
		gitkit.Git(t, records, "commit", "-q", "-m", "fixture: delete lenses file")
	}
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
	// The machine file sits under the board dir's never-tracked .lym, which no reconcile reads or writes.
	const machineBytes = "slots: 1\ntier_parallel:\n  tier0: 8\n"
	machineFile := configengine.MachineFile(h.BoardDir(), "gate")
	if err := os.MkdirAll(filepath.Dir(machineFile), 0o755); err != nil {
		t.Fatalf("create the board's .lym config dir: %v", err)
	}
	if err := os.WriteFile(machineFile, []byte(machineBytes), 0o644); err != nil {
		t.Fatalf("seed the machine file: %v", err)
	}
	primeBefore, pairBefore := commitCount(t, h.PrimeRecords()), commitCount(t, h.PairRecordsSibling("pair-a"))
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

	if loomDefaultGone && !slices.Contains(gitkit.LsFiles(t, h.BoardDir()), loomDefault) {
		t.Errorf("the board does not track the reseeded %s", loomDefault)
	}
	tracked := gitkit.LsFiles(t, h.BoardDir())
	if !slices.Contains(tracked, lensesDefault) || slices.Contains(tracked, burlerDefault) {
		t.Errorf("board tracks %v; want %s tracked and %s no longer tracked", tracked, lensesDefault, burlerDefault)
	}
	lensesData, err := os.ReadFile(filepath.Join(h.BoardDir(), lensesDefault))
	if err != nil {
		t.Fatalf("read the board's lenses.yaml: %v", err)
	}
	if !strings.Contains(string(lensesData), tunedGenericMarker) {
		t.Errorf("board lenses.yaml lacks %q after the move and reconcile:\n%s", tunedGenericMarker, lensesData)
	}
	// The template supplies every lens the hub did not tune, so reconcile never copies one in.
	if strings.Contains(string(lensesData), "correctness:") {
		t.Errorf("board lenses.yaml holds the untuned template lens %q after the move and reconcile:\n%s", "correctness:", lensesData)
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
	if run.unmigratedBoard {
		assertBoardStrippedOnce(t, h, modelsBefore)
	}

	// Each worktree is stripped and committed with its marker:
	// loom.yaml keeps only its differing key, or is deleted when the reseeded default equals it.
	// A former template value the file held, loom's discussion_lenses and shed's driver_kind, is dropped as well.
	for name, records := range map[string]string{"prime": h.PrimeRecords(), "pair": h.PairRecordsSibling("pair-a")} {
		if !markerTracked(t, records) {
			t.Errorf("%s does not track the sparse marker after the walk", name)
		}
		if data, err := os.ReadFile(configengine.ConfigFile(records, "shed")); err == nil && strings.Contains(string(data), "driver_kind") {
			t.Errorf("%s shed.yaml = %q; want the former driver_kind dropped", name, data)
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
	// On an unmigrated board the marker is present before the second walk,
	// so a base unit equal to a former value is a pin that walk keeps.
	const pinnedFormerDriver = "driver: sonnet[medium]\n"
	if run.unmigratedBoard {
		gitkit.CommitFile(t, h.BoardDir(), loomDefault, pinnedFormerDriver, "fixture: pin a former value")
	}
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
	if run.unmigratedBoard {
		if got, err := os.ReadFile(filepath.Join(h.BoardDir(), loomDefault)); err != nil || string(got) != pinnedFormerDriver {
			t.Errorf("board loom.yaml = %q, %v after the second walk; want the pinned former value kept", got, err)
		}
	}

	if run.markerRemoved {
		// A board commit removes the marker beside a template-equal unit;
		// the stamp still names the running build, yet the next walk strips the unit and marks the board again.
		gitkit.Git(t, h.BoardDir(), "rm", "-q", "--", filepath.ToSlash(configengine.BaseSparseMarkerRel()))
		gitkit.CommitFile(t, h.BoardDir(), loomDefault, "review_max_bounces: 3\n", "fixture: a template-equal unit and no marker")
		if key, found := stampKey(t, geom); !found || key != runningKey() {
			t.Fatalf("stamp = (%q, %v) before the third walk; want the running key", key, found)
		}
		if err := hubreconcile.Ensure(geom, hubreconcile.Options{}); err != nil {
			t.Fatalf("third Ensure: %v", err)
		}
		if got, err := os.ReadFile(filepath.Join(h.BoardDir(), loomDefault)); err != nil || string(got) != configengine.SparseBaseHeader {
			t.Errorf("board loom.yaml = %q, %v after the walk a removed marker triggered; want only the sparse header", got, err)
		}
		if !slices.Contains(gitkit.LsFiles(t, h.BoardDir()), filepath.ToSlash(configengine.BaseSparseMarkerRel())) {
			t.Errorf("the board does not track the marker after the walk a removed marker triggered")
		}
	}
}

// assertBoardStrippedOnce checks the board a walk stripped once.
// Every non-seed-only base file other than lenses holds only the sparse header, bar darn's and fabric's.
// darn's keeps the hub's tuned verify_attempts, and fabric's keeps the running build's key.
// The seed-only models.yaml is byte for byte as seeded.
// The marker is tracked in the walk's board commit.
// The board's loom base resolves the template's lens fan and its one webster bounce.
// lenses.yaml is skipped because the declared key move from burler.yaml gives it the hub's tuned lens.
func assertBoardStrippedOnce(t *testing.T, h *hubforge.Hub, modelsBefore []byte) {
	t.Helper()

	for _, module := range configreg.Modules() {
		if module.SeedOnly || module.Name == "lenses" {
			continue
		}
		got, err := os.ReadFile(configengine.ConfigFile(h.BoardDir(), module.Name))
		if err != nil {
			t.Errorf("read the board's %s.yaml: %v", module.Name, err)
			continue
		}
		if module.Name == "darn" {
			if !strings.HasPrefix(string(got), "verify_attempts: 5") || strings.Count(string(got), "\n") != 1 {
				t.Errorf("board darn.yaml = %q; want only the hub's tuned verify_attempts", got)
			}
			continue
		}
		if module.Name == "fabric" {
			if string(got) != newerHubWideKey+"\n" {
				t.Errorf("board fabric.yaml = %q; want only the newer binary's %q", got, newerHubWideKey)
			}
			continue
		}
		if string(got) != configengine.SparseBaseHeader {
			t.Errorf("board %s.yaml = %q; want only the sparse header", module.Name, got)
		}
	}
	if got, err := os.ReadFile(configengine.ConfigFile(h.BoardDir(), "models")); err != nil || string(got) != string(modelsBefore) {
		t.Errorf("board models.yaml = %q, %v; want it byte for byte as seeded", got, err)
	}
	marker := filepath.ToSlash(configengine.BaseSparseMarkerRel())
	if !slices.Contains(gitkit.LsFiles(t, h.BoardDir()), marker) {
		t.Errorf("the board does not track %s after the walk", marker)
	}
	if committed := gitkit.Git(t, h.BoardDir(), "log", "-1", "--name-only", "--format="); !strings.Contains(committed, marker) {
		t.Errorf("the walk's board commit holds %q; want it to carry %s", committed, marker)
	}
	// The load stacks the board's base alone:
	// the prime's own loom.yaml holds a review budget past the template's hard cap, which no load of the full stack accepts.
	cfg, err := loomengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "loom")
	if err != nil {
		t.Fatalf("loomengine.LoadConfig over the board's base: %v", err)
	}
	if cfg.DiscussionLenses != "twill-discussion" || cfg.CodeMaxBounces == nil || *cfg.CodeMaxBounces != 1 {
		t.Errorf("loom resolves DiscussionLenses %q and CodeMaxBounces %v; want twill-discussion and 1", cfg.DiscussionLenses, cfg.CodeMaxBounces)
	}
}

func TestEnsure_CurrentStampLeavesConfigAlone(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := newStaleHub(t)
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
			pairLoom := configengine.ConfigFile(pair, "loom")
			if got, err := os.ReadFile(pairLoom); err != nil || !strings.Contains(string(got), `discussion_lenses: ""`) {
				t.Errorf("mid-merge pair loom.yaml = %q (err %v); want its former discussion_lenses kept after the migrating walk", got, err)
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
			if got, err := os.ReadFile(pairLoom); err != nil || strings.Contains(string(got), "discussion_lenses") {
				t.Errorf("pair loom.yaml = %q (err %v); want the former discussion_lenses dropped by the walk that ran with the board marker present", got, err)
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
	tiergate.Require(t, 1)
	t.Parallel()

	h := newStaleHub(t)
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
		// afterRepair, when set, checks the pair after the repaired walk.
		afterRepair func(t *testing.T, h *hubforge.Hub)
		// unmigratedBoard pins that the failed walk leaves the board's config files at their prior bytes.
		// The board's marker stays unwritten.
		unmigratedBoard bool
		// wantInMessage is text the error message must hold; empty skips the check.
		wantInMessage string
		// wantAbsentFromMessage is text the error message must not hold; empty skips the check.
		wantAbsentFromMessage string
		// ensure runs the walk; nil runs the default walk.
		ensure func(geom hubreconcile.Geometry) error
	}{
		{
			name: "a declared key move is undone with the failed walk and committed by the repaired one",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				records := h.PairRecordsSibling("pair-a")
				// The stale pair holds every module's template; removing its claude.yaml leaves the key move a file to create.
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
				// A darn.yaml holding a retired key, which precedes fabric.yaml in the registry, is written before the fabric failure.
				darn, _ := configreg.Lookup("darn")
				gitkit.CommitFile(t, h.BoardDir(), configengine.ConfigFileRel("darn"), darn.Template()+"verify: make verify\n", "fixture: stale darn.yaml")
				hubforge.SeedFabricConfig(t, h, "a: [unclosed\n")
				return h.BoardDir(), h.BoardDir(), configengine.ConfigFile(h.BoardDir(), "fabric")
			},
			module: "fabric",
			repair: func(t *testing.T, h *hubforge.Hub) {
				m, _ := configreg.Lookup("fabric")
				hubforge.SeedFabricConfig(t, h, m.Template())
			},
		},
		{
			name: "unparseable hub-wide config on an unmigrated board names the default-layer command",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				commitPreMigrationBase(t, h.BoardDir())
				hubforge.SeedFabricConfig(t, h, "a: [unclosed\n")
				return h.BoardDir(), h.BoardDir(), configengine.ConfigFile(h.BoardDir(), "fabric")
			},
			module:          "fabric",
			unmigratedBoard: true,
			wantInMessage:   "lym config fabric --layer default",
			repair: func(t *testing.T, h *hubforge.Hub) {
				m, _ := configreg.Lookup("fabric")
				hubforge.SeedFabricConfig(t, h, m.Template())
			},
		},
		{
			name: "a failure after the hub-wide files, the marker and a template base were written restores the files and removes the marker and the base",
			breakConfig: func(t *testing.T, h *hubforge.Hub) (string, string, string) {
				commitPreMigrationBase(t, h.BoardDir())
				// The profiles template base is dropped from the board, so the walk writes it before the failure.
				// The restore must remove it again.
				gitkit.Git(t, h.BoardDir(), "rm", "-q", "--ignore-unmatch", "--", filepath.ToSlash(configengine.TemplateBaseFileRel("profiles")))
				gitkit.Git(t, h.BoardDir(), "commit", "-q", "--allow-empty", "-m", "fixture: no profiles template base")
				return h.BoardDir(), h.BoardDir(), h.BoardDir()
			},
			unmigratedBoard: true,
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
			if tc.unmigratedBoard {
				if boardFilesAfter := configFilesOf(t, h.BoardDir()); !maps.Equal(boardFilesBefore, boardFilesAfter) {
					t.Errorf("the failed walk changed the board's config files:\nbefore: %v\nafter: %v", boardFilesBefore, boardFilesAfter)
				}
				if _, statErr := os.Stat(configengine.BaseSparseMarker(h.BoardDir())); !os.IsNotExist(statErr) {
					t.Errorf("the failed walk left the board marker behind (stat err = %v)", statErr)
				}
				for _, module := range configreg.Modules() {
					for _, change := range module.Reinterpretations {
						if _, statErr := os.Stat(configengine.ReinterpretedMarker(h.BoardDir(), change.ID)); !os.IsNotExist(statErr) {
							t.Errorf("the failed walk left the reinterpretation marker %s behind (stat err = %v)", change.ID, statErr)
						}
					}
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

func TestEnsure_ConcurrentCallsLandOneCommitPerBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := newStaleHub(t)
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
	tiergate.Require(t, 1)
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
	seedRetiredHubWideKey(t, h)
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
	seedRetiredHubWideKey(t, h)
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
	const lightDarn = "  darn:\n    loom: {code_max_bounces: 1, code_hard_max_bounces: 2}\n"
	const lightDarnSolo = "  darn:\n    loom: {code_max_bounces: 1, code_hard_max_bounces: 2, code_reviewer_fan: \"\"}\n"
	const solidDarn = "  darn:\n    loom: {code_max_bounces: 2, code_hard_max_bounces: 3}\n"
	const solidDarnBase = "  darn:\n    loom: {code_max_bounces: 3, code_hard_max_bounces: 4}\n"
	const solidDarnTuned = "  darn:\n    loom: {code_max_bounces: 5, code_hard_max_bounces: 4}\n"
	const solidDarnFollowed = "  darn:\n    loom: {code_max_bounces: 5, code_hard_max_bounces: 3}\n"
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
	seedRetiredHubWideKey(t, h)
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
	// The first walk of a hub also commits each reinterpretation marker and each seed-only module's template base.
	// These are no config files.
	var changed []string
	for _, path := range strings.Fields(gitkit.Git(t, board, "show", "--name-only", "--format=", "HEAD")) {
		if !strings.Contains(path, "reinterpreted-") && !strings.HasSuffix(path, ".template-base") {
			changed = append(changed, path)
		}
	}
	if len(changed) != 1 {
		t.Fatalf("the hub-wide commit changed %v besides the reinterpretation markers; want one config file", changed)
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

func TestEnsure_PairCallReconcilesDespiteAFreshStamp(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := newStaleHub(t)
	geom := geometryOf(h)
	writeCurrentStamp(t, geom)
	pair := filepath.Join(h.Path, "pair-a")

	if err := hubreconcile.Ensure(geom, hubreconcile.Options{Pair: pair}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if retiredKeyPresent(t, pair) {
		t.Errorf("the pair still carries %s", retiredKey)
	}
	if got, err := os.ReadFile(configengine.ConfigFile(pair, "loom")); err != nil || strings.Contains(string(got), "discussion_lenses") {
		t.Errorf("pair loom.yaml = %q (err %v); want the former discussion_lenses dropped on a board holding the marker", got, err)
	}
	assertClean(t, h.PairRecordsSibling("pair-a"))
	if !retiredKeyPresent(t, h.PrimeWorktree()) {
		t.Errorf("the pair call reconciled the prime as well")
	}
}
