// record_test.go covers the run's pull-request record file and the review verdict read from a pull request's reviews.

package pullrequest

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRecord_RoundTripAndUpdate(t *testing.T) {
	tiergate.Require(t, 0)
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

	want := Record{
		Number: 7, URL: "https://github.com/o/r/pull/7", Base: "main", Head: "task", GatedHead: "abc", StrandGUID: "g", StrandName: "n", SessionID: "s", Engine: "claude",
		SendBack: "/run/pr-findings-2.md", FixHereCount: 2, SendBacks: 2, CountedSets: []string{"aa", "bb"},
	}
	if err := WriteRecord(path, lockPath, want); err != nil {
		t.Fatalf("WriteRecord() error: %v", err)
	}
	got, found, err := ReadRecord(path, lockPath)
	if err != nil || !found || !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadRecord() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
	}

	// An autonomy recorded false is an explicit false, and a nil one writes no key.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record file: %v", err)
	}
	if strings.Contains(string(raw), "autonomy") {
		t.Errorf("record file = %s; want no autonomy key while Autonomy is nil", raw)
	}
	off := false
	recorded := want
	recorded.Autonomy = &off
	if err := WriteRecord(path, lockPath, recorded); err != nil {
		t.Fatalf("WriteRecord(autonomy false) error: %v", err)
	}
	got, _, err = ReadRecord(path, lockPath)
	if err != nil || got.Autonomy == nil || *got.Autonomy {
		t.Fatalf("ReadRecord() = %+v, %v; want Autonomy pointing at false", got, err)
	}
	if err := WriteRecord(path, lockPath, want); err != nil {
		t.Fatalf("WriteRecord() error: %v", err)
	}

	// A record from an older run, without the send-back keys, reads them as zero.
	if err := os.WriteFile(path, []byte(`{"number": 7, "parent_review_asked": true}`), 0o644); err != nil {
		t.Fatalf("write older record file: %v", err)
	}
	older, _, err := ReadRecord(path, lockPath)
	if wantOlder := (Record{Number: 7, ParentReviewAsked: true}); err != nil || !reflect.DeepEqual(older, wantOlder) {
		t.Fatalf("ReadRecord(older) = %+v, %v; want %+v with the send-back fields zero", older, err, wantOlder)
	}
	if err := WriteRecord(path, lockPath, want); err != nil {
		t.Fatalf("WriteRecord() error: %v", err)
	}

	// A key a newer binary wrote is kept across the rewrite.
	raw, err = os.ReadFile(path)
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

// TestClearReviewSession pins that the clear drops the autonomy, the parent-review ask mark and the session fields.
// It keeps every other field.
// A missing record returns nil and creates no file.
func TestClearReviewSession(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pull-request.json")
	lockPath := path + ".lock"

	if err := ClearReviewSession(path, lockPath); err != nil {
		t.Errorf("ClearReviewSession(absent) error = %v; want nil", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("ClearReviewSession(absent) left a file at %s (stat error %v); want none", path, err)
	}

	on := true
	held := Record{
		Number: 7, Autonomy: &on, ParentReviewAsked: true, StrandGUID: "g", StrandName: "n", SessionID: "s", Engine: "claude",
		GatedHead: "abc", Continue: true, SendBacks: 2,
	}
	if err := WriteRecord(path, lockPath, held); err != nil {
		t.Fatalf("WriteRecord() error: %v", err)
	}

	if err := ClearReviewSession(path, lockPath); err != nil {
		t.Fatalf("ClearReviewSession() error = %v; want nil", err)
	}

	got, _, err := ReadRecord(path, lockPath)
	want := Record{Number: 7, GatedHead: "abc", Continue: true, SendBacks: 2}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ReadRecord() = %+v, %v; want %+v", got, err, want)
	}
}

func TestStandingReviews(t *testing.T) {
	tiergate.Require(t, 0)
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
