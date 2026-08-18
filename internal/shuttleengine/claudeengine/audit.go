// audit.go implements Claude's AuditForks/AuditForksIncremental: reading the on-disk session
// transcript layout Claude Code itself maintains under ~/.claude/projects/<encoded-cwd>/ to recover
// mechanical facts about a fork-authorized run's fork subagents, optionally filtered to only the
// fork transcripts a long-lived caller has not yet seen.
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

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// AuditForks implements shuttleengine.Engine.AuditForks for Claude, reporting every fork transcript
// by calling AuditForksIncremental with nil seenTranscripts.
func (c *Claude) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return c.AuditForksIncremental(sessionID, workdir, nil)
}

// AuditForksIncremental implements shuttleengine.Engine.AuditForksIncremental for Claude, deriving
// the session's project directory from workdir and reading transcripts.
// A nil seenTranscripts reports every fork transcript; missing subagents/ is not an error (zero
// forks).
// A missing parent transcript or unreadable fork transcript is an error (audit incomplete).
func (c *Claude) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	projectDir, err := claudeProjectDirFor(workdir)
	if err != nil {
		return shuttleengine.ForkAudit{}, err
	}

	parentPath := filepath.Join(projectDir, sessionID+".jsonl")
	spawnCalls, namedSpawns, writeCalls, writes, bashCommands, err := auditParentTranscript(parentPath)
	if err != nil {
		return shuttleengine.ForkAudit{}, err
	}

	subagentsDir := filepath.Join(projectDir, sessionID, "subagents")
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
				ParentWriteCalls:   writeCalls,
				ParentWrites:       writes,
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
		ParentWriteCalls:   writeCalls,
		ParentWrites:       writes,
		ParentBashCommands: bashCommands,
	}, nil
}

// claudeProjectDirFor derives the ~/.claude/projects/<encoded-workdir> directory,
// encoding workdir by replacing non-alphanumeric bytes with '-'.
func claudeProjectDirFor(workdir string) (string, error) {
	home, err := claudeHomeDir()
	if err != nil {
		return "", fmt.Errorf("claudeengine: resolve home dir: %w", err)
	}

	encoded := []byte(workdir)
	for i, b := range encoded {
		isAlnum := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
		if !isAlnum {
			encoded[i] = '-'
		}
	}
	return filepath.Join(home, ".claude", "projects", string(encoded)), nil
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
}

// transcriptLine is one JSONL line of a Claude session transcript,
// with Type discriminating entry kinds and Message.Content carrying assistant blocks.
type transcriptLine struct {
	Type    string `json:"type"`
	Message struct {
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

// auditParentTranscript reads the parent session's transcript and extracts
// spawnCalls (Agent tool_use count), namedSpawns (non-empty name fields),
// writeCalls (Write/Edit/NotebookEdit count), writes (file paths), and bashCommands.
func auditParentTranscript(path string) (spawnCalls, namedSpawns, writeCalls int, writes, bashCommands []string, err error) {
	err = forEachTranscriptLine(path, func(line transcriptLine) {
		if line.Type != "assistant" {
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
				}
			case "Write", "Edit", "NotebookEdit":
				writeCalls++
				filePath, ok := block.Input["file_path"].(string)
				if !ok || filePath == "" {
					// NotebookEdit carries its path under notebook_path, not
					// file_path — fall back before giving up on this block.
					filePath, ok = block.Input["notebook_path"].(string)
				}
				if ok && filePath != "" {
					writes = append(writes, filePath)
				}
			case "Bash":
				if cmd, _ := block.Input["command"].(string); cmd != "" {
					bashCommands = append(bashCommands, cmd)
				}
			}
		}
	})
	if err != nil {
		return 0, 0, 0, nil, nil, fmt.Errorf("claudeengine: read parent transcript %q: %w", path, err)
	}
	return spawnCalls, namedSpawns, writeCalls, writes, bashCommands, nil
}

// forkSpawnToolUseID returns the parent's Agent tool_use id that spawned the fork,
// read from the sibling .meta.json file. Returns "" if meta file is missing or unparseable.
func forkSpawnToolUseID(transcriptPath string) string {
	metaPath := strings.TrimSuffix(transcriptPath, ".jsonl") + ".meta.json"
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return ""
	}
	var meta struct {
		ToolUseID string `json:"toolUseId"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return meta.ToolUseID
}

// auditForkTranscript reads one fork subagent's transcript into a ForkReport,
// extracting ToolCalls, AgentCalls, WriteCalls, BashCommands, and ReportReturned
// (whether the final assistant message carried a text block).
func auditForkTranscript(path string) (shuttleengine.ForkReport, error) {
	// The parent's own spawning Agent call is replayed as the fork
	// transcript's inherited-context boundary entry; counting it would flag
	// every legitimate fork as a nested-Agent violation, so it is excluded by
	// its tool_use id (from the sibling .meta.json).
	spawnToolUseID := forkSpawnToolUseID(path)

	report := shuttleengine.ForkReport{
		TranscriptPath: path,
		ToolCalls:      map[string]int{},
	}
	reportReturned := false
	err := forEachTranscriptLine(path, func(line transcriptLine) {
		if line.Type != "assistant" {
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
					filePath, ok := block.Input["file_path"].(string)
					if !ok || filePath == "" {
						filePath, ok = block.Input["notebook_path"].(string)
					}
					if ok && filePath != "" {
						report.WritePaths = append(report.WritePaths, filePath)
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
		return shuttleengine.ForkReport{}, fmt.Errorf("claudeengine: read fork transcript %q: %w", path, err)
	}
	report.ReportReturned = reportReturned

	return report, nil
}
