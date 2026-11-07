// entryobservation.go implements a drive's entry observation, a read of the run lock and status file.
// It also writes the crash-resume friction note and holds the handoff voucher for step-to-driver handoffs;
// its functions take paths as arguments, so every branch is reachable with no tmux, no git, and no real run.

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

// handoffVoucher is the machine-local record `lym loom step` writes after every step that returns;
// `lym loom start` writes one too, before it spawns a driver, and a step writes one with or without an error.
// It holds the persisted history length and state exactly as that step or spawn left them.
// A completed step leaves the same on-disk state as a mid-run driver death:
// running, run lock free, history non-empty.
// So without it, the next drive would read each operator handoff to a driver as a crash-resume.
type handoffVoucher struct {
	HistoryLength int    `json:"history_length"`
	State         string `json:"state"`
}

// recordHandoffVoucher writes the handoff voucher, replacing any earlier one.
// It records historyLength persisted entries and persistedState for a step that returned, with or without an error;
// a driver spawn about to start is recorded the same way.
// A write failure is warned and nothing else:
// the voucher only narrows a false positive, so losing one write costs at most one spurious crash-resume reading.
// The voucher suppresses at most one entry observation, the first;
// it does so only when its history length and state equal what was recorded.
// A voucher whose spawn then fails its readiness check stays until the next start or completed step overwrites it;
// until then it may suppress one later matching observation.
// A driver spawned that then dies mid-run is not noted, and its evidence stays in the driver log and trace.
func recordHandoffVoucher(path, lockPath string, historyLength int, persistedState shedengine.State) {
	voucher := handoffVoucher{HistoryLength: historyLength, State: string(persistedState)}
	if err := state.WriteJSON(path, lockPath, voucher); err != nil {
		logger.Warn("loomcli: could not record the handoff voucher", "path", path, "cause", err)
	}
}

// recordHandoffVoucherFromStatus records the handoff voucher from the status file.
// It serves a step that returned an error after persisting a transition:
// a step process that returns at all did not crash,
// so what it left on disk is vouched for like a clean step's.
// It reads the status file strictly and warns, recording nothing, when the read fails or the file is absent.
func recordHandoffVoucherFromStatus(path, lockPath, statusPath, statusLockPath string) {
	st, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
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

// consumeHandoffVoucher reads the handoff voucher and deletes it.
// It returns what it read and whether it matches the observed history length and state.
// The delete is the point, not tidying: the voucher is good for exactly one drive entry.
// Consuming it here lets the next drive see a crash that the handoff would otherwise hide.
// A driver resuming from a step handoff that dies before appending any history looks identical to the handoff;
// keeping the voucher would hide that crash forever, since nothing would invalidate it.
// A missing or unreadable voucher reports found false and no match;
// a delete failure is warned and does not change the result;
// a lingering voucher can at worst suppress one further matching observation, and the warning names it.
func consumeHandoffVoucher(path, lockPath string, historyLength int, observedState shedengine.State) (voucher handoffVoucher, found, matches bool) {
	voucher, found, err := state.ReadJSON[handoffVoucher](path, lockPath)
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

// observeEntry probes the run lock non-blockingly and, on the reading path, reads the status file, in that order.
// The order is load-bearing and must not be swapped.
// Probing first makes the residual self-correcting:
// Shed persists its terminal state before Run returns, and the deferred release follows,
// so a driver that finishes between the two steps has already written done, and the read sees done rather than running.
// After the read it consumes the handoff voucher and reports the match on the observation.
// DetectCrashResume uses that match to exclude an operator's step-to-driver handoff.
// When enabled is false it returns the zero observation immediately, performing no probe and no read.
// A disabled run pays for neither and leaves the voucher unconsumed, because nothing on a disabled run acts on it.
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

	shed, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
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

// writeCrashResumeNote records a detected crash-resume as a friction note named `loom-crash-resume`.
// The note names the verb (`run` or `step`) and the trace file of this invocation.
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
