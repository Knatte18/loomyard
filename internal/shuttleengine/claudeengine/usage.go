// usage.go implements shuttleengine.SessionCycler.ContextTokens and CompactedSince for Claude: the context a session holds is read from its own transcript, which a Stop payload names through transcript_path.
// A /clear starts a new session ID and transcript file and every later Stop payload names the new file,
// so reading the path from the turn end being evaluated follows the session through a clear with no state.
// The reader walks the file from its end in growing chunks and stops at the latest qualifying entry,
// either a main-chain assistant usage entry or a compaction boundary, so a long transcript costs one small read.
// The reader degrades to "usage unknown" on every failure and never errors, since the transcript format is a Claude Code internal.
// It also implements shuttleengine.UsageReader: SessionUsage sums a finished session's tokens from its transcript and its fork transcripts.
// The transcript shape knowledge stays in this package, the parse here and the locator in transcriptread.go.
package claudeengine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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

var _ shuttleengine.UsageReader = (*Claude)(nil)

// SessionUsage reads the tokens of session sessionID, recorded under the pane cwd workdir: the parent transcript <project dir>/<session id>.jsonl and every fork transcript under <project dir>/<session id>/subagents/.
// The totals include the forks, which are also broken out as their count and their own sums.
// A missing subagents directory is zero forks.
// An unreadable parent or fork transcript makes the whole reading unknown, never an error, since the layout is a Claude Code internal.
func (c *Claude) SessionUsage(sessionID, workdir string) shuttleengine.SessionUsage {
	projectDir, err := claudeProjectDirFor(workdir)
	if err != nil {
		return shuttleengine.SessionUsage{}
	}
	parent, err := parseTranscript(filepath.Join(projectDir, sessionID+".jsonl"), sessionID, false, nil)
	if err != nil {
		return shuttleengine.SessionUsage{}
	}

	subagentsDir := filepath.Join(projectDir, sessionID, "subagents")
	entries, err := os.ReadDir(subagentsDir)
	if err != nil && !os.IsNotExist(err) {
		return shuttleengine.SessionUsage{}
	}

	parentFresh, parentCacheRead := parent.tokens()
	reading := shuttleengine.SessionUsage{Known: true, Fresh: int64(parentFresh), CacheRead: int64(parentCacheRead)}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		fork, err := parseTranscript(filepath.Join(subagentsDir, entry.Name()), sessionID+"/"+entry.Name(), true, parent.countedIDs)
		if err != nil {
			return shuttleengine.SessionUsage{}
		}
		forkFresh, forkCacheRead := fork.tokens()
		reading.Forks++
		reading.ForkFresh += int64(forkFresh)
		reading.ForkCacheRead += int64(forkCacheRead)
	}
	reading.Fresh += reading.ForkFresh
	reading.CacheRead += reading.ForkCacheRead
	return reading
}

// launchLine matches the launch line lyx types into a session, capturing the prompt file's path.
var launchLine = regexp.MustCompile(`(?m)^Read (.+) in full first; it is your complete, authoritative instructions\.\s*$`)

