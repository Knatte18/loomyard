// fingerprint.go computes the plan-identity hash a run records at first init.
// It also re-baselines the hash after the run's own plan rewrites.
// Every later entry recomputes it and compares it with the plan on disk,
// so the crash/resume guard can detect a plan that went stale across a crash/resume boundary.
// The re-baseline also stores the content behind every recorded hash, so the plan can be restored later.

package runplan

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

// PlanOverviewFile is the plan's overview file, which carries the integration verify.
// A rebaseline accepts a change to its Card Index only.
const PlanOverviewFile = "00-overview.md"

// ErrFingerprintMismatch is the sentinel an edit check returns for a plan that no longer matches its fingerprint.
// It means the on-disk plan's recomputed fingerprint disagrees with State.PlanFingerprint.
// It guards a crashed or resumed run against a plan edited since the run recorded its fingerprint.
var ErrFingerprintMismatch = errors.New("runplan: on-disk plan fingerprint does not match this run's recorded state")

// PlanBaselineDirName is the directory under the run directory that holds one file per recorded plan content.
// Each file is named by the hex SHA-256 of its content.
const PlanBaselineDirName = "plan-baseline"

// Fingerprint returns the hex SHA-256 digest over the name and contents of each plan file in planDir.
// The crash/resume guard compares this digest against the fingerprint the run recorded.
// It does not restamp the state, so a plan rewritten after the call leaves the recorded fingerprint stale.
func Fingerprint(planDir string) (string, error) {
	names, err := planFileNames(planDir)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			return "", fmt.Errorf("runplan: fingerprint %s: read %s: %w", planDir, name, err)
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
// Including them would make the run's own drift repair invalidate the plan it had just repaired.
func planFileNames(planDir string) ([]string, error) {
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return nil, fmt.Errorf("runplan: plan files %s: %w", planDir, err)
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

// PlanFileHashes returns the hex SHA-256 of each plan file in planDir, keyed by file name.
func PlanFileHashes(planDir string) (map[string]string, error) {
	names, err := planFileNames(planDir)
	if err != nil {
		return nil, err
	}
	hashes := make(map[string]string, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			return nil, fmt.Errorf("runplan: plan file hashes %s: read %s: %w", planDir, name, err)
		}
		sum := sha256.Sum256(data)
		hashes[name] = hex.EncodeToString(sum[:])
	}
	return hashes, nil
}

// OverviewFrameHash returns the hex SHA-256 of planDir's overview with its Card Index section cut out.
// It is the frame that State.PlanOverviewFrameHash records.
// A plan directory without an overview has no frame and hashes to "".
// That value makes a rebaseline refuse any overview change.
func OverviewFrameHash(planDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(planDir, PlanOverviewFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("runplan: overview frame hash %s: read %s: %w", planDir, PlanOverviewFile, err)
	}
	hash, err := OverviewFrameHashOf(data)
	if err != nil {
		return "", fmt.Errorf("runplan: overview frame hash %s: %w", planDir, err)
	}
	return hash, nil
}

// OverviewFrameHashOf returns the hex SHA-256 of overview, an overview file's content, with its Card Index cut out.
// It fails when overview has no Card Index or no well-formed frontmatter.
func OverviewFrameHashOf(overview []byte) (string, error) {
	frame, err := planparser.OverviewWithoutCardIndex(overview)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(frame)
	return hex.EncodeToString(sum[:]), nil
}

// The three causes RecordedOverviewFrame names for a run with no recorded overview frame.
const (
	OverviewFrameCauseNoHash         = "no overview hash was recorded"
	OverviewFrameCauseBaselineAbsent = "the baseline copy of the recorded overview is absent"
	OverviewFrameCauseNoCardIndex    = "the baseline copy of the recorded overview has no parseable Card Index"
)

// RecordedOverviewFrame returns the overview frame hash st records.
// A state without State.PlanOverviewFrameHash takes the frame of the baseline copy of its recorded 00-overview.md.
// That copy lies under runDir.
// The frame is "" when the run has no recorded frame:
// no overview hash recorded, the copy absent, or a copy without a parseable Card Index.
// For an empty frame, cause is one of the three OverviewFrameCause values, and path is the baseline copy's path.
// Path is empty when no hash was recorded; both are empty beside a frame.
// A read failure other than an absent copy is returned.
// A rebaseline and the plan gate both judge an overview edit against this frame,
// so the gate refuses what the rebaseline would.
func RecordedOverviewFrame(st *State, runDir string) (frame, cause, path string, err error) {
	if st.PlanOverviewFrameHash != "" {
		return st.PlanOverviewFrameHash, "", "", nil
	}
	hash := st.PlanFileHashes[PlanOverviewFile]
	if hash == "" {
		return "", OverviewFrameCauseNoHash, "", nil
	}
	path = PlanBaselinePath(runDir, hash)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", OverviewFrameCauseBaselineAbsent, path, nil
	}
	if err != nil {
		return "", "", "", fmt.Errorf("runplan: recorded overview frame: read plan baseline copy %s: %w", hash, err)
	}
	frame, err = OverviewFrameHashOf(data)
	if err != nil {
		return "", OverviewFrameCauseNoCardIndex, path, nil
	}
	return frame, "", "", nil
}

