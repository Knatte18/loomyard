// recoverbatch.go implements webster's re-entrant, bounded long-poll exception-path verb as three
// lease-scoped phases: RecoverSpawnOrAttach (the only place webster spawns a genuinely separate
// process — escalating a batch a fork reported stuck, or never reported at all, to a cold
// implementer strand at the recovery role, rendering the SEPARATE, full cold-start recovery prompt
// via RenderRecoveryPrompt — deliberately distinct from RenderForkPrompt's thin in-session fork
// prompt, since the recovery strand inherits no session context, per the fork-context-hygiene
// Shared Decision), RecoverAwait (the bounded wait, over webster's own classification machinery —
// Classify/PollUntilTerminal/ TurnEnded/StrandLive), and PersistRecoveryTerminal (the terminal
// digest merge into a freshly reloaded state).
// First call spawns and records;
// every call (the first included) blocks at most one wait window and returns either the terminal
// digest or a running snapshot;
// a caller (webstercli) re-calls until terminal.
//
// The three-phase split exists for the state-mutation lease: the caller holds it across
// spawn-or-attach and across the terminal persist,
// but NEVER across the bounded wait between them (see AcquireStateMutation's
// never-across-a-long-block contract).
// Nothing here touches fabric: the caller fabric-commits state.json after the spawn record and again at
// terminal persistence, webster's own fabric-commit-boundary discipline.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Clock abstracts time.Now/time.Sleep so RecoverBatch's bounded wait runs instantly under test,
// mirroring shuttleengine's wait.go seam and webster's own poll.go clock.
// Clock is deliberately a plain, exported webster-local interface — it structurally satisfies
// poll.go's unexported clock interface (identical Now/Sleep method set), which is what lets
// RecoverBatch hand a Clock value straight to PollUntilTerminal without any adapter: Go interface
// satisfaction is structural, not by declared type identity.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// RecoverDeps carries seams RecoverBatch needs: Starter, Plan, Batches, State, Roles, Config,
// Engine, Reed, ShuttleCfg, Layout, WorktreeRoot, WebsterDir, ReportsDir.
type RecoverDeps struct {
	Starter      Starter
	Plan         *planparser.Plan
	Batches      []batcher.Batch
	State        *State
	Roles        map[Role]modelspec.Resolved
	Config       Config
	Engine       shuttleengine.Engine
	Reed         shuttleengine.ReedOps
	ShuttleCfg   shuttleengine.Config
	Layout       *lyxcwd.Location
	WorktreeRoot string
	WebsterDir   string
	ReportsDir   string
}

// RecoverResult is what one RecoverAwait call hands back: Digest (nil while Running), Running (true
// if wait elapsed non-terminal), ElapsedS (since spawn), and Warnings (non-fatal substrate-cleanup
// failures).
type RecoverResult struct {
	Digest   *Digest
	Running  bool
	ElapsedS int
	Warnings []string
}

// archiveStaleReport renames a stale report to free the path, keeping it
// auditable rather than deleting. Absent file returns ("", nil).
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
	stamp := now().UTC().Format(archiveTimestampFormat)
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

// refuseRecoveringDoneReport refuses to recover a batch whose report already
// has status: OK (record-batch is the consuming verb), except when prior is
// terminal dead (a late orphan report), or missing/unparseable.
func refuseRecoveringDoneReport(reportsDir string, number int, slug string, prior *BatchState) error {
	// Dead-orphan exception: archive a late report the orphan wrote after dead classification.
	if prior != nil && prior.Terminal && prior.Status == DigestStatusDead {
		return nil
	}

	reportPath := filepath.Join(reportsDir, ReportFileName(number, slug))
	report, err := ParseReport(reportPath)
	if err != nil {
		// Absent or malformed report: recovery is the path for this.
		return nil
	}
	if report.Status == ReportStatusOK {
		return fmt.Errorf("webster: batch %02d-%s already has a report with status: OK at %s — recover-batch never archives finished work; record it with `lyx webster record-batch %d` instead", number, slug, reportPath, number)
	}
	return nil
}

