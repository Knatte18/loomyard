// hubsverb.go implements `lym creel hubs`, which lists the hubs of the machine record that mail across hubs can address.

package creelcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelconfig"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/preflight"
)

// currentHubPath resolves the hub of the cwd ctx carries.
func currentHubPath(ctx context.Context) (string, error) {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return "", err
	}
	location, _, err := preflight.ResolveMode(cwd)
	if err != nil {
		return "", err
	}
	return location.HubPath, nil
}

// daemonAnswers reports whether a daemon accepts a connection on the socket;
// it closes the connection at once and starts nothing.
func daemonAnswers(socketPath string) bool {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// hubsCmd returns the `creel hubs` verb.
func hubsCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "hubs",
		Short:       "list the other hubs of this machine that creel mail can reach",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `hubs lists the hubs recorded on this machine whose shortname record still reads
the shortname they recorded, as {"hubs":[{shortname, hub_path, daemon_answers,
trusted}]}. daemon_answers says whether the hub's daemon answers on its socket;
the verb never starts a daemon and writes no record. trusted says whether this
hub's creel.yaml lists the shortname in trusted_hubs, so its orch's mail is
obeyed here.

To mail a strand of another hub, post to its full name, as lym creel post
<shortname>:orch "<text>" does.

Example:
  lym creel hubs`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, 0, func(_ *creelengine.Client, _ io.Writer) (result, error) {
				hubPath, err := currentHubPath(cmd.Context())
				if err != nil {
					return result{}, err
				}
				registryDir, err := seam.registryDir()
				if err != nil {
					return result{}, err
				}
				live, err := hubregistry.Live(registryDir)
				if err != nil {
					return result{}, err
				}
				cfg, err := creelconfig.LoadConfig(configengine.Dirs{Board: fabricengine.BoltDir(hubPath)})
				if errors.Is(err, configengine.ErrInvalid) {
					return result{}, fmt.Errorf("%w; way forward: correct the file with `lym config creel`", err)
				}
				if err != nil {
					return result{}, err
				}
				root, err := lymtemp.ProcessRoot()
				if err != nil {
					return result{}, fmt.Errorf("open the lymtemp root: %w", err)
				}
				hubs := make([]map[string]any, 0, len(live))
				for _, entry := range live {
					geometry, err := hubgeom.CreelGeometry(entry.HubPath, root.Path())
					if err != nil {
						return result{}, err
					}
					hubs = append(hubs, map[string]any{
						"shortname": entry.Shortname, "hub_path": entry.HubPath,
						"daemon_answers": daemonAnswers(geometry.SocketPath), "trusted": slices.Contains(cfg.TrustedHubs, entry.Shortname),
					})
				}
				return result{fields: map[string]any{"hubs": hubs}}, nil
			})
		},
	}
}
