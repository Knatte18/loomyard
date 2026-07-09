// state.go persists one perch block's progress at <runDir>/state.json via
// internal/state's locked JSON I/O, and derives the two identities a block
// needs before it can even open that file: ProfileHash (what makes two
// profiles "the same" for resume purposes) and DeriveRunID (the default
// run-id a standalone caller mints from a profile path). It also implements
// the round-level crash-recovery mechanics: classifying an existing state
// file into fresh/resume/error, moving aside a partial round's stale
// artifacts before it re-runs, and the pause flag file the loop checks
// between rounds.

package perchengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/state"
)

// stateFileName is the state.json file name inside a block's run dir.
const stateFileName = "state.json"

// staleSuffix is appended to a stale artifact's path before its round
// re-runs; a numeric suffix is added on top when staleSuffix alone would
// collide with an already-stale file from an earlier partial resume.
const staleSuffix = ".stale"

// PauseFlagName is the pause flag file's name inside a block's run dir.
// It is exported so perchcli's pause verb can name the same file it writes
// without recomputing the join itself.
const PauseFlagName = "pause"

// roundRecord is the persisted history entry for one completed round: the
// round/attempt identity, the shuttle-level outcome that ended the last
// attempt, the burler verdict and blocking count, every artifact path the
// round produced (empty when that sub-step did not run — mirrors
// RoundSummary), and the SessionID of the burler run that reached done, for
// diagnosis. A round record is appended to runState.Rounds only on
// completion — an interrupted round simply has no record, which is what
// lets loadOrInitState tell "resume at the next round" apart from
// "re-run the round that was interrupted" without a separate in-progress
// flag.
type roundRecord struct {
	Round           int    `json:"round"`
	Attempts        int    `json:"attempts"`
	ShuttleOutcome  string `json:"shuttleOutcome"`
	Verdict         string `json:"verdict"`
	BlockingCount   int    `json:"blockingCount"`
	ReviewPath      string `json:"reviewPath"`
	FixerReportPath string `json:"fixerReportPath"`
	JudgePath       string `json:"judgePath,omitempty"`
	GatePath        string `json:"gatePath,omitempty"`
	TriagePath      string `json:"triagePath,omitempty"`
	JudgeVerdict    string `json:"judgeVerdict,omitempty"`
	GatePassed      *bool  `json:"gatePassed,omitempty"`
	SessionID       string `json:"sessionId"`
}

// runState is the persisted record for one perch block, written as
// <runDir>/state.json. ProfileHash and RoundCaps are stamped once at block
// creation (RoundCaps after default resolution, so a resumed block always
// re-applies the ladder it actually started with, even if perch.yaml's
// default later changes). Outcome is empty while the block is in progress;
// a non-empty Outcome (with StuckReason set alongside OutcomeStuck) marks
// the block terminal — loadOrInitState refuses to resume a terminal state.
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

