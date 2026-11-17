// beginbatch.go implements BeginBatch, the first of the two bracket verbs around each in-session fork;
// it runs the fingerprint refusal gate and captures the start SHA;
// it renders the previous batch's persisted digest into the fork prompt and writes the prompt file.
// BeginBatch never touches fabric and never persists deps.State itself.
// The caller holds the state-mutation lease (AcquireStateMutation) across its whole begin-batch call,
// and it saves state via SaveState once BeginBatch returns successfully.
// Master's model is set once at spawn and BeginBatch never reads or changes it.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// fingerprintMismatchWayForward is the trailing clause BeginBatch and Run put on an ErrFingerprintMismatch wrap.
// It reads the changed plan files, so the clause names the cards to pass to rebaseline.
// A state without PlanFileHashes names rebaseline without card numbers.
// A runplan.ChangedPlanFiles error falls back to the generic text.
// An overview change confined to the Card Index names rebaseline with the changed and added cards,
// and any other overview change names the follow-up card landing.
// The reset route it names ends in reentry.
func fingerprintMismatchWayForward(st *State, planDir, websterDir, reentry string) string {
	fresh := freshRestartSteps(reentry)
	const restore = `or restore the plan the run recorded with "lym webster restore-plan", `
	if len(st.PlanFileHashes) == 0 {
		return "way forward: if the edit keeps every begun batch's cards, run `lym webster rebaseline` to accept it, " + restore + "otherwise " + fresh
	}
	changed, err := runplan.ChangedPlanFiles(&st.State, planDir)
	if err != nil {
		return "way forward: if the edit keeps every begun batch's cards, run `lym webster rebaseline --card NN` naming each card you edited, " + restore + "otherwise " + fresh
	}
	var flags []string
	indexChanged := false
	for _, name := range changed {
		if name == runplan.PlanOverviewFile {
			recorded, _, _, err := recordedOverviewFrame(st, websterDir)
			indexOnly := false
			if err == nil {
				indexOnly, err = overviewIndexOnly(recorded, planDir)
			}
			if err != nil {
				return "way forward: if the edit keeps every begun batch's cards, run `lym webster rebaseline --card NN` naming each card you edited, " + restore + "otherwise " + fresh
			}
			if !indexOnly {
				if recorded == "" {
					return "way forward: " + runplan.PlanOverviewFile + " changed and is never rebaselined; restore it with \"lym webster restore-plan\", or " + fresh
				}
				return "way forward: " + runplan.PlanOverviewFile + " changed outside its Card Index; restore it with \"lym webster restore-plan\", or " + followUpCardLanding + ", or " + fresh
			}
			indexChanged = true
			continue
		}
		flags = append(flags, "--card "+cardNumberOf(name))
	}
	if len(flags) == 0 {
		if indexChanged {
			return "way forward: only the Card Index of " + runplan.PlanOverviewFile + " changed; run `lym webster rebaseline` to accept it, " + restore + "otherwise " + fresh
		}
		return "way forward: run `lym webster rebaseline --card NN` naming each card you edited, " + restore + "otherwise " + fresh
	}
	return "way forward: run `lym webster rebaseline " + strings.Join(flags, " ") + "` to accept the edit, " + restore + "otherwise " + fresh
}

// cardNumberOf returns the digits before the first "-" of a card file name, or the whole name when it has none.
func cardNumberOf(name string) string {
	num, _, _ := strings.Cut(name, "-")
	return num
}

