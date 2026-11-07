// batchfail.go implements failBatch, the primitive that takes a batch terminal-failed.
// It archives the batch report and records the failure,
// so a batch rejected on its merits never stays non-terminal with an OK report on disk.

package websterengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrBatchFailed is the sentinel that a batch-failed error unwraps to.
var ErrBatchFailed = errors.New("webster: batch failed")

// BatchFailedError reports a batch webster rejected on its merits.
// The batch is terminal with digest status failed and its report archived.
type BatchFailedError struct {
	Number         int
	Batch          string
	Reasons        []string
	SuspectPaths   []string
	ArchivedReport string
	// WayForward replaces the default recover-batch call as the way forward, for a failure that call cannot clear.
	// It carries no "way forward: " prefix.
	WayForward string
	// CardAmended is true when a card was amended after the attempt began and no recovery prompt has rendered it.
	CardAmended bool
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
	if e.WayForward != "" {
		fmt.Fprintf(&b, "; way forward: %s", e.WayForward)
	} else {
		fmt.Fprintf(&b, "; way forward: lym webster recover-batch %02d", e.Number)
	}
	return b.String()
}

// Unwrap returns ErrBatchFailed.
func (e *BatchFailedError) Unwrap() error { return ErrBatchFailed }

// amendedReasons returns one card_amended reason for each amended card that no recovery spawn has rendered yet.
func amendedReasons(bs *BatchState) []string {
	var reasons []string
	for _, a := range bs.AmendedCards {
		if !a.Rendered {
			reasons = append(reasons, fmt.Sprintf("card %s was amended after this attempt began; recovery re-runs the batch on the amended card", a.Card))
		}
	}
	return reasons
}

// failBatchInput carries everything failBatch needs to fail one batch.
type failBatchInput struct {
	State          *State
	Batch          *BatchState
	Number         int
	Slug           string
	ReportsDir     string
	PaneCwd        string
	WorktreeRoot   string
	HeadSHA        string
	Reasons        []string
	SuspectPaths   []string
	Uncheckable    []string
	NewTranscripts []string
	// WayForward is the alternative way forward reported in the failure; empty selects the default.
	WayForward string
	Now        func() time.Time
	// Git answers the suspect-blob probes.
	// Nil means the real repository.
	Git Git
}

// failBatch archives the batch's report, records it terminal failed with its reasons, and clears State.CurrentBatch.
// Every amendment not yet rendered into a recovery prompt adds its own reason and sets CardAmended on the error,
// so the failure carries it whatever else failed; it is never recorded as uncheckable.
// The returned error is an I/O failure of the blob probes or of the archive;
// both run before any mutation, so a failure leaves State untouched.
func failBatch(in failBatchInput) (*BatchFailedError, error) {
	blobs, err := suspectBlobs(orReal(in.Git), in.PaneCwd, in.WorktreeRoot, in.SuspectPaths)
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
	amended := amendedReasons(in.Batch)
	errorReasons := append(append([]string(nil), in.Reasons...), amended...)
	reasons := append([]string(nil), in.Reasons...)
	for _, p := range in.SuspectPaths {
		reasons = append(reasons, "suspect path: "+p)
	}
	reasons = append(reasons, amended...)

	// A path the batch already records keeps its recorded blob,
	// so a re-failed recovery still checks the content the audit first flagged.
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
		Reasons:        errorReasons,
		SuspectPaths:   in.SuspectPaths,
		ArchivedReport: archived,
		WayForward:     in.WayForward,
		CardAmended:    len(amended) > 0,
	}, nil
}
