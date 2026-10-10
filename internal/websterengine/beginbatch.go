// beginbatch.go implements BeginBatch, the first of webster's two bracket verbs Master calls around each in-session fork: the pause and fingerprint refusal gates, start-SHA capture, the previous batch's persisted digest rendered into the fork prompt, and the prompt file write itself.
// BeginBatch never touches fabric (webster is fabric-blind throughout) and never persists deps.State
// itself — the caller holds the state-mutation lease (AcquireStateMutation) across its whole
// begin-batch call and saves state via SaveState once BeginBatch returns successfully, webster's
// own fabric-commit-boundary discipline.
// Master's model is set once at spawn and BeginBatch never reads or changes it.
// Under the flat card-list model there is no deferred-verify chain and no oversized-batch escalation, and there is no --restart-chain surface.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/summaryparser"
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

// planOverviewFile is the plan's overview file, which carries the integration verify; rebaseline accepts a change to its Card Index only.
const planOverviewFile = "00-overview.md"

// fingerprintMismatchWayForward is the trailing clause BeginBatch and Run put on an ErrFingerprintMismatch wrap.
// It reads the changed plan files so the clause names the cards to pass to rebaseline;
// a state without PlanFileHashes names rebaseline without card numbers, and a changedPlanFiles error falls back to the generic text.
// An overview change confined to the Card Index names rebaseline with the changed and added cards, and any other overview change names the follow-up card landing.
// The reset route it names ends in reentry.
func fingerprintMismatchWayForward(st *State, planDir, websterDir, reentry string) string {
	fresh := freshRestartSteps(reentry)
	const restore = `or restore the plan the run recorded with "lyx webster restore-plan", `
	if len(st.PlanFileHashes) == 0 {
		return "way forward: if the edit keeps every begun batch's cards, run `lyx webster rebaseline` to accept it, " + restore + "otherwise " + fresh
	}
	changed, err := changedPlanFiles(st, planDir)
	if err != nil {
		return "way forward: if the edit keeps every begun batch's cards, run `lyx webster rebaseline --card NN` naming each card you edited, " + restore + "otherwise " + fresh
	}
	var flags []string
	indexChanged := false
	for _, name := range changed {
		if name == planOverviewFile {
			recorded, _, _, err := recordedOverviewFrame(st, websterDir)
			indexOnly := false
			if err == nil {
				indexOnly, err = overviewIndexOnly(recorded, planDir)
			}
			if err != nil {
				return "way forward: if the edit keeps every begun batch's cards, run `lyx webster rebaseline --card NN` naming each card you edited, " + restore + "otherwise " + fresh
			}
			if !indexOnly {
				if recorded == "" {
					return "way forward: " + planOverviewFile + " changed and is never rebaselined; restore it with \"lyx webster restore-plan\", or " + fresh
				}
				return "way forward: " + planOverviewFile + " changed outside its Card Index; restore it with \"lyx webster restore-plan\", or " + followUpCardLanding + ", or " + fresh
			}
			indexChanged = true
			continue
		}
		flags = append(flags, "--card "+cardNumberOf(name))
	}
	if len(flags) == 0 {
		if indexChanged {
			return "way forward: only the Card Index of " + planOverviewFile + " changed; run `lyx webster rebaseline` to accept it, " + restore + "otherwise " + fresh
		}
		return "way forward: run `lyx webster rebaseline --card NN` naming each card you edited, " + restore + "otherwise " + fresh
	}
	return "way forward: run `lyx webster rebaseline " + strings.Join(flags, " ") + "` to accept the edit, " + restore + "otherwise " + fresh
}

// cardNumberOf returns the digits before the first "-" of a card file name, or the whole name when it has none.
func cardNumberOf(name string) string {
	num, _, _ := strings.Cut(name, "-")
	return num
}

// ErrPlanDrifted is the sentinel BeginBatch returns when the dispatch-boundary re-resolution (planindex.Index.ValidateDispatch, called against deps.Geom.WorktreeRoot with the completed cards excluded) reports a non-empty blocking findings set — webster's own sentinel, per the webster-owns-its-own-domain-types decision, so a caller distinguishes this refusal from ErrPaused and ErrFingerprintMismatch via errors.Is.
// Dispatching a pack built on a re-resolve that failed is strictly worse than not dispatching.
var ErrPlanDrifted = errors.New("webster: plan re-resolution at begin-batch reported a blocking finding")

