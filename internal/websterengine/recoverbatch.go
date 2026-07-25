// recoverbatch.go implements webster's re-entrant, bounded long-poll
// exception-path verb as three lease-scoped phases: RecoverSpawnOrAttach
// (the only place webster spawns a genuinely separate process — escalating
// a batch a fork reported stuck, or never reported at all, to a cold
// implementer strand at the recovery role, rendering the SAME fork prompt a
// fork would have gotten via RenderForkPrompt, since the recovery strand
// implements the same batch), RecoverAwait (the bounded wait, over
// webster's own classification machinery — Classify/PollUntilTerminal/
// TurnEnded/StrandLive), and PersistRecoveryTerminal (the terminal digest
// merge into a freshly reloaded state). First call spawns and records;
// every call (the first included) blocks at most one wait window and
// returns either the terminal digest or a running snapshot; a caller
// (webstercli) re-calls until terminal.
//
// The three-phase split exists for the state-mutation lease: the caller
// holds it across spawn-or-attach and across the terminal persist, but
// NEVER across the bounded wait between them (see AcquireStateMutation's
// never-across-a-long-block contract). Nothing here touches weft: the
// caller weft-commits state.json after the spawn record and again at
// terminal persistence, mirroring builder's own weft-commit-boundary
// discipline.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Clock abstracts time.Now/time.Sleep so RecoverBatch's bounded wait runs
// instantly under test, mirroring shuttleengine's wait.go seam and
// webster's own poll.go clock. Clock is deliberately a plain, exported
// webster-local interface — it structurally satisfies poll.go's unexported
// clock interface (identical Now/Sleep method set), which is what lets
// RecoverBatch hand a Clock value straight to PollUntilTerminal without any
// adapter: Go interface satisfaction is structural, not by declared type
// identity.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// RecoverDeps carries every seam RecoverBatch needs, so a test can fake each
// one independently: Starter spawns the cold recovery strand (see strand.go's
// Starter — production code passes a real *shuttleengine.Runner); Plan is
// the already-parsed plan RenderForkPrompt reads its plan-level context
// from; Batches is the batchifier-derived execution batches (see
// internal/batcher.Select) `run` computed once at entry; State is the
// already-loaded run state RecoverBatch reads and mutates; Roles is the
// pre-flight-resolved role->model-spec map (see ResolveRoles); Config is the
// loaded webster.yaml; Engine supplies TurnEnded's event-grammar parsing;
// Mux is the live mux query surface StrandLive/RemoveStrand consult;
// ShuttleCfg and Layout are what shuttleengine.FindRun needs to resolve the
// just-started run's cross-process identity; WorktreeRoot, WebsterDir, and
// ReportsDir are the hubgeometry-resolved host checkout, _lyx/webster, and
// _lyx/webster/reports directories.
type RecoverDeps struct {
	Starter      Starter
	Plan         *planparser.Plan
	Batches      []batcher.Batch
	State        *State
	Roles        map[Role]modelspec.Resolved
	Config       Config
	Engine       shuttleengine.Engine
	Mux          shuttleengine.MuxOps
	ShuttleCfg   shuttleengine.Config
	Layout       *hubgeometry.Layout
	WorktreeRoot string
	WebsterDir   string
	ReportsDir   string
}

// RecoverResult is what one RecoverAwait call hands back to its caller
// (internal/webstercli's recover-batch verb): Digest is the distilled
// digest once the recovery strand reaches a terminal classification (nil
// while Running); Running reports whether this call's bounded wait elapsed
// with the batch still non-terminal (the caller re-calls); ElapsedS is the
// number of seconds since the recovery strand was spawned, measured across
// every re-entrant call, not merely this one; Warnings carries every
// non-fatal substrate-cleanup failure observed at terminal classification,
// never treated as a failure.
type RecoverResult struct {
	Digest   *Digest
	Running  bool
	ElapsedS int
	Warnings []string
}

