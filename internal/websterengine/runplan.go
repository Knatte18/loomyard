// runplan.go ties webster to the shared run-plan machinery of internal/runplan.
// It keeps webster's names for the types, constants and functions that moved there.
// Webster's importers therefore compile unchanged.
// It also holds the way forward webster names in each shared refusal.

package websterengine

import (
	"fmt"
	"path"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
)

// Webster's names for the run-plan types.
type (
	BatchState                = runplan.BatchState
	Digest                    = runplan.Digest
	AuditWarning              = runplan.AuditWarning
	SuspectPath               = runplan.SuspectPath
	AmendedCard               = runplan.AmendedCard
	ParentAnswer              = runplan.ParentAnswer
	PartitionBatch            = runplan.PartitionBatch
	Report                    = runplan.Report
	ReportRename              = runplan.ReportRename
	PlanConflictError         = runplan.PlanConflictError
	ConflictRecord            = runplan.ConflictRecord
	ConflictRoute             = runplan.ConflictRoute
	LaterTargetRemovedError   = runplan.LaterTargetRemovedError
	VerifyFailure             = runplan.VerifyFailure
	VerifyGateReport          = runplan.VerifyGateReport
	AwaitResult               = runplan.AwaitResult
	Clock                     = runplan.Clock
	Starter                   = runplan.Starter
	StrandStopper             = runplan.StrandStopper
	RecoveryStrandRemoveError = runplan.RecoveryStrandRemoveError
	CommitLogEntry            = runplan.CommitLogEntry
)

// Webster's names for the run-plan constants.
const (
	ReportStatusOK          = runplan.ReportStatusOK
	ReportStatusFailed      = runplan.ReportStatusFailed
	ReportStatusNeedsParent = runplan.ReportStatusNeedsParent

	DigestStatusRunning = runplan.DigestStatusRunning
	DigestStatusDone    = runplan.DigestStatusDone
	DigestStatusStuck   = runplan.DigestStatusStuck
	DigestStatusDead    = runplan.DigestStatusDead
	DigestStatusFailed  = runplan.DigestStatusFailed
	DigestStatusWaiting = runplan.DigestStatusWaiting

	DeadReasonAsking  = runplan.DeadReasonAsking
	DeadReasonTimeout = runplan.DeadReasonTimeout
	DeadReasonDied    = runplan.DeadReasonDied

	ConflictCausePlanDefect = runplan.ConflictCausePlanDefect
	ConflictCauseTreeMoved  = runplan.ConflictCauseTreeMoved
	ConflictCausePlanOrder  = runplan.ConflictCausePlanOrder

	FailureKindTest    = runplan.FailureKindTest
	FailureKindPackage = runplan.FailureKindPackage
	FailureKindOpaque  = runplan.FailureKindOpaque

	DefaultAwaitWaitS = runplan.DefaultAwaitWaitS
)

// Webster's names for the run-plan sentinels.
var (
	ErrFingerprintMismatch = runplan.ErrFingerprintMismatch
	ErrPlanConflict        = runplan.ErrPlanConflict
	ErrCardNotDone         = runplan.ErrCardNotDone
	ErrLaterTargetRemoved  = runplan.ErrLaterTargetRemoved
	ErrPartitionMismatch   = runplan.ErrPartitionMismatch
)

// Webster's names for the run-plan functions that take no webster state.
var (
	ReportFileName        = runplan.ReportFileName
	ParseReport           = runplan.ParseReport
	WriteReport           = runplan.WriteReport
	Fingerprint           = runplan.Fingerprint
	AcquireStateMutation  = runplan.AcquireStateMutation
	ConflictRecordPath    = runplan.ConflictRecordPath
	RoutedConflictPath    = runplan.RoutedConflictPath
	WriteConflictRecord   = runplan.WriteConflictRecord
	ReadConflictRecord    = runplan.ReadConflictRecord
	RemoveConflictRecord  = runplan.RemoveConflictRecord
	RemoveRoutedConflict  = runplan.RemoveRoutedConflict
	ClaimConflictRoute    = runplan.ClaimConflictRoute
	ParseVerifyFailures   = runplan.ParseVerifyFailures
	VerifyGateReportPath  = runplan.VerifyGateReportPath
	WriteVerifyGateReport = runplan.WriteVerifyGateReport
)

// findBatchWayForward is webster's way to list the run's batches, named when a batch number matches none.
const findBatchWayForward = "`lym webster status` lists the run's batches, name one of those"

// ConflictRecordRel returns the conflict record's path relative to the anchor, forward-slashed for recipe text.
func ConflictRecordRel() string {
	return path.Join(lymdirs.DotLymDirName, lymdirs.ImplementerDirName, runplan.ConflictRecordName)
}

// runPlan returns the shared machinery's view of g: webster's run directory is its WebsterDir.
func (g Geometry) runPlan() runplan.Geometry {
	return runplan.Geometry{
		WorktreeRoot: g.WorktreeRoot,
		PaneCwd:      g.PaneCwd,
		PlanDir:      g.PlanDir,
		RunDir:       g.WebsterDir,
		ReportsDir:   g.ReportsDir,
		PromptsDir:   g.PromptsDir,
		ScratchDir:   g.ScratchDir,
		Git:          g.git(),
		Index:        g.Index,
	}
}

