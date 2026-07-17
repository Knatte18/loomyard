// beginbatch.go implements BeginBatch, the first of webster's two bracket
// verbs Master calls around each in-session fork: the pause and fingerprint
// refusal gates, the optional --restart-chain reset, start-SHA capture and
// chain-anchor recording, the idempotent per-batch model assertion (the
// ONLY model-injection site in webster — see doc.go's package comment), the
// previous batch's persisted digest rendered into the fork prompt, and the
// prompt file write itself. BeginBatch never touches weft (webster is
// weft-blind throughout) and never persists deps.State itself — the caller
// holds the state-mutation lease (AcquireStateMutation) across its whole
// begin-batch call and saves state via SaveState once BeginBatch returns
// successfully, mirroring builder's own weft-commit-boundary discipline.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrPaused is the sentinel BeginBatch returns when deps.WebsterDir's pause
// flag is present at the batch boundary (builderengine.PauseRequested).
// Exported so a caller can distinguish the operational "paused" refusal
// from every other begin-batch failure via errors.Is(err, ErrPaused) —
// webster's own sentinel, independent of builder's ErrPaused, per the
// webster-owns-its-own-domain-types decision.
var ErrPaused = errors.New("webster: paused")

// ErrFingerprintMismatch is the sentinel BeginBatch returns when the
// on-disk plan's recomputed fingerprint disagrees with State.PlanFingerprint
// — the same crash/resume guard builder's own spawn-batch applies, but
// webster's own sentinel identity (webster-owns-its-own-domain-types).
var ErrFingerprintMismatch = errors.New("webster: on-disk plan fingerprint does not match this run's recorded state")

// Injector is the seam BeginBatch uses to switch Master's live pane to a
// different model: exactly (*shuttleengine.Runner).Inject's signature, so
// production code passes a real *shuttleengine.Runner directly and tests
// pass a fake that records every (guid, inputs) call.
type Injector interface {
	Inject(guid string, inputs []shuttleengine.PaneInput) error
}

// BeginDeps carries every seam BeginBatch needs, so a test can fake each one
// independently: Plan and State are the already-parsed/loaded plan and run
// state BeginBatch reads and mutates; Roles is the pre-flight-resolved
// role->model-spec map (see ResolveRoles); Config is the loaded
// webster.yaml; Engine supplies the provider-specific ModelSwitchSequence
// choreography; Injector is what actually types that choreography into
// Master's pane; Mux is the live mux query surface the strand-reclaim guards
// consult (--restart-chain stopping live chain members, and the
// prior-recovery-strand reclaim before a fork batch overwrites a
// dead-but-live recovery record); WorktreeRoot is the host repo checkout
// BeginBatch captures HeadSHA from; WebsterDir, ReportsDir, and PromptsDir
// are the hubgeometry-resolved _lyx/webster, _lyx/webster/reports, and
// _lyx/webster/prompts directories.
type BeginDeps struct {
	Plan         *builderengine.Plan
	State        *State
	Roles        map[Role]modelspec.Resolved
	Config       Config
	Engine       shuttleengine.Engine
	Injector     Injector
	Mux          shuttleengine.MuxOps
	WorktreeRoot string
	WebsterDir   string
	ReportsDir   string
	PromptsDir   string
}

// BeginResult is what one successful BeginBatch call hands back to its
// caller (internal/webstercli's begin-batch verb): exactly what that caller
// needs to weft-commit state.json at the batch boundary without re-deriving
// any of it from deps.State itself.
type BeginResult struct {
	// BatchName is the batch's "NN-<batch-slug>" identifier — the
	// (possibly restart-chain-re-pointed) batch BeginBatch actually began.
	BatchName string
	// PromptPath is the absolute path of the fork prompt file BeginBatch
	// just wrote — what the caller's Agent-tool fork call reads.
	PromptPath string
	// StartSHA is the host HEAD captured immediately before this call
	// returns — the same value now recorded as this batch's
	// BatchState.StartSHA.
	StartSHA string
	// AssertedModel is the model BeginBatch asserted Master's pane onto for
	// this batch (State.AssertedModel's new value), regardless of whether
	// an injection actually fired.
	AssertedModel string
}

