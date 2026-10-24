//go:build tier1

// cleanup_integration_test.go drives "lyx cleanup" through RunCLIIn over a hubforge hub seeded with one leftover for every segment: the prime-only refusal, the dry run that lists them all and the apply that removes them.

package cleanupcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
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
	boardkit.InstallReplay(t)

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

	// scratch: an old child of the prime's .scratch directory, which fabric wires as a link into the pair's records.
	scratchRoot := filepath.Join(h.PrimeWorktree(), scratchDirName)
	assertScratchLink := func(t *testing.T, step, wantTarget string) string {
		t.Helper()
		isLink, err := fslink.IsLink(scratchRoot)
		if err != nil || !isLink {
			t.Fatalf("%s: %s is not a link: isLink=%v err=%v", step, scratchRoot, isLink, err)
		}
		rawTarget, err := fslink.RawTarget(scratchRoot)
		if err != nil {
			t.Fatalf("%s: RawTarget(%s): %v", step, scratchRoot, err)
		}
		if wantTarget != "" && rawTarget != wantTarget {
			t.Errorf("%s: %s points at %q, want it unchanged at %q", step, scratchRoot, rawTarget, wantTarget)
		}
		return rawTarget
	}
	scratchTarget := assertScratchLink(t, "before the run", "")
	scratchChild := filepath.Join(scratchRoot, "old-thing")
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
	requireEmptyRecord(t, "dry run", dry, false)
	requireList(t, dry, "failed_pushes")

	// The other machine moved Bolt while this machine's own board commits are unpushed.
	boardDir := h.BoardDir()
	gitkit.CommitFile(t, boardDir, "notes/local.md", "from this machine\n", "local note")
	originCommit(t, h.RecordsBare, "main", "other-machine/notes.md", "from the other machine\n")
	boltBefore := gitkit.Git(t, boardDir, "for-each-ref", "--format=%(refname) %(objectname)")
	headBefore := gitkit.RevParse(t, boardDir, "HEAD")
	diverged, code := run(h.PrimeWorktree())
	if !diverged.OK || code != 0 {
		t.Fatalf("dry run over a diverged Bolt: ok=%v code=%d error=%q", diverged.OK, code, diverged.Error)
	}
	if state := reconcileBranch(t, diverged, "bolt", "main")["State"]; state != string(fabricengine.BranchNeedsReplay) {
		t.Errorf("dry run Bolt state = %v; want needs_replay", state)
	}
	requireEmptyRecord(t, "dry run over a diverged Bolt", diverged, false)
	requireList(t, diverged, "failed_pushes")
	if got := gitkit.Git(t, boardDir, "for-each-ref", "--format=%(refname) %(objectname)"); got != boltBefore || gitkit.RevParse(t, boardDir, "HEAD") != headBefore {
		t.Errorf("the dry run moved a ref of Bolt:\n%s\nwant:\n%s", got, boltBefore)
	}
	assertLeftoversRemain(t, "dry run over a diverged Bolt")

	// A tracked change on a path the Bolt replay touches stops --apply before any segment.
	dirtyPath := filepath.Join(boardDir, "notes", "dirty.md")
	gitkit.CommitFile(t, boardDir, "notes/dirty.md", "committed\n", "local notes")
	if err := os.WriteFile(dirtyPath, []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("dirty the Bolt checkout: %v", err)
	}
	dirty, code := run(h.PrimeWorktree(), "--apply")
	boltStop := reconcileBranch(t, dirty, "bolt", "main")
	boltWayForward, _ := boltStop["WayForward"].(string)
	if dirty.OK || code == 0 || boltStop["State"] != string(fabricengine.BranchDirty) || boltWayForward == "" ||
		!strings.Contains(dirty.Error, boltWayForward+"; then re-run lyx cleanup --apply from the prime") {
		t.Errorf("--apply over a dirty Bolt: ok=%v code=%d error=%q stop=%v; want a refusal naming the stop's way forward and the re-entry", dirty.OK, code, dirty.Error, boltStop)
	}
	if dirty.Raw["segments"] != nil {
		t.Errorf("--apply over a dirty Bolt ran segments: %v", dirty.Raw["segments"])
	}
	requireList(t, dirty, "failed_pushes")
	assertLeftoversRemain(t, "--apply over a dirty Bolt")
	gitkit.Git(t, boardDir, "checkout", "--", "notes/dirty.md")

	// A both-sides change of the prime's records is replayed to a conflict and stops --apply, the Bolt replay having run before it.
	primeRecords := h.PrimeRecords()
	recordsBranch := gitkit.CurrentBranch(t, primeRecords)
	gitkit.CommitFile(t, primeRecords, "conflict.md", "local\n", "local records change")
	originCommit(t, h.RecordsBare, recordsBranch, "conflict.md", "origin\n")
	conflict, code := run(h.PrimeWorktree(), "--apply")
	recordsStop := reconcileBranch(t, conflict, "records", recordsBranch)
	recordsWayForward, _ := recordsStop["WayForward"].(string)
	if conflict.OK || code == 0 || recordsStop["State"] != string(fabricengine.BranchDiverged) || recordsWayForward == "" ||
		!strings.Contains(conflict.Error, recordsWayForward) || !strings.Contains(recordsWayForward, "lyx fabric push") {
		t.Errorf("--apply over a both-sides records change: ok=%v code=%d error=%q stop=%v; want a refusal naming the conflict's way forward", conflict.OK, code, conflict.Error, recordsStop)
	}
	if conflict.Raw["segments"] != nil || conflict.Partial == nil || !*conflict.Partial {
		t.Errorf("--apply over a both-sides records change: segments=%v partial=%v; want no segment and partial, since the Bolt replay ran", conflict.Raw["segments"], conflict.Partial)
	}
	assertLeftoversRemain(t, "--apply over a both-sides records change")
	gitkit.Git(t, primeRecords, "fetch", "origin")
	gitkit.Git(t, primeRecords, "reset", "--hard", "origin/"+recordsBranch)

	// An origin that holds no repository is no transport failure: --apply refuses naming the error, and the dry run prints it and runs the segments.
	recordsRepo, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("records repository root: %v", err)
	}
	restoreOrigin := setOrigin(t, recordsRepo, filepath.Join(t.TempDir(), "no-repository"))
	broken, code := run(h.PrimeWorktree(), "--apply")
	if broken.OK || code == 0 || broken.Raw["segments"] != nil || !strings.Contains(broken.Error, "origin reconcile failed") || !strings.Contains(broken.Error, "no-repository") {
		t.Errorf("--apply over a broken origin: ok=%v code=%d error=%q segments=%v; want a refusal naming the error and no segment", broken.OK, code, broken.Error, broken.Raw["segments"])
	}
	assertLeftoversRemain(t, "--apply over a broken origin")
	brokenDry, code := run(h.PrimeWorktree())
	if reconcileError, _ := brokenDry.Raw["reconcile_error"].(string); !brokenDry.OK || code != 0 || !strings.Contains(reconcileError, "no-repository") || len(requireList(t, brokenDry, "segments")) == 0 {
		t.Errorf("dry run over a broken origin: ok=%v code=%d envelope=%v; want the error printed and the segments run", brokenDry.OK, code, brokenDry.Raw)
	}
	restoreOrigin()

	// A refused origin is unreachable: both modes warn and run the segments, and --apply removes every leftover.
	restoreOrigin = setOrigin(t, recordsRepo, "http://127.0.0.1:1/x")
	unreachableDry, code := run(h.PrimeWorktree())
	if warning, _ := unreachableDry.Raw["warning"].(string); !unreachableDry.OK || code != 0 || !strings.Contains(warning, "unreachable") || len(requireList(t, unreachableDry, "segments")) == 0 {
		t.Errorf("dry run over an unreachable origin: ok=%v code=%d envelope=%v; want the unreachable warning and the segments", unreachableDry.OK, code, unreachableDry.Raw)
	}
	assertLeftoversRemain(t, "dry run over an unreachable origin")
	applied, code := run(h.PrimeWorktree(), "--apply")
	if warning, _ := applied.Raw["warning"].(string); !applied.OK || code != 0 || applied.Raw["applied"] != true || !strings.Contains(warning, "unreachable") {
		t.Fatalf("apply: ok=%v code=%d error=%q envelope=%v", applied.OK, code, applied.Error, applied.Raw)
	}
	restoreOrigin()
	requireList(t, applied, "failed_pushes")
	for _, path := range []string{runDir, oldHandoff, scratchChild, traceFile} {
		if exists(path) {
			t.Errorf("--apply left %s", path)
		}
	}
	if !exists(keptHandoff) {
		t.Errorf("--apply removed the handoff the state names")
	}
	assertScratchLink(t, "after --apply", scratchTarget)
	if _, found, err := board.GetTask(taskSlug); err != nil || found {
		t.Errorf("--apply left the done board entry: found=%v err=%v", found, err)
	}

	// A pair whose branch both sides moved is stopped and reported, and the segments run.
	pairBranch := gitkit.CurrentBranch(t, h.PairCodeWorktree(hubforge.TemplatePairSlug))
	gitkit.CommitFile(t, h.PairCodeWorktree(hubforge.TemplatePairSlug), "local.txt", "local\n", "local pair change")
	originCommit(t, h.CodeBare, pairBranch, "origin.txt", "origin\n")
	stoppedPair, code := run(h.PrimeWorktree(), "--apply")
	if !stoppedPair.OK || code != 0 || reconcileBranch(t, stoppedPair, "code", pairBranch)["State"] != string(fabricengine.BranchDiverged) || len(requireList(t, stoppedPair, "segments")) == 0 {
		t.Errorf("--apply over a diverged pair: ok=%v code=%d envelope=%v; want the stop reported and the segments run", stoppedPair.OK, code, stoppedPair.Raw)
	}

	// A run origin's board removed or finished is torn down locally by --apply.
	endedSlug := "ended-task"
	endOnOrigin(t, h, board, endedSlug)
	endedRun, code := run(h.PrimeWorktree(), "--apply")
	if !endedRun.OK || code != 0 || !entryRemoved(t, endedRun, endedSlug) {
		t.Fatalf("--apply over an ended run: ok=%v code=%d error=%q envelope=%v", endedRun.OK, code, endedRun.Error, endedRun.Raw)
	}
	if exists(fabricengine.WorktreePath(h.Location, endedSlug)) || exists(fabricengine.RecordsWorktreePath(h.Location, endedSlug)) {
		t.Errorf("--apply left the ended run's worktrees")
	}
	if !slices.Contains(mutationKinds(t, endedRun), string(fabricengine.KindWorktreeRemoved)) {
		t.Errorf("--apply over an ended run: mutations = %v; want the removed pair recorded", mutationKinds(t, endedRun))
	}

	// The wake force case: a pair the hibernate daemon paused, ended on origin, with an uncommitted task worktree and a held teardown lock.
	wakeSlug := "wake-task"
	endOnOrigin(t, h, board, wakeSlug)
	wakeWorktree := h.PairCodeWorktree(wakeSlug)
	writeAged(t, filepath.Join(wakeWorktree, "uncommitted.txt"), "work in progress\n", 0)
	pauseGeometry, err := hubgeom.PauseGeometry(h.Location)
	if err != nil {
		t.Fatalf("pause geometry: %v", err)
	}
	now := time.Now()
	if _, err := pauserecord.Update(pauseGeometry.RecordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, pauserecord.Entry{Scope: wakeSlug, PausedAt: now, GraceEnd: now, Caller: hibernateengine.PauseCaller})
		return nil
	}); err != nil {
		t.Fatalf("write the wake pause entry: %v", err)
	}
	teardownLock := fabricengine.TeardownLockPath(h.Path, wakeSlug)
	if err := os.MkdirAll(filepath.Dir(teardownLock), 0o755); err != nil {
		t.Fatalf("create the teardown lock dir: %v", err)
	}
	held, err := lock.AcquireWriteLock(teardownLock)
	if err != nil {
		t.Fatalf("hold the teardown lock: %v", err)
	}
	t.Cleanup(func() { _ = held.Release() })

	busy, code := run(h.PrimeWorktree(), "--apply")
	if !busy.OK || code != 0 || entryRemoved(t, busy, wakeSlug) || !entryBusy(t, busy, wakeSlug) {
		t.Fatalf("first --apply over the wake pair: ok=%v code=%d error=%q envelope=%v; want the pair busy and not removed", busy.OK, code, busy.Error, busy.Raw)
	}
	if !exists(wakeWorktree) {
		t.Errorf("the first --apply removed the wake pair's worktree while its teardown lock was held")
	}
	snapshotTag := false
	for _, mutation := range mutationsOf(t, busy) {
		detail, _ := mutation["detail"].(string)
		snapshotTag = snapshotTag || (mutation["kind"] == string(fabricengine.KindTagCreated) && strings.Contains(detail, "set-aside/"+wakeSlug+"/"))
	}
	if !snapshotTag {
		t.Errorf("first --apply mutations = %v; want the snapshot's set-aside tag", mutationsOf(t, busy))
	}
	if record, err := pauserecord.Read(pauseGeometry.RecordPath); err != nil || slices.ContainsFunc(record.Live, func(entry pauserecord.Entry) bool { return entry.Scope == wakeSlug }) {
		t.Errorf("pause record live = %v, err = %v; want the wake hold lifted", record.Live, err)
	}

	if err := held.Release(); err != nil {
		t.Fatalf("release the teardown lock: %v", err)
	}
	removed, code := run(h.PrimeWorktree(), "--apply")
	if !removed.OK || code != 0 || !entryRemoved(t, removed, wakeSlug) || exists(wakeWorktree) {
		t.Fatalf("second --apply over the wake pair: ok=%v code=%d error=%q envelope=%v; want the pair removed", removed.OK, code, removed.Error, removed.Raw)
	}
	if !slices.Contains(mutationKinds(t, removed), string(fabricengine.KindWorktreeRemoved)) {
		t.Errorf("second --apply mutations = %v; want the removed pair recorded", mutationKinds(t, removed))
	}

	// A segment failing after a reconcile that moved a branch is a partial failure.
	primeBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
	originCommit(t, h.CodeBare, primeBranch, "fast-forward.txt", "moved\n")
	if err := os.WriteFile(orchPaths.StatePath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("corrupt the orch state: %v", err)
	}
	failedSegment, code := run(h.PrimeWorktree(), "--apply")
	if failedSegment.OK || code == 0 || failedSegment.Partial == nil || !*failedSegment.Partial || len(mutationsOf(t, failedSegment)) == 0 {
		t.Errorf("--apply with a failing segment after a move: ok=%v code=%d partial=%v mutations=%v; want a partial failure carrying the move", failedSegment.OK, code, failedSegment.Partial, mutationsOf(t, failedSegment))
	}
}