// findBatch returns the batch in batches whose number matches number, refusing with webster's way forward when none does.
func findBatch(batches []batcher.Batch, number int) (batcher.Batch, error) {
	return runplan.FindBatch(batches, number, findBatchWayForward)
}

// AwaitBatch blocks until batchNumber's batch-report file exists in reportsDir or wait elapses.
// It sits behind the `await-batch` verb.
// It reads and mutates nothing but the report path's existence.
func AwaitBatch(batches []batcher.Batch, reportsDir string, batchNumber int, wait time.Duration, clk Clock) (*AwaitResult, error) {
	return runplan.AwaitBatch(batches, reportsDir, batchNumber, wait, clk, findBatchWayForward)
}

// DispatchScope computes the one dispatch scoping shared across webster's begin, run and validate paths:
// begun is every card of a batch begin-batch recorded, and forthcoming the cards of every begun batch not yet terminal.
// A nil state returns two nil slices.
func DispatchScope(batches []batcher.Batch, st *State) (begun, forthcoming []planparser.Card) {
	if st == nil {
		return nil, nil
	}
	return runplan.DispatchScope(batches, &st.State)
}

// RecordPartition writes the batches' card ids, profiles, estimates and estimate breakdowns into st.Partition.
// It replaces any partition st held.
func RecordPartition(st *State, batches []batcher.Batch) {
	runplan.RecordPartition(&st.State, batches)
}

// PlanEditError returns nil when the plan on disk fingerprints to st.PlanFingerprint.
// Otherwise it returns the ErrFingerprintMismatch wrap naming webster's way forward.
// Callers that rewrite the plan call it before their own rewrites,
// so any difference it sees is someone else's edit rather than webster's own.
func PlanEditError(st *State, planDir, websterDir string) error {
	return runplan.PlanEditError(&st.State, planDir, func() string {
		return "the plan changed since state.json was created; " + fingerprintMismatchWayForward(st, planDir, websterDir, stepRun)
	})
}

// recoveredSuspectReasons returns the reasons batch number's recovery leaves bs's suspect paths unresolved at head.
// Each reason's way forward names re-running recover-batch for number.
func recoveredSuspectReasons(geom Geometry, st *State, bs *BatchState, number int, head string) ([]string, error) {
	return runplan.CheckRecoveredSuspects(geom.runPlan(), &st.State, bs, head, planPathClause(fmt.Sprintf("\"lym webster recover-batch %02d\"", number)))
}

// batchCardEditError returns nil when every card of b still hashes to the content bs recorded at begin-batch.
// It also returns nil when bs recorded none.
// Otherwise it returns an ErrFingerprintMismatch wrap naming each card that changed since its batch was begun.
func batchCardEditError(st *State, bs *BatchState, b batcher.Batch, planDir, websterDir string) error {
	return runplan.BatchCardEditError(bs, b, planDir, func() string {
		return fingerprintMismatchWayForward(st, planDir, websterDir, stepRun)
	})
}

// restampFingerprint re-baselines planDir's fingerprint, file hashes and begun card hashes into st.
// It runs after a sanctioned plan rewrite.
func restampFingerprint(st *State, planDir, websterDir string) error {
	return runplan.RestampFingerprint(&st.State, planDir, websterDir)
}

// RestampPlanBaseline re-baselines the plan fingerprint for a caller that has rewritten the plan.
// The caller must record the new baseline.
func RestampPlanBaseline(st *State, planDir, websterDir string) error {
	return restampFingerprint(st, planDir, websterDir)
}

// restampAndSaveFingerprint re-baselines the plan fingerprint and persists the state in one step.
// The two must happen together: an in-memory re-baseline is lost on any return before the next save,
// and that loss is the wedge the re-baseline exists to prevent.
func restampAndSaveFingerprint(geom Geometry, st *State) error {
	if err := restampFingerprint(st, geom.PlanDir, geom.WebsterDir); err != nil {
		return err
	}
	return SaveState(geom.WebsterDir, geom.ScratchDir, st)
}

// planConflictWayForward returns webster's way forward from a plan conflict at begin-batch number, by cause.
func planConflictWayForward(number int) func(cause string) string {
	return func(cause string) string {
		planEdit := "edit the plan so the named cards match the tree"
		if cause == ConflictCausePlanOrder {
			planEdit = "move each named delete to a card after the card that still references it"
		}
		return fmt.Sprintf("%s, run \"lym webster rebaseline --card NN\" naming each card you edited, then begin-batch %02d again", planEdit, number)
	}
}

// laterTargetHandBackWayForward is webster's way forward when record-batch hands batch number back to its fork.
func laterTargetHandBackWayForward(number int) string {
	return fmt.Sprintf("send this message to the batch's fork, which either restores each name and commits, or lists each rename under its report's renames, and then rewrites its report with its head; then run `lym webster record-batch %02d` again, which records the batch without handing it back a second time", number)
}

// planPathClause is the way forward for plan paths that differ from the plan the run recorded.
// It ends by naming next as the verb to re-run.
func planPathClause(next string) string {
	return fmt.Sprintf("restore them with \"lym webster restore-plan\", or accept an edit to a card no batch has begun with \"lym webster rebaseline --card NN\"; then re-run %s", next)
}

// partitionMismatchWayForward is webster's way forward from a recorded partition that no longer matches the plan.
const partitionMismatchWayForward = "`lym webster rebaseline --card NN` naming each card you edited, or restore the plan with `lym webster restore-plan`"
