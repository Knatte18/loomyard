// verifyfixer.go implements the cold verify-gate fixer that `lym webster fix-verify` drives.
// It spawns or attaches a strand at the recovery role with the cold verify-fix prompt;
// it waits until the reply file exists or the strand is dead, and it records the terminal state.
// The strand's spawn, wait and registration are internal/gatefixer's; this file is webster's adapter onto it.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// fixVerifyCommand is the verb that runs the cold fixer, the one the gate's way forward message names.
const fixVerifyCommand = "lym webster fix-verify"

// verifyFixBodyStencil is the shared verify-gate fixer body both fixers' prompts carry.
const verifyFixBodyStencil = "webster-body-verify-fix"

// verifyFixReplyFileName is the cold fixer's reply file name inside the webster scratch dir.
const verifyFixReplyFileName = "verify-fix-reply.md"

// VerifyFixReplyPath returns the path of the reply file the cold fixer writes as its final action, inside scratchDir.
func VerifyFixReplyPath(scratchDir string) string {
	return filepath.Join(scratchDir, verifyFixReplyFileName)
}

// FixVerifyDeps carries the seams the cold fixer's spawn and wait need.
// Geom is the Geometry every path is read from;
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

	// FrictionDir is the absolute friction directory, empty when Tier 2 is off.
	FrictionDir string

	// LanguageSkills are the language skills the fixer loads after its role skills; empty for none.
	LanguageSkills []string

	// Gate is the hub's gate config, from which the prompt renders the fixer's `webster-fix` gate call.
	Gate gateslot.Config

	// Register registers a spawned fixer strand with runproc by guid; nil leaves it unregistered, as a standalone or prime run does.
	Register func(guid string) error
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

// verifyFixerStore adapts a State's VerifyFixers to gatefixer.Store.
// Webster derives the report and reply paths from its geometry, so a saved record keeps neither.
type verifyFixerStore struct {
	st *State
}

// Live returns the last VerifyFixers record when it is not terminal, and nil otherwise.
func (s *verifyFixerStore) Live() (*gatefixer.Record, error) {
	n := len(s.st.VerifyFixers)
	if n == 0 || s.st.VerifyFixers[n-1].Status != "" {
		return nil, nil
	}
	return fixerRecord(s.st.VerifyFixers[n-1]), nil
}

// Save appends rec to VerifyFixers.
func (s *verifyFixerStore) Save(rec *gatefixer.Record) error {
	s.st.VerifyFixers = append(s.st.VerifyFixers, &VerifyFixerState{
		StrandGUID:    rec.StrandGUID,
		SessionID:     rec.SessionID,
		Engine:        rec.Engine,
		SpawnedAt:     rec.SpawnedAt,
		ShuttleRunDir: rec.ShuttleRunDir,
		EventsPath:    rec.EventsPath,
		EventsOffset:  rec.EventsOffset,
		Status:        rec.Status,
		DeadReason:    rec.DeadReason,
	})
	return nil
}

// fixerRecord converts a verify fixer's state record to the record gatefixer works on, with replyPath empty.
func fixerRecord(rec *VerifyFixerState) *gatefixer.Record {
	return &gatefixer.Record{
		StrandGUID:    rec.StrandGUID,
		SessionID:     rec.SessionID,
		Engine:        rec.Engine,
		SpawnedAt:     rec.SpawnedAt,
		ShuttleRunDir: rec.ShuttleRunDir,
		EventsPath:    rec.EventsPath,
		EventsOffset:  rec.EventsOffset,
		Status:        rec.Status,
		DeadReason:    rec.DeadReason,
	}
}

// awaitFixerStrand waits on rec's strand through gatefixer.Await, using timeout, wait and clk.
// It prefixes each warning with label, the verb the fixer runs under.
func awaitFixerStrand(engines AuditEngines, reed shuttleengine.ReedOps, stopper StrandStopper, rec *gatefixer.Record, label string, timeout, wait time.Duration, clk Clock) (*gatefixer.Result, error) {
	result, err := gatefixer.Await(gatefixer.Deps{Stopper: stopper, Reed: reed, Engines: engines, Clock: clk}, rec, timeout, wait)
	if err != nil {
		return nil, fmt.Errorf("webster: %s: %w", label, err)
	}
	for i, warning := range result.Warnings {
		result.Warnings[i] = label + ": " + warning
	}
	return result, nil
}

