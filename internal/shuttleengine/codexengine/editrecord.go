// editrecord.go implements the Codex edit hook: the PostToolUse hook command that runs on each apply_patch call.
// It also holds RecordEdit, which appends one slim record per file a patch touched to the run's edit records file.
// Shuttle's edit-loop watch reads those records.

package codexengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.EditRecorder = (*Codex)(nil)

// editRecordHookCommand returns the PostToolUse command of the edit hook over the run's edit records file at recordsPath:
// the shuttle verb, told this engine's registry name.
// The verb prints nothing, so the command prints the empty hook output after it,
// and a missing or failing lym never shows the session a hook error.
func editRecordHookCommand(recordsPath string) string {
	return "lym shuttle " + shuttleengine.EditHookVerb + " " + engineName + " " + shell.Posix().Quote(recordsPath) + "; " + emptyHookOutput
}

// contentHashKey is the edit record's key for the hash of the edited file's content right after the edit.
// It is lym's own key, beside the fields of Codex's payload.
const contentHashKey = "content_hash"

// editHookInput is the part of Codex's PostToolUse input for an apply_patch call that the edit hook reads.
type editHookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	ToolName       string `json:"tool_name"`
	ToolInput      struct {
		// Command is the patch text.
		Command string `json:"command"`
	} `json:"tool_input"`
	// ToolResponse is the call's outcome, whose shape Codex does not fix; only a failure marker in an object is read.
	ToolResponse json.RawMessage `json:"tool_response"`
}

// patchFailed reports whether a tool_response names a failed or rejected apply_patch call:
// an object with "success" false or an "error" that is a non-empty string or object.
// Any other response, including an absent or non-object one, reads as a success.
func patchFailed(toolResponse json.RawMessage) bool {
	var response struct {
		Success *bool `json:"success"`
		Error   any   `json:"error"`
	}
	if err := json.Unmarshal(toolResponse, &response); err != nil {
		return false
	}
	if response.Success != nil && !*response.Success {
		return true
	}
	switch value := response.Error.(type) {
	case string:
		return value != ""
	case map[string]any:
		return len(value) > 0
	}
	return false
}

// editRecord is the PostToolUse line the edit hook appends for one file a patch touched.
// It holds only the fields a file edit reads, and the hash of the file's content after the patch.
// It never holds the patch text, since every watch of the run rereads the edit records file.
type editRecord struct {
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
	ToolName      string `json:"tool_name"`
	ToolInput     struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
	ContentHash string `json:"content_hash,omitempty"`
}

// The file headers of Codex's patch format.
// An update header may be followed by a move header that names the file the update leaves.
const (
	patchAddFile    = "*** Add File: "
	patchUpdateFile = "*** Update File: "
	patchDeleteFile = "*** Delete File: "
	patchMoveTo     = "*** Move to: "
)

// threadIDLength is the length of a Codex thread id, a UUID in text form, which ends a rollout's file name.
const threadIDLength = 36

// RecordEdit appends the files that the apply_patch call of a Codex PostToolUse payload touched.
// They go to the edit records file at recordsPath.
// Each file gets a stamp line with the hook-side time, then an editRecord; all of them go in one write.
// A file is the one an add, update or delete header names, or the file a move header moves an update to.
// A relative path is resolved against the payload's cwd, and one with no cwd is skipped.
// A file the patch names twice is recorded once.
// The record carries the SHA-256 of the file's content as the hook reads it,
// or no hash when the file cannot be read, such as after a delete.
// It names the child thread that made the patch, by the thread id its rollout's name ends in.
// A payload naming another tool or no file, or whose tool_response reports a failed patch, appends nothing;
// a malformed payload or a failed write is an error.
func (c *Codex) RecordEdit(recordsPath string, payload []byte) error {
	var input editHookInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return fmt.Errorf("codexengine: parse the edit hook input: %w", err)
	}
	if input.ToolName != toolNameApplyPatch || patchFailed(input.ToolResponse) {
		return nil
	}
	stamp, err := json.Marshal(map[string]string{stampHookKey: hookEventPostToolUse, stampAtKey: c.now().UTC().Format(time.RFC3339)})
	if err != nil {
		return fmt.Errorf("codexengine: marshal the edit record's stamp: %w", err)
	}
	agentID := childThreadID(input.SessionID, input.TranscriptPath)
	var lines []byte
	for _, file := range patchedFiles(input.ToolInput.Command, input.Cwd) {
		record := editRecord{HookEventName: hookEventPostToolUse, SessionID: input.SessionID, AgentID: agentID, ToolName: input.ToolName}
		record.ToolInput.FilePath = file
		record.ContentHash = shuttleengine.FileContentHash(file)
		line, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("codexengine: marshal the edit record: %w", err)
		}
		lines = append(append(append(append(lines, stamp...), '\n'), line...), '\n')
	}
	if len(lines) == 0 {
		return nil
	}
	file, err := os.OpenFile(recordsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("codexengine: open the edit records file %s: %w", recordsPath, err)
	}
	_, writeErr := file.Write(lines)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("codexengine: append the edit records to %s: %w", recordsPath, err)
	}
	return nil
}

// patchedFiles returns the absolute paths of the files the patch text touches, each once, in patch order.
// A move header replaces the path of the update before it.
// A relative path is resolved against cwd, and one with no cwd to resolve it against is left out.
func patchedFiles(patch, cwd string) []string {
	var named []string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, patchAddFile), strings.HasPrefix(line, patchUpdateFile), strings.HasPrefix(line, patchDeleteFile):
			named = append(named, strings.TrimSpace(line[strings.Index(line, ": ")+2:]))
		case strings.HasPrefix(line, patchMoveTo) && len(named) > 0:
			named[len(named)-1] = strings.TrimSpace(strings.TrimPrefix(line, patchMoveTo))
		}
	}
	var files []string
	for _, path := range named {
		resolved := resolvePath(cwd, path)
		if resolved != "" && !slices.Contains(files, resolved) {
			files = append(files, resolved)
		}
	}
	return files
}

// childThreadID returns the thread id of the child thread a payload comes from, or "" for the session's own thread.
// A Codex payload carries no agent id, and a child's carries its parent's session id,
// but its transcript_path names the thread's own rollout, whose file name ends in the thread's id.
// A rollout whose name ends in the session id is the session's own.
func childThreadID(sessionID, transcriptPath string) string {
	if sessionID == "" || transcriptPath == "" {
		return ""
	}
	name := strings.TrimSuffix(filepath.Base(transcriptPath), filepath.Ext(transcriptPath))
	if strings.HasSuffix(name, sessionID) {
		return ""
	}
	if len(name) > threadIDLength {
		return name[len(name)-threadIDLength:]
	}
	return name
}
