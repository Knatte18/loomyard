// autorebaseline.go implements the Webster row's rebaseline on run entry.
// Behind RunOptions.AutoRebaseline, Run rebaselines a changed plan itself:
// it calls Rebaseline with every changed card named, as the operator's verb does, and reports it as a warning.

package websterengine

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrAutoRebaseline is the sentinel every error of an auto-rebaseline wraps.
// It also marks each plan refusal Run raises after an accepted one:
// the plan the row was stepped over cannot continue until it is fixed and the row is stepped again.
var ErrAutoRebaseline = errors.New("webster: the auto-rebaseline of the changed plan blocks this run")

// blockedByAutoRebaseline wraps err in ErrAutoRebaseline when step is set, and returns it unchanged otherwise.
func blockedByAutoRebaseline(err error, step string) error {
	if step == "" {
		return err
	}
	return fmt.Errorf("%w: %w", ErrAutoRebaseline, err)
}

// autoRebaseline rebaselines the plan on disk as the run's plan.
// It names every changed card by number and saves the restamped state.
// It returns the warning that names the rebaseline and its accepted cards.
// Its refusals keep Rebaseline's own step-keyed text.
// A failed read, base or save is marked transient, with the run's re-entry step as the way forward.
// Every error wraps ErrAutoRebaseline.
// A failed save returns before the caller starts anything on the unsaved restamp.
func autoRebaseline(deps RunDeps, plan *planparser.Plan, st *State, sizes batcher.SizeSource) (string, error) {
	step := deps.reentryStep()
	transient := func(err error) error {
		return fmt.Errorf("%w: %w; way forward: transient, %s", ErrAutoRebaseline, err, step)
	}

	changed, err := changedPlanFiles(st, deps.Geom.PlanDir)
	if err != nil {
		return "", transient(err)
	}
	var cards []int
	for _, name := range changed {
		if number, err := strconv.Atoi(cardNumberOf(name)); err == nil && name != planOverviewFile {
			cards = append(cards, number)
		}
	}
	instructionFiles, err := deps.instructionFiles()
	if err != nil {
		return "", err
	}
	base, err := MerriamBase(deps.Geom, instructionFiles)
	if err != nil {
		return "", transient(err)
	}

	res, err := Rebaseline(RebaselineDeps{Plan: plan, Active: deps.Batcher, Sizes: sizes, Base: base, State: st, Cards: cards, Geom: deps.Geom, Step: step})
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrAutoRebaseline, err)
	}
	if err := SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st); err != nil {
		return "", transient(err)
	}

	warning := autoRebaselineWarning(res)
	logger.Warn("websterengine: plan rebaselined on entry", "warning", warning)
	return warning, nil
}

// autoRebaselineWarning words the warning for an accepted auto-rebaseline:
// the begun batches kept, the card files accepted, and the in-flight cards amended.
func autoRebaselineWarning(res *RebaselineResult) string {
	accepted := "no card file"
	if len(res.CardsAccepted) > 0 {
		accepted = strings.Join(res.CardsAccepted, ", ")
	}
	warning := fmt.Sprintf("webster: the plan changed since the run recorded it and was rebaselined on entry, keeping %d begun batch(es) and accepting %s", res.BatchesKept, accepted)
	if len(res.CardsAmended) > 0 {
		warning += "; amended in-flight cards: " + strings.Join(res.CardsAmended, ", ")
	}
	return warning
}
