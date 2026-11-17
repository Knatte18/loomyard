// editrecord.go implements the edit hook: the PostToolUse hook command that runs on each Edit and Write call.
// It also holds RecordEdit, which appends one slim record of an edited file to the run's edit records file.
// Shuttle's edit-loop watch reads those records.

package claudeengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.EditRecorder = (*Claude)(nil)

// editRecordMatcher is the PostToolUse matcher of the edit hook: the tools that edit a file.
// No other tool call starts a process.
const editRecordMatcher = toolEdit + "|" + toolWrite

// editRecordHookCommand returns the PostToolUse command of the edit hook:
// the shuttle verb, told this engine's registry name and the run's edit records file at quotedRecordsPath.
// It ends in `; true`, so a missing or failing lym never shows the session a hook error.
func editRecordHookCommand(quotedRecordsPath string) string {
	return "lym shuttle " + shuttleengine.EditHookVerb + " " + engineName + " " + quotedRecordsPath + "; true"
}

// contentHashKey is the edit record's key for the hash of the edited file's content right after the edit.
// It is lym's own key, beside Claude Code's payload fields.
const contentHashKey = "content_hash"

// editRecord is the PostToolUse line the edit hook appends.
// It has Claude Code's own payload shape and holds only the fields a file edit reads.
// It adds the hash of the edited file's content.
// Claude Code's full payload carries the edit's text and the file's content.
// The edit records file must not hold them, since every watch of the run rereads it.
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

// RecordEdit appends the Edit or Write call a PostToolUse payload describes to the edit records file at recordsPath:
// a stamp line with the hook-side time, then an editRecord, in one write.
// The record carries the SHA-256 of the edited file's content as the hook reads it,
// or no hash when the file cannot be read, such as after a later edit removed it.
// A payload naming another tool or no file path appends nothing; a malformed payload or a failed write is an error.
func (c *Claude) RecordEdit(recordsPath string, payload []byte) error {
	var record editRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return fmt.Errorf("claudeengine: parse the edit hook input: %w", err)
	}
	if (record.ToolName != toolEdit && record.ToolName != toolWrite) || record.ToolInput.FilePath == "" {
		return nil
	}
	record.HookEventName = hookEventPostToolUse
	record.ContentHash = fileContentHash(record.ToolInput.FilePath)
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("claudeengine: marshal the edit record: %w", err)
	}
	stamp, err := json.Marshal(map[string]string{stampHookKey: hookEventPostToolUse, stampAtKey: c.now().UTC().Format(time.RFC3339)})
	if err != nil {
		return fmt.Errorf("claudeengine: marshal the edit record's stamp: %w", err)
	}
	file, err := os.OpenFile(recordsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("claudeengine: open the edit records file %s: %w", recordsPath, err)
	}
	_, writeErr := file.Write(append(append(append(stamp, '\n'), line...), '\n'))
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("claudeengine: append the edit record to %s: %w", recordsPath, err)
	}
	return nil
}

// fileContentHash returns the hex SHA-256 of the content of the file at filePath, or "" when it cannot be read.
// An unreadable file only costs the edit-loop watch this one edit, so the hook records the edit without a hash.
func fileContentHash(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}
