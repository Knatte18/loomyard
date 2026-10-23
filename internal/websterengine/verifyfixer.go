// verifyfixer.go implements the cold verify-gate fixer `lyx webster fix-verify` drives:
// the spawn-or-attach of a strand at the recovery role with the cold verify-fix prompt, the bounded wait that classifies it done on its reply file or dead, and its terminal record.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// fixVerifyCommand is the verb that runs the cold fixer, as the gate's way forward names it and Master runs it.
const fixVerifyCommand = "lyx webster fix-verify"

// verifyFixerStrandRole is the reed strand role the cold fixer runs under.
const verifyFixerStrandRole = "verify-fixer"

// verifyFixReplyFileName is the cold fixer's reply file name inside the webster scratch dir.
const verifyFixReplyFileName = "verify-fix-reply.md"

// VerifyFixReplyPath returns the path of the reply file the cold fixer writes as its final action, inside scratchDir.
func VerifyFixReplyPath(scratchDir string) string {
	return filepath.Join(scratchDir, verifyFixReplyFileName)
}

// FixVerifyDeps carries the seams the cold fixer's spawn and wait need.
// Geom is the told Geometry every path is read from;
// Reed answers the liveness read, and Stopper is the seam every stop goes through.
type FixVerifyDeps struct {
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

	// LanguageSkills are the hub's language skills the fixer loads after roleSkills; empty for none.
	LanguageSkills []string

	// Gate is the hub's gate config, from which the prompt renders the fixer's `round` gate call.
	Gate gateslot.Config
}

// FixVerifyResult is what one FixVerifyAwait call hands back.
type FixVerifyResult struct {
	// Status is DigestStatusRunning when the wait elapsed first, else DigestStatusDone or DigestStatusDead.
	Status string
	// DeadReason is set only when Status is DigestStatusDead.
	DeadReason string
	// Reply is the fixer's reply file content, set only when Status is DigestStatusDone.
	Reply string
	// ElapsedS is the seconds since the spawn.
	ElapsedS int
	// Warnings are cleanup failures that change no status.
	Warnings []string
}

// FixVerifySpawnOrAttach returns the cold fixer strand fix-verify waits on, and whether this call spawned it into st.
// A last record of st.VerifyFixers that is not terminal is attached to as it stands.
// Otherwise it spawns a strand at the recovery role with the cold verify-fix prompt, whose shuttle output file is the reply file, and appends its record to st.VerifyFixers;
// the caller saves st.
// It refuses, spawning nothing, when no verify-gate report is on disk, when the recovery role's engine cannot audit forks, and when the gate config has no `round` list.
// A stale reply file is removed before the spawn.
func FixVerifySpawnOrAttach(deps FixVerifyDeps, st *State, clk Clock) (*VerifyFixerState, bool, error) {
	if n := len(st.VerifyFixers); n > 0 && st.VerifyFixers[n-1].Status == "" {
		return st.VerifyFixers[n-1], false, nil
	}

	reportPath, err := filepath.Abs(VerifyGateReportPath(deps.Geom.ReportsDir))
	if err != nil {
		return nil, false, fmt.Errorf("webster: resolve verify-gate report path: %w", err)
	}
	if _, err := os.Stat(reportPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, fmt.Errorf("webster: no verify-gate report at %s, so no gate failure is waiting for the cold fixer; way forward: run `%s` only when a `Gate findings recorded at` message names it", reportPath, fixVerifyCommand)
		}
		return nil, false, fmt.Errorf("webster: stat verify-gate report %s: %w; way forward: transient, re-run `%s`", reportPath, err, fixVerifyCommand)
	}

	choice, ok := deps.Roles[RoleRecovery]
	if !ok {
		return nil, false, fmt.Errorf("webster: no resolved model-spec for role %q", RoleRecovery)
	}
	if err := roleAuditEngine(deps.Engines, RoleRecovery, choice); err != nil {
		return nil, false, err
	}
	fixGate, err := deps.Gate.GateCall("round")
	if err != nil {
		return nil, false, err
	}
	patternChanges, err := discussionparser.DeclaredPatternChanges(deps.Geom.DecisionRecordPath)
	if err != nil {
		return nil, false, fmt.Errorf("%w re-run `%s`", err, fixVerifyCommand)
	}

	outcomePath, err := filepath.Abs(OutcomePath(deps.Geom.WebsterDir))
	if err != nil {
		return nil, false, fmt.Errorf("webster: resolve outcome path: %w", err)
	}
	summaryPath, err := filepath.Abs(summaryparser.Path(deps.Geom.WebsterDir))
	if err != nil {
		return nil, false, fmt.Errorf("webster: resolve summary path: %w", err)
	}
	replyPath, err := filepath.Abs(VerifyFixReplyPath(deps.Geom.ScratchDir))
	if err != nil {
		return nil, false, fmt.Errorf("webster: resolve cold fixer reply path: %w", err)
	}

	notePath := friction.NotePath(deps.FrictionDir, "webster-verify-fix-cold")
	prompt, err := RenderVerifyFixColdPrompt(reportPath, replyPath, deps.Geom.WorktreeRoot, deps.Geom.RepoRoot, deps.Geom.PlanDir, deps.Geom.StencilsDir, outcomePath, summaryPath, notePath, fixGate, patternChanges, deps.Geom.ParentName)
	if err != nil {
		return nil, false, err
	}

	if err := os.MkdirAll(deps.Geom.ScratchDir, 0o755); err != nil {
		return nil, false, fmt.Errorf("webster: create webster scratch dir %s: %w", deps.Geom.ScratchDir, err)
	}
	if err := os.Remove(replyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("webster: remove stale cold fixer reply %s: %w", replyPath, err)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{replyPath},
		ModelChoice: choice,
		Skills:      languageconfig.AppendSkills(roleSkills, deps.LanguageSkills),
		Role:        verifyFixerStrandRole,
		Segment:     segmentcolor.Webster,
		Timeout:     time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute,
	}
	run, err := deps.Starter.Start(spec)
	if err != nil {
		return nil, false, fmt.Errorf("webster: start the cold verify-gate fixer: %w; way forward: transient, re-run `%s`", shuttleengine.ModelKey{Module: "webster", Key: string(RoleRecovery)}.Wrap(err), fixVerifyCommand)
	}
	runState, runDir, err := shuttleengine.FindRun(deps.ShuttleCfg, deps.Geom.AnchorRoot, run.StrandGUID())
	if err != nil {
		return nil, false, fmt.Errorf("webster: resolve spawned cold fixer run: %w", err)
	}

	rec := &VerifyFixerState{
		StrandGUID:    run.StrandGUID(),
		SessionID:     runState.SessionID,
		Engine:        runState.Engine,
		SpawnedAt:     clk.Now().UTC().Format(time.RFC3339),
		ShuttleRunDir: runDir,
		EventsPath:    runState.EventsPath,
		EventsOffset:  runState.PromptOffset,
	}
	st.VerifyFixers = append(st.VerifyFixers, rec)
	return rec, true, nil
}

