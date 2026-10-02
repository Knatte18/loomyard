// code.go implements `lyx fabric code [<code>]`: it prints the hub's recorded agent-name code, or
// records one on a hub that has none, which is how a repo bound before codes existed gets its code.

package fabriccli

import (
	"context"
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
)

// runCode executes the fabric code subcommand.
// With no argument it prints the recorded code; with one it validates it and records it when the hub has none.
// The hub's code is repo-wide, so the verb reads and writes the board root, reached from any worktree of the hub.
func runCode(ctx context.Context, out io.Writer, args []string) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}
	boardDir := fabricengine.BoardDir(l.HubPath)
	recorded, found := fabricengine.ReadCode(boardDir)

	if len(args) == 0 {
		if !found {
			return output.Err(out, "this hub records no repo code; way forward: run `lyx fabric code <code>`, 2-6 characters matching [a-z][a-z0-9]{1,5}")
		}
		return output.Ok(out, map[string]any{"code": recorded})
	}

	code := args[0]
	if err := agentname.ValidateCode(code); err != nil {
		return output.Err(out, err.Error())
	}
	if found {
		if recorded == code {
			return okWithRecord(out, fabricengine.NewMutations(l.HubPath).Snapshot(), map[string]any{"code": recorded})
		}
		return output.Err(out, fmt.Sprintf("this hub's recorded code is %q, not %q; refusing to change it, since that would orphan every name already in use — pass %q or no argument", recorded, code, recorded))
	}

	rec := fabricengine.NewMutations(l.HubPath)
	if err := fabricengine.WriteCode(boardDir, code); err != nil {
		return errWithRecord(out, rec.Snapshot(), err)
	}
	rec.Append(fabricengine.KindFileWritten, fabricengine.CodeFileName, "")

	b := fabricengine.NewBolt(boardDir)
	sha, committed, err := b.Commit("fabric code: record the repo's short code", fabricengine.SyncOptions{})
	if err != nil {
		return errWithRecord(out, rec.Snapshot(), err)
	}
	if committed {
		rec.Append(fabricengine.KindCommitCreated, boardDir, sha)
	}
	// Bolt.Push records nothing, for the reason CloneAndWire spells out.
	if err := b.Push(fabricengine.SyncOptions{}); err != nil {
		return errWithRecord(out, rec.Snapshot(), err)
	}
	return okWithRecord(out, rec.Snapshot(), map[string]any{"code": code})
}
