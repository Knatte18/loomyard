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
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// ErrFingerprintMismatch is the sentinel BeginBatch returns when the on-disk plan's recomputed
// fingerprint disagrees with State.PlanFingerprint.
// It guards a crashed or resumed run against a plan edited since the run recorded its fingerprint.
var ErrFingerprintMismatch = errors.New("webster: on-disk plan fingerprint does not match this run's recorded state")

// planOverviewFile is the plan's overview file, which carries the integration verify.
// rebaseline accepts a change to its Card Index only.
const planOverviewFile = "00-overview.md"

// fingerprintMismatchWayForward is the trailing clause BeginBatch and Run put on an ErrFingerprintMismatch wrap.
// It reads the changed plan files, so the clause names the cards to pass to rebaseline.
// A state without PlanFileHashes names rebaseline without card numbers.
// A changedPlanFiles error falls back to the generic text.
// An overview change confined to the Card Index names rebaseline with the changed and added cards,
// and any other overview change names the follow-up card landing.
// The reset route it names ends in reentry.
func fingerprintMismatchWayForward(st *State, planDir, websterDir, reentry string) string {
	fresh := freshRestartSteps(reentry)
	const restore = `or restore the plan the run recorded with "lym webster restore-plan", `
	if len(st.PlanFileHashes) == 0 {
		return "way forward: if the edit keeps every begun batch's cards, run `lym webster rebaseline` to accept it, " + restore + "otherwise " + fresh
	}
	changed, err := changedPlanFiles(st, planDir)
	if err != nil {
		return "way forward: if the edit keeps every begun batch's cards, run `lym webster rebaseline --card NN` naming each card you edited, " + restore + "otherwise " + fresh
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
				return "way forward: if the edit keeps every begun batch's cards, run `lym webster rebaseline --card NN` naming each card you edited, " + restore + "otherwise " + fresh
			}
			if !indexOnly {
				if recorded == "" {
					return "way forward: " + planOverviewFile + " changed and is never rebaselined; restore it with \"lym webster restore-plan\", or " + fresh
				}
				return "way forward: " + planOverviewFile + " changed outside its Card Index; restore it with \"lym webster restore-plan\", or " + followUpCardLanding + ", or " + fresh
			}
			indexChanged = true
			continue
		}
		flags = append(flags, "--card "+cardNumberOf(name))
	}
	if len(flags) == 0 {
		if indexChanged {
			return "way forward: only the Card Index of " + planOverviewFile + " changed; run `lym webster rebaseline` to accept it, " + restore + "otherwise " + fresh
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

// ErrPlanConflict is the sentinel a *PlanConflictError unwraps to.
// It marks a begin refused because the plan re-resolution at dispatch reported a blocking finding.
// Callers match it with errors.Is.
// Dispatching a fork on a re-resolve that failed is worse than not dispatching.
var ErrPlanConflict = errors.New("webster: plan re-resolution at begin-batch reported a blocking finding")

// The three causes a PlanConflictError carries, and the only values its Cause takes.
const (
	// ConflictCausePlanDefect is a conflict found before any batch was begun: the approved plan contradicts the tree.
	ConflictCausePlanDefect = "plan_defect"
	// ConflictCauseTreeMoved is a conflict found after a batch was begun: the tree moved under the plan.
	ConflictCauseTreeMoved = "tree_moved"
	// ConflictCausePlanOrder marks a conflict where every blocking finding is a delete a later card still references.
	ConflictCausePlanOrder = "plan_order"
)

// PlanConflictError is the refusal BeginBatch returns when the re-resolution at dispatch reports blocking findings.
// Batch is the batch whose begin was refused.
// Cause is ConflictCausePlanDefect, ConflictCauseTreeMoved or ConflictCausePlanOrder.
// Findings is every blocking finding as an error string, in the order the re-resolution reported them.
type PlanConflictError struct {
	Batch    int
	Cause    string
	Findings []string
}

// Error renders the refusal with its class and cause, the findings and the way forward.
func (e *PlanConflictError) Error() string {
	planEdit := "edit the plan so the named cards match the tree"
	if e.Cause == ConflictCausePlanOrder {
		planEdit = "move each named delete to a card after the card that still references it"
	}
	return fmt.Sprintf("%s (plan_conflict: %s): %s; way forward: %s, run \"lym webster rebaseline --card NN\" naming each card you edited, then begin-batch %02d again", ErrPlanConflict.Error(), e.Cause, strings.Join(e.Findings, "; "), planEdit, e.Batch)
}

// Unwrap returns ErrPlanConflict, so errors.Is identifies the refusal.
func (e *PlanConflictError) Unwrap() error { return ErrPlanConflict }

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

// completedCards returns the cards of every batch that has reached a terminal classification, in batches' own order.
//
// It scopes the dispatch-boundary re-resolution and record-batch's drift detection to work that has not landed yet.
// A plan describes intended change, so a card already built contradicts the tree it is re-resolved against:
// its Create target exists, its Delete target is gone, and its Rename's old side no longer resolves.
// A non-zero exclude also counts that batch as completed:
// record-batch calls this while the batch it records is still non-terminal, and that batch's work has just landed.
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

// DispatchScope computes the one dispatch scoping shared across webster's begin, run and validate paths.
// begun is every card of a batch that begin-batch recorded, terminal or not, in batches' own order.
// forthcoming is the cards of every begun batch whose record is not terminal.
// A terminal batch's cards are never forthcoming: its done-checks proved its targets present.
// A nil state returns two nil slices.
//
// A batch begun but never recorded may already have landed its work, so its Create targets can exist.
// Validating it as unstarted would refuse the very resume that records it.
// Its targets may equally not have landed yet, which is why forthcoming exists:
// the dispatch validation excludes a forthcoming card's Create and Rename New targets from its status check,
// so a later card that Uses them is not refused.
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
// It reads the batch records alone: a record's own card ids name its cards,
// and a record written before those ids existed names the single card NN-<Slug>.
// A recorded id the plan lacks is skipped, so a card renamed since its batch ran counts as not done.
// A nil state returns nil.
//
// The loom plan gate uses it to scope its tree-dependent checks:
// a done card's work is already in the tree, so it is history, not a target.
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

// LastBegunCard returns the highest card number among the cards of every batch record st holds, terminal or not.
// It returns 0 when there is none, including for a nil state.
// A record's cards are its own card ids.
// A record written before those ids existed names the single card numbered by its key.
// It bounds the begun set, whose cards are frozen, so it reads the records alone.
func LastBegunCard(st *State) int {
	if st == nil {
		return 0
	}

	last := 0
	for number, bs := range st.Batches {
		if bs == nil {
			continue
		}
		if len(bs.Cards) == 0 {
			last = max(last, number)
			continue
		}
		for _, id := range bs.Cards {
			last = max(last, cardNumberInt(id))
		}
	}
	return last
}

// EditedDoneCards returns, sorted, the NN-slug ids of edited cards whose batch is terminal and done.
// A card is edited when its file no longer hashes to the CardHashes entry recorded at begin.
// The plan's own source reads each card file, so an overlaid plan is judged by its proposed bytes.
// It hashes the files the way the recorded CardHashes were computed,
// and it reads only the record and those bytes, never parsing the plan.
// A nil state contributes nothing.
// So does a record without CardHashes, a batch not terminal done, or a recorded id the plan lacks.
// An unreadable card file is returned as an error.
//
// The plan gate uses it to refuse an edit to a card whose work has landed.
func EditedDoneCards(plan *planparser.Plan, st *State) ([]string, error) {
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
		now, err := cardHashesFrom(batcher.Batch{Cards: recorded}, plan.ReadFile)
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

// findBatch returns the batch in batches whose number matches number.
// When none matches, it returns an error that names the way forward.
func findBatch(batches []batcher.Batch, number int) (batcher.Batch, error) {
	for _, b := range batches {
		if n, _ := batchIdentity(b); n == number {
			return b, nil
		}
	}
	return batcher.Batch{}, fmt.Errorf("webster: batch %d not found in the plan's execution batches; way forward: `lym webster status` lists the run's batches, name one of those", number)
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
	number, slug := batchIdentity(batch)
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
// It locates batchNumber's position in batches by batchIdentity.
// It returns "" when the batch is absent from batches or sits at index 0, which means nothing ran before it.
// It also returns "" when the predecessor's state entry or its digest is absent.
// A nil st or nil st.Batches also yields "".
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

	// Re-resolve the plan against the current tree before the fork prompt is written, never from a cache.
	// An infrastructure error (errors.Is(err, planindex.ErrQuarryUnavailable)) blocks exactly like a blocking finding:
	// dispatching a fork on a re-resolve that failed is worse than not dispatching.
	// The scope is the cards still to be built:
	// a card already built contradicts the tree by design;
	// re-resolving it would report a working plan as a blocking defect.
	// The batch record is written further down, so on a first begin this batch is still validated;
	// a re-begin of a batch whose earlier fork already landed its work is not refused for it.
	// The forthcoming half keeps the Create targets of begun, unrecorded batches out of the status check,
	// so a re-begun batch whose fork landed nothing does not refuse the later cards that Use them.
	// Once any batch is begun, a pending card's Delete target that is already gone,
	// and it arrives in the advisories below as delete-target-gone, not as a blocking finding.
	begun, forthcoming := DispatchScope(deps.Batches, deps.State)
	index, err := deps.Geom.index()
	if err != nil {
		return nil, err
	}
	resolveFindings, resolveErr := index.ValidateDispatch(deps.Plan, deps.Geom.WorktreeRoot, begun, forthcoming)
	// ValidateDispatch's resolve pass canonicalizes handles, which rewrites the plan on disk.
	// The status and Create-inversion passes run after that rewrite,
	// so a rewritten plan routinely also reports a blocking finding,
	// and the rewrite also survives the pass's own hard-error paths.
	// The staleness re-baseline therefore runs before every refusal below.
	// Otherwise state.json keeps the pre-rewrite fingerprint while the plan on disk carries this run's own sanctioned edit,
	// and every later begin-batch refuses it as a foreign one.
	// A restamp failure never masks resolveErr: the caller is already returning for that reason.
	if err := restampFingerprint(deps.State, deps.Plan.Dir, deps.Geom.WebsterDir); err != nil && resolveErr == nil {
		return nil, err
	}
	if resolveErr != nil {
		return nil, resolveErr
	}
	var blocking []string
	var advisories []string
	orderOnly := true
	for _, f := range resolveFindings {
		if f.Severity == planindex.SeverityBlocking {
			blocking = append(blocking, f.Error())
			orderOnly = orderOnly && f.Check == planindex.CheckDeleteBeforeReference
		} else {
			advisories = append(advisories, f.Error())
		}
	}
	if len(blocking) > 0 {
		// begun is computed before this call records its batch:
		// a first begin of batch 1 has none begun (plan_defect), and a re-begin of a recorded batch has it begun (tree_moved).
		// A set holding any other finding keeps those two causes, since moving a delete would not settle it.
		cause := ConflictCauseTreeMoved
		switch {
		case orderOnly:
			cause = ConflictCausePlanOrder
		case len(begun) == 0:
			cause = ConflictCausePlanDefect
		}
		return nil, &PlanConflictError{Batch: batchNumber, Cause: cause, Findings: blocking}
	}

	batch, err := findBatch(deps.Batches, batchNumber)
	if err != nil {
		return nil, err
	}
	number, slug := batchIdentity(batch)

	cardHashes, err := batchCardHashes(batch, deps.Plan.Dir)
	if err != nil {
		return nil, fmt.Errorf("%w; way forward: transient, re-run `lym webster begin-batch %d`", err, batchNumber)
	}

	// The gate calls render before the first mutation,
	// so a gate list the config lacks refuses with no prompt written and no state recorded.
	cardGates, err := renderCardGates(batch.Cards, masterPlanDirDisplay(deps.Geom.PaneCwd, deps.Geom.PlanDir), deps.Gate)
	if err != nil {
		return nil, err
	}
	batchGate, err := renderBatchGate(batch.Cards, deps.Gate, isLastBatch(deps.Batches, batch))
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
		Cards:            batchCardIDs(batch),
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