// BeginDeps carries every seam BeginBatch needs, so a test can fake each one independently.
// Plan is the already-parsed plan.
// Batches is the execution order, read from the recorded partition by ExecutionBatches.
// The previous batch's digest lookup depends on Batches already being in that order.
// State is the already-loaded run state BeginBatch reads and mutates.
// Config is the loaded webster.yaml.
// Stopper is the seam that stops a leftover recovery strand:
// a dead but still live one that a fork batch is about to overwrite.
// Geom supplies every path BeginBatch reads.
// WorktreeRoot is the git worktree that HeadSHA is captured from and the plan's card paths resolve against.
// PaneCwd is the worktree root the fork prompt renders with.
// WebsterDir and ReportsDir locate the reports directory.
// PromptsDir and StencilsDir feed the prompt write and the fork template's read location.
type BeginDeps struct {
	Plan    *planparser.Plan
	Batches []batcher.Batch
	State   *State
	Config  Config
	Stopper StrandStopper
	Geom    Geometry

	// FrictionDir is the absolute friction directory (see internal/friction), empty when Tier 2 is off.
	// It needs no geometry derivation.
	FrictionDir string

	// GateTiers are the hub's rendered tier and gate-list lines that the fork prompt's gate_tiers marker takes;
	// it is "none" when no tier is declared.
	GateTiers string

	// RenameTools are the hub's structure-aware rename tools the fork prompt renders; empty for none.
	RenameTools []string

	// Gate is the hub's gate config, from which the fork prompt renders its card and batch gate calls.
	Gate gateslot.Config

	// CardFixer reports whether a failed card gate call goes through a cold fixer first.
	CardFixer bool
}

// BeginResult is what one successful BeginBatch call returns to its caller.
type BeginResult struct {
	// BatchName is the batch's "NN-<batch-slug>" identifier.
	BatchName string
	// PromptPath is the absolute path of the fork prompt file BeginBatch just wrote.
	PromptPath string
	// StartSHA is the repo HEAD captured before this call returns.
	// A re-begin over a record that already carries one keeps that earlier value,
	// so it stays the HEAD from before the batch's first fork.
	StartSHA string
	// Advisories are the informational findings the re-resolution reported; the run continues past them.
	Advisories []string
	// ArchivedReport is the path a report left with no begin-batch record was archived to, empty when there was none.
	ArchivedReport string
}

// ErrRenameToolMissing is the sentinel error a refusal wraps.
// It applies when a batch holds a symbol Rename pair and the hub lists no rename tool.
var ErrRenameToolMissing = errors.New("webster: the batch holds a symbol Rename pair and the hub lists no rename tool")

// renameToolRefusal refuses a batch holding a symbol Rename pair when renameTools is empty.
// The fork has no tool to run the rename with.
// It returns nil when renameTools is non-empty or the batch holds only file pairs.
// verb names the webster verb that the way forward re-runs.
func renameToolRefusal(plan *planparser.Plan, batch batcher.Batch, renameTools []string, verb string) error {
	if len(renameTools) > 0 {
		return nil
	}
	number, slug := runplan.BatchIdentity(batch)
	for _, card := range batch.Cards {
		pairs := planparser.SymbolRenamePairs(plan, []planparser.Card{card})
		if len(pairs) == 0 {
			continue
		}
		return fmt.Errorf("%w: batch %02d-%s, symbol pair %s -> %s on card %s; way forward: set rename_tools with \"lym config language\" from the prime, or recast the card, then re-run `lym webster %s %d`", ErrRenameToolMissing, number, slug, pairs[0].Old, pairs[0].New, card.ID(), verb, number)
	}
	return nil
}

// digestSummaryLine renders d as the one-line summary a fork prompt takes as its previous digest.
// A nil d renders as the empty string.
func digestSummaryLine(d *Digest) string {
	if d == nil {
		return ""
	}

	line := fmt.Sprintf("%s: %s head_sha=%s", d.Batch, d.Status, d.HeadSHA)
	if len(d.Deviations) > 0 {
		line += fmt.Sprintf(" scope-guard deviations=%s", strings.Join(d.Deviations, ","))
	}
	if len(d.Departures) > 0 {
		line += fmt.Sprintf(" pattern departures=%s", strings.Join(d.Departures, "; "))
	}
	return line
}

