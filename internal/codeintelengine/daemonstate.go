// daemonstate.go implements the supervised-strategy daemon's runtime state
// file: the JSON record a spawning EnsureServer call writes so a later,
// independent lyx invocation can discover an already-running daemon rather
// than spawning its own, plus the two-part staleness check that decides
// whether a recorded daemon is still safe to reuse. This file does no
// filesystem-location resolution of its own — every function here takes a
// plain caller-supplied path string, leaving hubgeometry.Layout.
// CodeintelDaemonStateFile/CodeintelDaemonLock resolution to batch 6's
// ensureSupervised, the sole production caller.

package codeintelengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/proc"
)

// supervisedProtocolVersion is lyx's own wire-compatibility version for the
// supervised daemon protocol — NOT gopls's version. It exists to detect "a
// still-running daemon was spawned by an older lyx binary whose
// expectations of the daemon no longer match this binary's" and is bumped
// by a future lyx change to the supervised protocol itself, never by a
// gopls upgrade. Do not confuse this with registry.Entry.PinnedVersion,
// which pins the gopls binary version resolveGoToolchain installs.
const supervisedProtocolVersion = "1"

// daemonState is the JSON shape written to the supervised daemon's state
// file. Address is the dial target in "network;addr" form (e.g.
// "unix;/path/to/sock"), matching gopls serve -listen's own flag syntax so
// the recorded value can be split on the same ";" the daemon itself was
// told to listen on. StartedAt is RFC3339
// (time.Now().UTC().Format(time.RFC3339)); it is produced by the caller,
// not this file — daemonState itself does no clock reads, keeping this
// file trivially unit-testable.
type daemonState struct {
	PID             int    `json:"pid"`
	Address         string `json:"address"`
	ProtocolVersion string `json:"protocol_version"`
	StartedAt       string `json:"started_at"`
}

// readDaemonState reads and parses the daemon state file at path. A missing
// file is the common case on a worktree's first EnsureServer call, so it is
// translated into (daemonState{}, false, nil) rather than an error, mirroring
// os.IsNotExist's own "absent means no recorded state" semantics. Any other
// read error is wrapped and returned; a successful read is json.Unmarshal'd
// and returned with found=true.
func readDaemonState(path string) (daemonState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return daemonState{}, false, nil
		}
		return daemonState{}, false, fmt.Errorf("codeintelengine: read daemon state file %s: %w", path, err)
	}

	var s daemonState
	if err := json.Unmarshal(data, &s); err != nil {
		return daemonState{}, false, fmt.Errorf("codeintelengine: unmarshal daemon state file %s: %w", path, err)
	}
	return s, true, nil
}

// writeDaemonState marshals s and writes it to path via a temp-file-then-
// rename sequence: it writes to path+".tmp" and then os.Renames it onto
// path. This is what makes a concurrent reader (a losing EnsureServer
// caller polling this same path, batch 6) never observe a partially-written
// file — os.Rename atomically replaces an existing destination on both
// Linux and Windows (Go's Windows implementation already uses
// MOVEFILE_REPLACE_EXISTING).
func writeDaemonState(path string, s daemonState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("codeintelengine: create daemon state dir %s: %w", filepath.Dir(path), err)
	}

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("codeintelengine: marshal daemon state: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("codeintelengine: write daemon state temp file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("codeintelengine: rename daemon state temp file %s to %s: %w", tmpPath, path, err)
	}
	return nil
}

// daemonStale reports whether the recorded daemon state s should be treated
// as unusable, forcing a fresh spawn rather than a reuse. Either half of
// this two-part check alone is sufficient to force a restart, per the
// design's "two-part staleness" decision: the recorded PID is no longer
// alive (the daemon process is gone), or the recorded protocol version does
// not match this binary's supervisedProtocolVersion (a still-running daemon
// spawned by an incompatible lyx binary).
func daemonStale(s daemonState) bool {
	return !proc.IsAlive(s.PID) || s.ProtocolVersion != supervisedProtocolVersion
}