// claudeUsage is the usage record of a Claude transcript message.
type claudeUsage struct {
	Input         int `json:"input_tokens"`
	CacheCreation int `json:"cache_creation_input_tokens"`
	CacheRead     int `json:"cache_read_input_tokens"`
	Output        int `json:"output_tokens"`
	// CacheCreationSplit breaks the cache write down by lifetime.
	CacheCreationSplit struct {
		OneHour int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// claudeMessage is the message a user or assistant transcript line carries.
type claudeMessage struct {
	ID         string          `json:"id"`
	Model      string          `json:"model"`
	StopReason string          `json:"stop_reason"`
	Content    json.RawMessage `json:"content"`
	Usage      *claudeUsage    `json:"usage"`
}

// claudeRecord is the part of a transcript line the parse reads.
type claudeRecord struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	Timestamp string `json:"timestamp"`
	// IsSidechain marks a line written by a sub-agent rather than the main chain.
	IsSidechain bool `json:"isSidechain"`
	// IsCompactSummary marks the user line that holds a compaction's summary.
	IsCompactSummary bool           `json:"isCompactSummary"`
	CustomTitle      string         `json:"customTitle"`
	Message          *claudeMessage `json:"message"`
}

// endsTurn reports whether the record ends its session's turn: an assistant line whose stop reason is not tool_use, or a user line that interrupts the turn or holds a local command's output.
func (r claudeRecord) endsTurn() bool {
	if r.Message == nil {
		return false
	}
	switch r.Type {
	case "assistant":
		return r.Message.StopReason != "" && r.Message.StopReason != "tool_use"
	case "user":
		for _, text := range recordUserTexts(r.Message.Content) {
			if strings.HasPrefix(text, "[Request interrupted by user") || strings.HasPrefix(text, "<local-command-stdout>") {
				return true
			}
		}
	}
	return false
}

// startsTurn reports whether the record starts a turn: a user line holding text and no tool result, other than a line that ends a turn or a compaction summary.
func (r claudeRecord) startsTurn() bool {
	if r.Type != "user" || r.Message == nil || r.IsCompactSummary || r.endsTurn() {
		return false
	}
	var text string
	if json.Unmarshal(r.Message.Content, &text) == nil {
		return true
	}
	var items []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(r.Message.Content, &items) != nil {
		return false
	}
	hasText := false
	for _, item := range items {
		switch item.Type {
		case "tool_result":
			return false
		case "text":
			hasText = true
		}
	}
	return hasText
}

// recordUserTexts returns the text of a user message's content, which is a string or a list of items, the text items of which count.
func recordUserTexts(content json.RawMessage) []string {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return []string{text}
	}
	var items []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &items) != nil {
		return nil
	}
	var texts []string
	for _, item := range items {
		if item.Type == "text" {
			texts = append(texts, item.Text)
		}
	}
	return texts
}

// recordToolCalls returns every tool call in an assistant message's content, classed by what it does.
func recordToolCalls(content json.RawMessage) []shuttleengine.ToolCall {
	var items []struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input struct {
			Command  string `json:"command"`
			FilePath string `json:"file_path"`
		} `json:"input"`
	}
	if json.Unmarshal(content, &items) != nil {
		return nil
	}
	var calls []shuttleengine.ToolCall
	for _, item := range items {
		if item.Type != "tool_use" {
			continue
		}
		call := shuttleengine.ToolCall{ID: item.ID, Kind: shuttleengine.ToolOther}
		switch item.Name {
		case "Bash":
			call.Kind, call.Command = shuttleengine.ToolShell, item.Input.Command
		case "Read":
			call.Kind, call.Path = shuttleengine.ToolFileRead, item.Input.FilePath
		}
		calls = append(calls, call)
	}
	return calls
}

