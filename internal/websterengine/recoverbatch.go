// recoverbatch.go implements RecoverBatch, webster's re-entrant, bounded
// long-poll exception-path verb: the only place webster spawns a genuinely
// separate process. It escalates a batch a fork reported stuck (or never
// reported at all) to a cold implementer strand at the recovery role,
// reusing builderengine's SpawnBatch machinery by import rather than
// duplicating it (spawn-or-attach decision, the stencil-filled implementer
// template, the shuttleengine.Spec construction, and the cross-process
// FindRun resolution) plus builder's own classification machinery
// (Classify/PollUntilTerminal/TurnEnded/StrandLive) for the bounded wait
// that follows. First call spawns and records; every call (the first
// included) blocks at most one wait window and returns either the terminal
// digest or a running snapshot; a caller (webstercli) re-calls until
// terminal.
//
// RecoverBatch never touches weft: the caller holds the state-mutation
// lease around the spawn-and-record phase and weft-commits state.json
// immediately after it, and again at terminal persistence, mirroring
// builder's own weft-commit-boundary discipline.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// Clock abstracts time.Now/time.Sleep so RecoverBatch's bounded wait runs
// instantly under test, mirroring shuttleengine's wait.go seam and
// builderengine's own poll.go clock. Clock is deliberately a plain,
// exported webster-local interface — it structurally satisfies
// builderengine's unexported clock interface (identical Now/Sleep method
// set), which is what lets RecoverBatch hand a Clock value straight to
// builderengine.PollUntilTerminal without any adapter: Go interface
// satisfaction is structural, not by declared type identity, so this is the
// documented reuse path, not a coincidence.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// RecoverDeps carries every seam RecoverBatch needs, so a test can fake each
// one independently: Starter spawns the cold recovery strand (exactly
// builderengine.Starter's signature — production code passes a real
// *shuttleengine.Runner); Plan and State are the already-parsed/loaded plan
// and run state RecoverBatch reads and mutates; Roles is the pre-flight-
// resolved role->model-spec map (see ResolveRoles); Config is the loaded
// webster.yaml; Engine supplies TurnEnded's event-grammar parsing; Mux is
// the live mux query surface StrandLive/RemoveStrand consult; ShuttleCfg and
// Layout are what shuttleengine.FindRun needs to resolve the just-started
// run's cross-process identity; WorktreeRoot, WebsterDir, and ReportsDir are
// the hubgeometry-resolved host checkout, _lyx/webster, and
// _lyx/webster/reports directories.
type RecoverDeps struct {
	Starter      builderengine.Starter
	Plan         *builderengine.Plan
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

// RecoverResult is what one RecoverBatch call hands back to its caller
// (internal/webstercli's recover-batch verb): Digest is the distilled
// digest once the recovery strand reaches a terminal classification (nil
// while Running); Running reports whether this call's bounded wait elapsed
// with the batch still non-terminal (the caller re-calls); Spawned reports
// whether THIS call performed the spawn (false on an attach-only call), so
// the caller knows whether to weft-commit a freshly-recorded strand;
// ElapsedS is the number of seconds since the recovery strand was spawned,
// measured across every re-entrant call, not merely this one; Warnings
// carries every non-fatal substrate-cleanup failure observed at terminal
// persistence, never treated as a failure.
type RecoverResult struct {
	Digest   *builderengine.Digest
	Running  bool
	Spawned  bool
	ElapsedS int
	Warnings []string
}

// recoverArchiveTimestampFormat is the UTC compact timestamp format
// archiveStaleReport archives a stale batch report under — webster's own
// copy of builder's archiveTimestampFormat (runlevel.go), which is
// unexported and so not importable; the two format strings are kept
// identical so an archived webster artifact sorts and reads the same way as
// builder's own archived artifacts.
const recoverArchiveTimestampFormat = "20060102T150405Z"

// archiveStaleReport renames reportsDir's NN-<slug>.yaml, if present, to
// NN-<slug>-<UTC-compact-timestamp>.yaml in place, mirroring
// builderengine's own archiveStaleReport (spawn.go): the recovery path's
// archive-never-refuse escape. A recovery spawn re-uses the batch's own
// report path as its sole shuttle OutputFiles entry, which both a
// pre-existing-report guard and shuttle's own Spec.validate would refuse
// when a prior stuck report is still on disk — archiving frees the path
// while keeping the stuck report auditable rather than deleting it. now is
// a seam so tests can pin the timestamp; production callers pass time.Now.
// Absent file: ("", nil), a no-op — a recovery spawn is legitimate even
// when no prior report exists. Built on builderengine.FirstFreeArchivePath
// so the same-second "-1"/"-2" collision rule lives in exactly one place,
// per the reuse-by-import-never-copy decision.
func archiveStaleReport(reportsDir string, number int, slug string, now func() time.Time) (string, error) {
	path := filepath.Join(reportsDir, builderengine.BatchReportFileName(number, slug))
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("webster: stat batch report %s: %w", path, err)
	}

	const ext = ".yaml"
	base := strings.TrimSuffix(builderengine.BatchReportFileName(number, slug), ext)
	stamp := now().UTC().Format(recoverArchiveTimestampFormat)
	target, err := builderengine.FirstFreeArchivePath(func(suffix string) string {
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

// recoverSpawn performs the SPAWN half of RecoverBatch's spawn-or-attach
// decision: archive any stale report at this batch's own report path
// (archive-never-refuse — the stuck report is the recovery spawn's own
// output path), stop prior's recorded strand when it is still live (a
// timed-out implementer may still be WORKING, not hung, and left alive it
// races the fresh session — the same reclaim discipline builder's
// dead-respawn ladder applies), fill builderengine.ImplementerTemplate()
// via stencil.Fill with the batch's markers (a cold recovery session gets
// builder's full implementer orientation, per discussion.md
// single-model-forks-and-cold-recovery), build and start the
// shuttleengine.Spec at the recovery role, and resolve the just-started
// run's cross-process identity via shuttleengine.FindRun. prior is the
// batch's existing BatchState, if any (nil for a batch that has never been
// touched by begin-batch or a prior recovery); its StrandGUID (empty for a
// plain fork batch, since only recovery batches carry strand fields) is the
// only field this function reads from it. Returns the freshly-built
// BatchState the caller records into deps.State.Batches[batchNumber].
func recoverSpawn(deps RecoverDeps, batch builderengine.PlanBatch, prior *BatchState) (*BatchState, error) {
	if _, err := archiveStaleReport(deps.ReportsDir, batch.Number, batch.Slug, time.Now); err != nil {
		return nil, err
	}

	if prior != nil {
		if err := builderengine.RemoveStrandIfLive(deps.Mux, prior.StrandGUID); err != nil {
			return nil, err
		}
	}

	batchName := fmt.Sprintf("%02d-%s", batch.Number, batch.Slug)
	reportPath, err := filepath.Abs(filepath.Join(deps.ReportsDir, builderengine.BatchReportFileName(batch.Number, batch.Slug)))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve report path: %w", err)
	}
	batchFilePath, err := filepath.Abs(filepath.Join(deps.Plan.Dir, batch.File))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve batch file path: %w", err)
	}

	values := map[string]string{
		"batch_file":    batchFilePath,
		"batch_name":    batchName,
		"report_path":   reportPath,
		"self_fix_cap":  fmt.Sprintf("%d", deps.Config.SelfFixCap),
		"worktree_root": deps.WorktreeRoot,
	}
	prompt, err := stencil.Fill(builderengine.ImplementerTemplate(), values)
	if err != nil {
		return nil, fmt.Errorf("webster: fill implementer template: %w", err)
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

	head, err := builderengine.HeadSHA(deps.WorktreeRoot)
	if err != nil {
		return nil, err
	}

	return &BatchState{
		Slug:          batch.Slug,
		StartSHA:      head,
		Kind:          "recovery",
		SpawnedAt:     time.Now().UTC().Format(time.RFC3339),
		StrandGUID:    run.StrandGUID(),
		ShuttleRunDir: runDir,
		EventsPath:    runState.EventsPath,
	}, nil
}

// RecoverBatch drives one recover-batch call to completion: the
// spawn-or-attach decision, then the bounded wait (see awaitTerminal). The
// spawn-or-attach decision reads deps.State.Batches[batchNumber]: a
// recorded, non-terminal BatchState whose Kind is "recovery" and whose
// StrandGUID is already set means a prior call already spawned this
// recovery strand — ATTACH, skip straight to the bounded wait. Any other
// state (no record, a plain fork batch's record, or a previously-terminal
// recovery record) means SPAWN a fresh recovery strand. The caller
// (webstercli) holds the state-mutation lease around this whole call and is
// responsible for persisting deps.State via SaveState once RecoverBatch
// returns successfully — RecoverBatch itself never calls SaveState and
// never touches weft.
func RecoverBatch(deps RecoverDeps, batchNumber int, wait time.Duration, clk Clock) (*RecoverResult, error) {
	batch, err := findBatch(deps.Plan, batchNumber)
	if err != nil {
		return nil, err
	}

	prior := deps.State.Batches[batchNumber]
	attach := prior != nil && prior.Kind == "recovery" && !prior.Terminal && prior.StrandGUID != ""

	spawned := false
	bs := prior
	if !attach {
		fresh, err := recoverSpawn(deps, batch, prior)
		if err != nil {
			return nil, err
		}
		if deps.State.Batches == nil {
			deps.State.Batches = map[int]*BatchState{}
		}
		deps.State.Batches[batchNumber] = fresh
		deps.State.CurrentBatch = batchNumber
		bs = fresh
		spawned = true
	}

	return awaitTerminal(deps, batch, bs, spawned, wait, clk)
}

// awaitTerminal is a placeholder bounded wait: report presence only, no
// TurnEnded/StrandLive/timeout classification and no substrate cleanup.
// TODO(recover-batch batch 26): replace with the full Classify-based gather
// (TurnEnded, StrandLive, Elapsed measured since bs.SpawnedAt across every
// re-entrant call, drift computation) and terminal substrate-release parity
// with builder's dead-respawn ladder.
func awaitTerminal(deps RecoverDeps, batch builderengine.PlanBatch, bs *BatchState, spawned bool, wait time.Duration, clk Clock) (*RecoverResult, error) {
	reportPath := filepath.Join(deps.ReportsDir, builderengine.BatchReportFileName(batch.Number, batch.Slug))
	deadline := clk.Now().Add(wait)
	for {
		if _, err := os.Stat(reportPath); err == nil {
			report, err := builderengine.ParseReport(reportPath)
			if err != nil {
				return nil, err
			}
			digest := builderengine.Distill(report, nil, batch.Scope, false)
			bs.Digest = &digest
			bs.Terminal = true
			bs.Status = digest.Status
			deps.State.CurrentBatch = 0
			return &RecoverResult{Digest: &digest, Spawned: spawned}, nil
		}
		if clk.Now().After(deadline) {
			return &RecoverResult{Running: true, Spawned: spawned}, nil
		}
		clk.Sleep(time.Second)
	}
}
