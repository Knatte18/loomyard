// rundir.go implements the per-run directory lifecycle of a shuttle run.
// It mints run ids, resolves the run-directory root, and persists each run's RunState as run.json.
// It also finds a run by its strand guid and sweeps orphaned run directories whose strand is gone from reed state.
// Its only I/O is over a caller-supplied root, with guids and the clock injected by the caller.
// It touches no tmux and no claude, so it is testable without either.

package shuttleengine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// runStateFileName is the run.json file name inside a per-run directory.
const runStateFileName = "run.json"

// runEndStampFileName names the file at the run-directory root.
// Its modification time records when that run was last finalized.
// The file outlives the run directory that a done run's cleanup removes,
// so the end of a removed run stays readable.
const runEndStampFileName = "run-ended"

// stampRunEnd sets the modification time of the run-end stamp under root to at, creating the file when it is absent.
func stampRunEnd(root string, at time.Time) error {
	path := filepath.Join(root, runEndStampFileName)
	stamp, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := stamp.Close(); err != nil {
		return err
	}
	return os.Chtimes(path, at, at)
}

// ErrNoRunRecord marks a strand guid that no run record names.
// It is returned when the run directory root is absent, or when every run.json was read and none names the guid.
// A run directory that could not be read does not wrap it.
// That guid may name a recorded run whose run.json is damaged.
var ErrNoRunRecord = errors.New("shuttle: no run record names the strand")

// noRunRecordError carries a not-found answer's full wording while satisfying errors.Is(err, ErrNoRunRecord).
type noRunRecordError struct{ msg string }

func (e noRunRecordError) Error() string { return e.msg }
func (e noRunRecordError) Unwrap() error { return ErrNoRunRecord }

// newRunID returns a 128-bit random identifier, hex-encoded, drawn from crypto/rand.
// It names one shuttle run's directory.
// It is distinct from the strand guid reed mints for the pane running it.
func newRunID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// runDirRoot resolves the directory under which every run's subdirectory is created.
// A non-empty cfg.RunDir wins: a relative value resolves against anchorPath, and an absolute value is used as is.
// When cfg.RunDir is empty, the root is the shuttle directory under the machine-local .lym tree in anchorPath.
// Both branches resolve against anchorPath, so a subpath-anchored repo gets one run-dir root.
func runDirRoot(cfg Config, anchorPath string) string {
	if cfg.RunDir == "" {
		return filepath.Join(anchorPath, lymdirs.DotLymDirName, "shuttle")
	}
	if filepath.IsAbs(cfg.RunDir) {
		return cfg.RunDir
	}
	return filepath.Join(anchorPath, cfg.RunDir)
}

// runOutcomeRunning is the sentinel RunState.Outcome value that Start writes before any classification.
// It is explicit, not an empty string, because an older run.json decodes with an empty Outcome.
const runOutcomeRunning = "running"