// recoverArchiveTimestampFormat is the UTC compact timestamp format
// archiveStaleReport archives a stale batch report under — shared with
// archive.go's own archiveTimestampFormat in spirit, kept as its own
// constant here since archiveStaleReport predates that shared const and the
// two format strings must simply stay identical so an archived webster
// artifact sorts and reads the same way regardless of which helper archived
// it.
const recoverArchiveTimestampFormat = archiveTimestampFormat

// archiveStaleReport renames reportsDir's NN-<slug>.yaml, if present, to
// NN-<slug>-<UTC-compact-timestamp>.yaml in place: the recovery path's
// archive-never-refuse escape. A recovery spawn re-uses the batch's own
// report path as its sole shuttle OutputFiles entry, which both a
// pre-existing-report guard and shuttle's own Spec.validate would refuse
// when a prior stuck report is still on disk — archiving frees the path
// while keeping the stuck report auditable rather than deleting it. now is
// a seam so tests can pin the timestamp; production callers pass time.Now.
// Absent file: ("", nil), a no-op — a recovery spawn is legitimate even
// when no prior report exists. Built on firstFreeArchivePath so the same-
// second "-1"/"-2" collision rule lives in exactly one place.
func archiveStaleReport(reportsDir string, number int, slug string, now func() time.Time) (string, error) {
	path := filepath.Join(reportsDir, ReportFileName(number, slug))
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("webster: stat batch report %s: %w", path, err)
	}

	const ext = ".yaml"
	base := strings.TrimSuffix(ReportFileName(number, slug), ext)
	stamp := now().UTC().Format(recoverArchiveTimestampFormat)
	target, err := firstFreeArchivePath(func(suffix string) string {
		return filepath.Join(reportsDir, fmt.Sprintf("%s-%s%s%s", base, stamp, suffix, ext))
	})
	if err != nil {
		return "", fmt.Errorf("webster: find archive target for batch report %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("webster: archive stale batch report %s: %w", path, err)
	}
	return target, nil
}

// refuseRecoveringDoneReport is recoverSpawn's finished-work guard: when the
// batch's on-disk report already parses to status: OK, spawning a recovery
// strand would silently archive finished work away and pay for a cold
// implementer to redo a complete batch — record-batch, not recover-batch, is
// the verb that consumes an OK report (found live in round fable-r1: a
// record-batch-wedged Master recovered two already-done batches). The ONE
// exception mirrors builder's dead-respawn rule: a batch whose persisted
// state is terminal dead may carry a LATE OK report its orphaned strand
// wrote after the classification — that report is archive-never-refuse
// material, so the guard steps aside. A missing or unparseable report also
// steps aside: recovery is exactly the path for a batch with no usable
// report.
func refuseRecoveringDoneReport(reportsDir string, number int, slug string, prior *BatchState) error {
	// The dead-orphan late-report exception: the ladder archives, never
	// refuses, a report the orphan wrote after a dead classification.
	if prior != nil && prior.Terminal && prior.Status == DigestStatusDead {
		return nil
	}

	reportPath := filepath.Join(reportsDir, ReportFileName(number, slug))
	report, err := ParseReport(reportPath)
	if err != nil {
		// Absent or malformed: nothing finished to protect — recovery is the
		// designed path for exactly this state.
		return nil
	}
	if report.Status == ReportStatusOK {
		return fmt.Errorf("webster: batch %02d-%s already has a report with status: OK at %s — recover-batch never archives finished work; record it with `lyx webster record-batch %d` instead", number, slug, reportPath, number)
	}
	return nil
}

