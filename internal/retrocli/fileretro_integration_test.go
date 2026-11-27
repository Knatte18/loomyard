// fileretro_integration_test.go drives `lym retro` through clihelp.ExecuteIn over a hubforge hub.
// It covers the prime-only and missing-scripts refusals, the parsed window, and the filing and release of the task.
// It also covers the duplicate-slug refusal, and the last retro read from both the board and the report directories.

package retrocli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/retrocli"
	"github.com/Knatte18/loomyard/internal/retroengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func marshalStatus(t *testing.T, status shedengine.Status) string {
	t.Helper()
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	return string(data)
}

// seedLandedRun lands the hub's template pair as a finished run, the way a landing leaves it.
// It returns the landing commit's time.
func seedLandedRun(t *testing.T, h *hubforge.Hub, slug string) time.Time {
	t.Helper()

	runStart := time.Now().UTC().Truncate(time.Second).Add(-3 * time.Hour)
	stamp := func(hours int) string { return runStart.Add(time.Duration(hours) * time.Hour).Format(time.RFC3339) }

	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub board: %v", err)
	}
	if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the board entry: %v", err)
	}
	commitBoardIfDirty(t, h, "board: seed the entry")

	battenStatus := shedengine.Status{State: shedengine.StateDone, CurrentProducer: battenrecipe.NameWorktreeTeardown, History: []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameWorktreeCreate, Outcome: shedengine.Done, At: stamp(0)},
		{Producer: battenrecipe.NameLand, Outcome: shedengine.Done, At: stamp(2)},
	}}
	writeFile(t, shedrun.StatusFile(h.Location, slug), marshalStatus(t, battenStatus))
	if _, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), h.Location, []string{shedrun.StatusRel(h.Location, slug)}, "test: batten record", fabricengine.SyncOptions{SkipPush: true}); err != nil || !committed {
		t.Fatalf("commit the batten record: committed=%v err=%v", committed, err)
	}

	pairCode := h.PairCodeWorktree(slug)
	writeFile(t, filepath.Join(pairCode, "fixed.txt"), "fixed\n")
	gitkit.Git(t, pairCode, "add", "fixed.txt")
	gitkit.Git(t, pairCode, "commit", "-m", "fix: repair the thing")
	taskCommit := gitkit.Git(t, pairCode, "rev-parse", "HEAD")

	pairLocation, err := lymcwd.Resolve(pairCode)
	if err != nil {
		t.Fatalf("resolve the pair: %v", err)
	}
	loomStatus := shedengine.Status{State: shedengine.StateDone, History: []shedengine.HistoryEntry{{Producer: loomshed.NameFinalize, Outcome: shedengine.Done, At: stamp(1)}}}
	writeFile(t, shedrun.SeedFile(pairLocation, slug), fmt.Sprintf(`{"recipe":%q,"driver":"go","started_at":%q}`, shedrun.RecipeTapestry, stamp(0)))
	writeFile(t, shedrun.StatusFile(pairLocation, slug), marshalStatus(t, loomStatus))
	fabric, err := fabricengine.Open(pairLocation)
	if err != nil {
		t.Fatalf("open the pair's fabric: %v", err)
	}
	finished := []string{shedrun.SeedRel(pairLocation, slug), shedrun.StatusRel(pairLocation, slug)}
	if _, err := fabric.Commit(finished, "records: the finished run", nil, fabricengine.SyncOptions{SkipPush: true}); err != nil {
		t.Fatalf("commit the finished run: %v", err)
	}

	pairRecords := h.PairRecordsSibling(slug)
	tipSHA := gitkit.Git(t, pairRecords, "rev-parse", "HEAD")
	gitkit.Git(t, pairRecords, "tag", fabriclayout.ArchiveTagPrefix+slug+"/"+tipSHA[:12], tipSHA)
	gitkit.Git(t, h.PrimeWorktree(), "merge", "--ff-only", taskCommit)

	seconds, err := strconv.ParseInt(gitkit.Git(t, h.PrimeWorktree(), "log", "-1", "--format=%ct"), 10, 64)
	if err != nil {
		t.Fatalf("parse the landing time: %v", err)
	}
	return time.Unix(seconds, 0).UTC()
}

// commitBoardIfDirty commits what a test wrote into the hub's board checkout, so the next board write starts clean.
func commitBoardIfDirty(t *testing.T, h *hubforge.Hub, message string) {
	t.Helper()
	if gitkit.GitStatusPorcelain(t, h.BoardDir()) != "" {
		gitkit.Git(t, h.BoardDir(), "add", "-A")
		gitkit.Git(t, h.BoardDir(), "commit", "-m", message)
	}
}