// recordToolResults returns every tool result in a user message's content, which is a string or a list of items;
// a tool result's own content is likewise a string or a list of text items, joined by newlines.
func recordToolResults(content json.RawMessage) []shuttleengine.ToolResult {
	var items []struct {
		Type      string          `json:"type"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
	}
	if json.Unmarshal(content, &items) != nil {
		return nil
	}
	var results []shuttleengine.ToolResult
	for _, item := range items {
		if item.Type != "tool_result" {
			continue
		}
		var text string
		if json.Unmarshal(item.Content, &text) == nil {
			results = append(results, shuttleengine.ToolResult{CallID: item.ToolUseID, Text: text})
			continue
		}
		var parts []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(item.Content, &parts) != nil {
			continue
		}
		joined := make([]string, 0, len(parts))
		for _, part := range parts {
			joined = append(joined, part.Text)
		}
		results = append(results, shuttleengine.ToolResult{CallID: item.ToolUseID, Text: strings.Join(joined, "\n")})
	}
	return results
}

// parsedTranscript is one Claude transcript read: its user and assistant entries, its name, its start and its repeat count.
type parsedTranscript struct {
	entries []shuttleengine.TranscriptEntry
	// name is the latest non-empty customTitle of a custom-title line.
	name string
	// started is the time of the first line whose timestamp parses, whatever the line's type.
	started time.Time
	// repeats counts the assistant lines folded into their message's last line, plus every line of a skipped message.
	repeats int
	// countedIDs holds the ids of the messages that carry usage, the set a fork's parse skips.
	countedIDs map[string]struct{}
}

// tokens sums the usage the entries carry: fresh is input plus cache-creation plus output tokens, cacheRead is cache-read tokens.
func (p parsedTranscript) tokens() (fresh, cacheRead int) {
	for _, entry := range p.entries {
		if entry.Usage != nil {
			fresh += entry.Usage.Input + entry.Usage.CacheWrite + entry.Usage.Output
			cacheRead += entry.Usage.CacheRead
		}
	}
	return fresh, cacheRead
}

// parseTranscript reads the transcript at path into entries for every user and assistant line.
// Sidechain lines get an entry only when includeSidechain is set, as a fork transcript needs.
// transcriptKey names the transcript among all the engine records and keeps the keys of id-less lines unique across transcripts.
// An assistant line whose message id is in skipIDs carries no usage, since a fork transcript replays the parent's spawning message as its first entry.
// A message streamed over several lines carries usage on its last line only, since the earlier lines hold a partial output count.
// Malformed lines, a half-written last line included, are skipped, and an unreadable file is an error.
func parseTranscript(path, transcriptKey string, includeSidechain bool, skipIDs map[string]struct{}) (parsedTranscript, error) {
	f, err := os.Open(path)
	if err != nil {
		return parsedTranscript{}, err
	}
	defer f.Close()

	parsed := parsedTranscript{countedIDs: map[string]struct{}{}}
	lastUsageEntry := map[string]int{}

	reader := bufio.NewReader(f)
	for lineNumber := 1; ; lineNumber++ {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			return parsedTranscript{}, readErr
		}
		var record claudeRecord
		if json.Unmarshal(bytes.TrimSpace(line), &record) == nil {
			at, timeErr := time.Parse(time.RFC3339Nano, record.Timestamp)
			if parsed.started.IsZero() && timeErr == nil {
				parsed.started = at
			}
			if record.Type == "custom-title" && record.CustomTitle != "" {
				parsed.name = record.CustomTitle
			}
			if (record.Type == "user" || record.Type == "assistant") && (includeSidechain || !record.IsSidechain) {
				entry := newTranscriptEntry(record, transcriptKey, lineNumber)
				if timeErr == nil {
					entry.At = at
				}
				parsed.addEntry(entry, record, skipIDs, lastUsageEntry)
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	return parsed, nil
}

// newTranscriptEntry builds the entry of a user or assistant record, with no time and no usage.
func newTranscriptEntry(record claudeRecord, transcriptKey string, lineNumber int) shuttleengine.TranscriptEntry {
	var message claudeMessage
	if record.Message != nil {
		message = *record.Message
	}
	entry := shuttleengine.TranscriptEntry{
		Role:       shuttleengine.EntryRole(record.Type),
		StartsTurn: record.startsTurn(),
		EndsTurn:   record.endsTurn(),
		Model:      message.Model,
	}
	switch {
	case message.ID != "":
		entry.MessageKey = message.ID
	case record.UUID != "":
		entry.MessageKey = record.UUID
	default:
		entry.MessageKey = fmt.Sprintf("line:%s:%d", transcriptKey, lineNumber)
	}
	if record.Type == "assistant" {
		entry.ToolCalls = recordToolCalls(message.Content)
		return entry
	}
	entry.UserTexts = recordUserTexts(message.Content)
	entry.ToolResults = recordToolResults(message.Content)
	for _, text := range entry.UserTexts {
		if match := launchLine.FindStringSubmatch(text); match != nil {
			entry.PromptPath = match[1]
			break
		}
	}
	return entry
}

// addEntry appends entry and settles its usage: a skipped message's line adds a repeat and carries none,
// and a line of a message already seen takes the usage over from the earlier line, which adds a repeat and loses it.
// lastUsageEntry maps a message key to the index of the entry carrying that message's usage.
func (p *parsedTranscript) addEntry(entry shuttleengine.TranscriptEntry, record claudeRecord, skipIDs map[string]struct{}, lastUsageEntry map[string]int) {
	if record.Type == "assistant" && record.Message != nil {
		message := record.Message
		if _, skipped := skipIDs[message.ID]; skipped {
			p.repeats++
		} else if message.Usage != nil {
			if previous, seen := lastUsageEntry[entry.MessageKey]; seen {
				p.entries[previous].Usage = nil
				p.repeats++
			}
			lastUsageEntry[entry.MessageKey] = len(p.entries)
			entry.Usage = &shuttleengine.MessageUsage{
				Input:             message.Usage.Input,
				CacheWrite:        message.Usage.CacheCreation,
				CacheWriteOneHour: message.Usage.CacheCreationSplit.OneHour,
				CacheRead:         message.Usage.CacheRead,
				Output:            message.Usage.Output,
			}
			if message.ID != "" {
				p.countedIDs[message.ID] = struct{}{}
			}
		}
	}
	p.entries = append(p.entries, entry)
}
