// audit.go implements Claude's AuditForks/AuditForksIncremental.
// It reads the on-disk session transcript layout Claude Code itself maintains under ~/.claude/projects/<encoded-cwd>/.
// From it, it recovers mechanical facts about a fork-authorized run's subagents.
// It can filter to only the subagent transcripts a long-lived caller has not yet seen.
// A transcript's sibling .meta.json tells a fork from a subagent started with a fresh context.
// All of this file's knowledge — the project directory's cwd-encoding scheme, the parent/fork
// transcript paths, and the JSONL message shape — is Claude-specific and stays contained here, per
// the Shuttle Provider-Seam Invariant;
// shuttleengine itself only ever sees the provider-invariant ForkAudit/ForkReport value types this
// file populates.

package claudeengine

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// AuditForks implements shuttleengine.ForkAuditor for Claude, reporting every subagent transcript
// by calling AuditForksIncremental with nil seenTranscripts.
func (c *Claude) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return c.AuditForksIncremental(sessionID, workdir, nil)
}

// AuditForksIncremental implements shuttleengine.ForkAuditor for Claude, deriving
// the session's project directory from workdir and reading transcripts.
// A nil seenTranscripts reports every subagent transcript; missing subagents/ is not an error (zero
// forks).
// A missing parent transcript or unreadable subagent transcript is an error (audit incomplete).
func (c *Claude) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	projectDir, err := claudeProjectDirFor(workdir)
	if err != nil {
		return shuttleengine.ForkAudit{}, err
	}

	parentPath := filepath.Join(projectDir, sessionID+".jsonl")
	spawnCalls, namedSpawns, namedForkSpawns, writeCalls, writes, writeEvents, bashCommands, err := auditParentTranscript(parentPath)
	if err != nil {
		return shuttleengine.ForkAudit{}, err
	}

	subagentsDir := filepath.Join(projectDir, sessionID, subagentsDirName)
	entries, err := os.ReadDir(subagentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// The run never actually spawned a fork (or Claude has not yet
			// created the subagents/ directory) — a legitimate, zero-fork
			// finding, not a failure to complete the audit.
			return shuttleengine.ForkAudit{
				Forks:              []shuttleengine.ForkReport{},
				SpawnCalls:         spawnCalls,
				NamedSpawns:        namedSpawns,
				NamedForkSpawns:    namedForkSpawns,
				ParentWriteCalls:   writeCalls,
				ParentWrites:       writes,
				ParentWriteEvents:  writeEvents,
				ParentBashCommands: bashCommands,
			}, nil
		}
		return shuttleengine.ForkAudit{}, fmt.Errorf("claudeengine: read subagents dir %q: %w", subagentsDir, err)
	}

	forks := []shuttleengine.ForkReport{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		transcriptPath := filepath.Join(subagentsDir, entry.Name())
		if seenTranscripts != nil && seenTranscripts[transcriptPath] {
			// Already processed by the caller in an earlier incremental call —
			// re-parsing it would re-report facts a long-lived caller has
			// already acted on.
			continue
		}
		report, err := auditForkTranscript(transcriptPath)
		if err != nil {
			return shuttleengine.ForkAudit{}, err
		}
		forks = append(forks, report)
	}

	return shuttleengine.ForkAudit{
		Forks:              forks,
		SpawnCalls:         spawnCalls,
		NamedSpawns:        namedSpawns,
		NamedForkSpawns:    namedForkSpawns,
		ParentWriteCalls:   writeCalls,
		ParentWrites:       writes,
		ParentWriteEvents:  writeEvents,
		ParentBashCommands: bashCommands,
	}, nil
}

// claudeProjectDirFor derives the ~/.claude/projects/<encoded-workdir> directory,
// encoding workdir by replacing non-alphanumeric bytes with '-'.
func claudeProjectDirFor(workdir string) (string, error) {
	projects, err := projectsRoot()
	if err != nil {
		return "", err
	}
	return projectDirUnder(projects, workdir), nil
}

// claudeHomeDir resolves the home directory Claude Code uses for ~/.claude/projects/,
// honoring HOME before falling back to os.UserHomeDir() for platform-specific resolution.
func claudeHomeDir() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	return os.UserHomeDir()
}

