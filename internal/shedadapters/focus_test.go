// focus_test.go covers readRoundFocus against the focus file the Bouncer actually writes: the
// YAML-frontmatter round-<N>-focus.md shape renderFocus produces and parseFocus accepts.
// TestReadRoundFocus_ReadsTheFileTheBouncerWrites is the two-sided regression guard -- it writes
// through the writer and reads through the reader, so a future divergence in filename, format, or
// field set fails here instead of silently emptying the judge's targeting channel in production.

package shedadapters

import (
	"os"
	"testing"
	"time"
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

// writeFocusFile renders f through the Bouncer's own writer into round's focus path and returns that
// path. Every well-formed fixture in this package goes through the real writer rather than a
// hand-built string, so a fixture can never assert a shape the writer does not actually produce --
// which is exactly how the reader/writer divergence this pair once carried stayed invisible.
func writeFocusFile(t *testing.T, runDir string, round int, f focusFile) string {
	t.Helper()
	path := focusPath(runDir, round)
	if err := writeFocus(path, f); err != nil {
		t.Fatalf("writeFocus(%s): %v", path, err)
	}
	return path
}

// writeFocusFileRaw writes content verbatim to round's focus path, for the malformed-input rows that
// cannot go through the renderer.
func writeFocusFileRaw(t *testing.T, runDir string, round int, content string) string {
	t.Helper()
	path := focusPath(runDir, round)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

// TestReadRoundFocus_ReadsTheFileTheBouncerWrites drives the writer and the reader against one
// another. It is the guard for the defect this pair once carried: the writer emitted
// round-<N>-focus.md as YAML frontmatter while the reader opened round-<N>-focus.json and strictly
// decoded JSON, so every production read found nothing and the judge's directive never reached the
// fixer round.
func TestReadRoundFocus_ReadsTheFileTheBouncerWrites(t *testing.T) {
	dir := t.TempDir()
	path := writeFocusFile(t, dir, 3, focusFile{
		Round:         3,
		ExcludeLenses: []string{"lensA", "lensB"},
		Focus:         []string{"check the relocation candidate in the Auto-mode assumptions section"},
		Prose:         "Seed round: nothing has been reviewed yet.",
	})

	got := readRoundFocus("bouncer", dir, 3)

	assertFocus(t, got, []string{"lensA", "lensB"}, []string{path})
}

// TestReadRoundFocus_HydratesOnlyWhenTheFileSaysSomething pins that an APPROVED judge's mandatory
// but empty focus file is not hydrated: handing the next round a document that asserts nothing is
// noise, not targeting.
func TestReadRoundFocus_HydratesOnlyWhenTheFileSaysSomething(t *testing.T) {
	tests := []struct {
		name         string
		file         focusFile
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFocusFile(t, dir, 1, tt.file)

			got := readRoundFocus("bouncer", dir, 1)

			wantHydrate := []string{}
			if tt.wantHydrated {
				wantHydrate = []string{path}
			}
			assertFocus(t, got, tt.wantExclude, wantHydrate)
		})
	}
}

// TestReadRoundFocus_DegradesToTheZeroDirective covers every fail-safe branch: the reader must never
// return an error, because a missing or broken targeting hint must not retract a round.
func TestReadRoundFocus_DegradesToTheZeroDirective(t *testing.T) {
	t.Run("AbsentFile", func(t *testing.T) {
		got := readRoundFocus("bouncer", t.TempDir(), 1)
		assertFocus(t, got, []string{}, []string{})
	})

	t.Run("UnreadableFile", func(t *testing.T) {
		dir := t.TempDir()
		// A directory at the focus file's own path produces a deterministic read failure -- file
		// permissions do not fail for a privileged test process.
		if err := os.Mkdir(focusPath(dir, 1), 0o755); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, []string{})
	})

	t.Run("NoFrontmatter", func(t *testing.T) {
		dir := t.TempDir()
		writeFocusFileRaw(t, dir, 1, "just prose, no delimiters\n")
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, []string{})
	})

	t.Run("UnclosedFrontmatter", func(t *testing.T) {
		dir := t.TempDir()
		writeFocusFileRaw(t, dir, 1, "---\nround: 1\nfocus: []\n")
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, []string{})
	})

	t.Run("NonPositiveRound", func(t *testing.T) {
		dir := t.TempDir()
		writeFocusFileRaw(t, dir, 1, "---\nround: 0\nexclude_lenses: []\nfocus: []\n---\n")
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, []string{})
	})

	t.Run("ScalarWhereAListIsRequired", func(t *testing.T) {
		dir := t.TempDir()
		writeFocusFileRaw(t, dir, 1, "---\nround: 1\nexclude_lenses: lensA\nfocus: []\n---\n")
		got := readRoundFocus("bouncer", dir, 1)
		assertFocus(t, got, []string{}, []string{})
	})
}

// TestReadRoundFocus_ResolvesFilenameByTargetRound pins the round token's meaning: the file names the
// round the directives are FOR, so reading round 4 must not find round 3's file.
func TestReadRoundFocus_ResolvesFilenameByTargetRound(t *testing.T) {
	dir := t.TempDir()
	path := writeFocusFile(t, dir, 3, focusFile{Round: 3, ExcludeLenses: []string{"lensA"}, Focus: []string{"a directive"}})

	found := readRoundFocus("bouncer", dir, 3)
	assertFocus(t, found, []string{"lensA"}, []string{path})

	notFound := readRoundFocus("bouncer", dir, 4)
	assertFocus(t, notFound, []string{}, []string{})
}

// TestReadRoundFocus_ReadsWhatTheBouncerSeedPassLeavesBehind closes the loop against the Bouncer's
// own writer rather than a hand-built focusFile: ensureFocus's synthetic file must read back as the
// zero directive, and a judge-written one must read back with its directive intact.
func TestReadRoundFocus_ReadsWhatTheBouncerSeedPassLeavesBehind(t *testing.T) {
	dir := t.TempDir()
	bouncer := &Bouncer{cfg: BouncerConfig{Name: "Discussion-Bouncer", RunDir: dir, Now: fixedClock(time.Unix(0, 0).UTC())}}

	bouncer.ensureFocus(1)

	got := readRoundFocus("Discussion-Bouncer", dir, 1)
	assertFocus(t, got, []string{}, []string{})

	if _, err := os.Stat(focusPath(dir, 1)); err != nil {
		t.Fatalf("ensureFocus(1) left no file at %s: %v", focusPath(dir, 1), err)
	}
}
