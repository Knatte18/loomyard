// amendment_test.go covers AppendAmendment: a first append creates the file with its heading and every field;
// a second append leaves the first entry byte-identical and adds the second below it;
// and an unreadable amendments file yields a wrapped error.

package planparser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAppendAmendment_AppendsEntries asserts a first append creates the file with its fixed heading and every field,
// and a second append leaves the first entry byte-identical and adds the second below it.
func TestAppendAmendment_AppendsEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, planparser.AmendmentsFileName)
	first := planparser.Amendment{
		Timestamp: "2026-09-06T10:00:00Z",
		Card:      "1-only",
		OldGlyph:  "plan:internal/foo#NewThing",
		NewGlyph:  "internal/foo#NewThing",
		Tier:      "syntactic",
		SHA:       "abc123",
	}
	second := planparser.Amendment{Timestamp: "t2", Card: "2-b", OldGlyph: "o2", NewGlyph: "n2", Tier: "resolve", SHA: "sha2"}

	if err := planparser.AppendAmendment(dir, first); err != nil {
		t.Fatalf("AppendAmendment() first call error = %v; want nil", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read amendments file: %v", err)
	}
	afterFirst := string(data)
	if !strings.HasPrefix(afterFirst, "# Amendments\n") {
		t.Errorf("amendments file = %q; want it to start with the fixed heading", afterFirst)
	}
	for _, field := range []string{first.Timestamp, first.Card, first.OldGlyph, first.NewGlyph, first.Tier, first.SHA} {
		if !strings.Contains(afterFirst, field) {
			t.Errorf("amendments file = %q; want it to carry field %q", afterFirst, field)
		}
	}

	if err := planparser.AppendAmendment(dir, second); err != nil {
		t.Fatalf("AppendAmendment() second call error = %v; want nil", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second append: %v", err)
	}
	afterSecond := string(data)
	if !strings.HasPrefix(afterSecond, afterFirst) {
		t.Errorf("after second append = %q; want it to start with the first append's byte-identical content %q", afterSecond, afterFirst)
	}
	if !strings.Contains(afterSecond, "sha2") {
		t.Errorf("after second append = %q; want the second entry appended below the first", afterSecond)
	}
}

func TestAppendAmendment_UnreadableAmendmentsFileIsWrappedError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	// A directory named amendments.md cannot be read as a file, so os.ReadFile fails with an error other than not-exist.
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
