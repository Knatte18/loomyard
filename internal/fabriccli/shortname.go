// shortname.go implements `lyx fabric shortname [<shortname>]`: it prints the hub's recorded shortname,
// or records one on a hub that has none, which is how a repo bound before shortnames existed gets its shortname.

package fabriccli

import (
	"context"
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
)

// runShortname executes the fabric shortname subcommand.
// With no argument it prints the recorded shortname; with one it validates it and records it when the hub has none.
// The hub's shortname is repo-wide, so the verb reads and writes the board root, reached from any worktree of the hub.
func runShortname(ctx context.Context, out io.Writer, args []string) int {
	// Nothing has been mutated yet at cwd/location resolution: a bare output.Err carries no record.
	_, l, err := resolveWarpLocation(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}
	boardDir := fabricengine.BoardDir(l.HubPath)
	recorded, found := fabricengine.ReadShortname(boardDir)

	if len(args) == 0 {
		if !found {
			return output.Err(out, "this hub records no repo shortname; way forward: run `lyx fabric shortname <shortname>`, 2-6 characters matching [a-z][a-z0-9]{1,5}")
		}
		return output.Ok(out, map[string]any{"shortname": recorded})
	}

	shortname := args[0]
	if err := agentname.ValidateShortname(shortname); err != nil {
		return output.Err(out, err.Error())
	}
	if found {
		if recorded == shortname {
			return okWithRecord(out, fabricengine.NewMutations(l.HubPath).Snapshot(), map[string]any{"shortname": recorded})
		}
		return output.Err(out, fmt.Sprintf("this hub's recorded shortname is %q, not %q; refusing to change it, since that would orphan every name already in use — pass %q or no argument", recorded, shortname, recorded))
	}

	// The board is live, so the record is committed alone under the board write lock rather than through Bolt.Commit,
	// which would sweep any pending board change into this commit.
	rec := fabricengine.NewMutations(l.HubPath)
	if _, _, err := fabricengine.CommitShortname(l.HubPath, shortname, "fabric shortname: record the repo's shortname", rec); err != nil {
		return errWithRecord(out, rec.Snapshot(), err)
	}
	// The push records no branch_pushed entry, for the reason CloneAndWire spells out, but a seed-commit drop behind it is recorded in rec.
	if err := fabricengine.NewBolt(boardDir).PushRecorded(fabricengine.SyncOptions{}, rec); err != nil {
		return errWithRecord(out, rec.Snapshot(), err)
	}
	return okWithRecord(out, rec.Snapshot(), map[string]any{"shortname": shortname})
}
