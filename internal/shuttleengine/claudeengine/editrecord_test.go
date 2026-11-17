// editrecord_test.go pins the edit hook: the slim record RecordEdit appends, read back as a file-edit signal,
// and the settings entry that runs it.

package claudeengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRecordEdit pins the record an edit hook input appends and the file-edit signal ParseSessionSignals reads from it.
// The record leaves out the edit's text and the file's content the input carries,
// and it carries the hash of the edited file's content when the file can be read.
func TestRecordEdit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	at := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	const validEdit = `{"hook_event_name":"PostToolUse","tool_name":"Edit","tool_input":{"file_path":"/w/a.go"}}`
	const editedContent = "package a\n"
	contentSum := sha256.Sum256([]byte(editedContent))
	tests := []struct {
		name    string
		payload string
		// content, when set, is written to a file in a temporary directory before the hook runs.
		// That file's path then stands for editedFilePath in the payload and in want.
		content string
		// missingDir puts the edit records file under a directory that does not exist, so it cannot be opened.
		missingDir bool
		// want is the signals the edit records file reads as; nil wants nothing appended.
		want []shuttleengine.SessionSignal
		// wantErr is a text the error names; empty wants none.
		wantErr string
	}{
		{
			name:    "AnEditOfAReadableFileCarriesItsContentHash",
			payload: `{"hook_event_name":"PostToolUse","session_id":"sess-1","tool_name":"Edit","tool_input":{"file_path":"` + editedFilePath + `","old_string":"x","new_string":"y"}}`,
			content: editedContent,
			want:    []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalFileEdit, At: at, SessionID: "sess-1", FilePath: editedFilePath, ContentHash: hex.EncodeToString(contentSum[:])}},
		},
		{
			// The file /w/a.go does not exist, so the edit is recorded with no content hash.
			name:    "AnEditOfTheSessionIsAFileEdit",
			payload: `{"hook_event_name":"PostToolUse","session_id":"sess-1","tool_name":"Edit","tool_input":{"file_path":"/w/a.go","old_string":"x","new_string":"y"},"tool_response":{"originalFile":"package a"}}`,
			want:    []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalFileEdit, At: at, SessionID: "sess-1", FilePath: "/w/a.go"}},
		},
		{
			name:    "ASubagentsWriteNamesTheAgent",
			payload: `{"hook_event_name":"PostToolUse","session_id":"sess-1","agent_id":"agent-1","tool_name":"Write","tool_input":{"file_path":"/w/b.go","content":"package b"}}`,
			want:    []shuttleengine.SessionSignal{{Kind: shuttleengine.SessionSignalFileEdit, At: at, SessionID: "sess-1", FilePath: "/w/b.go", AgentID: "agent-1"}},
		},
		{name: "AnotherToolAppendsNothing", payload: `{"tool_name":"Read","tool_input":{"file_path":"/w/a.go"}}`},
		{name: "NoFilePathAppendsNothing", payload: `{"tool_name":"Edit","tool_input":{}}`},
		{name: "AMalformedPayloadIsAnError", payload: `{not json`, wantErr: "parse the edit hook input"},
		{name: "AnUnopenableRecordsFileIsAnError", payload: validEdit, missingDir: true, wantErr: "open the edit records file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recordsPath := filepath.Join(t.TempDir(), shuttleengine.EditRecordsFileName)
			if tt.missingDir {
				recordsPath = filepath.Join(t.TempDir(), "no-such-dir", shuttleengine.EditRecordsFileName)
			}
			payload, want := tt.payload, slices.Clone(tt.want)
			if tt.content != "" {
				filePath := filepath.Join(t.TempDir(), "a.go")
				if err := os.WriteFile(filePath, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("write the edited file: %v", err)
				}
				quotedPath, err := json.Marshal(filePath)
				if err != nil {
					t.Fatalf("quote the edited file's path: %v", err)
				}
				payload = strings.ReplaceAll(payload, `"`+editedFilePath+`"`, string(quotedPath))
				for index := range want {
					want[index].FilePath = filePath
				}
			}
			engine := New()
			engine.now = func() time.Time { return at }
			err := engine.RecordEdit(recordsPath, []byte(payload))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("RecordEdit() error = %v; want one naming %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("RecordEdit() error: %v", err)
			}
			data, err := os.ReadFile(recordsPath)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("read the edit records file: %v", err)
			}
			if want == nil && len(data) != 0 {
				t.Errorf("edit records file = %q; want nothing appended", data)
			}
			for _, carried := range []string{"old_string", "new_string", "originalFile", `"content"`, "package a"} {
				if strings.Contains(string(data), carried) {
					t.Errorf("edit records file = %q; want no %s from the hook input or the edited file", data, carried)
				}
			}
			got, consumed := engine.ParseSessionSignals(data)
			if consumed != len(data) {
				t.Errorf("ParseSessionSignals consumed %d of %d bytes; want the whole record", consumed, len(data))
			}
			assertSignals(t, "recorded edit", got, want)
		})
	}
}

// editedFilePath stands in a TestRecordEdit row's payload and want for the path of the file the row writes.
const editedFilePath = "EDITED_FILE"

// TestBuildSettings_EditHook pins that the settings carry the edit hook, which names the edit records file.
// They carry it exactly when the edit-loop watch is on and the run is not interactive.
func TestBuildSettings_EditHook(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const eventsPath = "/run/events.jsonl"
	tests := []struct {
		name        string
		edits       int
		interactive bool
		wantHook    bool
	}{
		{name: "WatchOnInstallsTheHook", edits: 4, wantHook: true},
		{name: "WatchOffInstallsNone", edits: 0},
		{name: "AnInteractiveRunGetsNone", edits: 4, interactive: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := shuttleengine.Config{EditLoopEdits: tt.edits, EditLoopWindowS: 300}
			data, err := buildSettings(eventsPath, tt.interactive, cfg, templateConfig(t), false, false, "", "", "", "", sessionScope{})
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			var got []string
			for _, raw := range hooksFor(parseSettings(t, data), "PostToolUse") {
				entry, _ := raw.(map[string]any)
				if entry["matcher"] != editRecordMatcher {
					continue
				}
				hooks, _ := entry["hooks"].([]any)
				for _, hook := range hooks {
					command, _ := hook.(map[string]any)["command"].(string)
					got = append(got, command)
				}
			}
			var want []string
			if tt.wantHook {
				want = []string{editRecordHookCommand(shQuote("/run/" + shuttleengine.EditRecordsFileName))}
			}
			if !slices.Equal(got, want) {
				t.Errorf("edit hook commands = %q; want %q", got, want)
			}
		})
	}
}
