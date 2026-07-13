// spawn.go implements SpawnBatch, the `spawn-batch` verb's engine core: the
// pause gate, the batch's own oversized-driven role selection (the
// orchestrator overriding only for recovery), the optional --restart-chain
// reset, the start-SHA and chain-anchor capture, the stencil-filled
// implementer prompt, the shuttleengine.Spec construction (the modelspec ->
// shuttle mapping the discussion pins), the non-blocking spawn itself, and
// the cross-process run-identity resolution (FindRun) that lets a caller
// record durable state without ever holding an in-process shuttle Run
// handle.
//
// SpawnBatch itself never touches weft (see doc.go's package-level weft
// section): it only mutates and SaveState's the CALLER-owned deps.State on
// the plain host filesystem. The discussion pins three distinct weft-commit
// points across the whole builder loop, and this is the first of them:
// internal/buildercli's spawn-batch verb weft-commits state.json
// immediately after a successful SpawnBatch call (the just-recorded
// start-SHA and BatchState entry); the poll verb weft-commits the batch
// report + state.json once a batch reaches a terminal classification; and
// the run verb performs one backstop weft-commit at its own exit. Every one
// of those three commits belongs to buildercli, never to this function or
// this package — the perchcli precedent (block-exit weft Commit+Push,
// engine stays weft-blind) applied to builder's own batch boundary.

package builderengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// ErrBatchInFlight is the sentinel SpawnBatch returns when state records a
// non-terminal in-flight batch whose implementer strand the mux still
// reports live. The batch loop is strictly sequential, so spawning anything
// while a live implementer is mid-flight is always wrong — it silently
// clobbers the in-flight batch's BatchState and races two agents on the
// same host repo. The refusal never fires on the intended
// respawn-on-top-of-a-kept-pane ladder (dead respawn, recovery after
// stuck): every one of those passes through a terminal poll first, which
// sets BatchState.Terminal and clears CurrentBatch. A caller resolves the
// refusal by long-polling (`lyx builder poll`) until the in-flight batch
// classifies terminal.
var ErrBatchInFlight = errors.New("builder: a batch is already in flight")

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
	// Mux is the live mux query surface the in-flight guard (ErrBatchInFlight)
	// and the dead-respawn orphan cleanup consult via StrandLive/RemoveStrand —
	// the same handle buildercli's poll verb already holds for its own
	// classification gathers.
	Mux shuttleengine.MuxOps
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

// BatchReportFileName returns the batch-report filename plan-format.md pins
// for a batch numbered number with slug slug: "NN-<slug>.yaml", matching
// validate.go's batchID naming (batchID omits the extension; this adds it).
// Exported and reused by every builderengine call site that names a batch
// report file (this file, chain.go's RestartChain, runlevel.go's
// renderProgress) plus buildercli's status/poll verbs, so the filename
// convention lives in exactly one place.
func BatchReportFileName(number int, slug string) string {
	return fmt.Sprintf("%02d-%s.yaml", number, slug)
}

// archiveStaleReport renames reportsDir's NN-<slug>.yaml, if present, to
// NN-<slug>-<UTC-compact-timestamp>.yaml in place, mirroring
// ArchiveStaleOutcome and Run's --fresh state/reports archiving: it is the
// recovery path's archive-never-refuse escape. A --role recovery respawn
// re-uses the batch's own report path as its sole shuttle OutputFiles entry,
// which both SpawnBatch's pre-existing-report guard AND shuttle's own
// Spec.validate refuse when a file is already there; the prior stuck report is
// still on disk (poll weft-committed it when it classified the batch stuck), so
// without this the orchestrator's documented stuck -> --role recovery
// escalation could never spawn. Archiving frees the path while keeping the
// stuck report auditable rather than deleting it. now is a seam so tests can
// pin the timestamp; production callers pass time.Now. Absent file: ("", nil),
// a no-op — a recovery spawn is legitimate even when no prior report exists.
// Reuses runlevel.go's firstFreeArchivePath so the same-second "-1"/"-2"
// collision rule lives in exactly one place.
func archiveStaleReport(reportsDir string, number int, slug string, now func() time.Time) (string, error) {
	path := filepath.Join(reportsDir, BatchReportFileName(number, slug))
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("builder: stat batch report %s: %w", path, err)
	}

	// Split the ".yaml" extension off so the timestamp lands on the stem
	// (NN-slug-<stamp>.yaml), matching the state file's archive shape
	// (state-<stamp>.json) rather than appending after the extension.
	const ext = ".yaml"
	base := strings.TrimSuffix(BatchReportFileName(number, slug), ext)
	stamp := now().UTC().Format(archiveTimestampFormat)
	target, err := firstFreeArchivePath(func(suffix string) string {
		return filepath.Join(reportsDir, fmt.Sprintf("%s-%s%s%s", base, stamp, suffix, ext))
	})
	if err != nil {
		return "", fmt.Errorf("builder: find archive target for batch report %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("builder: archive stale batch report %s: %w", path, err)
	}
	return target, nil
}