// findBatch returns the PlanBatch in plan whose Number matches number, or an
// error naming the missing number.
func findBatch(plan *builderengine.Plan, number int) (builderengine.PlanBatch, error) {
	for _, b := range plan.Batches {
		if b.Number == number {
			return b, nil
		}
	}
	return builderengine.PlanBatch{}, fmt.Errorf("webster: batch %d not found in plan", number)
}

// digestSummaryLine renders d into the fixed one-line summary
// RenderForkPrompt's prevDigest parameter expects: batch name, status,
// tests, and files_changed always; stuck_reason and drift_unreported
// appended only when the digest actually carries them. Returns "" for a nil
// d (no persisted digest yet), which RenderForkPrompt itself turns into the
// "none (first batch)" sentinel — this function never renders that sentinel
// itself, so the one fallback-wording decision lives in exactly one place.
func digestSummaryLine(d *builderengine.Digest) string {
	if d == nil {
		return ""
	}

	line := fmt.Sprintf("%s: %s tests=%s files_changed=%d", d.Batch, d.Status, d.Tests, d.FilesChanged)
	if d.StuckReason != "" {
		line += fmt.Sprintf(" stuck_reason=%q", d.StuckReason)
	}
	if len(d.DriftUnreported) > 0 {
		line += fmt.Sprintf(" drift_unreported=%s", strings.Join(d.DriftUnreported, ","))
	}
	return line
}

