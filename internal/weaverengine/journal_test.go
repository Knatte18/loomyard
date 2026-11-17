// journal_test.go covers the run journal's idempotent append and the per-strand file names.

package weaverengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

func TestAppendJournal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	read := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	first := weaverengine.JournalHeading("01-one", 1)
	second := weaverengine.JournalHeading("01-one", 2)

	t.Run("two entries append in order", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "run", "journal.md")
		if err := weaverengine.AppendJournal(path, first, "first text\n"); err != nil {
			t.Fatal(err)
		}
		if err := weaverengine.AppendJournal(path, second, "second text"); err != nil {
			t.Fatal(err)
		}
		want := "## Batch 01-one, attempt 1\n\nfirst text\n\n## Batch 01-one, attempt 2\n\nsecond text\n"
		if got := read(t, path); got != want {
			t.Errorf("journal = %q; want %q", got, want)
		}
	})

	t.Run("a repeated heading adds nothing", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "journal.md")
		if err := weaverengine.AppendJournal(path, first, "text"); err != nil {
			t.Fatal(err)
		}
		before := read(t, path)
		if err := weaverengine.AppendJournal(path, first, "other text"); err != nil {
			t.Fatal(err)
		}
		if got := read(t, path); got != before {
			t.Errorf("journal after a repeated heading = %q; want it unchanged %q", got, before)
		}
	})

	t.Run("an empty text appends the heading alone", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "journal.md")
		if err := weaverengine.AppendJournal(path, first, ""); err != nil {
			t.Fatal(err)
		}
		if got, want := read(t, path), "## Batch 01-one, attempt 1\n"; got != want {
			t.Errorf("journal = %q; want %q", got, want)
		}
	})

	t.Run("the strand files are named by batch, attempt and fixer", func(t *testing.T) {
		t.Parallel()
		reports := filepath.Join("reports")
		if got, want := weaverengine.AttemptReportPath(reports, 3, "slug", 2), filepath.Join(reports, "03-slug-a2.yaml"); got != want {
			t.Errorf("AttemptReportPath() = %q; want %q", got, want)
		}
		if got, want := weaverengine.FixerReplyPath(reports, 4), filepath.Join(reports, "fix-4.md"); got != want {
			t.Errorf("FixerReplyPath() = %q; want %q", got, want)
		}
	})
}