// requireList fails the test unless env holds a JSON array under key, and returns it.
func requireList(t *testing.T, env envelope.Envelope, key string) []any {
	t.Helper()
	list, ok := env.Raw[key].([]any)
	if !ok {
		t.Fatalf("envelope %s = %#v; want an array; envelope = %v", key, env.Raw[key], env.Raw)
	}
	return list
}

// mutationsOf returns the envelope's mutation entries.
func mutationsOf(t *testing.T, env envelope.Envelope) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, item := range requireList(t, env, "mutations") {
		entries = append(entries, item.(map[string]any))
	}
	return entries
}

// mutationKinds returns the kind of each mutation entry of the envelope.
func mutationKinds(t *testing.T, env envelope.Envelope) []string {
	t.Helper()
	var kinds []string
	for _, entry := range mutationsOf(t, env) {
		kinds = append(kinds, entry["kind"].(string))
	}
	return kinds
}

// requireEmptyRecord fails the test unless the envelope carries an empty mutations array and the given partial flag.
func requireEmptyRecord(t *testing.T, step string, env envelope.Envelope, wantPartial bool) {
	t.Helper()
	if len(requireList(t, env, "mutations")) != 0 || env.Partial == nil || *env.Partial != wantPartial {
		t.Errorf("%s: mutations = %v, partial = %v; want an empty array and %v", step, env.Raw["mutations"], env.Partial, wantPartial)
	}
}

