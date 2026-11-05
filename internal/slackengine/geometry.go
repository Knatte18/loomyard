// geometry.go declares the Geometry the Slack engine is told and the names of the files it holds.

package slackengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// The names of the files the engine holds: the token in the per-user credential directory, the rest in the hub's Slack directory.
const (
	// TokenFileName is the machine's bot token file.
	TokenFileName = "slack.json"
	// SetupFileName is the hub's channel and operator record.
	SetupFileName = "setup.json"
	// PresenceFileName is the hub's presence record.
	PresenceFileName = "presence.json"
	// CursorFileName is the record of the newest Slack message the relay has read.
	CursorFileName = "cursor.json"
	// OutboxFileName is the record of the mail the relay has read and not yet finished posting.
	OutboxFileName = "outbox.json"
	// RelayStateFileName is the relay's state record.
	RelayStateFileName = "relay.json"
	// RelayLockFileName is the lock the live relay holds.
	RelayLockFileName = "relay.lock"
	// RelayLogFileName is the relay's log.
	RelayLogFileName = "relay.log"
)

// privateDirMode is the mode of a directory the engine creates, readable by its user alone.
const privateDirMode = 0o700

// Geometry is every path and fact the engine is told about one hub.
type Geometry struct {
	// HubPath is the hub's directory.
	HubPath string
	// Shortname is the hub's recorded shortname, the prefix of the relay's creel program name.
	Shortname string
	// Dir is the hub's Slack directory.
	Dir string
	// TokenPath is the machine's bot token file.
	TokenPath string
	// SetupPath is the hub's channel and operator record.
	SetupPath string
	// PresencePath is the hub's presence record.
	PresencePath string
	// CursorPath is the record of the newest Slack message the relay has read.
	CursorPath string
	// OutboxPath is the record of the mail the relay has read and not yet finished posting.
	OutboxPath string
	// RelayStatePath is the relay's state record.
	RelayStatePath string
	// LockPath is the lock the live relay holds.
	LockPath string
	// LogPath is the relay's log.
	LogPath string
	// RunProcDir is the directory of the hub's process owner records.
	RunProcDir string
	// LymExecutable is the path of the running `lym` binary, which the relay's start re-executes.
	LymExecutable string
}

// recordLockPath returns the lock path of the record at path: a file in a locks directory beside the record.
func recordLockPath(path string) string {
	return filepath.Join(filepath.Dir(path), lymdirs.LocksDirName, filepath.Base(path)+".lock")
}
