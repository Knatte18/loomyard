// fingerprint_test.go covers fingerprint's identity properties: identical directories fingerprint
// identically,
// and a rename, a one-byte content edit, or an added batch file each change the result, while
// non-.md entries and subdirectories are ignored entirely.
// Tier 1: no git, only t.TempDir().

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// fingerprintWriteFiles writes every entry of files (keyed by relative
// path) into dir, creating any needed subdirectories.
func fingerprintWriteFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestFingerprint_IdenticalDirsMatch(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"00-overview.md": "overview content",
		"01-first.md":    "first content",
	}

	dirA := t.TempDir()
	dirB := t.TempDir()
	fingerprintWriteFiles(t, dirA, files)
	fingerprintWriteFiles(t, dirB, files)

	fpA, err := fingerprint(dirA)
	if err != nil {
		t.Fatalf("fingerprint(dirA) error = %v; want nil", err)
	}
	fpB, err := fingerprint(dirB)
	if err != nil {
		t.Fatalf("fingerprint(dirB) error = %v; want nil", err)
	}

	if fpA != fpB {
		t.Errorf("fingerprint(dirA) = %q; fingerprint(dirB) = %q; want equal for identical content", fpA, fpB)
	}
}

func TestFingerprint_ChangesOnRename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "content"})
	before, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if err := os.Rename(filepath.Join(dir, "01-first.md"), filepath.Join(dir, "01-renamed.md")); err != nil {
		t.Fatalf("rename: %v", err)
	}

	after, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if before == after {
		t.Errorf("fingerprint() = %q both before and after a rename; want it to change", before)
	}
}

func TestFingerprint_ChangesOnByteEdit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "content"})
	before, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "contenu"}) // one byte differs

	after, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if before == after {
		t.Errorf("fingerprint() = %q both before and after a one-byte edit; want it to change", before)
	}
}

func TestFingerprint_ChangesOnAddedBatchFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "content"})
	before, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	fingerprintWriteFiles(t, dir, map[string]string{"02-second.md": "more content"})

	after, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if before == after {
		t.Errorf("fingerprint() = %q both before and after adding a batch file; want it to change", before)
	}
}

func TestFingerprint_IgnoresNonMarkdownAndSubdirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "content"})
	before, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	// Add a non-.md file and a subdirectory containing a .md file; neither
	// should affect the fingerprint since only top-level *.md files count.
	fingerprintWriteFiles(t, dir, map[string]string{
		"notes.txt":           "ignored",
		"reports/report-1.md": "also ignored: this is inside a subdirectory",
	})

	after, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint() error = %v; want nil", err)
	}

	if before != after {
		t.Errorf("fingerprint() changed after adding a non-.md file and a subdirectory .md file; want unchanged (got %q, want %q)", after, before)
	}
}

// TestFingerprint_IgnoresTheAmendmentLog covers the amendment log's exclusion from plan identity.
// DetectDrift's exact-tier repair creates planparser.AmendmentsFileName inside the plan directory,
// so folding it into the fingerprint made webster's own repair invalidate the plan it had just
// repaired.
func TestFingerprint_IgnoresTheAmendmentLog(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{
		"00-overview.md": "overview content",
		"01-card.md":     "card content",
	})

	before, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint(before) returned error: %v", err)
	}

	fingerprintWriteFiles(t, dir, map[string]string{
		planparser.AmendmentsFileName: "# Amendments\n- Timestamp: t, Card: 1-card, OldGlyph: a#B, NewGlyph: a#C, Tier: exact, SHA: deadbeef\n",
	})

	after, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint(after) returned error: %v", err)
	}
	if before != after {
		t.Errorf("fingerprint changed when the amendment log appeared: before %s, after %s", before, after)
	}
}

