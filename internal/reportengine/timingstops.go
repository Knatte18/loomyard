// timingstops.go lists a run's stops for the timing report and names who settled each: it reads the Go driver's settle records, joins them to the stops and falls back to the caller labels of the history.

package reportengine

import (
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// The kinds of stop a run is counted under.
const (
	// StopKindStop is a halt or awaiting stop as convergence lists it.
	StopKindStop = "stop"
	// StopKindBounce is a loom stuck entry an OnStuck edge bounced.
	StopKindBounce = "bounce"
	// StopKindBattenHalt is a halt of one of batten's own rows.
	StopKindBattenHalt = "batten-halt"
)

// The settlers a stop is credited to; a stop moved on by a caller the history labels takes that label instead.
const (
	SettledByJudge     = "judge"
	SettledByShed      = "shed"
	SettledByTackler   = "tackler"
	SettledByArbiter   = "arbiter"
	SettledByAlignment = "alignment"
	SettledByUnknown   = "unknown"
	SettledByOpen      = "open"
)

// TimingStop is one stop of a run and who settled it.
// At is nil for a stop with no time of its own, a halt note no batten change recorded.
type TimingStop struct {
	Kind     string     `json:"kind"`
	Producer string     `json:"producer"`
	State    string     `json:"state"`
	At       *time.Time `json:"at"`
	Settler  string     `json:"settler"`

	// changed marks a stop batten recorded as a child state change.
	changed bool

	// position is the index of the history entry the stop is read from: batten's for a batten-row halt, the loom's for a bounce and for a halt note no change recorded; -1 when none was found.
	position int
}

// readSettles reads the run's two settle-record files through shedrun.ParseSettleRecords: the loom run's at tag, and batten's at the commit holding the run's batten record.
// An absent file holds no record, and a line that does not decode is a finding naming the file and the line.
func (row *RunRow) readSettles(records Reader, tag gitrepo.TagRef) ([]Finding, error) {
	rels, err := shedrun.TreeRunRels(row.Slug)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	read := func(rev, label string) ([]shedrun.SettleRecord, error) {
		data, err := records.FileAtRevision(rev, rels.SettleRecords)
		if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reportengine: read %s at %s: %w", rels.SettleRecords, rev, err)
		}
		parsed, skipped := shedrun.ParseSettleRecords(data)
		for _, skip := range skipped {
			findings = append(findings, Finding{Tag: tag.Name, File: rels.SettleRecords, Error: label + skip.Error()})
		}
		return parsed, nil
	}

	if row.childSettles, err = read(tag.Commit, ""); err != nil {
		return nil, err
	}
	if row.battenCommit != "" {
		if row.battenSettles, err = read(row.battenCommit, "in batten's copy: "); err != nil {
			return nil, err
		}
	}
	return findings, nil
}

// stampPointer returns the pointer to the stamp of entry, nil when it has none.
func stampPointer(entry shedengine.HistoryEntry) *time.Time {
	stamp, ok := entryStamp(entry)
	if !ok {
		return nil
	}
	return &stamp
}

// runStops lists the run's stops and sets each one's settler.
// StopKindStop holds the halts convergence lists, so it is absent without the halt-note source: the k-th stop with a change takes the time of the k-th batten change into blocked, failed or awaiting, and a note-only stop has no time.
// StopKindBounce is a loom stuck entry an OnStuck edge bounced, and StopKindBattenHalt a halt of a batten row as battenRowHalts finds it.
// A settle record that matches no stop is a finding, except for the loom file when StopKindStop is absent.
func runStops(row RunRow, rows []timedRow) ([]TimingStop, []Finding) {
	var stops []TimingStop
	if row.Stops != nil {
		var changes []time.Time
		for _, change := range row.changes {
			switch change.To {
			case shedengine.StateAwaiting, shedengine.StateBlocked, shedengine.StateFailed:
				changes = append(changes, change.At)
			}
		}
		claimed := map[int]bool{}
		next := 0
		for _, held := range row.Stops {
			stop := TimingStop{Kind: StopKindStop, Producer: held.Producer, State: held.State, changed: held.Change, position: -1}
			if held.Change {
				if next < len(changes) && !changes[next].IsZero() {
					at := changes[next]
					stop.At = &at
				}
				next++
			} else {
				stop.position = unclaimedHaltEntry(row.childHistory, held.Producer, claimed)
			}
			stops = append(stops, stop)
		}
	}
	for i, entry := range row.childHistory {
		if entry.Outcome == shedengine.Stuck && entry.RoutedTo != "" {
			stops = append(stops, TimingStop{Kind: StopKindBounce, Producer: entry.Producer, State: string(entry.Outcome), At: stampPointer(entry), position: i})
		}
	}
	battenHistory := latestBattenHistory(row)
	for _, index := range battenRowHalts(battenHistory) {
		entry := battenHistory[index]
		state := string(shedengine.StateBlocked)
		if entry.Outcome == shedengine.Awaiting {
			state = string(shedengine.StateAwaiting)
		}
		stops = append(stops, TimingStop{Kind: StopKindBattenHalt, Producer: entry.Producer, State: state, At: stampPointer(entry), position: index})
	}

	childMatched, childFindings := matchChildRecords(stops, row)
	battenMatched, battenFindings := matchBattenRecords(stops, row)
	findings := append(childFindings, battenFindings...)
	for i := range stops {
		record, held := childMatched[i]
		if !held {
			record, held = battenMatched[i]
		}
		var matched *shedrun.SettleRecord
		if held {
			matched = &record
		}
		stops[i].Settler = settlerOf(stops[i], matched, row, rows)
	}
	return stops, findings
}

