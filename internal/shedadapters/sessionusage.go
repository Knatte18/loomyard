// sessionusage.go writes a session's usage record under a run's usage directory when the session ends:
// its tokens split by kind and model, priced from its transcripts when the engine reads them.
// It also builds the session-end hook that records each session of a run's worktree under the run's current row.

package shedadapters

import (
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// modelTally is one model's token counts within a session and the cost of its priced messages.
// unpriced is true once any of its messages with usage had no known price.
type modelTally struct {
	counts   shedrun.ModelUsage
	costUSD  float64
	unpriced bool
}

// RecordSessionUsage writes the usage record of the session that end describes into dir, the run's usage directory.
// row is the run row that spawned the session.
// With a TranscriptReader in reads, it locates the session by end.SessionID and reads it with its forks.
// It adds each message's four counts to its model's total and prices the message at its own model through reads.Pricer.
// A message whose model has no price, or no pricer at all, leaves the cost unknown.
// The record's model is the one most of the main transcript's messages ran on,
// and its models list is set only when more than one model ran.
// With no TranscriptReader, or when locating or reading any transcript fails, the counts come from end.Usage;
// each unknown when it is not split, and the cost is unknown.
// A locate or read failure is logged at Warn.
// A failed write is logged at Warn.
// It returns nothing and stops nothing.
func RecordSessionUsage(dir, row string, end shuttleengine.SessionEnd, reads shuttleengine.RecordReads) {
	record := shedrun.SessionUsageRecord{
		SessionID: end.SessionID,
		Row:       row,
		Role:      end.Role,
		Model:     end.Model,
		Engine:    end.Engine,
		Started:   formatUsageTime(end.StartedAt),
		Ended:     formatUsageTime(end.EndedAt),
	}
	if !fillFromTranscripts(&record, end, reads) {
		fillFromSessionUsage(&record, end.Usage)
	}
	if err := shedrun.WriteSessionUsage(dir, record); err != nil {
		logger.Warn("shedadapters: write the session usage record failed (non-fatal)", "dir", dir, "sessionID", end.SessionID, "error", err)
	}
}

// UsageRecorder returns the session-end hook of the run in l's worktree.
// Each ended session is recorded into the run's usage directory under the run's current row.
// The row is read from the run's status file when the session ends,
// and it is empty when the status is absent or unreadable.
// A session's transcripts are read with the capabilities readers holds for its engine;
// an engine with no entry is recorded from the counts shuttle read, its cost unknown.
func UsageRecorder(l *lymcwd.Location, readers map[string]shuttleengine.RecordReads) func(shuttleengine.SessionEnd) {
	return func(end shuttleengine.SessionEnd) {
		RecordSessionUsage(shedrun.UsageDir(l), currentRunRow(l), end, readers[end.Engine])
	}
}

// currentRunRow returns the producer the run in l's worktree is at.
// It returns "" when the status file is absent or does not read;
// an unreadable status is logged at Warn.
func currentRunRow(l *lymcwd.Location) string {
	status, _, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(l, shedrun.SelfRunID), shedrun.StatusLock(l, shedrun.SelfRunID))
	if err != nil {
		logger.Warn("shedadapters: read the run status for the session usage row failed; recording an empty row", "error", err)
		return ""
	}
	return status.CurrentProducer
}

// formatUsageTime renders t as RFC 3339 UTC, and "" for the zero time.
func formatUsageTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// fillFromSessionUsage sets record's counts from usage, each nil unless the provider read them split;
// the cost is left unknown.
func fillFromSessionUsage(record *shedrun.SessionUsageRecord, usage shuttleengine.SessionUsage) {
	if !usage.SplitKnown {
		return
	}
	record.InputTokens, record.CacheWriteTokens, record.CacheReadTokens, record.OutputTokens = &usage.Input, &usage.CacheWrite, &usage.CacheRead, &usage.Output
}

