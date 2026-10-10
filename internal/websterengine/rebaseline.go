// rebaseline.go implements Rebaseline, the engine half of `lyx webster rebaseline`:
// it accepts a mid-run plan edit as the run's plan while every begun batch keeps its recorded card set.

package websterengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrRebaselineCardSetChanged is the sentinel Rebaseline returns when the edited plan no longer holds a begun batch's recorded card set, which no restamp can make safe.
var ErrRebaselineCardSetChanged = errors.New("webster: the edited plan changes the cards of a batch this run already begun")

// followUpCardLanding is the landing for a decision that reaches a card whose batch is done, or an overview outside its Card Index: a follow-up card after the last begun batch.
// It carries no "way forward: " prefix.
const followUpCardLanding = "add a follow-up card after the last begun batch that carries the decision, with its Card Index line in " + planOverviewFile + ", then run `lyx webster rebaseline --card NN` naming it"

// The three causes of an empty recorded overview frame.
const (
	causeNoOverviewHash     = "no overview hash was recorded"
	causeBaselineCopyAbsent = "the baseline copy of the recorded overview is absent"
	causeNoCardIndex        = "the baseline copy of the recorded overview has no parseable Card Index"
)

// followUpLanding renders the landing for a decision that reaches a done batch's card or an overview outside its Card Index.
// An empty step is the manual landing, followUpCardLanding;
// a set step says to fix the plan, carry the decision in a follow-up card after the last begun batch with its Card Index line, and take step.
// It carries no "way forward: " prefix.
func followUpLanding(step string) string {
	if step == "" {
		return followUpCardLanding
	}
	return "fix the plan: move a done card's edit to a follow-up card after the last begun batch that carries the decision, with its Card Index line in " + planOverviewFile + ", then " + step
}

// transientWayForward is the clause of a transient rebaseline failure: manual, the text naming the verb to re-run, when step is empty, and otherwise the instruction to take step.
// It carries no "way forward: " prefix.
func transientWayForward(step, manual string) string {
	if step == "" {
		return manual
	}
	return "transient, " + step
}

// recordedOverviewFrame returns the overview frame hash the run recorded.
// A state without State.PlanOverviewFrameHash takes the frame of the stored baseline copy of its recorded 00-overview.md, under websterDir.
// It returns "" when the run has no recorded frame: no overview hash recorded, the copy absent, or a copy without a parseable Card Index.
// For an empty frame cause is one of the three fixed texts above and path is the baseline copy's path, empty when no hash was recorded; both are empty beside a frame.
// It logs nothing.
// A read failure other than an absent copy is returned.
func recordedOverviewFrame(st *State, websterDir string) (frame, cause, path string, err error) {
	if st.PlanOverviewFrameHash != "" {
		return st.PlanOverviewFrameHash, "", "", nil
	}
	hash := st.PlanFileHashes[planOverviewFile]
	if hash == "" {
		return "", causeNoOverviewHash, "", nil
	}
	path = planBaselinePath(websterDir, hash)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", causeBaselineCopyAbsent, path, nil
	}
	if err != nil {
		return "", "", "", fmt.Errorf("websterengine: recorded overview frame: read plan baseline copy %s: %w", hash, err)
	}
	frame, err = overviewFrameHashOf(data)
	if err != nil {
		return "", causeNoCardIndex, path, nil
	}
	return frame, "", "", nil
}

// logEmptyOverviewFrame logs at Info why the run has no recorded overview frame, naming the baseline copy's path where there is one.
func logEmptyOverviewFrame(cause, path string) {
	if path == "" {
		logger.Info("websterengine: rebaseline: no recorded overview frame", "cause", cause)
		return
	}
	logger.Info("websterengine: rebaseline: no recorded overview frame", "cause", cause, "path", path)
}

// overviewIndexOnly reports whether 00-overview.md differs from the run's record only inside its Card Index: recorded is the run's overview frame hash and the file's frame hash still equals it.
// An empty recorded frame reports false.
func overviewIndexOnly(recorded, planDir string) (bool, error) {
	if recorded == "" {
		return false, nil
	}
	now, err := overviewFrameHash(planDir)
	if err != nil {
		return false, err
	}
	return now == recorded, nil
}

// overviewRefusal is the refusal for a 00-overview.md change Rebaseline cannot accept: one outside the Card Index, or any change in a run whose recorded frame is empty.
// For an empty frame the refusal names cause and, where there is one, the baseline copy's path.
// Its way forward names step, empty meaning `lyx webster run`.
func overviewRefusal(recorded, cause, path, step string) error {
	if recorded == "" {
		where := cause
		if path != "" {
			where += " (" + path + ")"
		}
		return fmt.Errorf("%w: %s changed and this run recorded no overview frame: %s, so the change cannot be confined to its Card Index; it carries the plan's integration verify and is never rebaselined; way forward: restore %s, or %s", ErrRebaselineCardSetChanged, planOverviewFile, where, planOverviewFile, freshRestartSteps(stepOrRun(step)))
	}
	return fmt.Errorf("%w: %s changed outside its Card Index; it carries the plan's integration verify and only its Card Index is rebaselined; way forward: restore %s, or %s", ErrRebaselineCardSetChanged, planOverviewFile, planOverviewFile, followUpLanding(step))
}

