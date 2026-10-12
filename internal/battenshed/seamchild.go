// seamchild.go implements NewSeedChild, the producer that seeds the task worktree's own inner
// shed run with a recipe derived from the Board task's own type and a driver inherited from
// prime's own seed.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
	if deps.MarkWatched == nil {
		deps.MarkWatched = func(context.Context) (bool, error) { return false, nil }
	}
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
// defaultChildRecipe. It then reads the driver, writes the task worktree's local config overrides from the Board task's run settings, then writes, commits and pushes the seed, in that order.
//
// Verdicts: Done on a successful write-and-commit; Stuck on an unreadable Board, run settings that do not validate, a refused recipe
// (a deps.WriteSeed error wrapping ErrUnknownRecipe, ErrUnsupportedChildRecipe, or
// ErrDisagreeingChildSeed), or a failed commit, each naming which of the four failed in its stuck
// reason; a hard error on a deps.ChildDriver failure or a deps.WriteSeed failure that wraps none of
// the three sentinels (a path-resolution or write failure), matching innerRunProducer's
// error-vs-verdict split; and a failed push warns via internal/logger and still returns Done,
// because an offline machine must not halt a run and the next push on that pair catches the branch
// up.
// It then asks deps.MarkWatched to write or remove the batten-watched marker, so the child's first driver launch already renders the watched rule;
// a failure there only warns.
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
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
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

	if reason, err := p.writeRunSettings(ctx); err != nil || reason != "" {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}

	if err := p.deps.WriteSeed(ctx, recipe, driver); err != nil {
		if reason, refused := childRecipeRefusal(recipe, err); refused {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
			return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
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
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}

	if err := p.deps.PushSeed(ctx); err != nil {
		logger.Warn("battenshed: push seed failed; the next push on this pair will catch the branch up", "producer", p.name, "slug", p.slug, "error", err)
	}

	if _, err := p.deps.MarkWatched(ctx); err != nil {
		logger.Warn("battenshed: could not write or remove the batten-watched marker; the child's driver keeps its own parent notifications", "producer", p.name, "slug", p.slug, "error", err)
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// writeRunSettings makes the task worktree's local config layer equal the Board task's run settings:
// it writes the local file of each module the settings name, then removes every other module's, logging one Warn per removed file.
// A non-empty reason means the row stands Stuck: the board is unreadable, or a key no longer validates.
// Any other failure is returned as an error.
// The whole write is redone on every Call, so a Call interrupted between its writes and its removals leaves nothing the next one does not settle.
func (p *seedChildProducer) writeRunSettings(ctx context.Context) (reason string, err error) {
	settings, err := p.deps.ReadRunSettings(ctx)
	if err != nil {
		return fmt.Sprintf("read Board task run settings failed: %s", err.Error()), nil
	}
	documents, err := p.deps.PrepareRunSettings(settings)
	if err != nil {
		if errors.Is(err, ErrInvalidRunSettings) {
			return fmt.Sprintf("%s; way forward: correct the task's run_settings with `lyx board upsert`, then `lyx batten run %s`", err.Error(), p.slug), nil
		}
		return "", fmt.Errorf("battenshed: %s: prepare run settings: %w", p.name, err)
	}
	modules := make([]string, 0, len(documents))
	for module := range documents {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	for _, module := range modules {
		if err := p.deps.WriteLocalOverride(ctx, module, documents[module]); err != nil {
			return "", fmt.Errorf("battenshed: %s: write the %s local override: %w", p.name, module, err)
		}
	}
	removed, err := p.deps.RemoveLocalOverrides(ctx, modules)
	if err != nil {
		return "", fmt.Errorf("battenshed: %s: remove stale local overrides: %w", p.name, err)
	}
	for _, path := range removed {
		logger.Warn("battenshed: removed a local config override the task's run settings do not name", "producer", p.name, "slug", p.slug, "path", path)
	}
	return "", nil
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
		return fmt.Sprintf("the task worktree already has its own seed, and it disagrees with the one Seed-Child would write: %s", err.Error()), true
	default:
		return "", false
	}
}
