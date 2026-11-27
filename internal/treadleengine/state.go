// state.go persists one treadle block's progress at <runDir>/state.json via internal/state's locked JSON I/O.
// It classifies an existing state file as fresh, resume, or error.
// It moves aside a partial round's stale artifacts before the round re-runs.
// It also implements the pause flag file the loop checks between rounds.
// Identity derivation (ProfileHash, DeriveRunID, ValidRunID, sanitizeSlug) is not here.
// Treadle takes ProfileHash as caller-supplied data.
// A caller owns the identity functions, alongside whatever resolves its own profile data.
//
// The persisted artifacts split across two directories.
// state.json and every round artifact live in runDir.
// state.json.lock, run.lock (run.go), and the pause flag live in scratchDir.
// That is the block's never-tracked, .lym-anchored tree.
// A caller that has not split the two passes the same directory for both.
// Every function below treats that as an ordinary, harmless case.

package treadleengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/state"
)

// stateFileName is the state.json file name inside a block's run dir.
const stateFileName = "state.json"

// staleSuffix is appended to a stale artifact's path before its round re-runs.
// A numeric suffix is added when staleSuffix alone would collide with an earlier stale file.
const staleSuffix = ".stale"

// PauseFlagName is the pause flag file's name inside a block's run dir.
// It is exported so a caller writing the pause flag targets the file the run loop checks.
const PauseFlagName = "pause"

// roundRecord is the persisted history entry for one completed round: identity,
// shuttle outcome, verdict, blocking count, artifact paths, and session ID.
// Appended only on completion; an interrupted round has no record.
type roundRecord struct {
	Round           int    `json:"round"`
	Attempts        int    `json:"attempts"`
	ShuttleOutcome  string `json:"shuttleOutcome"`
	Verdict         string `json:"verdict"`
	BlockingCount   int    `json:"blockingCount"`
	ReviewPath      string `json:"reviewPath"`
	FixerReportPath string `json:"fixerReportPath"`
	JudgePath       string `json:"judgePath,omitempty"`
	// HandoffPath is the path of this round's judge-maintained handoff file (see handoff.go).
	// It is set only when that round's judge call succeeded and ParseHandoff validated its handoff output.
	// The field is additive: a block written by an older binary resumes with no migration.
	// Its records lack handoff coverage, which the judge read-set fallback already handles.
	HandoffPath string `json:"handoffPath,omitempty"`
	GatePath    string `json:"gatePath,omitempty"`
	// SeedPath is the path of this round's pre-round targeting seed file (see run.go).
	// It is set only when pre-round targeting was enabled and that round's targeting call succeeded.
	// Like HandoffPath, the field is additive, so older state files still load.
	SeedPath     string `json:"seedPath,omitempty"`
	JudgeVerdict string `json:"judgeVerdict,omitempty"`
	GatePassed   *bool  `json:"gatePassed,omitempty"`
	SessionID    string `json:"sessionId"`
}

// runState is the persisted record for one treadle block, written as
// <runDir>/state.json. ProfileHash and RoundCaps are stamped once at block
// creation. Outcome is empty while in progress; non-empty marks terminal.
type runState struct {
	ProfileHash string        `json:"profileHash"`
	RoundCaps   []int         `json:"roundCaps"`
	Rounds      []roundRecord `json:"rounds"`
	Outcome     string        `json:"outcome,omitempty"`
	StuckReason string        `json:"stuckReason,omitempty"`
}

// resumeInfo is loadOrInitState's classification of an existing (or absent)
// run dir: whether a fresh initial state was just written, and the round
// number the loop should start at (1 for fresh, len(Rounds)+1 for resume).
type resumeInfo struct {
	Fresh     bool
	NextRound int
}

// loadOrInitState reads <runDir>/state.json (locked against <scratchDir>/state.json.lock)
// and classifies it against hash (the incoming profile's ProfileHash) and caps (the incoming profile's
// resolved RoundCaps). Every error message is prefixed with name, the calling engine's own name.
//   - no state.json: a fresh block. An initial runState (ProfileHash: hash, RoundCaps: caps) is written
//     before returning, so a concurrent second invocation against the same runDir observes a non-fresh state.
//   - unfinished state (Outcome == "") with a matching ProfileHash: resume; NextRound is len(Rounds)+1.
//   - unfinished state with a different ProfileHash: fail loud. An edited profile must never silently
//     continue rounds recorded under the old one; the caller is told to use a fresh --run-id instead.
//   - terminal state (Outcome != ""): fail loud. This block already ran to completion,
//     and treadle never re-opens a finished block.
func loadOrInitState(name string, runDir string, scratchDir string, hash string, caps []int) (runState, resumeInfo, error) {
	path := filepath.Join(runDir, stateFileName)
	lockPath := filepath.Join(scratchDir, stateFileName+".lock")

	existing, found, err := state.ReadJSON[runState](path, lockPath)
	if err != nil {
		return runState{}, resumeInfo{}, err
	}

	if !found {
		fresh := runState{ProfileHash: hash, RoundCaps: caps}
		if err := state.WriteJSON(path, lockPath, fresh); err != nil {
			return runState{}, resumeInfo{}, err
		}
		return fresh, resumeInfo{Fresh: true, NextRound: 1}, nil
	}

	// A terminal state is refused regardless of hash — the block already
	// finished, and re-opening a finished block (even under the profile
	// that produced it) is never a valid resume.
	if existing.Outcome != "" {
		return runState{}, resumeInfo{}, fmt.Errorf("%s: this block already finished (%s)", name, existing.Outcome)
	}

	if existing.ProfileHash != hash {
		return runState{}, resumeInfo{}, fmt.Errorf("%s: run dir %s was started with a different profile; use a fresh --run-id", name, runDir)
	}

	return existing, resumeInfo{Fresh: false, NextRound: len(existing.Rounds) + 1}, nil
}

