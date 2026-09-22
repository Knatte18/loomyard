// seamchild.go implements NewSeedChild, the producer that seeds the task worktree's own inner
// shed run with a recipe derived from the Board task's own type and a driver inherited from
// prime's own seed.

package battenshed

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// defaultChildRecipe is the recipe a child worktree seeds when the Board task's own "type" field
// is empty.
const defaultChildRecipe = "loom"

// seedChildProducer writes, commits and pushes the child worktree's own seed file, deriving its
// recipe from the Board task's own type and its driver from prime's own seed.
type seedChildProducer struct {
	name       string
	slug       string
	deps       SeedChildDeps
	scratchDir string
}

var _ shedengine.ShedProducer = (*seedChildProducer)(nil)

// NewSeedChild returns a shedengine.ShedProducer that reads the Board task's own type and prime's
// own seed driver, then writes, commits and pushes the child worktree's own seed.
//
// name is told rather than hardcoded because a second producer list may name this row
// independently. slug is used only as a log field and in stuck-reason text -- never compared,
// parsed, or used for control flow.
func NewSeedChild(name, slug string, deps SeedChildDeps, scratchDir string) shedengine.ShedProducer {
	return &seedChildProducer{
		name:       name,
		slug:       slug,
		deps:       deps,
		scratchDir: scratchDir,
	}
}

// Call implements shedengine.ShedProducer.
//
// It reads the Board task's own type fresh at Call time -- never a value captured at wiring time,
// so a type corrected after prime was seeded is still honoured -- defaulting an empty value to
// defaultChildRecipe. It then reads the driver, writes, commits and pushes, in that order.
//
// Verdicts: Done on a successful write-and-commit; Stuck on an unreadable Board, a refused recipe
// (a deps.WriteSeed error wrapping ErrUnknownRecipe, ErrUnsupportedChildRecipe, or
// ErrDisagreeingChildSeed), or a failed commit, each naming which of the four failed in its stuck
// reason; a hard error on a deps.ChildDriver failure or a deps.WriteSeed failure that wraps none of
// the three sentinels (a path-resolution or write failure), matching innerRunProducer's
// error-vs-verdict split; and a failed push warns via internal/logger and still returns Done,
// because an offline machine must not halt a run and the next push on that pair catches the branch
// up.
func (p *seedChildProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	boardType, err := p.deps.ReadBoardType(ctx)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("read Board task type failed: %s", err.Error())
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}
	recipe := boardType
	if recipe == "" {
		recipe = defaultChildRecipe
	}

	driver, err := p.deps.ChildDriver()
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read child driver: %w", p.name, err)
	}

	if err := p.deps.WriteSeed(ctx, recipe, driver); err != nil {
		if reason, refused := childRecipeRefusal(recipe, err); refused {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
			return shedengine.Stuck, shedengine.OutputPointer{}, nil
		}
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: write seed: %w", p.name, err)
	}

	if err := p.deps.CommitSeed(ctx); err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("commit seed failed: %s", err.Error())
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}

	if err := p.deps.PushSeed(ctx); err != nil {
		logger.Warn("battenshed: push seed failed; the next push on this pair will catch the branch up", "producer", p.name, "slug", p.slug, "error", err)
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// childRecipeRefusal reports whether err is one of the three refusals a WriteSeed seam may raise,
// returning the stuck reason to record for it: an unknown recipe name, a known recipe the task
// worktree cannot run as its own, or a pre-existing child seed that disagrees with the one being
// written. Any other error is mechanism failure and is not a refusal.
func childRecipeRefusal(recipe string, err error) (reason string, refused bool) {
	switch {
	case errors.Is(err, ErrUnknownRecipe):
		return fmt.Sprintf("unknown recipe name %q: %s", recipe, err.Error()), true
	case errors.Is(err, ErrUnsupportedChildRecipe):
		return fmt.Sprintf("Board task type %q cannot be the task worktree's own run: %s", recipe, err.Error()), true
	case errors.Is(err, ErrDisagreeingChildSeed):
		return fmt.Sprintf("the task worktree's own seed already disagrees with recipe %q: %s", recipe, err.Error()), true
	default:
		return "", false
	}
}
