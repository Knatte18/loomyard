// batchfail.go implements the one primitive that takes a batch terminal-failed.
// record-batch and recover-batch share it, so a batch rejected on its merits never stays non-terminal with an OK report on disk, the state that made the three verbs refuse each other.

package websterengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrBatchFailed is the sentinel a *BatchFailedError unwraps to.
var ErrBatchFailed = errors.New("webster: batch failed")

// BatchFailedError reports a batch webster rejected on its merits.
// The batch is terminal with digest status failed and its report archived.
type BatchFailedError struct {
	Number         int
	Batch          string
	Reasons        []string
	SuspectPaths   []string
	ArchivedReport string
}

// Error names the reasons, the suspect paths and the archived report, and ends with the way forward.
func (e *BatchFailedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "webster: batch %s failed: %s", e.Batch, strings.Join(e.Reasons, "; "))
	if len(e.SuspectPaths) > 0 {
		fmt.Fprintf(&b, "; suspect paths: %s", strings.Join(e.SuspectPaths, ", "))
	}
	if e.ArchivedReport != "" {
		fmt.Fprintf(&b, "; report archived to %s", e.ArchivedReport)
	}
	fmt.Fprintf(&b, "; way forward: lyx webster recover-batch %02d", e.Number)
	return b.String()
}

// Unwrap returns ErrBatchFailed.
func (e *BatchFailedError) Unwrap() error { return ErrBatchFailed }

// failBatchInput carries everything failBatch needs to fail one batch.
type failBatchInput struct {
	State          *State
	Batch          *BatchState
	Number         int
	Slug           string
	ReportsDir     string
	HeadSHA        string
	Reasons        []string
	SuspectPaths   []string
	NewTranscripts []string
	Now            func() time.Time
}

// failBatch archives the batch's report, records the batch terminal failed with its reasons,
// and clears State.CurrentBatch.
// The returned error is only the archive's I/O failure;
// the archive runs first, so nothing in State has been mutated when it fails.
func failBatch(in failBatchInput) (*BatchFailedError, error) {
	archived, err := archiveStaleReport(in.ReportsDir, in.Number, in.Slug, in.Now)
	if err != nil {
		return nil, err
	}

	for _, t := range in.NewTranscripts {
		if !slices.Contains(in.State.SeenForkTranscripts, t) {
			in.State.SeenForkTranscripts = append(in.State.SeenForkTranscripts, t)
		}
		if !slices.Contains(in.Batch.ForkTranscripts, t) {
			in.Batch.ForkTranscripts = append(in.Batch.ForkTranscripts, t)
		}
	}

	batchName := fmt.Sprintf("%02d-%s", in.Number, in.Slug)
	reasons := append([]string(nil), in.Reasons...)
	for _, p := range in.SuspectPaths {
		reasons = append(reasons, "suspect path: "+p)
	}

	in.Batch.Terminal = true
	in.Batch.Status = DigestStatusFailed
	in.Batch.Digest = &Digest{
		Batch:   batchName,
		Status:  DigestStatusFailed,
		HeadSHA: in.HeadSHA,
		Reasons: reasons,
	}
	in.State.CurrentBatch = 0

	return &BatchFailedError{
		Number:         in.Number,
		Batch:          batchName,
		Reasons:        in.Reasons,
		SuspectPaths:   in.SuspectPaths,
		ArchivedReport: archived,
	}, nil
}