// editableTerminalStatus reports whether a batch with this terminal status accepts a named card edit.
func editableTerminalStatus(status string) bool {
	return status == DigestStatusFailed || status == DigestStatusDead || status == DigestStatusStuck
}

// cardNumberInt returns the number of a NN-<slug> card id, or 0 when the id has none.
func cardNumberInt(id string) int {
	n, _ := strconv.Atoi(cardNumberOf(id))
	return n
}

// RebaselineDeps is what Rebaseline reads: the edited on-disk plan, the batchifier and size source that group its cards, and the run state whose fingerprint it restamps.
type RebaselineDeps struct {
	Plan   *planparser.Plan
	Active batcher.Batcher
	Sizes  batcher.SizeSource
	// Base is Merriam's start base the tail's batches are priced from.
	Base  batcher.StartBase
	State *State
	// Cards are the card numbers the operator names as edited; a changed card file whose number is absent is refused.
	Cards []int
	// Geom locates the worktree whose history names the run's start commit.
	Geom Geometry
	// Step is the step every refusal names as the way to continue; empty means `lyx webster run`, as RunDeps.ReentryStep does.
	// The loom's Webster row sets its re-step text, so no refusal on that path tells an agent to run a verb by hand.
	Step string
}

// RebaselineResult reports what one successful Rebaseline changed.
type RebaselineResult struct {
	// PreviousFingerprint is State.PlanFingerprint before the restamp.
	PreviousFingerprint string
	// Fingerprint is the on-disk plan's fingerprint now recorded in State.
	Fingerprint string
	// BatchesKept is the number of begun batch records the edited plan still holds unchanged.
	BatchesKept int
	// CardsAccepted are the changed card file names this call accepted.
	CardsAccepted []string
	// CardsAmended are the NN-<slug> ids of the accepted cards that belong to an in-flight batch, which a recovery re-runs on the edited text.
	CardsAmended []string
}

// rebaselineBatches returns the batches the edited plan runs as, and whether they replace the recorded partition.
// With a recorded partition, the batches up to and including the last begun one are kept as recorded, with their profile and estimate, and every plan card after them is batched by deps.Active;
// a plan whose first cards are not exactly the kept batches' recorded cards wraps ErrRebaselineCardSetChanged.
// A state without a partition is grouped by the identity batchifier and records none.
// Either way the result is asserted with CheckBatchOrder.
// The tail is batched told how many batches precede it, the number kept, so its first batch is priced at the position after them.
// That count is derived in Go from the recorded partition, never typed by an agent, and is told as if Merriam were never resumed, so after a resume the estimate is conservative;
// work Merriam does between batches that the model does not know makes it too low, and nothing re-checks it at run time.
func rebaselineBatches(deps RebaselineDeps) ([]batcher.Batch, bool, error) {
	st, plan := deps.State, deps.Plan
	if len(st.Partition) == 0 {
		batches, err := batchCards(plan, plan.Cards, batcher.Identity(), deps.Sizes, 0, deps.Base)
		if err != nil {
			return nil, false, err
		}
		return batches, false, CheckBatchOrder(batches, deps.Step)
	}

	keptCount := 0
	for i, pb := range st.Partition {
		if st.Batches[cardNumberInt(pb.Cards[0])] != nil {
			keptCount = i + 1
		}
	}

	var changed []string
	var batches []batcher.Batch
	offset := 0
	for _, pb := range st.Partition[:keptCount] {
		end := min(offset+len(pb.Cards), len(plan.Cards))
		batch := batcher.Batch{Cards: plan.Cards[offset:end], Profile: pb.Profile, Estimate: pb.Estimate, Breakdown: pb.Breakdown}
		if now := batchCardIDs(batch); !slices.Equal(now, pb.Cards) {
			nowText := "no longer in the plan"
			if len(now) > 0 {
				nowText = "[" + strings.Join(now, ", ") + "]"
			}
			changed = append(changed, fmt.Sprintf("batch %d recorded [%s], plan now %s", cardNumberInt(pb.Cards[0]), strings.Join(pb.Cards, ", "), nowText))
		}
		batches = append(batches, batch)
		offset = end
	}
	if len(changed) > 0 {
		return nil, false, fmt.Errorf("%w: %s; way forward: restore those cards in the plan, or %s", ErrRebaselineCardSetChanged, strings.Join(changed, "; "), freshRestartSteps(stepOrRun(deps.Step)))
	}

	if tail := plan.Cards[offset:]; len(tail) > 0 {
		tailBatches, err := batchCards(plan, tail, deps.Active, deps.Sizes, len(batches), deps.Base)
		if err != nil {
			return nil, false, err
		}
		batches = append(batches, tailBatches...)
	}
	if err := CheckBatchOrder(batches, deps.Step); err != nil {
		return nil, false, err
	}
	return batches, true, nil
}

