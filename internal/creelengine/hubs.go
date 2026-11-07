// hubs.go declares the told seam to the other hubs of the machine and the recording of this daemon's hub.

package creelengine

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// HubDirectory is the seam through which the engine reaches the other hubs of the machine.
// A nil HubDirectory means a daemon that serves no other hub.
type HubDirectory interface {
	// Record records this hub in the machine record, so the hubs of the machine can address it.
	Record() error
	// Lookup answers the path of the one hub that recorded shortname.
	// An error matching ErrHubUnresolved means no hub or several hubs read it.
	// Any other error is a fault of reading the record.
	Lookup(shortname string) (string, error)
	// Geometry builds the geometry of the hub at hubPath.
	Geometry(hubPath string) (Geometry, error)
	// Spawn starts the daemon of the hub at hubPath, with its stderr appended to the log at logPath.
	Spawn(hubPath, logPath string) (Spawn, error)
	// TrustedHubs reads the repository names, owner/repo, of the hubs whose orch this hub obeys, at each call.
	TrustedHubs() ([]string, error)
	// RepositoryName answers the repository name, owner/repo, of the hub at hubPath, read from that hub's origin remote.
	// An error means the hub has no repository name, and its text says why.
	RepositoryName(hubPath string) (string, error)
}

// ListsRepository reports whether listed names repository, ignoring case as GitHub does.
// An empty repository is in no list, so a hub with no repository name is never trusted.
func ListsRepository(listed []string, repository string) bool {
	return repository != "" && slices.ContainsFunc(listed, func(entry string) bool { return strings.EqualFold(entry, repository) })
}

// ErrHubUnresolved is matched by the refusal of a shortname that names no hub or more than one hub on this machine.
var ErrHubUnresolved = errors.New("creel: shortname does not name exactly one hub")

// hubUnresolved is a refusal that carries its own text and matches the unresolved-hub sentinel under errors.Is.
type hubUnresolved string

func (e hubUnresolved) Error() string { return string(e) }

func (e hubUnresolved) Is(target error) bool { return target == ErrHubUnresolved }

// NewHubUnresolved returns an error with the given text that matches the unresolved-hub sentinel under errors.Is.
func NewHubUnresolved(text string) error { return hubUnresolved(text) }

// hubRecordInterval is how soon after a record that succeeded the daemon records its hub again at a served local verb.
const hubRecordInterval = time.Minute

// hubRecordState is what the daemon remembers of recording its hub.
type hubRecordState struct {
	mu sync.Mutex
	// recorded is true while the last record succeeded, and at is the clock time of that record.
	recorded bool
	at       time.Time
	// warned is true while a failing record's Warn for the current episode is out.
	warned bool
}

// recordHub records this daemon's hub in the machine record.
// After a record that succeeded, it records at most once per hubRecordInterval.
// After one that failed, it records at the next call.
// A failed record logs one Warn per episode.
// The daemon serves on, since a hub that cannot be addressed must not lose its own mail.
func (d *daemon) recordHub() {
	hubs := d.opts.Hubs
	if hubs == nil {
		return
	}
	state := &d.hubRecord
	state.mu.Lock()
	defer state.mu.Unlock()
	now := d.opts.Clock.Now()
	if state.recorded && now.Sub(state.at) < hubRecordInterval {
		return
	}
	if err := hubs.Record(); err != nil {
		state.recorded = false
		if !state.warned {
			state.warned = true
			logger.Warn("creel daemon: could not record this hub in the machine record, so other hubs cannot address it", "error", err)
		}
		return
	}
	state.recorded, state.at, state.warned = true, now, false
}

// SetHubs sets the seam to the other hubs and the platform the daemon runs on.
// It is called once, before the scheduler starts.
// A nil seam keeps a store that serves no other hub.
func (s *Store) SetHubs(hubs HubDirectory, goos string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hubs, s.goos = hubs, goos
}
