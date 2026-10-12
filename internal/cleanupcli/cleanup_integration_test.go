//go:build integration

// cleanup_integration_test.go drives "lyx cleanup" through RunCLIIn over a hubforge hub seeded with one leftover for every segment: the prime-only refusal, the dry run that lists them all and the apply that removes them.

package cleanupcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func writeAged(t *testing.T, path, content string, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	modTime := time.Now().Add(-age)
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestCleanupVerb_RefusesOutsideThePrimeThenListsAndThenRemovesEverySeededLeftover(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	const (
		runSlug  = "finished-run"
		taskSlug = "finished-task"
	)

	// batten: a done run directory, committed and pushed on the records side.
	status, err := json.Marshal(shedengine.Status{CurrentProducer: "Worktree-Teardown", State: shedengine.StateDone, History: []shedengine.HistoryEntry{}})
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	writeAged(t, shedrun.StatusFile(h.Location, runSlug), string(status), 0)
	writeAged(t, shedrun.SeedFile(h.Location, runSlug), "{}", 0)
	rels := []string{shedrun.SeedRel(h.Location, runSlug), shedrun.StatusRel(h.Location, runSlug)}
	if _, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), h.Location, rels, "test: finished run", fabricengine.EnvSyncOptions()); err != nil || !committed {
		t.Fatalf("commit the finished run: committed=%v err=%v", committed, err)
	}
	if _, err := fabricengine.PushAnchored(h.Location, fabricengine.EnvSyncOptions(), fabricengine.StatusPushLockWait); err != nil {
		t.Fatalf("push the finished run: %v", err)
	}
	runDir := shedrun.RunDir(h.Location, runSlug)

	// board: a done entry.
	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub board: %v", err)
	}
	if _, err := board.UpsertTask(map[string]any{"slug": taskSlug, "title": taskSlug, "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the board entry: %v", err)
	}
	done := "done"
	if err := board.SetStatus(taskSlug, &done); err != nil {
		t.Fatalf("mark the board entry done: %v", err)
	}

	// orch: the handoff the state names, and an old one beside it.
	orchPaths := orchcli.PrimePaths(h.Location)
	if err := orchengine.SaveState(orchPaths, orchengine.State{Phase: orchengine.PhaseIdle, LastHandoff: "keep.md"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	keptHandoff := filepath.Join(orchPaths.HandoffsDir, "keep.md")
	oldHandoff := filepath.Join(orchPaths.HandoffsDir, "old.md")
	writeAged(t, keptHandoff, "note", 0)
	writeAged(t, oldHandoff, "note", 0)

	// scratch: an old child of the prime's .scratch directory.
	scratchChild := filepath.Join(h.PrimeWorktree(), scratchDirName, "old-thing")
	writeAged(t, filepath.Join(scratchChild, "file.txt"), "x", 30*24*time.Hour)
	oldScratch := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(scratchChild, oldScratch, oldScratch); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// logs: a trace group older than a one-day bound and younger than the default.
	anchor := h.Location.AnchorPath()
	writeAged(t, loggerconfig.ConfigPath(anchor), "trace_retention_count: 100\ntrace_retention_days: 1\n", 0)
	traceFile := filepath.Join(logger.LogsDirAt(anchor), "trace-20260101T000000Z-0123456789abcdef-2147483.log")
	writeAged(t, traceFile, "trace", 3*24*time.Hour)

	assertLeftoversRemain := func(t *testing.T, step string) {
		t.Helper()
		for _, path := range []string{runDir, oldHandoff, keptHandoff, scratchChild, traceFile} {
			if !exists(path) {
				t.Errorf("%s: %s was removed", step, path)
			}
		}
		if _, found, err := board.GetTask(taskSlug); err != nil || !found {
			t.Errorf("%s: board entry found=%v err=%v, want it kept", step, found, err)
		}
	}

	run := func(cwd string, args ...string) (envelope.Envelope, int) {
		var out bytes.Buffer
		code := RunCLIIn(cwd, &out, args)
		return envelope.Decode(t, out.String()), code
	}

	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}
	refused, code := run(h.PairCodeWorktree(hubforge.TemplatePairSlug), "--apply")
	if refused.OK || code == 0 || !bytes.Contains([]byte(refused.Error), []byte(primeName)) {
		t.Errorf("cleanup from the task worktree: ok=%v code=%d error=%q, want a refusal naming %q", refused.OK, code, refused.Error, primeName)
	}
	assertLeftoversRemain(t, "refusal")

	dry, code := run(h.PrimeWorktree())
	if !dry.OK || code != 0 || dry.Raw["applied"] != false {
		t.Fatalf("dry run: ok=%v code=%d envelope=%v", dry.OK, code, dry.Raw)
	}
	var names []string
	listed := map[string]map[string]any{}
	for _, item := range dry.Raw["segments"].([]any) {
		segment := item.(map[string]any)
		names = append(names, segment["name"].(string))
		listed[segment["name"].(string)] = segment["listed"].(map[string]any)
	}
	if want := []string{"batten", "board", "orch", "scratch", "logs", "temp"}; !slices.Equal(names, want) {
		t.Errorf("dry run segments = %v, want %v", names, want)
	}
	if removed := listed["logs"]["removed"].([]any); len(removed) != 1 || removed[0] != traceFile {
		t.Errorf("dry run logs segment lists %v, want the seeded trace group %s", removed, traceFile)
	}
	assertLeftoversRemain(t, "dry run")

	applied, code := run(h.PrimeWorktree(), "--apply")
	if !applied.OK || code != 0 || applied.Raw["applied"] != true {
		t.Fatalf("apply: ok=%v code=%d error=%q envelope=%v", applied.OK, code, applied.Error, applied.Raw)
	}
	for _, path := range []string{runDir, oldHandoff, scratchChild, traceFile} {
		if exists(path) {
			t.Errorf("--apply left %s", path)
		}
	}
	if !exists(keptHandoff) {
		t.Errorf("--apply removed the handoff the state names")
	}
	if _, found, err := board.GetTask(taskSlug); err != nil || found {
		t.Errorf("--apply left the done board entry: found=%v err=%v", found, err)
	}
}
