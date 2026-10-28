// activity.go implements shuttleengine.ActivityReader for Claude: the session's last activity, its forks included, and whether the newest main-chain turn end of the transcript a Stop payload names through transcript_path is an API error, and whether that error is the usage limit, with its reset time.
// It also implements shuttleengine.UsageLimitReader: whether Claude Code's usage-limit classification stopped that turn end or a fork during its turn.
// The session's last activity is the newest of the main transcript's last write and its fork transcripts' activity;
// a fork's pending tool call reads active up to bashMaxTimeout past its last write, and the API-error part reads the main transcript only.
// The marker Claude Code writes on a synthetic API-error assistant entry stays in this file, per the Shuttle Provider-Seam Invariant.
// Like the other transcript reads it degrades to the zero reading on every failure and never errors, since the transcript format is a Claude Code internal.
package claudeengine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var (
	_ shuttleengine.ActivityReader   = (*Claude)(nil)
	_ shuttleengine.UsageLimitReader = (*Claude)(nil)
)

// usageLimitError is the classification Claude Code sets in the error field of the synthetic API-error entry it writes when the account's usage limit refuses a request.
const usageLimitError = "rate_limit"

// apiErrorEntry is a transcript entry with the marker Claude Code sets on the synthetic assistant entry it writes for an API error, and that entry's error classification.
type apiErrorEntry struct {
	transcriptEntry
	IsAPIErrorMessage bool   `json:"isApiErrorMessage"`
	Error             string `json:"error"`
}

// isUsageLimit reports whether e is a synthetic API-error entry the usage limit caused.
func (e apiErrorEntry) isUsageLimit() bool {
	return e.Type == "assistant" && e.IsAPIErrorMessage && e.Error == usageLimitError
}

// TurnEndActivity reads the transcript turnEnd's transcript_path names.
// Its TranscriptModTime is the session's last activity, its forks included: the later of the transcript's modification time and forkActivity over the session's fork transcripts.
// The newest main-chain assistant entry that ends a turn or carries the API-error marker decides whether the session stands on an API error, with that entry's final text.
// A missing transcript_path or an unreadable file reads as the zero reading.
func (c *Claude) TurnEndActivity(turnEnd shuttleengine.Event) shuttleengine.TurnEndActivity {
	var payload struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if err := json.Unmarshal(turnEnd.Raw, &payload); err != nil || payload.TranscriptPath == "" {
		return shuttleengine.TurnEndActivity{}
	}
	f, err := os.Open(payload.TranscriptPath)
	if err != nil {
		return shuttleengine.TurnEndActivity{}
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return shuttleengine.TurnEndActivity{}
	}
	activity := shuttleengine.TurnEndActivity{TranscriptModTime: info.ModTime()}
	if forks := forkActivity(payload.TranscriptPath, c.now()); forks.After(activity.TranscriptModTime) {
		activity.TranscriptModTime = forks
	}
	readBackward(f, info.Size(), initialReadChunk, func(data []byte) bool {
		var found bool
		activity.APIError, activity.APIErrorText, activity.UsageLimit, found = newestTurnEndError(data)
		return found
	})
	return activity
}

// forkActivity returns the newest activity across the fork transcripts under the subagents directory beside transcriptPath, or the zero time when there is none.
// A fork last written no more than bashMaxTimeout before now that holds a pending tool call reads now, since a held foreground call writes nothing;
// any other fork reads its last write time.
// A fork transcript that cannot be read is skipped.
func forkActivity(transcriptPath string, now time.Time) time.Time {
	dir := filepath.Join(strings.TrimSuffix(transcriptPath, ".jsonl"), "subagents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}
	}
	var newest time.Time
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		reading := info.ModTime()
		if !now.After(reading.Add(bashMaxTimeout)) && holdsPendingToolCall(filepath.Join(dir, entry.Name())) {
			reading = now
		}
		if reading.After(newest) {
			newest = reading
		}
	}
	return newest
}

// holdsPendingToolCall reports whether the newest assistant message of the transcript at path that holds tool_use blocks has a tool_use no later tool_result names.
// It reads the transcript backward from its end, so its cost is bounded by the tail.
// A transcript with no tool use, and a read error, report false.
func holdsPendingToolCall(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	var pending bool
	readBackward(f, info.Size(), initialReadChunk, func(data []byte) bool {
		var settled bool
		pending, settled = newestToolCallingMessagePending(data)
		return settled
	})
	return pending
}

// newestToolCallingMessagePending scans the complete lines of data from the last to the first and reports whether the newest assistant message holding tool_use blocks has one that no tool_result in data names.
// A message is every assistant entry sharing one message id, since Claude writes each parallel call as its own entry.
// settled is true once the scan has passed every entry of that message, which is when an older assistant entry of another message turns up; false when data ends inside the message or holds no tool use, so a wider window may change the answer.
func newestToolCallingMessagePending(data []byte) (pending, settled bool) {
	called := map[string]bool{}
	answered := map[string]bool{}
	messageID := ""
	inMessage := false
	for end := len(data); end > 0 && !settled; {
		start := bytes.LastIndexByte(data[:end], '\n') + 1
		raw := bytes.TrimSpace(data[start:end])
		end = start - 1
		if len(raw) == 0 {
			continue
		}
		var line transcriptLine
		if err := json.Unmarshal(raw, &line); err != nil {
			continue
		}
		if inMessage && line.Type == "assistant" && line.Message.ID != messageID {
			settled = true
			break
		}
		for _, block := range line.Message.Content {
			switch {
			case block.Type == "tool_result":
				answered[block.ToolUseID] = true
			case block.Type == "tool_use" && line.Type == "assistant":
				if !inMessage {
					inMessage, messageID = true, line.Message.ID
				}
				called[block.ID] = true
			}
		}
	}
	for id := range called {
		if !answered[id] {
			return true, settled
		}
	}
	return false, settled
}

