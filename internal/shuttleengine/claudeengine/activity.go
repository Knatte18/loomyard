// activity.go implements shuttleengine.ActivityReader for Claude: the session's last activity, its forks included, and whether the newest main-chain turn end of the transcript a Stop payload names through transcript_path is an API error.
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

var _ shuttleengine.ActivityReader = (*Claude)(nil)

// apiErrorEntry is a transcript entry with the marker Claude Code sets on the synthetic assistant entry it writes for an API error.
type apiErrorEntry struct {
	transcriptEntry
	IsAPIErrorMessage bool `json:"isApiErrorMessage"`
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
		activity.APIError, activity.APIErrorText, found = newestTurnEndError(data)
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

// holdsPendingToolCall reports whether the transcript at path holds a tool_use block whose id no tool_result block names, wherever the call sits.
// A read error reports false.
func holdsPendingToolCall(path string) bool {
	called := map[string]bool{}
	answered := map[string]bool{}
	err := forEachTranscriptLine(path, func(line transcriptLine) {
		for _, block := range line.Message.Content {
			switch block.Type {
			case "tool_use":
				called[block.ID] = true
			case "tool_result":
				answered[block.ToolUseID] = true
			}
		}
	})
	if err != nil {
		return false
	}
	for id := range called {
		if !answered[id] {
			return true
		}
	}
	return false
}

// newestTurnEndError scans the complete lines of data from the last to the first and reports whether the first main-chain assistant entry that ends a turn or carries the API-error marker is an API error, with its final text.
// found is false when data holds no such entry.
func newestTurnEndError(data []byte) (apiError bool, text string, found bool) {
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
		if e.IsAPIErrorMessage {
			return true, e.finalText(), true
		}
		return false, "", true
	}
	return false, "", false
}