// unclaimedHaltEntry returns the index of the first entry of history that halted producer and that claimed has not marked, and marks it; -1 when none is left.
func unclaimedHaltEntry(history []shedengine.HistoryEntry, producer string, claimed map[int]bool) int {
	for i, entry := range history {
		if claimed[i] || entry.Producer != producer || (entry.Outcome != shedengine.Stuck && entry.Outcome != shedengine.Awaiting) {
			continue
		}
		claimed[i] = true
		return i
	}
	return -1
}

// namesStop reports whether the record's stop key holds stop's producer and state.
func namesStop(record shedrun.SettleRecord, stop TimingStop) bool {
	return record.StopKey.CurrentProducer == stop.Producer && record.StopKey.State == stop.State
}

// countedBefore reports whether the last history entry the record counts is stamped at or before at.
func countedBefore(history []shedengine.HistoryEntry, record shedrun.SettleRecord, at time.Time) bool {
	length := record.StopKey.HistoryLength
	if length < 1 || length > len(history) {
		return false
	}
	stamp, ok := entryStamp(history[length-1])
	return ok && !stamp.After(at)
}

// matchChildRecords matches the loom file's records to the stops of kind StopKindStop by index.
// A stop with a change and a producer takes the record naming its producer and state with the greatest history length among those whose last counted entry is stamped at or before the change, and every earlier such record folds into it.
// A note-only stop takes the earliest record naming its producer and state that no stop took.
// Any other record is a finding, which is left out when the run lists no StopKindStop.
func matchChildRecords(stops []TimingStop, row RunRow) (map[int]shedrun.SettleRecord, []Finding) {
	matched := map[int]shedrun.SettleRecord{}
	taken := make([]bool, len(row.childSettles))
	for i, stop := range stops {
		if stop.Kind != StopKindStop || !stop.changed || stop.At == nil || stop.Producer == "" {
			continue
		}
		best := -1
		var candidates []int
		for j, record := range row.childSettles {
			if !namesStop(record, stop) || !countedBefore(row.childHistory, record, *stop.At) {
				continue
			}
			candidates = append(candidates, j)
			if best < 0 || record.StopKey.HistoryLength > row.childSettles[best].StopKey.HistoryLength {
				best = j
			}
		}
		if best < 0 {
			continue
		}
		matched[i] = row.childSettles[best]
		for _, j := range candidates {
			taken[j] = true
		}
	}
	for i, stop := range stops {
		if stop.Kind != StopKindStop || stop.changed || stop.Producer == "" {
			continue
		}
		for j, record := range row.childSettles {
			if !taken[j] && namesStop(record, stop) {
				matched[i] = record
				taken[j] = true
				break
			}
		}
	}

	var findings []Finding
	if row.Stops != nil {
		findings = unmatchedRecords(row, row.childSettles, taken, "")
	}
	return matched, findings
}

// matchBattenRecords matches batten's file records to the batten-row halts by index.
// A record names a halt when its producer and state equal the halt's and its history length is the 1-based position of the halt's entry in batten's history; a record naming no halt is a finding.
func matchBattenRecords(stops []TimingStop, row RunRow) (map[int]shedrun.SettleRecord, []Finding) {
	matched := map[int]shedrun.SettleRecord{}
	taken := make([]bool, len(row.battenSettles))
	for i, stop := range stops {
		if stop.Kind != StopKindBattenHalt {
			continue
		}
		for j, record := range row.battenSettles {
			if !taken[j] && namesStop(record, stop) && record.StopKey.HistoryLength == stop.position+1 {
				matched[i] = record
				taken[j] = true
				break
			}
		}
	}
	return matched, unmatchedRecords(row, row.battenSettles, taken, "in batten's copy: ")
}

