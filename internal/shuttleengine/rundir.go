// rundir.go implements the per-run directory lifecycle: minting a run id, resolving the run-dir
// root from Config and a told anchor path, persisting a run's RunState as run.json, looking a run up
// by its owning strand guid, and sweeping orphaned run dirs left behind when a strand no longer
// exists in reed state.
// Everything here is pure I/O over a caller-supplied root and caller-injected guids/clock — no
// tmux, no claude, so it is testable without either.

package shuttleengine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// runStateFileName is the run.json file name inside a per-run directory.
const runStateFileName = "run.json"

// newRunID returns a 128-bit random identifier, hex-encoded, generated from
// crypto/rand — the same recipe as reedengine's newGUID. This is the
// directory-naming identity for one shuttle run; it is distinct from the
// strand guid reed mints for the pane that runs it.
func newRunID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// runDirRoot resolves the directory under which every run's subdirectory is
// created. cfg.RunDir wins when non-empty — a relative value is resolved
// against anchorPath, an already-absolute value is used verbatim.
// When cfg.RunDir is empty, the default is
// filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "shuttle"): the
// ephemeral, machine-local .lyx tree, built from lyxdirs.DotLyxDirName, never
// a literal ".lyx" inline.
// Both branches share anchorPath as their base deliberately: one
// function must never resolve against two different bases when
// the repo is subpath-anchored, or a subpath-anchored repo would end up with two
// distinct run-dir roots depending on which branch a given cfg took.
func runDirRoot(cfg Config, anchorPath string) string {
	if cfg.RunDir == "" {
		return filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "shuttle")
	}
	if filepath.IsAbs(cfg.RunDir) {
		return cfg.RunDir
	}
	return filepath.Join(anchorPath, cfg.RunDir)
}

// runOutcomeRunning is the sentinel RunState.Outcome value Start writes before any classification is
// reached. It is explicit rather than relying on "empty means running", because every run.json
// written by a pre-this-change binary decodes with Outcome == "" — inverting the default this way
// means an upgraded worktree never mistakes a legacy record for an attachable one (see
// RunState.Outcome's own doc comment).
const runOutcomeRunning = "running"

// RunState is the persisted record for one shuttle run, written as <runDir>/run.json.
// It carries exactly what the CLI's interrupt/send verbs and post-hoc diagnosis need: the run and
// strand identities, the session the engine resumed/produced, whether the run was launched
// interactive, the output files the caller expects, the on-disk paths of the run's
// prompt/settings/event files (so a resumed or re-attached session can find them without
// recomputing), when the run was created (RFC3339, supplied by the caller so RunState itself does no
// clock I/O), and whether the run has ever ended.
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
	// Outcome has three writable states. Start writes the sentinel
	// runOutcomeRunning ("running") when it first persists this record.
	// Run.finalize overwrites it with the classification string
	// (done/asking/died/timeout) for EVERY terminal outcome, not only
	// OutcomeDone. Any other value, INCLUDING THE EMPTY STRING, means the
	// record was written by a binary that did not know about this field and
	// is therefore never attachable — a plain "" decodes from every run.json
	// a pre-change binary wrote, so treating empty as attachable would let an
	// in-flight worktree upgraded mid-Asking attach to an idle pane and wait
	// out a freshly restarted run_timeout_min.
	Outcome string `json:"outcome"`
}

// createRunDir mints a fresh run id, creates <root>/<runID>, and returns
// both. The directory is created before the strand exists (reed.AddStrand
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

// saveRunState writes rs to <runDir>/run.json atomically.
func saveRunState(runDir string, rs RunState) error {
	path := filepath.Join(runDir, runStateFileName)
	lockPath := path + ".lock"
	return state.WriteJSON(path, lockPath, rs)
}

// loadRunState reads the RunState persisted at <runDir>/run.json.
// Returns (zero, false, nil) if the file is absent.
// Read by findRunByStrand's guid scan and by Attach's output-files scan (attach.go), both of which
// skip an unreadable or absent record rather than aborting their scan for every other run.
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
//
// How many dirs were skipped that way is reported in the not-found error,
// because the two situations need opposite remedies and the bare "no run
// found" text conflated them: a guid this package genuinely never ran is a
// caller mistake, while a guid whose run.json was truncated by a crash or a
// full disk names a run whose AGENT MAY STILL BE LIVE in its pane — and
// telling that operator their guid "is not a shuttle strand" (the wrapper
// Runner.Interrupt/Send add) sends them away from a running agent. Proven
// live: truncating a live run's run.json produced exactly that message.
func findRunByStrand(root, guid string) (RunState, string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return RunState{}, "", fmt.Errorf("shuttle: no run found for strand %q: run dir root does not exist", guid)
		}
		return RunState{}, "", fmt.Errorf("read run dir root: %w", err)
	}

	unreadable := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		rs, found, err := loadRunState(runDir)
		if err != nil || !found {
			// Skip: a corrupt or missing run.json here is not this scan's
			// concern, only a mismatch on the guid we're looking for. Count it
			// so the not-found error can say the scan was incomplete.
			unreadable++
			continue
		}
		if rs.StrandGUID == guid {
			return rs, runDir, nil
		}
	}

	if unreadable > 0 {
		return RunState{}, "", fmt.Errorf(
			"shuttle: no run found for strand %q, but %d run director%s under %s could not be read — if this guid names a run that is still live, its run.json is damaged rather than absent, and its agent is still in its pane; inspect those directories before treating the guid as unknown",
			guid, unreadable, pluralDirectorySuffix(unreadable), root)
	}
	return RunState{}, "", fmt.Errorf("shuttle: no run found for strand %q", guid)
}

// pluralDirectorySuffix returns the suffix that completes "director" for count: "y" for one,
// "ies" otherwise.
func pluralDirectorySuffix(count int) string {
	if count == 1 {
		return "y"
	}
	return "ies"
}

// FindRun resolves guid to the RunState and run directory of the shuttle run whose strand it names,
// deriving the run directory root from cfg/anchorPath the same way Start does.
// This is how the CLI's interrupt/send verbs (and any other out-of-process caller) turn an
// operator-supplied guid into the run they need to act on, confirming the guid actually names a
// shuttle run before ever touching reed.
func FindRun(cfg Config, anchorPath, guid string) (RunState, string, error) {
	return findRunByStrand(runDirRoot(cfg, anchorPath), guid)
}

// sweepOrphans removes every run directory under root whose run.json names
// a StrandGUID absent from strandGUIDs (the live set from reed state),
// guarded by minAge: a directory whose mtime is younger than minAge is
// never removed, live guid or not. The guard exists because a concurrently
// starting run creates its directory and run.json before AddStrand
// persists the strand — without it, an unguarded sweep could delete a
// run that is still starting up. strandGUIDs comes from ONE worktree's
// reed.json, which is why the configured run_dir must stay worktree-local
// (template.yaml documents this): under a root shared across worktrees,
// every other worktree's runs would look like orphans here and their kept
// diagnosis dirs would be swept once past the age guard. A directory whose
// run.json is missing or
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