// RunState is the persisted record for one shuttle run, written as <runDir>/run.json.
// It holds what the CLI's interrupt and send verbs and post-hoc diagnosis need.
// That covers the run and strand identities and the session the engine resumed or produced.
// It also records whether the run was launched interactive and which output files the caller expects.
// It holds the paths of the run's prompt, settings and event files.
// A resumed or re-attached session reads those paths instead of recomputing them.
// It records when the run was created, as an RFC3339 string the caller supplies, so RunState does no clock I/O.
// It also records whether the run has ever ended.
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
	// Outcome holds one of three kinds of value.
	// Start writes the sentinel runOutcomeRunning ("running") when it first persists the record.
	// Run.finalize overwrites it with the classification (done, died, timeout, api-error) for every terminal outcome.
	// The value "asking" is one only a legacy binary wrote.
	// Any other value, including the empty string, marks a record that is never attachable.
	// An empty string is what a run.json written before this field decodes to.
	// Treating it as attachable would let a worktree upgraded mid-run attach to an idle pane.
	// Such a run would then wait out a freshly restarted run timeout.
	// The sentinel keeps a running record explicit,
	// so a legacy empty value never reads as running.
	Outcome string `json:"outcome"`
	// Started is false when Start first persists the record.
	// It becomes true when the startup probe sees the provider reach shuttleengine.StartupReady,
	// and the record is persisted again.
	// The startup probe runs from Start, or from a later Wait for an attached run never marked Started.
	// Attach never sets it.
	// A strand being live in reed does not set it either.
	// A live reed pane does not show that the provider inside it ever came up.
	// Without the field, a pane at its shell prompt looks like a booted run by Outcome and liveness alone.
	// This happens when a driver dies before its first liveness tick, or when the launch's binary does not exist.
	// Attach reads the field to decide whether the reconstructed run may skip Wait's startup probe.
	// False is the safe default, and a record from an older binary decodes to it.
	// It costs such a record one extra probe rather than silently skipping one it never earned.
	Started bool `json:"started"`
	// AskingOffset is the events-file byte offset an older binary's finalize had consumed when it classified an ask.
	// This binary leaves it nil and reads it only for legacy records.
	// Attach replays such a record from this offset and counts the ask before it as already notified.
	AskingOffset *int64 `json:"askingOffset,omitempty"`
	// StrandName is the reed name of the run's strand, which a hold notice gives its parent to answer by.
	// Empty for a record that predates the field, where the notice names the strand guid instead.
	StrandName string `json:"strandName,omitempty"`
	// NotifiedOffset is the events-file byte offset past the last notified held turn end; zero for none.
	// Wait persists it before calling the notifier and never notifies a held turn end at or below it,
	// so a crash between the write and the call loses that one notice and never repeats it.
	NotifiedOffset int64 `json:"notifiedOffset,omitempty"`
	// DialogNotifiedOffset is the events-file byte offset past the last ask whose dialog notice Wait sent; zero for none.
	// Wait persists it before calling the notifier and never rebuilds a dialog latch from an ask at or below it,
	// so a crash between the write and the call loses that one notice and never repeats it.
	// It is kept apart from NotifiedOffset so a held-turn-end notice and a dialog notice never suppress each other.
	DialogNotifiedOffset int64 `json:"dialogNotifiedOffset,omitempty"`
	// PromptLine is the deferred prompt, typed once the provider is ready; empty for a prompt on the command line.
	// It lets a later attach deliver a prompt that a dead step never sent.
	PromptLine string `json:"promptLine,omitempty"`
	// Step is the shed step that started the run, nil for a run whose runner was told none.
	Step *StepTie `json:"step,omitempty"`
	// APIContinued is true while the turn an automatic continue line resumed has not ended on a non-API-error turn end.
	// Wait persists it before typing the line and clears it at the next held turn end that is not an API error.
	// One turn thus gets at most one continue, and a reattached Wait never sends twice.
	APIContinued bool `json:"apiContinued,omitempty"`
	// PromptOffset is the events-file size just before Start sends the prompt.
	// It is zero when Start loaded no skills or for a record that predates the field.
	// Readers that replay the events file begin here, so a load turn's end is never read as the run's own.
	PromptOffset int64 `json:"promptOffset,omitempty"`
	// PID is the pid of the process that waits on the run; Start records its own, and an attach rewrites it.
	// Zero for a record an older binary wrote, which therefore never counts as live.
	PID int `json:"pid,omitempty"`
	// RecordsSessionStarts is the launch's declaration that it writes a session-start line, copied from the launch.
	// Startup needs that line beside the ready pane; an older binary's record decodes false and keeps pane-only startup.
	RecordsSessionStarts bool `json:"recordsSessionStarts,omitempty"`
	// ForkSubagents records whether the spawning Spec authorized forking subagents, copied at spawn.
	// A record an older binary wrote decodes false and reads as not authorized.
	ForkSubagents bool `json:"forkSubagents,omitempty"`
	// Engine is the name of the engine the run was started with.
	// A record written before the field existed reads as the engine set's default engine.
	Engine string `json:"engine,omitempty"`
	// RoleKey is the spawning Spec's name override, else its role, which a later run start matches its predecessor by.
	// A record without it is never a predecessor.
	RoleKey string `json:"roleKey,omitempty"`
	// CarriedInstructions are the operator instructions a predecessor run carried over, so a re-spawn carries them again.
	CarriedInstructions []string `json:"carriedInstructions,omitempty"`
	// Role is the spawning Spec's role segment, kept so the stop path can report the ended session from the record alone.
	// A record an older binary wrote leaves it empty.
	Role string `json:"role,omitempty"`
	// Model is the spawning Spec's model, which the stop path reports; a record an older binary wrote leaves it empty.
	Model string `json:"model,omitempty"`
}

// createRunDir mints a fresh run id, creates <root>/<runID>, and returns both.
// The directory is created before its strand exists,
// so a sweep must treat a directory this fresh as a run still starting, never as an orphan.
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
// Scans over many run directories skip an unreadable or absent record rather than abort on it,
// so one damaged run does not hide the others.
func loadRunState(runDir string) (RunState, bool, error) {
	path := filepath.Join(runDir, runStateFileName)
	lockPath := path + ".lock"
	return state.ReadJSON[RunState](path, lockPath)
}

