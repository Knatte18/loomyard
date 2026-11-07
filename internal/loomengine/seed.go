// seed.go implements VerifySeedOwnership, the guard that refuses a loom status file recording another task's slug.

package loomengine

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// VerifySeedOwnership reports an error when the status file at statusPath records a task slug other than wantSlug.
// A worktree forked with `lym fabric add` from a task worktree inherits that task's state.
// The inheritance happens when the pair still carries `_lym/loom/status.json`;
// without this guard the driver would silently resume the inherited task's run.
//
// A missing file, an empty recorded slug, and a product that does not decode all pass.
// Malformed JSON in the outer shed envelope passes too.
// The caller's missing-file handling and the driver's own read of the status file answer those cases.
// Any error that is not a decode failure is returned as is, never converted into a verdict.
func VerifySeedOwnership(statusPath, statusLockPath, wantSlug string) error {
	shed, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		if errors.Is(err, state.ErrDecode) {
			return nil
		}
		return err
	}
	if !found {
		return nil
	}

	var product Status
	if len(shed.Product) > 0 {
		if uerr := json.Unmarshal(shed.Product, &product); uerr != nil {
			return nil
		}
	}
	if product.Slug == "" || product.Slug == wantSlug {
		return nil
	}
	return fmt.Errorf("loomengine: status file %s records task %q, not this worktree's own %q — the task state was inherited (fabric add forks the whole pair, `_lym` task state included) or copied; reset the inherited `_lym` task state (loom, discussion, plan, webster) on the paired side and re-run, or drive task %q from its own worktree", statusPath, product.Slug, wantSlug, product.Slug)
}