// BeginDeps carries every seam BeginBatch needs, so a test can fake each one independently:
// Plan is the already-parsed plan;
// Batches is the execution order, the batchifier's own order read from the recorded partition by ExecutionBatches — predecessorDigestLine's lookup depends on Batches already being in that order;
// State is the already-loaded run state BeginBatch reads and mutates;
// Config is the loaded webster.yaml;
// Stopper is the seam the prior-recovery-strand reclaim stops a leftover strand through (a dead-but-live recovery record a fork batch is about to overwrite);
// Geom is the told Geometry BeginBatch reads every path from: WorktreeRoot is the repo checkout HeadSHA is captured from and RenderForkPrompt's promptWorktreeRoot, WebsterDir and ReportsDir are the reports directory,
// and PromptsDir and StencilsDir feed the prompt write and the fork template's read location.
type BeginDeps struct {
	Plan    *planparser.Plan
	Batches []batcher.Batch
	State   *State
	Config  Config
	Stopper StrandStopper
	Geom    Geometry

	// FrictionDir is the told absolute friction directory (see internal/friction), empty when Tier 2
	// is off. It lives here rather than on Geometry because internal/hubgeom and
	// internal/standalonegeom are the Told-Geometry Invariant's only Geometry-struct constructors and
	// this value needs no geometry derivation.
	FrictionDir string
}

// BeginResult is what one successful BeginBatch call returns to its caller.
type BeginResult struct {
	// BatchName is the batch's "NN-<batch-slug>" identifier.
	BatchName string
	// PromptPath is the absolute path of the fork prompt file BeginBatch just wrote.
	PromptPath string
	// StartSHA is the value recorded in the batch's state: the repo HEAD captured before this call returns,
	// or — on a re-begin over a record that already carries one — that earlier value,
	// so it stays the HEAD from before the batch's first fork.
	StartSHA string
	// Advisories is every informational finding the dispatch-boundary re-resolution (planindex.Index.ValidateDispatch) reported, rendered via Finding.Error, so an operator sees them without the run stopping — a non-empty blocking findings set never reaches this far, since it returns ErrPlanDrifted instead.
	Advisories []string
	// ArchivedReport is the path a report left with no begin-batch record was archived to, empty when there was none.
	ArchivedReport string
}

// completedCards returns every card belonging to a batch that has already reached a terminal
// classification, in batches' own order.
//
// It is what scopes the dispatch-boundary re-resolution (and record-batch's drift detection) to work
// that has NOT landed yet. A plan describes intended change, so a card already built necessarily
// contradicts the tree it would otherwise be re-resolved against — its Create target now exists, its
// Delete target is gone, its Rename's old side no longer resolves — and reporting that as a plan
// defect wedged every multi-batch plan carrying one of those card types.
// exclude, when non-zero, additionally counts that batch as completed: record-batch calls this while
// the batch it is recording is still non-terminal, and that batch's own work has just landed.
func completedCards(batches []batcher.Batch, st *State, exclude int) []planparser.Card {
	if st == nil {
		return nil
	}

	var done []planparser.Card
	for _, b := range batches {
		number, _ := batchIdentity(b)
		if number != exclude {
			bs, ok := st.Batches[number]
			if !ok || bs == nil || !bs.Terminal {
				continue
			}
		}
		done = append(done, b.Cards...)
	}
	return done
}

// DispatchScope computes the one dispatch scoping begin-batch, run entry and `lyx webster validate` share.
// begun is every card of a batch begin-batch recorded, terminal or not, in batches' own order.
// forthcoming is the cards of every begun batch whose record is not terminal.
// A terminal batch's cards are never forthcoming: its done-checks proved its targets present.
// A nil state returns two nil slices.
//
// A batch begun but never recorded (its record-batch refused, or the run died between the fork's commit and the record) may already have landed its work, so its Create targets can legitimately exist;
// validating it as unstarted reported them as create-already-exists and refused the very resume that would record the batch.
// Its targets may equally not have landed yet, which is why forthcoming exists:
// planindex.Index.ValidateDispatch excludes a forthcoming card's Create and Rename New targets from the status check, so a later card that Uses them is not refused.
// Master re-drives such a batch through record-batch or recover-batch, which apply their own checks.
func DispatchScope(batches []batcher.Batch, st *State) (begun, forthcoming []planparser.Card) {
	if st == nil {
		return nil, nil
	}

	for _, b := range batches {
		number, _ := batchIdentity(b)
		bs, ok := st.Batches[number]
		if !ok || bs == nil {
			continue
		}
		begun = append(begun, b.Cards...)
		if !bs.Terminal {
			forthcoming = append(forthcoming, b.Cards...)
		}
	}
	return begun, forthcoming
}

