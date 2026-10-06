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
	WorktreeRoot   string
	HeadSHA        string
	Reasons        []string
	SuspectPaths   []string
	Uncheckable    []string
	NewTranscripts []string
	Now            func() time.Time
	// Git answers the suspect-blob probes; nil means the real repository.
	Git Git
}

// failBatch archives the batch's report, records the batch terminal failed with its reasons,
// and clears State.CurrentBatch.
// The returned error is only the archive's I/O failure;
// the blob probes and the archive run first, so nothing in State has been mutated when one fails.
func failBatch(in failBatchInput) (*BatchFailedError, error) {
	blobs, err := suspectBlobs(orReal(in.Git), in.WorktreeRoot, in.SuspectPaths)
	if err != nil {
		return nil, err
	}
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

	// A path the batch already records keeps its recorded blob, so a re-failed recovery still checks the content the audit first flagged.
	recorded := map[string]string{}
	for _, sp := range in.Batch.SuspectPaths {
		recorded[sp.Path] = sp.Blob
	}
	for i, sp := range blobs {
		if blob, ok := recorded[sp.Path]; ok {
			blobs[i].Blob = blob
		}
	}
	if len(blobs) > 0 {
		in.Batch.SuspectPaths = blobs
	}
	if len(in.Uncheckable) > 0 {
		in.Batch.Uncheckable = in.Uncheckable
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
