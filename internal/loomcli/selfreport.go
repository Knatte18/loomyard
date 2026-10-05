// selfreport.go implements the whole detect-and-file step for Go-detected loom structural
// anomalies as told-input functions, kept here rather than inline in drive's cobra closure
// precisely so every branch is reachable from a direct Tier-1 call with no tmux, no git, and no
// real run.

package loomcli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// selfreportDeps carries every input detectAndFileAnomalies needs, told rather than derived, so
// the whole branch suite can be driven with stub seams and no filesystem, git, or tmux access.
// FileIssue matches selfreportengine.CreateIssue's own shape and is filled with that function in
// production.
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
	// FileIssue files one GitHub issue, matching selfreportengine.CreateIssue's own signature.
	FileIssue func(title string, body *string, labels []string) (url string, number int, err error)
}

// stepHandoffMarker is the machine-local record `lyx loom step` writes after every completed step:
// the persisted history length and state exactly as that step left them. It exists because a
// completed step's on-disk aftermath -- state running, run lock free, history non-empty -- is
// byte-identical to a mid-run driver death, and without this marker the next drive's Tier-1 entry
// observation filed a spurious crash-resume issue for every operator handing a supervised task to
// a driver (crucible round 2, R2-F1).
type stepHandoffMarker struct {
	HistoryLength int    `json:"history_length"`
	State         string `json:"state"`
}

// recordStepHandoff writes the clean-handoff marker for a step that just completed with
// historyLength persisted entries in persistedState. A write failure is warned and nothing else:
// the marker only narrows a false positive, so losing one write costs at most one spurious issue,
// exactly the trade the filed-title marker already accepts.
func recordStepHandoff(path, lockPath string, historyLength int, persistedState shedengine.State) {
	marker := stepHandoffMarker{HistoryLength: historyLength, State: string(persistedState)}
	if err := state.WriteJSON(path, lockPath, marker); err != nil {
		logger.Warn("loomcli: could not record the step clean-handoff marker", "path", path, "cause", err)
	}
}

// consumeStepHandoffMatch reads the clean-handoff marker, DELETES it, and reports whether it
// matches the observed history length and state. The delete is the point, not tidying: the marker
// is a one-shot voucher for exactly one drive entry. Consuming it there means a driver that
// resumes from a step handoff and then itself dies before appending any history -- an observation
// otherwise identical to the handoff -- is correctly reported as a crash by the drive after it,
// instead of being suppressed forever by a marker nothing invalidated.
// A missing or unreadable marker reports false; a delete failure is warned and does not change the
// result, since a lingering marker can at worst suppress one further matching observation and the
// warn names it.
func consumeStepHandoffMatch(path, lockPath string, historyLength int, observedState shedengine.State) bool {
	marker, found, err := state.ReadJSONStrict[stepHandoffMarker](path, lockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the step clean-handoff marker; treating it as absent", "path", path, "cause", err)
		return false
	}
	if !found {
		return false
	}
	if err := os.Remove(path); err != nil {
		logger.Warn("loomcli: could not consume the step clean-handoff marker; a later matching observation may be suppressed once more", "path", path, "cause", err)
	}
	return marker.HistoryLength == historyLength && marker.State == string(observedState)
}

// observeEntry probes the run lock non-blockingly and, on the reading path, reads the status file,
// in that order -- an order that is load-bearing and must not be swapped. Probing first is what
// makes the residual self-correcting: Shed persists its terminal state before Run returns and the
// deferred release follows, so a driver that finishes between the two steps has already written
// done, and the read that follows sees done rather than running.
// After the read it consumes the step clean-handoff marker (stepHandoffMarker) and reports the
// match on the observation, so DetectCrashResume can exclude an operator's step-to-driver handoff.
// When enabled is false it returns the zero observation immediately, performing no probe and no
// read -- a disabled run must pay for neither, and the marker is deliberately left unconsumed,
// since nothing on a disabled run will act on it.
// Any probe or read failure degrades to a logger.Warn and a zero observation with Observed false;
// nothing propagates.
func observeEntry(enabled bool, runLockPath, statusPath, statusLockPath, stepHandoffPath, stepHandoffLockPath string) loomengine.EntryObservation {
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
		Observed:         true,
		RunLockHeld:      runLockHeld,
		State:            shed.State,
		CurrentProducer:  shed.CurrentProducer,
		HistoryLength:    len(shed.History),
		Slug:             product.Slug,
		Parent:           product.Parent,
		CleanStepHandoff: consumeStepHandoffMatch(stepHandoffPath, stepHandoffLockPath, len(shed.History), shed.State),
	}
}

// detectAndFileAnomalies owns all three skips itself, checked before anything is read: first, a
// disabled knob returns immediately, before the status file, before the marker; second, a busy shed.Run error returns immediately, because that return means this
// process never ran the machine and never owned the run lock, so anything it observed at entry
// belongs to a live driver; third, a done context returns -- but only after one exception, filing
// a lone crash-resume anomaly when deps.Entry reports one, because the cancellation arm persists
// the paused state and the in-memory crash observation would otherwise be lost for good.
// Past the three skips, the ordinary filing pass re-reads the final status from the status file --
// never from the value shed.Run returned, which is documented meaningless whenever the returned
// error is non-nil, and this step runs on that path too.
// Every path past the skips reaches the filing pass;
// a status file that is missing, unreadable or carries an undecodable product reaches it with no new anomalies.
// So the anomalies an earlier pass failed to file and kept pending in the marker are retried on every pass the skips let through.
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

	runFilingPass(deps, detectFinalAnomalies(deps))
}