// transcriptBlock is one entry of a transcript message's content array
// (tool_use or text). Only audited fields are modeled.
type transcriptBlock struct {
	Type  string         `json:"type"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
	Text  string         `json:"text"`
	// ToolUseID and IsError are carried by a tool_result block.
	ToolUseID string `json:"tool_use_id"`
	IsError   bool   `json:"is_error"`
}

// transcriptLine is one JSONL line of a Claude session transcript,
// with Type discriminating entry kinds and Message.Content carrying assistant blocks.
type transcriptLine struct {
	Type string `json:"type"`
	// Timestamp is the line's RFC 3339 time, kept as text so a malformed value costs only the time, not the line.
	Timestamp string `json:"timestamp"`
	// CustomTitle is the session name carried by a `custom-title` line.
	CustomTitle string `json:"customTitle"`
	Message     struct {
		// ID is the API message id shared by every assistant entry Claude writes for one message, one entry per content block.
		ID      string            `json:"id"`
		Content []transcriptBlock `json:"content"`
	} `json:"message"`
}

// forEachTranscriptLine leniently decodes path one JSONL line at a time and calls visit with each
// decoded line, skipping blank lines and JSON errors.
// File open and read errors are not tolerated (the caller's job to classify).
//
// It STREAMS rather than reading the file whole because a session transcript is unbounded in
// practice: os.ReadFile plus strings.Split(string(data), "\n") held the raw bytes, a full string
// copy of them, a per-line slice header, and every decoded line all at once, so a REAL 83 MiB
// transcript (the largest observed on a development machine) peaked at 178 MiB resident and a
// 303 MiB one at 931 MiB. Both callers only ever walk the result forward, exactly once, so nothing
// was buying that. This runs inside finalize, i.e. inside whatever long-lived process owns the
// run — webster's Master audits once per batch for the whole life of a plan — where a several-
// hundred-MiB spike is a real availability risk.
//
// bufio.Reader.ReadString is deliberate, and bufio.Scanner is deliberately NOT used: Scanner's
// 64 KiB default token cap would turn a single long transcript line (a large tool result, which is
// ordinary here) from "handled" into an error, which would be a regression rather than a fix.
func forEachTranscriptLine(path string, visit func(transcriptLine)) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		raw, readErr := reader.ReadString('\n')
		// A final line with no trailing newline comes back alongside io.EOF and is a real line, so
		// it must be decoded before the loop exits — an abnormally ended session is exactly the
		// case that produces one.
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			var line transcriptLine
			if json.Unmarshal([]byte(trimmed), &line) == nil {
				visit(line)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

// lineTime parses the line's timestamp, returning the zero time when it is absent or malformed.
func (l transcriptLine) lineTime() time.Time {
	at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
	if err != nil {
		return time.Time{}
	}
	return at
}

// writeTracker pairs each write tool_use with the tool_result that names its id.
type writeTracker struct {
	events []shuttleengine.WriteEvent
	byID   map[string]int
}

// call records a write tool_use made on line, unresolved until its result arrives.
func (w *writeTracker) call(id, path string, line transcriptLine) {
	if w.byID == nil {
		w.byID = map[string]int{}
	}
	if id != "" {
		w.byID[id] = len(w.events)
	}
	w.events = append(w.events, shuttleengine.WriteEvent{Path: path, At: line.lineTime()})
}

// result resolves the write named by block, if any, with the time of the result line.
func (w *writeTracker) result(block transcriptBlock, line transcriptLine) {
	idx, ok := w.byID[block.ToolUseID]
	if !ok || block.ToolUseID == "" {
		return
	}
	w.events[idx].At = line.lineTime()
	w.events[idx].Succeeded = !block.IsError
	delete(w.byID, block.ToolUseID)
}

// resolveResults feeds every tool_result block of a non-assistant line to the tracker.
func (w *writeTracker) resolveResults(line transcriptLine) {
	for _, block := range line.Message.Content {
		if block.Type == "tool_result" {
			w.result(block, line)
		}
	}
}

// writePath extracts a write block's target: file_path first, then NotebookEdit's notebook_path.
func writePath(block transcriptBlock) string {
	if filePath, _ := block.Input["file_path"].(string); filePath != "" {
		return filePath
	}
	notebookPath, _ := block.Input["notebook_path"].(string)
	return notebookPath
}

// auditParentTranscript reads the parent session's transcript.
// It extracts spawnCalls (Agent tool_use count) and namedSpawns (non-empty name fields).
// namedForkSpawns is the number of named spawns whose subagent_type is the fork type.
// It also extracts writeCalls (Write/Edit/NotebookEdit count), writes (file paths), their timed events,
// and bashCommands.
func auditParentTranscript(path string) (spawnCalls, namedSpawns, namedForkSpawns, writeCalls int, writes []string, events []shuttleengine.WriteEvent, bashCommands []string, err error) {
	var tracker writeTracker
	err = forEachTranscriptLine(path, func(line transcriptLine) {
		if line.Type != "assistant" {
			tracker.resolveResults(line)
			return
		}
		for _, block := range line.Message.Content {
			if block.Type != "tool_use" {
				continue
			}
			switch block.Name {
			case "Agent":
				spawnCalls++
				if name, _ := block.Input["name"].(string); name != "" {
					namedSpawns++
					if subagentType, _ := block.Input["subagent_type"].(string); subagentType == forkSubagentType {
						namedForkSpawns++
					}
				}
			case "Write", "Edit", "NotebookEdit":
				writeCalls++
				// NotebookEdit carries its path under notebook_path, not file_path.
				if filePath := writePath(block); filePath != "" {
					writes = append(writes, filePath)
					tracker.call(block.ID, filePath, line)
				}
			case "Bash":
				if cmd, _ := block.Input["command"].(string); cmd != "" {
					bashCommands = append(bashCommands, cmd)
				}
			}
		}
	})
	if err != nil {
		return 0, 0, 0, 0, nil, nil, nil, fmt.Errorf("claudeengine: read parent transcript %q: %w", path, err)
	}
	return spawnCalls, namedSpawns, namedForkSpawns, writeCalls, writes, tracker.events, bashCommands, nil
}

// subagentMeta is what Claude Code records in the .meta.json beside a subagent's transcript.
type subagentMeta struct {
	// ToolUseID is the parent's Agent tool_use id that spawned the subagent.
	ToolUseID string `json:"toolUseId"`
	// AgentType is the subagent type the spawn named, forkSubagentType for a fork.
	AgentType string `json:"agentType"`
	// IsFork is true for a fork, which inherits the parent's context.
	IsFork bool `json:"isFork"`
}

// readSubagentMeta reads the .meta.json beside the subagent transcript at transcriptPath.
// It reports false when the file is missing or unparseable.
func readSubagentMeta(transcriptPath string) (subagentMeta, bool) {
	metaPath := strings.TrimSuffix(transcriptPath, ".jsonl") + ".meta.json"
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return subagentMeta{}, false
	}
	var meta subagentMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return subagentMeta{}, false
	}
	return meta, true
}

// auditForkTranscript reads one subagent's transcript into a ForkReport,
// extracting ToolCalls, AgentCalls, WriteCalls, BashCommands, and ReportReturned
// (whether the final assistant message carried a text block).
// A transcript reads as a fresh subagent only on positive evidence:
// a parsed .meta.json that names a subagent type other than the fork type and does not flag a fork.
// A transcript whose .meta.json is missing, unparseable or records no kind reads as a fork.
func auditForkTranscript(path string) (shuttleengine.ForkReport, error) {
	// The parent's own spawning Agent call is replayed as the fork
	// transcript's inherited-context boundary entry; counting it would flag
	// every legitimate fork as a nested-Agent violation, so it is excluded by
	// its tool_use id (from the sibling .meta.json).
	meta, metaFound := readSubagentMeta(path)
	spawnToolUseID := meta.ToolUseID

	report := shuttleengine.ForkReport{
		TranscriptPath: path,
		ToolCalls:      map[string]int{},
		FreshSubagent:  metaFound && !meta.IsFork && meta.AgentType != "" && meta.AgentType != forkSubagentType,
	}
	reportReturned := false
	var tracker writeTracker
	err := forEachTranscriptLine(path, func(line transcriptLine) {
		if line.Type != "assistant" {
			tracker.resolveResults(line)
			return
		}
		// Overwritten (not OR-ed) on every assistant-type line, so this ends
		// up holding the LAST such message's hasText value — the "final
		// assistant message" ReportReturned actually means, not "any
		// assistant message ever had text".
		hasText := false
		for _, block := range line.Message.Content {
			switch block.Type {
			case "tool_use":
				if spawnToolUseID != "" && block.ID == spawnToolUseID {
					// The parent's spawning call, not something this fork did.
					continue
				}
				report.ToolCalls[block.Name]++
				switch block.Name {
				case "Agent":
					report.AgentCalls++
				case "Write", "Edit", "NotebookEdit":
					report.WriteCalls++
					// Mirror auditParentTranscript's path extraction: file_path
					// first, notebook_path fallback, and a pathless block still
					// counts above without contributing an entry.
					if filePath := writePath(block); filePath != "" {
						report.WritePaths = append(report.WritePaths, filePath)
						tracker.call(block.ID, filePath, line)
					}
				case "Bash":
					if cmd, _ := block.Input["command"].(string); cmd != "" {
						report.BashCommands = append(report.BashCommands, cmd)
					}
				}
			case "text":
				if block.Text != "" {
					hasText = true
				}
			}
		}
		reportReturned = hasText
	})
	if err != nil {
		return shuttleengine.ForkReport{}, fmt.Errorf("claudeengine: read subagent transcript %q: %w", path, err)
	}
	report.ReportReturned = reportReturned
	report.WriteEvents = tracker.events

	return report, nil
}
