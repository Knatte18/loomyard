// unwire.go implements the fabriccli handler for the fabric unwire subcommand: a per-warp-worktree
// full deactivation of fabric wiring, the teardown successor to the deleted `lyx init --undo`.

package fabriccli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
)

// runUnwire executes the fabric unwire subcommand, removing every on-disk
// fabric junction for this worktree. Weft-side content, including _lyx and
// .lyx, is never touched.
func runUnwire(out io.Writer, _ []string) int {
	cwd, _, err := resolveWarpLocation()
	if err != nil {
		return output.Err(out, err.Error())
	}

	res, err := fabricengine.Unwire(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"junctions_removed": res.JunctionsRemoved,
		"weft_content":      res.WeftContent,
		"git_exclude":       res.GitExclude,
	})
}
