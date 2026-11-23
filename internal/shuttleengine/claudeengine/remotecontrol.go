// remotecontrol.go implements shuttleengine.RemoteControlReader for Claude.
// It reads whether a session's remote control connected from the record Claude Code writes into the session's transcript.

package claudeengine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.RemoteControlReader = (*Claude)(nil)

// bridgeSessionRecordType is the transcript record type Claude Code writes once a session's remote control connects.
// A connection the provider refuses, such as one an organization's policy disables, writes none.
const bridgeSessionRecordType = "bridge-session"

// RemoteControlCheckpoint returns the size of the transcript of the session sessionID run in workdir.
// A session with no transcript yet returns zero.
func (c *Claude) RemoteControlCheckpoint(sessionID, workdir string) (int64, error) {
	projectDir, err := c.sessionProjectDir(sessionID, workdir)
	if err != nil {
		return 0, err
	}
	path := transcriptPathIn(projectDir, sessionID)
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("claudeengine: read the size of transcript %s: %w", path, err)
	}
	return info.Size(), nil
}

// RemoteControlConnected reports whether the session sessionID run in workdir recorded its remote control connecting.
// Only the transcript from the byte offset checkpoint on is read.
// A session with no transcript records no connection.
func (c *Claude) RemoteControlConnected(sessionID, workdir string, checkpoint int64) (bool, error) {
	projectDir, err := c.sessionProjectDir(sessionID, workdir)
	if err != nil {
		return false, err
	}
	return bridgeSessionRecordedIn(transcriptPathIn(projectDir, sessionID), checkpoint)
}

// bridgeSessionRecordedIn reports whether the transcript at path holds a bridge-session record at or past checkpoint.
// A transcript shorter than checkpoint has been rewritten since and is read whole.
// A missing transcript holds none, and a line still being written is not read as a record.
func bridgeSessionRecordedIn(path string, checkpoint int64) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claudeengine: open transcript %s: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("claudeengine: read the size of transcript %s: %w", path, err)
	}
	if checkpoint > info.Size() {
		checkpoint = 0
	}
	if _, err := file.Seek(checkpoint, io.SeekStart); err != nil {
		return false, fmt.Errorf("claudeengine: seek transcript %s: %w", path, err)
	}
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadBytes('\n')
		if bytes.Contains(line, []byte(bridgeSessionRecordType)) && isBridgeSessionRecord(line) {
			return true, nil
		}
		if errors.Is(readErr, io.EOF) {
			return false, nil
		}
		if readErr != nil {
			return false, fmt.Errorf("claudeengine: read transcript %s: %w", path, readErr)
		}
	}
}

// isBridgeSessionRecord reports whether line is a bridge-session record naming the remote session it connected.
func isBridgeSessionRecord(line []byte) bool {
	var record struct {
		Type            string `json:"type"`
		BridgeSessionID string `json:"bridgeSessionId"`
	}
	if json.Unmarshal(line, &record) != nil {
		return false
	}
	return record.Type == bridgeSessionRecordType && record.BridgeSessionID != ""
}