// TerminalOutcome reports the terminal Outcome recorded in runDir's state.json.
// ok is true only when a state file exists and records a finished block (APPROVED or STUCK).
// A missing state file or an in-flight block (empty Outcome) returns ok false with no error.
// A caller that writes a pause flag uses it to refuse a block that already finished.
// Accepting the flag for a finished block would mislead the operator, since no run loop will observe it.
// It reads under the same state.json.lock that the run takes.
// scratchDir must therefore be the scratch base the run passes to the engine.
func TerminalOutcome(runDir, scratchDir string) (Outcome, bool, error) {
	path := filepath.Join(runDir, stateFileName)
	lockPath := filepath.Join(scratchDir, stateFileName+".lock")

	existing, found, err := state.ReadJSON[runState](path, lockPath)
	if err != nil {
		return "", false, err
	}
	if !found || existing.Outcome == "" {
		return "", false, nil
	}
	return Outcome(existing.Outcome), true, nil
}

// saveState writes s to <runDir>/state.json atomically under an exclusive
// lock at <scratchDir>/state.json.lock, the same file loadOrInitState reads.
// scratchDir is created first — internal/state's WriteJSON creates the
// parent of the file it writes (runDir) but not the parent of a lock in a
// sibling tree.
func saveState(runDir, scratchDir string, s runState) error {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return fmt.Errorf("create scratch dir %q: %w", scratchDir, err)
	}
	path := filepath.Join(runDir, stateFileName)
	lockPath := filepath.Join(scratchDir, stateFileName+".lock")
	return state.WriteJSON(path, lockPath, s)
}

// moveStaleArtifacts renames aside every artifact file that already exists for round/attempt inside runDir.
// A re-run round then never trips a fresh spawn's no-pre-existing-output-file rule.
// The caller invokes it for a round that started but never completed, before the round re-runs.
// At attempt 1 it runs before the pre-round targeting call.
// That clears a leftover seed file from an interrupted attempt before targeting writes a fresh one.
// It runs again at the start of each later retry attempt.
func moveStaleArtifacts(name string, runDir string, round, attempt int) error {
	paths := artifactPaths(runDir, round, attempt)
	for _, p := range []string{paths.Review, paths.FixerReport, paths.Judge, paths.Handoff, paths.Gate, paths.Seed} {
		if err := moveStaleIfExists(name, p); err != nil {
			return err
		}
	}
	return nil
}

// moveStaleIfExists renames path to path+staleSuffix if path exists, doing
// nothing if it does not. If the destination itself already exists (a
// second stale collision — e.g. two consecutive interrupted resumes of the
// same round before a third succeeds), a numeric suffix is appended
// (".stale.2", ".stale.3", ...) until a free name is found.
func moveStaleIfExists(name string, path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%s: stat %q: %w", name, path, err)
	}

	dest := path + staleSuffix
	for n := 2; fileExists(dest); n++ {
		dest = fmt.Sprintf("%s%s.%d", path, staleSuffix, n)
	}

	if err := os.Rename(path, dest); err != nil {
		return fmt.Errorf("%s: rename stale artifact %q to %q: %w", name, path, dest, err)
	}
	return nil
}

// fileExists reports whether path names an existing filesystem entry.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// PauseFlagPath returns the path of the pause flag file inside scratchDir.
// scratchDir is the block's never-tracked scratch directory, not its durable run directory.
// The writer of the flag and the run loop's check for it between rounds share this path.
// Both must resolve scratchDir from the same scratch base the run verb passes to the engine,
// so the path is built in one place rather than duplicated at each call site.
func PauseFlagPath(scratchDir string) string {
	return filepath.Join(scratchDir, PauseFlagName)
}

// clearPauseFlag removes the pause flag file if present, doing nothing if it is absent.
// It runs at the start of a run, so a resumed block does not pause again on a flag left by the run it resumes from.
// It also runs at every terminal return other than a pause, so a finished block never leaves the flag behind.
// The error is prefixed with name, the calling engine's name, because the caller returns it unwrapped.
// Without that prefix a removal failure would reach the CLI envelope with no module label at all.
func clearPauseFlag(name string, scratchDir string) error {
	path := PauseFlagPath(scratchDir)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%s: remove pause flag %q: %w", name, path, err)
	}
	return nil
}