// recoverSpawn archives any stale report, stops a live prior strand, renders
// the recovery prompt, and starts the recovery strand, returning a fresh BatchState.
// clk stamps SpawnedAt so elapsed-since-spawn is measured against the same clock.
func recoverSpawn(deps RecoverDeps, batch batcher.Batch, prior *BatchState, prevDigest string, clk Clock) (*BatchState, error) {
	number, slug := batchIdentity(batch)

	if err := refuseRecoveringDoneReport(deps.ReportsDir, number, slug, prior); err != nil {
		return nil, err
	}

	// Ensure reports dir exists so the recovery strand's report write succeeds.
	if err := os.MkdirAll(deps.ReportsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create reports dir %s: %w", deps.ReportsDir, err)
	}

	if _, err := archiveStaleReport(deps.ReportsDir, number, slug, clk.Now); err != nil {
		return nil, err
	}

	if prior != nil {
		if err := removeStrandIfLive(deps.Reed, prior.StrandGUID); err != nil {
			return nil, err
		}
	}

	batchName := fmt.Sprintf("%02d-%s", number, slug)
	reportPath, err := filepath.Abs(filepath.Join(deps.ReportsDir, ReportFileName(number, slug)))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve report path: %w", err)
	}

	prompt, err := RenderRecoveryPrompt(batch, prevDigest, reportPath, deps.Layout, deps.Config.SelfFixCap)
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

	runState, runDir, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.Layout.AnchorPath(), run.StrandGUID())
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

// RecoverSpawnOrAttach decides spawn-or-attach: if a recorded, non-terminal recovery BatchState
// exists, ATTACH and return it;
// otherwise SPAWN fresh.
// Caller persists deps.State via SaveState when spawned is true.
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

// RecoverAwait drives the bounded wait for a recovery strand: the long-poll classification loop
// (see awaitTerminal) plus substrate release on terminal.
// Caller runs this with the state-mutation lease RELEASED.
func RecoverAwait(deps RecoverDeps, batchNumber int, bs *BatchState, wait time.Duration, clk Clock) (*RecoverResult, error) {
	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	return awaitTerminal(deps, batch, bs, wait, clk)
}

// PersistRecoveryTerminal merges a terminal digest into st (loaded fresh under the lease after the
// unleased wait).
// Marks batch terminal and clears the in-flight cursor.
func PersistRecoveryTerminal(st *State, batchNumber int, digest *Digest) error {
	bs, ok := st.Batches[batchNumber]
	if !ok || bs == nil {
		return fmt.Errorf("webster: no recorded state for batch %d at recovery terminal persistence — state.json changed underneath the recovery wait", batchNumber)
	}
	bs.Digest = digest
	bs.Terminal = true
	bs.Status = digest.Status
	// Record CardSHAs like record-batch does, so integration bisect has no gaps.
	if digest.HeadSHA != "" {
		bs.CardSHAs = []string{digest.HeadSHA}
	}
	st.CurrentBatch = 0
	return nil
}

// awaitTerminal drives one bounded long-poll wait for bs's recovery strand,
// assembling ClassifyInputs and releasing substrate with status-specific rules
// on terminal (done removes strand+rundir, stuck removes strand, dead keeps both).
// Cleanup failures are warnings, not fatal errors.
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
		strandLive, err := StrandLive(deps.Reed, bs.StrandGUID)
		if err != nil {
			return Digest{}, false, err
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

	// Cross-check report's head_sha against worktree's actual HEAD like RecordBatch does.
	if digest.HeadSHA != "" {
		actualHead, err := headSHA(deps.WorktreeRoot)
		if err != nil {
			return nil, err
		}
		if actualHead != digest.HeadSHA {
			return nil, fmt.Errorf("webster: recovery report for batch %02d-%s: head_sha %q does not match the worktree's actual HEAD %q", number, slug, digest.HeadSHA, actualHead)
		}
	}

	var warnings []string
	removeStrand := func() {
		if err := removeStrandIfLive(deps.Reed, bs.StrandGUID); err != nil {
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
		// Remove strand but keep run dir for diagnosis.
		removeStrand()
	case DigestStatusDead:
		// Keep both: dead-classified strand may still be working.
	}

	return &RecoverResult{Digest: &digest, ElapsedS: elapsedS, Warnings: warnings}, nil
}