// DoneCards returns the cards of plan, in plan order, that belong to a batch st records terminal with status done.
// It reads the batch records alone, so it needs no batchifier: a record's own card ids name its cards, and a record written before those ids existed names the single card NN-<Slug>.
// A recorded id the plan lacks is skipped, so a card renamed since its batch ran counts as not done.
// A nil state returns nil.
//
// It is what the loom plan gate scopes its tree-dependent checks by: a done card's work is already in the tree, so it is history, not a target.
func DoneCards(plan *planparser.Plan, st *State) []planparser.Card {
	if st == nil {
		return nil
	}

	done := make(map[string]bool)
	for number, bs := range st.Batches {
		if bs == nil || !bs.Terminal || bs.Status != DigestStatusDone {
			continue
		}
		ids := bs.Cards
		if len(ids) == 0 {
			ids = []string{fmt.Sprintf("%02d-%s", number, bs.Slug)}
		}
		for _, id := range ids {
			done[id] = true
		}
	}

	var cards []planparser.Card
	for _, c := range plan.Cards {
		if done[cardID(c)] {
			cards = append(cards, c)
		}
	}
	return cards
}

// EditedDoneCards returns, sorted, the NN-slug ids of the cards of every batch st records terminal with status done whose card file under planDir no longer hashes to the CardHashes entry the batch recorded at begin.
// It hashes the card files as batchCardHashes does and reads the record and those bytes alone, never parsing the plan.
// A nil state, a record without CardHashes, a batch not terminal done and a recorded id plan lacks each contribute nothing;
// an unreadable card file is returned as an error.
//
// It is what the loom plan gate reads to refuse an edit to a card whose work has landed.
func EditedDoneCards(plan *planparser.Plan, st *State, planDir string) ([]string, error) {
	if st == nil {
		return nil, nil
	}

	cardsByID := make(map[string]planparser.Card, len(plan.Cards))
	for _, c := range plan.Cards {
		cardsByID[cardID(c)] = c
	}

	var edited []string
	for _, bs := range st.Batches {
		if bs == nil || !bs.Terminal || bs.Status != DigestStatusDone {
			continue
		}
		var recorded []planparser.Card
		for id := range bs.CardHashes {
			if c, ok := cardsByID[id]; ok {
				recorded = append(recorded, c)
			}
		}
		now, err := batchCardHashes(batcher.Batch{Cards: recorded}, planDir)
		if err != nil {
			return nil, err
		}
		for id, hash := range now {
			if hash != bs.CardHashes[id] {
				edited = append(edited, id)
			}
		}
	}
	sort.Strings(edited)
	return edited, nil
}