// recoverSpawn performs the SPAWN half of RecoverBatch's spawn-or-attach
// decision: archive any stale report at this batch's own report path
// (archive-never-refuse — the stuck report is the recovery spawn's own
// output path), stop prior's recorded strand when it is still live (a
// timed-out implementer may still be WORKING, not hung, and left alive it
// races the fresh session — the same reclaim discipline builder's
// dead-respawn ladder applies), render the SAME fork prompt a fork would
// have gotten via RenderForkPrompt (the recovery strand implements the same
// batch), build and start the shuttleengine.Spec at the recovery role, and
// resolve the just-started run's cross-process identity via
// shuttleengine.FindRun. prior is the batch's existing BatchState, if any
// (nil for a batch that has never been touched by begin-batch or a prior
// recovery); its StrandGUID (empty for a plain fork batch, since only
// recovery batches carry strand fields) is the only field this function
// reads from it. clk stamps SpawnedAt (clk.Now(), never the wall clock
// directly) so the elapsed-since-spawn measurement awaitTerminal performs
// later is computed against the SAME clock a caller injects for the whole
// call — the property a fake clock's tests depend on. Returns the
// freshly-built BatchState the caller records into
// deps.State.Batches[batchNumber].
func recoverSpawn(deps RecoverDeps, batch batcher.Batch, prior *BatchState, prevDigest string, clk Clock) (*BatchState, error) {
	number, slug := batchIdentity(batch)

	if err := refuseRecoveringDoneReport(deps.ReportsDir, number, slug, prior); err != nil {
		return nil, err
	}

	if _, err := archiveStaleReport(deps.ReportsDir, number, slug, clk.Now); err != nil {
		return nil, err
	}

	if prior != nil {
		if err := removeStrandIfLive(deps.Mux, prior.StrandGUID); err != nil {
			return nil, err
		}
	}

	batchName := fmt.Sprintf("%02d-%s", number, slug)
	reportPath, err := filepath.Abs(filepath.Join(deps.ReportsDir, ReportFileName(number, slug)))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve report path: %w", err)
	}

	prompt, err := RenderForkPrompt(deps.Plan, batch, prevDigest, reportPath, deps.WorktreeRoot, deps.Config.SelfFixCap)
	if err != nil {
		return nil, err
	}

	resolved, ok := deps.Roles[RoleRecovery]
	if !ok {
		return nil, fmt.Errorf("webster: no resolved model-spec for role %q", RoleRecovery)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{reportPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Role:        string(RoleRecovery),
		Round:       batchName,
		Timeout:     time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute,
	}

	run, err := deps.Starter.Start(spec)
	if err != nil {
		return nil, fmt.Errorf("webster: start recovery strand for batch %s: %w", batchName, err)
	}

	runState, runDir, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.Layout, run.StrandGUID())
	if err != nil {
		return nil, fmt.Errorf("webster: resolve spawned recovery run: %w", err)
	}

	head, err := headSHA(deps.WorktreeRoot)
	if err != nil {
		return nil, err
	}

	return &BatchState{
		Slug:          slug,
		StartSHA:      head,
		Kind:          "recovery",
		SpawnedAt:     clk.Now().UTC().Format(time.RFC3339),
		StrandGUID:    run.StrandGUID(),
		ShuttleRunDir: runDir,
		EventsPath:    runState.EventsPath,
	}, nil
}

// RecoverSpawnOrAttach drives the spawn-or-attach half of one recover-batch
// call. The decision reads deps.State.Batches[batchNumber]: a recorded,
// non-terminal BatchState whose Kind is "recovery" and whose StrandGUID is
// already set means a prior call already spawned this recovery strand —
// ATTACH, mutate nothing, return that record. Any other state (no record, a
// plain fork batch's record, or a previously-terminal recovery record)
// means SPAWN a fresh recovery strand and record it into deps.State. The
// caller (webstercli) holds the state-mutation lease across this call ONLY
// — never across the bounded wait that follows (RecoverAwait) — and is
// responsible for persisting deps.State via SaveState when spawned is true.
// This function never calls SaveState and never touches weft.
func RecoverSpawnOrAttach(deps RecoverDeps, batchNumber int, clk Clock) (bs *BatchState, spawned bool, err error) {
	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, false, err
	}

	prior := deps.State.Batches[batchNumber]
	if prior != nil && prior.Kind == "recovery" && !prior.Terminal && prior.StrandGUID != "" {
		return prior, false, nil
	}

	var prevDigest string
	if batchNumber > 1 {
		if prev, ok := deps.State.Batches[batchNumber-1]; ok && prev != nil {
			prevDigest = digestSummaryLine(prev.Digest)
		}
	}

	fresh, err := recoverSpawn(deps, batch, prior, prevDigest, clk)
	if err != nil {
		return nil, false, err
	}
	if deps.State.Batches == nil {
		deps.State.Batches = map[int]*BatchState{}
	}
	deps.State.Batches[batchNumber] = fresh
	deps.State.CurrentBatch = batchNumber
	return fresh, true, nil
}

