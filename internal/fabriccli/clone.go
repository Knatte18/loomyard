// clone.go implements the fabriccli handler half for the fabric clone subcommand.
// runCloneWithReset delegates into fabricengine.CloneHub after optionally tearing
// down an existing hub when --reset is given. Adapted from warpcli's clone.go —
// identical control flow, fabricengine.RemoveAll/CloneHub in place of warpengine's.

package fabriccli

import (
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
)

// runCloneWithReset executes the clone subcommand.
//
// When reset is true it tears down any existing hub at the derived path before
// cloning, making the operation idempotent. The teardown uses fabricengine.RemoveAll
// so tests can inject errors by swapping that exported var. This is fabric's own
// teardown seam, distinct from warpcli's — the two modules never share a clone
// teardown path during the parallel-build period.
func runCloneWithReset(out io.Writer, args []string, reset bool) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	if len(args) < 2 {
		return output.Err(out, "usage: lyx fabric clone [--reset] <host-url> <weft-url> [board-url]")
	}
	hostURL := args[0]
	weftURL := args[1]
	var boardURL string
	if len(args) >= 3 {
		boardURL = args[2]
	}

	if reset {
		// Derive the hub path so we can remove it before cloning (idempotent re-clone).
		// DeriveHostName returns "" for blank/unparseable URLs; guard defensively.
		name := fabricengine.DeriveHostName(hostURL)
		if name == "" {
			return output.Err(out, fmt.Sprintf("could not derive repo name from host URL %s", hostURL))
		}
		hubPath := hubgeometry.HubPath(cwd, name)
		if err := fabricengine.RemoveAll(hubPath); err != nil {
			return output.Err(out, fmt.Sprintf("reset: remove hub at %s: %v", hubPath, err))
		}
	}

	hubPath, resolvedBoard, err := fabricengine.CloneHub(cwd, hostURL, weftURL, boardURL)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"hub":   hubPath,
		"board": resolvedBoard,
	})
}