// FixVerifyAwait blocks up to wait for rec's cold fixer to reach a terminal classification, polling on the shared tick.
// The fixer is done once its reply file exists.
// It is dead when its newest turn ended without one (DeadReasonAsking), when recovery_timeout_min has passed since the spawn (DeadReasonTimeout), or when its strand is gone (DeadReasonDied), checked in that order.
// Every terminal result stops the strand; only done also reads the reply and removes the run dir.
// A failed stop, reply read or removal is a warning.
// The caller holds no state-mutation lease across it.
func FixVerifyAwait(deps FixVerifyDeps, rec *VerifyFixerState, wait time.Duration, clk Clock) (*FixVerifyResult, error) {
	spawnedAt, err := time.Parse(time.RFC3339, rec.SpawnedAt)
	if err != nil {
		return nil, fmt.Errorf("webster: parse the cold fixer's recorded spawnedAt %q: %w", rec.SpawnedAt, err)
	}
	engine, err := sessionAuditEngine(deps.Engines, rec.SessionID, rec.Engine)
	if err != nil {
		return nil, err
	}
	replyPath := VerifyFixReplyPath(deps.Geom.ScratchDir)
	timeout := time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute

	gather := func() (Digest, bool, error) {
		if _, err := os.Stat(replyPath); err == nil {
			return Digest{Status: DigestStatusDone}, true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return Digest{}, false, fmt.Errorf("webster: stat cold fixer reply %s: %w", replyPath, err)
		}
		turnEnded, err := TurnEndedAfter(rec.EventsPath, rec.EventsOffset, engine, TurnEndRead{Now: clk.Now()})
		if err != nil {
			return Digest{}, false, err
		}
		if turnEnded {
			return Digest{Status: DigestStatusDead, DeadReason: DeadReasonAsking}, true, nil
		}
		if clk.Now().Sub(spawnedAt) > timeout {
			return Digest{Status: DigestStatusDead, DeadReason: DeadReasonTimeout}, true, nil
		}
		live, err := StrandLive(deps.Reed, rec.StrandGUID)
		if err != nil {
			return Digest{}, false, err
		}
		if !live {
			return Digest{Status: DigestStatusDead, DeadReason: DeadReasonDied}, true, nil
		}
		return Digest{Status: DigestStatusRunning}, false, nil
	}

	digest, err := PollUntilTerminal(gather, wait, clk)
	if err != nil {
		return nil, err
	}
	result := &FixVerifyResult{Status: digest.Status, DeadReason: digest.DeadReason, ElapsedS: int(clk.Now().Sub(spawnedAt).Seconds())}
	if digest.Status == DigestStatusRunning {
		return result, nil
	}

	if err := deps.Stopper.StopStrand(rec.StrandGUID); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("fix-verify: stop strand %s: %v", rec.StrandGUID, err))
	}
	if digest.Status == DigestStatusDone {
		reply, err := os.ReadFile(replyPath)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("fix-verify: read reply %s: %v", replyPath, err))
		}
		result.Reply = string(reply)
		if rec.ShuttleRunDir != "" {
			if err := os.RemoveAll(rec.ShuttleRunDir); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("fix-verify: remove run dir %s: %v", rec.ShuttleRunDir, err))
			}
		}
	}
	return result, nil
}

// RecordVerifyFixerTerminal sets the terminal status of the record in st whose strand is strandGUID, from result.
// It reports false when st records no such fixer, which a reset that archived the state leaves.
func RecordVerifyFixerTerminal(st *State, strandGUID string, result *FixVerifyResult) bool {
	for _, rec := range st.VerifyFixers {
		if rec.StrandGUID == strandGUID {
			rec.Status = result.Status
			rec.DeadReason = result.DeadReason
			return true
		}
	}
	return false
}
