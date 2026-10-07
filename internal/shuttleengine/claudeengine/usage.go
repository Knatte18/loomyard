// usage.go implements shuttleengine.SessionCycler.ContextTokens and CompactedSince for Claude: the context a session holds is read from its own transcript, which a Stop payload names through transcript_path.
// A /clear starts a new session ID and transcript file and every later Stop payload names the new file,
// so reading the path from the turn end being evaluated follows the session through a clear with no state.
// The reader walks the file from its end in growing chunks and stops at the latest qualifying entry,
// either a main-chain assistant usage entry or a compaction boundary, so a long transcript costs one small read.
// The reader degrades to "usage unknown" on every failure and never errors, since the transcript format is a Claude Code internal.
// All payload and transcript shape knowledge stays in this file, per the Shuttle Provider-Seam Invariant.
package claudeengine

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// initialReadChunk is the size of the first backward read; each further round doubles the window.
const initialReadChunk = 64 * 1024

// ContextTokens reads the context reading the transcript named by turnEnd's transcript_path holds.
// It returns an unknown reading for a missing transcript_path, an unreadable file, or a transcript with no usable entry.
func (c *Claude) ContextTokens(turnEnd shuttleengine.Event) shuttleengine.ContextReading {
	var payload struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if err := json.Unmarshal(turnEnd.Raw, &payload); err != nil || payload.TranscriptPath == "" {
		return shuttleengine.ContextReading{}
	}
	f, err := os.Open(payload.TranscriptPath)
	if err != nil {
		return shuttleengine.ContextReading{}
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return shuttleengine.ContextReading{}
	}
	return readContext(f, info.Size(), initialReadChunk)
}

// readContext returns the reading of the latest qualifying entry in the size bytes of the JSONL transcript r.
// It reads a window from the end, doubling it from chunk until a qualifying entry turns up or the window reaches the file start,
// and parses only complete lines: the window's first line is dropped unless the window starts the file,
// so an entry spanning the window edge is read whole once a later round widens the window past its start.
// A qualifying entry is a main-chain assistant entry whose input, cache-creation and cache-read tokens do not sum to zero
// (a synthetic entry for an API error or interrupt says nothing about the context),
// or a main-chain compact_boundary system entry with a positive postTokens and an RFC 3339 timestamp.
// Sidechain entries and malformed lines are skipped.
func readContext(r io.ReaderAt, size int64, chunk int) shuttleengine.ContextReading {
	var reading shuttleengine.ContextReading
	readBackward(r, size, chunk, func(data []byte) bool {
		var found bool
		reading, found = latestReading(data)
		return found
	})
	return reading
}

// readBackward hands scan the complete lines of a window from the end of the size bytes of r, doubling the window from chunk until scan returns true or the window reaches the file start.
// The window's first line is dropped unless the window starts the file, so an entry spanning the window edge is scanned whole once a later round widens the window past its start.
// A read error ends the walk without a further scan.
func readBackward(r io.ReaderAt, size int64, chunk int, scan func(data []byte) bool) {
	if chunk < 1 {
		chunk = 1
	}
	window := int64(chunk)
	for {
		start := size - window
		if start < 0 {
			start = 0
		}
		buf := make([]byte, size-start)
		if _, err := r.ReadAt(buf, start); err != nil && err != io.EOF {
			return
		}
		if start > 0 {
			nl := bytes.IndexByte(buf, '\n')
			if nl < 0 {
				buf = nil
			} else {
				buf = buf[nl+1:]
			}
		}
		if scan(buf) || start == 0 {
			return
		}
		window *= 2
	}
}

// CompactedSince returns the newest main-chain compaction boundary after since in the transcript turnEnd names, with the main-chain turn ends that follow it.
// ReadTurnEndAfter compares the turn end's last_assistant_message with the final text block of the newest of those turn ends, ignoring surrounding whitespace;
// a Stop payload without a message cannot be matched,
// so it reports false.
// Claude writes a turn's assistant entry before its Stop hook fires,
// so the turn end being read is already in the transcript.
// Like ContextTokens it degrades to not found on every failure and never errors.
func (c *Claude) CompactedSince(turnEnd shuttleengine.Event, since time.Time) (shuttleengine.CompactionBoundary, bool) {
	var payload struct {
		TranscriptPath       string `json:"transcript_path"`
		LastAssistantMessage string `json:"last_assistant_message"`
	}
	if err := json.Unmarshal(turnEnd.Raw, &payload); err != nil || payload.TranscriptPath == "" {
		return shuttleengine.CompactionBoundary{}, false
	}
	f, err := os.Open(payload.TranscriptPath)
	if err != nil {
		return shuttleengine.CompactionBoundary{}, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return shuttleengine.CompactionBoundary{}, false
	}
	return compactedSince(f, info.Size(), initialReadChunk, since, payload.LastAssistantMessage)
}

