// unwire.go implements the fabriccli handler for the fabric unwire subcommand: a per-warp-worktree
// full deactivation of fabric wiring, the teardown successor to the deleted `lyx init --undo`.

package fabriccli

import (
	"context"
	"io"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
)

// runUnwire executes the fabric unwire subcommand, removing every on-disk
// fabric junction for this worktree. Weft-side content, including _lyx and
// .lyx, is never touched.
func runUnwire(ctx context.Context, out io.Writer, _ []string) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	cwd, _, err := resolveWarpLocation(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}

	res, err := fabricengine.Unwire(cwd)
	if err != nil {
		return errWithRecord(out, res.Mutated(), err)
	}
	return okWithRecord(out, res.Mutated(), map[string]any{
		"junctions_removed": res.JunctionsRemoved,
		"weft_content":      res.WeftContent,
		"git_exclude":       res.GitExclude,
		// _board is a named special case outside the pathspec-derived sweep, so its removal can
		// never appear in junctions_removed and must be surfaced under its own key.
		"board_junction_removed": res.BoardJunctionRemoved,
	})
}
