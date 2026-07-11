// spawn.go implements SpawnBatch, the `spawn-batch` verb's engine core: the
// pause gate, the batch's own oversized-driven role selection (the
// orchestrator overriding only for recovery), the optional --restart-chain
// reset, the start-SHA and chain-anchor capture, the stencil-filled
// implementer prompt, the shuttleengine.Spec construction (the modelspec ->
// shuttle mapping the discussion pins), the non-blocking spawn itself, and
// the cross-process run-identity resolution (FindRun) that lets a caller
// record durable state without ever holding an in-process shuttle Run
// handle. SpawnBatch never touches weft: recording SpawnResult's fields is
// enough for the caller (internal/buildercli) to perform the weft commit at
// the batch boundary — see doc.go's weft-ownership section and SpawnResult's
// own doc comment for the exact commit-boundary sequence.

package builderengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// ErrPaused is the sentinel SpawnBatch returns when builderDir's pause flag
// is present at the batch boundary (PauseRequested). Exported so a caller
// can distinguish the operational "paused" refusal from every other spawn
// failure via errors.Is(err, ErrPaused), per the discussion's pause
// decision: the orchestrator reads this refusal and writes its own outcome
// file with outcome: paused, rather than treating it as a hard error.
var ErrPaused = errors.New("builder: paused")

// Starter is the seam SpawnBatch spawns an implementer through: exactly
// (*shuttleengine.Runner).Start's signature, so production code passes a
// real *shuttleengine.Runner directly and tests pass one built over local
// fake shuttleengine.MuxOps/shuttleengine.Engine doubles (the shuttleengine
// fakes_test.go pattern; builderengine's own fakes are test-file-local, per
// the discussion's test-conventions decision). Start is deliberately
// non-blocking — spawn-batch returns as soon as the implementer's strand is
// registered, never waiting for it to finish; the tool-call cap is `poll`'s
// problem, not spawn-batch's.
type Starter interface {
	Start(shuttleengine.Spec) (*shuttleengine.Run, error)
}

// SpawnDeps carries every seam SpawnBatch needs, so a test can fake each one
// independently: Starter spawns the implementer; Plan and State are the
// already-parsed/loaded plan and run state SpawnBatch reads and mutates;
// Roles is the pre-flight-resolved role->model-spec map (see ResolveRoles);
// Config is the loaded builder.yaml; WorktreeRoot is the host repo checkout
// SpawnBatch captures HeadSHA from; BuilderDir and ReportsDir are the
// hubgeometry-resolved _lyx/builder and _lyx/builder/reports directories;
// ShuttleCfg and Layout are what shuttleengine.FindRun needs to resolve the
// just-started run's cross-process identity (a *shuttleengine.Run exposes
// only StrandGUID() — there is no run-dir accessor on the handle itself).
type SpawnDeps struct {
	Starter      Starter
	Plan         *Plan
	State        *State
	Roles        map[Role]modelspec.Resolved
	Config       Config
	WorktreeRoot string
	BuilderDir   string
	ReportsDir   string
	ShuttleCfg   shuttleengine.Config
	Layout       *hubgeometry.Layout
}

// SpawnBatchOptions carries one `spawn-batch` invocation's caller-supplied
// choices: BatchNumber names the plan batch to spawn; RoleOverride, when
// non-empty, MUST be RoleRecovery (any other value is an error) — Go picks
// the role from the batch's own oversized: flag, the orchestrator overrides
// only for the recovery escalation path, per the discussion's role-selection
// decision; RestartChain requests the --restart-chain reset (see RestartChain)
// before this spawn proceeds.
type SpawnBatchOptions struct {
	BatchNumber  int
	RoleOverride Role
	RestartChain bool
}

