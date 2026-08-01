// unwire.go implements the fabriccli handler for the fabric unwire
// subcommand: a per-host-worktree full deactivation of fabric wiring, the
// teardown successor to the deleted `lyx init --undo`.

package fabriccli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
)

// runUnwire executes the fabric unwire subcommand.
//
// It resolves the current working directory and delegates to
// fabricengine.Unwire, which removes every on-disk fabric junction for this
// worktree, clears the weft-side _lyx content (never _pattern), and reverts
// the managed .gitignore block's ".lyx/" entry. On success it emits a JSON
// object mirroring the deleted initcli runUndo's output keys.
func runUnwire(out io.Writer, _ []string) int {
	cwd, _ := hubgeometry.Getwd()

	res, err := fabricengine.Unwire(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"junctions_removed": res.JunctionsRemoved,
		"weft_content":      res.WeftContent,
		"git_exclude":       res.GitExclude,
		"gitignore":         res.Gitignore,
	})
}