// ChangedPlanFiles returns the plan file names whose current hash differs from st.PlanFileHashes.
// It also returns files added or removed since, all sorted.
func ChangedPlanFiles(st *State, planDir string) ([]string, error) {
	now, err := PlanFileHashes(planDir)
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

// PlanEditError returns nil when the plan on disk fingerprints to st.PlanFingerprint.
// Otherwise it returns the ErrFingerprintMismatch wrap naming both fingerprints, followed by the text wayForward returns.
// That text is the caller's own account of the mismatch and its way forward.
// wayForward runs only on a mismatch, so a caller may read the changed files to word it.
// Callers that rewrite the plan call it before their own rewrites,
// so any difference it sees is someone else's edit rather than the run's own.
func PlanEditError(st *State, planDir string, wayForward func() string) error {
	fp, err := Fingerprint(planDir)
	if err != nil {
		return err
	}
	if st.PlanFingerprint != fp {
		return fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; %s", ErrFingerprintMismatch, fp, st.PlanFingerprint, wayForward())
	}
	return nil
}

// BatchCardEditError returns nil when every card of b still hashes to the content bs recorded at begin.
// It also returns nil when bs recorded none.
// Otherwise it returns an ErrFingerprintMismatch wrap naming each card that changed since its batch was begun.
// The wrap ends in the way forward wayForward returns.
func BatchCardEditError(bs *BatchState, b batcher.Batch, planDir string, wayForward func() string) error {
	if len(bs.CardHashes) == 0 {
		return nil
	}
	now, err := BatchCardHashes(b, planDir)
	if err != nil {
		return err
	}
	number, _ := BatchIdentity(b)
	var changed []string
	for _, id := range BatchCardIDs(b) {
		if want, ok := bs.CardHashes[id]; ok && want != now[id] {
			changed = append(changed, fmt.Sprintf("batch %02d card %s changed since it was begun", number, id))
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s; %s", ErrFingerprintMismatch, strings.Join(changed, "; "), wayForward())
}

// RestampFingerprint re-baselines planDir's fingerprint into st.PlanFingerprint.
// It sets st.PlanFileHashes to the per-file hashes of planDir too.
// Callers call it after any pass that may have rewritten the plan on disk.
//
// The staleness guard catches a plan edited from outside the run between two batches,
// and it cannot tell such an edit apart from the run's own rewrites unless the run re-baselines after making them.
// Sanctioned rewrites: handle canonicalization at dispatch; handle binding and drift repair in the post-batch checks.
// Without the re-baseline, one rewrite would make every later dispatch fail with ErrFingerprintMismatch.
//
// Re-baselining gives up nothing the guard provides:
// an edit that lands between this call and the next dispatch is still caught.
//
// It stores the hashed content under runDir before it mutates st.
// A store failure therefore leaves the state's hashes unchanged.
//
// It moves each begun batch's recorded CardHashes entry along with the rewrite,
// so a card that the run's own rewrite changed is not later refused as an operator edit.
// The move is bounded: only a card whose recorded hash equals the pre-rewrite PlanFileHashes entry for its file moves.
// Callers run a foreign-edit check first, so an edit already on disk is refused rather than adopted;
// an edit that lands between that check and this restamp is adopted with the rewrite.
func RestampFingerprint(st *State, planDir, runDir string) error {
	before := st.PlanFileHashes
	if err := RestampBaseline(st, planDir, runDir); err != nil {
		return err
	}
	moveBegunCardHashes(st, before, st.PlanFileHashes)
	return nil
}

// RestampBaseline records planDir's fingerprint, per-file hashes, overview frame hash and stored baseline into st.
// It touches no batch record.
func RestampBaseline(st *State, planDir, runDir string) error {
	fp, err := Fingerprint(planDir)
	if err != nil {
		return err
	}
	hashes, err := PlanFileHashes(planDir)
	if err != nil {
		return err
	}
	frame, err := OverviewFrameHash(planDir)
	if err != nil {
		return fmt.Errorf("%w; way forward: transient, re-run the verb", err)
	}
	if err := StorePlanBaseline(runDir, planDir, hashes); err != nil {
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

// PlanBaselinePath returns the stored copy's path for a content hash under runDir.
func PlanBaselinePath(runDir, hash string) string {
	return filepath.Join(runDir, PlanBaselineDirName, hash)
}

// StorePlanBaseline writes each plan file's bytes to its hash's file under runDir when that file is absent.
// The store is content-addressed, so a repeat write is a no-op.
// A file whose bytes no longer hash to their recorded value is refused, and nothing is written for it,
// so no stored copy is ever wrong.
// An empty runDir is a wiring error, returned rather than resolved against the working directory.
func StorePlanBaseline(runDir, planDir string, hashes map[string]string) error {
	if runDir == "" {
		return errors.New("runplan: store plan baseline: empty run dir")
	}
	dir := filepath.Join(runDir, PlanBaselineDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runplan: store plan baseline: %w", err)
	}
	names := make([]string, 0, len(hashes))
	for name := range hashes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dst := PlanBaselinePath(runDir, hashes[name])
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			return fmt.Errorf("runplan: store plan baseline: read %s: %w", name, err)
		}
		if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != hashes[name] {
			return fmt.Errorf("runplan: store plan baseline: %s changed since its hash was recorded; way forward: transient, re-run the verb once nothing else is editing the plan", name)
		}
		if err := WriteFileAtomic(dir, dst, data); err != nil {
			return fmt.Errorf("runplan: store plan baseline: %s: %w", name, err)
		}
	}
	return nil
}

// WriteFileAtomic writes data to dst through a temp file in dir and a rename,
// so a reader never observes a half-written file.
func WriteFileAtomic(dir, dst string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
