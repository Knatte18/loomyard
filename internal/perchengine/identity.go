// identity.go implements perch's own block-identity derivation — ProfileHash, DeriveRunID,
// ValidRunID, sanitizeSlug — extracted out of the old state.go when its round-state machinery moved
// to treadleengine, plus perch's re-exports of treadleengine's identity/pause-flag/error-sentinel/
// verdict vocabulary (the byte-identical-perch-api shared decision): TerminalOutcome,
// PauseFlagPath, PauseFlagName, ErrBlockBusy, and the JudgeVerdict/TriageVerdict types and
// constants.

package perchengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/treadleengine"
)

// perchDirName is the relative-path segment perchengine joins onto both
// lyxdirs.LyxDirName (RunsDir) and lyxdirs.DotLyxDirName (ScratchDir) to
// form perch's durable and scratch base directories, respectively.
// perchengine is this segment's sole declarer.
const perchDirName = "perch"

// RunsDir returns the path to the base directory for perch run artifacts.
// It lives under _lyx so artifacts are fabric-synced.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func RunsDir(anchorPath string) string {
	return filepath.Join(anchorPath, lyxdirs.LyxDirName, perchDirName)
}

// ScratchDir returns the path to the base directory for a perch block's
// never-tracked artifacts (run.lock, state.json.lock, the pause flag) — the
// mirrored sibling of RunsDir under .lyx instead of _lyx. A caller joins a
// block's run-id onto this base exactly as it joins one onto RunsDir.
// Per the Cwd Resolution Invariant, no other package may construct this
// path.
func ScratchDir(anchorPath string) string {
	return filepath.Join(anchorPath, lyxdirs.DotLyxDirName, perchDirName)
}

// ProfileHash returns the sha256 hex digest of p's canonical JSON encoding.
func ProfileHash(p Profile) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("perch: marshal profile for hashing: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// DeriveRunID returns the default run-id for a standalone invocation: the profile file's sanitized
// basename plus first 8 hex characters of hash.
func DeriveRunID(profilePath string, hash string) string {
	base := filepath.Base(profilePath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return fmt.Sprintf("%s-%s", sanitizeSlug(base), hash[:8])
}

// ValidRunID reports whether id is a legal explicit --run-id: lowercase alphanumerics and single
// dashes, no leading/trailing dash, non-empty.
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

// sanitizeSlug lowercases s and replaces every run of non-alphanumeric characters with a single dash.
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

// TerminalOutcome reports the terminal Outcome recorded in runDir's state.json.
func TerminalOutcome(runDir, scratchDir string) (Outcome, bool, error) {
	outcome, ok, err := treadleengine.TerminalOutcome(runDir, scratchDir)
	if err != nil {
		logger.Warn("perch: read terminal outcome failed", "runDir", runDir, "scratchDir", scratchDir, "err", err)
	}
	return Outcome(outcome), ok, err
}

// PauseFlagPath returns the path to the pause flag file inside scratchDir.
func PauseFlagPath(scratchDir string) string {
	return treadleengine.PauseFlagPath(scratchDir)
}

// PauseFlagName is the pause flag file's name inside a block's run dir.
const PauseFlagName = treadleengine.PauseFlagName

// ErrBlockBusy is treadleengine's busy-block sentinel.
var ErrBlockBusy = treadleengine.ErrBlockBusy

// JudgeVerdict and TriageVerdict are type aliases onto treadleengine's identically-named types.
type (
	JudgeVerdict  = treadleengine.JudgeVerdict
	TriageVerdict = treadleengine.TriageVerdict
)

// The aliased JudgeVerdict/TriageVerdict constants.
const (
	JudgeProgressing = treadleengine.JudgeProgressing
	JudgeCircling    = treadleengine.JudgeCircling
	JudgeContinue    = treadleengine.JudgeContinue
	JudgeStop        = treadleengine.JudgeStop
	JudgeUncertain   = treadleengine.JudgeUncertain
	TriageRetry      = treadleengine.TriageRetry
	TriageGiveUp     = treadleengine.TriageGiveUp
)
