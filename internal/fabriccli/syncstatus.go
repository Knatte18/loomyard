// syncstatus.go holds the hidden `lym fabric sync-status` verb reed's status bar runs at each redraw.

package fabriccli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/spf13/cobra"
)

const (
	// SyncStatusVerb is the name of the hidden verb that prints a hub's sync field.
	SyncStatusVerb = "sync-status"
	// syncStatusHubFlag names the hub whose field the verb prints.
	syncStatusHubFlag = "hub"
	// syncStatusUnread is the field of a hub whose configuration or ledger does not read.
	syncStatusUnread = "sync: ?"
)

// syncStatusCmd builds the hidden sync-status verb.
// It writes no envelope: its one line is the field the bar draws, and it exits zero whatever it reads.
func syncStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    SyncStatusVerb,
		Short:  "print the hub's sync field for reed's status bar",
		Hidden: true,
		Annotations: map[string]string{
			clihelp.AudienceAnnotation:        clihelp.AudienceInternal,
			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
		},
		Long: `sync-status prints one line, ` + "`sync: <minutes>m · <seconds>s`" + `: the minutes since
the hub last had nothing owed to origin, and the push interval in force. It
only reads, and touches no network. A hub it cannot read prints
` + "`" + syncStatusUnread + "`" + `, and a directory that is no hub prints an empty line.
reed's status bar runs it at each redraw.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			hub, err := cmd.Flags().GetString(syncStatusHubFlag)
			if err != nil {
				return err
			}
			line, err := syncStatusLine(hub, time.Now())
			if err != nil {
				logger.Debug("fabriccli: the hub's sync field was not read", "hub", hub, "error", err)
				line = syncStatusUnread
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
			return nil
		},
	}
	cmd.Flags().String(syncStatusHubFlag, "", "the hub directory")
	return cmd
}

// syncStatusLine returns the sync field of the hub at hubPath, empty for a directory with no Bolt checkout.
// The base interval is the one the prime's configuration sets.
func syncStatusLine(hubPath string, now time.Time) (string, error) {
	bolt := fabriclayout.BoltDir(hubPath)
	if _, err := os.Stat(bolt); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	boltLocation, err := lymcwd.ResolveWorktree(bolt)
	if err != nil {
		return "", err
	}
	prime, err := hubgeom.PrimeWithoutGit(boltLocation)
	if err != nil {
		return "", err
	}
	config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(prime))
	if err != nil {
		return "", err
	}
	timing, _ := config.PushTiming()
	return pushledger.SyncStatus(hubPath, now, timing.Interval)
}
