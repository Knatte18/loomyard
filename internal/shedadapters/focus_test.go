// focus_test.go covers readRoundFocus against the focus file the Bouncer writes.
// Its tests write through the writer and read through the reader.
// A divergence in filename, format or field set fails here instead of emptying the judge's targeting channel.

package shedadapters

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// assertFocus compares got against the wanted ExcludeLenses contents and DirectivePath.
// A nil and an empty slice compare equal: readRoundFocus's choice between them is not part of its contract.
func assertFocus(t *testing.T, got RoundFocus, wantExclude []string, wantDirective string) {
	t.Helper()
	if !stringSlicesEqual(got.ExcludeLenses, wantExclude) {
		t.Errorf("readRoundFocus() ExcludeLenses = %v; want %v", got.ExcludeLenses, wantExclude)
	}
	if got.DirectivePath != wantDirective {
		t.Errorf("readRoundFocus() DirectivePath = %q; want %q", got.DirectivePath, wantDirective)
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

// writeFocusFile renders f through the Bouncer's own writer into round's focus path and returns that
// path. Every well-formed fixture in this package goes through the real writer rather than a hand-built string.
// A fixture can therefore never assert a shape the writer does not produce.
func writeFocusFile(t *testing.T, runDir string, round int, f focusFile) string {
	t.Helper()
	path := focusPath(runDir, round)
	if err := writeFocus(path, f); err != nil {
		t.Fatalf("writeFocus(%s): %v", path, err)
	}
	return path
}

// writeFocusFileRaw writes content verbatim to round's focus path.
// It serves the malformed-input rows, which cannot go through the renderer.
func writeFocusFileRaw(t *testing.T, runDir string, round int, content string) string {
	t.Helper()
	path := focusPath(runDir, round)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

// TestReadRoundFocus_DirectivePathOnlyWhenTheFileSaysSomething drives the writer and the reader against one another.
// The directive path appears only when the file says something.
// A CONVERGED judge's mandatory but empty focus file carries no directive path.
// Handing the next round a document that asserts nothing is noise, not targeting.
// The round in the file's name is the round the directives are for, so reading another round must not find the file.
//
//testtiming:keep pins the focus reader against the file the writer produces: the directive path only when the file says something, the exclude lenses, and the target round's filename
func TestReadRoundFocus_DirectivePathOnlyWhenTheFileSaysSomething(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		file focusFile
		// readRound is the round read; zero reads the file's own round.
		readRound    int
		wantExclude  []string
		wantHydrated bool
	}{
		{
			name:         "EmptyListsAndNoProse",
			file:         focusFile{Round: 1, ExcludeLenses: []string{}, Focus: []string{}},
			wantExclude:  []string{},
			wantHydrated: false,
		},
		{
			name:         "FocusDirectiveOnly",
			file:         focusFile{Round: 1, ExcludeLenses: []string{}, Focus: []string{"look at the card index"}},
			wantExclude:  []string{},
			wantHydrated: true,
		},
		{
			name:         "ProseOnly",
			file:         focusFile{Round: 1, ExcludeLenses: []string{}, Focus: []string{}, Prose: "the plan grew a scope the record does not license"},
			wantExclude:  []string{},
			wantHydrated: true,
		},
		{
			name:         "ExcludeLensesAloneIsNotADirectiveToRead",
			file:         focusFile{Round: 1, ExcludeLenses: []string{"lensA"}, Focus: []string{}},
			wantExclude:  []string{"lensA"},
			wantHydrated: false,
		},
		{
			name: "ExcludeLensesFocusAndProseTogether",
			file: focusFile{
				Round:         3,
				ExcludeLenses: []string{"lensA", "lensB"},
				Focus:         []string{"check the relocation candidate in the Auto-mode assumptions section"},
				Prose:         "Seed round: nothing has been reviewed yet.",
			},
			wantExclude:  []string{"lensA", "lensB"},
			wantHydrated: true,
		},
		{
			name:         "AnotherRoundsFileIsNotFound",
			file:         focusFile{Round: 3, ExcludeLenses: []string{"lensA"}, Focus: []string{"a directive"}},
			readRound:    4,
			wantExclude:  []string{},
			wantHydrated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := writeFocusFile(t, dir, tt.file.Round, tt.file)
			readRound := tt.readRound
			if readRound == 0 {
				readRound = tt.file.Round
			}

			got := readRoundFocus("bouncer", dir, readRound)

			wantDirective := ""
			if tt.wantHydrated {
				wantDirective = path
			}
			assertFocus(t, got, tt.wantExclude, wantDirective)
		})
	}
}

// TestReadRoundFocus_DegradesToTheZeroDirective covers every fail-safe branch: the reader must never
// return an error, because a missing or broken targeting hint must not retract a round.
func TestReadRoundFocus_DegradesToTheZeroDirective(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("AbsentFile", func(t *testing.T) {
		got := readRoundFocus("bouncer", t.TempDir(), 1)
		assertFocus(t, got, []string{}, "")
	})

	t.Run("UnreadableFile", func(t *testing.T) {
		dir := t.TempDir()
		// A directory at the focus file's own path produces a deterministic read failure;
		// file
		// permissions do not fail for a privileged test process.
		if err := os.Mkdir(focusPath(dir, 1), 0o755); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, "")
	})

	t.Run("NoFrontmatter", func(t *testing.T) {
		dir := t.TempDir()
		writeFocusFileRaw(t, dir, 1, "just prose, no delimiters\n")
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, "")
	})

	t.Run("UnclosedFrontmatter", func(t *testing.T) {
		dir := t.TempDir()
		writeFocusFileRaw(t, dir, 1, "---\nround: 1\nfocus: []\n")
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, "")
	})

	t.Run("NonPositiveRound", func(t *testing.T) {
		dir := t.TempDir()
		writeFocusFileRaw(t, dir, 1, "---\nround: 0\nexclude_lenses: []\nfocus: []\n---\n")
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, "")
	})

	t.Run("ScalarWhereAListIsRequired", func(t *testing.T) {
		dir := t.TempDir()
		writeFocusFileRaw(t, dir, 1, "---\nround: 1\nexclude_lenses: lensA\nfocus: []\n---\n")
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, "")
	})
}

// TestReadRoundFocus_FrontmatterRoundMustMatchItsOwnFilename checks the focus reader's round match.
// A focus file whose round: frontmatter field disagrees with the round its filename encodes is malformed.
// The reader degrades such a file to the zero directive; it never trusts that round.
func TestReadRoundFocus_FrontmatterRoundMustMatchItsOwnFilename(t *testing.T) {
	tiergate.Require(t, 0)
	dir := t.TempDir()
	// The file lands at round-3-focus.md, per focusPath(dir, 3), but its frontmatter claims round: 1.
	path := writeFocusFile(t, dir, 3, focusFile{Round: 1, ExcludeLenses: []string{"lensA"}, Focus: []string{"a directive"}})

	got := readRoundFocus("bouncer", dir, 3)
	assertFocus(t, got, []string{}, "")

	// The fixture must exist and parse on its own.
	// Then the empty result above can only come from the round check.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture file missing: %v", err)
	}
	if parsed, err := parseFocus(mustReadFile(t, path)); err != nil || parsed.Round != 1 {
		t.Fatalf("fixture file does not carry the mismatch it is meant to: parseFocus = (%+v, %v)", parsed, err)
	}
}

// mustReadFile reads path or fails the test.
// It serves fixture checks, which must not tolerate an I/O error.
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	return content
}
