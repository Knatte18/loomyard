// delivery.go implements creelengine's Delivery seam over the hub's worktrees, reed's persisted state and shuttle.

package creelcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// worktreeOps is the five calls the adapter makes on one worktree: reed's status and a shuttle runner's idle probe, send, command and interrupt.
type worktreeOps interface {
	// Status reports the worktree's strands and whether each is live.
	Status() (reedengine.StatusResult, error)
	// SessionIdle reports whether the strand with the guid can take typed input.
	SessionIdle(guid string) (shuttleengine.IdleProbe, error)
	// Send types text into the strand with the guid and confirms it landed.
	Send(guid, text string) error
	// RunCommand runs one command of creel's closed set in the session of the strand with the guid.
	RunCommand(guid string, command creelengine.Command) error
	// Interrupt sends the guarded interrupt to the strand with the guid.
	Interrupt(guid string) error
}

// hubDelivery is creelengine.Delivery over one hub: the worktrees git lists, each worktree's reed state and a shuttle runner built per call.
type hubDelivery struct {
	hubPath string
	// wire builds the worktreeOps of a resolved worktree; a test replaces it with a stub.
	wire func(location *lymcwd.Location) (worktreeOps, error)
	// skippedMu guards skipped, since the delivery workers list the strands concurrently.
	skippedMu sync.Mutex
	// skipped holds the worktree paths Strands has logged a skip for, until the path resolves again or the listing no longer holds it.
	skipped map[string]bool
}