// fillFromTranscripts sets record's counts, cost, model and models list from the session's transcripts.
// It reports false, leaving record untouched, when the engine reads no transcripts.
// It also reports false when the session cannot be located or read, or when no message carries usage.
func fillFromTranscripts(record *shedrun.SessionUsageRecord, end shuttleengine.SessionEnd, reads shuttleengine.RecordReads) bool {
	if reads.Transcripts == nil {
		return false
	}
	located, err := reads.Transcripts.LocateSessions(end.Worktree)
	if err != nil {
		logger.Warn("shedadapters: locate the session transcripts failed; taking the counts shuttle read", "sessionID", end.SessionID, "worktree", end.Worktree, "error", err)
		return false
	}
	var session shuttleengine.LocatedSession
	found := false
	for _, candidate := range located {
		if candidate.ID == end.SessionID || strings.TrimSuffix(candidate.ID, path.Ext(candidate.ID)) == end.SessionID {
			session, found = candidate, true
			break
		}
	}
	if !found {
		logger.Warn("shedadapters: no transcript found for the session; taking the counts shuttle read", "sessionID", end.SessionID, "worktree", end.Worktree)
		return false
	}
	read, err := reads.Transcripts.ReadSession(session)
	if err != nil {
		logger.Warn("shedadapters: read the session transcripts failed; taking the counts shuttle read", "sessionID", end.SessionID, "error", err)
		return false
	}

	tallies, mainMessages := tallyTranscripts(read, reads.Pricer)
	if len(tallies) == 0 {
		return false
	}
	models := make([]string, 0, len(tallies))
	for model := range tallies {
		models = append(models, model)
	}
	sort.Strings(models)

	var input, cacheWrite, cacheRead, output int64
	var totalCost float64
	allPriced := true
	perModel := make([]shedrun.ModelUsage, 0, len(models))
	for _, model := range models {
		tally := tallies[model]
		input += tally.counts.InputTokens
		cacheWrite += tally.counts.CacheWriteTokens
		cacheRead += tally.counts.CacheReadTokens
		output += tally.counts.OutputTokens
		entry := tally.counts
		if tally.unpriced {
			allPriced = false
		} else {
			totalCost += tally.costUSD
			cost := tally.costUSD
			entry.CostUSD = &cost
		}
		perModel = append(perModel, entry)
	}

	record.InputTokens, record.CacheWriteTokens, record.CacheReadTokens, record.OutputTokens = &input, &cacheWrite, &cacheRead, &output
	if allPriced {
		record.CostUSD = &totalCost
	}
	if len(perModel) > 1 {
		record.Models = perModel
	}
	if model, ok := dominantModel(mainMessages); ok {
		record.Model = model
	}
	return true
}

// tallyTranscripts sums the usage of every message of read, forks included, by model.
// It prices each message at its own model.
// A message with no tokens costs nothing whatever its model.
// mainMessages counts the main transcript's messages carrying usage by model.
func tallyTranscripts(read shuttleengine.SessionRead, pricer shuttleengine.MessagePricer) (tallies map[string]*modelTally, mainMessages map[string]int) {
	tallies = map[string]*modelTally{}
	mainMessages = map[string]int{}
	add := func(transcript shuttleengine.TranscriptRead, isMain bool) {
		for _, entry := range transcript.Entries {
			if entry.Usage == nil {
				continue
			}
			tally := tallies[entry.Model]
			if tally == nil {
				tally = &modelTally{counts: shedrun.ModelUsage{Model: entry.Model}}
				tallies[entry.Model] = tally
			}
			usage := *entry.Usage
			tally.counts.InputTokens += int64(usage.Input)
			tally.counts.CacheWriteTokens += int64(usage.CacheWrite)
			tally.counts.CacheReadTokens += int64(usage.CacheRead)
			tally.counts.OutputTokens += int64(usage.Output)
			if isMain {
				mainMessages[entry.Model]++
			}
			if usage.Input+usage.CacheWrite+usage.CacheRead+usage.Output == 0 {
				continue
			}
			if pricer == nil {
				tally.unpriced = true
				continue
			}
			usd, priced := pricer.PriceMessage(entry.Model, usage)
			if !priced {
				tally.unpriced = true
				continue
			}
			tally.costUSD += usd
		}
	}
	add(read.Main, true)
	for _, fork := range read.Forks {
		add(fork, false)
	}
	return tallies, mainMessages
}

// dominantModel returns the model with the most messages, the first by name among equals, and false when counts is empty.
func dominantModel(counts map[string]int) (string, bool) {
	best, bestCount := "", 0
	for model, count := range counts {
		if count > bestCount || (count == bestCount && model < best) {
			best, bestCount = model, count
		}
	}
	return best, bestCount > 0
}
