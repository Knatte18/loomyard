// integration.go implements the plan-level integration-suite stage: the
// skip-check (ShouldRunIntegration) and the single dedicated integration
// fork's own await/report plumbing (AwaitIntegration/RunIntegration,
// reusing AwaitBatch's own bounded long-poll idiom over a fixed, non-batch
// report path, and webster's own ParseReport for the fork's OK/FAILED). The
// integration fork itself is spawned the same way a batch's own
// implementer is — Master's own in-session Agent-tool fork call, per
// master-template.md's own integration-fork bracket instruction — so this
// file never spawns anything; it only confirms the fork's report has
// landed and interprets it. The in-process SHA-bisect this stage runs on a
// FAILED report, and the escalation path that follows, are added alongside
// this file's own bisect/BisectAndEscalate/RecordIntegrationFailure.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// IntegrationReportFileName is the integration fork's own fixed report file
// name inside a webster reports dir — distinct from ReportFileName's
// per-batch "NN-<slug>.yaml" naming, since the integration stage is not
// itself a plan card or execution batch: there is exactly one integration
// report per run.
const IntegrationReportFileName = "integration.yaml"

// IntegrationReportPath returns the path to the integration fork's report
// file inside reportsDir. Per the Hub Geometry Invariant, the caller
// resolves reportsDir (hubgeometry.WebsterReportsDir(...)); this function
// never constructs a `_lyx` path itself.
func IntegrationReportPath(reportsDir string) string {
	return filepath.Join(reportsDir, IntegrationReportFileName)
}

// ShouldRunIntegration reports whether plan carries a plan-level
// "## verify:" section at all — the skip-check for the WHOLE integration
// stage. A plan with no such section (plan.Verify == "") never drives the
// integration fork and proceeds straight to the summary/finish path, per
// plan-format-v3.md's "verify model" (the plan-level integration verify is
// optional): no error, no empty fork.
func ShouldRunIntegration(plan *planparser.Plan) bool {
	return plan.Verify != ""
}

// IntegrationAwaitResult is what one AwaitIntegration call hands back:
// ReportPresent reports whether the integration fork's report file existed
// by the time the call returned, and ElapsedS is how many seconds this call
// actually blocked — mirroring AwaitResult's own shape (awaitbatch.go) for
// the per-batch case.
type IntegrationAwaitResult struct {
	ReportPresent bool
	ElapsedS      int
}

// AwaitIntegration blocks until the integration fork's report file exists
// in reportsDir or wait elapses, re-checking on a fixed one-second tick
// (awaitTick, awaitbatch.go's own constant) via clk — the integration-stage
// analog of AwaitBatch, over the ONE fixed IntegrationReportPath rather than
// a per-batch report path, since the integration stage is not itself a plan
// card or execution batch. It reads and mutates NOTHING but the report
// path's existence, mirroring AwaitBatch's own read-only posture.
func AwaitIntegration(reportsDir string, wait time.Duration, clk Clock) (*IntegrationAwaitResult, error) {
	reportPath := IntegrationReportPath(reportsDir)

	start := clk.Now()
	for {
		if _, statErr := os.Stat(reportPath); statErr == nil {
			return &IntegrationAwaitResult{
				ReportPresent: true,
				ElapsedS:      int(clk.Now().Sub(start).Seconds()),
			}, nil
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("webster: stat integration report %s: %w", reportPath, statErr)
		}

		elapsed := clk.Now().Sub(start)
		if elapsed >= wait {
			return &IntegrationAwaitResult{
				ReportPresent: false,
				ElapsedS:      int(elapsed.Seconds()),
			}, nil
		}
		clk.Sleep(awaitTick)
	}
}

// RunIntegration is the integration stage's own run-once trigger: it blocks
// (via AwaitIntegration) until the integration fork's report lands or wait
// elapses, then parses it via ParseReport. It never runs the bisect itself
// — see this file's own bisect and BisectAndEscalate, which a caller
// invokes separately once RunIntegration reports a FAILED status — this
// function is the trigger/skip/await plumbing only.
func RunIntegration(reportsDir string, wait time.Duration, clk Clock) (*Report, error) {
	result, err := AwaitIntegration(reportsDir, wait, clk)
	if err != nil {
		return nil, err
	}
	if !result.ReportPresent {
		return nil, fmt.Errorf("webster: integration report did not land within %s", wait)
	}
	return ParseReport(IntegrationReportPath(reportsDir))
}
