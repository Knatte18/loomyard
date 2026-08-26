// reconcile_test.go covers Read, Reconcile, and ForceRefresh against a bare t.TempDir(), with no
// git and no hub fixture -- see fakeRegistry below for the Registry stand-in every test here uses.

package stencilstore

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// fakeRegistry implements Registry over a plain map, so no test in this package depends on the real
// stencils package.
type fakeRegistry struct {
	names    []string
	defaults map[string][]byte
}

// newFakeRegistry builds a fakeRegistry whose Names() returns defaults' keys in sorted order.
func newFakeRegistry(defaults map[string][]byte) *fakeRegistry {
	names := make([]string, 0, len(defaults))
	for name := range defaults {
		names = append(names, name)
	}
	sort.Strings(names)
	return &fakeRegistry{names: names, defaults: defaults}
}

// Names implements Registry.
func (r *fakeRegistry) Names() []string {
	return r.names
}

// Default implements Registry.
func (r *fakeRegistry) Default(name string) ([]byte, bool) {
	content, ok := r.defaults[name]
	return content, ok
}

// readAll returns the raw bytes on disk at baseDir for name, or nil if absent.
func readAll(t *testing.T, baseDir, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(Path(baseDir, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("os.ReadFile(%s) failed: %v", Path(baseDir, name), err)
	}
	return content
}

func TestReconcile_SeedsAbsentFile(t *testing.T) {
	baseDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{"family-one": []byte("shipped body\n")})

	written, err := Reconcile(baseDir, registry, ModeProduction, "")
	if err != nil {
		t.Fatalf("Reconcile(...) returned error: %v", err)
	}

	onDisk := readAll(t, baseDir, "family-one")
	stamp, ok := ParseStamp(onDisk)
	if !ok || stamp != BodyHash(onDisk) {
		t.Fatalf("seeded file's stamp = (%q, %v); want it to equal its own BodyHash", stamp, ok)
	}
	if !containsPath(written, RelPath("family-one")) {
		t.Errorf("Reconcile(...) written = %v; want it to include %q", written, RelPath("family-one"))
	}
}

// containsPath reports whether paths contains want.
func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

// seedGitattributesForTest pre-creates baseDir/.gitattributes so a test's own Reconcile call can
// assert on written without that call also seeding .gitattributes for the first time.
func seedGitattributesForTest(t *testing.T, baseDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(baseDir, gitattributesName), []byte(gitattributesContent), 0o644); err != nil {
		t.Fatalf("seedGitattributesForTest: os.WriteFile failed: %v", err)
	}
}

func TestReconcile_UntouchedUnchangedDefaultLeftAlone(t *testing.T) {
	baseDir := t.TempDir()
	shipped := []byte("shipped body\n")
	registry := newFakeRegistry(map[string][]byte{"family-one": shipped})

	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}
	before := readAll(t, baseDir, "family-one")

	written, err := Reconcile(baseDir, registry, ModeProduction, "")
	if err != nil {
		t.Fatalf("second Reconcile(...) returned error: %v", err)
	}
	after := readAll(t, baseDir, "family-one")

	if string(before) != string(after) {
		t.Errorf("Reconcile(...) modified an untouched-unchanged file: before=%q after=%q", before, after)
	}
	if len(written) != 0 {
		t.Errorf("Reconcile(...) written = %v; want empty", written)
	}
}

func TestReconcile_UntouchedChangedDefault(t *testing.T) {
	baseDir := t.TempDir()
	original := []byte("original shipped body\n")
	registry := newFakeRegistry(map[string][]byte{"family-one": original})
	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}

	updated := []byte("updated shipped body\n")
	registry.defaults["family-one"] = updated

	t.Run("ModeProduction_Overwrites", func(t *testing.T) {
		written, err := Reconcile(baseDir, registry, ModeProduction, "")
		if err != nil {
			t.Fatalf("Reconcile(...) returned error: %v", err)
		}
		onDisk := readAll(t, baseDir, "family-one")
		if BodyHash(onDisk) != BodyHash(updated) {
			t.Errorf("ModeProduction did not refresh: BodyHash(onDisk) = %q; want %q", BodyHash(onDisk), BodyHash(updated))
		}
		if len(written) != 1 {
			t.Errorf("Reconcile(...) written = %v; want one entry", written)
		}
	})
}