// BeginBatch drives one begin-batch call to completion, immediately before
// Master forks batchNumber's implementer: the pause gate, the fingerprint
// gate, the optional --restart-chain reset (re-pointing at the chain's
// lowest member per builder's own rule), start-SHA capture and first-member
// chain-anchor recording, the idempotent per-batch model assertion, the
// previous batch's persisted digest rendered into the fork prompt, and the
// prompt file write itself. The caller holds the state-mutation lease
// across this whole call and is responsible for persisting deps.State via
// SaveState once BeginBatch returns successfully — BeginBatch itself never
// calls SaveState and never touches weft.
func BeginBatch(deps BeginDeps, batchNumber int, restartChain bool) (*BeginResult, error) {
	if builderengine.PauseRequested(deps.WebsterDir) {
		return nil, ErrPaused
	}

	fingerprint, err := builderengine.Fingerprint(deps.Plan.Dir)
	if err != nil {
		return nil, err
	}
	if deps.State.PlanFingerprint != fingerprint {
		return nil, fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; the plan changed since state.json was created — re-run `lyx webster run --fresh` to archive the stale state and reports and start over", ErrFingerprintMismatch, fingerprint, deps.State.PlanFingerprint)
	}

	if restartChain {
		lowest, err := RestartChain(deps.Mux, deps.WorktreeRoot, deps.State, deps.Plan, batchNumber, deps.ReportsDir)
		if err != nil {
			return nil, err
		}
		// The chain always restarts from its lowest member, regardless of
		// which member the caller named — the same re-point rule builder's
		// own spawn-batch applies.
		batchNumber = lowest
	}

	batch, err := findBatch(deps.Plan, batchNumber)
	if err != nil {
		return nil, err
	}

	// Builder's pre-existing-report refusal, applied to the fork path: a
	// batch whose report already landed is finished work — silently
	// overwriting its BatchState (and letting a fresh fork overwrite the
	// report) must never happen by accident. Every legitimate re-begin path
	// arrives here report-free: --restart-chain deleted its members' reports
	// above, a no_report re-fork never calls begin-batch again (the bracket
	// is still open), and a crash-resumed re-drive targets the first batch
	// WITHOUT a report by definition.
	existingReport := filepath.Join(deps.ReportsDir, builderengine.BatchReportFileName(batch.Number, batch.Slug))
	if _, statErr := os.Stat(existingReport); statErr == nil {
		return nil, fmt.Errorf("webster: batch %02d-%s already has a report at %s — begin-batch never overwrites finished work; a stuck batch escalates via `lyx webster recover-batch %d` (which archives the report), and a stuck deferred-verify chain restarts via `begin-batch --restart-chain`", batch.Number, batch.Slug, existingReport, batch.Number)
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("webster: stat batch report %s: %w", existingReport, statErr)
	}

	head, err := builderengine.HeadSHA(deps.WorktreeRoot)
	if err != nil {
		return nil, err
	}

	if chainEnd := builderengine.ChainEndFor(deps.Plan, batch.Number); chainEnd != 0 {
		if deps.State.ChainStartSHAs == nil {
			deps.State.ChainStartSHAs = map[int]string{}
		}
		if _, recorded := deps.State.ChainStartSHAs[chainEnd]; !recorded {
			// The anchor is this HEAD: the host commit immediately before
			// the chain's first member ever forks. Recorded once, at
			// whichever member begins first — never overwritten by a later
			// member's own begin-batch call.
			deps.State.ChainStartSHAs[chainEnd] = head
		}
	}

	target := RoleMaster
	if batch.Oversized {
		target = RoleMasterOversized
	}
	resolved, ok := deps.Roles[target]
	if !ok {
		return nil, fmt.Errorf("webster: no resolved model-spec for role %q", target)
	}
	targetModel := resolved.Model

	// The ONLY model-injection site in webster (discussion.md
	// oversized-model-escalation): idempotent against State.AssertedModel,
	// so a resumed or repeated begin-batch call for the same batch never
	// re-injects a switch Master's pane is already running.
	if deps.State.AssertedModel != targetModel {
		if err := deps.Injector.Inject(deps.State.MasterStrand, deps.Engine.ModelSwitchSequence(targetModel)); err != nil {
			return nil, fmt.Errorf("webster: inject model switch for batch %d: %w", batchNumber, err)
		}
		deps.State.AssertedModel = targetModel
	}

	var prevDigest string
	if batchNumber > 1 {
		if prev, ok := deps.State.Batches[batchNumber-1]; ok && prev != nil {
			prevDigest = digestSummaryLine(prev.Digest)
		}
	}

	batchName := fmt.Sprintf("%02d-%s", batch.Number, batch.Slug)
	reportPath, err := filepath.Abs(filepath.Join(deps.ReportsDir, builderengine.BatchReportFileName(batch.Number, batch.Slug)))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve report path: %w", err)
	}

	prompt, err := RenderForkPrompt(batch, deps.Plan.Dir, prevDigest, reportPath, deps.WorktreeRoot, deps.Config.SelfFixCap)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(deps.PromptsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create prompts dir %s: %w", deps.PromptsDir, err)
	}
	promptPath, err := filepath.Abs(filepath.Join(deps.PromptsDir, batchName+".md"))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve prompt path: %w", err)
	}
	// The prompt file is a re-renderable artifact, never a durable record —
	// overwriting an existing one (a re-run begin-batch for the same batch)
	// is expected, not an error.
	if err := os.WriteFile(promptPath, prompt, 0o644); err != nil {
		return nil, fmt.Errorf("webster: write fork prompt %s: %w", promptPath, err)
	}

	if deps.State.Batches == nil {
		deps.State.Batches = map[int]*BatchState{}
	}
	// If a prior recovery attempt for this batch left a recorded strand (a
	// dead classification keeps its substrate alive by design, and it may
	// still be genuinely working), stop it before the record below erases
	// its StrandGUID: an unreclaimed recovery strand would race this batch's
	// fresh fork on the host repo — the same kept-strand reclaim builder's
	// respawn path performs. A plain fork batch's record has an empty
	// StrandGUID and RemoveStrandIfLive no-ops on it.
	if prior, ok := deps.State.Batches[batch.Number]; ok && prior != nil && prior.StrandGUID != "" {
		if err := builderengine.RemoveStrandIfLive(deps.Mux, prior.StrandGUID); err != nil {
			return nil, err
		}
	}
	deps.State.Batches[batch.Number] = &BatchState{
		Slug:      batch.Slug,
		StartSHA:  head,
		Kind:      "fork",
		SpawnedAt: time.Now().UTC().Format(time.RFC3339),
		// Stamp the opening Master session so the run-exit audit cross-check
		// can scope its begun-batch count to the session whose forks the
		// whole-session audit actually covers.
		SessionID: deps.State.MasterSessionID,
	}
	deps.State.CurrentBatch = batch.Number

	return &BeginResult{
		BatchName:     batchName,
		PromptPath:    promptPath,
		StartSHA:      head,
		AssertedModel: deps.State.AssertedModel,
	}, nil
}
