// editrecord_test.go pins the Codex edit hook.
// It checks the slim records RecordEdit appends for an apply_patch call, read back as file-edit signals.

package codexengine

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRecordEditAppendsAcrossCalls pins that a second hook call appends to the records an earlier call wrote.
// Each edit carries the hash of the file as that call read it.
func TestRecordEditAppendsAcrossCalls(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const sessionID = "01a1ea92-ca1c-7342-ade2-3f95786f1981"
	cwd := t.TempDir()
	recordsPath := filepath.Join(t.TempDir(), shuttleengine.EditRecordsFileName)
	target := filepath.Join(cwd, "a.go")
	engine := New()
	var want []string
	for _, content := range []string{"package a\n", "package b\n"} {
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatalf("write the patched file: %v", err)
		}
		payload, err := json.Marshal(map[string]any{
			"hook_event_name": hookEventPostToolUse,
			"session_id":      sessionID,
			"tool_name":       toolNameApplyPatch,
			"cwd":             cwd,
			"tool_input":      map[string]any{"command": "*** Begin Patch\n*** Update File: a.go\n@@\n+x\n*** End Patch"},
		})
		if err != nil {
			t.Fatalf("marshal the hook input: %v", err)
		}
		if err := engine.RecordEdit(recordsPath, payload); err != nil {
			t.Fatalf("RecordEdit() error: %v", err)
		}
		sum := sha256.Sum256([]byte(content))
		want = append(want, hex.EncodeToString(sum[:]))
	}
	data, err := os.ReadFile(recordsPath)
	if err != nil {
		t.Fatalf("read the edit records file: %v", err)
	}
	signals, _ := engine.ParseSessionSignals(data)
	var got []string
	for _, signal := range signals {
		got = append(got, signal.ContentHash)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("recorded content hashes = %v; want %v", got, want)
	}
}

