// entryobservation.go implements loom's entry observation -- the told-input read of the run lock and status file that lets a crash-resume be recognised -- the friction note a recognised one leaves, and the handoff voucher that keeps an operator's step-to-driver handoff from reading as one.
// Both are functions of told paths, kept here rather than inline in drive's cobra closure so every branch is reachable with no tmux, no git, and no real run.

package loomcli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// handoffVoucher is the machine-local record `lyx loom step` writes after every step that returns, with or without an error,
// and `lyx loom start` writes right before it spawns a driver:
// the persisted history length and state exactly as that step or spawn left them.
// It exists because a completed step's on-disk aftermath -- state running, run lock free, history non-empty -- is byte-identical to a mid-run driver death,
// and without this voucher the next drive's entry observation would read as a crash-resume for every operator handing a supervised task to a driver (crucible round 2, R2-F1).
type handoffVoucher struct {
	HistoryLength int    `json:"history_length"`
	State         string `json:"state"`
}

// recordHandoffVoucher writes the handoff voucher, replacing any earlier one, for a step that just
// returned, with or without an error, or a driver spawn that is about to start, with historyLength persisted entries in persistedState.
// A write failure is warned and nothing else:
// the voucher only narrows a false positive, so losing one write costs at most one spurious crash-resume reading.
// The voucher suppresses at most one entry observation, the first, and only when its history length and state equal what was recorded.
// A voucher whose spawn then fails its readiness check stays until the next start or completed step overwrites it, and suppresses at most one later matching observation;
// a driver start spawned that then dies mid-run is not noted, and its evidence stays in the driver log and trace.
func recordHandoffVoucher(path, lockPath string, historyLength int, persistedState shedengine.State) {
	voucher := handoffVoucher{HistoryLength: historyLength, State: string(persistedState)}
	if err := state.WriteJSON(path, lockPath, voucher); err != nil {
		logger.Warn("loomcli: could not record the handoff voucher", "path", path, "cause", err)
	}
}

// recordHandoffVoucherFromStatus records the handoff voucher from what the status file holds, for a step that returned an error after persisting a transition:
// a step process that returns at all did not crash,
// so what it left on disk is vouched for like a clean step's.
// It reads the status file strictly and warns, recording nothing, when the read fails or the file is absent.
func recordHandoffVoucherFromStatus(path, lockPath, statusPath, statusLockPath string) {
	st, found, err := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the status file to record the handoff voucher", "path", statusPath, "cause", err)
		return
	}
	if !found {
		logger.Warn("loomcli: no status file to record the handoff voucher from", "path", statusPath)
		return
	}
	recordHandoffVoucher(path, lockPath, len(st.History), st.State)
}

// consumeHandoffVoucher reads the handoff voucher, DELETES it, and returns what it read together with whether it
// matches the observed history length and state. The delete is the point, not tidying: the voucher
// is good for exactly one drive entry. Consuming it there means a driver that
// resumes from a step handoff and then itself dies before appending any history -- an observation
// otherwise identical to the handoff -- is correctly reported as a crash by the drive after it,
// instead of being suppressed forever by a voucher nothing invalidated.
// A missing or unreadable voucher reports found false and no match;
// a delete failure is warned and does not change the result,
// since a lingering voucher can at worst suppress one further matching observation and the warn names it.
func consumeHandoffVoucher(path, lockPath string, historyLength int, observedState shedengine.State) (voucher handoffVoucher, found, matches bool) {
	voucher, found, err := state.ReadJSONStrict[handoffVoucher](path, lockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the handoff voucher; treating it as absent", "path", path, "cause", err)
		return handoffVoucher{}, false, false
	}
	if !found {
		return handoffVoucher{}, false, false
	}
	if err := os.Remove(path); err != nil {
		logger.Warn("loomcli: could not consume the handoff voucher; a later matching observation may be suppressed once more", "path", path, "cause", err)
	}
	return voucher, true, voucher.HistoryLength == historyLength && voucher.State == string(observedState)
}

