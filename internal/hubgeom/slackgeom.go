// slackgeom.go builds the geometry the Slack engine is told for one hub.

package hubgeom

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/slackengine"
)

// SlackGeometry builds the slackengine.Geometry of the hub at hubPath.
// A hub with no recorded shortname is an error.
// The token path and the machine socket lock lie in the per-user credential directory.
// A machine whose name cannot be read is an error, which is machinename.Name's own.
// A machine where that directory does not resolve is an error naming the environment variables that decide it.
func SlackGeometry(hubPath string) (slackengine.Geometry, error) {
	creel, err := CreelGeometry(hubPath, "")
	if err != nil {
		return slackengine.Geometry{}, err
	}
	credentialDir, ok := githubclient.CredentialDir()
	if !ok {
		return slackengine.Geometry{}, errors.New("slack: no per-user directory for the token file; way forward: set HOME, XDG_CONFIG_HOME or LOCALAPPDATA")
	}
	executable, err := os.Executable()
	if err != nil {
		return slackengine.Geometry{}, fmt.Errorf("slack: find the running lym binary: %w", err)
	}
	machine, err := machinename.Name()
	if err != nil {
		return slackengine.Geometry{}, err
	}
	dir := filepath.Join(fabricengine.HubScratchDir(hubPath), lymdirs.SlackDirName)
	return slackengine.Geometry{
		HubPath:        hubPath,
		Shortname:      creel.Shortname,
		Machine:        machine,
		Dir:            dir,
		TokenPath:      filepath.Join(credentialDir, slackengine.TokenFileName),
		ChannelMapPath: filepath.Join(dir, slackengine.ChannelMapFileName),
		InboxDir:       filepath.Join(dir, slackengine.InboxDirName),
		SocketLockPath: filepath.Join(credentialDir, slackengine.SocketLockFileName),
		SetupPath:      filepath.Join(dir, slackengine.SetupFileName),
		PresencePath:   filepath.Join(dir, slackengine.PresenceFileName),
		OutboxPath:     filepath.Join(dir, slackengine.OutboxFileName),
		RelayStatePath: filepath.Join(dir, slackengine.RelayStateFileName),
		LockPath:       filepath.Join(dir, slackengine.RelayLockFileName),
		LogPath:        filepath.Join(dir, slackengine.RelayLogFileName),
		RunProcDir:     fabricengine.RunProcDir(hubPath),
		LymExecutable:  executable,
	}, nil
}
