// spawn.go declares the seams a fixer's spawn and wait go through.
// It also holds SpawnOrAttach, which gives a caller its one fixer strand per failed evaluation.

package gatefixer

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Starter is the seam a fixer strand is spawned through.
// Start returns once the spawned provider is past its startup gates, and never waits for the run to finish.
type Starter interface {
	Start(shuttleengine.Spec) (*shuttleengine.Run, error)
}

// Stopper is the seam a fixer strand is stopped through by guid.
// StopStrand records the stop on the strand's run, then ends the strand when it is live.
// It is a no-op for a strand that is not live.
type Stopper interface {
	StopStrand(guid string) error
}

// Engines is the engine lookup a fixer's recorded engine name is resolved through.
// An empty name resolves the default engine, and DefaultName names that engine.
type Engines interface {
	Lookup(name string) (shuttleengine.Engine, error)
	DefaultName() string
}

// Clock is the time source the wait polls on, so a test advances it without waiting.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// Deps carries the seams a fixer's spawn and wait need.
type Deps struct {
	Starter Starter
	Stopper Stopper
	// Reed answers the liveness read of the wait.
	Reed    shuttleengine.ReedOps
	Engines Engines
	// ShuttleCfg and AnchorRoot locate a spawned strand's shuttle run.
	ShuttleCfg shuttleengine.Config
	AnchorRoot string
	Clock      Clock
	// Register registers a spawned strand with runproc by guid; nil leaves the strand unregistered.
	Register func(guid string) error
}

// Spec is one fixer strand's launch.
type Spec struct {
	// Prompt is the rendered prompt the strand starts with.
	Prompt []byte
	// ReportPath is the report the fixer reads, kept on the record.
	ReportPath string
	// ReplyPath is the reply file the fixer writes as its final action, and the strand's only output file.
	ReplyPath string
	// Role is the strand's reed role, one of Roles.
	Role        string
	ModelChoice shuttleengine.ModelChoice
	// Skills are the skills the strand loads after the gate-fixer role's agents.yaml list, such as the hub's language skills.
	Skills  []string
	Segment segmentcolor.Segment
	// Timeout bounds the strand's own run.
	Timeout time.Duration
}

// SpawnOrAttach returns the fixer strand a caller waits on, and whether this call spawned it.
// A live record in store is attached to as it stands, and nothing is spawned.
// Otherwise a stale reply file is removed, and a strand is started with the reply file as its only output file.
// The strand is registered through deps.Register when set, and its record is saved in store before it is returned.
// A start failure is returned wrapped as transient.
// A run that cannot be resolved, a registration that fails or a record that cannot be saved stops the strand.
// The call then returns the joined error, wrapped as transient, so no unrecorded or unregistered strand outlives it.
// It refuses a spec.Role outside Roles, a wiring error.
func SpawnOrAttach(deps Deps, store Store, spec Spec) (*Record, bool, error) {
	if !IsFixerRole(spec.Role) {
		return nil, false, fmt.Errorf("gatefixer: %q is not a fixer role; way forward: spawn under %s or %s", spec.Role, RoleGateFixer, RoleVerifyFixer)
	}
	live, err := store.Live()
	if err != nil {
		return nil, false, fmt.Errorf("gatefixer: read the live %s record: %w", spec.Role, err)
	}
	if live != nil {
		return live, false, nil
	}
	if err := os.Remove(spec.ReplyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("gatefixer: remove the stale %s reply %s: %w", spec.Role, spec.ReplyPath, err)
	}

	run, err := deps.Starter.Start(shuttleengine.Spec{
		Prompt:      string(spec.Prompt),
		OutputFiles: []string{spec.ReplyPath},
		ModelChoice: spec.ModelChoice,
		ConfigRole:  agentconfig.RoleGateFixer,
		Skills:      spec.Skills,
		Role:        spec.Role,
		Segment:     spec.Segment,
		Timeout:     spec.Timeout,
	})
	if err != nil {
		return nil, false, markTransient(fmt.Errorf("gatefixer: start the %s: %w", spec.Role, err))
	}
	guid := run.StrandGUID()

	runState, runDir, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.AnchorRoot, guid)
	if err != nil {
		return nil, false, abandon(deps, guid, fmt.Errorf("gatefixer: resolve the spawned %s run: %w", spec.Role, err))
	}
	if deps.Register != nil {
		if err := deps.Register(guid); err != nil {
			return nil, false, abandon(deps, guid, fmt.Errorf("gatefixer: register the spawned %s: %w", spec.Role, err))
		}
	}
	rec := &Record{
		StrandGUID:    guid,
		SessionID:     runState.SessionID,
		Engine:        runState.Engine,
		SpawnedAt:     deps.Clock.Now().UTC().Format(time.RFC3339),
		ShuttleRunDir: runDir,
		EventsPath:    runState.EventsPath,
		EventsOffset:  runState.PromptOffset,
		ReportPath:    spec.ReportPath,
		ReplyPath:     spec.ReplyPath,
	}
	if err := store.Save(rec); err != nil {
		return nil, false, abandon(deps, guid, fmt.Errorf("gatefixer: save the spawned %s record: %w", spec.Role, err))
	}
	return rec, true, nil
}

// abandon stops the strand a failed spawn left live.
// It returns cause joined with the stop's own error, wrapped as transient.
func abandon(deps Deps, guid string, cause error) error {
	return markTransient(errors.Join(cause, deps.Stopper.StopStrand(guid)))
}

// markTransient marks err as an agent start failure a single re-run may clear.
func markTransient(err error) error {
	return shedengine.MarkTransient(shedengine.TransientAgentStart, err)
}