// removeStrandIfLive removes guid's mux strand when the mux still reports
// it live, and is a no-op otherwise. It exists for the respawn paths that
// re-claim a dead-classified batch's deliberately-kept pane: a timed-out
// implementer may still be WORKING, not hung, and left alive it races the
// fresh session (late commits to the host repo, a late report landing on
// the very path the new spawn names as its output file). A StrandLive error
// is treated as not-live — a downed mux session hosts no live strand. A
// failed removal of a genuinely live strand propagates: spawning while the
// orphan cannot be stopped is exactly the double-drive this helper exists
// to prevent.
func removeStrandIfLive(mux shuttleengine.MuxOps, guid string) error {
	live, err := StrandLive(mux, guid)
	if err != nil || !live {
		return nil
	}
	if _, err := mux.RemoveStrand(guid, false); err != nil {
		return fmt.Errorf("builder: remove kept strand %s before respawn: %w", guid, err)
	}
	return nil
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
// later). On success it persists deps.State via SaveState — to the plain
// host filesystem only, never through weft — and returns a SpawnResult; on
// any failure deps.State is left exactly as SpawnBatch found it except
// where a step's own doc says otherwise (RestartChain mutates deps.State in
// place before SpawnBatch's own SaveState call, per its own contract).
//
// Weft commit boundary: SpawnBatch performs NO weft commit itself. Its
// caller (internal/buildercli's spawn-batch verb) is responsible for
// weft-committing the state.json SpawnBatch just wrote — using SpawnResult's
// fields plus deps.BuilderDir, with no need to re-derive anything — as soon
// as SpawnBatch returns successfully. That is the first of the loop's three
// weft-commit points (see this file's package doc above for the other two:
// poll at terminal classification, run as an exit-time backstop).
func SpawnBatch(deps SpawnDeps, opts SpawnBatchOptions) (*SpawnResult, error) {
	if PauseRequested(deps.BuilderDir) {
		return nil, ErrPaused
	}

	// Recompute the plan fingerprint and compare it against the one recorded
	// at run init — the same crash/resume guard Run applies at its own entry
	// (state.go's documented contract names BOTH entry points). A plan edited
	// mid-run must fail loud here, before any spawn: state.json keys batches
	// by number and records per-batch start-SHAs, so driving a mutated plan
	// against the old state silently corrupts the run's semantics (a
	// renumbered batch is the worst case). There is no --fresh escape on this
	// path — re-initializing is run's job, so the refusal points there.
	fingerprint, err := Fingerprint(deps.Plan.Dir)
	if err != nil {
		return nil, err
	}
	if deps.State.PlanFingerprint != fingerprint {
		return nil, fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; the plan changed since state.json was created — re-run `lyx builder run --fresh` to archive the stale state and reports and start over", ErrFingerprintMismatch, fingerprint, deps.State.PlanFingerprint)
	}

	// The in-flight guard: the loop is strictly sequential, so a recorded
	// non-terminal batch whose strand the mux still reports live means an
	// implementer is mid-flight RIGHT NOW — spawning anything on top of it
	// double-drives the host repo and clobbers its BatchState (an orphaned
	// live implementer after an orchestrator crash, or a stray manual
	// spawn-batch during a run). The intended respawn ladders never trip
	// this: they always pass through a terminal poll first (Terminal set,
	// CurrentBatch cleared). A Status error is deliberately non-fatal — a
	// downed mux session cannot host a live strand, and if the substrate is
	// genuinely unavailable the spawn's own Start surfaces that error.
	if cur := deps.State.CurrentBatch; cur != 0 {
		if inFlight := deps.State.Batches[cur]; inFlight != nil && !inFlight.Terminal {
			if live, liveErr := StrandLive(deps.Mux, inFlight.StrandGUID); liveErr == nil && live {
				return nil, fmt.Errorf("%w: batch %02d-%s's implementer strand %s is still live; long-poll it with `lyx builder poll` until it classifies terminal before spawning another batch", ErrBatchInFlight, cur, inFlight.Slug, inFlight.StrandGUID)
			}
		}
	}

	batch, err := findBatch(deps.Plan, opts.BatchNumber)
	if err != nil {
		return nil, err
	}

	// --restart-chain rolls the host repo back to the chain's recorded start
	// SHA and re-runs the WHOLE chain from the bottom, so the batch that must
	// spawn next is always the chain's lowest-numbered member — never
	// necessarily the member the caller named. The chain-END batch runs the
	// chain's real verify:, so it is the member most likely to go stuck and
	// thus the most likely --restart-chain target; spawning it directly on the
	// rolled-back tree would skip every earlier member's just-discarded work
	// (found live in round opus-r3: `spawn-batch <chain-end> --restart-chain`
	// reset to the anchor but then spawned the chain-end, silently dropping the
	// lower members). Re-point the spawn to the lowest member HERE, before role,
	// report-path, and chain-anchor resolution all key off batch, so Go owns
	// "restart from the bottom" rather than trusting the caller to name it —
	// matching builder.md's chain-rollback contract ("the chain always restarts
	// from its lowest member"). The chainless refusal moves here too, so it
	// fires before any later work rather than mid-way through the reset block.
	if opts.RestartChain {
		chainEnd := ChainEndFor(deps.Plan, batch.Number)
		if chainEnd == 0 {
			return nil, fmt.Errorf("builder: batch %d is chainless; --restart-chain requires a deferred-verify chain member", batch.Number)
		}
		// ChainMembers is sorted ascending and non-empty whenever chainEnd is
		// non-zero (the chain always contains at least its declaring member and
		// its end), so [0] is the lowest member.
		if lowest := ChainMembers(deps.Plan, chainEnd)[0]; lowest != batch.Number {
			batch, err = findBatch(deps.Plan, lowest)
			if err != nil {
				return nil, err
			}
		}
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
	reportPath, err := filepath.Abs(filepath.Join(deps.ReportsDir, BatchReportFileName(batch.Number, batch.Slug)))
	if err != nil {
		return nil, fmt.Errorf("builder: resolve report path: %w", err)
	}

	chainEnd := ChainEndFor(deps.Plan, batch.Number)

	// --restart-chain's reset (which deletes every chain member's stale
	// report, including this batch's own reportPath when it is itself a
	// chain member) runs BEFORE the pre-existing-report check below, so the
	// very report that motivated --restart-chain never trips that check.
	// Reordering these two any other way makes --restart-chain unreachable
	// on the exact invocation ("re-spawn the batch whose stale report is
	// still on disk") it exists to recover.
	if opts.RestartChain {
		// chainEnd is guaranteed non-zero here: the early re-point block above
		// already refused a chainless --restart-chain, and batch is now a
		// resolved chain member, so ChainEndFor above returned the chain's end.
		// A chain member's dead-classified implementer may still be live in
		// its kept pane; the hard reset below yanks the repo out from under
		// it, so any late commit it makes lands on top of the rolled-back
		// tree. Stop every member's recorded strand before the reset.
		for _, member := range ChainMembers(deps.Plan, chainEnd) {
			if memberState := deps.State.Batches[member]; memberState != nil {
				if err := removeStrandIfLive(deps.Mux, memberState.StrandGUID); err != nil {
					return nil, err
				}
			}
		}
		if err := RestartChain(deps.WorktreeRoot, deps.State, deps.Plan, chainEnd, deps.ReportsDir); err != nil {
			return nil, err
		}
		// The reset just hard-reset the host repo and deleted member reports
		// on disk; persist the matching state NOW rather than only at the
		// spawn's own SaveState below — if any later step fails (role spawn,
		// FindRun), a state.json still recording the rolled-back members'
		// BatchStates would disagree with the repo it describes.
		if err := SaveState(deps.BuilderDir, deps.State); err != nil {
			return nil, err
		}
	}

	// A respawn of a dead-classified batch (the orchestrator's documented
	// "respawn the SAME batch fresh" ladder) re-claims the kept substrate:
	// the pane was deliberately left alive at classification for diagnosis,
	// but a timed-out implementer may still be WORKING — its late report
	// would refuse this very spawn (found live in round fable-r2: the report
	// landed a minute after the dead/timeout classification), and left alive
	// it races the fresh session on the host repo and the report path.
	priorState := deps.State.Batches[batch.Number]
	respawnOfDead := priorState != nil && priorState.Terminal && priorState.Status == DigestStatusDead
	if respawnOfDead {
		if err := removeStrandIfLive(deps.Mux, priorState.StrandGUID); err != nil {
			return nil, err
		}
	}

	// A --role recovery respawn re-uses the stuck batch's own report path as
	// its sole shuttle OutputFiles entry, but that stuck report is still on
	// disk (poll weft-committed it), so both the pre-existing-report check
	// below and shuttle's own Spec.validate would refuse the spawn. Archive it
	// here — archive-never-refuse, like ArchiveStaleOutcome — so the
	// orchestrator's documented stuck -> --role recovery escalation actually
	// spawns. This runs BEFORE the pre-existing-report check for the same
	// reason --restart-chain's clear does: otherwise the very report the
	// recovery is retrying past would trip that check first. --restart-chain
	// (which deletes chain members' reports above) already cleared this batch's
	// report when both flags are set, so archiveStaleReport then finds nothing.
	// The dead-respawn case archives too: a kept-alive orphan that finished
	// AFTER its dead classification leaves a late report on the live path,
	// and refusing on it would wedge the respawn ladder on a report nobody
	// distilled. A done batch's report is deliberately NOT archived — an
	// accidental respawn of finished work must stay a loud refusal below.
	if role == RoleRecovery || respawnOfDead {
		if _, err := archiveStaleReport(deps.ReportsDir, batch.Number, batch.Slug, time.Now); err != nil {
			return nil, err
		}
	}

	// A pre-existing report file is refused here, as builder's own named
	// error, BEFORE any spawn — shuttle's own Spec.validate would reject a
	// pre-existing OutputFiles entry too, but surfacing it under builder's
	// own wording first keeps the caller from ever reaching a shuttle-side
	// message about a file it never named itself.
	if _, statErr := os.Stat(reportPath); statErr == nil {
		return nil, fmt.Errorf("builder: batch report already exists: %s; remove it (or let --restart-chain clear it) before spawning again", reportPath)
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