// observeEntry probes the run lock non-blockingly and, on the reading path, reads the status file,
// in that order -- an order that is load-bearing and must not be swapped. Probing first is what
// makes the residual self-correcting: Shed persists its terminal state before Run returns and the
// deferred release follows, so a driver that finishes between the two steps has already written
// done, and the read that follows sees done rather than running.
// After the read it consumes the handoff voucher (handoffVoucher) and reports the
// match on the observation, so DetectCrashResume can exclude an operator's step-to-driver handoff.
// When enabled is false it returns the zero observation immediately, performing no probe and no
// read -- a disabled run must pay for neither, and the voucher is deliberately left unconsumed,
// since nothing on a disabled run will act on it.
// Any probe or read failure degrades to a logger.Warn and a zero observation with Observed false;
// nothing propagates.
func observeEntry(enabled bool, runLockPath, statusPath, statusLockPath, handoffVoucherPath, handoffVoucherLockPath string) loomengine.EntryObservation {
	if !enabled {
		return loomengine.EntryObservation{}
	}

	probe, free, err := lock.TryAcquireWriteLock(runLockPath)
	if err != nil {
		logger.Warn("loomcli: could not probe the run lock for the entry observation", "path", runLockPath, "cause", err)
		return loomengine.EntryObservation{}
	}
	runLockHeld := !free
	if free {
		_ = probe.Release()
	}

	shed, found, err := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		logger.Warn("loomcli: could not read the status file for the entry observation", "path", statusPath, "cause", err)
		return loomengine.EntryObservation{}
	}
	if !found {
		return loomengine.EntryObservation{}
	}

	var product loomengine.Status
	if len(shed.Product) > 0 {
		if uerr := json.Unmarshal(shed.Product, &product); uerr != nil {
			logger.Warn("loomcli: could not decode the product for the entry observation", "path", statusPath, "cause", uerr)
			return loomengine.EntryObservation{}
		}
	}

	voucher, voucherFound, vouched := consumeHandoffVoucher(handoffVoucherPath, handoffVoucherLockPath, len(shed.History), shed.State)

	return loomengine.EntryObservation{
		Observed:             true,
		RunLockHeld:          runLockHeld,
		State:                shed.State,
		CurrentProducer:      shed.CurrentProducer,
		HistoryLength:        len(shed.History),
		Slug:                 product.Slug,
		Parent:               product.Parent,
		Vouched:              vouched,
		VoucherFound:         voucherFound,
		VoucherHistoryLength: voucher.HistoryLength,
		VoucherState:         shedengine.State(voucher.State),
		History:              shed.History,
	}
}

// writeCrashResumeNote records a detected crash-resume as a friction note named `loom-crash-resume`, for the named verb (`run` or `step`) and the trace file of this invocation.
// It is a no-op when frictionDir is empty (Tier 2 off) or when friction.NotePath rejects the id.
func writeCrashResumeNote(frictionDir string, entry loomengine.EntryObservation, verb, traceFile string) error {
	if frictionDir == "" {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "loom-crash-resume")
	if path == "" {
		return nil
	}

	var b strings.Builder
	b.WriteString("loom resumed after a suspected driver crash at " + entry.CurrentProducer + "\n\n")
	b.WriteString("slug: " + entry.Slug + "\n")
	b.WriteString("parent: " + entry.Parent + "\n")
	b.WriteString("state: " + string(entry.State) + "\n")
	b.WriteString("current_producer: " + entry.CurrentProducer + "\n")
	b.WriteString("history_entries: " + strconv.Itoa(entry.HistoryLength) + "\n")
	b.WriteString("run_lock_held: " + strconv.FormatBool(entry.RunLockHeld) + "\n")
	if entry.VoucherFound {
		b.WriteString("voucher_history_entries: " + strconv.Itoa(entry.VoucherHistoryLength) + "\n")
		b.WriteString("voucher_state: " + string(entry.VoucherState) + "\n")
	} else {
		b.WriteString("voucher: none\n")
	}
	b.WriteString("verb: " + verb + "\n")
	b.WriteString("trace_file: " + traceFile + "\n")
	writeHistoryRows(&b, entry.History)

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("loom: write crash-resume note %s: %w", path, err)
	}
	return nil
}
