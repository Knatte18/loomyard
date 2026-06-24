// cli.go implements the RunCLI entry point for the git-clone command.

package gitclone

import (
	"io"

	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// RunCLI parses and executes the git-clone command, writing JSON results to out.
//
// It accepts exactly 2 or 3 positional arguments: hostURL, weftURL, and an optional boardURL.
// The boardURL defaults to the derived weft wiki URL if omitted.
//
// Precondition: the board repository (default: the weft repository's wiki) must already exist
// and be reachable via the provided URL. If the board repository is missing or inaccessible,
// the board clone fails and the entire command aborts with the Hub directory cleaned up.
//
// Returns exit code 0 on success or 1 on error. Success output is JSON with hub path and URLs;
// error output is JSON with an error message.
func RunCLI(out io.Writer, args []string) int {
	// Validate positional arguments
	if len(args) < 2 || len(args) > 3 {
		return output.Err(out, "usage: lyx git-clone <host-url> <weft-url> [board-url]")
	}

	hostURL := args[0]
	weftURL := args[1]
	boardURL := ""
	if len(args) == 3 {
		boardURL = args[2]
	}

	// Obtain current working directory
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Perform the clone
	hubPath, resolvedBoardURL, err := cloneHub(cwd, hostURL, weftURL, boardURL)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Return success with hub path and URLs (resolvedBoardURL comes from cloneHub)
	return output.Ok(out, map[string]any{
		"hub":   hubPath,
		"host":  hostURL,
		"weft":  weftURL,
		"board": resolvedBoardURL,
	})
}
