// geometry.go declares the Geometry the Slack engine is told and the names of the files it holds.

package slackengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// The names of the files the engine holds.
// The token and the socket lock live in the per-user credential directory.
// The rest live in the hub's Slack directory.
const (
	// TokenFileName is the machine's credential file: the bot token, the app-level token and the app id.
	TokenFileName = "slack.json"
	// SocketLockFileName is the lock the holder of the machine's one Socket Mode connection holds.
	SocketLockFileName = "slack-socket.lock"
	// ChannelMapFileName is the hub's record of the Slack channel of each orch.
	ChannelMapFileName = "channels.json"
	// InboxDirName is the directory where the socket holder drops a wake hint for the hub's relay.
	InboxDirName = "inbox"
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
	// Machine is this machine's name, which is part of every channel name.
	Machine string
	// Dir is the hub's Slack directory.
	Dir string
	// TokenPath is the machine's bot token file.
	TokenPath string
	// ChannelMapPath is the hub's record of the Slack channel of each orch.
	ChannelMapPath string
	// InboxDir is the directory where the socket holder drops a wake hint for the hub's relay.
	InboxDir string
	// SocketLockPath is the lock the holder of the machine's one Socket Mode connection holds.
	SocketLockPath string
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

// LiveHubs lists the Slack geometries of the machine's live hubs.
// A failed read is the returned error, which a caller quotes as it is.
type LiveHubs func() ([]Geometry, error)

// recordLockPath returns the lock path of the record at path: a file in a locks directory beside the record.
func recordLockPath(path string) string {
	return filepath.Join(filepath.Dir(path), lymdirs.LocksDirName, filepath.Base(path)+".lock")
}
