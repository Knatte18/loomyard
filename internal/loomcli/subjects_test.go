//go:build tier0

// subjects_test.go drives the document and merge-in subjects and the review input seams over plain directories, with no git.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// subjectLocation returns a location whose anchor is a fresh temporary directory.
func subjectLocation(t *testing.T) *lyxcwd.Location {
	t.Helper()
	anchor := filepath.Join(t.TempDir(), "pair")
	if err := os.MkdirAll(anchor, 0o755); err != nil {
		t.Fatal(err)
	}
	return hubLocationAt(t, anchor, "")
}

// writeSubjectFile writes body to path, creating its directory.
func writeSubjectFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// documentSubjectOf reads the document subject of paths and returns its hash, failing the test on an error or a not-found answer.
func documentSubjectOf(t *testing.T, location *lyxcwd.Location, paths ...string) string {
	t.Helper()
	subject, found, err := documentSubject(location, paths)
	if err != nil || !found || subject.Kind != shedadapters.SubjectDocument {
		t.Fatalf("documentSubject(%v) = %+v, %v, %v; want a document subject, found, nil", paths, subject, found, err)
	}
	return subject.Hash
}

// TestDocumentSubject covers what documentSubject adds over the identity text it hashes: the decision record's carry-over blocks and a plan directory's approval flag, archived generations and lens review never change the subject, and any other edit does.
func TestDocumentSubject(t *testing.T) {
	t.Parallel()

	t.Run("a carry-over block leaves the decision record's subject unchanged", func(t *testing.T) {
		t.Parallel()
		location := subjectLocation(t)
		record := loomengine.DiscussionDecisionRecord(location)
		writeSubjectFile(t, record, "# Record\n\n## Goal\n\ng.\n\n## Decisions\n\nd.\n\n## Open risks\n\nnone yet.\n\n## Acceptance criteria\n\na.\n")
		before := documentSubjectOf(t, location, record)
		entry := discussionparser.CarryOver{
			Segment: "Plan-Review", Round: 2, Closing: discussionparser.CarryOverConverged,
			ReviewPath: "reviews/plan/round-2-review.md", FixerReportPath: "reviews/plan/round-2-fixer-report.md",
			Findings: []discussionparser.CarryOverFinding{{Key: "cache-key", Class: "design", Severity: "MEDIUM", Rounds: []int{1, 2}}},
		}
		if err := discussionparser.WriteCarryOver(record, entry); err != nil {
			t.Fatalf("WriteCarryOver() = %v; want nil", err)
		}
		if after := documentSubjectOf(t, location, record); after != before {
			t.Errorf("subject after a carry-over block = %+v; want the unchanged %+v", after, before)
		}
		if got, _ := os.ReadFile(record); !strings.Contains(string(got), "lyx:carry-over") {
			t.Fatalf("decision record holds no carry-over block after WriteCarryOver:\n%s", got)
		}
		writeSubjectFile(t, record, "# Record, edited\n")
		if edited := documentSubjectOf(t, location, record); edited == before {
			t.Error("subject after an edit of the record = the earlier subject; want a changed one")
		}
	})

	t.Run("a plan directory's subject ignores approval, archived generations and the lens review and follows a card edit", func(t *testing.T) {
		t.Parallel()
		location := subjectLocation(t)
		planDir := planparser.PlanDir(location.AnchorPath())
		plankit.Write(t, planDir, plankit.Plan{
			Language: "go",
			Cards: []plankit.Card{{
				Number: 1, Slug: "first", Summary: "the first card",
				Groups: []plankit.Group{{Label: "Create", Targets: []string{"internal/x#X"}}},
				Intent: "the first card.", Commit: "1: first",
			}},
		})
		before := documentSubjectOf(t, location, planDir)

		if err := planparser.SetApproved(planDir); err != nil {
			t.Fatalf("SetApproved() = %v; want nil", err)
		}
		writeSubjectFile(t, filepath.Join(planDir, "archive-1", "01-first.md"), "an older generation\n")
		writeSubjectFile(t, filepath.Join(planDir, "lens-review.md"), "lens notes\n")
		if after := documentSubjectOf(t, location, planDir); after != before {
			t.Errorf("subject after approval, an archive and a lens review = %+v; want the unchanged %+v", after, before)
		}

		card := filepath.Join(planDir, "01-first.md")
		body, err := os.ReadFile(card)
		if err != nil {
			t.Fatal(err)
		}
		writeSubjectFile(t, card, string(body)+"\nan edit\n")
		if edited := documentSubjectOf(t, location, planDir); edited == before {
			t.Error("subject after a card edit = the earlier subject; want a changed one")
		}
	})

	t.Run("an absent path gives a subject and creating it changes the subject", func(t *testing.T) {
		t.Parallel()
		location := subjectLocation(t)
		path := filepath.Join(location.AnchorPath(), "_lyx", "discussion", "support-log.md")
		absent := documentSubjectOf(t, location, path)
		writeSubjectFile(t, path, "log\n")
		if created := documentSubjectOf(t, location, path); created == absent {
			t.Error("subject after creating the path = the absent subject; want a changed one")
		}
	})
}

// TestReviewInputSeams asserts each review input seam answers not found while its file is absent under the scratch directory, and the file's path with its modification time once it is there.
func TestReviewInputSeams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		seam func(scratchDir string) func() (string, time.Time, bool, error)
		path func(scratchDir string) string
	}{
		{"FixerReport", fixerReportInput, func(scratchDir string) string {
			return filepath.Join(scratchDir, loomshed.NameParentAlign+"-fixer-report.md")
		}},
		{"FixerReportUnderRetiredName", fixerReportInput, func(scratchDir string) string {
			return filepath.Join(scratchDir, "Merge-In-fixer-report.md")
		}},
		{"OverlapNote", overlapNoteInput, landingshed.OverlapNotePath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scratchDir := t.TempDir()
			seam := tt.seam(scratchDir)
			if path, _, found, err := seam(); err != nil || found {
				t.Fatalf("seam() with no file = %q, %v, %v; want not found, nil", path, found, err)
			}

			want := tt.path(scratchDir)
			if err := os.WriteFile(want, []byte("input\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) = %v; want nil", want, err)
			}
			writtenAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
			if err := os.Chtimes(want, writtenAt, writtenAt); err != nil {
				t.Fatalf("Chtimes(%q) = %v; want nil", want, err)
			}
			path, gotAt, found, err := seam()
			if err != nil || !found || path != want || !gotAt.Equal(writtenAt) {
				t.Errorf("seam() = %q, %v, %v, %v; want %q, %v, found, nil", path, gotAt, found, err, want, writtenAt)
			}
		})
	}

	t.Run("FixerReportBothNames_NewWins", func(t *testing.T) {
		t.Parallel()
		scratchDir := t.TempDir()
		current := filepath.Join(scratchDir, loomshed.NameParentAlign+"-fixer-report.md")
		for _, path := range []string{current, filepath.Join(scratchDir, "Merge-In-fixer-report.md")} {
			if err := os.WriteFile(path, []byte("input\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
			}
		}
		if path, _, found, err := fixerReportInput(scratchDir)(); err != nil || !found || path != current {
			t.Errorf("fixerReportInput() = %q, %v, %v; want %q, found, nil", path, found, err, current)
		}
	})
}
