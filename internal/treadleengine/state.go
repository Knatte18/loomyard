// state.go persists one treadle block's progress at <runDir>/state.json via internal/state's locked
// JSON I/O.
// It implements the round-level crash-recovery mechanics: classifying an existing state file into
// fresh/resume/error, moving aside a partial round's stale artifacts before it re-runs, and the
// pause flag file the loop checks between rounds.
// Identity derivation (ProfileHash, DeriveRunID, ValidRunID, sanitizeSlug) is NOT here — treadle
// takes ProfileHash as caller-supplied data (see Profile.ProfileHash and the treadle-owns-no-config
// shared decision);
// a caller owns them, alongside whatever resolves its own profile data.
//
// The persisted artifacts split across two directories (the told-never-derived-scratch-dir shared
// decision): state.json and every round artifact live in runDir, while state.json.lock, run.lock
// (run.go), and the pause flag live in scratchDir — the block's never-tracked, .lyx-anchored tree.
// A caller that has not split the two passes the same directory for both, which every function
// below treats as an ordinary, harmless case.

package treadleengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/state"
)

// stateFileName is the state.json file name inside a block's run dir.
const stateFileName = "state.json"

// staleSuffix is appended to a stale artifact's path before its round
// re-runs; a numeric suffix is added on top when staleSuffix alone would
// collide with an already-stale file from an earlier partial resume.
const staleSuffix = ".stale"

// PauseFlagName is the pause flag file's name inside a block's run dir.
// It is exported so a caller's own pause verb can name the same file it writes without
// recomputing the join itself.
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
	// HandoffPath is the path of this round's judge-maintained handoff file
	// (see handoff.go), set only when that round's judge call succeeded AND
	// its handoff output read and ParseHandoff-validated cleanly — additive
	// per the state-json-compatibility shared decision, so a block written
	// by an older binary resumes with zero migration; its records simply
	// lack handoff coverage, which judgeReadSet's fallback already handles.
	HandoffPath string `json:"handoffPath,omitempty"`
	GatePath    string `json:"gatePath,omitempty"`
	TriagePath  string `json:"triagePath,omitempty"`
	// SeedPath is the path of this round's pre-round targeting seed file
	// (see run.go), set only when Profile.PreRoundTargeting was on AND that
	// round's targeting call succeeded — additive per the
	// state-json-compatibility shared decision, exactly like HandoffPath.
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

// loadOrInitState reads <runDir>/state.json (locked against
// <scratchDir>/state.json.lock) and classifies it against hash (the
// incoming profile's ProfileHash) and caps (the incoming profile's resolved
// RoundCaps). Every error message is prefixed with name (the calling
// engine's own name), per the name-parameterized-diagnostics shared
// decision:
//   - no state.json: a fresh block. An initial runState (ProfileHash: hash,
//     RoundCaps: caps) is written before returning, so a concurrent second
//     invocation against the same runDir observes a non-fresh state.
//   - unfinished state (Outcome == "") with a matching ProfileHash: resume —
//     NextRound is len(Rounds)+1.
//   - unfinished state with a different ProfileHash: fail loud. An edited
//     profile must never silently continue rounds recorded under the old
//     one; the caller is told to use a fresh --run-id instead.
//   - terminal state (Outcome != ""): fail loud — this block already ran to
//     completion and treadle never re-opens a finished block.
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

// TerminalOutcome reports the terminal Outcome recorded in runDir's state.json: ok is true only
// when a state file exists AND records a finished block (APPROVED or STUCK).
// A missing state file or an in-flight block (empty Outcome) returns ok false with no error.
// It exists for a caller's own pause verb, which must refuse to write a pause flag against a
// block that already finished — no
// run loop will ever observe that flag, so reporting the pause as accepted would mislead the
// operator.
// Reads under the same state.json.lock discipline as loadOrInitState, locked against
// <scratchDir>/state.json.lock — a caller's pause verb must resolve scratchDir from the same
// scratch base the run verb passes to the engine.
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

// moveStaleArtifacts renames aside every artifact file that already exists
// for round/attempt inside runDir, so a re-run round never trips a fresh
// spawn's no-pre-existing-output-file rule. It is called on resume for a
// round that started but never reached done (no roundRecord was appended
// for it), just before that round is re-run from scratch — once at attempt 1
// (run.go, before the round's pre-round targeting call, so a leftover seed
// file from an interrupted prior attempt at this round is cleared before
// targeting tries to write a fresh one) and again at the top of each later
// retry attempt (run.go's runRound).
func moveStaleArtifacts(name string, runDir string, round, attempt int) error {
	paths := artifactPaths(runDir, round, attempt)
	for _, p := range []string{paths.Review, paths.FixerReport, paths.Judge, paths.Handoff, paths.Gate, paths.Triage, paths.Seed} {
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

// PauseFlagPath returns the path to the pause flag file inside scratchDir —
// the block's never-tracked scratch dir, not its durable run dir.
// A caller's own pause verb writes this file,
// and the run loop's PauseRequested seam checks for it between rounds;
// both must resolve scratchDir from the same scratch base a caller's run
// verb passes to the engine, which is why this is exported rather than
// duplicated at each call site.
func PauseFlagPath(scratchDir string) string {
	return filepath.Join(scratchDir, PauseFlagName)
}

// clearPauseFlag removes the pause flag file if present, doing nothing if
// it is absent. It is called at Run's entry so a resumed block does not
// instantly re-pause on a flag left over from the run that requested the
// pause it is now resuming from, and again at every terminal, non-PAUSED
// return so a finished block never leaves the flag behind. Its error is
// prefixed with name (the calling engine's own name) like every other
// message in this file — Run returns it verbatim rather than re-wrapping,
// so without the prefix a removal failure would reach the caller's CLI
// envelope as the one error carrying no module label at all.
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
