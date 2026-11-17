// stencilpins.go pins the stencils a run may render when the run is first seeded.
// A start or resume that puts a driver to work warns the run's parent orch.
// The warning names each pinned stencil whose installed board copy changed since.

package loomcli

import (
	"strings"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// stencilsChangedField is the envelope key that lists the stencils changed since the run started.
const stencilsChangedField = "stencils_changed"

// writeStencilPins records the body hash of each installed stencil as the run's stencil pins.
// A failed hash or write is logged at Warn:
// the run starts without pins, and its resumes warn of nothing.
func (c *loomCLI) writeStencilPins() {
	pins, err := stencilstore.Pins(c.runDeps.Geom.StencilsDir, stencils.Registry())
	if err != nil {
		logger.Warn("loom: could not hash the installed stencils; the run starts without stencil pins", "error", err)
		return
	}
	if err := shedrun.WriteStencilPins(c.location, shedrun.SelfRunID, shedrun.StencilPins{Stencils: pins}); err != nil {
		logger.Warn("loom: could not write the run's stencil pins; the run starts without them", "error", err)
	}
}

// warnChangedStencils compares the run's stencil pins with the installed board copies.
// It returns the names of the pinned stencils that changed or are gone.
// When any did, it posts one notice to the run's parent orch naming them and `lym stencil diff`.
// A run with no pins returns none.
// The notice names `lym stencil diff` as the way to inspect each one against its shipped default and its board copy.
// A failed read, compare or post is logged at Warn and never fails the verb that resumes the run.
func (c *loomCLI) warnChangedStencils() []string {
	pins, found, err := shedrun.ReadStencilPins(c.location, c.runID)
	if err != nil {
		logger.Warn("loom: could not read the run's stencil pins; no stencil change is reported", "error", err)
		return nil
	}
	if !found {
		return nil
	}
	changed, err := stencilstore.ChangedPins(c.runDeps.Geom.StencilsDir, pins.Stencils)
	if err != nil {
		logger.Warn("loom: could not compare the installed stencils with the run's pins; no stencil change is reported", "error", err)
		return nil
	}
	if len(changed) == 0 {
		return nil
	}
	if err := c.notifyParent(stencilsChangedNotice(seedSlug(c.location.WorktreeName), changed)); err != nil {
		logger.Warn("loom: could not tell the parent orch which stencils changed since the run started", "changed", changed, "error", err)
	}
	return changed
}

// stencilsChangedNotice is the parent notice naming the stencils of the run slug that changed since it started.
func stencilsChangedNotice(slug string, changed []string) string {
	return "loom " + slug + ": the stencils " + strings.Join(changed, ", ") +
		` changed since the run started, so its later rows render prompts that differ from its earlier ones;` +
		` inspect each with "lym stencil diff <name>", which compares it with its shipped default and its board copy`
}

// withChangedStencils returns fields with changed listed under stencilsChangedField.
// It returns fields as they are when changed is empty.
func withChangedStencils(fields map[string]any, changed []string) map[string]any {
	if len(changed) > 0 {
		fields[stencilsChangedField] = changed
	}
	return fields
}
