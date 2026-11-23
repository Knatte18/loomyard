// remotecontrol_test.go covers which Claude transcripts read as a connected remote control,
// and the checkpoint and connection reads over a session's transcript on disk.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	userLine   = `{"type":"user","message":{"role":"user","content":"go"}}` + "\n"
	bridgeLine = `{"type":"bridge-session","sessionId":"s","bridgeSessionId":"cse_01","lastSequenceNum":0}` + "\n"
)

// TestBridgeSessionRecordedIn pins which transcripts read as a session whose remote control connected.
func TestBridgeSessionRecordedIn(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const (
		policyLine = `{"type":"system","subtype":"local_command",` +
			`"content":"<local-command-stdout>Remote Control is disabled by your organization's policy.</local-command-stdout>"}` + "\n"
		quotedLine          = `{"type":"user","message":{"role":"user","content":"a \"bridge-session\" record"}}` + "\n"
		bridgeWithoutIDLine = `{"type":"bridge-session","sessionId":"s"}` + "\n"
	)
	tests := []struct {
		name string
		// transcript is the file's content; absent leaves no file.
		transcript string
		absent     bool
		// checkpointAfter sets the checkpoint to the end of this prefix of transcript.
		checkpointAfter string
		// checkpoint is used as is when checkpointAfter is empty.
		checkpoint int64
		want       bool
	}{
		{name: "BridgeRecordConnects", transcript: userLine + bridgeLine, want: true},
		{name: "PolicyRefusalRecordsNoConnection", transcript: userLine + policyLine},
		{name: "BridgeRecordBeforeTheCheckpointIsOld", transcript: bridgeLine + userLine + policyLine, checkpointAfter: bridgeLine + userLine},
		{name: "BridgeRecordAtTheCheckpointConnects", transcript: userLine + bridgeLine, checkpointAfter: userLine, want: true},
		{name: "ShrunkTranscriptIsReadWhole", transcript: bridgeLine, checkpoint: 1 << 20, want: true},
		{name: "RecordTypeQuotedInAMessageIsNoConnection", transcript: quotedLine},
		{name: "BridgeRecordWithoutASessionIDIsNoConnection", transcript: userLine + bridgeWithoutIDLine},
		{name: "BridgeRecordStillBeingWrittenIsNotRead", transcript: userLine + strings.TrimSuffix(bridgeLine, "\"lastSequenceNum\":0}\n")},
		{name: "MissingTranscriptRecordsNoConnection", absent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "s.jsonl")
			if !tt.absent {
				if err := os.WriteFile(path, []byte(tt.transcript), 0o644); err != nil {
					t.Fatalf("write the transcript: %v", err)
				}
			}
			checkpoint := tt.checkpoint
			if tt.checkpointAfter != "" {
				checkpoint = int64(len(tt.checkpointAfter))
			}

			got, err := bridgeSessionRecordedIn(path, checkpoint)

			if err != nil {
				t.Fatalf("bridgeSessionRecordedIn() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("bridgeSessionRecordedIn(%d) = %v; want %v", checkpoint, got, tt.want)
			}
		})
	}
}

// TestClaude_RemoteControlReadsTheSessionTranscript pins the checkpoint as the transcript's size.
// A missing transcript checkpoints at zero.
// It also pins that the connection read looks in that same transcript for a bridge-session record past the checkpoint.
// It is not parallel: it points HOME at a fake Claude config directory.
func TestClaude_RemoteControlReadsTheSessionTranscript(t *testing.T) {
	tiergate.Require(t, 0)
	const (
		workdir   = "/work/tree"
		sessionID = "sess-1"
	)
	tests := []struct {
		name string
		// absent leaves no transcript before the checkpoint is taken.
		absent bool
		// before is the transcript when the checkpoint is taken, and after is appended to it before the connection is read.
		before, after  string
		wantCheckpoint int64
		wantConnected  bool
	}{
		{name: "MissingTranscriptCheckpointsAtZeroAndRecordsNoConnection", absent: true},
		{name: "CheckpointIsTheTranscriptSize", before: userLine, wantCheckpoint: int64(len(userLine))},
		{name: "BridgeRecordAppendedPastTheCheckpointConnects", before: userLine, after: bridgeLine, wantCheckpoint: int64(len(userLine)), wantConnected: true},
		{name: "BridgeRecordBeforeTheCheckpointIsOld", before: bridgeLine + userLine, wantCheckpoint: int64(len(bridgeLine + userLine))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv(claudeConfigDirEnv, "")
			projectDir := projectDirUnder(filepath.Join(home, claudeConfigDirName, projectsDirName), workdir)
			path := filepath.Join(projectDir, sessionID+".jsonl")
			if err := os.MkdirAll(projectDir, 0o755); err != nil {
				t.Fatalf("make the project directory: %v", err)
			}
			engine := New()
			if !tt.absent {
				if err := os.WriteFile(path, []byte(tt.before), 0o644); err != nil {
					t.Fatalf("write the transcript: %v", err)
				}
			}

			checkpoint, err := engine.RemoteControlCheckpoint(sessionID, workdir)
			if err != nil || checkpoint != tt.wantCheckpoint {
				t.Fatalf("RemoteControlCheckpoint() = %d, %v; want %d, nil", checkpoint, err, tt.wantCheckpoint)
			}
			if !tt.absent {
				if err := os.WriteFile(path, []byte(tt.before+tt.after), 0o644); err != nil {
					t.Fatalf("append to the transcript: %v", err)
				}
			}
			connected, err := engine.RemoteControlConnected(sessionID, workdir, checkpoint)

			if err != nil || connected != tt.wantConnected {
				t.Errorf("RemoteControlConnected() = %v, %v; want %v, nil", connected, err, tt.wantConnected)
			}
		})
	}
}
