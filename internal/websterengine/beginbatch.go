// beginbatch.go implements BeginBatch, the first of webster's two bracket verbs Master calls around
// each in-session fork: the pause and fingerprint refusal gates, start-SHA capture, the idempotent
// per-batch model assertion (the ONLY model-injection site in webster — see doc.go's package
// comment), the previous batch's persisted digest rendered into the fork prompt, and the prompt
// file write itself.
// BeginBatch never touches fabric (webster is fabric-blind throughout) and never persists deps.State
// itself — the caller holds the state-mutation lease (AcquireStateMutation) across its whole
// begin-batch call and saves state via SaveState once BeginBatch returns successfully, webster's
// own fabric-commit-boundary discipline.
// Under the flat card-list model there is no deferred-verify chain and no oversized-batch
// escalation: BeginBatch always asserts the single RoleMaster model,
// and there is no --restart-chain surface.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrPaused is the sentinel BeginBatch returns when deps.Geom.ScratchDir's pause flag is present at the
// batch boundary (PauseRequested).
// Exported so a caller can distinguish the operational "paused" refusal from every other
// begin-batch failure via errors.Is(err, ErrPaused) — webster's own sentinel, per the
// webster-owns-its-own-domain-types decision.
var ErrPaused = errors.New("webster: paused")

// ErrFingerprintMismatch is the sentinel BeginBatch returns when the on-disk plan's recomputed
// fingerprint disagrees with State.PlanFingerprint — webster's own crash/resume guard, with its own
// sentinel identity (webster-owns-its-own-domain-types).
var ErrFingerprintMismatch = errors.New("webster: on-disk plan fingerprint does not match this run's recorded state")

// Injector is the seam BeginBatch uses to switch Master's live pane to a different model: exactly
// (*shuttleengine.Runner).Inject's signature, so production code passes a real
// *shuttleengine.Runner directly and tests pass a fake that records every (guid, inputs) call.
type Injector interface {
	Inject(guid string, inputs []shuttleengine.PaneInput) error
}

// BeginDeps carries every seam BeginBatch needs, so a test can fake each one independently: Plan is
// the already-parsed plan;
// Batches is the batchifier-derived execution batches (see RunDeps.Batcher) `run` computed
// once at entry and threads through every bracket verb call;
// State is the already-loaded run state BeginBatch reads and mutates;
// Roles is the pre-flight-resolved role->model-spec map (see ResolveRoles);
// Config is the loaded webster.yaml;
// Engine supplies the provider-specific ModelSwitchSequence choreography;
// Injector is what actually types that choreography into Master's pane; Reed is the live reed query
// surface the prior-recovery-strand reclaim consults (a dead-but-live recovery record a fork batch
// is about to overwrite);
// Geom is the told Geometry BeginBatch reads every path from: WorktreeRoot is the repo checkout
// HeadSHA is captured from and RenderForkPrompt's promptWorktreeRoot, WebsterDir and ReportsDir are
// the reports directory, and PromptsDir and StencilsDir feed the prompt write and the fork
// template's read location.
type BeginDeps struct {
	Plan     *planparser.Plan
	Batches  []batcher.Batch
	State    *State
	Roles    map[Role]modelspec.Resolved
	Config   Config
	Engine   shuttleengine.Engine
	Injector Injector
	Reed     shuttleengine.ReedOps
	Geom     Geometry
}

// BeginResult is what one successful BeginBatch call returns to its caller.
type BeginResult struct {
	// BatchName is the batch's "NN-<batch-slug>" identifier.
	BatchName string
	// PromptPath is the absolute path of the fork prompt file BeginBatch just wrote.
	PromptPath string
	// StartSHA is the repo HEAD captured before this call returns.
	StartSHA string
	// AssertedModel is the model BeginBatch asserted Master's pane onto for this batch.
	AssertedModel string
}

// findBatch returns the batcher.Batch in batches whose identity matches number.
func findBatch(batches []batcher.Batch, number int) (batcher.Batch, error) {
	for _, b := range batches {
		if n, _ := batchIdentity(b); n == number {
			return b, nil
		}
	}
	return batcher.Batch{}, fmt.Errorf("webster: batch %d not found in the plan's execution batches", number)
}

// digestSummaryLine renders d into the one-line summary RenderForkPrompt's prevDigest parameter expects.
func digestSummaryLine(d *Digest) string {
	if d == nil {
		return ""
	}

	line := fmt.Sprintf("%s: %s head_sha=%s", d.Batch, d.Status, d.HeadSHA)
	if len(d.Deviations) > 0 {
		line += fmt.Sprintf(" deviations=%s", strings.Join(d.Deviations, ","))
	}
	return line
}

