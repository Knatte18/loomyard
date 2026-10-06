package loomcli

import (
	"os"
	"path/filepath"
	"strconv"
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

// TestReviewWaiting asserts the waiting note names an open request and whether its notice was delivered, and is empty when there is no review, a settled round or a superseded one.
func TestReviewWaiting(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, s parentreview.Store)
		wantEmpty  bool
		wantPrefix string
		wantIn     string
	}{
		{name: "absent dir is empty", wantEmpty: true},
		{
			name:       "open undelivered",
			setup:      openReview,
			wantPrefix: "parent review: waiting on reviewer hub:orch",
			wantIn:     "notice not delivered",
		},
		{
			name: "open delivered",
			setup: func(t *testing.T, s parentreview.Store) {
				openReview(t, s)
				if err := s.RecordDelivered(""); err != nil {
					t.Fatal(err)
				}
			},
			wantIn: "notice delivered at",
		},
		{
			name: "verdicted round reports nothing",
			setup: func(t *testing.T, s parentreview.Store) {
				openReview(t, s)
				if err := s.RecordVerdict(parentreview.VerdictApprove, ""); err != nil {
					t.Fatal(err)
				}
			},
			wantEmpty: true,
		},
		{
			name: "expired round reports nothing",
			setup: func(t *testing.T, s parentreview.Store) {
				openReview(t, s)
				if err := s.MarkExpired(); err != nil {
					t.Fatal(err)
				}
			},
			wantEmpty: true,
		},
		{
			// Only the latest round counts: the superseded round's open request must not leak.
			name: "superseded round reports nothing",
			setup: func(t *testing.T, s parentreview.Store) {
				openReview(t, s)
				if _, err := s.BeginRound(); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(s.Root, "round-2")); err != nil {
					t.Fatal(err)
				}
			},
			wantEmpty: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := reviewStore(t)
			if tt.setup != nil {
				tt.setup(t, s)
			}
			note := waitingNote(t, s)
			if tt.wantEmpty && note != "" {
				t.Errorf("note = %q, want empty", note)
			}
			if !strings.HasPrefix(note, tt.wantPrefix) || !strings.Contains(note, tt.wantIn) {
				t.Errorf("note = %q; want prefix %q containing %q", note, tt.wantPrefix, tt.wantIn)
			}
		})
	}
}

// writeVerifyMarker writes a running marker held by pid and returns its path.
func writeVerifyMarker(t *testing.T, pid int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "running.yaml")
	body := "site: Publish\nattempt: 2\nstarted: 2026-10-03T09:15:00Z\npid: " + strconv.Itoa(pid) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestVerifyWaiting asserts a live verify marker is reported ahead of an open review, a dead marker falls through to the review note, and no marker with no review is empty.
func TestVerifyWaiting(t *testing.T) {
	tests := []struct {
		name       string
		openReview bool
		marker     func(t *testing.T) string
		check      func(t *testing.T, note string)
	}{
		{
			name:       "live marker ahead of review",
			openReview: true,
			marker:     func(t *testing.T) string { return writeVerifyMarker(t, os.Getpid()) },
			check: func(t *testing.T, note string) {
				if !strings.HasPrefix(note, "verify Publish (attempt 2, since ") || strings.Contains(note, "parent review") {
					t.Errorf("note = %q", note)
				}
			},
		},
		{
			name:       "dead marker falls to review",
			openReview: true,
			marker:     func(t *testing.T) string { return writeVerifyMarker(t, 2147483646) },
			check: func(t *testing.T, note string) {
				if !strings.HasPrefix(note, "parent review: ") {
					t.Errorf("note = %q", note)
				}
			},
		},
		{
			name:   "no marker no review is empty",
			marker: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.yaml") },
			check: func(t *testing.T, note string) {
				if note != "" {
					t.Errorf("note = %q, want empty", note)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := reviewStore(t)
			if tt.openReview {
				openReview(t, s)
			}
			note, err := verifyWaiting(tt.marker(t), reviewWaiting(s.Root, s.LockDir))()
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, note)
		})
	}
}

//testtiming:keep pins the status spec carrying no waiting hook when the CLI has no location; its covering tests build the spec without asserting its hooks
func TestSpecFor_WaitingHookNilWithoutLocation(t *testing.T) {
	c := &loomCLI{}
	if got := c.specFor("status").Hooks.Waiting; got != nil {
		t.Errorf("Waiting set with no location")
	}
	var _ shedverbs.Spec = c.specFor("status")
}
