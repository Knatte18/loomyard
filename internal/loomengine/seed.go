// seed.go implements VerifySeedOwnership, the guard that refuses a loom status file recording another task's slug.

package loomengine

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// VerifySeedOwnership reports an error when the status file at statusPath records a different
// task's slug than wantSlug — the state a worktree inherits when it is forked (via `lym fabric
// add`) from a task worktree whose pair still carries `_lym/loom/status.json`. Without this guard
// the driver silently resumed the INHERITED task's run — observed live as a new task failing at
// the OLD task's Finalize, with the status envelope naming the old slug (crucible round
// fable5-high-r3, F-B7).
//
// A missing file, an empty recorded slug, a product that does not decode, and a decode failure on the outer shed envelope itself (malformed JSON) all pass: the caller's own missing-file handling and the driver's own read of the status file answer them.
// A genuine read or lock failure, anything that is not a decode failure, is still returned as its own error, never converted into a verdict.
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
