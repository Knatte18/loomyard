// delivery.go implements creelengine's Delivery seam over the hub's worktrees, reed's persisted state and shuttle.

package creelcli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// worktreeOps is the four calls the adapter makes on one worktree: reed's status and a shuttle runner's idle probe, send and interrupt.
type worktreeOps interface {
	// Status reports the worktree's strands and whether each is live.
	Status() (reedengine.StatusResult, error)
	// SessionIdle reports whether the strand with the guid can take typed input.
	SessionIdle(guid string) (shuttleengine.IdleProbe, error)
	// Send types text into the strand with the guid and confirms it landed.
	Send(guid, text string) error
	// Interrupt sends the guarded interrupt to the strand with the guid.
	Interrupt(guid string) error
}

// hubDelivery is creelengine.Delivery over one hub: the worktrees git lists, each worktree's reed state and a shuttle runner built per call.
type hubDelivery struct {
	hubPath string
	// wire builds the worktreeOps of a resolved worktree; a test replaces it with a stub.
	wire func(location *lyxcwd.Location) (worktreeOps, error)
}

// newHubDelivery returns the Delivery of the hub at hubPath, building each worktree's shuttle engines with enginesFor.
func newHubDelivery(hubPath string, enginesFor func(shuttleengine.Config, configengine.Dirs) (shuttleengine.EngineSet, error)) *hubDelivery {
	return &hubDelivery{
		hubPath: hubPath,
		wire: func(location *lyxcwd.Location) (worktreeOps, error) {
			return wireShuttle(location, enginesFor)
		},
	}
}

// shuttleOps is a worktree's reed engine and the shuttle runner over it.
type shuttleOps struct {
	reed   *reedengine.Engine
	runner *shuttleengine.Runner
}

// wireShuttle builds the worktree's reed and shuttle config, its reed geometry, and the reed engine and runner over them.
func wireShuttle(location *lyxcwd.Location, enginesFor func(shuttleengine.Config, configengine.Dirs) (shuttleengine.EngineSet, error)) (shuttleOps, error) {
	configDirs := fabricengine.ConfigDirs(location)
	reedCfg, err := reedengine.LoadConfig(configDirs, "reed")
	if err != nil {
		return shuttleOps{}, err
	}
	shuttleCfg, err := shuttleengine.LoadConfig(configDirs, "shuttle")
	if err != nil {
		return shuttleOps{}, err
	}
	reedGeom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return shuttleOps{}, err
	}
	engines, err := enginesFor(shuttleCfg, configDirs)
	if err != nil {
		return shuttleOps{}, err
	}
	reed := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	runner := shuttleengine.NewRunner(reed, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	return shuttleOps{reed: reed, runner: runner}, nil
}

func (o shuttleOps) Status() (reedengine.StatusResult, error) { return o.reed.Status() }

func (o shuttleOps) SessionIdle(guid string) (shuttleengine.IdleProbe, error) {
	return o.runner.SessionIdle(guid)
}

func (o shuttleOps) Send(guid, text string) error { return o.runner.Send(guid, text) }

func (o shuttleOps) Interrupt(guid string) error { return o.runner.InterruptRestoring(guid) }

// Strands lists every strand reed persists in any worktree of the hub.
// A worktree that does not resolve, a reed state that cannot be read and a status that fails each fail the call naming the worktree,
// so an unreadable strand is never mistaken for an absent one.
func (d *hubDelivery) Strands() ([]creelengine.Strand, error) {
	entries, err := fabricengine.List(d.primeWorktree())
	if err != nil {
		return nil, err
	}
	var strands []creelengine.Strand
	for _, entry := range entries {
		if entry.Prunable {
			continue
		}
		found, err := d.worktreeStrands(entry)
		if err != nil {
			return nil, err
		}
		strands = append(strands, found...)
	}
	return strands, nil
}

// primeWorktree is the hub's prime worktree directory, named for the repository the hub holds.
func (d *hubDelivery) primeWorktree() string {
	return filepath.Join(d.hubPath, strings.TrimSuffix(filepath.Base(d.hubPath), fabricengine.HubSuffix))
}

