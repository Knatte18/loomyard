//go:build tier0

// landrequest_test.go covers the land request's file record.

package pawlengine_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pawlengine"
)

// TestLandRequest asserts a request reads back what was written, keeps a key it does not declare across a rewrite, reads an absent file as none and removes cleanly.
func TestLandRequest(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	requestedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	if _, found, err := pawlengine.ReadLandRequest(w.paths); err != nil || found {
		t.Fatalf("ReadLandRequest() on an absent file = found %t, error %v; want none", found, err)
	}
	if err := pawlengine.RemoveLandRequest(w.paths); err != nil {
		t.Fatalf("RemoveLandRequest() on an absent file = %v; want nil", err)
	}

	if err := pawlengine.WriteLandRequest(w.paths, pawlengine.LandRequest{RunID: "run-1", RequestedAt: requestedAt}); err != nil {
		t.Fatal(err)
	}
	got, found, err := pawlengine.ReadLandRequest(w.paths)
	if err != nil || !found {
		t.Fatalf("ReadLandRequest() = found %t, error %v; want the request", found, err)
	}
	if got.RunID != "run-1" || !got.RequestedAt.Equal(requestedAt) {
		t.Errorf("ReadLandRequest() = %+v; want run-1 at %v", got, requestedAt)
	}

	raw, err := os.ReadFile(w.paths.LandRequest)
	if err != nil {
		t.Fatal(err)
	}
	withKey := strings.Replace(string(raw), "{", `{"written_by_a_later_binary": "kept",`, 1)
	if err := os.WriteFile(w.paths.LandRequest, []byte(withKey), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pawlengine.WriteLandRequest(w.paths, pawlengine.LandRequest{RunID: "run-2", RequestedAt: requestedAt}); err != nil {
		t.Fatal(err)
	}
	rewritten, err := os.ReadFile(w.paths.LandRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rewritten), "written_by_a_later_binary") || !strings.Contains(string(rewritten), "run-2") {
		t.Errorf("rewritten request = %s; want run-2 and the key it does not declare", rewritten)
	}

	if err := pawlengine.RemoveLandRequest(w.paths); err != nil {
		t.Fatal(err)
	}
	if _, found, err := pawlengine.ReadLandRequest(w.paths); err != nil || found {
		t.Errorf("ReadLandRequest() after removal = found %t, error %v; want none", found, err)
	}
}