// pendingSendFileName is the file name of the pending-send record inside a per-run directory.
// It is a file of its own so its writes never race run.json's writers.
const pendingSendFileName = "pending-send.json"

// pendingSend records a send that may have left its text in the strand's input box.
// EventsOffset is the events-file size just before the text was written,
// so a later turn start past it reads as the text going out.
type pendingSend struct {
	Text         string `json:"text"`
	EventsOffset int64  `json:"eventsOffset"`
}

// savePendingSend writes p to <runDir>/pending-send.json atomically.
func savePendingSend(runDir string, p pendingSend) error {
	path := filepath.Join(runDir, pendingSendFileName)
	return state.WriteJSON(path, path+".lock", p)
}

// loadPendingSend reads the pending-send record at <runDir>/pending-send.json.
// found is false for an absent file; a file that does not decode is an error.
func loadPendingSend(runDir string) (pendingSend, bool, error) {
	path := filepath.Join(runDir, pendingSendFileName)
	return state.ReadJSON[pendingSend](path, path+".lock")
}

// clearPendingSend removes the pending-send record; an absent record is already cleared.
func clearPendingSend(runDir string) error {
	err := os.Remove(filepath.Join(runDir, pendingSendFileName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// findRunByStrand scans <root>/*/run.json for the run whose StrandGUID
// matches guid, returning its RunState and owning run directory.
// It is how an operator-supplied guid becomes the run to act on.
// It also lets a caller confirm the guid names a shuttle run at all.
// It returns an error if no run.json has a matching StrandGUID.
// Unreadable or corrupt run.json files along the way are skipped, not fatal.
// A partially written or already swept directory must not abort the scan for every other run.
//
// The not-found error reports how many directories were skipped, because the two cases need opposite remedies.
// A guid this package never ran is a caller mistake.
// A guid whose run.json was truncated by a crash or a full disk names a run whose agent may still be live in its pane.
// Telling that operator the guid is not a shuttle strand sends them away from a running agent.
func findRunByStrand(root, guid string) (RunState, string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return RunState{}, "", noRunRecordError{fmt.Sprintf("shuttle: no run found for strand %q: run dir root does not exist", guid)}
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
	return RunState{}, "", noRunRecordError{fmt.Sprintf("shuttle: no run found for strand %q", guid)}
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
// deriving the run directory root from cfg and anchorPath.
// An out-of-process caller uses it to turn an operator-supplied guid into the run it acts on.
// It also confirms the guid names a shuttle run.
func FindRun(cfg Config, anchorPath, guid string) (RunState, string, error) {
	return findRunByStrand(runDirRoot(cfg, anchorPath), guid)
}

// sweepOrphans removes every run directory under root whose run.json names a StrandGUID absent from strandGUIDs.
// strandGUIDs is the live set from reed state.
// A directory younger than minAge is never removed, live guid or not.
// The age guard exists because a starting run creates its directory and run.json before its strand is persisted.
// An unguarded sweep could delete a run that is still starting up.
// strandGUIDs comes from one worktree's reed.json, so the configured run_dir must stay worktree-local:
// under a root shared across worktrees, every other worktree's runs look like orphans,
// and their kept diagnosis directories would be swept once past the age guard.
// A directory whose run.json is missing or unreadable counts as an orphan: no strand can be confirmed live for it.
// The same age guard applies to it.
// now is the clock the caller supplies, so tests can control aging deterministically.
// It returns the removed directory paths and a joined error naming every directory that could not be removed.
// The sweep visits every entry, so one undeletable directory costs only that directory.
func sweepOrphans(root string, strandGUIDs map[string]bool, minAge time.Duration, now time.Time) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read run dir root: %w", err)
	}

	var removed []string
	var removeErrs []error
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
				// Accumulate the failure and keep sweeping: returning on the first failure would leave every later orphan in place.
				// A directory that cannot be removed costs that one directory and nothing more.
				// Its causes include a file held open on Windows, a permission-denied subtree, and an NFS .nfs* stub.
				removeErrs = append(removeErrs, fmt.Errorf("remove orphan run dir %s: %w", runDir, rerr))
				continue
			}
			removed = append(removed, runDir)
		}
	}

	return removed, errors.Join(removeErrs...)
}
