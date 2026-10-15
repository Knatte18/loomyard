// geometry.go declares the paths creelengine is told, the socket name derived from a hub, and the clock seam.

package creelengine

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Geometry is the set of absolute paths the engine works on.
// The engine is told every one and derives none: its constructor's caller resolves them.
type Geometry struct {
	// HubPath is the hub container the daemon serves.
	HubPath string
	// Shortname is the hub's agent-name shortname.
	Shortname string
	// StateDir is the hub's creel state directory, <hub>/_board/.lyx/creel.
	StateDir string
	// JournalDir holds the journal files, one per UTC day.
	JournalDir string
	// LockPath is the daemon's single-instance lock file in StateDir.
	LockPath string
	// SocketPath is the daemon's unix socket.
	SocketPath string
}

// socketHashHexLen is how many hex digits of the hub path's hash the socket name carries.
const socketHashHexLen = 8

// SocketName returns the file name of a hub's daemon socket, creel-<shortname>-<h>.sock.
// <h> is the first 8 hex digits of the SHA-256 of hubPath, so two hubs sharing a shortname get different sockets.
func SocketName(shortname, hubPath string) string {
	sum := sha256.Sum256([]byte(hubPath))
	return "creel-" + shortname + "-" + hex.EncodeToString(sum[:])[:socketHashHexLen] + ".sock"
}

// Clock is the time seam: every wait and timestamp of the engine goes through it, so a test never waits out an interval.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// After returns a channel that receives once d has elapsed.
	After(d time.Duration) <-chan time.Time
}