// TestRecordEdit pins the records an apply_patch hook input appends and the file-edit signals ParseSessionSignals reads.
// Each file a patch touches is recorded once, resolved against the payload's cwd, with its content hash when readable.
// A move records the file it moves to, and a child thread's records name its thread id.
// The records leave out the patch text.
func TestRecordEdit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	at := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	const (
		sessionID   = "01a1ea92-ca1c-7342-ade2-3f95786f1981"
		childThread = "01a1eb00-1111-7222-8333-944455556666"
		mainRollout = "/s/rollout-2026-11-17T18-34-10-" + sessionID + ".jsonl"
	)
	// wantEdit is one file-edit signal a row wants, by the file's name in the payload's cwd.
	type wantEdit struct {
		file    string
		agentID string
	}
	const bareThreadRollout = "/s/" + childThread + ".jsonl"
	tests := []struct {
		name string
		// toolName is the payload's tool name; empty sends apply_patch.
		toolName string
		patch    string
		// transcript is the payload's transcript_path; empty sends the session's own rollout.
		transcript string
		// noCwd sends a payload with no cwd.
		noCwd bool
		// noSession sends a payload with no session_id.
		noSession bool
		// noTranscript sends a payload with no transcript_path.
		noTranscript bool
		// toolResponse, when set, is sent as the payload's tool_response.
		toolResponse any
		// files are written in the payload's cwd, by name, before the hook runs.
		files map[string]string
		// rawPayload, when set, is sent in place of the payload the other fields build.
		rawPayload string
		// missingDir puts the edit records file under a directory that does not exist, so it cannot be opened.
		missingDir bool
		want       []wantEdit
		// wantErr is a text the error names; empty wants none.
		wantErr string
	}{
		{
			name:  "AnUpdateOfAReadableFileCarriesItsContentHash",
			patch: "*** Begin Patch\n*** Update File: a.go\n@@\n-package x\n+package a\n*** End Patch",
			files: map[string]string{"a.go": "package a\n"},
			want:  []wantEdit{{file: "a.go"}},
		},
		{
			// gone.go does not exist after the delete, so its record carries no hash.
			name:  "EachFileOfAPatchIsRecordedOnce",
			patch: "*** Begin Patch\r\n*** Add File: b.go\r\n+package b\r\n*** Update File: a.go\r\n@@\r\n+// a\r\n*** Delete File: gone.go\r\n*** Update File: a.go\r\n@@\r\n+// again\r\n*** End Patch",
			files: map[string]string{"a.go": "package a\n// a\n// again\n", "b.go": "package b\n"},
			want:  []wantEdit{{file: "b.go"}, {file: "a.go"}, {file: "gone.go"}},
		},
		{
			name:  "AMoveRecordsTheFileItMovesTo",
			patch: "*** Begin Patch\n*** Update File: old.go\n*** Move to: new.go\n@@\n+package n\n*** End Patch",
			files: map[string]string{"new.go": "package n\n"},
			want:  []wantEdit{{file: "new.go"}},
		},
		{
			name:       "AChildThreadsPatchNamesItsThread",
			patch:      "*** Begin Patch\n*** Update File: a.go\n@@\n+package a\n*** End Patch",
			transcript: "/s/rollout-2026-11-17T18-40-00-" + childThread + ".jsonl",
			files:      map[string]string{"a.go": "package a\n"},
			want:       []wantEdit{{file: "a.go", agentID: childThread}},
		},
		{
			name:  "AHeaderWithLeadingWhitespaceIsRecorded",
			patch: "*** Begin Patch\n  *** Update File: a.go\n@@\n+package a\n*** End Patch",
			files: map[string]string{"a.go": "package a\n"},
			want:  []wantEdit{{file: "a.go"}},
		},
		{
			name:       "ABareThreadIDRolloutNamesItsThread",
			patch:      "*** Begin Patch\n*** Update File: a.go\n@@\n+package a\n*** End Patch",
			transcript: bareThreadRollout,
			files:      map[string]string{"a.go": "package a\n"},
			want:       []wantEdit{{file: "a.go", agentID: childThread}},
		},
		{
			name:       "AShortRolloutNameIsTheWholeThreadID",
			patch:      "*** Begin Patch\n*** Update File: a.go\n@@\n+package a\n*** End Patch",
			transcript: "/s/short.jsonl",
			files:      map[string]string{"a.go": "package a\n"},
			want:       []wantEdit{{file: "a.go", agentID: "short"}},
		},
		{
			name:         "ANoTranscriptPayloadNamesNoThread",
			patch:        "*** Begin Patch\n*** Update File: a.go\n@@\n+package a\n*** End Patch",
			noTranscript: true,
			files:        map[string]string{"a.go": "package a\n"},
			want:         []wantEdit{{file: "a.go"}},
		},
		{
			name:         "AnErrorFlagOfFalseIsRecorded",
			patch:        "*** Begin Patch\n*** Update File: a.go\n@@\n+package a\n*** End Patch",
			files:        map[string]string{"a.go": "package a\n"},
			toolResponse: map[string]any{"error": false},
			want:         []wantEdit{{file: "a.go"}},
		},
		{
			name:      "ANoSessionPayloadNamesNoThread",
			patch:     "*** Begin Patch\n*** Update File: a.go\n@@\n+package a\n*** End Patch",
			noSession: true,
			files:     map[string]string{"a.go": "package a\n"},
			want:      []wantEdit{{file: "a.go"}},
		},
		{
			name:         "AFailedPatchAppendsNothing",
			patch:        "*** Begin Patch\n*** Update File: a.go\n@@\n+package a\n*** End Patch",
			files:        map[string]string{"a.go": "package a\n"},
			toolResponse: map[string]any{"success": false},
		},
		{
			name:         "ARejectedPatchWithAnErrorAppendsNothing",
			patch:        "*** Begin Patch\n*** Update File: a.go\n@@\n+package a\n*** End Patch",
			files:        map[string]string{"a.go": "package a\n"},
			toolResponse: map[string]any{"error": "patch does not apply"},
		},
		{
			name:         "ASuccessfulResponseIsRecorded",
			patch:        "*** Begin Patch\n*** Update File: a.go\n@@\n+package a\n*** End Patch",
			files:        map[string]string{"a.go": "package a\n"},
			toolResponse: map[string]any{"success": true},
			want:         []wantEdit{{file: "a.go"}},
		},
		{name: "ARelativePathWithNoCwdAppendsNothing", patch: "*** Begin Patch\n*** Update File: a.go\n*** End Patch", noCwd: true},
		{name: "APatchNamingNoFileAppendsNothing", patch: "*** Begin Patch\n*** End Patch"},
		{name: "AnotherToolAppendsNothing", toolName: toolMatcherBash, patch: "*** Update File: a.go"},
		{name: "AMalformedPayloadIsAnError", rawPayload: `{not json`, wantErr: "parse the edit hook input"},
		{name: "AnUnopenableRecordsFileIsAnError", patch: "*** Update File: /w/a.go", missingDir: true, wantErr: "open the edit records file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recordsPath := filepath.Join(t.TempDir(), shuttleengine.EditRecordsFileName)
			if tt.missingDir {
				recordsPath = filepath.Join(t.TempDir(), "no-such-dir", shuttleengine.EditRecordsFileName)
			}
			cwd := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(cwd, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write the patched file %s: %v", name, err)
				}
			}
			payload := []byte(tt.rawPayload)
			if tt.rawPayload == "" {
				input := map[string]any{
					"hook_event_name": hookEventPostToolUse,
					"session_id":      sessionID,
					"transcript_path": cmp.Or(tt.transcript, mainRollout),
					"tool_name":       cmp.Or(tt.toolName, toolNameApplyPatch),
					"tool_input":      map[string]any{"command": tt.patch},
				}
				if !tt.noCwd {
					input["cwd"] = cwd
				}
				if tt.noSession {
					delete(input, "session_id")
				}
				if tt.noTranscript {
					delete(input, "transcript_path")
				}
				if tt.toolResponse != nil {
					input["tool_response"] = tt.toolResponse
				}
				var err error
				if payload, err = json.Marshal(input); err != nil {
					t.Fatalf("marshal the hook input: %v", err)
				}
			}
			engine := New()
			engine.now = func() time.Time { return at }

			err := engine.RecordEdit(recordsPath, payload)
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
			if strings.Contains(string(data), "***") || strings.Contains(string(data), "package") {
				t.Errorf("edit records file = %q; want no patch text or file content", data)
			}
			got, consumed := engine.ParseSessionSignals(data)
			if consumed != len(data) {
				t.Errorf("ParseSessionSignals consumed %d of %d bytes; want every record", consumed, len(data))
			}
			var want []shuttleengine.SessionSignal
			for _, edit := range tt.want {
				signal := shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalFileEdit, At: at, SessionID: sessionID, FilePath: filepath.Join(cwd, edit.file), AgentID: edit.agentID}
				if tt.noSession {
					signal.SessionID = ""
				}
				if content, ok := tt.files[edit.file]; ok {
					sum := sha256.Sum256([]byte(content))
					signal.ContentHash = hex.EncodeToString(sum[:])
				}
				want = append(want, signal)
			}
			for index := range got {
				got[index].Raw = nil
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("recorded edits =\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}
