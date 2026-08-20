package shedadapters

import (
	"os"
	"path/filepath"
	"testing"
)

// assertFocus compares got against the wanted ExcludeLenses/Hydrate contents, treating a nil slice
// and an empty slice as equal -- readRoundFocus's choice between the two in any given branch is an
// implementation detail, not part of its contract.
func assertFocus(t *testing.T, got RoundFocus, wantExclude, wantHydrate []string) {
	t.Helper()
	if !stringSlicesEqual(got.ExcludeLenses, wantExclude) {
		t.Errorf("readRoundFocus() ExcludeLenses = %v; want %v", got.ExcludeLenses, wantExclude)
	}
	if !stringSlicesEqual(got.Hydrate, wantHydrate) {
		t.Errorf("readRoundFocus() Hydrate = %v; want %v", got.Hydrate, wantHydrate)
	}
}

func stringSlicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func writeFocusFile(t *testing.T, runDir string, round int, content string) {
	t.Helper()
	path := filepath.Join(runDir, roundFocusFilename(round))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestReadRoundFocus_WellFormedFileYieldsBothFields(t *testing.T) {
	dir := t.TempDir()
	hydrateA := filepath.Join(dir, "a.md")
	hydrateB := filepath.Join(dir, "b.md")
	for _, p := range []string{hydrateA, hydrateB} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", p, err)
		}
	}
	writeFocusFile(t, dir, 3, `{"exclude_lenses":["lensA","lensB"],"hydrate":["`+hydrateA+`","`+hydrateB+`"]}`)

	got := readRoundFocus("bouncer", dir, 3)

	assertFocus(t, got, []string{"lensA", "lensB"}, []string{hydrateA, hydrateB})
}

func TestReadRoundFocus_OnlyExcludeLensesYieldsEmptyHydrate(t *testing.T) {
	dir := t.TempDir()
	writeFocusFile(t, dir, 1, `{"exclude_lenses":["lensA"]}`)

	got := readRoundFocus("bouncer", dir, 1)

	assertFocus(t, got, []string{"lensA"}, []string{})
}

func TestReadRoundFocus_OnlyHydrateYieldsEmptyExcludeLenses(t *testing.T) {
	dir := t.TempDir()
	hydrateA := filepath.Join(dir, "a.md")
	if err := os.WriteFile(hydrateA, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", hydrateA, err)
	}
	writeFocusFile(t, dir, 1, `{"hydrate":["`+hydrateA+`"]}`)

	got := readRoundFocus("bouncer", dir, 1)

	assertFocus(t, got, []string{}, []string{hydrateA})
}

func TestReadRoundFocus_AbsentFileYieldsZeroValue(t *testing.T) {
	dir := t.TempDir()

	got := readRoundFocus("bouncer", dir, 1)

	assertFocus(t, got, []string{}, []string{})
}

func TestReadRoundFocus_UnreadableFileYieldsZeroValue(t *testing.T) {
	dir := t.TempDir()
	// A directory at the focus file's own path produces a deterministic read failure -- file
	// permissions do not fail for a privileged test process.
	path := filepath.Join(dir, roundFocusFilename(1))
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir(%s): %v", path, err)
	}

	got := readRoundFocus("bouncer", dir, 1)

	assertFocus(t, got, []string{}, []string{})
}

func TestReadRoundFocus_MalformedJSONYieldsZeroValue(t *testing.T) {
	dir := t.TempDir()
	writeFocusFile(t, dir, 1, `{not valid json`)

	got := readRoundFocus("bouncer", dir, 1)

	assertFocus(t, got, []string{}, []string{})
}

func TestReadRoundFocus_UnknownFieldYieldsZeroValue(t *testing.T) {
	dir := t.TempDir()
	writeFocusFile(t, dir, 1, `{"exclude_lenses":["lensA"],"unexpected_field":true}`)

	got := readRoundFocus("bouncer", dir, 1)

	assertFocus(t, got, []string{}, []string{})
}

func TestReadRoundFocus_RelativeHydrateEntryDroppedOthersSurvive(t *testing.T) {
	dir := t.TempDir()
	hydrateA := filepath.Join(dir, "a.md")
	if err := os.WriteFile(hydrateA, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", hydrateA, err)
	}
	writeFocusFile(t, dir, 1, `{"hydrate":["`+hydrateA+`","relative/path.md"]}`)

	got := readRoundFocus("bouncer", dir, 1)

	assertFocus(t, got, []string{}, []string{hydrateA})
}

func TestReadRoundFocus_AbsentHydrateEntryDroppedOthersSurvive(t *testing.T) {
	dir := t.TempDir()
	hydrateA := filepath.Join(dir, "a.md")
	if err := os.WriteFile(hydrateA, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", hydrateA, err)
	}
	missing := filepath.Join(dir, "missing.md")
	writeFocusFile(t, dir, 1, `{"hydrate":["`+hydrateA+`","`+missing+`"]}`)

	got := readRoundFocus("bouncer", dir, 1)

	assertFocus(t, got, []string{}, []string{hydrateA})
}

func TestReadRoundFocus_ResolvesFilenameByTargetRound(t *testing.T) {
	dir := t.TempDir()
	writeFocusFile(t, dir, 3, `{"exclude_lenses":["lensA"]}`)

	found := readRoundFocus("bouncer", dir, 3)
	assertFocus(t, found, []string{"lensA"}, []string{})

	notFound := readRoundFocus("bouncer", dir, 4)
	assertFocus(t, notFound, []string{}, []string{})
}