// FixVerifySpawnOrAttach returns the cold fixer strand fix-verify waits on, and whether this call spawned it into st.
// A last record of st.VerifyFixers that is not terminal is attached to as it stands.
// Otherwise it spawns a strand at the recovery role with the cold verify-fix prompt.
// The strand's shuttle output file is the reply file.
// It registers the strand through deps.Register when set, and it appends its record to st.VerifyFixers;
// the caller saves st.
// It refuses, spawning nothing, when no verify-gate report is on disk.
// It also refuses when the recovery role's engine cannot audit forks, and when the gate config has no `webster-fix` key.
// A stale reply file is removed before the spawn.
// A spawned strand whose run cannot be resolved, registered or recorded is stopped, since no record of it will be saved.
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
		return nil, false, fmt.Errorf("webster: no resolved model-spec for role %q; way forward: set %s in webster.yaml with lym config webster, then re-run `%s`", RoleRecovery, RoleRecovery, fixVerifyCommand)
	}
	if err := roleAuditEngine(deps.Engines, RoleRecovery, choice); err != nil {
		return nil, false, err
	}
	fixGate, err := deps.Gate.AgentGateCall("webster-fix")
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

	// The friction directive is rendered by gatefixer.RenderPrompt from the note path, so the body's own values carry none.
	bodyValues, err := verifyFixValues(reportPath, deps.Geom.PaneCwd, deps.Geom.PlanDir, deps.Geom.StencilsDir, outcomePath, summaryPath, "", fixGate, verifyFixBodyStencil)
	if err != nil {
		return nil, false, err
	}
	prompt, err := gatefixer.RenderPrompt(gatefixer.PromptInputs{
		StencilsDir:    deps.Geom.StencilsDir,
		BodyStencil:    verifyFixBodyStencil,
		BodyValues:     bodyValues,
		ReportPath:     reportPath,
		ReplyPath:      replyPath,
		RepoRoot:       deps.Geom.WorktreeRoot,
		PatternChanges: patternChanges,
		ParentName:     deps.Geom.ParentName,
		NotePath:       friction.NotePath(deps.FrictionDir, "webster-verify-fix-cold"),
	})
	if err != nil {
		return nil, false, err
	}

	if err := os.MkdirAll(deps.Geom.ScratchDir, 0o755); err != nil {
		return nil, false, fmt.Errorf("webster: create webster scratch dir %s: %w", deps.Geom.ScratchDir, err)
	}

	_, spawned, err := gatefixer.SpawnOrAttach(gatefixer.Deps{
		Starter:    deps.Starter,
		Stopper:    deps.Stopper,
		Reed:       deps.Reed,
		Engines:    deps.Engines,
		ShuttleCfg: deps.ShuttleCfg,
		AnchorRoot: deps.Geom.AnchorRoot,
		Clock:      clk,
		Register:   deps.Register,
	}, &verifyFixerStore{st: st}, gatefixer.Spec{
		Prompt:      prompt,
		ReportPath:  reportPath,
		ReplyPath:   replyPath,
		Role:        gatefixer.RoleVerifyFixer,
		ModelChoice: choice,
		Skills:      languageconfig.AppendSkills(roleSkills, deps.LanguageSkills),
		Segment:     segmentcolor.Webster,
		Timeout:     time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute,
	})
	if err != nil {
		return nil, false, fmt.Errorf("webster: spawn the cold verify-gate fixer: %w; way forward: transient, re-run `%s`", shuttleengine.ModelKey{Module: "webster", Key: string(RoleRecovery)}.Wrap(err), fixVerifyCommand)
	}
	return st.VerifyFixers[len(st.VerifyFixers)-1], spawned, nil
}

// FixVerifyAwait blocks up to wait for rec's cold fixer to reach a terminal classification, polling on the shared tick.
// The fixer is done once its reply file exists.
// It is dead, checked in this order, when its newest turn ended without one (DeadReasonAsking);
// when recovery_timeout_min has passed since the spawn (DeadReasonTimeout);
// or when its strand is gone (DeadReasonDied).
// Every terminal result stops the strand; only done also reads the reply and removes the run dir.
// A failed stop, reply read or removal is a warning.
// The caller holds no state-mutation lease across it.
func FixVerifyAwait(deps FixVerifyDeps, rec *VerifyFixerState, wait time.Duration, clk Clock) (*FixVerifyResult, error) {
	strand := fixerRecord(rec)
	strand.ReplyPath = VerifyFixReplyPath(deps.Geom.ScratchDir)
	waited, err := awaitFixerStrand(deps.Engines, deps.Reed, deps.Stopper, strand, "fix-verify", time.Duration(deps.Config.RecoveryTimeoutMin)*time.Minute, wait, clk)
	if err != nil {
		return nil, err
	}
	return &FixVerifyResult{Status: waited.Status, DeadReason: waited.DeadReason, Reply: waited.Reply, ElapsedS: waited.ElapsedS, Warnings: waited.Warnings}, nil
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
