// amendment_test.go covers AppendAmendment: a first append creating the file with its heading; a
// second append leaving the first entry byte-identical and adding the second below it; every
// field rendered; and a wrapped error convention matching this package's other write paths.

package planparser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestAppendAmendment_FirstAppendCreatesFileWithHeading(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := planparser.Amendment{
		Timestamp: "2026-09-06T10:00:00Z",
		Card:      "1-only",
		OldGlyph:  "plan:internal/foo#NewThing",
		NewGlyph:  "internal/foo#NewThing",
		Tier:      "syntactic",
		SHA:       "abc123",
	}
	if err := planparser.AppendAmendment(dir, a); err != nil {
		t.Fatalf("AppendAmendment() error = %v; want nil", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, planparser.AmendmentsFileName))
	if err != nil {
		t.Fatalf("read amendments file: %v", err)
	}
	got := string(data)

	if !strings.HasPrefix(got, "# Amendments\n") {
		t.Errorf("amendments file = %q; want it to start with the fixed heading", got)
	}
	for _, field := range []string{a.Timestamp, a.Card, a.OldGlyph, a.NewGlyph, a.Tier, a.SHA} {
		if !strings.Contains(got, field) {
			t.Errorf("amendments file = %q; want it to carry field %q", got, field)
		}
	}
}

func TestAppendAmendment_SecondAppendLeavesFirstEntryByteIdentical(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := planparser.Amendment{Timestamp: "t1", Card: "1-a", OldGlyph: "o1", NewGlyph: "n1", Tier: "syntactic", SHA: "sha1"}
	second := planparser.Amendment{Timestamp: "t2", Card: "2-b", OldGlyph: "o2", NewGlyph: "n2", Tier: "resolve", SHA: "sha2"}

	if err := planparser.AppendAmendment(dir, first); err != nil {
		t.Fatalf("AppendAmendment() first call error = %v; want nil", err)
	}
	afterFirst, err := os.ReadFile(filepath.Join(dir, planparser.AmendmentsFileName))
	if err != nil {
		t.Fatalf("read after first append: %v", err)
	}

	if err := planparser.AppendAmendment(dir, second); err != nil {
		t.Fatalf("AppendAmendment() second call error = %v; want nil", err)
	}
	afterSecond, err := os.ReadFile(filepath.Join(dir, planparser.AmendmentsFileName))
	if err != nil {
		t.Fatalf("read after second append: %v", err)
	}

	if !strings.HasPrefix(string(afterSecond), string(afterFirst)) {
		t.Errorf("after second append = %q; want it to start with the first append's byte-identical content %q", afterSecond, afterFirst)
	}
	if !strings.Contains(string(afterSecond), "sha2") {
		t.Errorf("after second append = %q; want the second entry appended below the first", afterSecond)
	}
}

func TestAppendAmendment_UnreadableAmendmentsFileIsWrappedError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// A directory named amendments.md is unreadable as a file, forcing os.ReadFile to fail with a
	// non-not-exist error.
	if err := os.Mkdir(filepath.Join(dir, planparser.AmendmentsFileName), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err := planparser.AppendAmendment(dir, planparser.Amendment{})
	if err == nil {
		t.Fatal("AppendAmendment() error = nil; want a wrapped error")
	}
	if !strings.HasPrefix(err.Error(), "planparser:") {
		t.Errorf("AppendAmendment() error = %q; want \"planparser:\" prefix", err.Error())
	}
}