// unmatchedRecords returns a finding for each record taken has not marked, naming the settle-record file and the record's key.
func unmatchedRecords(row RunRow, records []shedrun.SettleRecord, taken []bool, label string) []Finding {
	file := ""
	if rels, err := shedrun.TreeRunRels(row.Slug); err == nil {
		file = rels.SettleRecords
	}
	var findings []Finding
	for j, record := range records {
		if !taken[j] {
			key := record.StopKey
			findings = append(findings, Finding{Tag: row.Tag, File: file, Error: fmt.Sprintf("%ssettle record at %s %s, history length %d, matches no stop", label, key.CurrentProducer, key.State, key.HistoryLength)})
		}
	}
	return findings
}

// settlerOf names who settled the stop, the first of these that holds:
// a bounce was settled by the judge;
// a batten automatic step accounting for it, or a go-rule restep or verify-retry record, make it the shed's;
// a tackler restep record makes it the tackler's;
// an applied arbiter attempt or alignment verdict spawned after the stop makes it the arbiter's or alignment's;
// else the by label of the first history entry that moved it on, unknown when that entry has none, and open when none follows in a run in flight.
// A record of any other settler or action settles nothing by itself.
func settlerOf(stop TimingStop, record *shedrun.SettleRecord, row RunRow, rows []timedRow) string {
	if stop.Kind == StopKindBounce {
		return SettledByJudge
	}
	if stop.Kind == StopKindStop && stop.At != nil {
		for _, step := range row.automaticSteps {
			if step.Producer == stop.Producer && step.At.Equal(*stop.At) {
				return SettledByShed
			}
		}
	}
	if record != nil {
		if record.Settler == shedrun.SettlerGoRule && (record.Action == shedrun.SettleRestep || record.Action == shedrun.SettleVerifyRetry) {
			return SettledByShed
		}
		if record.Settler == shedrun.SettlerTackler && record.Action == shedrun.SettleRestep {
			return SettledByTackler
		}
	}
	if stop.At != nil {
		for _, attempt := range row.arbiterRecords {
			if attempt.Applied && attempt.Attempt.StopKey.CurrentProducer == stop.Producer && attempt.Attempt.StopKey.State == stop.State && attempt.Attempt.SpawnedAt.After(*stop.At) {
				return SettledByArbiter
			}
		}
		for _, attempt := range row.alignmentAttempts {
			if stop.Producer == loomshed.NamePRGate && attempt.Point == alignment.PointPRGate && attempt.Applied != "" && attempt.SpawnedAt.After(*stop.At) {
				return SettledByAlignment
			}
		}
		for _, round := range row.parentRounds {
			if round.Alignment != nil && round.Alignment.State == parentreview.AlignmentApplied && round.Alignment.SpawnedAt.After(*stop.At) {
				return SettledByAlignment
			}
		}
	}
	return movedOnBy(stop, row, rows)
}

// movedOnBy returns the caller label of the first entry after the stop in the history that moved it on, batten's for a batten-row halt and the loom's otherwise.
// Batten's Run-Shed entries record state changes and never move a halt on, so they are skipped.
func movedOnBy(stop TimingStop, row RunRow, rows []timedRow) string {
	var mover *shedengine.HistoryEntry
	switch {
	case stop.Kind == StopKindBattenHalt:
		history := latestBattenHistory(row)
		for i := stop.position + 1; i < len(history); i++ {
			if history[i].Producer != battenrecipe.NameRunShed {
				mover = &history[i]
				break
			}
		}
	case stop.At != nil:
		for _, candidate := range rows {
			if !candidate.batten && candidate.stamped && candidate.stamp.After(*stop.At) {
				mover = &row.childHistory[candidate.position]
				break
			}
		}
	case stop.position >= 0 && stop.position+1 < len(row.childHistory):
		mover = &row.childHistory[stop.position+1]
	}

	switch {
	case mover == nil && row.inFlight:
		return SettledByOpen
	case mover == nil || mover.By == "":
		return SettledByUnknown
	case mover.By == shedengine.CallerShed:
		return SettledByShed
	}
	return mover.By
}

// settlersInWindow counts the stops of runs by kind and settler for one bucket window, each in the bucket its time falls in and a stop with no time in its run's closing bucket.
// last says window is the report's final bucket, which also holds a time equal to its close.
func settlersInWindow(runs []TimingRun, window Interval, last bool) map[string]map[string]int {
	settlers := map[string]map[string]int{}
	for _, run := range runs {
		for _, stop := range run.Stops {
			at := run.End
			if stop.At != nil {
				at = *stop.At
			}
			if at.Before(window.From) || at.After(window.To) || (at.Equal(window.To) && !last) {
				continue
			}
			if settlers[stop.Kind] == nil {
				settlers[stop.Kind] = map[string]int{}
			}
			settlers[stop.Kind][stop.Settler]++
		}
	}
	return settlers
}
