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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
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

// overviewFrameHash hashes planDir's 00-overview.md with its Card Index section cut out, as the hex SHA-256 State.PlanOverviewFrameHash records.
// A plan directory without an overview has no frame and hashes to "", the value that makes Rebaseline refuse any overview change.
func overviewFrameHash(planDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(planDir, planOverviewFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("websterengine: overview frame hash %s: read %s: %w", planDir, planOverviewFile, err)
	}
	frame, err := planparser.OverviewWithoutCardIndex(data)
	if err != nil {
		return "", fmt.Errorf("websterengine: overview frame hash %s: %w", planDir, err)
	}
	sum := sha256.Sum256(frame)
	return hex.EncodeToString(sum[:]), nil
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

// PlanEditError returns nil when the plan on disk fingerprints to st.PlanFingerprint, and otherwise the ErrFingerprintMismatch wrap naming the way forward.
// BeginBatch, RecordBatch, PersistRecoveryTerminal and webstercli's validate call it before their own rewrites,
// so any difference it sees is someone else's edit rather than webster's own.
func PlanEditError(st *State, planDir string) error {
	fp, err := fingerprint(planDir)
	if err != nil {
		return err
	}
	if st.PlanFingerprint != fp {
		return fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; the plan changed since state.json was created; %s", ErrFingerprintMismatch, fp, st.PlanFingerprint, fingerprintMismatchWayForward(st, planDir, stepRun))
	}
	return nil
}

// batchCardEditError returns nil when every card of b still hashes to the content bs recorded at begin-batch, or when bs recorded none.
// Otherwise it returns an ErrFingerprintMismatch wrap naming each card that changed since its batch was begun.
func batchCardEditError(st *State, bs *BatchState, b batcher.Batch, planDir string) error {
	if len(bs.CardHashes) == 0 {
		return nil
	}
	now, err := batchCardHashes(b, planDir)
	if err != nil {
		return err
	}
	number, _ := batchIdentity(b)
	var changed []string
	for _, id := range batchCardIDs(b) {
		if want, ok := bs.CardHashes[id]; ok && want != now[id] {
			changed = append(changed, fmt.Sprintf("batch %02d card %s changed since it was begun", number, id))
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s; %s", ErrFingerprintMismatch, strings.Join(changed, "; "), fingerprintMismatchWayForward(st, planDir, stepRun))
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
// re-baselines after making them.
// Without this, the first batch that bound a handle or repaired drift made every later begin-batch fail ErrFingerprintMismatch, whose advised recourse (the reset route, then a plain `lyx webster run`) restarts the same plan into the same wall.
//
// Re-baselining costs nothing the guard was actually providing: a foreign edit landing between this
// call and the next begin-batch is still caught, which is the whole window the guard covers.
//
// It also stores the hashed content under websterDir (see storePlanBaseline) before mutating st, so a store failure leaves the state's hashes unchanged.
//
// It moves each begun batch's recorded CardHashes entry along with the rewrite (see moveBegunCardHashes),
// so a card webster's own rewrite changed is not later refused as an operator edit.
// The move is bounded: only a card whose recorded hash equals the pre-rewrite PlanFileHashes entry for its file moves.
// Every caller runs after a foreign-edit check passed in the same call, so an edit on disk when the call starts is refused rather than adopted;
// an edit landing between that check and this restamp is adopted with the rewrite.
// Rebaseline calls restampBaseline instead and never moves a begun card's hash.
func restampFingerprint(st *State, planDir, websterDir string) error {
	before := st.PlanFileHashes
	if err := restampBaseline(st, planDir, websterDir); err != nil {
		return err
	}
	moveBegunCardHashes(st, before, st.PlanFileHashes)
	return nil
}

// restampBaseline records planDir's fingerprint, per-file hashes, overview frame hash and stored baseline into st, and touches no batch record.
func restampBaseline(st *State, planDir, websterDir string) error {
	fp, err := fingerprint(planDir)
	if err != nil {
		return err
	}
	hashes, err := planFileHashes(planDir)
	if err != nil {
		return err
	}
	frame, err := overviewFrameHash(planDir)
	if err != nil {
		return err
	}
	if err := storePlanBaseline(websterDir, planDir, hashes); err != nil {
		return err
	}
	st.PlanFingerprint = fp
	st.PlanFileHashes = hashes
	st.PlanOverviewFrameHash = frame
	return nil
}

// moveBegunCardHashes sets each batch record's CardHashes entry for card id to after[id+".md"] when it equals before[id+".md"].
// A card whose recorded hash differs from before keeps it, so a card an earlier untracked rewrite already moved stays refused.
// A file absent from either map is left alone, and so is every record when before is empty.
func moveBegunCardHashes(st *State, before, after map[string]string) {
	if len(before) == 0 {
		return
	}
	for _, bs := range st.Batches {
		if bs == nil {
			continue
		}
		for id, recorded := range bs.CardHashes {
			file := id + ".md"
			old, okBefore := before[file]
			now, okAfter := after[file]
			if okBefore && okAfter && recorded == old {
				bs.CardHashes[id] = now
			}
		}
	}
}

// RestampPlanBaseline is restampFingerprint's exported seam for webstercli's validate verb, which re-baselines after its own rewrite-capable pass.
func RestampPlanBaseline(st *State, planDir, websterDir string) error {
	return restampFingerprint(st, planDir, websterDir)
}

// restampAndSaveFingerprint is restampFingerprint followed by SaveState, for Run — the one
// re-baseline site that owns its own state persistence rather than handing the state back to a CLI
// verb to save. Both halves are needed together: a re-baseline held only in memory is discarded by
// every path that returns before Run's later saves, which is exactly the wedge the re-baseline
// exists to prevent.
func restampAndSaveFingerprint(geom Geometry, st *State) error {
	if err := restampFingerprint(st, geom.PlanDir, geom.WebsterDir); err != nil {
		return err
	}
	return SaveState(geom.WebsterDir, geom.ScratchDir, st)
}
