// waiting.go implements the batch that waits on its parent: a fork or recovery strand that stopped on a step only the run's parent can take reports NEEDS_PARENT.
// record-batch or recover-batch then records the batch in flight with status waiting, spending no recovery.

package websterengine

import (
	"fmt"
	"time"
)

// waitsOnParent reports whether bs is an in-flight batch waiting on a step only the run's parent can take.
func waitsOnParent(bs *BatchState) bool {
	return bs != nil && !bs.Terminal && bs.Status == DigestStatusWaiting
}

// recordWaiting records bs as waiting on the parent with digest, archiving the report the strand wrote so a re-begin or a fresh recovery finds none.
// The batch stays in flight with its recovery count and start commit unchanged, and the in-flight cursor stays on it.
func recordWaiting(bs *BatchState, digest *Digest, reportsDir string, number int, slug string, now func() time.Time) error {
	if _, err := archiveStaleReport(reportsDir, number, slug, now); err != nil {
		return err
	}
	bs.Digest = digest
	bs.Terminal = false
	bs.Status = DigestStatusWaiting
	return nil
}

// resumeStep is the verb that resumes waiting batch number once the parent has acted:
// begin-batch for a fork batch, recover-batch for a recovery batch.
func resumeStep(bs *BatchState, number int) string {
	if bs.Kind == "recovery" {
		return fmt.Sprintf("lym webster recover-batch %d", number)
	}
	return fmt.Sprintf("lym webster begin-batch %d", number)
}

// waitingProgressLine renders waiting batch number's progress line: its need and the verb that resumes it.
func waitingProgressLine(bs *BatchState, number int, slug string) string {
	return fmt.Sprintf("%02d-%s: %s (need: %s; resume: %s)", number, slug, DigestStatusWaiting, waitingNeed(bs), resumeStep(bs, number))
}

// waitingNeed returns the need waiting batch bs records, empty when its digest is absent.
func waitingNeed(bs *BatchState) string {
	if bs.Digest == nil {
		return ""
	}
	return bs.Digest.Need
}

// waitingRefusal is the refusal of a verb that has nothing to record for waiting batch number.
func waitingRefusal(verb string, bs *BatchState, number int) error {
	return fmt.Errorf("webster: batch %02d waits on its parent (%s), so %s has nothing to record; way forward: once the parent has taken the step, `%s`", number, waitingNeed(bs), verb, resumeStep(bs, number))
}
