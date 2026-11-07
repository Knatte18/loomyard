// fingerprint.go computes the plan-identity hash that webster records in state.json at first init.
// Every later run and begin-batch entry recomputes it and compares it with the plan on disk,
// so the crash/resume guard can detect a plan that went stale across a crash/resume boundary.

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

// fingerprint computes a SHA-256 digest over the name and contents of each plan file in planDir.
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

// planFileNames lists the ".md" files in planDir that are plan content, sorted.
// The pipeline's own records are left out: the amendment log, the lens review, and the lens reports saved beside it.
// They record changes to the plan rather than plan content.
// Including them would make webster's own drift repair invalidate the plan it had just repaired.
func planFileNames(planDir string) ([]string, error) {
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return nil, fmt.Errorf("websterengine: plan files %s: %w", planDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == planparser.AmendmentsFileName || e.Name() == planparser.LensReviewFileName || planparser.IsLensReportFileName(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// planFileHashes returns the hex SHA-256 of each plan file in planDir, keyed by file name.
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

// overviewFrameHash returns the hex SHA-256 of planDir's 00-overview.md with its Card Index section cut out.
// It is the frame that State.PlanOverviewFrameHash records.
// A plan directory without an overview has no frame and hashes to "", the value that makes Rebaseline refuse any overview change.
func overviewFrameHash(planDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(planDir, planOverviewFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("websterengine: overview frame hash %s: read %s: %w", planDir, planOverviewFile, err)
	}
	hash, err := overviewFrameHashOf(data)
	if err != nil {
		return "", fmt.Errorf("websterengine: overview frame hash %s: %w", planDir, err)
	}
	return hash, nil
}

// overviewFrameHashOf returns the hex SHA-256 of overview, a 00-overview.md's content, with its Card Index cut out.
// It fails when overview has no Card Index or no well-formed frontmatter.
func overviewFrameHashOf(overview []byte) (string, error) {
	frame, err := planparser.OverviewWithoutCardIndex(overview)
	if err != nil {
		return "", err
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
// Callers that rewrite the plan call it before their own rewrites,
// so any difference it sees is someone else's edit rather than webster's own.
func PlanEditError(st *State, planDir, websterDir string) error {
	fp, err := fingerprint(planDir)
	if err != nil {
		return err
	}
	if st.PlanFingerprint != fp {
		return fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; the plan changed since state.json was created; %s", ErrFingerprintMismatch, fp, st.PlanFingerprint, fingerprintMismatchWayForward(st, planDir, websterDir, stepRun))
	}
	return nil
}

// batchCardEditError returns nil when every card of b still hashes to the content bs recorded at begin-batch, or when bs recorded none.
// Otherwise it returns an ErrFingerprintMismatch wrap naming each card that changed since its batch was begun.
func batchCardEditError(st *State, bs *BatchState, b batcher.Batch, planDir, websterDir string) error {
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
	return fmt.Errorf("%w: %s; %s", ErrFingerprintMismatch, strings.Join(changed, "; "), fingerprintMismatchWayForward(st, planDir, websterDir, stepRun))
}

// Fingerprint returns the hex SHA-256 digest of the plan files in planDir.
// The crash/resume guard in state.json compares this digest against the fingerprint the run recorded.
//
// It does not restamp state.json,
// so a plan rewritten after the call leaves the recorded fingerprint stale.
func Fingerprint(planDir string) (string, error) {
	return fingerprint(planDir)
}

// restampFingerprint re-baselines planDir's fingerprint into st.PlanFingerprint.
// It sets st.PlanFileHashes to the per-file hashes of planDir too.
// Callers call it after any planglyph pass that may have rewritten the plan on disk.
//
// The staleness guard catches a plan edited from outside the run between two batches,
// and it cannot tell such an edit apart from webster's own rewrites unless the run re-baselines after making them.
// Sanctioned rewrites: handle canonicalization at begin-batch; handle binding and drift repair at record-batch.
// Without the re-baseline, one rewrite would make every later begin-batch fail with ErrFingerprintMismatch.
//
// Re-baselining gives up nothing the guard provides:
// an edit that lands between this call and the next begin-batch is still caught.
//
// It stores the hashed content under websterDir before it mutates st.
// A store failure therefore leaves the state's hashes unchanged.
//
// It moves each begun batch's recorded CardHashes entry along with the rewrite,
// so a card that webster's own rewrite changed is not later refused as an operator edit.
// The move is bounded: only a card whose recorded hash equals the pre-rewrite PlanFileHashes entry for its file moves.
// Callers run a foreign-edit check first, so an edit already on disk is refused rather than adopted;
// an edit that lands between that check and this restamp is adopted with the rewrite.
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
		return fmt.Errorf("%w; way forward: transient, re-run the verb", err)
	}
	if err := storePlanBaseline(websterDir, planDir, hashes); err != nil {
		return err
	}
	st.PlanFingerprint = fp
	st.PlanFileHashes = hashes
	st.PlanOverviewFrameHash = frame
	return nil
}

// moveBegunCardHashes moves each batch record's CardHashes entry for a card to that card's hash in after.
// It moves only an entry equal to the card's hash in before.
// An entry that differs keeps its hash, so a card an earlier untracked rewrite already moved stays refused.
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

// RestampPlanBaseline re-baselines the plan fingerprint for a caller that has rewritten the plan and must record the new baseline.
func RestampPlanBaseline(st *State, planDir, websterDir string) error {
	return restampFingerprint(st, planDir, websterDir)
}

// restampAndSaveFingerprint re-baselines the plan fingerprint and persists the state in one step.
// The two must happen together: an in-memory re-baseline is lost on any return before the next save,
// and that loss is the wedge the re-baseline exists to prevent.
func restampAndSaveFingerprint(geom Geometry, st *State) error {
	if err := restampFingerprint(st, geom.PlanDir, geom.WebsterDir); err != nil {
		return err
	}
	return SaveState(geom.WebsterDir, geom.ScratchDir, st)
}
