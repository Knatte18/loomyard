// planfixer.go implements the cold plan fixer `lym webster fix-plan` drives after begin-batch refuses with a plan conflict:
// the bounded spawn-or-attach of a strand at the master role with the plan-fix prompt, the wait that reads its verdict, and its terminal record.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fixPlanCommand is the verb that runs the plan fixer, as Master runs it.
const fixPlanCommand = "lym webster fix-plan"

// planFixerStrandRole is the reed strand role the plan fixer runs under.
const planFixerStrandRole = "plan-fixer"

// planFixReplyFileName is the plan fixer's reply file name inside the webster scratch dir.
const planFixReplyFileName = "plan-fix-reply.md"

// planFixersPerBatch is the most plan fixers fix-plan spawns for one batch's conflicts in a run.
const planFixersPerBatch = 2

// The plan fixer's verdicts, the values its reply's first line names after "verdict: ".
const (
	// PlanFixVerdictFixed is a fix that edited and rebaselined the named cards without changing what any card does.
	PlanFixVerdictFixed = "fixed"
	// PlanFixVerdictPlanWriter is a conflict only the plan writer can answer, with the plan left as the run recorded it.
	PlanFixVerdictPlanWriter = "plan_writer"
)

// planFixRefusalWayForward is the way forward of every fix-plan refusal and of a fixer that ends dead: the conflict goes on to the plan writer.
const planFixRefusalWayForward = "way forward: write outcome: stuck to your outcome file, with a stuck_reason quoting begin-batch's plan_conflict refusal verbatim, so the conflict goes to the plan writer"

// PlanFixReplyPath returns the path of the reply file the plan fixer writes as its final action, inside scratchDir.
func PlanFixReplyPath(scratchDir string) string {
	return filepath.Join(scratchDir, planFixReplyFileName)
}

// FixPlanDeps carries the seams the plan fixer's spawn and wait need.
// Geom is the told Geometry every path is read from;
// Reed answers the liveness read, and Stopper is the seam every stop goes through.
type FixPlanDeps struct {
	Starter    Starter
	Roles      map[Role]shuttleengine.ModelChoice
	Config     Config
	Engines    AuditEngines
	Reed       shuttleengine.ReedOps
	Stopper    StrandStopper
	ShuttleCfg shuttleengine.Config
	Geom       Geometry

	// FrictionDir is the told absolute friction directory, empty when Tier 2 is off.
	FrictionDir string
}

// FixPlanResult is what one FixPlanAwait call hands back.
type FixPlanResult struct {
	// Status is DigestStatusRunning when the wait elapsed first, else DigestStatusDone or DigestStatusDead.
	Status string
	// DeadReason is set only when Status is DigestStatusDead.
	DeadReason string
	// Verdict is set only when Status is DigestStatusDone: PlanFixVerdictFixed or PlanFixVerdictPlanWriter.
	Verdict string
	// Reply is the fixer's reply file content, set only when Status is DigestStatusDone.
	Reply string
	// ElapsedS is the seconds since the spawn.
	ElapsedS int
	// Warnings are cleanup failures and the reason a fixed verdict was overruled, none of which changes the status.
	Warnings []string
}

