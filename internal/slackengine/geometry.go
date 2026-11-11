// geometry.go declares the Geometry the Slack engine is told and the names of the files it holds.

package slackengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// The names of the files the engine holds.
// The token, the socket lock and the presence live in the per-user credential directory.
// Every hub of the machine shares them.
// The rest live in the hub's Slack directory.
const (
	// TokenFileName is the machine's credential file: the bot token, the app-level token and the app id.
	TokenFileName = "slack.json"
	// SocketLockFileName is the lock the holder of the machine's one Socket Mode connection holds.
	SocketLockFileName = "slack-socket.lock"
	// PresenceFileName is the machine's presence record.
	PresenceFileName = "slack-presence.json"
	// ForwardingFileName is the hub's record of whether each orch's slack button is on.
	ForwardingFileName = "forwarding.json"
	// ChannelMapFileName is the hub's record of the Slack channel of each orch.
	// The channels an older map named `channels.json` recorded are never read, so the relay leaves them alone.
	ChannelMapFileName = "orch-channels.json"
	// InboxDirName is the directory where the socket holder drops a wake hint for the hub's relay.
	InboxDirName = "inbox"
	// SetupFileName is the hub's operator record.
	SetupFileName = "setup.json"
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

// MachinePaths are the paths of the machine's own Slack files, which every hub of the machine shares.
type MachinePaths struct {
	// TokenPath is the machine's credential file.
	TokenPath string
	// SocketLockPath is the lock the holder of the machine's one Socket Mode connection holds.
	SocketLockPath string
	// PresencePath is the machine's presence record.
	PresencePath string
}

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
	// SetupPath is the hub's operator record.
	SetupPath string
	// PresencePath is the machine's presence record, which every hub of the machine shares.
	PresencePath string
	// ForwardingPath is the hub's record of whether each orch's slack button is on.
	ForwardingPath string
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

// MachinePaths returns the paths of the machine's own Slack files that the geometry carries.
func (geom Geometry) MachinePaths() MachinePaths {
	return MachinePaths{TokenPath: geom.TokenPath, SocketLockPath: geom.SocketLockPath, PresencePath: geom.PresencePath}
}

// LiveHubs lists the Slack geometries of the machine's live hubs.
// A failed read is the returned error, which a caller quotes as it is.
type LiveHubs func() ([]Geometry, error)

// recordLockPath returns the lock path of the record at path: a file in a locks directory beside the record.
func recordLockPath(path string) string {
	return filepath.Join(filepath.Dir(path), lymdirs.LocksDirName, filepath.Base(path)+".lock")
}
