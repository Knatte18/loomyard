//go:build tier0

// record_test.go covers the run's pull-request record file and the review verdict read from a pull request's reviews.

package pullrequest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"
)

func TestRecord_RoundTripAndUpdate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pull-request.json")
	lockPath := path + ".lock"

	if _, found, err := ReadRecord(path, lockPath); err != nil || found {
		t.Fatalf("ReadRecord(absent) = found %v, err %v; want not found, nil", found, err)
	}
	if err := UpdateRecord(path, lockPath, func(rec *Record) error { rec.Continue = true; return nil }); !errors.Is(err, ErrNoRecord) {
		t.Errorf("UpdateRecord(absent) error = %v; want ErrNoRecord", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("UpdateRecord(absent) left a file at %s (stat error %v); want none", path, err)
	}

	want := Record{Number: 7, URL: "https://github.com/o/r/pull/7", Base: "main", Head: "task", GatedHead: "abc", StrandGUID: "g", StrandName: "n", SessionID: "s", Engine: "claude"}
	if err := WriteRecord(path, lockPath, want); err != nil {
		t.Fatalf("WriteRecord() error: %v", err)
	}
	got, found, err := ReadRecord(path, lockPath)
	if err != nil || !found || got != want {
		t.Fatalf("ReadRecord() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
	}

	// A key a newer binary wrote is kept across the rewrite.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record file: %v", err)
	}
	withUnknown := strings.Replace(string(raw), "{", `{"future_key": "kept",`, 1)
	if err := os.WriteFile(path, []byte(withUnknown), 0o644); err != nil {
		t.Fatalf("write record file: %v", err)
	}
	if err := UpdateRecord(path, lockPath, func(rec *Record) error { rec.Continue = true; return nil }); err != nil {
		t.Fatalf("UpdateRecord() error: %v", err)
	}
	updated, _, err := ReadRecord(path, lockPath)
	if err != nil || !updated.Continue || updated.Number != want.Number {
		t.Errorf("ReadRecord() after UpdateRecord = %+v, %v; want Continue set and Number %d", updated, err, want.Number)
	}
	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record file: %v", err)
	}
	if !strings.Contains(string(rewritten), `"future_key": "kept"`) {
		t.Errorf("record file after UpdateRecord = %s; want the unknown key kept", rewritten)
	}

	for i := 0; i < 2; i++ {
		if err := RemoveRecord(path, lockPath); err != nil {
			t.Fatalf("RemoveRecord() call %d error: %v", i+1, err)
		}
	}
	if _, found, _ := ReadRecord(path, lockPath); found {
		t.Errorf("ReadRecord() after RemoveRecord found a record; want none")
	}
}

func TestStandingReviews(t *testing.T) {
	t.Parallel()
	review := func(login, state, commit string) *github.PullRequestReview {
		return &github.PullRequestReview{User: &github.User{Login: &login}, State: &state, CommitID: &commit}
	}
	tests := []struct {
		name                 string
		reviews              []*github.PullRequestReview
		wantApproved         bool
		wantChangesRequested bool
	}{
		{"None", nil, false, false},
		{"ApprovalOfHead", []*github.PullRequestReview{review("a", "APPROVED", "h2")}, true, false},
		{"LaterCommentKeepsApproval", []*github.PullRequestReview{review("a", "APPROVED", "h2"), review("a", "COMMENTED", "h2")}, true, false},
		{"ApprovalOfOlderHeadIsNone", []*github.PullRequestReview{review("a", "APPROVED", "h1")}, false, false},
		{"DismissedClearsChangeRequest", []*github.PullRequestReview{review("a", "CHANGES_REQUESTED", "h1"), review("a", "DISMISSED", "h1")}, false, false},
		{"ChangeRequestBesideApproval", []*github.PullRequestReview{review("a", "APPROVED", "h2"), review("b", "CHANGES_REQUESTED", "h2")}, true, true},
		{"LaterApprovalReplacesChangeRequest", []*github.PullRequestReview{review("a", "CHANGES_REQUESTED", "h1"), review("a", "APPROVED", "h2")}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			approved, changesRequested := standingReviews(tt.reviews, "h2")
			if approved != tt.wantApproved || changesRequested != tt.wantChangesRequested {
				t.Errorf("standingReviews() = approved %v, changesRequested %v; want %v, %v", approved, changesRequested, tt.wantApproved, tt.wantChangesRequested)
			}
		})
	}
}