func TestReconcile_ModeDevDoesNotRefreshUntouched(t *testing.T) {
	baseDir := t.TempDir()
	original := []byte("original shipped body\n")
	registry := newFakeRegistry(map[string][]byte{"family-one": original})
	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}
	before := readAll(t, baseDir, "family-one")

	registry.defaults["family-one"] = []byte("updated shipped body\n")

	written, err := Reconcile(baseDir, registry, ModeDev, "")
	if err != nil {
		t.Fatalf("Reconcile(ModeDev) returned error: %v", err)
	}
	after := readAll(t, baseDir, "family-one")

	if string(before) != string(after) {
		t.Errorf("ModeDev modified an untouched file on disk: before=%q after=%q", before, after)
	}
	if len(written) != 0 {
		t.Errorf("Reconcile(ModeDev) written = %v; want empty", written)
	}
}

func TestReconcile_ModeDevStillRestampsReconciledRow(t *testing.T) {
	baseDir := t.TempDir()
	shipped := []byte("dev default body\n")
	registry := newFakeRegistry(map[string][]byte{"family-one": shipped})

	// Seed a file whose body equals the dev default but whose stamp names an older hash.
	staleHash := BodyHash([]byte("some other old default\n"))
	path := Path(baseDir, "family-one")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, ApplyStamp(shipped, staleHash), 0o644); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}
	seedGitattributesForTest(t, baseDir)

	written, err := Reconcile(baseDir, registry, ModeDev, "")
	if err != nil {
		t.Fatalf("Reconcile(ModeDev) returned error: %v", err)
	}
	if len(written) != 1 {
		t.Errorf("Reconcile(ModeDev) written = %v; want one restamp entry", written)
	}

	onDisk := readAll(t, baseDir, "family-one")
	stamp, ok := ParseStamp(onDisk)
	if !ok || stamp != BodyHash(onDisk) {
		t.Fatalf("restamped file's stamp = (%q, %v); want it to equal its own BodyHash", stamp, ok)
	}

	if got := Classify(onDisk, true, shipped); got != StateUntouched {
		t.Errorf("Classify(restamped file) = %v; want %v", got, StateUntouched)
	}
}

func TestForceRefresh_PerformsRefreshRowFromDevSkippedFile(t *testing.T) {
	baseDir := t.TempDir()
	original := []byte("original shipped body\n")
	registry := newFakeRegistry(map[string][]byte{"family-one": original})
	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}

	updated := []byte("updated shipped body\n")
	registry.defaults["family-one"] = updated
	if _, err := Reconcile(baseDir, registry, ModeDev, ""); err != nil {
		t.Fatalf("Reconcile(ModeDev) returned error: %v", err)
	}
	skipped := readAll(t, baseDir, "family-one")
	if BodyHash(skipped) != BodyHash(original) {
		t.Fatalf("precondition failed: ModeDev already refreshed the file")
	}

	written, err := ForceRefresh(baseDir, registry, "")
	if err != nil {
		t.Fatalf("ForceRefresh(...) returned error: %v", err)
	}
	if len(written) != 1 {
		t.Errorf("ForceRefresh(...) written = %v; want one entry", written)
	}

	refreshed := readAll(t, baseDir, "family-one")
	if BodyHash(refreshed) != BodyHash(updated) {
		t.Errorf("ForceRefresh(...) did not refresh: BodyHash(refreshed) = %q; want %q", BodyHash(refreshed), BodyHash(updated))
	}
}