// findBatch returns the batcher.Batch in batches whose identity matches number.
func findBatch(batches []batcher.Batch, number int) (batcher.Batch, error) {
	for _, b := range batches {
		if n, _ := batchIdentity(b); n == number {
			return b, nil
		}
	}
	return batcher.Batch{}, fmt.Errorf("webster: batch %d not found in the plan's execution batches; way forward: `lyx webster status` lists the run's batches, name one of those", number)
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

// predecessorDigestLine renders the digest of whichever batch actually ran immediately before
// batchNumber in execution order.
// batches is required to already be in execution order, the batchifier's order that ExecutionBatches returns at every call site;
// batchNumber-1 arithmetic would be correct only while batch number and execution position coincide.
// It locates batchNumber's position in batches by batchIdentity, exactly as findBatch does, and
// returns "" when the batch is absent from batches or sits at index 0 (nothing executed before
// it), or when the predecessor's state entry or its digest is absent.
// It tolerates a nil st and a nil st.Batches by returning "".
func predecessorDigestLine(batches []batcher.Batch, st *State, batchNumber int) string {
	idx := -1
	for i, b := range batches {
		if n, _ := batchIdentity(b); n == batchNumber {
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

	prevNumber, _ := batchIdentity(batches[idx-1])
	prev, ok := st.Batches[prevNumber]
	if !ok || prev == nil {
		return ""
	}
	return digestSummaryLine(prev.Digest)
}

// existingReportRemedy names the one step the recorded state of batch number calls for when begin-batch finds the batch's report already on disk.
func existingReportRemedy(number int, recorded *BatchState) string {
	if !recorded.Terminal {
		if recorded.Kind == "recovery" {
			return fmt.Sprintf("`lyx webster recover-batch %d`", number)
		}
		return fmt.Sprintf("`lyx webster record-batch %d`, after fixing whatever its last refusal named", number)
	}
	switch recorded.Status {
	case DigestStatusDone:
		return "the batch is finished, so begin the next batch"
	case DigestStatusDead:
		return fmt.Sprintf("the recovery of batch %d is exhausted, so end the run stuck naming the batch", number)
	default:
		return fmt.Sprintf("`lyx webster recover-batch %d`", number)
	}
}

// BeginBatch drives one begin-batch call to completion, immediately before Master forks batchNumber's implementer: the pause gate, the fingerprint gate, start-SHA capture, the previous batch's persisted digest rendered into the fork prompt, and the prompt file write itself.
// The caller holds the state-mutation lease across this whole call and is responsible for
// persisting deps.State via SaveState once BeginBatch returns successfully — BeginBatch itself
// never calls SaveState and never touches fabric.
func BeginBatch(deps BeginDeps, batchNumber int) (*BeginResult, error) {
	// The plan is a hard precondition, refused loudly rather than dereferenced below, for the same
	// reason RecordBatch refuses one: a nil here is a wiring mistake in a caller, and a nil-pointer
	// panic names neither the missing field nor the verb that failed to supply it.
	if deps.Plan == nil {
		return nil, fmt.Errorf("webster: begin-batch requires a parsed plan; BeginDeps.Plan is nil")
	}
	// State is the same kind of hard precondition, and was the half-applied one: it is dereferenced a
	// few lines below for PlanFingerprint, so the discipline the Plan check states was only actually
	// enforced for one of the two fields (crucible round opus-medium-r6, R6-21).
	if deps.State == nil {
		return nil, fmt.Errorf("webster: begin-batch requires loaded run state; BeginDeps.State is nil")
	}

	if PauseRequested(deps.Geom.ScratchDir) {
		return nil, ErrPaused
	}

	if err := PlanEditError(deps.State, deps.Plan.Dir, deps.Geom.WebsterDir); err != nil {
		return nil, err
	}

	// Re-resolve the plan against the current tree before a pack is built, never from a cache.
	// deps.Geom.WorktreeRoot is the same root BeginBatch already reads for its head-SHA capture
	// below, so this adds no path derivation and no internal/lyxcwd import. An infrastructure error
	// (errors.Is(err, planindex.ErrQuarryUnavailable)) blocks exactly like a blocking finding does:
	// dispatching a pack built on a re-resolve that failed is strictly worse than not dispatching.
	// Scoped to the cards still to be built: a card already built contradicts the tree by design, and
	// re-resolving it reports the plan working correctly as a blocking defect.
	// DispatchScope, not completedCards: the batch record is written further down,
	// so on a first begin this batch is still validated, while a re-begin of a batch whose earlier fork already landed its work is not refused for it.
	// The forthcoming half keeps the Create targets of begun, unrecorded batches out of the status check,
	// so a re-begun batch whose fork landed nothing does not refuse the later cards that Use them (#329).
	// Once any batch is begun, a pending card's Delete target that is already gone arrives in the advisories below as delete-target-gone, not as a blocking finding.
	begun, forthcoming := DispatchScope(deps.Batches, deps.State)
	index, err := deps.Geom.index()
	if err != nil {
		return nil, err
	}
	resolveFindings, resolveErr := index.ValidateDispatch(deps.Plan, deps.Geom.WorktreeRoot, begun, forthcoming)
	// ValidateDispatch's resolve pass canonicalizes handles, which rewrites the plan on disk, and it then keeps going: the status and Create-inversion passes both run after the rewrite, so "rewrote the plan" and "reported a blocking finding" co-occur routinely, and the rewrite also survives the pass's own hard-error paths.
	// The staleness re-baseline therefore runs HERE, ahead of every refusal below, rather than once past them — otherwise state.json keeps the pre-rewrite fingerprint while the plan on disk carries this run's own sanctioned edit, and every later begin-batch refuses it as a foreign one.
	// See this package's doc.go.
	// A restamp failure never masks resolveErr: the caller is already returning for that reason.
	if err := restampFingerprint(deps.State, deps.Plan.Dir, deps.Geom.WebsterDir); err != nil && resolveErr == nil {
		return nil, err
	}
	if resolveErr != nil {
		return nil, resolveErr
	}
	var blocking []string
	var advisories []string
	for _, f := range resolveFindings {
		if f.Severity == planindex.SeverityBlocking {
			blocking = append(blocking, f.Error())
		} else {
			advisories = append(advisories, f.Error())
		}
	}
	if len(blocking) > 0 {
		return nil, fmt.Errorf("%w: %s; way forward: edit the plan so the named cards match the tree, run \"lyx webster rebaseline --card NN\" naming each card you edited, then begin-batch %02d again", ErrPlanDrifted, strings.Join(blocking, "; "), batchNumber)
	}

	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := batchIdentity(batch)

	cardHashes, err := batchCardHashes(batch, deps.Plan.Dir)
	if err != nil {
		return nil, fmt.Errorf("%w; way forward: transient, re-run `lyx webster begin-batch %d`", err, batchNumber)
	}

	// The fork writes its report here with whatever tool it likes — a plain
	// shell redirect included, which unlike an agent Write tool never creates
	// missing parents. Only the --fresh archive path recreated this dir
	// before; the ordinary first run left it absent (found live in crucible
	// round fable-r1).
	if err := os.MkdirAll(deps.Geom.ReportsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create reports dir %s: %w", deps.Geom.ReportsDir, err)
	}

	// webster's own pre-existing-report guard, applied to the fork path: a batch whose report already landed is finished work — silently overwriting its BatchState (and letting a fresh fork overwrite the report) must never happen by accident.
	// A no_report re-fork never calls begin-batch again (the bracket is still open), with ONE exception:
	// a run resumed after a crash that landed between the fork's report and record-batch re-drives a batch whose report IS on disk — that report is consumed by record-batch (the audit keys on the bracket-opening session, see RecordBatch),
	// so the refusal message names the one remedy the recorded state calls for.
	// Bound: only a batch with no record in state.json has its report archived and the begin proceeds, since such a report cannot be attributed to any begun batch and record-batch would only archive it and send the caller back here;
	// a recorded batch's report is never archived by begin-batch.
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
			return nil, fmt.Errorf("webster: batch %02d-%s already has a report at %s and state.json records the batch as %s — begin-batch never overwrites finished work; way forward: %s", number, slug, existingReport, seen, existingReportRemedy(number, recorded))
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

	// WorktreeRoot, not AnchorRoot, is correct in both modes here: hub
	// mode's WorktreeRoot is the anchor path, the exact value this call
	// rendered before this Geometry split.
	notePath := friction.NotePath(deps.FrictionDir, batchName)
	cardGates := renderCardGates(deps.Plan, batch.Cards, masterPlanDirDisplay(deps.Geom.WorktreeRoot, deps.Geom.PlanDir), deps.Geom.WorktreeRoot)
	batchGate := renderBatchGate(deps.Plan, batch.Cards, deps.Geom.WorktreeRoot)
	prompt, err := RenderForkPrompt(batch, cardGates, batchGate, prevDigest, reportPath, deps.Geom.PlanDir, deps.Geom.WorktreeRoot, deps.Geom.StencilsDir, deps.Geom.SpecsDir, OutcomePath(deps.Geom.WebsterDir), summaryparser.Path(deps.Geom.WebsterDir), deps.Config.SelfFixCap, notePath)
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
	// first.
	// A plain fork batch's record has an empty StrandGUID, which the reclaim skips.
	if prior, ok := deps.State.Batches[number]; ok && prior != nil && prior.StrandGUID != "" {
		if err := deps.Stopper.StopStrand(prior.StrandGUID); err != nil {
			return nil, fmt.Errorf("websterengine: stop prior recovery strand %s before respawn: %w", prior.StrandGUID, err)
		}
	}

	// A re-begin keeps the StartSHA the batch was first recorded with: the captured head may already sit past commits an earlier fork landed,
	// and the recorded start must name the base of the whole bracket (recover-batch applies the same inheritance to a recovery record).
	startSHA := head
	prior := deps.State.Batches[number]
	if prior != nil && prior.StartSHA != "" {
		startSHA = prior.StartSHA
	}
	// Recorded warnings carry over too: their identities stay dispositioned, so no later call would record them again.
	// So do the fork transcripts already attributed to the batch, so the run-exit audit still knows which report each of those forks owns.
	var priorWarnings []AuditWarning
	var priorTranscripts []string
	if prior != nil {
		priorWarnings = prior.AuditWarnings
		priorTranscripts = prior.ForkTranscripts
	}

	deps.State.Batches[number] = &BatchState{
		Slug:            slug,
		Cards:           batchCardIDs(batch),
		CardHashes:      cardHashes,
		StartSHA:        startSHA,
		Kind:            "fork",
		AuditWarnings:   priorWarnings,
		ForkTranscripts: priorTranscripts,
		SpawnedAt:       time.Now().UTC().Format(time.RFC3339),
		// Stamp the opening Master session so the run-exit audit cross-check
		// can scope its begun-batch count to the session whose forks the
		// whole-session audit actually covers.
		SessionID: deps.State.MasterSessionID,
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