// ProfileHash returns the sha256 hex digest of p's canonical JSON encoding.
// Engine.Run hashes the profile AS SUPPLIED by the caller — before default
// resolution — so a block's identity is the caller's own content contract:
// editing the profile (or changing a CLI tuning flag folded into it) changes
// the hash and fails a resume loud, while a perch.yaml default change never
// silently alters or invalidates an in-flight block (its resolved ladder is
// stamped into state.json instead — see runState.RoundCaps). The same rule
// holds for a loom-supplied Go struct: identity is what the caller passed in.
func ProfileHash(p Profile) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("perch: marshal profile for hashing: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// DeriveRunID returns the default run-id for a standalone `lyx perch run`
// invocation: the profile file's basename with its extension stripped and
// sanitized to lowercase alphanumerics and dashes, followed by the first 8
// hex characters of hash. Sanitizing keeps the id filesystem-safe (it names
// a directory under hubgeometry.PerchRunsDir) while the hash suffix keeps
// two same-named profiles in different directories from colliding.
func DeriveRunID(profilePath string, hash string) string {
	base := filepath.Base(profilePath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return fmt.Sprintf("%s-%s", sanitizeSlug(base), hash[:8])
}

// sanitizeSlug lowercases s and replaces every run of non-alphanumeric
// characters with a single dash, trimming leading/trailing dashes.
func sanitizeSlug(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// loadOrInitState reads <runDir>/state.json and classifies it against hash
// (the incoming profile's ProfileHash) and caps (the incoming profile's
// resolved RoundCaps):
//   - no state.json: a fresh block. An initial runState (ProfileHash: hash,
//     RoundCaps: caps) is written before returning, so a concurrent second
//     invocation against the same runDir observes a non-fresh state.
//   - unfinished state (Outcome == "") with a matching ProfileHash: resume —
//     NextRound is len(Rounds)+1.
//   - unfinished state with a different ProfileHash: fail loud. An edited
//     profile must never silently continue rounds recorded under the old
//     one; the caller is told to use a fresh --run-id instead.
//   - terminal state (Outcome != ""): fail loud — this block already ran to
//     completion and perch never re-opens a finished block.
func loadOrInitState(runDir string, hash string, caps []int) (runState, resumeInfo, error) {
	path := filepath.Join(runDir, stateFileName)
	lockPath := path + ".lock"

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
		return runState{}, resumeInfo{}, fmt.Errorf("perch: this block already finished (%s)", existing.Outcome)
	}

	if existing.ProfileHash != hash {
		return runState{}, resumeInfo{}, fmt.Errorf("perch: run dir %s was started with a different profile; use a fresh --run-id", runDir)
	}

	return existing, resumeInfo{Fresh: false, NextRound: len(existing.Rounds) + 1}, nil
}

// saveState writes s to <runDir>/state.json atomically under an exclusive
// lock at <runDir>/state.json.lock, the same file loadOrInitState reads.
func saveState(runDir string, s runState) error {
	path := filepath.Join(runDir, stateFileName)
	lockPath := path + ".lock"
	return state.WriteJSON(path, lockPath, s)
}

// moveStaleArtifacts renames aside every artifact file that already exists
// for round/attempt inside runDir, so a re-run round never trips shuttle's
// no-pre-existing-output-file rule. It is called on resume for a round that
// started but never reached done (no roundRecord was appended for it), just
// before that round is re-run from scratch.
func moveStaleArtifacts(runDir string, round, attempt int) error {
	paths := artifactPaths(runDir, round, attempt)
	for _, p := range []string{paths.Review, paths.FixerReport, paths.Judge, paths.Gate, paths.Triage} {
		if err := moveStaleIfExists(p); err != nil {
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
func moveStaleIfExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("perch: stat %q: %w", path, err)
	}

	dest := path + staleSuffix
	for n := 2; fileExists(dest); n++ {
		dest = fmt.Sprintf("%s%s.%d", path, staleSuffix, n)
	}

	if err := os.Rename(path, dest); err != nil {
		return fmt.Errorf("perch: rename stale artifact %q to %q: %w", path, dest, err)
	}
	return nil
}

// fileExists reports whether path names an existing filesystem entry.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// PauseFlagPath returns the path to the pause flag file inside runDir.
// perchcli's pause verb writes this file, and the run loop's PauseRequested
// seam checks for it between rounds; both must resolve the same path, which
// is why this is exported rather than duplicated at each call site.
func PauseFlagPath(runDir string) string {
	return filepath.Join(runDir, PauseFlagName)
}

// clearPauseFlag removes the pause flag file if present, doing nothing if
// it is absent. It is called at Run's entry so a resumed block does not
// instantly re-pause on a flag left over from the run that requested the
// pause it is now resuming from.
func clearPauseFlag(runDir string) error {
	path := PauseFlagPath(runDir)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("perch: remove pause flag %q: %w", path, err)
	}
	return nil
}