// FixPlanSpawnOrAttach returns the plan fixer fix-plan waits on, and whether this call spawned it into st.
// A last record of st.PlanFixers that is not terminal is attached to as it stands.
// Otherwise it spawns a strand at the master role with the plan-fix prompt, whose shuttle output file is the reply file, and appends its record to st.PlanFixers, holding the conflict's batch and findings and the worktree's HEAD and uncommitted paths;
// the caller saves st.
// It refuses, spawning nothing, when no plan-conflict record names a finding, when an earlier plan fixer of the run was given the same findings, when the record's batch already had planFixersPerBatch fixers, and when the master role's engine cannot audit forks.
// A stale reply file is removed before the spawn.
// A spawned strand whose run cannot be resolved is stopped, since no record of it will be saved.
func FixPlanSpawnOrAttach(deps FixPlanDeps, st *State, clk Clock) (*PlanFixerState, bool, error) {
	if n := len(st.PlanFixers); n > 0 && st.PlanFixers[n-1].Status == "" {
		return st.PlanFixers[n-1], false, nil
	}

	record, err := ReadConflictRecord(deps.Geom.ScratchDir)
	if err != nil {
		return nil, false, fmt.Errorf("%w; way forward: transient, re-run `%s`", err, fixPlanCommand)
	}
	if len(record.Findings) == 0 {
		return nil, false, fmt.Errorf("webster: no plan-conflict record names a finding, so no begin-batch refusal is waiting for the plan fixer; way forward: run `%s` only right after begin-batch refuses with plan_conflict", fixPlanCommand)
	}
	fixersForBatch := 0
	for _, earlier := range st.PlanFixers {
		if sameFindingSet(earlier.Findings, record.Findings) {
			return nil, false, fmt.Errorf("webster: a plan fixer was already given these plan-conflict findings and did not clear them; %s", planFixRefusalWayForward)
		}
		if earlier.Batch == record.Batch {
			fixersForBatch++
		}
	}
	if fixersForBatch >= planFixersPerBatch {
		return nil, false, fmt.Errorf("webster: batch %02d already had %d plan fixers this run; %s", record.Batch, fixersForBatch, planFixRefusalWayForward)
	}

	choice, ok := deps.Roles[RoleMaster]
	if !ok {
		return nil, false, fmt.Errorf("webster: no resolved model-spec for role %q; way forward: set %s in webster.yaml with lym config webster, then re-run `%s`", RoleMaster, RoleMaster, fixPlanCommand)
	}
	if err := roleAuditEngine(deps.Engines, RoleMaster, choice); err != nil {
		return nil, false, err
	}
	git := deps.Geom.git()
	head, err := git.HeadSHA(deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, false, fmt.Errorf("webster: read HEAD before the plan fixer: %w; way forward: transient, re-run `%s`", err, fixPlanCommand)
	}
	dirtyAtSpawn, err := git.DirtyPaths(deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, false, fmt.Errorf("webster: read the worktree status before the plan fixer: %w; way forward: transient, re-run `%s`", err, fixPlanCommand)
	}

	replyPath, err := filepath.Abs(PlanFixReplyPath(deps.Geom.ScratchDir))
	if err != nil {
		return nil, false, fmt.Errorf("webster: resolve plan fixer reply path: %w", err)
	}
	recordPath, err := filepath.Abs(ConflictRecordPath(deps.Geom.ScratchDir))
	if err != nil {
		return nil, false, fmt.Errorf("webster: resolve plan-conflict record path: %w", err)
	}
	notePath := friction.NotePath(deps.FrictionDir, "webster-plan-fix")
	prompt, err := RenderPlanFixPrompt(recordPath, replyPath, deps.Geom.WorktreeRoot, deps.Geom.PlanDir, deps.Geom.StencilsDir, deps.Geom.SpecsDir, notePath, deps.Geom.ParentName)
	if err != nil {
		return nil, false, err
	}

	if err := os.Remove(replyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("webster: remove stale plan fixer reply %s: %w", replyPath, err)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{replyPath},
		ModelChoice: choice,
		Skills:      roleSkills,
		Role:        planFixerStrandRole,
		Segment:     segmentcolor.Webster,
		Timeout:     time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute,
	}
	run, err := deps.Starter.Start(spec)
	if err != nil {
		return nil, false, fmt.Errorf("webster: start the plan fixer: %w; way forward: transient, re-run `%s`", shuttleengine.ModelKey{Module: "webster", Key: string(RoleMaster)}.Wrap(err), fixPlanCommand)
	}
	runState, runDir, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.Geom.AnchorRoot, run.StrandGUID())
	if err != nil {
		// The strand is live but no record of it will be saved, so nothing later could stop it.
		stopErr := deps.Stopper.StopStrand(run.StrandGUID())
		return nil, false, errors.Join(fmt.Errorf("webster: resolve spawned plan fixer run: %w; way forward: transient, re-run `%s`", err, fixPlanCommand), stopErr)
	}

	rec := &PlanFixerState{
		StrandGUID:    run.StrandGUID(),
		SessionID:     runState.SessionID,
		Engine:        runState.Engine,
		SpawnedAt:     clk.Now().UTC().Format(time.RFC3339),
		ShuttleRunDir: runDir,
		EventsPath:    runState.EventsPath,
		EventsOffset:  runState.PromptOffset,
		Batch:         record.Batch,
		Findings:      record.Findings,
		HeadAtSpawn:   head,
		DirtyAtSpawn:  dirtyAtSpawn,
	}
	st.PlanFixers = append(st.PlanFixers, rec)
	return rec, true, nil
}