// TestRestampFingerprint_RebaselinesTheStalenessGuard covers the re-baseline both bracket verbs
// perform after their own sanctioned plan rewrites. Without it, the first batch that bound a handle
// or repaired drift made every later begin-batch fail ErrFingerprintMismatch on webster's own edit.
func TestRestampFingerprint_RebaselinesTheStalenessGuard(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fingerprintWriteFiles(t, dir, map[string]string{"01-card.md": "before"})

	original, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint(original) returned error: %v", err)
	}
	st := &State{PlanFingerprint: original}

	// Stand in for BindHandles' own RewriteRefs pass.
	fingerprintWriteFiles(t, dir, map[string]string{"01-card.md": "after the bind"})

	websterDir := t.TempDir()
	if err := restampFingerprint(st, dir, websterDir); err != nil {
		t.Fatalf("restampFingerprint(...) returned error: %v", err)
	}
	if st.PlanFingerprint == original {
		t.Fatal("restampFingerprint left the stale fingerprint in place")
	}
	stored, err := os.ReadDir(filepath.Join(websterDir, planBaselineDirName))
	if err != nil {
		t.Fatalf("read plan baseline store: %v", err)
	}
	if len(stored) != len(st.PlanFileHashes) {
		t.Errorf("stored copies = %d; want one per recorded hash (%d)", len(stored), len(st.PlanFileHashes))
	}

	current, err := fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint(current) returned error: %v", err)
	}
	if st.PlanFingerprint != current {
		t.Errorf("State.PlanFingerprint = %s; want the plan directory's current fingerprint %s", st.PlanFingerprint, current)
	}
}

// editFixture writes a two-file plan, restamps a state over it, and returns the state and the batch holding card 01-a.
func editFixture(t *testing.T) (*State, *BatchState, batcher.Batch, string) {
	t.Helper()
	planDir := t.TempDir()
	fingerprintWriteFiles(t, planDir, map[string]string{
		"00-overview.md": "overview\n",
		"01-a.md":        "card a\n",
	})
	st := &State{}
	if err := restampFingerprint(st, planDir, t.TempDir()); err != nil {
		t.Fatalf("restampFingerprint() error = %v", err)
	}
	b := batcher.Batch{Cards: []planparser.Card{{Number: 1, Slug: "a"}}}
	hashes, err := batchCardHashes(b, planDir)
	if err != nil {
		t.Fatalf("batchCardHashes() error = %v", err)
	}
	return st, &BatchState{CardHashes: hashes}, b, planDir
}

// TestPlanEditError_NilOnUnchangedPlanAndNamesWayForwardAfterEdit proves an unchanged plan passes and a card edit wraps ErrFingerprintMismatch naming rebaseline and restore-plan.
func TestPlanEditError_NilOnUnchangedPlanAndNamesWayForwardAfterEdit(t *testing.T) {
	st, _, _, planDir := editFixture(t)

	if err := PlanEditError(st, planDir); err != nil {
		t.Fatalf("PlanEditError() on an unchanged plan = %v; want nil", err)
	}

	fingerprintWriteFiles(t, planDir, map[string]string{"01-a.md": "card a, edited\n"})
	err := PlanEditError(st, planDir)
	if !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("PlanEditError() after a card edit = %v; want ErrFingerprintMismatch", err)
	}
	for _, want := range []string{"rebaseline --card 01", "restore-plan"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("PlanEditError() = %q; want it to name %q", err, want)
		}
	}
}

// TestBatchCardEditError_NamesTheEditedBegunCard proves a begun card edited since its batch began is named,
// and an unedited one passes.
func TestBatchCardEditError_NamesTheEditedBegunCard(t *testing.T) {
	st, bs, b, planDir := editFixture(t)

	if err := batchCardEditError(st, bs, b, planDir); err != nil {
		t.Fatalf("batchCardEditError() on unedited cards = %v; want nil", err)
	}

	fingerprintWriteFiles(t, planDir, map[string]string{"01-a.md": "card a, edited\n"})
	err := batchCardEditError(st, bs, b, planDir)
	if !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("batchCardEditError() after a card edit = %v; want ErrFingerprintMismatch", err)
	}
	if want := "batch 01 card 01-a changed since it was begun"; !strings.Contains(err.Error(), want) {
		t.Errorf("batchCardEditError() = %q; want it to name %q", err, want)
	}
	if !strings.Contains(err.Error(), "restore-plan") {
		t.Errorf("batchCardEditError() = %q; want the restore-plan way forward", err)
	}

	if err := batchCardEditError(st, &BatchState{}, b, planDir); err != nil {
		t.Errorf("batchCardEditError() on a record without card hashes = %v; want nil", err)
	}
}
