// clone.go implements the fabriccli handler half for the fabric clone subcommand.
// runCloneWithReset delegates into fabricengine.CloneHub after optionally tearing
// down an existing hub when --reset is given.

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
// so tests can inject errors by swapping that exported var.
func runCloneWithReset(out io.Writer, args []string, reset bool) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	if len(args) != 2 {
		return output.Err(out, "usage: lyx fabric clone [--reset] <host-url> <weft-url>")
	}
	hostURL := args[0]
	weftURL := args[1]

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

	hubPath, err := fabricengine.CloneHub(cwd, hostURL, weftURL)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"hub": hubPath,
	})
}
