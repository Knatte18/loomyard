// hubdirectory.go implements the engine's HubDirectory seam over the machine record of hubs, the hub geometry and the daemon spawn.

package creelcli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelconfig"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubregistry"
)

var _ creelengine.HubDirectory = (*hubDirectory)(nil)

// hubDirectory is the production creelengine.HubDirectory of one hub's daemon.
type hubDirectory struct {
	hubPath   string
	shortname string
	tempRoot  string
	// registryDir is the per-user state directory the machine record sits in; registryErr is why it could not be read, when it could not.
	registryDir string
	registryErr error
}

// newHubDirectory returns the directory of the hub at hubPath, which records itself under shortname in registryDir and builds other hubs' sockets under tempRoot.
// A non-nil registryErr makes Record and Lookup refuse naming that cause, so a hub without a readable state directory still serves its own strands.
func newHubDirectory(hubPath, shortname, tempRoot, registryDir string, registryErr error) *hubDirectory {
	return &hubDirectory{hubPath: hubPath, shortname: shortname, tempRoot: tempRoot, registryDir: registryDir, registryErr: registryErr}
}

// daemonHubs returns the directory the daemon of the hub at hubPath is told, with the per-user state directory read through the client seam.
func daemonHubs(hubPath, shortname, tempRoot string) *hubDirectory {
	registryDir, err := seam.registryDir()
	return newHubDirectory(hubPath, shortname, tempRoot, registryDir, err)
}

// unreadableRegistry is the refusal that names why the machine record cannot be reached.
func (d *hubDirectory) unreadableRegistry() error {
	return fmt.Errorf("creel: the per-user state directory cannot be read (%w); way forward: set XDG_STATE_HOME or HOME in the environment the hub's daemon starts from", d.registryErr)
}

// Record records this hub in the machine record.
func (d *hubDirectory) Record() error {
	if d.registryErr != nil {
		return d.unreadableRegistry()
	}
	return hubregistry.Record(d.registryDir, d.hubPath, d.shortname, seam.clock.Now())
}

// Lookup answers the path of the one hub that recorded shortname.
// The refusal of an unknown or shared shortname matches creelengine.ErrHubUnresolved and a record that cannot be read does not.
func (d *hubDirectory) Lookup(shortname string) (string, error) {
	if d.registryErr != nil {
		return "", d.unreadableRegistry()
	}
	path, err := hubregistry.Lookup(d.registryDir, shortname)
	if errors.Is(err, hubregistry.ErrUnresolved) {
		return "", creelengine.NewHubUnresolved(err.Error())
	}
	return path, err
}

// Geometry builds the geometry of the hub at hubPath.
func (d *hubDirectory) Geometry(hubPath string) (creelengine.Geometry, error) {
	return hubgeom.CreelGeometry(hubPath, d.tempRoot)
}

// Spawn starts the daemon of the hub at hubPath through the client seam, so each start is logged and reaped.
func (d *hubDirectory) Spawn(hubPath, logPath string) (creelengine.Spawn, error) {
	return seam.spawn(hubPath, logPath)
}

// TrustedHubs reads this hub's creel.yaml at each call, so a changed list applies with no daemon restart.
func (d *hubDirectory) TrustedHubs() ([]string, error) {
	cfg, err := creelconfig.LoadConfig(configengine.Dirs{Board: fabricengine.BoltDir(d.hubPath)})
	if err != nil {
		return nil, err
	}
	return cfg.TrustedHubs, nil
}
