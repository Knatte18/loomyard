// hubs.go fakes creelengine.HubDirectory, the seam to the other hubs of the machine.

package creelfake

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Knatte18/loomyard/internal/creelengine"
)

var _ creelengine.HubDirectory = (*Hubs)(nil)

// Hubs is a creelengine.HubDirectory whose hubs and answers are scripted.
// With nothing set, Spawn answers an error and starts nothing.
// Geometry of an unknown path answers an error.
// TrustedHubs answers an empty list.
type Hubs struct {
	mu          sync.Mutex
	paths       map[string][]string
	geometries  map[string]creelengine.Geometry
	spawnFn     func(hubPath string) (creelengine.Spawn, error)
	trusted     []string
	trustedErr  error
	recordErr   error
	lookupErrs  map[string]error
	geometryErr map[string]error
	records     int
	spawns      map[string]int
}

// NewHubs returns a Hubs that knows no hub.
func NewHubs() *Hubs {
	return &Hubs{
		paths: map[string][]string{}, geometries: map[string]creelengine.Geometry{},
		lookupErrs: map[string]error{}, geometryErr: map[string]error{}, spawns: map[string]int{},
	}
}

// Add makes shortname name the hub of geom; a second hub under one shortname makes the lookup refuse it as shared.
func (h *Hubs) Add(shortname string, geom creelengine.Geometry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.paths[shortname] = append(h.paths[shortname], geom.HubPath)
	h.geometries[geom.HubPath] = geom
}

// SetSpawn sets fn as the function that starts a hub's daemon, called without the fake's lock held;
// nil restores the error answer.
func (h *Hubs) SetSpawn(fn func(hubPath string) (creelengine.Spawn, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.spawnFn = fn
}

// SetTrusted scripts the shortnames and the error answered for the trusted hubs.
func (h *Hubs) SetTrusted(shortnames []string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.trusted, h.trustedErr = shortnames, err
}

// SetRecordError makes the record operation fail with err until it is set to nil.
func (h *Hubs) SetRecordError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recordErr = err
}

// SetLookupError makes the lookup of shortname fail with err; nil removes it.
func (h *Hubs) SetLookupError(shortname string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err == nil {
		delete(h.lookupErrs, shortname)
		return
	}
	h.lookupErrs[shortname] = err
}

// SetGeometryError makes the geometry query of hubPath fail with err; nil removes it.
func (h *Hubs) SetGeometryError(hubPath string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err == nil {
		delete(h.geometryErr, hubPath)
		return
	}
	h.geometryErr[hubPath] = err
}

// Records returns how many times the record operation was called.
func (h *Hubs) Records() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.records
}

// Spawns returns how many times the spawn operation was called for hubPath.
func (h *Hubs) Spawns(hubPath string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.spawns[hubPath]
}

// Record implements creelengine.HubDirectory.
func (h *Hubs) Record() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records++
	return h.recordErr
}

// Lookup implements creelengine.HubDirectory.
// An unknown shortname and one added twice are refused with an error matching creelengine.ErrHubUnresolved.
func (h *Hubs) Lookup(shortname string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.lookupErrs[shortname]; err != nil {
		return "", err
	}
	paths := slices.Clone(h.paths[shortname])
	slices.Sort(paths)
	switch len(paths) {
	case 1:
		return paths[0], nil
	case 0:
		return "", creelengine.NewHubUnresolved(fmt.Sprintf("creel: no hub with the shortname %q is recorded on this machine; way forward: run lym creel inbox in that hub once", shortname))
	default:
		return "", creelengine.NewHubUnresolved(fmt.Sprintf("creel: mail across hubs needs distinct shortnames, but %d hubs on this machine read the shortname %q; way forward: remove the extra clone, since a recorded shortname never changes; the hubs are %s", len(paths), shortname, strings.Join(paths, ", ")))
	}
}

// Geometry implements creelengine.HubDirectory.
func (h *Hubs) Geometry(hubPath string) (creelengine.Geometry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.geometryErr[hubPath]; err != nil {
		return creelengine.Geometry{}, err
	}
	geom, ok := h.geometries[hubPath]
	if !ok {
		return creelengine.Geometry{}, fmt.Errorf("creelfake: no geometry is set for the hub %q", hubPath)
	}
	return geom, nil
}

// Spawn implements creelengine.HubDirectory.
// It counts the call, and answers an error without starting anything when no spawn function is set.
func (h *Hubs) Spawn(hubPath, logPath string) (creelengine.Spawn, error) {
	h.mu.Lock()
	h.spawns[hubPath]++
	fn := h.spawnFn
	h.mu.Unlock()
	if fn == nil {
		return creelengine.Spawn{}, errors.New("creelfake: no spawn is set")
	}
	return fn(hubPath)
}

// TrustedHubs implements creelengine.HubDirectory.
func (h *Hubs) TrustedHubs() ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.trusted), h.trustedErr
}