// BeginBatch drives one begin-batch call to completion, immediately before Master forks
// batchNumber's implementer: the pause gate, the fingerprint gate, start-SHA capture, the previous
// batch's persisted digest rendered into the fork prompt, the prompt file write itself, and — last,
// so an earlier failure never leaves the pane switched with nothing persisted — the idempotent
// per-batch model assertion.
// The caller holds the state-mutation lease across this whole call and is responsible for
// persisting deps.State via SaveState once BeginBatch returns successfully — BeginBatch itself
// never calls SaveState and never touches fabric.
func BeginBatch(deps BeginDeps, batchNumber int) (*BeginResult, error) {
	if PauseRequested(deps.Geom.ScratchDir) {
		return nil, ErrPaused
	}

	fp, err := fingerprint(deps.Plan.Dir)
	if err != nil {
		return nil, err
	}
	if deps.State.PlanFingerprint != fp {
		return nil, fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; the plan changed since state.json was created — re-run `lyx webster run --fresh` to archive the stale state and reports and start over", ErrFingerprintMismatch, fp, deps.State.PlanFingerprint)
	}

	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := batchIdentity(batch)

	// The fork writes its report here with whatever tool it likes — a plain
	// shell redirect included, which unlike an agent Write tool never creates
	// missing parents. Only the --fresh archive path recreated this dir
	// before; the ordinary first run left it absent (found live in crucible
	// round fable-r1).
	if err := os.MkdirAll(deps.Geom.ReportsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create reports dir %s: %w", deps.Geom.ReportsDir, err)
	}

	// webster's own pre-existing-report refusal, applied to the fork path: a
	// batch whose report already landed is finished work — silently
	// overwriting its BatchState (and letting a fresh fork overwrite the
	// report) must never happen by accident. A no_report re-fork never
	// calls begin-batch again (the bracket is still open), with ONE
	// exception: a run resumed after a crash that landed between the
	// fork's report and record-batch re-drives a batch whose report IS on
	// disk — that report is consumed by record-batch (the audit keys on
	// the bracket-opening session, see RecordBatch), so the refusal
	// message names that recourse alongside the stuck-batch one.
	existingReport := filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug))
	if _, statErr := os.Stat(existingReport); statErr == nil {
		return nil, fmt.Errorf("webster: batch %02d-%s already has a report at %s — begin-batch never overwrites finished work; a report left behind by a crashed session is consumed by `lyx webster record-batch %d` (or `lyx webster recover-batch %d` for a recovery batch), and a stuck batch escalates via `lyx webster recover-batch %d` (which archives the report)", number, slug, existingReport, number, number, number)
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("webster: stat batch report %s: %w", existingReport, statErr)
	}

	head, err := headSHA(deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}

	target := RoleMaster
	resolved, ok := deps.Roles[target]
	if !ok {
		return nil, fmt.Errorf("webster: no resolved model-spec for role %q", target)
	}
	targetModel := resolved.Model

	var prevDigest string
	if batchNumber > 1 {
		if prev, ok := deps.State.Batches[batchNumber-1]; ok && prev != nil {
			prevDigest = digestSummaryLine(prev.Digest)
		}
	}

	batchName := fmt.Sprintf("%02d-%s", number, slug)
	reportPath, err := filepath.Abs(filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug)))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve report path: %w", err)
	}

	// WorktreeRoot, not AnchorRoot, is correct in both modes here: hub
	// mode's WorktreeRoot is the anchor path, the exact value this call
	// rendered before this Geometry split.
	prompt, err := RenderForkPrompt(batch, prevDigest, reportPath, deps.Geom.WorktreeRoot, deps.Geom.StencilsDir, deps.Config.SelfFixCap)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(deps.Geom.PromptsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create prompts dir %s: %w", deps.Geom.PromptsDir, err)
	}
	promptPath, err := filepath.Abs(filepath.Join(deps.Geom.PromptsDir, batchName+".md"))
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
	// fresh fork on the repo, so this respawn path reclaims the kept strand
	// first. A plain fork batch's record has an empty
	// StrandGUID and removeStrandIfLive no-ops on it.
	if prior, ok := deps.State.Batches[number]; ok && prior != nil && prior.StrandGUID != "" {
		if err := removeStrandIfLive(deps.Reed, prior.StrandGUID); err != nil {
			return nil, err
		}
	}

	// The ONLY model-injection site in webster: idempotent against
	// State.AssertedModel, so a resumed or repeated begin-batch call for
	// the same batch never re-injects a switch Master's pane is already
	// running. Deliberately the LAST fallible act of this call — every
	// earlier step (prompt render and write, strand reclaim) can still fail
	// without the pane having been switched, so the pane's model and the
	// persisted AssertedModel can never diverge across an error return:
	// either the injection and its memory both happen (only infallible
	// in-memory recording remains below) or neither does.
	if deps.State.AssertedModel != targetModel {
		if err := deps.Injector.Inject(deps.State.MasterStrand, deps.Engine.ModelSwitchSequence(targetModel)); err != nil {
			return nil, fmt.Errorf("webster: inject model switch for batch %d: %w", batchNumber, err)
		}
		deps.State.AssertedModel = targetModel
	}

	deps.State.Batches[number] = &BatchState{
		Slug:      slug,
		StartSHA:  head,
		Kind:      "fork",
		SpawnedAt: time.Now().UTC().Format(time.RFC3339),
		// Stamp the opening Master session so the run-exit audit cross-check
		// can scope its begun-batch count to the session whose forks the
		// whole-session audit actually covers.
		SessionID: deps.State.MasterSessionID,
	}
	deps.State.CurrentBatch = number

	return &BeginResult{
		BatchName:     batchName,
		PromptPath:    promptPath,
		StartSHA:      head,
		AssertedModel: deps.State.AssertedModel,
	}, nil
}