// reconcileBranch returns the reconcile report's outcome for the branch of the repository side, failing the test when the report holds none.
func reconcileBranch(t *testing.T, env envelope.Envelope, repo, branch string) map[string]any {
	t.Helper()
	report, _ := env.Raw["reconcile"].(map[string]any)
	branches, _ := report["Branches"].([]any)
	for _, item := range branches {
		outcome := item.(map[string]any)
		if outcome["Repo"] == repo && outcome["Branch"] == branch {
			return outcome
		}
	}
	t.Fatalf("the reconcile report holds no %s branch %q; report = %v", repo, branch, report)
	return nil
}

// reconcileEntry returns the reconcile report's entry for slug, nil when it holds none.
func reconcileEntry(env envelope.Envelope, slug string) map[string]any {
	report, _ := env.Raw["reconcile"].(map[string]any)
	entries, _ := report["Entries"].([]any)
	for _, item := range entries {
		if entry := item.(map[string]any); entry["Slug"] == slug {
			return entry
		}
	}
	return nil
}

// entryRemoved reports whether the reconcile removed slug's pair as an ended run.
func entryRemoved(t *testing.T, env envelope.Envelope, slug string) bool {
	t.Helper()
	entry := reconcileEntry(env, slug)
	return entry != nil && entry["State"] == string(fabricengine.EntryEnded) && entry["Removed"] == true
}