// worktreeStrands reads the strands persisted in one worktree and their liveness from its reed status.
func (d *hubDelivery) worktreeStrands(entry fabricengine.WorktreeEntry) ([]creelengine.Strand, error) {
	location, err := lyxcwd.ResolveWorktree(entry.Path)
	if err != nil {
		return nil, fmt.Errorf("creel: resolve worktree %s: %w", entry.Path, err)
	}
	reedState, err := reedengine.LoadState(filepath.Join(location.AnchorPath(), lyxdirs.DotLyxDirName))
	if err != nil {
		return nil, fmt.Errorf("creel: read the reed state of worktree %s: %w", entry.Path, err)
	}
	if reedState == nil || len(reedState.Strands) == 0 {
		return nil, nil
	}
	ops, err := d.wire(location)
	if err != nil {
		return nil, fmt.Errorf("creel: wire worktree %s: %w", entry.Path, err)
	}
	status, err := ops.Status()
	switch {
	case errors.Is(err, reedengine.ErrNoSession):
		status = reedengine.StatusResult{}
	case err != nil:
		return nil, fmt.Errorf("creel: read the reed status of worktree %s: %w", entry.Path, err)
	}
	statusByGUID := make(map[string]reedengine.StrandStatus, len(status.Strands))
	for _, strandStatus := range status.Strands {
		statusByGUID[strandStatus.GUID] = strandStatus
	}
	var slug string
	if !entry.Main {
		slug = location.WorktreeName
	}
	strands := make([]creelengine.Strand, 0, len(reedState.Strands))
	for _, persisted := range reedState.Strands {
		strandStatus := statusByGUID[persisted.GUID]
		strands = append(strands, creelengine.Strand{
			Name:     persisted.Name,
			GUID:     persisted.GUID,
			Worktree: entry.Path,
			Slug:     slug,
			Live:     strandStatus.Live,
			PanePID:  strandStatus.PanePID,
		})
	}
	return strands, nil
}

// opsFor resolves the strand's worktree and wires it.
func (d *hubDelivery) opsFor(strand creelengine.Strand) (worktreeOps, error) {
	location, err := lyxcwd.ResolveWorktree(strand.Worktree)
	if err != nil {
		return nil, fmt.Errorf("creel: resolve worktree %s: %w", strand.Worktree, err)
	}
	ops, err := d.wire(location)
	if err != nil {
		return nil, fmt.Errorf("creel: wire worktree %s: %w", strand.Worktree, err)
	}
	return ops, nil
}

// Idle reports whether the strand's agent can take typed input, and for a pane that is not idle the stall the probe found.
func (d *hubDelivery) Idle(strand creelengine.Strand) (creelengine.Idleness, error) {
	ops, err := d.opsFor(strand)
	if err != nil {
		return creelengine.Idleness{}, err
	}
	probe, err := ops.SessionIdle(strand.GUID)
	if err != nil {
		return creelengine.Idleness{}, err
	}
	if probe.Idle {
		return creelengine.Idleness{Idle: true}, nil
	}
	switch {
	case probe.TooShort:
		return creelengine.Idleness{Stall: creelengine.StallTooShort}, nil
	case probe.Draft:
		return creelengine.Idleness{Stall: creelengine.StallDraft}, nil
	}
	return creelengine.Idleness{}, nil
}

// Paused reports whether a pause covers the strand.
// A prime strand is covered by a hub entry only, a pair strand by a hub entry or its own.
func (d *hubDelivery) Paused(strand creelengine.Strand) (bool, error) {
	location, err := lyxcwd.ResolveWorktree(strand.Worktree)
	if err != nil {
		return false, fmt.Errorf("creel: resolve worktree %s: %w", strand.Worktree, err)
	}
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return false, err
	}
	coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, strand.Slug, time.Now())
	if err != nil {
		return false, fmt.Errorf("creel: read the pause record %s: %w", geometry.RecordPath, err)
	}
	return coverage.Covered, nil
}

// Send types one line into the strand's pane and confirms it landed.
func (d *hubDelivery) Send(strand creelengine.Strand, text string) error {
	ops, err := d.opsFor(strand)
	if err != nil {
		return err
	}
	return mapDeliveryError(ops.Send(strand.GUID, text))
}

// Interrupt sends the guarded interrupt to the strand's running turn.
func (d *hubDelivery) Interrupt(strand creelengine.Strand) error {
	ops, err := d.opsFor(strand)
	if err != nil {
		return err
	}
	return mapDeliveryError(ops.Interrupt(strand.GUID))
}

// mapDeliveryError maps shuttle's send and interrupt sentinels onto creel's, and passes any other error through.
func mapDeliveryError(err error) error {
	switch {
	case errors.Is(err, shuttleengine.ErrSessionBusy):
		return creelengine.ErrBusy
	case errors.Is(err, shuttleengine.ErrSubmissionNotLanded):
		return creelengine.ErrNotLanded
	case errors.Is(err, shuttleengine.ErrNoRunningTurn):
		return creelengine.ErrNoRunningTurn
	case errors.Is(err, shuttleengine.ErrInterruptClearFailed):
		return fmt.Errorf("%w: %s", creelengine.ErrClearFailed, err)
	default:
		return err
	}
}