func TestRetro_FilesAndReleasesTheTaskOfALandedRun(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	slug := hubforge.TemplatePairSlug
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{slug}})
	prime := h.PrimeWorktree()
	// The hub's own board vocabulary lists the retro task's type label.
	boltDir := fabriclayout.BoltDir(h.Path)
	writeFile(t, configengine.ConfigFile(boltDir, "board"), "types: [bug, "+retroengine.TaskLabel+"]\n")
	if _, _, err := fabricengine.NewBolt(boltDir).Commit("test: the board vocabulary", fabricengine.SyncOptions{SkipPush: true}); err != nil {
		t.Fatalf("commit the board vocabulary: %v", err)
	}
	run := func(cwd string, args ...string) (string, int) {
		var out bytes.Buffer
		code := clihelp.ExecuteIn(retrocli.Command(), cwd, &out, args)
		return out.String(), code
	}
	requireRefused := func(cwd string, want []string, args ...string) {
		t.Helper()
		out, code := run(cwd, args...)
		if code != 1 {
			t.Errorf("retro %v from %s: exit %d, output %s; want a refusal with exit 1", args, cwd, code, out)
		}
		for _, text := range want {
			envelope.RequireErr(t, out, text)
		}
	}

	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}
	requireRefused(h.PairCodeWorktree(slug), []string{"prime worktree only", primeName})
	requireRefused(prime, []string{retroengine.ToolsDirRel})

	if err := os.MkdirAll(filepath.Join(prime, filepath.FromSlash(retroengine.ToolsDirRel)), 0o755); err != nil {
		t.Fatalf("create the scripts directory: %v", err)
	}
	stencilkit.SeedInto(t, fabriclayout.StencilsDir(h.Path))
	commitBoardIfDirty(t, h, "board: seed the stencils")
	landed := seedLandedRun(t, h, slug)
	retroSlug := retroengine.SlugPrefix + landed.Format("20060102-150405")

	// A window of two days in the year 2000 holds no landing, and its refusal names both parsed ends.
	requireRefused(prime, []string{"no run landed after 2000-01-01T00:00:00Z and up to 2000-01-02T00:00:00Z"}, "--since", "2000-01-01", "--until", "2000-01-02")

	out, code := run(prime, slug)
	filed := envelope.RequireOK(t, out)
	if code != 0 {
		t.Fatalf("retro %s: exit %d, output %s", slug, code, out)
	}
	for key, want := range map[string]any{
		"slug":       retroSlug,
		"runs":       []any{slug},
		"from":       landed.Add(-time.Second).Format(time.RFC3339),
		"until":      landed.Format(time.RFC3339),
		"report_dir": retroengine.ReportDirRel + "/" + retroSlug,
	} {
		if !reflect.DeepEqual(filed.Raw[key], want) {
			t.Errorf("envelope %s = %v; want %v", key, filed.Raw[key], want)
		}
	}

	tasks, err := boardengine.ReadTasks(h.BoardDir())
	if err != nil {
		t.Fatalf("read the board: %v", err)
	}
	var task *boardengine.Task
	for i := range tasks {
		if tasks[i].Slug == retroSlug {
			task = &tasks[i]
		}
	}
	if task == nil {
		t.Fatalf("the board holds no entry %s: %+v", retroSlug, tasks)
	}
	if task.Status != nil {
		t.Errorf("task status = %q; want it released, a null status", *task.Status)
	}
	if task.Recipe != shedrun.RecipeDarn || task.Profile != retroengine.TaskProfile || !reflect.DeepEqual(task.Labels, []string{retroengine.TaskLabel}) {
		t.Errorf("task recipe, profile, labels = %q, %q, %v; want a darn task on the %s profile labelled %s",
			task.Recipe, task.Profile, task.Labels, retroengine.TaskProfile, retroengine.TaskLabel)
	}
	if !strings.Contains(task.Body, "- `"+slug+"`, landed "+landed.Format(time.RFC3339)) {
		t.Errorf("task body does not list the landed run:\n%s", task.Body)
	}

	requireRefused(prime, []string{"already holds an entry " + retroSlug}, slug)

	// The board's retro task is the last retro, and the run it covers is not after it.
	requireRefused(prime, []string{"no run landed after " + landed.Format(time.RFC3339)})

	// A newer report directory is the last retro over the older board entry.
	if err := os.MkdirAll(filepath.Join(prime, filepath.FromSlash(retroengine.ReportDirRel), "retro-29990101-000000"), 0o755); err != nil {
		t.Fatalf("create a report directory: %v", err)
	}
	requireRefused(prime, []string{"no run landed after 2999-01-01T00:00:00Z"})
}
