// identity.go implements perch's own block-identity derivation —
// ProfileHash, DeriveRunID, ValidRunID, sanitizeSlug — extracted out of the
// old state.go when its round-state machinery moved to treadleengine, plus
// perch's re-exports of treadleengine's identity/pause-flag/error-sentinel/
// verdict vocabulary (the byte-identical-perch-api shared decision):
// TerminalOutcome, PauseFlagPath, PauseFlagName, ErrBlockBusy, and the
// JudgeVerdict/TriageVerdict types and constants.

package perchengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/treadleengine"
)

// ProfileHash returns the sha256 hex digest of p's canonical JSON encoding.
// Engine.Run hashes the profile AS SUPPLIED by the caller — before default
// resolution — so a block's identity is the caller's own content contract:
// editing the profile (or changing a CLI tuning flag folded into it) changes
// the hash and fails a resume loud, while a perch.yaml default change never
// silently alters or invalidates an in-flight block (its resolved ladder is
// stamped into state.json instead — see treadleengine's runState.RoundCaps).
// The same rule holds for a loom-supplied Go struct: identity is what the
// caller passed in.
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

// ValidRunID reports whether id is a legal explicit --run-id: the exact
// shape sanitizeSlug produces for a derived id (lowercase alphanumerics and
// single dashes, no leading/trailing dash), non-empty. This is deliberately
// stricter than "anything filepath.Join tolerates" — an operator-supplied
// --run-id is joined directly into a directory path under
// hubgeometry.PerchRunsDir, so anything containing a path separator or a
// "." component could escape that directory (e.g. "../elsewhere") or land
// on a Windows-illegal name; both perch verbs reject the flag outright
// rather than let MkdirAll or a later git operation fail confusingly deep
// inside the run.
func ValidRunID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return id[0] != '-' && id[len(id)-1] != '-'
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

// TerminalOutcome reports the terminal Outcome recorded in runDir's
// state.json, delegating to treadleengine's identical mechanics and
// converting its Outcome vocabulary onto perch's own distinct (but
// identically spelled) Outcome type. Exists for perchcli's pause verb,
// which must refuse to write a pause flag against a block that already
// finished.
func TerminalOutcome(runDir string) (Outcome, bool, error) {
	outcome, ok, err := treadleengine.TerminalOutcome(runDir)
	return Outcome(outcome), ok, err
}

// PauseFlagPath returns the path to the pause flag file inside runDir,
// delegating to treadleengine's identical mechanics so perchcli's pause
// verb and the run loop's PauseRequested seam resolve the same path.
func PauseFlagPath(runDir string) string {
	return treadleengine.PauseFlagPath(runDir)
}

// PauseFlagName is the pause flag file's name inside a block's run dir,
// re-exported from treadleengine.
const PauseFlagName = treadleengine.PauseFlagName

// ErrBlockBusy is treadleengine's busy-block sentinel, re-exported (not
// copied) as the SAME instance so errors.Is matches across the two
// packages — perchcli branches on this exact sentinel regardless of which
// package's Run call produced it.
var ErrBlockBusy = treadleengine.ErrBlockBusy

// JudgeVerdict and TriageVerdict are type aliases onto treadleengine's
// identically-named types (byte-identical-perch-api shared decision):
// ParseJudgeVerdict/ParseTriageVerdict themselves move to treadleengine and
// are NOT re-exported — no external caller could have used them, since their
// judgeFraming parameter type was already unexported — but perch-side code
// (including run_test.go) that names a verdict value keeps compiling
// unchanged.
type (
	JudgeVerdict  = treadleengine.JudgeVerdict
	TriageVerdict = treadleengine.TriageVerdict
)

// The aliased JudgeVerdict/TriageVerdict constants, re-exported so
// perch-side references compile unchanged.
const (
	JudgeProgressing = treadleengine.JudgeProgressing
	JudgeCircling    = treadleengine.JudgeCircling
	JudgeContinue    = treadleengine.JudgeContinue
	JudgeStop        = treadleengine.JudgeStop
	JudgeUncertain   = treadleengine.JudgeUncertain
	TriageRetry      = treadleengine.TriageRetry
	TriageGiveUp     = treadleengine.TriageGiveUp
)