// FixPlanAwait blocks up to wait for rec's plan fixer to reach a terminal classification, as the cold fixers' shared wait classifies it, with recovery_timeout_min as its timeout.
// On done, the verdict is the reply's first line after "verdict: ", and a reply without a known verdict reads as PlanFixVerdictPlanWriter.
// A fixed verdict from a fixer that moved HEAD or left an uncommitted change on a path that was clean at the spawn is overruled to PlanFixVerdictPlanWriter, with a warning naming why, since the fixer's one job is the plan.
// The caller holds no state-mutation lease across it.
func FixPlanAwait(deps FixPlanDeps, rec *PlanFixerState, wait time.Duration, clk Clock) (*FixPlanResult, error) {
	strand := coldFixerStrand{
		label:         "fix-plan",
		strandGUID:    rec.StrandGUID,
		sessionID:     rec.SessionID,
		engine:        rec.Engine,
		spawnedAt:     rec.SpawnedAt,
		shuttleRunDir: rec.ShuttleRunDir,
		eventsPath:    rec.EventsPath,
		eventsOffset:  rec.EventsOffset,
		replyPath:     PlanFixReplyPath(deps.Geom.ScratchDir),
	}
	waited, err := awaitColdFixer(deps.Engines, deps.Reed, deps.Stopper, strand, time.Duration(deps.Config.RecoveryTimeoutMin)*time.Minute, wait, clk)
	if err != nil {
		return nil, err
	}
	result := &FixPlanResult{Status: waited.Status, DeadReason: waited.DeadReason, Reply: waited.Reply, ElapsedS: waited.ElapsedS, Warnings: waited.Warnings}
	if result.Status != DigestStatusDone {
		return result, nil
	}

	result.Verdict = replyVerdict(result.Reply)
	if result.Verdict != PlanFixVerdictFixed {
		return result, nil
	}
	if reason, err := codeTouchedSince(deps.Geom, rec.HeadAtSpawn, rec.DirtyAtSpawn); err != nil {
		return nil, err
	} else if reason != "" {
		result.Verdict = PlanFixVerdictPlanWriter
		result.Warnings = append(result.Warnings, "fix-plan: the fixer's fixed verdict is overruled, since "+reason)
	}
	return result, nil
}

// replyVerdict returns the verdict reply's first line names, and PlanFixVerdictPlanWriter when it names no known one.
func replyVerdict(reply string) string {
	firstLine, _, _ := strings.Cut(reply, "\n")
	verdict, _ := strings.CutPrefix(strings.TrimSpace(firstLine), "verdict: ")
	if verdict == PlanFixVerdictFixed {
		return PlanFixVerdictFixed
	}
	return PlanFixVerdictPlanWriter
}

// codeTouchedSince returns why the task worktree is no longer as the plan fixer found it: HEAD moved from headAtSpawn, or a path not among dirtyAtSpawn is uncommitted.
// It returns "" when neither holds.
func codeTouchedSince(geom Geometry, headAtSpawn string, dirtyAtSpawn []string) (string, error) {
	git := geom.git()
	head, err := git.HeadSHA(geom.WorktreeRoot)
	if err != nil {
		return "", fmt.Errorf("webster: read HEAD after the plan fixer: %w; way forward: transient, re-run `%s`", err, fixPlanCommand)
	}
	if head != headAtSpawn {
		return fmt.Sprintf("HEAD moved from %s to %s while it ran", headAtSpawn, head), nil
	}
	dirty, err := git.DirtyPaths(geom.WorktreeRoot)
	if err != nil {
		return "", fmt.Errorf("webster: read the worktree status after the plan fixer: %w; way forward: transient, re-run `%s`", err, fixPlanCommand)
	}
	var added []string
	for _, path := range dirty {
		if !slices.Contains(dirtyAtSpawn, path) {
			added = append(added, path)
		}
	}
	if len(added) > 0 {
		return "it left uncommitted changes in " + strings.Join(added, ", "), nil
	}
	return "", nil
}

// RecordPlanFixerTerminal sets the terminal status and verdict of the record in st whose strand is strandGUID, from result.
// It reports false when st records no such fixer, which a reset that archived the state leaves.
func RecordPlanFixerTerminal(st *State, strandGUID string, result *FixPlanResult) bool {
	for _, rec := range st.PlanFixers {
		if rec.StrandGUID == strandGUID {
			rec.Status = result.Status
			rec.DeadReason = result.DeadReason
			rec.Verdict = result.Verdict
			return true
		}
	}
	return false
}