// predecessorDigestLine renders the digest of the batch that ran immediately before batchNumber in execution order.
// batches must already be in execution order, which need not match batch number order.
// The predecessor is found by position: batchNumber-1 is right only while batch number and execution position coincide.
// It locates batchNumber's position in batches by runplan.BatchIdentity.
// It returns "" when the batch is absent from batches or sits at index 0, which means nothing ran before it.
// It also returns "" when the predecessor's state entry or its digest is absent.
// A nil st or nil st.Batches also yields "".
func predecessorDigestLine(batches []batcher.Batch, st *State, batchNumber int) string {
	idx := -1
	for i, b := range batches {
		if n, _ := runplan.BatchIdentity(b); n == batchNumber {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return ""
	}
	if st == nil || st.Batches == nil {
		return ""
	}

	prevNumber, _ := runplan.BatchIdentity(batches[idx-1])
	prev, ok := st.Batches[prevNumber]
	if !ok || prev == nil {
		return ""
	}
	return digestSummaryLine(prev.Digest)
}

// existingReportRemedy names the one step the recorded state of batch number calls for.
// It applies when begin-batch finds the batch's report already on disk.
// retry reports whether a dead recovery should be retried; if so, the step names recover-batch once more.
// For a terminal record other than done, the wording follows recover-batch's cases in order.
// The exhausted case comes first and also reports true.
// Then come a dead recovery with a retry, any other dead recovery, and a stuck or failed record.
func existingReportRemedy(number int, recorded *BatchState, retry bool) (string, bool) {
	if !recorded.Terminal {
		if recorded.Kind == "recovery" {
			return fmt.Sprintf("`lym webster recover-batch %d`", number), false
		}
		return fmt.Sprintf("`lym webster record-batch %d`, after fixing whatever its last refusal named", number), false
	}
	if recorded.Status == DigestStatusDone {
		return "the batch is finished, so begin the next batch", false
	}
	if recorded.Recoveries >= maxRecoveries && !rendersNewAmendment(recorded) {
		return fmt.Sprintf("the batch's recoveries are exhausted, so end the run stuck naming the batch; for the operator, %s; or %s",
			strings.TrimPrefix(wayForwardSteps(resetVerb(ResetToBatchStart, number), fmt.Sprintf("lym webster recover-batch %d", number)), "way forward: "), freshRestartSteps(stepRun)), true
	}
	if recorded.Status == DigestStatusDead {
		if retry {
			return fmt.Sprintf("`lym webster recover-batch %d`, once more, since the batch earns another recovery", number), false
		}
		return fmt.Sprintf("the recovery of batch %d is dead and earns no retry, so end the run stuck naming the batch", number), false
	}
	return fmt.Sprintf("`lym webster recover-batch %d`", number), false
}

// BeginBatch drives one begin-batch call to completion, immediately before Master forks batchNumber's implementer.
// It runs the fingerprint gate and captures the start SHA;
// it renders the previous batch's persisted digest into the fork prompt, and it writes the prompt file.
// The caller holds the state-mutation lease across this whole call,
// and it must persist deps.State via SaveState once BeginBatch returns successfully.
// BeginBatch never calls SaveState and never touches fabric.
func BeginBatch(deps BeginDeps, batchNumber int) (*BeginResult, error) {
	// The plan is a hard precondition, refused loudly rather than dereferenced below.
	// A nil here is a wiring mistake in a caller,
	// and a nil-pointer panic names neither the missing field nor the verb that failed to supply it.
	if deps.Plan == nil {
		return nil, fmt.Errorf("webster: begin-batch requires a parsed plan; BeginDeps.Plan is nil")
	}
	// State is a hard precondition too: it is dereferenced below, so a nil one is refused here, not left to panic.
	if deps.State == nil {
		return nil, fmt.Errorf("webster: begin-batch requires loaded run state; BeginDeps.State is nil")
	}

	if err := PlanEditError(deps.State, deps.Plan.Dir, deps.Geom.WebsterDir); err != nil {
		return nil, err
	}

	// An unknown batch falls through to the findBatch refusal below.
	if requested, findErr := findBatch(deps.Batches, batchNumber); findErr == nil {
		if err := renameToolRefusal(deps.Plan, requested, deps.RenameTools, "begin-batch"); err != nil {
			return nil, err
		}
	}

	// Re-resolve the plan against the current tree before the fork prompt is written.
	// The batch record is written further down, so on a first begin this batch is still validated.
	advisories, err := runplan.ResolveAtDispatch(deps.Geom.runPlan(), deps.Plan, deps.Batches, &deps.State.State, batchNumber, planConflictWayForward(batchNumber))
	if err != nil {
		return nil, err
	}

	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := runplan.BatchIdentity(batch)

	cardHashes, err := runplan.BatchCardHashes(batch, deps.Plan.Dir)
	if err != nil {
		return nil, fmt.Errorf("%w; way forward: transient, re-run `lym webster begin-batch %d`", err, batchNumber)
	}

	// The gate calls render before the first mutation,
	// so a gate list the config lacks refuses with no prompt written and no state recorded.
	cardGates, err := runplan.RenderCardGates(batch.Cards, masterPlanDirDisplay(deps.Geom.PaneCwd, deps.Geom.PlanDir), deps.Gate)
	if err != nil {
		return nil, err
	}
	batchGate, err := runplan.RenderBatchGate(batch.Cards, deps.Gate, runplan.IsLastBatch(deps.Batches, batch))
	if err != nil {
		return nil, err
	}

	// The fork writes its report here with whatever tool it likes;
	// a plain shell redirect never creates missing parents, unlike an agent Write tool.
	// The directory must therefore exist on every begin.
	if err := os.MkdirAll(deps.Geom.ReportsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create reports dir %s: %w", deps.Geom.ReportsDir, err)
	}

	// A batch whose report already landed is finished work.
	// begin-batch must never overwrite its report or its BatchState by accident.
	// A no_report re-fork never calls begin-batch again, because the bracket is still open.
	// The one exception is a run resumed after a crash that landed between the fork's report and record-batch:
	// it re-drives a batch whose report is on disk; record-batch consumes that report,
	// and the audit keys on the bracket-opening session.
	// So the refusal names the one remedy the recorded state calls for.
	// Only a batch with no record in state.json has its report archived and the begin proceeds.
	// Such a report cannot be attributed to any begun batch.
	// Otherwise record-batch would only archive it and send the caller back here.
	// A recorded batch's report is never archived by begin-batch.
	existingReport := filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug))
	var archivedReport string
	if _, statErr := os.Stat(existingReport); statErr == nil {
		recorded := deps.State.Batches[number]
		if recorded == nil {
			archivedReport, err = archiveStaleReport(deps.Geom.ReportsDir, number, slug, time.Now)
			if err != nil {
				return nil, err
			}
		} else {
			seen := "begun and not terminal"
			if recorded.Terminal {
				seen = "terminal with status " + recorded.Status
			}
			retry, _ := RecoveryRetry(deps.Geom, deps.State, number)
			remedy, exhausted := existingReportRemedy(number, recorded, retry)
			refusal := fmt.Errorf("webster: batch %02d-%s already has a report at %s and state.json records the batch as %s — begin-batch never overwrites finished work; way forward: %s", number, slug, existingReport, seen, remedy)
			if exhausted {
				return nil, fmt.Errorf("%w: %w", ErrRecoveryExhausted, refusal)
			}
			return nil, refusal
		}
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("webster: stat batch report %s: %w", existingReport, statErr)
	}

	head, err := deps.Geom.git().HeadSHA(deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}

	prevDigest := predecessorDigestLine(deps.Batches, deps.State, batchNumber)

	batchName := fmt.Sprintf("%02d-%s", number, slug)
	reportPath, err := filepath.Abs(filepath.Join(deps.Geom.ReportsDir, ReportFileName(number, slug)))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve report path: %w", err)
	}

	notePath := friction.NotePath(deps.FrictionDir, batchName)
	promptPath, err := filepath.Abs(filepath.Join(deps.Geom.PromptsDir, batchName+".md"))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve prompt path: %w", err)
	}
	prompt, err := RenderForkPrompt(batch, cardGates, batchGate, prevDigest, reportPath, deps.Geom.PlanDir, deps.Geom.PaneCwd, deps.Geom.StencilsDir, deps.Geom.SpecsDir, deps.GateTiers, deps.RenameTools, OutcomePath(deps.Geom.WebsterDir), summaryparser.Path(deps.Geom.WebsterDir), deps.Config.SelfFixCap, notePath, deps.CardFixer)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(deps.Geom.PromptsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create prompts dir %s: %w", deps.Geom.PromptsDir, err)
	}
	// The prompt file is a re-renderable artifact, never a durable record.
	// Overwriting an existing one, as a re-run of begin-batch for the same batch does, is expected, not an error.
	if err := os.WriteFile(promptPath, prompt, 0o644); err != nil {
		return nil, fmt.Errorf("webster: write fork prompt %s: %w", promptPath, err)
	}

	if deps.State.Batches == nil {
		deps.State.Batches = map[int]*BatchState{}
	}
	// A prior recovery attempt for this batch may have left a recorded strand.
	// A dead classification keeps its substrate alive by design, and that strand may still be working.
	// Stop it before the record below erases its StrandGUID:
	// an unreclaimed recovery strand would race this batch's fresh fork on the repo,
	// so the respawn path reclaims the kept strand first.
	// A plain fork batch's record has an empty StrandGUID, which the reclaim skips.
	if prior, ok := deps.State.Batches[number]; ok && prior != nil && prior.StrandGUID != "" {
		if err := deps.Stopper.StopStrand(prior.StrandGUID); err != nil {
			return nil, fmt.Errorf("websterengine: stop prior recovery strand %s before respawn: %w", prior.StrandGUID, err)
		}
	}

	// A re-begin keeps the StartSHA the batch was first recorded with:
	// the captured head may already sit past commits an earlier fork landed,
	// and the recorded start must name the base of the whole bracket.
	// recover-batch applies the same inheritance to a recovery record.
	startSHA := head
	prior := deps.State.Batches[number]
	if prior != nil && prior.StartSHA != "" {
		startSHA = prior.StartSHA
	}
	// Recorded warnings carry over too: their identities stay dispositioned, so no later call would record them again.
	// The fork transcripts already attributed to the batch carry over,
	// so the run-exit audit still knows which report each of those forks owns.
	// The recovery count and its start commit carry over as well,
	// so a re-begin does not hand a batch a fresh pair of recoveries.
	var priorWarnings []AuditWarning
	var priorTranscripts []string
	var priorRecoveries int
	var priorRecoveryStart string
	var priorRecoverySessions []string
	var priorRecoveryEngines map[string]string
	if prior != nil {
		priorRecoverySessions = prior.RecoverySessions
		priorRecoveryEngines = prior.RecoveryEngines
		priorWarnings = prior.AuditWarnings
		priorTranscripts = prior.ForkTranscripts
		priorRecoveries = prior.Recoveries
		priorRecoveryStart = prior.RecoveryStartSHA
	}

	deps.State.Batches[number] = &BatchState{
		Slug:             slug,
		Cards:            runplan.BatchCardIDs(batch),
		CardHashes:       cardHashes,
		StartSHA:         startSHA,
		Kind:             "fork",
		AuditWarnings:    priorWarnings,
		ForkTranscripts:  priorTranscripts,
		Recoveries:       priorRecoveries,
		RecoverySessions: priorRecoverySessions,
		RecoveryEngines:  priorRecoveryEngines,
		RecoveryStartSHA: priorRecoveryStart,
		SpawnedAt:        time.Now().UTC().Format(time.RFC3339),
		// Stamp the opening Master session.
		// The run-exit audit cross-check scopes its begun-batch count to that session;
		// the whole-session audit covers that session's forks.
		SessionID: deps.State.MasterSessionID,
		Engine:    deps.State.MasterEngine,
	}
	deps.State.CurrentBatch = number

	return &BeginResult{
		BatchName:      batchName,
		PromptPath:     promptPath,
		StartSHA:       startSHA,
		Advisories:     advisories,
		ArchivedReport: archivedReport,
	}, nil
}
