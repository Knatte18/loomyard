// bouncerstalemate.go holds the Bouncer's stalemate check.
// It records the review target's id as a round's fix left it, and judges whether a round left the target unchanged.
// It counts the unchanged rounds in a row against BouncerConfig.StalemateRounds.

package shedadapters

import (
	"os"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// recordPostFixTargetID records the target's id as round's fix left it, when the round's record holds none.
// The id comes from a copy of the configured Snapshot whose scope and paths are the round record's.
// A later settle of the round, a replay included, reads the recorded id, so a target edited since changes no verdict.
// A missing record, an unset Snapshot, a failed id read and a failed write record nothing.
// The failures are logged, and the round counts as changed.
func (b *Bouncer) recordPostFixTargetID(round int) {
	if b.cfg.Snapshot == nil {
		return
	}
	record, found, err := readRoundSnapshot(b.cfg.RunDir, round)
	if err != nil {
		logger.Warn("shedadapters: bouncer could not read the round's snapshot record; no post-fix target id recorded", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "error", err)
		return
	}
	if !found || record.PostFixTargetID != "" {
		return
	}
	snapshot := *b.cfg.Snapshot
	snapshot.Scope, snapshot.Paths = burlerengine.FixScope(record.FixScope), record.TargetPaths
	id, err := snapshot.ID()
	if err != nil {
		logger.Warn("shedadapters: bouncer could not read the target's id after the round's fix; the round counts as changed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "error", err)
		return
	}
	err = updateRoundSnapshot(b.cfg.RunDir, round, func(rec *roundSnapshot, exists bool) {
		if exists && rec.PostFixTargetID == "" {
			rec.PostFixTargetID = id
		}
	})
	if err != nil {
		logger.Warn("shedadapters: bouncer could not record the target's id after the round's fix; the round counts as changed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "error", err)
	}
}

// roundUnchanged reports whether round's fix left the review target as the round found it.
// It holds only when the round's record holds equal target and post-fix ids,
// and a fixer that was spawned answered every finding of the round's review in its fixer-report.
// A round with no record or no post-fix id, a skipped fixer,
// and an unparseable or incomplete report all count as changed.
func (b *Bouncer) roundUnchanged(round int) bool {
	record, found, err := readRoundSnapshot(b.cfg.RunDir, round)
	if err != nil || !found || record.TargetID == "" || record.TargetID != record.PostFixTargetID || record.DecisionsRouted == nil {
		return false
	}
	content, err := os.ReadFile(roundFixerReportPath(b.cfg.RunDir, round))
	if err != nil {
		return false
	}
	report, err := burlerengine.ParseFixerReport(content)
	if err != nil || report.Spawned != nil {
		return false
	}
	findings, err := b.reviewFindings(round)
	if err != nil {
		return false
	}
	return burlerengine.CheckFixerReport(report, findings, *record.DecisionsRouted) == nil
}

// stalemated reports whether round escalates with cause stalemate.
// A round whose own escalation record holds that cause always does.
// So a replay escalates again, whatever StalemateRounds now is.
// A round whose record holds another cause, or a record that cannot be read, never does.
// Otherwise it needs StalemateRounds above 0.
// In an open window it yields when the round's ledger holds no design-wrong entry or cannot be read,
// so the soft-cap rule settles the round as it would without this check.
// Last, it holds when the unchanged rounds in a row ending at round number at least StalemateRounds.
// The count stops at the first changed round.
// It also stops at the latest earlier round whose escalation record holds cause stalemate.
func (b *Bouncer) stalemated(round int, windowOpen bool) bool {
	if cause, _, exists, err := readEscalation(b.cfg.RunDir, round); err != nil || exists {
		return err == nil && cause == EscalationStalemate
	}
	if b.cfg.StalemateRounds <= 0 {
		return false
	}
	if windowOpen {
		ledger, err := readLedger(b.cfg.RunDir, round)
		if err != nil || len(designWrongKeys(ledger)) == 0 {
			return false
		}
	}
	unchanged := 0
	for n := round; n >= 1; n-- {
		if n < round {
			if cause, _, exists, err := readEscalation(b.cfg.RunDir, n); err == nil && exists && cause == EscalationStalemate {
				break
			}
		}
		if !b.roundUnchanged(n) {
			break
		}
		unchanged++
	}
	return unchanged >= b.cfg.StalemateRounds
}
