// fingerprint.go implements fingerprint, webster's own plan-identity hash constructor,
// deliberately module-local rather than shared.
// webster's state.json records the fingerprint at first init,
// and every later run/begin-batch entry recomputes and compares it against the on-disk plan, so a
// --fresh crash/resume guard (wired into state and runlevel in batch 7) can detect a stale plan
// across a crash/resume boundary.

package websterengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// fingerprint computes a SHA-256 digest over every "*.md" file's name and
// contents in planDir (sorted lexically). Non-.md entries are ignored, and so
// is planparser.AmendmentsFileName: the amendment log is an append-only record
// OF plan changes written by the pipeline itself, never plan content an author
// wrote, so folding it into plan identity made webster's own drift repair
// invalidate the plan it had just repaired.
func fingerprint(planDir string) (string, error) {
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return "", fmt.Errorf("websterengine: fingerprint %s: %w", planDir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == planparser.AmendmentsFileName {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			return "", fmt.Errorf("websterengine: fingerprint %s: read %s: %w", planDir, name, err)
		}
		// Trailing NUL separates name and contents to prevent concatenation collisions.
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// Fingerprint is fingerprint's exported seam for a caller outside this package that needs to know
// the SAME plan-identity digest webster's own bracket verbs compute, without going through
// BeginBatch/RecordBatch's own State-restamping side effect.
//
// webstercli's validate verb is the one caller (crucible round sonnet-xhigh-r8, WS-1): unlike every
// bracket verb, it runs the same rewrite-capable planglyph pass (handle canonicalization) without
// ever restamping state.json's PlanFingerprint afterward, so a plan rewritten mid-validate silently
// desynced the crash/resume guard until this seam existed for it to close that gap with.
func Fingerprint(planDir string) (string, error) {
	return fingerprint(planDir)
}

// restampFingerprint recomputes planDir's fingerprint into st.PlanFingerprint, and is called by
// each bracket verb after any planglyph pass that may have rewritten the plan on disk.
//
// The staleness guard exists to catch a plan edited from OUTSIDE the run between two batches, and
// it cannot tell that apart from webster's own sanctioned rewrites — handle canonicalization at
// begin-batch, handle binding and exact-tier drift repair at record-batch — unless the run
// re-baselines after making them. Without this, the first batch that bound a handle or repaired
// drift made every later begin-batch fail ErrFingerprintMismatch, whose advised recourse
// (`lyx webster run --fresh`) restarts the same plan into the same wall.
//
// Re-baselining costs nothing the guard was actually providing: a foreign edit landing between this
// call and the next begin-batch is still caught, which is the whole window the guard covers.
func restampFingerprint(st *State, planDir string) error {
	fp, err := fingerprint(planDir)
	if err != nil {
		return err
	}
	st.PlanFingerprint = fp
	return nil
}

// restampAndSaveFingerprint is restampFingerprint followed by SaveState, for Run — the one
// re-baseline site that owns its own state persistence rather than handing the state back to a CLI
// verb to save. Both halves are needed together: a re-baseline held only in memory is discarded by
// every path that returns before Run's later saves, which is exactly the wedge the re-baseline
// exists to prevent.
func restampAndSaveFingerprint(geom Geometry, st *State) error {
	if err := restampFingerprint(st, geom.PlanDir); err != nil {
		return err
	}
	return SaveState(geom.WebsterDir, geom.ScratchDir, st)
}
