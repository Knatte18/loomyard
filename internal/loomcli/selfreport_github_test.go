// selfreport_github_test.go is an untagged Tier-1 suite binding the engine boundary rather than
// the told-seam boundary: the told filing seam is the loomcli unit boundary where every branch,
// filing-pass-order, collapse, marker, and config assertion already binds in selfreport_test.go,
// and this file is the engine boundary, where the actual argument shapes reaching the filing
// primitive are observable. No test here binds both.
//
// Every test here swaps selfreportengine.NewGitHubClient -- the exported package variable that
// exists for exactly this -- pointing it at a real go-github client aimed at an httptest server,
// following the shape internal/selfreportengine/selfreport_test.go already uses, and restores it
// via t.Cleanup. No test here resolves a real token, reaches the network, spawns a process, or
// builds a fixture tree.

package loomcli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// githubRequestCapture describes one issue-create request the filing primitive actually sent.
type githubRequestCapture struct {
	path string
	body map[string]any
}

// newGitHubIssueServer returns an httptest server that responds to every request with status and
// respBody, appending a githubRequestCapture for each request it receives to captured.
func newGitHubIssueServer(t *testing.T, status int, respBody string, captured *[]githubRequestCapture) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		*captured = append(*captured, githubRequestCapture{path: r.URL.Path, body: decoded})
		w.WriteHeader(status)
		if respBody != "" {
			_, _ = w.Write([]byte(respBody))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// installGitHubClientForDrive swaps selfreportengine.NewGitHubClient for a closure returning a
// real go-github client pointed at baseURL, authenticated with a fixed placeholder token set
// directly on the client rather than through githubclient's resolution chain, and restores the
// original seam via t.Cleanup.
func installGitHubClientForDrive(t *testing.T, baseURL string) {
	t.Helper()
	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(baseURL + "/")
	if err != nil {
		t.Fatalf("url.Parse(%s): %v", baseURL, err)
	}
	client.BaseURL = parsed

	orig := selfreportengine.NewGitHubClient
	selfreportengine.NewGitHubClient = func() (*github.Client, error) { return client, nil }
	t.Cleanup(func() { selfreportengine.NewGitHubClient = orig })
}

// engineBoundaryFixture builds a fresh selfreportDeps wired against the real
// selfreportengine.CreateIssue filing seam, so every request this test drives crosses the actual
// engine boundary rather than a stub.
func engineBoundaryFixture(t *testing.T) (*selfreportTestFixture, selfreportDeps) {
	t.Helper()
	f := newSelfreportTestFixture(t)
	deps := f.deps(context.Background(), loomengine.EntryObservation{}, nil)
	deps.FileIssue = selfreportengine.CreateIssue
	return f, deps
}

// TestDetectAndFileAnomalies_EngineBoundary_RequestShapes asserts, with selfreportengine.CreateIssue
// itself as the filing seam: one issue-create request per detected anomaly; each request's title
// matches the deterministic shape for its kind; each request's body contains the slug, and for a
// halt kind the halt reason, and for a recurring finding the ledger key and every element of its
// rounds list; and each request's label list is exactly the engine's own default.
func TestDetectAndFileAnomalies_EngineBoundary_RequestShapes(t *testing.T) {
	var captured []githubRequestCapture
	server := newGitHubIssueServer(t, http.StatusCreated, `{"html_url":"https://example.invalid/issues/1","number":1}`, &captured)
	installGitHubClientForDrive(t, server.URL)

	f, deps := engineBoundaryFixture(t)
	deps = withLedgerSeams(deps, f, map[string]ledgerFixture{
		"/run/round-3-bouncer-ledger.md": {round: 3, entries: []shedadapters.LedgerEntry{{Key: "finding-a", Rounds: []int{1, 2, 3}, Status: "open"}}},
	})
	deps.Entry = crashResumeEntry()

	st := haltStatus()
	st.History = append(st.History, shedengine.HistoryEntry{Producer: "Discussion-Bouncer", Outcome: shedengine.Stuck, Output: "/run/round-3-bouncer-ledger.md", At: "t9"})
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, deps.StatusPath, deps.StatusLockPath, st)

	detectAndFileAnomalies(deps)

	if len(captured) != 3 {
		t.Fatalf("request count = %d; want 3 (crash-resume, producer-hard-failure, recurring-finding): %+v", len(captured), captured)
	}
	for _, req := range captured {
		if req.path != "/repos/Knatte18/loomyard/issues" {
			t.Errorf("request path = %q; want %q", req.path, "/repos/Knatte18/loomyard/issues")
		}
		title, _ := req.body["title"].(string)
		if !strings.HasPrefix(title, "loom anomaly: ") {
			t.Errorf("request title = %q; want it to start with %q", title, "loom anomaly: ")
		}
		if !strings.Contains(title, "a-task") {
			t.Errorf("request title = %q; want it to carry the slug %q", title, "a-task")
		}

		body, _ := req.body["body"].(string)
		if !strings.Contains(body, "a-task") {
			t.Errorf("request body = %q; want it to carry the slug %q", body, "a-task")
		}

		labelsRaw, _ := req.body["labels"].([]any)
		if len(labelsRaw) != 1 || labelsRaw[0] != "bug" {
			t.Errorf("request labels = %v; want exactly the engine's default [\"bug\"]", labelsRaw)
		}

		switch {
		case strings.Contains(title, "producer-hard-failure"):
			if !strings.Contains(body, "boom") {
				t.Errorf("producer-hard-failure body = %q; want it to carry the halt reason %q", body, "boom")
			}
		case strings.Contains(title, "recurring-finding"):
			if !strings.Contains(body, "finding-a") || !strings.Contains(body, "[1 2 3]") {
				t.Errorf("recurring-finding body = %q; want it to carry the ledger key and full rounds list", body)
			}
		}
	}
}