// SpawnResult is what one successful SpawnBatch call hands back to its
// caller (internal/buildercli's spawn-batch verb): exactly what that caller
// needs to weft-commit state.json at the batch boundary without re-deriving
// any of it from deps.State itself. Per the discussion's weft-commit
// decision, SpawnBatch's own caller performs that commit — builderengine
// stays weft-blind — immediately after SpawnBatch returns successfully,
// mirroring the commit boundary poll's own terminal classification and
// run's own exit-time backstop use elsewhere in the loop.
type SpawnResult struct {
	// BatchName is the batch's "NN-<batch-slug>" identifier.
	BatchName string
	// Role is the shuttle role the implementer spawned under.
	Role Role
	// StartSHA is the host HEAD captured immediately before this spawn —
	// the same value now recorded as this batch's BatchState.StartSHA.
	StartSHA string
	// StrandGUID identifies the mux strand the implementer spawned into.
	StrandGUID string
	// RunDir is the shuttle run directory FindRun resolved for this spawn.
	RunDir string
	// ReportPath is the absolute path this batch's implementer is expected
	// to write its batch-report YAML to (the spec's sole OutputFiles entry).
	ReportPath string
}

// batchReportFileName returns the batch-report filename plan-format.md pins
// for batch: "NN-<batch-slug>.yaml", matching chain.go's RestartChain and
// validate.go's batchID naming.
func batchReportFileName(b PlanBatch) string {
	return fmt.Sprintf("%02d-%s.yaml", b.Number, b.Slug)
}

// findBatch returns the PlanBatch in plan whose Number matches number, or
// an error naming the missing number — SpawnBatch's first lookup, since
// every later step needs the batch's own fields (Slug, Oversized, File).
func findBatch(plan *Plan, number int) (PlanBatch, error) {
	for _, b := range plan.Batches {
		if b.Number == number {
			return b, nil
		}
	}
	return PlanBatch{}, fmt.Errorf("builder: batch %d not found in plan", number)
}

// selectRole picks the implementer role a batch spawns under: oversized
// picks RoleImplementerOversized over the RoleImplementer default, and
// override — when it is RoleRecovery — always wins regardless of oversized,
// per the discussion's "Go picks from the batch, the orchestrator overrides
// only for recovery" decision. override values other than "" or RoleRecovery
// are rejected loud: any other override is a caller error, never silently
// honored or silently ignored.
func selectRole(oversized bool, override Role) (Role, error) {
	switch override {
	case "", RoleRecovery:
	default:
		return "", fmt.Errorf("builder: invalid role override %q; only %q is a valid override (Go selects implementer/implementer_oversized from the batch itself)", override, RoleRecovery)
	}

	role := RoleImplementer
	if oversized {
		role = RoleImplementerOversized
	}
	if override == RoleRecovery {
		role = RoleRecovery
	}
	return role, nil
}