// entryBusy reports whether the reconcile left slug's ended pair in place because it was busy.
func entryBusy(t *testing.T, env envelope.Envelope, slug string) bool {
	t.Helper()
	entry := reconcileEntry(env, slug)
	return entry != nil && entry["State"] == string(fabricengine.EntryEnded) && entry["Busy"] == true
}

// originCommit commits rel from a second clone of bare at branch, as the other machine, and pushes it.
func originCommit(t *testing.T, bare, branch, rel, content string) {
	t.Helper()
	other := filepath.Join(t.TempDir(), "other")
	gitkit.Git(t, filepath.Dir(other), "clone", "--branch", branch, bare, other)
	gitkit.CommitFile(t, other, rel, content, "origin "+rel)
	gitkit.Git(t, other, "push", "origin", branch)
}

// setOrigin points the origin remote of repoDir at url and returns the call that puts the old url back.
func setOrigin(t *testing.T, repoDir, url string) (restore func()) {
	t.Helper()
	old := strings.TrimSpace(gitkit.Git(t, repoDir, "remote", "get-url", "origin"))
	gitkit.Git(t, repoDir, "remote", "set-url", "origin", url)
	restored := false
	restore = func() {
		if !restored {
			restored = true
			gitkit.Git(t, repoDir, "remote", "set-url", "origin", old)
		}
	}
	t.Cleanup(restore)
	return restore
}

// endOnOrigin gives the hub a pair for slug whose run has a batten record, and has origin's board mark the entry done while this machine's board still holds it live.
func endOnOrigin(t *testing.T, h *hubforge.Hub, board *boardengine.Board, slug string) {
	t.Helper()
	hubforge.AddPair(t, h, slug)
	writeAged(t, shedrun.StatusFile(h.Location, slug), `{"current_producer":"Implementer"}`, 0)
	if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the board entry %s: %v", slug, err)
	}
	// Board writes stay uncommitted in the Bolt checkout until its sync, which a test binary never spawns, so the test commits them itself.
	boardDir := h.BoardDir()
	commitBoard := func(message string) {
		gitkit.Git(t, boardDir, "add", "board.json")
		gitkit.Git(t, boardDir, "commit", "-m", message)
		gitkit.Git(t, boardDir, "push", "origin", "main")
	}
	commitBoard("board: " + slug + " live")
	live := gitkit.RevParse(t, boardDir, "HEAD")
	done := "done"
	if err := board.SetStatus(slug, &done); err != nil {
		t.Fatalf("mark the board entry %s done: %v", slug, err)
	}
	commitBoard("board: " + slug + " done")
	gitkit.Git(t, boardDir, "reset", "--hard", live)
}