// RecoverAwait drives the bounded-wait half of one recover-batch call for a
// recovery strand RecoverSpawnOrAttach already recorded: the long-poll
// classification loop (see awaitTerminal) plus, on a terminal
// classification, the strand/run-dir substrate release. It reads and
// mutates NO run state — the caller runs it with the state-mutation lease
// RELEASED (a single call blocks up to poll_wait_s, and holding the lease
// across that block would stall every concurrent verb and run entry for
// minutes — the exact hold AcquireStateMutation's contract forbids), then
// re-acquires the lease and persists a terminal digest into a FRESHLY
// loaded state via PersistRecoveryTerminal.
func RecoverAwait(deps RecoverDeps, batchNumber int, bs *BatchState, wait time.Duration, clk Clock) (*RecoverResult, error) {
	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	return awaitTerminal(deps, batch, bs, wait, clk)
}

// PersistRecoveryTerminal merges a terminal recovery digest into st — which
// the caller re-loaded FRESH under the state-mutation lease AFTER the
// unleased bounded wait, so a concurrently persisted mutation is never
// erased by saving a stale pre-wait copy (builder's poll applies the same
// reload-under-lease discipline). It marks st.Batches[batchNumber]
// terminal with digest and clears the in-flight cursor; a missing batch
// record is a hard error, never silently recreated — the record was
// persisted at spawn time and nothing in the normal flow removes it.
func PersistRecoveryTerminal(st *State, batchNumber int, digest *Digest) error {
	bs, ok := st.Batches[batchNumber]
	if !ok || bs == nil {
		return fmt.Errorf("webster: no recorded state for batch %d at recovery terminal persistence — state.json changed underneath the recovery wait", batchNumber)
	}
	bs.Digest = digest
	bs.Terminal = true
	bs.Status = digest.Status
	st.CurrentBatch = 0
	return nil
}