// TestDetectAndFileAnomalies_FilingFailure_LeavesMarkerUnfiledAndDoesNotPropagate asserts a
// filing call that fails leaves that title out of the marker and does not propagate -- both
// halves, the absent marker entry and the unchanged surrounding behaviour.
func TestDetectAndFileAnomalies_FilingFailure_LeavesMarkerUnfiledAndDoesNotPropagate(t *testing.T) {
	var captured []githubRequestCapture
	server := newGitHubIssueServer(t, http.StatusUnprocessableEntity, `{"message":"Validation Failed"}`, &captured)
	installGitHubClientForDrive(t, server.URL)

	f, deps := engineBoundaryFixture(t)
	deps.Entry = crashResumeEntry()

	st := shedengine.Status{}
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, deps.StatusPath, deps.StatusLockPath, st)

	detectAndFileAnomalies(deps) // must not panic on the API rejection

	if len(captured) != 1 {
		t.Fatalf("request count = %d; want 1", len(captured))
	}

	marker := readFiledMarker(f.markerPath, f.markerLockPath)
	if len(marker.Titles) != 0 {
		t.Errorf("marker after a failed filing call = %+v; want empty (the title stays unrecorded so it is retried)", marker)
	}
}

// TestDetectAndFileAnomalies_AlreadyFiledTitle_ProducesNoRequest asserts an anomaly whose title is
// already in the marker produces no request at all.
func TestDetectAndFileAnomalies_AlreadyFiledTitle_ProducesNoRequest(t *testing.T) {
	var captured []githubRequestCapture
	server := newGitHubIssueServer(t, http.StatusCreated, `{"html_url":"https://example.invalid/issues/1","number":1}`, &captured)
	installGitHubClientForDrive(t, server.URL)

	f, deps := engineBoundaryFixture(t)
	deps.Entry = crashResumeEntry()

	st := shedengine.Status{}
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, deps.StatusPath, deps.StatusLockPath, st)

	// Prime the marker directly with the title this run would otherwise file, mirroring
	// production's own record step.
	crash, ok := loomengine.DetectCrashResume(deps.Entry)
	if !ok {
		t.Fatal("crashResumeEntry() no longer produces a crash-resume anomaly; fixture is stale")
	}
	writeFiledMarker(f.markerPath, f.markerLockPath, selfreportFiledMarker{Titles: []string{crash.Title}})

	detectAndFileAnomalies(deps)

	if len(captured) != 0 {
		t.Errorf("request count = %d; want 0 (the title is already recorded in the marker)", len(captured))
	}
}

// TestDetectAndFileAnomalies_MarkerReadFailure_TreatedAsEmptyAndFilingProceeds asserts a marker
// read failure is treated as an empty marker and filing proceeds.
func TestDetectAndFileAnomalies_MarkerReadFailure_TreatedAsEmptyAndFilingProceeds(t *testing.T) {
	var captured []githubRequestCapture
	server := newGitHubIssueServer(t, http.StatusCreated, `{"html_url":"https://example.invalid/issues/1","number":1}`, &captured)
	installGitHubClientForDrive(t, server.URL)

	f, deps := engineBoundaryFixture(t)
	deps.Entry = crashResumeEntry()

	st := shedengine.Status{}
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, deps.StatusPath, deps.StatusLockPath, st)

	// Corrupt the marker file directly -- genuinely malformed JSON, not merely an unknown field --
	// so state.ReadJSONStrict's decode step fails.
	if err := writeRawFile(f.markerPath, `{ "titles": [`); err != nil {
		t.Fatalf("write malformed marker: %v", err)
	}

	detectAndFileAnomalies(deps)

	if len(captured) != 1 {
		t.Errorf("request count = %d; want 1 (an unreadable marker must not block filing)", len(captured))
	}
}

// TestDetectAndFileAnomalies_MarkerWriteFailure_DoesNotPropagate asserts a marker write failure
// does not propagate: filing still happens, and the call does not panic or otherwise surface the
// write failure.
func TestDetectAndFileAnomalies_MarkerWriteFailure_DoesNotPropagate(t *testing.T) {
	var captured []githubRequestCapture
	server := newGitHubIssueServer(t, http.StatusCreated, `{"html_url":"https://example.invalid/issues/1","number":1}`, &captured)
	installGitHubClientForDrive(t, server.URL)

	_, deps := engineBoundaryFixture(t)
	deps.Entry = crashResumeEntry()
	// A directory in place of the marker file makes the write step's rename-into-place fail. The
	// read step degrades the same way a genuinely absent marker does (both are "no marker
	// content"), which is exactly the surrounding behaviour this case asserts stays unchanged.
	deps.MarkerPath = filepath.Join(t.TempDir(), "marker-dir")
	if err := os.MkdirAll(deps.MarkerPath, 0o755); err != nil {
		t.Fatalf("mkdir marker path: %v", err)
	}

	st := shedengine.Status{}
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, deps.StatusPath, deps.StatusLockPath, st)

	detectAndFileAnomalies(deps) // must not panic on the write failure

	if len(captured) != 1 {
		t.Errorf("request count = %d; want 1 (filing itself must still proceed)", len(captured))
	}
}

// writeRawFile writes content verbatim to path, creating parent directories as needed.
func writeRawFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
