// clone.go implements the warpcli handler half for the warp clone subcommand.
// runClone and runCloneWithReset delegate into warpengine.CloneHub after optionally
// tearing down an existing hub when --reset is given.

package warpcli

import (
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/warpengine"
)

// runClone executes the clone subcommand without the --reset flag.
// It is a convenience wrapper around runCloneWithReset with reset=false.
func runClone(out io.Writer, args []string) int {
	return runCloneWithReset(out, args, false)
}

// runCloneWithReset executes the clone subcommand.
//
// When reset is true it tears down any existing hub at the derived path before
// cloning, making the operation idempotent. The teardown uses warpengine.RemoveAll
// so tests can inject errors by swapping that exported var.
func runCloneWithReset(out io.Writer, args []string, reset bool) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	if len(args) < 2 {
		return output.Err(out, "usage: lyx warp clone [--reset] <host-url> <weft-url> [board-url]")
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
		name := warpengine.DeriveHostName(hostURL)
		if name == "" {
			return output.Err(out, fmt.Sprintf("could not derive repo name from host URL %s", hostURL))
		}
		hubPath := hubgeometry.HubPath(cwd, name)
		if err := warpengine.RemoveAll(hubPath); err != nil {
			return output.Err(out, fmt.Sprintf("reset: remove hub at %s: %v", hubPath, err))
		}
	}

	hubPath, resolvedBoard, err := warpengine.CloneHub(cwd, hostURL, weftURL, boardURL)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"hub":   hubPath,
		"board": resolvedBoard,
	})
}