// Rebaseline accepts the on-disk plan as the run's plan without discarding any batch record.
// With a recorded partition it keeps every batch up to the last begun one as recorded, re-batches the plan's cards after it with the active batchifier, and replaces State.Partition with the result once every check passes;
// cards added, removed or reordered after the last begun batch are accepted.
// It refuses, wrapping ErrRebaselineCardSetChanged, when a begun batch's card set differs from the card set the edited plan's batch of that number now holds, or the plan no longer has that number, or when a begun card's file content differs from the hash recorded at begin (a record without hashes compares ids only), except that a card named in deps.Cards is accepted when its batch is in flight or terminal failed, dead or stuck.
// It also refuses when a changed card file's number is not in deps.Cards, unless the state predates State.PlanFileHashes.
// A change to 00-overview.md is accepted only when it is confined to the Card Index: the run's recorded overview frame, State.PlanOverviewFrameHash or else the frame of the stored baseline copy, still equals the file's frame hash.
// The index change is then held to the same card-set rule, so only cards after the last begun batch can be added, removed or reordered.
// A change outside the Card Index, and any overview change in a run with no recorded frame, is refused.
// The start commit a refusal names is picked by git ancestry.
// The bound on the accepted card edit: only cards named with --card, only in batches in flight or terminal failed, dead or stuck, never a failed batch whose record lists Uncheckable entries, never a change to a batch's card-ID set, never 00-overview.md outside its Card Index.
// A begun card's reworded one-line intent in the Card Index passes, since the card file stays pinned by its recorded hash.
// A dead batch's strand, kept alive when classified dead, may still work on the old card, and so may the fork or recovery strand of an in-flight batch;
// the restamp stops no strand, and the next recover-batch stops it before it spawns and archives a late report from it.
// A card accepted in an in-flight batch is also recorded in the batch's AmendedCards with Rendered false, which resets an entry already there.
// Otherwise it restamps State.PlanFingerprint, State.PlanFileHashes and the CardHashes entry of each accepted card, and leaves every other field untouched.
// A refusal leaves the state unchanged.
// It never saves;
// the caller holds the state-mutation lease and saves, as for the bracket verbs.
func Rebaseline(deps RebaselineDeps) (*RebaselineResult, error) {
	if deps.Plan == nil {
		return nil, fmt.Errorf("webster: rebaseline requires a parsed plan; RebaselineDeps.Plan is nil")
	}
	if deps.State == nil {
		return nil, fmt.Errorf("webster: rebaseline requires loaded run state; RebaselineDeps.State is nil")
	}

	batches, partition, err := rebaselineBatches(deps)
	if err != nil {
		return nil, err
	}

	var cardsAccepted []string
	if len(deps.State.PlanFileHashes) > 0 {
		changedFiles, err := changedPlanFiles(deps.State, deps.Plan.Dir)
		if err != nil {
			return nil, fmt.Errorf("%w; way forward: %s", err, transientWayForward(deps.Step, "transient, re-run `lyx webster rebaseline`"))
		}
		var unnamed []string
		for _, name := range changedFiles {
			if name == planOverviewFile {
				recorded, cause, path, overviewErr := recordedOverviewFrame(deps.State, deps.Geom.WebsterDir)
				indexOnly := false
				if overviewErr == nil {
					if cause != "" {
						logEmptyOverviewFrame(cause, path)
					}
					indexOnly, overviewErr = overviewIndexOnly(recorded, deps.Plan.Dir)
				}
				if overviewErr != nil {
					return nil, fmt.Errorf("%w; way forward: %s", overviewErr, transientWayForward(deps.Step, "transient, re-run `lyx webster rebaseline` with the same `--card` flags"))
				}
				if !indexOnly {
					return nil, overviewRefusal(recorded, cause, path, deps.Step)
				}
				continue
			}
			n, convErr := strconv.Atoi(cardNumberOf(name))
			if convErr != nil || !slices.Contains(deps.Cards, n) {
				unnamed = append(unnamed, name)
				continue
			}
			cardsAccepted = append(cardsAccepted, name)
		}
		if len(unnamed) > 0 {
			wayForward := `re-run "lyx webster rebaseline" naming every changed card with --card NN, or restore the unnamed cards`
			if deps.Step != "" {
				wayForward = "fix the plan so every changed card is one the rebaseline can name, or restore the unnamed cards, then " + deps.Step
			}
			return nil, fmt.Errorf("%w: %s changed but not named; way forward: %s", ErrRebaselineCardSetChanged, strings.Join(unnamed, ", "), wayForward)
		}
	}

	current := make(map[int][]string, len(batches))
	currentCards := make(map[int][]planparser.Card, len(batches))
	for _, b := range batches {
		n, _ := batchIdentity(b)
		current[n] = batchCardIDs(b)
		currentCards[n] = b.Cards
	}

	numbers := make([]int, 0, len(deps.State.Batches))
	for n, bs := range deps.State.Batches {
		if bs == nil {
			continue
		}
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	var changed, doneChanged []string
	restamps := make(map[int]map[string]string)
	for _, n := range numbers {
		bs := deps.State.Batches[n]
		recorded := bs.Cards
		if len(recorded) == 0 {
			recorded = []string{fmt.Sprintf("%02d-%s", n, bs.Slug)}
		}
		now, ok := current[n]
		if ok && slices.Equal(recorded, now) {
			if len(bs.CardHashes) == 0 {
				continue
			}
			got, err := batchCardHashes(batcher.Batch{Cards: currentCards[n]}, deps.Plan.Dir)
			if err != nil {
				return nil, fmt.Errorf("%w; way forward: %s", err, transientWayForward(deps.Step, "transient, re-run `lyx webster rebaseline`"))
			}
			for _, id := range recorded {
				want, hashed := bs.CardHashes[id]
				if !hashed || got[id] == want {
					continue
				}
				switch {
				case bs.Terminal && !editableTerminalStatus(bs.Status):
					doneChanged = append(doneChanged, fmt.Sprintf("batch %d card %s changed since it was begun, and the batch is %s so its work has landed and --card cannot accept the edit", n, id, bs.Status))
				case bs.Terminal && len(bs.Uncheckable) > 0:
					changed = append(changed, fmt.Sprintf("batch %d card %s changed since it was begun, and the batch failed on findings recovery cannot check so --card cannot accept the edit", n, id))
				case !slices.Contains(deps.Cards, cardNumberInt(id)):
					changed = append(changed, fmt.Sprintf("batch %d card %s changed since it was begun and is not named with --card", n, id))
				default:
					if restamps[n] == nil {
						restamps[n] = make(map[string]string)
					}
					restamps[n][id] = got[id]
				}
			}
			continue
		}
		nowText := "no longer in the plan"
		if ok {
			nowText = "[" + strings.Join(now, ", ") + "]"
		}
		changed = append(changed, fmt.Sprintf("batch %d recorded [%s], plan now %s", n, strings.Join(recorded, ", "), nowText))
	}
	if len(changed)+len(doneChanged) > 0 {
		fresh := freshRestartSteps(stepOrRun(deps.Step))
		wayForward := "restore those cards in the plan, or " + fresh
		switch {
		case len(changed) == 0:
			wayForward = "restore those cards in the plan, or " + followUpLanding(deps.Step)
		case len(doneChanged) > 0:
			wayForward = "restore those cards in the plan, or, for a done batch's card, " + followUpLanding(deps.Step) + ", or " + fresh
		}
		return nil, fmt.Errorf("%w: %s; way forward: %s", ErrRebaselineCardSetChanged, strings.Join(append(changed, doneChanged...), "; "), wayForward)
	}
	var cardsAmended []string
	for _, n := range numbers {
		bs := deps.State.Batches[n]
		ids := make([]string, 0, len(restamps[n]))
		for id := range restamps[n] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			bs.CardHashes[id] = restamps[n][id]
			if bs.Terminal {
				continue
			}
			cardsAmended = append(cardsAmended, id)
			if i := slices.IndexFunc(bs.AmendedCards, func(a AmendedCard) bool { return a.Card == id }); i >= 0 {
				bs.AmendedCards[i].Rendered = false
			} else {
				bs.AmendedCards = append(bs.AmendedCards, AmendedCard{Card: id})
			}
		}
	}
	sort.Strings(cardsAmended)

	previous := deps.State.PlanFingerprint
	if err := restampBaseline(deps.State, deps.Plan.Dir, deps.Geom.WebsterDir); err != nil {
		if deps.Step == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w; way forward: %s", err, transientWayForward(deps.Step, ""))
	}
	if partition {
		RecordPartition(deps.State, batches)
	}
	return &RebaselineResult{
		PreviousFingerprint: previous,
		Fingerprint:         deps.State.PlanFingerprint,
		BatchesKept:         len(numbers),
		CardsAccepted:       cardsAccepted,
		CardsAmended:        cardsAmended,
	}, nil
}
