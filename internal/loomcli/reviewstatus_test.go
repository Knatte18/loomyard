package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

func reviewStore(t *testing.T) parentreview.Store {
	t.Helper()
	d := t.TempDir()
	return parentreview.Store{Root: filepath.Join(d, "reviews"), LockDir: filepath.Join(d, "locks")}
}

func openReview(t *testing.T, s parentreview.Store) {
	t.Helper()
	if _, err := s.BeginRound(); err != nil {
		t.Fatal(err)
	}
	spec := parentreview.OpenSpec{Slug: "x", Reviewer: "hub:orch", DecisionRecord: "d.md", SupportLog: "s.md", Brief: "brief"}
	if _, err := s.OpenRequest(spec); err != nil {
		t.Fatal(err)
	}
}

func waitingNote(t *testing.T, s parentreview.Store) string {
	t.Helper()
	note, err := reviewWaiting(s.Root, s.LockDir)()
	if err != nil {
		t.Fatal(err)
	}
	return note
}

func TestReviewWaiting_AbsentDirIsEmpty(t *testing.T) {
	if note := waitingNote(t, reviewStore(t)); note != "" {
		t.Errorf("note = %q, want empty", note)
	}
}

func TestReviewWaiting_OpenUndelivered(t *testing.T) {
	s := reviewStore(t)
	openReview(t, s)
	note := waitingNote(t, s)
	if !strings.HasPrefix(note, "parent review: waiting on reviewer hub:orch") || !strings.Contains(note, "notice not delivered") {
		t.Errorf("note = %q", note)
	}
}

func TestReviewWaiting_OpenDelivered(t *testing.T) {
	s := reviewStore(t)
	openReview(t, s)
	if err := s.RecordDelivered(""); err != nil {
		t.Fatal(err)
	}
	if note := waitingNote(t, s); !strings.Contains(note, "notice delivered at") {
		t.Errorf("note = %q", note)
	}
}

func TestReviewWaiting_SettledRoundsReportNothing(t *testing.T) {
	t.Run("verdicted", func(t *testing.T) {
		s := reviewStore(t)
		openReview(t, s)
		if err := s.RecordVerdict(parentreview.VerdictApprove, ""); err != nil {
			t.Fatal(err)
		}
		if note := waitingNote(t, s); note != "" {
			t.Errorf("note = %q, want empty", note)
		}
	})
	t.Run("expired", func(t *testing.T) {
		s := reviewStore(t)
		openReview(t, s)
		if err := s.MarkExpired(); err != nil {
			t.Fatal(err)
		}
		if note := waitingNote(t, s); note != "" {
			t.Errorf("note = %q, want empty", note)
		}
	})
	t.Run("superseded", func(t *testing.T) {
		s := reviewStore(t)
		openReview(t, s)
		if _, err := s.BeginRound(); err != nil {
			t.Fatal(err)
		}
		if note := waitingNote(t, s); note != "" {
			t.Errorf("note = %q, want empty", note)
		}
	})
}

func TestReviewWaiting_LatestRoundOnly(t *testing.T) {
	s := reviewStore(t)
	openReview(t, s)
	if _, err := s.BeginRound(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "round-2")); err != nil {
		t.Fatal(err)
	}
	if note := waitingNote(t, s); note != "" {
		t.Errorf("superseded round leaked: %q", note)
	}
}

func TestSpecFor_WaitingHookNilWithoutLocation(t *testing.T) {
	c := &loomCLI{}
	if got := c.specFor("status").Hooks.Waiting; got != nil {
		t.Errorf("Waiting set with no location")
	}
	var _ shedverbs.Spec = c.specFor("status")
}
