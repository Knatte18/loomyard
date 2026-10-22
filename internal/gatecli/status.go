// status.go implements `lyx gate status`, the read-only view of which slots of the hub's gate-slot pool are held, by which run and tier, and since when.

package gatecli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// statusCmd builds `lyx gate status`.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "show which gate slots are held, by which run and tier, and since when",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `status shows the hub's gate-slot pool: its slot count, whether a run that needs several slots holds the drain, which makes new single-slot runs wait even while a slot is free, and one entry per held slot with its slot number, the holding worktree, site and tier, its pid and when it took the slot.
It takes no slot and changes nothing.
Outside every hub it answers in_hub false, since the tiers run unslotted there.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), runStatus(cmd.Context(), cmd.OutOrStdout()))
			return nil
		},
	}
}

// runStatus writes the status of the gate-slot pool of the hub that contains the seam cwd and returns the verb's exit code.
func runStatus(ctx context.Context, out io.Writer) int {
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate status: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate status: %v; way forward: re-run the same command", err))
	}
	if !inHub {
		return output.Ok(out, map[string]any{"in_hub": false, "held": []map[string]any{}, "message": "gate status: no hub contains this directory, so its tiers run unslotted"})
	}

	pool := hubgeom.GateSlots(location)
	limits, err := pool.Limits()
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate status: %v; way forward: take the step it names, then re-run the same command", err))
	}
	holders, err := pool.Holders()
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate status: %v; way forward: re-run the same command", err))
	}
	draining, err := pool.Draining()
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate status: read the drain lock: %v; way forward: re-run the same command", err))
	}

	held := make([]map[string]any, len(holders))
	for i, holder := range holders {
		held[i] = map[string]any{
			"slot":     holder.Slot,
			"worktree": holder.Worktree,
			"site":     holder.Site,
			"tier":     holder.Tier,
			"pid":      holder.PID,
			"started":  holder.Started.Format(time.RFC3339),
		}
	}
	return output.Ok(out, map[string]any{"in_hub": true, "slots": limits.Slots, "draining": draining, "held": held})
}
