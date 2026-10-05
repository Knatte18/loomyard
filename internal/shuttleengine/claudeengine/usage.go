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

// CompactedSince returns the timestamp of the newest main-chain compaction boundary after since in the transcript turnEnd names.
// Like ContextTokens it degrades to not found on every failure and never errors.
func (c *Claude) CompactedSince(turnEnd shuttleengine.Event, since time.Time) (time.Time, bool) {
	var payload struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if err := json.Unmarshal(turnEnd.Raw, &payload); err != nil || payload.TranscriptPath == "" {
		return time.Time{}, false
	}
	f, err := os.Open(payload.TranscriptPath)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return time.Time{}, false
	}
	return compactedSince(f, info.Size(), initialReadChunk, since)
}

// compactedSince walks the transcript backward and returns the first compaction boundary after since.
// It stops at the first entry whose timestamp is not after since, so the walk reads only what is newer than since.
func compactedSince(r io.ReaderAt, size int64, chunk int, since time.Time) (time.Time, bool) {
	var at time.Time
	var found bool
	readBackward(r, size, chunk, func(data []byte) bool {
		at, found = boundarySince(data, since)
		return found || reachedSince(data, since)
	})
	return at, found
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
}

// boundarySince scans the complete lines of data from the last to the first and returns the first main-chain compaction boundary after since.
// It stops at the first entry whose timestamp is not after since.
func boundarySince(data []byte, since time.Time) (time.Time, bool) {
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
		at, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		if !at.After(since) {
			return time.Time{}, false
		}
		if !e.IsSidechain && e.Type == "system" && e.Subtype == "compact_boundary" && e.CompactMetadata.PostTokens > 0 {
			return at, true
		}
	}
	return time.Time{}, false
}

// reachedSince reports whether data holds an entry whose timestamp is not after since, which ends the backward walk.
func reachedSince(data []byte, since time.Time) bool {
	for end := len(data); end > 0; {
		start := bytes.LastIndexByte(data[:end], '\n') + 1
		line := bytes.TrimSpace(data[start:end])
		end = start - 1
		var e transcriptEntry
		if len(line) == 0 || json.Unmarshal(line, &e) != nil {
			continue
		}
		if at, err := time.Parse(time.RFC3339, e.Timestamp); err == nil && !at.After(since) {
			return true
		}
	}
	return false
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
