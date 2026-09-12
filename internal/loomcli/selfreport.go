// selfreport.go implements the whole detect-and-file step for Go-detected loom structural
// anomalies as told-input functions, kept here rather than inline in drive's cobra closure
// precisely so every branch is reachable from a direct Tier-1 call with no tmux, no git, and no
// real run.

package loomcli

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// selfreportDeps carries every input detectAndFileAnomalies needs, told rather than derived, so
// the whole branch suite can be driven with stub seams and no filesystem, git, or tmux access.
// IsLedgerPath and ReadLedger are function fields, not shedadapters calls made directly, so the
// branch suite can count calls without reaching the engine or the filesystem; drive fills them
// with shedadapters.IsLedgerPath and shedadapters.ReadLedger. FileIssue matches
// selfreportengine.CreateIssue's own shape and is filled with that function in production.
type selfreportDeps struct {
	// Ctx is the drive verb's own context, consulted only for its Err(), never for cancellation
	// propagation into any I/O this step performs.
	Ctx context.Context
	// Selfreport is the resolved loom.yaml selfreport knob. False skips the whole step before any
	// read.
	Selfreport bool
	// Entry is the entry-time observation observeEntry produced, told rather than reread here so
	// the cancelled-context path can detect a crash-resume with no read of its own.
	Entry loomengine.EntryObservation
	// StatusPath and StatusLockPath name the status file this step re-reads after shed.Run
	// returns, never the value shed.Run itself returned.
	StatusPath     string
	StatusLockPath string
	// MarkerPath and MarkerLockPath name the machine-local filed-title marker.
	MarkerPath     string
	MarkerLockPath string
	// RunErr is the error shed.Run returned, told rather than re-derived, which is what keeps
	// every branch reachable with no real run.
	RunErr error
	// IsLedgerPath recognizes a history entry's output value as a ledger file path.
	IsLedgerPath func(path string) bool
	// ReadLedger reads and parses a recognized ledger file path.
	ReadLedger func(path string) (shedadapters.Ledger, error)
	// FileIssue files one GitHub issue, matching selfreportengine.CreateIssue's own signature.
	FileIssue func(title string, body *string, labels []string) (url string, number int, err error)
}

// observeEntry probes the run lock non-blockingly and, on the reading path, reads the status file,
// in that order -- an order that is load-bearing and must not be swapped. Probing first is what
// makes the residual self-correcting: Shed persists its terminal state before Run returns and the
// deferred release follows, so a driver that finishes between the two steps has already written
// done, and the read that follows sees done rather than running.
// When enabled is false it returns the zero observation immediately, performing no probe and no
// read -- a disabled run must pay for neither.
// Any probe or read failure degrades to a logger.Warn and a zero observation with Observed false;
// nothing propagates.
func observeEntry(enabled bool, runLockPath, statusPath, statusLockPath string) loomengine.EntryObservation {
	if !enabled {
		return loomengine.EntryObservation{}
	}

	probe, free, err := lock.TryAcquireWriteLock(runLockPath)
	if err != nil {
		logger.Warn("loomcli: could not probe the run lock for the self-report entry observation", "path", runLockPath, "cause", err)
		return loomengine.EntryObservation{}
	}
	runLockHeld := !free
	if free {
		_ = probe.Release()
	}

	shed, found, err := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the status file for the self-report entry observation", "path", statusPath, "cause", err)
		return loomengine.EntryObservation{}
	}
	if !found {
		return loomengine.EntryObservation{}
	}

	var product loomengine.Status
	if len(shed.Product) > 0 {
		if uerr := json.Unmarshal(shed.Product, &product); uerr != nil {
			logger.Warn("loomcli: could not decode the product for the self-report entry observation", "path", statusPath, "cause", uerr)
			return loomengine.EntryObservation{}
		}
	}

	return loomengine.EntryObservation{
		Observed:        true,
		RunLockHeld:     runLockHeld,
		State:           shed.State,
		CurrentProducer: shed.CurrentProducer,
		HistoryLength:   len(shed.History),
		Slug:            product.Slug,
		Parent:          product.Parent,
	}
}