func TestReconcile_EditedFileNeverModified(t *testing.T) {
	baseDir := t.TempDir()
	shipped := []byte("shipped body\n")
	registry := newFakeRegistry(map[string][]byte{"family-one": shipped})
	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}

	edited := ApplyStamp([]byte("hand-edited body\n"), BodyHash(shipped))
	if err := os.WriteFile(Path(baseDir, "family-one"), edited, 0o644); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}

	registry.defaults["family-one"] = []byte("newer shipped body\n")
	written, err := Reconcile(baseDir, registry, ModeProduction, "")
	if err != nil {
		t.Fatalf("Reconcile(...) returned error: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("Reconcile(...) written = %v; want empty for an edited file", written)
	}

	onDisk := readAll(t, baseDir, "family-one")
	if string(onDisk) != string(edited) {
		t.Errorf("Reconcile(...) modified an edited file: onDisk=%q want=%q", onDisk, edited)
	}

	read, err := Read(baseDir, "family-one")
	if err != nil {
		t.Fatalf("Read(...) returned error: %v", err)
	}
	if string(read) != string(edited) {
		t.Errorf("Read(...) = %q; want the edited content %q", read, edited)
	}
}

func TestReconcile_MissingOrMalformedStampTreatedAsEdited(t *testing.T) {
	baseDir := t.TempDir()
	shipped := []byte("shipped body\n")
	registry := newFakeRegistry(map[string][]byte{"family-one": shipped})

	unstamped := []byte("body with no stamp at all\n")
	path := Path(baseDir, "family-one")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, unstamped, 0o644); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}
	seedGitattributesForTest(t, baseDir)

	written, err := Reconcile(baseDir, registry, ModeProduction, "")
	if err != nil {
		t.Fatalf("Reconcile(...) returned error: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("Reconcile(...) written = %v; want empty for an unstamped file", written)
	}
	onDisk := readAll(t, baseDir, "family-one")
	if string(onDisk) != string(unstamped) {
		t.Errorf("Reconcile(...) modified an unstamped file: onDisk=%q want=%q", onDisk, unstamped)
	}
}

func TestReconcile_RevertedFileRestampsAndReturnsToUntouched(t *testing.T) {
	baseDir := t.TempDir()
	shipped := []byte("shipped body\n")
	registry := newFakeRegistry(map[string][]byte{"family-one": shipped})
	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}

	// Edit then revert to the exact default body, but leave the (now stale) stamp behind.
	staleHash := BodyHash([]byte("an unrelated edit\n"))
	reverted := ApplyStamp(shipped, staleHash)
	if err := os.WriteFile(Path(baseDir, "family-one"), reverted, 0o644); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}

	written, err := Reconcile(baseDir, registry, ModeProduction, "")
	if err != nil {
		t.Fatalf("Reconcile(...) returned error: %v", err)
	}
	if len(written) != 1 {
		t.Errorf("Reconcile(...) written = %v; want one restamp entry", written)
	}

	onDisk := readAll(t, baseDir, "family-one")
	if got := Classify(onDisk, true, shipped); got != StateUntouched {
		t.Errorf("Classify(reverted+restamped file) = %v; want %v", got, StateUntouched)
	}
}

func TestReconcile_NoChangesWritesNothing(t *testing.T) {
	baseDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{
		"family-one": []byte("body one\n"),
		"family-two": []byte("body two\n"),
	})
	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}
	before := map[string][]byte{
		"family-one": readAll(t, baseDir, "family-one"),
		"family-two": readAll(t, baseDir, "family-two"),
	}

	written, err := Reconcile(baseDir, registry, ModeProduction, "")
	if err != nil {
		t.Fatalf("second Reconcile(...) returned error: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("Reconcile(...) written = %v; want empty", written)
	}
	for name, want := range before {
		got := readAll(t, baseDir, name)
		if string(got) != string(want) {
			t.Errorf("Reconcile(...) modified %q: before=%q after=%q", name, want, got)
		}
	}
}