// newHubDelivery returns the Delivery of the hub at hubPath, building each worktree's shuttle engines with enginesFor.
func newHubDelivery(hubPath string, enginesFor func(shuttleengine.Config, configengine.Dirs) (shuttleengine.EngineSet, error)) *hubDelivery {
	return &hubDelivery{
		hubPath: hubPath,
		wire: func(location *lymcwd.Location) (worktreeOps, error) {
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
func wireShuttle(location *lymcwd.Location, enginesFor func(shuttleengine.Config, configengine.Dirs) (shuttleengine.EngineSet, error)) (shuttleOps, error) {
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
	pauseGeom, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return shuttleOps{}, err
	}
	reed := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	runner := shuttleengine.NewRunner(reed, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	// Told the hub's record with no slug, so each send judges its target by the target's own strand name.
	runner.SetPause(pauserecord.Geometry{RecordPath: pauseGeom.RecordPath})
	return shuttleOps{reed: reed, runner: runner}, nil
}

func (o shuttleOps) Status() (reedengine.StatusResult, error) { return o.reed.Status() }

func (o shuttleOps) SessionIdle(guid string) (shuttleengine.IdleProbe, error) {
	return o.runner.SessionIdle(guid)
}

func (o shuttleOps) Send(guid, text string) error { return o.runner.Send(guid, text) }

func (o shuttleOps) RunCommand(guid string, command creelengine.Command) error {
	return runSessionCommand(o.runner, guid, command)
}

func (o shuttleOps) Interrupt(guid string) error { return o.runner.InterruptRestoring(guid) }

// sessionCommands is the session commands a shuttle runner offers, one per command of creel's closed set.
type sessionCommands interface {
	ClearSession(guid string) error
	CompactSession(guid, focus string) error
	ReloadPlugins(guid string) error
	TypeColor(guid string, color segmentcolor.Color) error
	LoadSkills(guid string, skills []string) error
}

// runSessionCommand runs command through ops: compact takes its argument as the focus, color as the segment color, load-skills as a space-separated skill list.
// A command outside the closed set is refused.
func runSessionCommand(ops sessionCommands, guid string, command creelengine.Command) error {
	switch command.Name {
	case creelengine.CommandClear:
		return ops.ClearSession(guid)
	case creelengine.CommandCompact:
		return ops.CompactSession(guid, command.Argument)
	case creelengine.CommandReloadPlugins:
		return ops.ReloadPlugins(guid)
	case creelengine.CommandColor:
		return ops.TypeColor(guid, segmentcolor.Color(command.Argument))
	case creelengine.CommandLoadSkills:
		return ops.LoadSkills(guid, strings.Fields(command.Argument))
	default:
		return fmt.Errorf("creel: unknown command %q", command.Name)
	}
}

// errWorktreeGone marks a worktree that no longer resolves as a worktree, so Strands skips it instead of failing the listing.
var errWorktreeGone = errors.New("creel: the worktree is gone")

// worktreeGoneMessage is the log line of a skipped worktree.
const worktreeGoneMessage = "creel: skipped a worktree that no longer resolves"

// Strands lists every strand reed persists in any worktree of the hub.
// A worktree that no longer resolves as a worktree is skipped, and its strands get no delivery until it resolves again:
// the resolve fails as not a git repository, or the reed state read, the wire or the reed status fails while the worktree's directory no longer exists.
// Any other resolve failure, and a state read, wire or status failure with the directory present, fail the call naming the worktree,
// so an unreadable strand is never mistaken for an absent one.
func (d *hubDelivery) Strands() ([]creelengine.Strand, error) {
	entries, err := fabricengine.List(d.primeWorktree())
	if err != nil {
		return nil, err
	}
	var strands []creelengine.Strand
	listed := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.Prunable {
			continue
		}
		listed[entry.Path] = true
		found, err := d.worktreeStrands(entry)
		if errors.Is(err, errWorktreeGone) {
			d.noteSkippedWorktree(entry.Path, err)
			continue
		}
		if err != nil {
			return nil, err
		}
		d.forgetSkippedWorktrees(func(path string) bool { return path == entry.Path })
		strands = append(strands, found...)
	}
	d.forgetSkippedWorktrees(func(path string) bool { return !listed[path] })
	return strands, nil
}

// noteSkippedWorktree logs the skip of the worktree at path.
// The first skip of a path logs at Info when worktreeTornDown reads a teardown and at Warn otherwise, so a cause that is no teardown shows;
// a repeated skip of the same path logs at Debug, since the delivery workers list the strands on every probe.
// The Warn line of a worktree whose .git is a file names the repair step, and that of one whose .git is a directory names that cause.
func (d *hubDelivery) noteSkippedWorktree(path string, cause error) {
	d.skippedMu.Lock()
	repeated := d.skipped[path]
	if d.skipped == nil {
		d.skipped = make(map[string]bool)
	}
	d.skipped[path] = true
	d.skippedMu.Unlock()

	switch {
	case repeated:
		logger.Debug(worktreeGoneMessage, "path", path, "cause", cause)
	case worktreeTornDown(path):
		logger.Info(worktreeGoneMessage, "path", path, "cause", cause)
	default:
		args := []any{"path", path, "cause", cause}
		if info, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
			if info.IsDir() {
				args = append(args, "reason", ".git is a directory, not a worktree link")
			} else {
				args = append(args, "way_forward", "git worktree repair "+path+" from the hub's prime rewrites the link when the worktree's admin directory stands; otherwise the worktree needs re-creating")
			}
		}
		logger.Warn(worktreeGoneMessage, args...)
	}
}

// forgetSkippedWorktrees drops every remembered skip whose path forgets selects, so a later skip of that path logs at its first level again.
func (d *hubDelivery) forgetSkippedWorktrees(forgets func(path string) bool) {
	d.skippedMu.Lock()
	defer d.skippedMu.Unlock()
	for path := range d.skipped {
		if forgets(path) {
			delete(d.skipped, path)
		}
	}
}

// worktreeTornDown reports a teardown's mark: the worktree directory at path is gone, or its .git is a file whose gitdir line names a directory that is gone.
func worktreeTornDown(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return true
	}
	gitPath := filepath.Join(path, ".git")
	if info, err := os.Stat(gitPath); err != nil || info.IsDir() {
		return false
	}
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(content), "\n") {
		if target, found := strings.CutPrefix(strings.TrimSpace(line), "gitdir:"); found {
			gitDir := filepath.FromSlash(strings.TrimSpace(target))
			if !filepath.IsAbs(gitDir) {
				gitDir = filepath.Join(path, gitDir)
			}
			_, err := os.Stat(gitDir)
			return errors.Is(err, fs.ErrNotExist)
		}
	}
	return false
}

// worktreeStepError wraps the failure of step on the worktree at path, naming the worktree.
// It wraps errWorktreeGone as well when the worktree's directory no longer exists, since a teardown can land after a successful resolve.
func worktreeStepError(path, step string, cause error) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s worktree %s: %w", errWorktreeGone, step, path, cause)
	}
	return fmt.Errorf("creel: %s worktree %s: %w", step, path, cause)
}

// primeWorktree is the hub's prime worktree directory, named for the repository the hub holds.
func (d *hubDelivery) primeWorktree() string {
	return filepath.Join(d.hubPath, strings.TrimSuffix(filepath.Base(d.hubPath), fabricengine.HubSuffix))
}

