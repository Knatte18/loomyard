// daemonstate.go implements the supervised-strategy daemon's runtime state file: the JSON record a
// spawning EnsureServer call writes so a later, independent lyx invocation can discover an
// already-running daemon rather than spawning its own, plus the two-part staleness check that
// decides whether a recorded daemon is still safe to reuse.
// It also declares DaemonStateFile/DaemonLock, the module's own told, anchorRoot-joined accessors —
// ensureSupervised, the sole production caller, resolves its state-file and lock paths through these
// rather than any other package's helper.

package scoutengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/proc"
)

// supervisedProtocolVersion is lyx's wire-compatibility version for the
// supervised daemon protocol, distinct from gopls's version.
// It detects when a still-running daemon was spawned by an incompatible lyx binary.
const supervisedProtocolVersion = "1"

// scoutDirName is the relative-path segment scoutengine joins onto
// lyxdirs.DotLyxDirName to form the supervised daemon's runtime-state directory.
// scoutengine is this segment's sole declarer.
const scoutDirName = "scout"

// DaemonStateFile returns the path to the scout daemon's runtime state file for the given language,
// joined onto the told anchorRoot.
// It is anchor-joined so the state tree is a directory sibling of the durable _lyx tree, and the
// daemon remains a per-worktree, per-language singleton — an anchored repo has exactly one anchor.
// It lives under .lyx (ephemeral) not _lyx (durable) so PIDs/sockets don't get committed.
// Relocating this path to the anchor deliberately re-keys the supervised daemon in subpath-anchored
// repos, since the daemon's identity is its state-file path — the intended consequence of having
// exactly one .lyx root per worktree.
func DaemonStateFile(anchorRoot string, lang string) string {
	return filepath.Join(anchorRoot, lyxdirs.DotLyxDirName, scoutDirName, lang, "daemon.json")
}

// DaemonLock returns the path to the advisory lock file guarding concurrent access to
// DaemonStateFile(anchorRoot, lang).
// It shares that function's anchoring, per-lang scoping, and daemon-re-keying consequence in
// subpath-anchored repos.
func DaemonLock(anchorRoot string, lang string) string {
	return filepath.Join(anchorRoot, lyxdirs.DotLyxDirName, scoutDirName, lang, "daemon.lock")
}

// daemonState is the JSON shape written to the supervised daemon's state file.
// Address is the dial target in "network;addr" form; StartedAt is RFC3339 format.
type daemonState struct {
	PID             int    `json:"pid"`
	Address         string `json:"address"`
	ProtocolVersion string `json:"protocol_version"`
	StartedAt       string `json:"started_at"`
}

// readDaemonState reads and parses the daemon state file at path.
// Missing files return (daemonState{}, false, nil) rather than an error;
// other errors are wrapped and returned with found=false.
func readDaemonState(path string) (daemonState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return daemonState{}, false, nil
		}
		return daemonState{}, false, fmt.Errorf("scoutengine: read daemon state file %s: %w", path, err)
	}

	var s daemonState
	if err := json.Unmarshal(data, &s); err != nil {
		return daemonState{}, false, fmt.Errorf("scoutengine: unmarshal daemon state file %s: %w", path, err)
	}
	return s, true, nil
}

// writeDaemonState marshals s and writes it to path atomically
// (via temp-file-then-rename) to ensure concurrent readers never observe partial writes.
func writeDaemonState(path string, s daemonState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("scoutengine: create daemon state dir %s: %w", filepath.Dir(path), err)
	}

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("scoutengine: marshal daemon state: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("scoutengine: write daemon state temp file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("scoutengine: rename daemon state temp file %s to %s: %w", tmpPath, path, err)
	}
	return nil
}

// daemonStale reports whether the daemon state s is unusable.
// It checks whether the PID is still alive and whether the protocol version matches.
func daemonStale(s daemonState) bool {
	return !proc.IsAlive(s.PID) || s.ProtocolVersion != supervisedProtocolVersion
}