// detectAndFileAnomalies owns all three skips itself, checked before anything is read: first, a
// disabled knob returns immediately, before the status file, before any ledger, before the
// marker; second, a busy shed.Run error returns immediately, because that return means this
// process never ran the machine and never owned the run lock, so anything it observed at entry
// belongs to a live driver; third, a done context returns -- but only after one exception, filing
// a lone crash-resume anomaly when deps.Entry reports one, because the cancellation arm persists
// the paused state and the in-memory crash observation would otherwise be lost for good.
// Past the three skips, the ordinary filing pass re-reads the final status from the status file --
// never from the value shed.Run returned, which is documented meaningless whenever the returned
// error is non-nil, and this step runs on that path too.
func detectAndFileAnomalies(deps selfreportDeps) {
	if !deps.Selfreport {
		return
	}
	if errors.Is(deps.RunErr, shedengine.ErrShedBusy) {
		return
	}
	if deps.Ctx.Err() != nil {
		if crash, ok := loomengine.DetectCrashResume(deps.Entry); ok {
			runFilingPass(deps, []loomengine.Anomaly{crash})
		}
		return
	}

	final, found, err := state.ReadJSONStrict[shedengine.Status](deps.StatusPath, deps.StatusLockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the final status for self-report detection", "path", deps.StatusPath, "cause", err)
		return
	}
	if !found {
		return
	}

	var product loomengine.Status
	if len(final.Product) > 0 {
		if uerr := json.Unmarshal(final.Product, &product); uerr != nil {
			logger.Warn("loomcli: could not decode the product for self-report detection", "path", deps.StatusPath, "cause", uerr)
			return
		}
	}

	ledgers := discoverLedgers(deps, final)
	anomalies := loomengine.DetectAnomalies(deps.Entry, final, product, ledgers)
	runFilingPass(deps, anomalies)
}

// discoverLedgers walks final's history entries, offering every non-empty output value to
// deps.IsLedgerPath and reading each distinct accepted path through deps.ReadLedger. Three skips
// apply, each non-fatal and none producing an anomaly: an empty output is skipped before the
// predicate is consulted, since the Bouncer's seed call publishes an explicitly empty pointer; a
// non-empty output the predicate rejects is skipped and never read, so a fail-loud parser is never
// handed a producer artifact or one of the Burler's own same-prefix files; and an accepted path
// whose read or parse fails -- including a file that no longer exists, which is normal for an
// ephemeral ledger -- is warned and skipped.
// Each parsed ledger entry becomes one loomengine.LedgerObservation carrying the round the file
// claimed and the producer name read from the history entry that published the path, never a
// recipe row name re-declared here.
func discoverLedgers(deps selfreportDeps, final shedengine.Status) []loomengine.LedgerObservation {
	seen := make(map[string]bool)
	var ledgers []loomengine.LedgerObservation
	for _, h := range final.History {
		if h.Output == "" {
			continue
		}
		if !deps.IsLedgerPath(h.Output) {
			continue
		}
		if seen[h.Output] {
			continue
		}
		seen[h.Output] = true

		ledger, err := deps.ReadLedger(h.Output)
		if err != nil {
			logger.Warn("loomcli: could not read a discovered ledger for self-report detection", "path", h.Output, "cause", err)
			continue
		}
		for _, e := range ledger.Entries {
			ledgers = append(ledgers, loomengine.LedgerObservation{
				Producer: h.Producer,
				Round:    ledger.Round,
				Key:      e.Key,
				Rounds:   e.Rounds,
				Status:   e.Status,
			})
		}
	}
	return ledgers
}