// detectFinalAnomalies reads the final status from deps.StatusPath and returns the anomalies the detector finds in it.
// A status file that is missing, unreadable or carries an undecodable product yields no anomalies, the last two warned.
func detectFinalAnomalies(deps selfreportDeps) []loomengine.Anomaly {
	final, found, err := state.ReadJSONStrict[shedengine.Status](deps.StatusPath, deps.StatusLockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the final status for self-report detection", "path", deps.StatusPath, "cause", err)
		return nil
	}
	if !found {
		return nil
	}

	var product loomengine.Status
	if len(final.Product) > 0 {
		if uerr := json.Unmarshal(final.Product, &product); uerr != nil {
			logger.Warn("loomcli: could not decode the product for self-report detection", "path", deps.StatusPath, "cause", uerr)
			return nil
		}
	}

	return loomengine.DetectAnomalies(deps.Entry, final, product)
}

// runFilingPass performs the ordered steps of the filing pass.
// It reads the marker and returns without a write when the marker holds no pending entry and there is no new anomaly.
// It then retries every pending entry in recorded order with its stored body:
// a success moves the title from Pending to Titles and writes the marker at once,
// and a failure warns and leaves the entry pending.
// It then collapses the new anomalies by title, skips a title already recorded or pending, and files one filing-seam call per surviving anomaly in the slice's deterministic order.
// A success records the title and writes the marker, never in advance and never in one batch at the end;
// a failure appends the title and its rendered body to Pending and writes the marker at once,
// so the anomaly is retried on every later pass even when no later detection finds it again.
func runFilingPass(deps selfreportDeps, anomalies []loomengine.Anomaly) {
	collapsed := collapseAnomaliesByTitle(anomalies)
	marker := readFiledMarker(deps.MarkerPath, deps.MarkerLockPath)
	if len(marker.Pending) == 0 && len(collapsed) == 0 {
		return
	}

	for _, p := range slices.Clone(marker.Pending) {
		body := p.Body
		if _, _, err := deps.FileIssue(p.Title, &body, selfreportengine.DefaultLabels()); err != nil {
			logger.Warn("loomcli: could not file a pending self-report issue; it stays pending", "title", p.Title, "cause", err)
			continue
		}

		marker.settle(p.Title)
		writeFiledMarker(deps.MarkerPath, deps.MarkerLockPath, marker)
	}

	for _, a := range collapsed {
		if marker.has(a.Title) || marker.isPending(a.Title) {
			continue
		}

		body := loomengine.RenderAnomalyBody(a)
		if _, _, err := deps.FileIssue(a.Title, &body, selfreportengine.DefaultLabels()); err != nil {
			logger.Warn("loomcli: could not file a self-report issue for a detected loom anomaly; it is kept pending", "title", a.Title, "cause", err)
			marker.Pending = append(marker.Pending, pendingAnomaly{Title: a.Title, Body: body})
			writeFiledMarker(deps.MarkerPath, deps.MarkerLockPath, marker)
			continue
		}

		marker.record(a.Title)
		writeFiledMarker(deps.MarkerPath, deps.MarkerLockPath, marker)
	}
}

// collapseAnomaliesByTitle reduces anomalies to one anomaly per distinct title, keeping the first
// occurrence, in the input slice's own order of first appearance.
// One detection returns at most one anomaly per title shape: one loomengine.DetectAnomalies call
// returns at most one crash-resume and at most one halt-kind anomaly, whose title shapes collide
// neither with each other nor with any other.
// A duplicate is therefore unreachable and is discarded silently, with deliberately no guard.
// If a future change to the detector's output makes it reachable, the symptom is a dropped anomaly
// with nothing reported anywhere, so the fix then is to add the guard rather than to widen this
// comment.
func collapseAnomaliesByTitle(anomalies []loomengine.Anomaly) []loomengine.Anomaly {
	order := make([]string, 0, len(anomalies))
	byTitle := make(map[string]loomengine.Anomaly, len(anomalies))

	for _, a := range anomalies {
		if _, ok := byTitle[a.Title]; ok {
			continue
		}
		byTitle[a.Title] = a
		order = append(order, a.Title)
	}

	collapsed := make([]loomengine.Anomaly, 0, len(order))
	for _, title := range order {
		collapsed = append(collapsed, byTitle[title])
	}
	return collapsed
}

// pendingAnomaly is an anomaly whose filing failed: its title and the body rendered when it was detected, kept so a later pass can file it without detecting it again.
type pendingAnomaly struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// selfreportFiledMarker is the machine-local record of which anomaly titles have already been filed as GitHub issues, and of the anomalies whose filing failed and awaits a retry.
// Titles stays a dumb string set with no parsing of its own -- dedupe granularity is whatever the title shape already encodes.
// Pending holds the failed filings in the order they failed, each retried on every later pass until it succeeds;
// a marker written before Pending existed reads as one with no pending entries.
// A title is in at most one of Titles and Pending.
type selfreportFiledMarker struct {
	Titles  []string         `json:"titles"`
	Pending []pendingAnomaly `json:"pending,omitempty"`
}

// has reports whether title is already recorded in m.
func (m selfreportFiledMarker) has(title string) bool {
	return slices.Contains(m.Titles, title)
}

// isPending reports whether title is waiting in m's pending list.
func (m selfreportFiledMarker) isPending(title string) bool {
	return slices.ContainsFunc(m.Pending, func(p pendingAnomaly) bool { return p.Title == title })
}

// settle moves title from m's pending list to its recorded set.
func (m *selfreportFiledMarker) settle(title string) {
	m.Pending = slices.DeleteFunc(m.Pending, func(p pendingAnomaly) bool { return p.Title == title })
	m.record(title)
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