// worktreeStrands reads the strands persisted in one worktree and their liveness from its reed status.
func (d *hubDelivery) worktreeStrands(entry fabricengine.WorktreeEntry) ([]creelengine.Strand, error) {
	location, err := lymcwd.ResolveWorktree(entry.Path)
	if errors.Is(err, lymcwd.ErrNotAGitRepo) {
		return nil, fmt.Errorf("%w: resolve worktree %s: %w", errWorktreeGone, entry.Path, err)
	}
	if err != nil {
		return nil, fmt.Errorf("creel: resolve worktree %s: %w", entry.Path, err)
	}
	reedState, err := reedengine.LoadState(filepath.Join(location.AnchorPath(), lymdirs.DotLymDirName))
	if err != nil {
		return nil, worktreeStepError(entry.Path, "read the reed state of", err)
	}
	if reedState == nil || len(reedState.Strands) == 0 {
		return nil, nil
	}
	ops, err := d.wire(location)
	if err != nil {
		return nil, worktreeStepError(entry.Path, "wire", err)
	}
	status, err := ops.Status()
	switch {
	case errors.Is(err, reedengine.ErrNoSession):
		status = reedengine.StatusResult{}
	case err != nil:
		return nil, worktreeStepError(entry.Path, "read the reed status of", err)
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
	location, err := lymcwd.ResolveWorktree(strand.Worktree)
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
	case probe.NoInputBox:
		return creelengine.Idleness{Stall: creelengine.StallNoInputBox}, nil
	}
	return creelengine.Idleness{}, nil
}

// Paused reports whether a pause covers the strand.
// A prime strand is covered by a hub entry only, a pair strand by a hub entry or its own.
func (d *hubDelivery) Paused(strand creelengine.Strand) (bool, error) {
	location, err := lymcwd.ResolveWorktree(strand.Worktree)
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

// ClaimPause records the strand in the live pause entry that covers it, under the record lock.
// The prime's orch strand is recorded as the orch session, which its watcher finds owned, so the watcher sends no interrupt of its own and still posts the resume line;
// any other strand is recorded by name, guid and slug.
// The claim's StillLive re-reads the record and reports whether the owning entry is still live.
func (d *hubDelivery) ClaimPause(strand creelengine.Strand) (creelengine.PauseClaim, error) {
	location, err := lymcwd.ResolveWorktree(strand.Worktree)
	if err != nil {
		return creelengine.PauseClaim{}, fmt.Errorf("creel: resolve worktree %s: %w", strand.Worktree, err)
	}
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return creelengine.PauseClaim{}, err
	}
	session := pauserecord.Session{Strand: strand.Name, GUID: strand.GUID, Slug: strand.Slug}
	if parsed, parseErr := agentname.Parse(strand.Name); parseErr == nil && strand.Slug == "" && agentname.MatchesRole(parsed.Role, agentname.RoleOrch) {
		session = pauserecord.Session{GUID: strand.GUID, Orch: true}
	}
	entry, covered, err := pauserecord.ClaimSession(geometry.RecordPath, strand.Slug, session, time.Now())
	if err != nil {
		return creelengine.PauseClaim{}, fmt.Errorf("creel: claim %s into the pause record %s: %w", strand.Name, geometry.RecordPath, err)
	}
	if !covered {
		return creelengine.PauseClaim{}, nil
	}
	stillLive := func() (bool, error) {
		record, err := pauserecord.Read(geometry.RecordPath)
		if err != nil {
			return false, err
		}
		for _, live := range record.Live {
			if live.Scope == entry.Scope && live.PausedAt.Equal(entry.PausedAt) {
				return true, nil
			}
		}
		return false, nil
	}
	return creelengine.PauseClaim{Covered: true, StillLive: stillLive}, nil
}

// Send types one line into the strand's pane and confirms it landed.
func (d *hubDelivery) Send(strand creelengine.Strand, text string) error {
	ops, err := d.opsFor(strand)
	if err != nil {
		return err
	}
	return mapDeliveryError(ops.Send(strand.GUID, text))
}

// Command runs one command of creel's closed set in the strand's session and confirms it landed.
func (d *hubDelivery) Command(strand creelengine.Strand, command creelengine.Command) error {
	ops, err := d.opsFor(strand)
	if err != nil {
		return err
	}
	return mapDeliveryError(ops.RunCommand(strand.GUID, command))
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
// A send refused because a pause covers the session maps onto creelengine.ErrBusy, so the delivery worker requeues the inject and types it after the resume.
// A send that failed after the paste, its text found in the operator's draft or foreign text in the input box, maps onto creelengine.ErrNotLanded instead:
// ErrBusy means nothing was typed, and requeueing such an inject would type it again.
func mapDeliveryError(err error) error {
	switch {
	case errors.Is(err, shuttleengine.ErrSentTextInDraft), errors.Is(err, shuttleengine.ErrForeignBoxText):
		return creelengine.ErrNotLanded
	case errors.Is(err, shuttleengine.ErrSessionBusy), errors.Is(err, shuttleengine.ErrPaused):
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
