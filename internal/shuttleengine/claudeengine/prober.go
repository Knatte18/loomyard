// prober.go implements shuttleengine.SessionProber for Claude: process liveness from Claude's session registry, and the interrupt marker in the transcript a turn start names.
// It also implements shuttleengine.TurnProgressReader, reading whether that turn is in flight from the same transcript.
// Both layouts are Claude Code internals, so every failure degrades to the unproven, not-interrupted or unknown answer and never errors.

package claudeengine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Compile-time assertion that Claude offers the optional session prober.
var _ shuttleengine.SessionProber = (*Claude)(nil)

// Compile-time assertion that Claude offers the optional turn progress reader.
var _ shuttleengine.TurnProgressReader = (*Claude)(nil)

// interruptMarkerPrefix starts the text of the user entry Claude Code writes when the user interrupts a turn.
const interruptMarkerPrefix = "[Request interrupted by user"

// ProcessLiveness implements shuttleengine.SessionProber.
// It resolves the registry directory from the same home resolution CheckResume uses.
func (c *Claude) ProcessLiveness(sessionID string) shuttleengine.Liveness {
	home, err := claudeHomeDir()
	if err != nil {
		return shuttleengine.LivenessUnproven
	}
	return processLiveness(sessionID, filepath.Join(home, ".claude", "sessions"), proc.IsAlive, proc.StartTime)
}

// processLiveness answers from the registry entries that name sessionID.
// A live pid whose start time equals the entry's procStart is alive, and any alive entry wins.
// Dead needs an entry whose pid is gone and no entry or file that leaves the answer open;
// no entry, an unreadable registry or entry, an unreadable start time, an entry without procStart and a live pid with a different start time (a reused pid) all read unproven.
func processLiveness(sessionID, registryDir string, alive func(pid int) bool, startTime func(pid int) (string, bool)) shuttleengine.Liveness {
	records, err := readRegistry(registryDir)
	if err != nil {
		return shuttleengine.LivenessUnproven
	}
	sawDead, sawDoubt := false, false
	for _, record := range records {
		if record.ReadErr != nil || record.DecodeErr != nil {
			sawDoubt = true
			continue
		}
		entry := record.Entry
		if entry.SessionID != sessionID {
			continue
		}
		if !alive(entry.PID) {
			sawDead = true
			continue
		}
		live, readable := startTime(entry.PID)
		if readable && entry.ProcStart != "" && live == entry.ProcStart {
			return shuttleengine.LivenessAlive
		}
		sawDoubt = true
	}
	if sawDead && !sawDoubt {
		return shuttleengine.LivenessDead
	}
	return shuttleengine.LivenessUnproven
}

// TurnStartInterrupt implements shuttleengine.SessionProber.
// It reads the transcript the turn start's UserPromptSubmit payload names through transcript_path, backward from its end.
func (c *Claude) TurnStartInterrupt(turnStart shuttleengine.SessionSignal) (time.Time, bool) {
	var at time.Time
	var interrupted bool
	scanTurnStartTranscript(turnStart, func(data []byte) bool {
		var found bool
		at, interrupted, found = newestEntryInterrupt(data)
		return found
	})
	return at, interrupted
}

// TurnStartProgress implements shuttleengine.TurnProgressReader.
// It reads the newest main-chain entry of the transcript the turn start names, and every failure to read it reads unknown.
func (c *Claude) TurnStartProgress(turnStart shuttleengine.SessionSignal) shuttleengine.TurnProgress {
	progress := shuttleengine.TurnProgressUnknown
	scanTurnStartTranscript(turnStart, func(data []byte) bool {
		var found bool
		progress, found = newestTurnProgress(data)
		return found
	})
	return progress
}

// scanTurnStartTranscript opens the transcript the turn start's UserPromptSubmit payload names through transcript_path and hands scan its complete lines backward from the end, as readBackward does.
// A missing path, an undecodable payload and an open or stat failure return without calling scan.
func scanTurnStartTranscript(turnStart shuttleengine.SessionSignal, scan func(data []byte) bool) {
	var payload struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if err := json.Unmarshal(turnStart.Raw, &payload); err != nil || payload.TranscriptPath == "" {
		return
	}
	f, err := os.Open(payload.TranscriptPath)
	if err != nil {
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return
	}
	readBackward(f, info.Size(), initialReadChunk, scan)
}

// newestTurnProgress reads the turn's progress from the newest main-chain user or assistant entry among the complete lines of data, scanning from the last to the first.
// Bytes after the last newline are a line still being written and are never read; blank and unparsable lines, sidechain entries and entries of any other type are skipped.
// A user entry reads in flight (a prompt, a task notification or a tool result), except the interrupt marker, which reads unknown.
// An assistant entry is read by its message's stop reason, never by its last content block:
// tool_use reads in flight, end_turn and stop_sequence read over, and an absent, null or other reason reads unknown.
// found is false when data holds no such entry.
func newestTurnProgress(data []byte) (progress shuttleengine.TurnProgress, found bool) {
	data = data[:bytes.LastIndexByte(data, '\n')+1]
	for end := len(data); end > 0; {
		start := bytes.LastIndexByte(data[:end-1], '\n') + 1
		line := bytes.TrimSpace(data[start:end])
		end = start
		if len(line) == 0 {
			continue
		}
		var e transcriptEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.IsSidechain || (e.Type != "user" && e.Type != "assistant") {
			continue
		}
		if e.Type == "user" {
			if strings.HasPrefix(e.finalText(), interruptMarkerPrefix) {
				return shuttleengine.TurnProgressUnknown, true
			}
			return shuttleengine.TurnInFlight, true
		}
		switch e.Message.StopReason {
		case "tool_use":
			return shuttleengine.TurnInFlight, true
		case "end_turn", "stop_sequence":
			return shuttleengine.TurnOver, true
		}
		return shuttleengine.TurnProgressUnknown, true
	}
	return shuttleengine.TurnProgressUnknown, false
}

// newestEntryInterrupt scans the complete lines of data from the last to the first and reports whether the first main-chain user or assistant entry is the user's interrupt marker, with that entry's timestamp.
// found is false when data holds no such entry; an entry whose timestamp does not parse is found and not an interrupt.
func newestEntryInterrupt(data []byte) (at time.Time, interrupted, found bool) {
	for end := len(data); end > 0; {
		start := bytes.LastIndexByte(data[:end], '\n') + 1
		line := bytes.TrimSpace(data[start:end])
		end = start - 1
		if len(line) == 0 {
			continue
		}
		var e transcriptEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.IsSidechain || (e.Type != "user" && e.Type != "assistant") {
			continue
		}
		if e.Type != "user" || !strings.HasPrefix(e.finalText(), interruptMarkerPrefix) {
			return time.Time{}, false, true
		}
		at, err := time.Parse(time.RFC3339Nano, e.Timestamp)
		if err != nil {
			return time.Time{}, false, true
		}
		return at, true, true
	}
	return time.Time{}, false, false
}