// awaitTerminal drives one bounded long-poll wait for bs's recovery strand:
// a gather closure assembles ClassifyInputs — the report-presence branch of
// Classify (parsed only when the report file exists), TurnEnded(bs.EventsPath,
// deps.Engine), StrandLive(deps.Mux, bs.StrandGUID), Elapsed computed from
// bs.SpawnedAt (parsed once, up front) so RecoveryTimeoutMin measures
// wall-clock time since the strand was SPAWNED, not since this particular
// call started — the property that makes the long-poll safely re-entrant
// across the tool-call ceiling — and Changed/Dirty via the gitwrap helpers,
// populated only once a report has landed, mirroring Classify's own
// documented contract. gather is handed to PollUntilTerminal, which re-runs
// it on its fixed tick until terminal or wait elapses.
//
// A non-terminal return after wait elapses reports RecoverResult{Running:
// true, ElapsedS: <since spawn>} — nothing anywhere is mutated, so the
// caller's next call re-enters cleanly.
//
// A terminal return carries the digest for the caller to persist (see
// PersistRecoveryTerminal — awaitTerminal itself mutates no state, since it
// runs with the state-mutation lease released), then releases the recovery
// strand's substrate with builder's own parity rules: done removes the
// strand (if still live) and the shuttle run directory; stuck removes the
// strand but KEEPS the run directory (the stuck trail a human or a fresh
// recovery diagnosis reads); dead keeps BOTH — the kept pane may still be
// genuinely working, exactly the kept-substrate reclaim builder's
// dead-respawn ladder consumes elsewhere. A cleanup failure is collected as
// a warning on the result, never returned as a fatal error: the terminal
// classification itself already succeeded, and a caller must not lose that
// judgment over a best-effort housekeeping failure.
func awaitTerminal(deps RecoverDeps, batch batcher.Batch, bs *BatchState, wait time.Duration, clk Clock) (*RecoverResult, error) {
	number, slug := batchIdentity(batch)

	spawnedAt, err := time.Parse(time.RFC3339, bs.SpawnedAt)
	if err != nil {
		return nil, fmt.Errorf("webster: parse recorded spawnedAt %q for batch %d: %w", bs.SpawnedAt, number, err)
	}

	reportPath := filepath.Join(deps.ReportsDir, ReportFileName(number, slug))
	timeout := time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute

	gather := func() (Digest, bool, error) {
		var report *Report
		if _, statErr := os.Stat(reportPath); statErr == nil {
			r, err := ParseReport(reportPath)
			if err != nil {
				return Digest{}, false, err
			}
			report = r
		} else if !os.IsNotExist(statErr) {
			return Digest{}, false, fmt.Errorf("webster: stat batch report %s: %w", reportPath, statErr)
		}

		turnEnded, err := TurnEnded(bs.EventsPath, deps.Engine)
		if err != nil {
			return Digest{}, false, err
		}
		strandLive, err := StrandLive(deps.Mux, bs.StrandGUID)
		if err != nil {
			return Digest{}, false, err
		}

		// Changed/Dirty are distill's own optional cross-check inputs,
		// populated only once a report has landed — a running snapshot
		// never touches git, per ClassifyInputs' documented contract.
		var changed []string
		var isDirty bool
		if report != nil {
			changed, err = changedFiles(deps.WorktreeRoot, bs.StartSHA)
			if err != nil {
				return Digest{}, false, err
			}
			isDirty, err = dirty(deps.WorktreeRoot)
			if err != nil {
				return Digest{}, false, err
			}
		}

		in := ClassifyInputs{
			BatchNumber:  number,
			BatchSlug:    slug,
			ReportPath:   reportPath,
			Report:       report,
			TurnEnded:    turnEnded,
			StrandLive:   strandLive,
			Elapsed:      clk.Now().Sub(spawnedAt),
			BatchTimeout: timeout,
			Changed:      changed,
			Dirty:        isDirty,
		}
		digest, terminal := Classify(in)
		return digest, terminal, nil
	}

	digest, err := PollUntilTerminal(gather, wait, clk)
	if err != nil {
		return nil, err
	}

	elapsedS := int(clk.Now().Sub(spawnedAt).Seconds())

	if digest.Status == DigestStatusRunning {
		return &RecoverResult{Running: true, ElapsedS: elapsedS}, nil
	}

	var warnings []string
	removeStrand := func() {
		if err := removeStrandIfLive(deps.Mux, bs.StrandGUID); err != nil {
			warnings = append(warnings, fmt.Sprintf("recover-batch: remove strand %s: %v", bs.StrandGUID, err))
		}
	}
	removeRunDir := func() {
		if bs.ShuttleRunDir == "" {
			return
		}
		if err := os.RemoveAll(bs.ShuttleRunDir); err != nil {
			warnings = append(warnings, fmt.Sprintf("recover-batch: remove run dir %s: %v", bs.ShuttleRunDir, err))
		}
	}

	switch digest.Status {
	case DigestStatusDone:
		removeStrand()
		removeRunDir()
	case DigestStatusStuck:
		// Strand removed, run dir KEPT — the stuck trail a fresh recovery
		// diagnosis or a human reads.
		removeStrand()
	case DigestStatusDead:
		// Both kept: a dead-classified strand may still be genuinely
		// working, not hung — the same kept-substrate reclaim builder's own
		// dead-respawn ladder consumes on its next spawn.
	}

	return &RecoverResult{Digest: &digest, ElapsedS: elapsedS, Warnings: warnings}, nil
}