// compactedSince walks the transcript backward and returns the first compaction boundary after since, with the turn ends after it.
// It stops at the first entry whose timestamp is not after since, so the walk reads only what is newer than since.
// readMessage is the Stop payload's last_assistant_message.
func compactedSince(r io.ReaderAt, size int64, chunk int, since time.Time, readMessage string) (shuttleengine.CompactionBoundary, bool) {
	var boundary shuttleengine.CompactionBoundary
	var found bool
	readBackward(r, size, chunk, func(data []byte) bool {
		var reachedSince bool
		boundary, found, reachedSince = boundarySince(data, since, readMessage)
		return found || reachedSince
	})
	return boundary, found
}

// transcriptEntry is the part of a transcript line the compaction reads look at.
type transcriptEntry struct {
	Type            string `json:"type"`
	Subtype         string `json:"subtype"`
	IsSidechain     bool   `json:"isSidechain"`
	Timestamp       string `json:"timestamp"`
	CompactMetadata struct {
		PostTokens int `json:"postTokens"`
	} `json:"compactMetadata"`
	Message struct {
		StopReason string          `json:"stop_reason"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
}

// isTurnEnd reports whether e is a main-chain assistant entry that ends a turn: its stop reason is set and is not tool_use.
func (e transcriptEntry) isTurnEnd() bool {
	return !e.IsSidechain && e.Type == "assistant" && e.Message.StopReason != "" && e.Message.StopReason != "tool_use"
}

// finalText returns the text of the last text block of the entry's message content, or "" when it has none.
// A bare string content counts as one text block.
func (e transcriptEntry) finalText() string {
	var bare string
	if json.Unmarshal(e.Message.Content, &bare) == nil {
		return bare
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(e.Message.Content, &blocks) != nil {
		return ""
	}
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Type == "text" {
			return blocks[i].Text
		}
	}
	return ""
}

// boundarySince scans the complete lines of data from the last to the first and returns the first main-chain compaction boundary after since,
// counting the turn ends it passes on the way.
// reachedSince is true when it met an entry whose timestamp is not after since, which ends the backward walk whether or not a boundary was found.
func boundarySince(data []byte, since time.Time, readMessage string) (boundary shuttleengine.CompactionBoundary, found, reachedSince bool) {
	turnEnds := 0
	newestText := ""
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
		at, timeErr := time.Parse(time.RFC3339, e.Timestamp)
		if timeErr == nil && !at.After(since) {
			return shuttleengine.CompactionBoundary{}, false, true
		}
		switch {
		case timeErr == nil && !e.IsSidechain && e.Type == "system" && e.Subtype == "compact_boundary" && e.CompactMetadata.PostTokens > 0:
			wanted := strings.TrimSpace(readMessage)
			return shuttleengine.CompactionBoundary{
				At:               at,
				TurnEndsAfter:    turnEnds,
				ReadTurnEndAfter: turnEnds > 0 && wanted != "" && strings.TrimSpace(newestText) == wanted,
			}, true, false
		case e.isTurnEnd():
			if turnEnds == 0 {
				newestText = e.finalText()
			}
			turnEnds++
		}
	}
	return shuttleengine.CompactionBoundary{}, false, false
}

// latestReading scans the complete lines of data from the last to the first and returns the reading of the first qualifying entry.
func latestReading(data []byte) (shuttleengine.ContextReading, bool) {
	type usage struct {
		Input         int `json:"input_tokens"`
		CacheCreation int `json:"cache_creation_input_tokens"`
		CacheRead     int `json:"cache_read_input_tokens"`
	}
	type entry struct {
		Type            string `json:"type"`
		Subtype         string `json:"subtype"`
		IsSidechain     bool   `json:"isSidechain"`
		Timestamp       string `json:"timestamp"`
		CompactMetadata struct {
			PostTokens int `json:"postTokens"`
		} `json:"compactMetadata"`
		Message struct {
			Usage *usage `json:"usage"`
		} `json:"message"`
	}

	for end := len(data); end > 0; {
		start := bytes.LastIndexByte(data[:end], '\n') + 1
		line := bytes.TrimSpace(data[start:end])
		end = start - 1
		if len(line) == 0 {
			continue
		}
		var e entry
		if err := json.Unmarshal(line, &e); err != nil || e.IsSidechain {
			continue
		}
		switch {
		case e.Type == "assistant" && e.Message.Usage != nil:
			sum := e.Message.Usage.Input + e.Message.Usage.CacheCreation + e.Message.Usage.CacheRead
			if sum == 0 {
				continue
			}
			return shuttleengine.ContextReading{Tokens: sum, Known: true}, true
		case e.Type == "system" && e.Subtype == "compact_boundary" && e.CompactMetadata.PostTokens > 0:
			at, err := time.Parse(time.RFC3339, e.Timestamp)
			if err != nil {
				continue
			}
			return shuttleengine.ContextReading{Tokens: e.CompactMetadata.PostTokens, Known: true, Compacted: true, BoundaryAt: at}, true
		}
	}
	return shuttleengine.ContextReading{}, false
}