// runFilingPass performs the four ordered steps of the filing pass: collapse by title, filter
// against the marker, file one filing-seam call per surviving anomaly in the slice's deterministic
// order, and record a title in the marker immediately after that title's own filing call
// succeeds -- never in advance and never in one batch at the end, so a failed call leaves its title
// unrecorded and therefore retried next run while its already-filed siblings stay recorded.
func runFilingPass(deps selfreportDeps, anomalies []loomengine.Anomaly) {
	collapsed := collapseAnomaliesByTitle(anomalies)
	if len(collapsed) == 0 {
		return
	}

	marker := readFiledMarker(deps.MarkerPath, deps.MarkerLockPath)

	for _, a := range collapsed {
		if marker.has(a.Title) {
			continue
		}

		body := loomengine.RenderAnomalyBody(a)
		if _, _, err := deps.FileIssue(a.Title, &body, selfreportengine.DefaultLabels()); err != nil {
			logger.Warn("loomcli: could not file a self-report issue for a detected loom anomaly", "title", a.Title, "cause", err)
			continue
		}

		marker.record(a.Title)
		writeFiledMarker(deps.MarkerPath, deps.MarkerLockPath, marker)
	}
}

// collapseAnomaliesByTitle reduces anomalies to one anomaly per distinct title, keeping the
// highest Round when the duplicates are recurring-finding anomalies and the first occurrence
// otherwise, in the input slice's own order of first appearance.
// This is required, not defensive tidying -- the judge carries an open entry forward losslessly
// into every later round's ledger, so from round three onward one recurring finding appears in
// several ledger files at once and would otherwise produce one identically-titled anomaly per
// file. Keeping the highest-round occurrence is what makes the body carry the fullest rounds list.
func collapseAnomaliesByTitle(anomalies []loomengine.Anomaly) []loomengine.Anomaly {
	order := make([]string, 0, len(anomalies))
	byTitle := make(map[string]loomengine.Anomaly, len(anomalies))

	for _, a := range anomalies {
		existing, ok := byTitle[a.Title]
		if !ok {
			byTitle[a.Title] = a
			order = append(order, a.Title)
			continue
		}
		if a.Kind == loomengine.AnomalyRecurringFinding {
			if a.Round > existing.Round {
				byTitle[a.Title] = a
			}
			continue
		}
		// Unreachable by construction: only recurring-finding anomalies carry a non-zero Round,
		// and one loomengine.DetectAnomalies call returns at most one crash-resume and at most
		// one halt-kind anomaly, whose title shapes collide neither with each other nor with a
		// recurring-finding title. Kept as a defensive guard, following the in-repo convention
		// selfreportengine.CreateIssue's own targetRepo guard already sets, so a future change to
		// the detector's output fails predictably rather than silently.
	}

	collapsed := make([]loomengine.Anomaly, 0, len(order))
	for _, title := range order {
		collapsed = append(collapsed, byTitle[title])
	}
	return collapsed
}

// selfreportFiledMarker is the machine-local record of which anomaly titles have already been
// filed as GitHub issues. It stays a dumb string set with no parsing of its own -- dedupe
// granularity is whatever the title shape already encodes.
type selfreportFiledMarker struct {
	Titles []string `json:"titles"`
}

// has reports whether title is already recorded in m.
func (m selfreportFiledMarker) has(title string) bool {
	return slices.Contains(m.Titles, title)
}

// record appends title to m's recorded set.
func (m *selfreportFiledMarker) record(title string) {
	m.Titles = append(m.Titles, title)
}

// readFiledMarker reads the marker at path/lockPath. A read failure -- including the file not
// existing -- is treated as an empty marker.
func readFiledMarker(path, lockPath string) selfreportFiledMarker {
	m, found, err := state.ReadJSONStrict[selfreportFiledMarker](path, lockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the self-report filed marker; treating it as empty", "path", path, "cause", err)
		return selfreportFiledMarker{}
	}
	if !found {
		return selfreportFiledMarker{}
	}
	return m
}

// writeFiledMarker writes m to path/lockPath. A write failure is warned and nothing else.
func writeFiledMarker(path, lockPath string, m selfreportFiledMarker) {
	if err := state.WriteJSON(path, lockPath, m); err != nil {
		logger.Warn("loomcli: could not write the self-report filed marker", "path", path, "cause", err)
	}
}