// SpawnBatch drives one `spawn-batch <NN>` invocation to completion: the
// pause gate, role selection, the optional --restart-chain reset, start-SHA
// and chain-anchor capture, the stencil-filled implementer prompt, the
// shuttleengine.Spec the modelspec mapping pins, the non-blocking spawn
// itself, and the cross-process FindRun resolution that lets it record
// durable BatchState without ever holding an in-process shuttle Run handle
// (spawn-batch exits right after Start; poll re-derives everything else
// later). On success it persists deps.State via SaveState and returns a
// SpawnResult; on any failure deps.State is left exactly as SpawnBatch found
// it except where a step's own doc says otherwise (RestartChain mutates
// deps.State in place before SpawnBatch's own SaveState call, per its own
// contract).
func SpawnBatch(deps SpawnDeps, opts SpawnBatchOptions) (*SpawnResult, error) {
	if PauseRequested(deps.BuilderDir) {
		return nil, ErrPaused
	}

	batch, err := findBatch(deps.Plan, opts.BatchNumber)
	if err != nil {
		return nil, err
	}

	role, err := selectRole(batch.Oversized, opts.RoleOverride)
	if err != nil {
		return nil, err
	}

	resolved, ok := deps.Roles[role]
	if !ok {
		return nil, fmt.Errorf("builder: no resolved model-spec for role %q", role)
	}

	batchName := fmt.Sprintf("%02d-%s", batch.Number, batch.Slug)
	reportPath, err := filepath.Abs(filepath.Join(deps.ReportsDir, batchReportFileName(batch)))
	if err != nil {
		return nil, fmt.Errorf("builder: resolve report path: %w", err)
	}

	// A pre-existing report file is refused here, as builder's own named
	// error, BEFORE any spawn — shuttle's own Spec.validate would reject a
	// pre-existing OutputFiles entry too, but surfacing it under builder's
	// own wording first keeps the caller from ever reaching a shuttle-side
	// message about a file it never named itself.
	if _, statErr := os.Stat(reportPath); statErr == nil {
		return nil, fmt.Errorf("builder: batch report already exists: %s; remove it (or let --restart-chain clear it) before spawning again", reportPath)
	}

	chainEnd := ChainEndFor(deps.Plan, batch.Number)

	if opts.RestartChain {
		if chainEnd == 0 {
			return nil, fmt.Errorf("builder: batch %d is chainless; --restart-chain requires a deferred-verify chain member", batch.Number)
		}
		if err := RestartChain(deps.WorktreeRoot, deps.State, deps.Plan, chainEnd, deps.ReportsDir); err != nil {
			return nil, err
		}
	}

	head, err := HeadSHA(deps.WorktreeRoot)
	if err != nil {
		return nil, err
	}

	if chainEnd != 0 {
		if deps.State.ChainStartSHAs == nil {
			deps.State.ChainStartSHAs = map[int]string{}
		}
		if _, recorded := deps.State.ChainStartSHAs[chainEnd]; !recorded {
			// The anchor is this HEAD: the host commit immediately before
			// the lowest-numbered chain member's first card commit, per the
			// discussion's chain-anchor decision. Recorded once, at whichever
			// member spawns first — never overwritten by a later member's
			// own spawn.
			deps.State.ChainStartSHAs[chainEnd] = head
		}
	}

	batchFilePath, err := filepath.Abs(filepath.Join(deps.Plan.Dir, batch.File))
	if err != nil {
		return nil, fmt.Errorf("builder: resolve batch file path: %w", err)
	}

	values := map[string]string{
		"batch_file":    batchFilePath,
		"batch_name":    batchName,
		"report_path":   reportPath,
		"self_fix_cap":  fmt.Sprintf("%d", deps.Config.SelfFixCap),
		"worktree_root": deps.WorktreeRoot,
	}
	prompt, err := stencil.Fill(ImplementerTemplate(), values)
	if err != nil {
		return nil, fmt.Errorf("builder: fill implementer template: %w", err)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{reportPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Role:        string(role),
		Round:       batchName,
		Timeout:     time.Duration(deps.Config.BatchTimeoutMin) * time.Minute,
		KeepPane:    true,
	}

	run, err := deps.Starter.Start(spec)
	if err != nil {
		return nil, fmt.Errorf("builder: start batch %s implementer: %w", batchName, err)
	}

	runState, runDir, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.Layout, run.StrandGUID())
	if err != nil {
		return nil, fmt.Errorf("builder: resolve spawned run: %w", err)
	}

	if deps.State.Batches == nil {
		deps.State.Batches = map[int]*BatchState{}
	}
	deps.State.Batches[batch.Number] = &BatchState{
		Slug:          batch.Slug,
		StartSHA:      head,
		Role:          string(role),
		StrandGUID:    run.StrandGUID(),
		ShuttleRunDir: runDir,
		EventsPath:    runState.EventsPath,
		SpawnedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	deps.State.CurrentBatch = batch.Number

	if err := SaveState(deps.BuilderDir, deps.State); err != nil {
		return nil, err
	}

	return &SpawnResult{
		BatchName:  batchName,
		Role:       role,
		StartSHA:   head,
		StrandGUID: run.StrandGUID(),
		RunDir:     runDir,
		ReportPath: reportPath,
	}, nil
}
