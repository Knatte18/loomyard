// audit.go implements Claude's AuditForks: reading the on-disk session transcript
// layout Claude Code itself maintains under ~/.claude/projects/<encoded-cwd>/ to
// recover mechanical facts about a fork-authorized run's fork subagents. All of this
// file's knowledge — the project directory's cwd-encoding scheme, the parent/fork
// transcript paths, and the JSONL message shape — is Claude-specific and stays
// contained here, per the Shuttle Provider-Seam Invariant; shuttleengine itself only
// ever sees the provider-invariant ForkAudit/ForkReport value types this file
// populates.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// AuditForks implements shuttleengine.Engine.AuditForks for Claude. It derives the
// session's project directory from workdir (claudeProjectDirFor), reads
// <projectDir>/<sessionID>.jsonl as the parent session's own transcript
// (SpawnCalls/NamedSpawns), and reads every
// <projectDir>/<sessionID>/subagents/*.jsonl as one fork subagent's transcript
// (one ForkReport each). workdir MUST be the pane's actual process cwd — see the
// call site in wait.go's finalize for why layout.Cwd, never layout.WorktreeRoot, is
// what must be passed here. A missing subagents/ directory is not an error: zero
// forks is a legitimate finding for the caller's policy to interpret (an
// authorized-but-unused capability), so it yields ForkAudit{Forks: []ForkReport{}}
// with a nil error. A missing parent transcript, or a fork transcript that exists
// but cannot be read, IS an error — the audit could not be completed at all, and
// wait.go's fail-loud posture on this error is what keeps a fork-mode run whose
// audit came back incomplete from ever silently classifying as a clean done.
func (c *Claude) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
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
		report, err := auditForkTranscript(filepath.Join(subagentsDir, entry.Name()))
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

// claudeProjectDirFor derives the ~/.claude/projects/<encoded-workdir> directory
// Claude persists this session's transcripts into, mirroring claudeProjectDir in
// internal/muxcli/smoke_test.go (verified there against a live transcript):
// workdir with every non-alphanumeric byte replaced by '-'. Production code must
// not import test code, so this ~6-line encoding loop is re-implemented here
// rather than shared with that test helper.
func claudeProjectDirFor(workdir string) (string, error) {
	home, err := os.UserHomeDir()
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

// transcriptBlock is one entry of a transcript message's content array: a
// "tool_use" block (Name is the tool name, Input its arguments) or a "text" block
// (Text is the assistant's message text). Only the fields this audit reads are
// modeled — any other field Claude's transcript format carries is silently
// ignored by json.Unmarshal, exactly like ParseEvents' leniency for unrecognized
// fields (events.go).
type transcriptBlock struct {
	Type  string         `json:"type"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
	Text  string         `json:"text"`
}

// transcriptLine is one JSONL line of a Claude session transcript: Type
// discriminates "assistant"/"user"/other entry kinds, and Message.Content carries
// the assistant's content blocks (tool_use and text) this audit inspects. Lines of
// any other Type are skipped entirely by the callers below.
type transcriptLine struct {
	Type    string `json:"type"`
	Message struct {
		Content []transcriptBlock `json:"content"`
	} `json:"message"`
}

// readTranscriptLines reads path and leniently decodes it into transcriptLines,
// one per JSONL line: a blank line is skipped, and a line that fails to parse as
// JSON is skipped rather than aborting the whole read — mirroring ParseEvents'
// posture (events.go), since a transcript belonging to a run that raced this
// audit can end mid-line. The file itself failing to open is NOT tolerated here;
// that is the caller's job to classify (a missing parent transcript is an error,
// a missing subagents/ directory is not — see AuditForks).
func readTranscriptLines(path string) ([]transcriptLine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lines []transcriptLine
	for _, raw := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		var line transcriptLine
		if err := json.Unmarshal([]byte(trimmed), &line); err != nil {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// auditParentTranscript reads path (the parent session's own transcript) and
// extracts three provider-invariant fact groups the caller's policy interprets:
// spawnCalls is the total count of Agent tool_use invocations, and namedSpawns
// is how many of those carried a non-empty tool_input.name field — a defect
// signal the caller's fail-loud policy hard-errors on (named forks silently
// lose inherited context in Claude Code ≤2.1.206). writeCalls counts every
// Write/Edit/NotebookEdit tool_use block, and writes carries the file path of
// each one, in transcript order — read from the block's file_path input key,
// falling back to notebook_path for NotebookEdit; a block whose input carries
// neither key still increments writeCalls but contributes no entry to writes
// (skipped, not a panic). bashCommands carries the verbatim command input of
// every Bash tool_use block, in transcript order. A missing or unreadable
// parent transcript is an error: without it there is no way to know what the
// session actually spawned or did.
func auditParentTranscript(path string) (spawnCalls, namedSpawns, writeCalls int, writes, bashCommands []string, err error) {
	lines, err := readTranscriptLines(path)
	if err != nil {
		return 0, 0, 0, nil, nil, fmt.Errorf("claudeengine: read parent transcript %q: %w", path, err)
	}

	for _, line := range lines {
		if line.Type != "assistant" {
			continue
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
	}
	return spawnCalls, namedSpawns, writeCalls, writes, bashCommands, nil
}

// auditForkTranscript reads path (one fork subagent's own transcript) into a
// ForkReport: ToolCalls tallies every tool_use by tool name, AgentCalls and
// WriteCalls narrow that tally to the two tool families the fail-loud posture
// treats as defect signals (a nested Agent spawn; a Write/Edit/NotebookEdit
// mutation), BashCommands carries every Bash tool_use's command string verbatim
// and in order, and ReportReturned reports whether the LAST assistant-type
// message in the transcript carried a non-empty text block — the fork's own
// "report returned" signal. A missing or unreadable fork transcript is an error:
// the fork clearly ran (its transcript file exists in the caller's directory
// listing, or the caller would not have called this at all), so a read failure
// here is a real I/O problem, not a legitimate zero-fork finding.
func auditForkTranscript(path string) (shuttleengine.ForkReport, error) {
	lines, err := readTranscriptLines(path)
	if err != nil {
		return shuttleengine.ForkReport{}, fmt.Errorf("claudeengine: read fork transcript %q: %w", path, err)
	}

	report := shuttleengine.ForkReport{
		TranscriptPath: path,
		ToolCalls:      map[string]int{},
	}
	reportReturned := false
	for _, line := range lines {
		if line.Type != "assistant" {
			continue
		}
		// Overwritten (not OR-ed) on every assistant-type line, so this ends
		// up holding the LAST such message's hasText value — the "final
		// assistant message" ReportReturned actually means, not "any
		// assistant message ever had text".
		hasText := false
		for _, block := range line.Message.Content {
			switch block.Type {
			case "tool_use":
				report.ToolCalls[block.Name]++
				switch block.Name {
				case "Agent":
					report.AgentCalls++
				case "Write", "Edit", "NotebookEdit":
					report.WriteCalls++
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
	}
	report.ReportReturned = reportReturned

	return report, nil
}
