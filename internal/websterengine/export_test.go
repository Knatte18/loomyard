// export_test.go exposes the unexported done-check failure seams of record-batch to the package's external tests.
// It also exposes the composed prompt templates.
// It gives the package's internal tests webster's names for the run-plan helpers that moved to internal/runplan.

package websterengine

import (
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// ImplementerBodyTemplate returns the shared implementer job body read from stencilsDir.
func ImplementerBodyTemplate(stencilsDir string) ([]byte, error) {
	return stencilstore.Read(stencilsDir, "webster-body-implementer")
}

// ForkTemplate returns the composed thin in-session fork prompt template, read from stencilsDir.
var ForkTemplate = composeForkTemplate

// RecoveryTemplate returns the composed full cold-start recovery prompt template, read from stencilsDir.
var RecoveryTemplate = composeRecoveryTemplate

// MaxRecoveries is how many counted recovery spawns one batch gets.
const MaxRecoveries = maxRecoveries

// CardNotDoneInputs is the input failCardNotDone takes.
type CardNotDoneInputs = cardNotDoneInputs

// FailCardNotDone exposes failCardNotDone.
var FailCardNotDone = failCardNotDone

// CardNotDoneFor returns the error the post-batch checks build from findings, or nil when findings is empty.
func CardNotDoneFor(findings []planindex.Finding) error {
	notDone := runplan.CardNotDoneFor(0, nil, findings)
	if notDone == nil {
		return nil
	}
	return notDone
}

// The run-plan helpers the internal tests call by their webster names.
var (
	fingerprint         = runplan.Fingerprint
	planFileHashes      = runplan.PlanFileHashes
	overviewFrameHash   = runplan.OverviewFrameHash
	batchCardHashes     = runplan.BatchCardHashes
	storePlanBaseline   = runplan.StorePlanBaseline
	planBaselinePath    = runplan.PlanBaselinePath
	distill             = runplan.Distill
	trackedRel          = runplan.TrackedRel
	suspectBlobs        = runplan.SuspectBlobs
	cardGateCommand     = runplan.CardGateCommand
	renderCardGates     = runplan.RenderCardGates
	renderBatchGate     = runplan.RenderBatchGate
	isLastBatch         = runplan.IsLastBatch
	cardHint            = runplan.CardHint
	parseVerifyFailures = runplan.ParseVerifyRun
	canonicalPath       = runplan.CanonicalPath
)

// planBaselineDirName is the plan baseline store's directory under the run directory.
const planBaselineDirName = runplan.PlanBaselineDirName

// opaqueFailureID is the id of the identity a verify run with nothing parseable reports.
const opaqueFailureID = runplan.OpaqueFailureID

// evidenceBases is the pair of commits the suspect-path evidence is checked against.
type evidenceBases = runplan.EvidenceBases

// completedCards returns the cards of every terminal batch in batches, counting exclude as completed too.
func completedCards(batches []batcher.Batch, st *State, exclude int) []planparser.Card {
	if st == nil {
		return nil
	}
	return runplan.CompletedCards(batches, &st.State, exclude)
}

// restampBaseline records planDir's baseline into st.
func restampBaseline(st *State, planDir, websterDir string) error {
	return runplan.RestampBaseline(&st.State, planDir, websterDir)
}

// runEvidenceBases picks st's start commit and last batch head by git ancestry.
func runEvidenceBases(geom Geometry, st *State) (evidenceBases, error) {
	return runplan.RunEvidenceBases(geom.runPlan(), &st.State)
}

// checkSuspectPaths sorts paths into differing and unverifiable against st and base.
func checkSuspectPaths(geom Geometry, st *State, base string, paths []string) (differing, unverifiable []string, err error) {
	return runplan.CheckSuspectPaths(geom.runPlan(), &st.State, base, paths)
}

// checkRecoveredSuspects is the production suspect check recover-batch runs, with webster's recover-batch way forward.
var checkRecoveredSuspects = recoveredSuspectReasons
