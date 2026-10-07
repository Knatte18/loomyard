// activity.go implements shuttleengine.ActivityReader for Claude: the transcript a Stop payload names through transcript_path is read for its last write time and for whether its newest main-chain turn end is an API error.
// The marker Claude Code writes on a synthetic API-error assistant entry stays in this file, per the Shuttle Provider-Seam Invariant.
// Like the other transcript reads it degrades to the zero reading on every failure and never errors, since the transcript format is a Claude Code internal.
package claudeengine

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.ActivityReader = (*Claude)(nil)

// apiErrorEntry is a transcript entry with the marker Claude Code sets on the synthetic assistant entry it writes for an API error.
type apiErrorEntry struct {
	transcriptEntry
	IsAPIErrorMessage bool `json:"isApiErrorMessage"`
}

// TurnEndActivity reads the transcript turnEnd's transcript_path names.
// Its modification time is the last write time, and the newest main-chain assistant entry that ends a turn or carries the API-error marker decides whether the session stands on an API error, with that entry's final text.
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
	readBackward(f, info.Size(), initialReadChunk, func(data []byte) bool {
		var found bool
		activity.APIError, activity.APIErrorText, found = newestTurnEndError(data)
		return found
	})
	return activity
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
