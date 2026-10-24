//go:build tier1

// failedpushstatus_integration_test.go covers `lyx fabric status` printing the hub's outstanding failed-push records over a hubforge hub.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestStatus_PrintsOutstandingFailedPushes(t *testing.T) {
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()

	code, out := runFabric(t, prime, "status")
	if code != 0 {
		t.Fatalf("status = %d; want 0\noutput: %s", code, out)
	}
	none := envelope.RequireOK(t, out)
	if rows, isArray := none.Raw["failed_pushes"].([]any); !isArray || len(rows) != 0 {
		t.Errorf("failed_pushes with no record = %v; want an empty array", none.Raw["failed_pushes"])
	}

	record := fabricengine.FailedPush{
		Repo:     "code",
		RepoPath: prime,
		Branch:   "feature/x",
		Class:    fabricengine.PushFailureAuth,
		Error:    "permission denied",
	}
	if err := fabricengine.RecordPushFailure(h.Location.HubPath, record, time.Now()); err != nil {
		t.Fatalf("RecordPushFailure: %v", err)
	}

	code, out = runFabric(t, prime, "status")
	if code != 0 {
		t.Fatalf("status = %d; want 0\noutput: %s", code, out)
	}
	withRecord := envelope.RequireOK(t, out)
	rows, _ := withRecord.Raw["failed_pushes"].([]any)
	if len(rows) != 1 {
		t.Fatalf("failed_pushes = %v; want the one record", withRecord.Raw["failed_pushes"])
	}
	row, _ := rows[0].(map[string]any)
	for key, want := range map[string]any{"repo": "code", "branch": "feature/x", "class": "auth", "error": "permission denied", "count": float64(1)} {
		if row[key] != want {
			t.Errorf("failed_pushes row %s = %v; want %v", key, row[key], want)
		}
	}
	if wayForward, _ := row["way_forward"].(string); !strings.Contains(wayForward, "permission to push feature/x") || !strings.Contains(wayForward, "lyx fabric push") {
		t.Errorf("failed_pushes way_forward = %q; want the auth way forward naming the branch and the push step", wayForward)
	}
}
