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
	names, err := planFileNames(planDir)
	if err != nil {
		return "", err
	}

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

// planFileNames lists the plan files fingerprint covers, sorted: every ".md" file in planDir but planparser.AmendmentsFileName.
func planFileNames(planDir string) ([]string, error) {
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return nil, fmt.Errorf("websterengine: plan files %s: %w", planDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == planparser.AmendmentsFileName {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// planFileHashes hashes the same file set fingerprint reads, one hex SHA-256 per file name.
func planFileHashes(planDir string) (map[string]string, error) {
	names, err := planFileNames(planDir)
	if err != nil {
		return nil, err
	}
	hashes := make(map[string]string, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			return nil, fmt.Errorf("websterengine: plan file hashes %s: read %s: %w", planDir, name, err)
		}
		sum := sha256.Sum256(data)
		hashes[name] = hex.EncodeToString(sum[:])
	}
	return hashes, nil
}

// changedPlanFiles returns the plan file names whose current hash differs from st.PlanFileHashes, plus files added or removed since, sorted.
func changedPlanFiles(st *State, planDir string) ([]string, error) {
	now, err := planFileHashes(planDir)
	if err != nil {
		return nil, err
	}
	var changed []string
	for name, h := range now {
		if want, ok := st.PlanFileHashes[name]; !ok || want != h {
			changed = append(changed, name)
		}
	}
	for name := range st.PlanFileHashes {
		if _, ok := now[name]; !ok {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed, nil
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

// restampFingerprint recomputes planDir's fingerprint into st.PlanFingerprint and its per-file hashes into st.PlanFileHashes.
// Each bracket verb calls it after any planglyph pass that may have rewritten the plan on disk.
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
	hashes, err := planFileHashes(planDir)
	if err != nil {
		return err
	}
	st.PlanFingerprint = fp
	st.PlanFileHashes = hashes
	return nil
}

// RestampPlanBaseline is restampFingerprint's exported seam for webstercli's validate verb, which re-baselines after its own rewrite-capable pass.
func RestampPlanBaseline(st *State, planDir string) error {
	return restampFingerprint(st, planDir)
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

// RebaselinePlanFingerprint restamps the recorded plan fingerprint to the plan's current digest, for a caller outside this package that has just rewritten the plan on disk.
//
// It serves one rule: every sanctioned plan rewrite is re-baselined immediately by its writer.
// Without it the next run entry sees a plan that differs from the recorded fingerprint and refuses with ErrFingerprintMismatch, whose advised recourse (a fresh run) would discard the batch records the rewrite meant to keep.
//
// It runs outside Run, so it takes the state-mutation lease itself and holds it across LoadState, the restamp and SaveState, releasing it before returning.
// A nil state means no webster run has started, so there is no recorded fingerprint to desync and the call is a no-op.
func RebaselinePlanFingerprint(geom Geometry) error {
	lease, err := AcquireStateMutation(geom.ScratchDir)
	if err != nil {
		return err
	}
	defer lease.Release()

	st, err := LoadState(geom.WebsterDir, geom.ScratchDir)
	if err != nil {
		return err
	}
	if st == nil {
		return nil
	}
	return restampAndSaveFingerprint(geom, st)
}
