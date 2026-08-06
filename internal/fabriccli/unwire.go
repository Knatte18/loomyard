// unwire.go implements the fabriccli handler for the fabric unwire subcommand: a per-host-worktree
// full deactivation of fabric wiring, the teardown successor to the deleted `lyx init --undo`.

package fabriccli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// runUnwire executes the fabric unwire subcommand, removing every on-disk
// fabric junction for this worktree and clearing weft-side _lyx content.
func runUnwire(out io.Writer, _ []string) int {
	cwd, err := lyxcwd.Getwd()
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
		"gitignore":         res.Gitignore,
	})
}