func TestReconcile_SeedsGitattributesOnce(t *testing.T) {
	baseDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{"family-one": []byte("body\n")})

	written, err := Reconcile(baseDir, registry, ModeProduction, "")
	if err != nil {
		t.Fatalf("Reconcile(...) returned error: %v", err)
	}

	found := false
	for _, path := range written {
		if path == gitattributesName {
			found = true
		}
	}
	if !found {
		t.Errorf("Reconcile(...) written = %v; want it to include %q on the seeding run", written, gitattributesName)
	}

	content, err := os.ReadFile(filepath.Join(baseDir, gitattributesName))
	if err != nil {
		t.Fatalf("os.ReadFile(.gitattributes) failed: %v", err)
	}
	if string(content) != gitattributesContent {
		t.Errorf(".gitattributes content = %q; want %q", content, gitattributesContent)
	}

	if err := os.WriteFile(filepath.Join(baseDir, gitattributesName), []byte("custom content\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}

	written, err = Reconcile(baseDir, registry, ModeProduction, "")
	if err != nil {
		t.Fatalf("Reconcile(...) returned error: %v", err)
	}
	for _, path := range written {
		if path == gitattributesName {
			t.Errorf("Reconcile(...) rewrote an existing .gitattributes")
		}
	}
	content, err = os.ReadFile(filepath.Join(baseDir, gitattributesName))
	if err != nil {
		t.Fatalf("os.ReadFile(.gitattributes) failed: %v", err)
	}
	if string(content) != "custom content\n" {
		t.Errorf(".gitattributes content = %q; want it left untouched", content)
	}

	for _, name := range registry.Names() {
		if name == gitattributesName {
			t.Errorf("registry.Names() = %v; want it to never include %q", registry.Names(), gitattributesName)
		}
	}
}

func TestReconcile_EmptySourceDirSkipsDriftComparison(t *testing.T) {
	baseDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{"family-one": []byte("body\n")})

	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("Reconcile(...) with empty sourceDir returned error: %v", err)
	}
}

func TestReconcile_DriftingSourceDirStillSucceeds(t *testing.T) {
	baseDir := t.TempDir()
	sourceDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{"family-one": []byte("board body\n")})

	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}

	sourcePath := filepath.Join(sourceDir, filepath.FromSlash(RelPath("family-one")))
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("different worktree body\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}

	written, err := Reconcile(baseDir, registry, ModeProduction, sourceDir)
	if err != nil {
		t.Fatalf("Reconcile(...) with a drifting sourceDir returned error: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("Reconcile(...) written = %v; want empty on a run where only .gitattributes was already seeded", written)
	}
}

// TestReconcile_ModeDevRefusalNamesItsRemedy pins the dev-mode refusal's message, not just its
// behaviour. The refusal is one-way -- an older installed binary running in production mode DOES
// refresh, so it can downgrade a board's stencils, after which a newer dev build can only warn,
// on every invocation, forever. A warning that does not name "lyx stencil sync" reads as benign
// housekeeping while it is reporting that every producer will run on the older on-disk prompt.
func TestReconcile_ModeDevRefusalNamesItsRemedy(t *testing.T) {
	baseDir := t.TempDir()
	registry := newFakeRegistry(map[string][]byte{"family-one": []byte("original shipped body\n")})
	if _, err := Reconcile(baseDir, registry, ModeProduction, ""); err != nil {
		t.Fatalf("seed Reconcile(...) returned error: %v", err)
	}
	registry.defaults["family-one"] = []byte("updated shipped body\n")

	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	if _, err := Reconcile(baseDir, registry, ModeDev, ""); err != nil {
		t.Fatalf("Reconcile(ModeDev) returned error: %v", err)
	}

	got := buf.String()
	for _, want := range []string{"lyx stencil sync", "OLDER", "family-one"} {
		if !strings.Contains(got, want) {
			t.Errorf("dev-mode refusal log = %q; want it to contain %q", got, want)
		}
	}
}