// newestTurnEndError scans the complete lines of data from the last to the first and reports whether the first main-chain assistant entry that ends a turn or carries the API-error marker is an API error, with its final text and its usage-limit reading.
// found is false when data holds no such entry.
func newestTurnEndError(data []byte) (apiError bool, text string, limit shuttleengine.UsageLimit, found bool) {
	for end := len(data); end > 0; {
		start := bytes.LastIndexByte(data[:end], '\n') + 1
		line := bytes.TrimSpace(data[start:end])
		end = start - 1
		if len(line) == 0 {
			continue
		}
		var e apiErrorEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.IsSidechain || e.Type != "assistant" || !(e.isTurnEnd() || e.IsAPIErrorMessage) {
			continue
		}
		if !e.IsAPIErrorMessage {
			return false, "", shuttleengine.UsageLimit{}, true
		}
		text = e.finalText()
		if e.isUsageLimit() {
			limit = shuttleengine.UsageLimit{Limited: true, ResetsAt: usageLimitResetsAt(text, e.Timestamp)}
		}
		return true, text, limit, true
	}
	return false, "", shuttleengine.UsageLimit{}, false
}

// TurnEndUsageLimited reads the transcript turnEnd's transcript_path names.
// The usage limit stopped the turn end when the newest main-chain assistant entry that ends a turn or carries the API-error marker is a usage-limit entry.
// It stopped a fork during the turn when a fork transcript beside it, last written after the main chain's previous turn end, has a usage-limit entry as its newest assistant entry.
// A transcript with no previous turn end counts every fork.
// A missing transcript_path and an unreadable transcript read as no limit.
func (c *Claude) TurnEndUsageLimited(turnEnd shuttleengine.Event) bool {
	var payload struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if err := json.Unmarshal(turnEnd.Raw, &payload); err != nil || payload.TranscriptPath == "" {
		return false
	}
	f, err := os.Open(payload.TranscriptPath)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	var reading turnLimitReading
	readBackward(f, info.Size(), initialReadChunk, func(data []byte) bool {
		var settled bool
		reading, settled = readTurnLimit(data)
		return settled
	})
	return reading.limited || forkStoppedOnUsageLimit(payload.TranscriptPath, reading.previousTurnEndAt)
}

// turnLimitReading is what the main transcript's tail says about the usage limit at its newest turn end.
type turnLimitReading struct {
	// limited is true when the newest main-chain turn end is a usage-limit entry.
	limited bool
	// previousTurnEndAt is the time of the main-chain turn end before the newest one; zero when there is none or its time does not parse.
	previousTurnEndAt time.Time
}

// readTurnLimit scans the complete lines of data from the last to the first for the two newest main-chain assistant entries that end a turn or carry the API-error marker.
// settled is true once the reading cannot change: the newest is a usage-limit entry, or the one before it turned up.
func readTurnLimit(data []byte) (reading turnLimitReading, settled bool) {
	foundNewest := false
	for end := len(data); end > 0; {
		start := bytes.LastIndexByte(data[:end], '\n') + 1
		line := bytes.TrimSpace(data[start:end])
		end = start - 1
		if len(line) == 0 {
			continue
		}
		var e apiErrorEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.IsSidechain || e.Type != "assistant" || !(e.isTurnEnd() || e.IsAPIErrorMessage) {
			continue
		}
		if !foundNewest {
			if e.isUsageLimit() {
				return turnLimitReading{limited: true}, true
			}
			foundNewest = true
			continue
		}
		reading.previousTurnEndAt, _ = time.Parse(time.RFC3339, e.Timestamp)
		return reading, true
	}
	return reading, false
}

// forkStoppedOnUsageLimit reports whether a fork transcript under the subagents directory beside transcriptPath, last written after since, has a usage-limit entry as its newest assistant entry.
// A fork transcript that cannot be read is skipped.
func forkStoppedOnUsageLimit(transcriptPath string, since time.Time) bool {
	dir := filepath.Join(strings.TrimSuffix(transcriptPath, ".jsonl"), "subagents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().After(since) {
			continue
		}
		if newestAssistantIsUsageLimit(filepath.Join(dir, entry.Name()), info.Size()) {
			return true
		}
	}
	return false
}

// newestAssistantIsUsageLimit reports whether the newest assistant entry of the size bytes of the transcript at path is a usage-limit entry.
// A read error and a transcript with no assistant entry report false.
func newestAssistantIsUsageLimit(path string, size int64) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	limited := false
	readBackward(f, size, initialReadChunk, func(data []byte) bool {
		for end := len(data); end > 0; {
			start := bytes.LastIndexByte(data[:end], '\n') + 1
			line := bytes.TrimSpace(data[start:end])
			end = start - 1
			var e apiErrorEntry
			if len(line) == 0 || json.Unmarshal(line, &e) != nil || e.Type != "assistant" {
				continue
			}
			limited = e.isUsageLimit()
			return true
		}
		return false
	})
	return limited
}
