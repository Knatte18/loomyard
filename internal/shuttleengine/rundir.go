// rundir.go implements the per-run directory lifecycle: minting a run id,
// resolving the run-dir root from Config/hubgeometry, persisting a run's
// RunState as run.json, looking a run up by its owning strand guid, and
// sweeping orphaned run dirs left behind when a strand no longer exists in
// mux state. Everything here is pure I/O over a caller-supplied root and
// caller-injected guids/clock — no psmux, no claude, so it is testable
// without either.

package shuttleengine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/state"
)

// runStateFileName is the run.json file name inside a per-run directory.
const runStateFileName = "run.json"

// newRunID returns a 128-bit random identifier, hex-encoded, generated from
// crypto/rand — the same recipe as muxengine's newGUID. This is the
// directory-naming identity for one shuttle run; it is distinct from the
// strand guid mux mints for the pane that runs it.
func newRunID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// runDirRoot resolves the directory under which every run's subdirectory is
// created. cfg.RunDir wins when non-empty — a relative value is resolved
// against layout.WorktreeRoot, an already-absolute value is used verbatim.
// When cfg.RunDir is empty, the default is
// filepath.Join(layout.DotLyxDir(), "shuttle"): the ephemeral, machine-local
// .lyx tree, never a literal ".lyx" (Hub Geometry Invariant).
func runDirRoot(cfg Config, layout *hubgeometry.Layout) string {
	if cfg.RunDir == "" {
		return filepath.Join(layout.DotLyxDir(), "shuttle")
	}
	if filepath.IsAbs(cfg.RunDir) {
		return cfg.RunDir
	}
	return filepath.Join(layout.WorktreeRoot, cfg.RunDir)
}

// RunState is the persisted record for one shuttle run, written as
// <runDir>/run.json. It carries exactly what the CLI's interrupt/send verbs
// and post-hoc diagnosis need: the run and strand identities, the session
// the engine resumed/produced, whether the run was launched interactive,
// the output files the caller expects, the on-disk paths of the run's
// prompt/settings/event files (so a resumed or re-attached session can find
// them without recomputing), and when the run was created (RFC3339,
// supplied by the caller so RunState itself does no clock I/O).
type RunState struct {
	RunID        string   `json:"runId"`
	StrandGUID   string   `json:"strandGuid"`
	SessionID    string   `json:"sessionId"`
	Interactive  bool     `json:"interactive"`
	OutputFiles  []string `json:"outputFiles"`
	PromptPath   string   `json:"promptPath"`
	SettingsPath string   `json:"settingsPath"`
	EventsPath   string   `json:"eventsPath"`
	CreatedAt    string   `json:"createdAt"`
}

// createRunDir mints a fresh run id, creates <root>/<runID>, and returns
// both. The directory is created before the strand exists (mux.AddStrand
// has not run yet) — this ordering is exactly what sweepOrphans' age guard
// protects against: a dir this fresh must never be mistaken for an orphan.
func createRunDir(root string) (runID, runDir string, err error) {
	runID, err = newRunID()
	if err != nil {
		return "", "", fmt.Errorf("mint run id: %w", err)
	}
	runDir = filepath.Join(root, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create run dir: %w", err)
	}
	return runID, runDir, nil
}

// saveRunState writes rs to <runDir>/run.json atomically under an
// exclusive lock at <runDir>/run.json.lock.
func saveRunState(runDir string, rs RunState) error {
	path := filepath.Join(runDir, runStateFileName)
	lockPath := path + ".lock"
	return state.WriteJSON(path, lockPath, rs)
}

// loadRunState reads the RunState persisted at <runDir>/run.json under a
// shared read lock. Returns (zero, false, nil) if the file is absent.
func loadRunState(runDir string) (RunState, bool, error) {
	path := filepath.Join(runDir, runStateFileName)
	lockPath := path + ".lock"
	return state.ReadJSON[RunState](path, lockPath)
}

// findRunByStrand scans <root>/*/run.json for the run whose StrandGUID
// matches guid, returning its RunState and owning run directory. This is
// how the CLI's interrupt/send verbs turn an operator-supplied guid into
// the run they need to act on, and how they confirm the guid actually names
// a shuttle run. Returns an error if no run dir's run.json has a matching
// StrandGUID; unreadable/corrupt run.json files along the way are skipped,
// not fatal, since a partially-written or already-swept dir must not abort
// the scan for every other run.
func findRunByStrand(root, guid string) (RunState, string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return RunState{}, "", fmt.Errorf("shuttle: no run found for strand %q: run dir root does not exist", guid)
		}
		return RunState{}, "", fmt.Errorf("read run dir root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		rs, found, err := loadRunState(runDir)
		if err != nil || !found {
			// Skip: a corrupt or missing run.json here is not this scan's
			// concern, only a mismatch on the guid we're looking for.
			continue
		}
		if rs.StrandGUID == guid {
			return rs, runDir, nil
		}
	}

	return RunState{}, "", fmt.Errorf("shuttle: no run found for strand %q", guid)
}

// sweepOrphans removes every run directory under root whose run.json names
// a StrandGUID absent from strandGUIDs (the live set from mux state),
// guarded by minAge: a directory whose mtime is younger than minAge is
// never removed, live guid or not. The guard exists because a concurrently
// starting run creates its directory and run.json before AddStrand
// persists the strand — without it, an unguarded sweep could delete a
// run that is still starting up. A directory whose run.json is missing or
// unreadable is treated the same as an orphan (no strand can be confirmed
// live for it) but is still subject to the same age guard. now is the
// caller-supplied clock so tests can control aging deterministically.
// Returns the list of removed directory paths.
func sweepOrphans(root string, strandGUIDs map[string]bool, minAge time.Duration, now time.Time) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read run dir root: %w", err)
	}

	var removed []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())

		info, err := entry.Info()
		if err != nil {
			// Cannot stat this entry at all; leave it for a later sweep
			// rather than guessing at its age.
			continue
		}
		if now.Sub(info.ModTime()) < minAge {
			// Too young to trust: a run in the middle of starting up looks
			// identical to an orphan from here.
			continue
		}

		rs, found, err := loadRunState(runDir)
		if err != nil || !found || !strandGUIDs[rs.StrandGUID] {
			if rerr := os.RemoveAll(runDir); rerr != nil {
				return removed, fmt.Errorf("remove orphan run dir %s: %w", runDir, rerr)
			}
			removed = append(removed, runDir)
		}
	}

	return removed, nil
}
