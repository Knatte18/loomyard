// cardpack.go renders one card's kick-start pack against the tree as it stands now, the logic behind the read-only `lyx loom card-pack` verb.

package websterengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// CardPackWayForward is the way forward every card-pack refusal ends with: the fallback a per-card fork takes when its pack call fails.
const CardPackWayForward = `way forward: locate the card's glyphs with "lyx quarry resolve" or a grep`

// CardPackMarkdown renders card number's subsection of the kick-start pack, resolved by index against the tree at worktreeRoot as it stands now, committed or not.
// The plan is parsed afresh from planDir, and the subsection is byte-identical to the one the batch pack holds for that card, read from the fragments in stencilsDir.
// It refuses a planDir that does not exist, a plan that does not parse and a number no card carries, each ending in CardPackWayForward.
// An index error is a Warn and the degraded pack the index returned is rendered, so it never refuses.
// A rendered pack logs one Info line with the card number and its distinct resolved and unresolved ref counts.
// The result ends with one newline.
func CardPackMarkdown(index planindex.Index, planDir, worktreeRoot, stencilsDir string, number int) ([]byte, error) {
	if _, err := os.Stat(planDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("webster: card pack: plan directory %s does not exist; %s", planDir, CardPackWayForward)
		}
		return nil, fmt.Errorf("webster: card pack: stat plan directory %s: %w", planDir, err)
	}
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		return nil, fmt.Errorf("webster: card pack: parse the plan in %s: %v; %s", planDir, err, CardPackWayForward)
	}
	cardIndex := slices.IndexFunc(plan.Cards, func(c planparser.Card) bool { return c.Number == number })
	if cardIndex < 0 {
		return nil, fmt.Errorf("webster: card pack: the plan in %s has no card %d; %s", planDir, number, CardPackWayForward)
	}
	card := plan.Cards[cardIndex]

	packs, err := index.KickstartPack(plan, []planparser.Card{card}, worktreeRoot)
	if err != nil {
		logger.Warn("webster: card pack index failed, keeping the degraded pack", "card", number, "error", err)
	}
	var pack planindex.Pack
	if len(packs) > 0 {
		pack = packs[0].Pack
	}

	section, err := renderCardPack(stencilsDir, cardPointerPath(card, masterPlanDirDisplay(worktreeRoot, planDir)), pack)
	if err != nil {
		return nil, fmt.Errorf("webster: card pack: render card %d: %w", number, err)
	}

	resolved, unresolved := make(map[string]bool), make(map[string]bool)
	for _, e := range pack.Resolved {
		resolved[e.Glyph] = true
	}
	for _, u := range pack.Unresolved {
		unresolved[u.Ref] = true
	}
	logger.Info("webster: card pack", "card", number, "resolved", len(resolved), "unresolved", len(unresolved))
	return []byte(section + "\n"), nil
}
