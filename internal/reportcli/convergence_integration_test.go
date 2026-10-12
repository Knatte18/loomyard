//go:build integration

// convergence_integration_test.go drives `lyx report convergence` through RunCLIIn over a hubforge hub: the prime-only refusals, the empty report before any tag, and the report of one finished run seeded the way a landing leaves it.

package reportcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reportengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
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

func TestConvergence_RefusesOutsideThePrimeThenReportsTheSeededRun(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	slug := hubforge.TemplatePairSlug

	run := func(cwd string, args ...string) (string, int) {
		var out bytes.Buffer
		code := RunCLIIn(cwd, &out, append([]string{"convergence"}, args...))
		return out.String(), code
	}
	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}
	recordsRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("records repo root: %v", err)
	}

	out, code := run(recordsRoot)
	if code != 1 {
		t.Errorf("from the prime's records-side worktree: exit %d, output %s; want a refusal with exit 1", code, out)
	}
	envelope.RequireErr(t, out, "prime")

	out, code = run(h.PairCodeWorktree(slug))
	if code != 1 {
		t.Errorf("from the pair's worktree: exit %d, output %s; want a refusal with exit 1", code, out)
	}
	envelope.RequireErr(t, out, primeName)

	out, code = run(h.PrimeWorktree())
	empty := envelope.RequireOK(t, out)
	if runs, _ := empty.Raw["runs"].([]any); code != 0 || len(runs) != 0 {
		t.Fatalf("from the prime before any tag: exit %d, runs %v; want exit 0 and no runs", code, empty.Raw["runs"])
	}

	// The board entry and the batten record come first: both must be committed no later than the task commit and the tag.
	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub board: %v", err)
	}
	if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the board entry: %v", err)
	}
	if gitkit.GitStatusPorcelain(t, h.BoardDir()) != "" {
		gitkit.Git(t, h.BoardDir(), "add", "-A")
		gitkit.Git(t, h.BoardDir(), "commit", "-m", "board: seed the entry")
	}

	battenStatus := shedengine.Status{State: shedengine.StateDone, CurrentProducer: battenrecipe.NameWorktreeTeardown, History: []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameWorktreeCreate, Outcome: shedengine.Done},
		{Producer: battenrecipe.NameLand, Outcome: shedengine.Done},
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

	pairLocation, err := lyxcwd.Resolve(pairCode)
	if err != nil {
		t.Fatalf("resolve the pair: %v", err)
	}
	loomStatus := shedengine.Status{State: shedengine.StateDone, History: []shedengine.HistoryEntry{{Producer: loomshed.NameFinalize, Outcome: shedengine.Done}}}
	writeFile(t, shedrun.SeedFile(pairLocation, slug), fmt.Sprintf(`{"recipe":%q,"driver":"go"}`, shedrun.RecipeLoom))
	writeFile(t, shedrun.StatusFile(pairLocation, slug), marshalStatus(t, loomStatus))
	writeFile(t, loomengine.DiscussionDecisionRecord(pairLocation), "# Decision record\n\n## Decisions\n\n### Added after Discussion (alice): a call with no date\n")
	fabric, err := fabricengine.Open(pairLocation)
	if err != nil {
		t.Fatalf("open the pair's fabric: %v", err)
	}
	finished := []string{shedrun.SeedRel(pairLocation, slug), shedrun.StatusRel(pairLocation, slug), loomengine.DiscussionDecisionRecordRel()}
	if _, err := fabric.Commit(finished, "records: the finished run", nil, fabricengine.SyncOptions{SkipPush: true}); err != nil {
		t.Fatalf("commit the finished run: %v", err)
	}

	pairRecords := h.PairRecordsSibling(slug)
	tipSHA := gitkit.Git(t, pairRecords, "rev-parse", "HEAD")
	gitkit.Git(t, pairRecords, "tag", fabricengine.ArchiveTagPrefix+slug+"/"+tipSHA[:12], tipSHA)
	gitkit.Git(t, h.PrimeWorktree(), "merge", "--ff-only", taskCommit)

	before := time.Now().UTC()
	out, code = run(h.PrimeWorktree(), "--fresh-days", "1")
	after := time.Now().UTC()
	report := envelope.RequireOK(t, out)
	if code != 0 {
		t.Fatalf("report from the prime: exit %d, output %s", code, out)
	}

	selection, _ := report.Raw["selection"].(map[string]any)
	wantSinces := map[string]bool{}
	for _, day := range []time.Time{before, after} {
		date := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		wantSinces[date.AddDate(0, 0, -84).Format(time.RFC3339)] = true
	}
	if since, _ := selection["since"].(string); !wantSinces[since] {
		t.Errorf("selection.since = %q; want one of %v", since, wantSinces)
	}

	runs, _ := report.Raw["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("runs = %v; want the one seeded run", report.Raw["runs"])
	}
	row, _ := runs[0].(map[string]any)
	if sources, _ := row["sources"].(map[string]any); sources[reportengine.SourceBatten] != true {
		t.Errorf("run sources = %v; want the batten source present", row["sources"])
	}

	var landed []map[string]any
	landings, _ := report.Raw["landings"].([]any)
	for _, item := range landings {
		landing, _ := item.(map[string]any)
		if landing["slug"] == slug {
			landed = append(landed, landing)
		}
	}
	if len(landed) != 1 || landed[0]["kind"] != reportengine.LandingKindFix || landed[0]["join"] != reportengine.JoinExact || landed[0]["commit"] != taskCommit {
		t.Errorf("landings of %s = %v; want one fix joined exactly to %s", slug, landed, taskCommit)
	}
	if mapped, _ := row["landing"].(map[string]any); mapped["commit"] != taskCommit {
		t.Errorf("run landing = %v; want commit %s", row["landing"], taskCommit)
	}

	findings, _ := report.Raw["findings"].([]any)
	if len(findings) != 1 || !strings.Contains(fmt.Sprint(findings[0]), filepath.ToSlash(loomengine.DiscussionDecisionRecordRel())) {
		t.Errorf("findings = %v; want exactly one, naming the decision record", findings)
	}

	out, code = run(h.PrimeWorktree(), "--fresh-days", "1", "--format", "markdown")
	if code != 0 {
		t.Fatalf("markdown report: exit %d, output %s", code, out)
	}
	for _, heading := range []string{reportengine.HeadingWakes, reportengine.HeadingStops, reportengine.HeadingFreshFixes, reportengine.HeadingRounds} {
		if !strings.Contains(out, heading) {
			t.Errorf("markdown report lacks the heading %q:\n%s", heading, out)
		}
	}
}
