// report_test.go contains unit tests for the sandbox-report.json contract and the fetchReport
// validate/stamp/fetch pipeline.
// All tests use t.TempDir() -- no real lyx, claude, or network calls are made.
// The runFetch tests rewrite the package-level devBinPath and lookPath seams, so they do not call
// t.Parallel; the fetchReport tests touch no seam and run in parallel.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeBinaryInfo returns a fixed binaryInfo for fetchReport tests.
func fakeBinaryInfo() binaryInfo {
	return binaryInfo{
		Path:    "/fake/lyx.exe",
		Size:    1234,
		ModTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		SHA256:  "abc123def456",
		Source:  sourceProd,
	}
}

// writeReport writes body to <repoDir>/sandbox-report.json.
func writeReport(t *testing.T, repoDir, body string) {
	t.Helper()
	path := filepath.Join(repoDir, reportFileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
}

// scratchIsEmpty reports whether loomyardRoot/.scratch is absent or empty.
func scratchIsEmpty(t *testing.T, loomyardRoot string) bool {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(loomyardRoot, ".scratch"))
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		t.Fatalf("read .scratch dir: %v", err)
	}
	return len(entries) == 0
}

// TestFetchReport_AcceptsValidReport verifies a valid report is fetched into a .scratch directory
// that fetchReport creates, with its meta stamped from the binary's fingerprint over whatever meta
// the report carried; a present but empty items array is accepted, not rejected as malformed.
//
//testtiming:keep pins the stamped fingerprint and the decoded items of a fetched report, which TestRunFetch_StampsTheResolvedBinary reads only as far as the source
func TestFetchReport_AcceptsValidReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantItems []reportItem
	}{
		{
			name: "items and stale meta",
			body: `{
			"source": "sandbox-report",
			"meta": {"fingerprint": {"path": "stale", "sha256": "stale", "size": 0, "modtime": "stale"}},
			"items": [{"ref": "S6", "title": "bad error", "body": "verdict: WARN\n\nrepro steps"}]
		}`,
			wantItems: []reportItem{{Ref: "S6", Title: "bad error", Body: "verdict: WARN\n\nrepro steps"}},
		},
		{
			name:      "empty items array",
			body:      `{"source": "sandbox-report", "items": []}`,
			wantItems: []reportItem{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoDir := t.TempDir()
			loomyardRoot := t.TempDir()
			info := fakeBinaryInfo()

			scratchDir := filepath.Join(loomyardRoot, ".scratch")
			if _, err := os.Stat(scratchDir); !os.IsNotExist(err) {
				t.Fatalf(".scratch unexpectedly pre-exists: %v", err)
			}

			writeReport(t, repoDir, tt.body)

			if _, _, err := fetchReport(repoDir, loomyardRoot, info); err != nil {
				t.Fatalf("fetchReport() error: %v", err)
			}

			destPath := filepath.Join(scratchDir, "sandbox-report-"+info.SHA256+".json")
			raw, err := os.ReadFile(destPath)
			if err != nil {
				t.Fatalf("read fetched report %s: %v", destPath, err)
			}

			var got sandboxReport
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("decode fetched report: %v", err)
			}

			wantFingerprint := reportFingerprint{
				Path:    info.Path,
				SHA256:  info.SHA256,
				Size:    info.Size,
				ModTime: info.ModTime.Format(time.RFC3339),
				Source:  info.Source,
			}
			if got.Meta.Fingerprint != wantFingerprint {
				t.Errorf("Meta.Fingerprint = %+v; want %+v", got.Meta.Fingerprint, wantFingerprint)
			}
			if got.Meta.Fingerprint.Source != "prod" {
				t.Errorf("Meta.Fingerprint.Source = %q; want %q", got.Meta.Fingerprint.Source, "prod")
			}

			if got.Items == nil || len(*got.Items) != len(tt.wantItems) {
				t.Fatalf("Items = %v; want %d items", got.Items, len(tt.wantItems))
			}
			for i, wantItem := range tt.wantItems {
				if gotItem := (*got.Items)[i]; gotItem != wantItem {
					t.Errorf("Items[%d] = %+v; want %+v", i, gotItem, wantItem)
				}
			}
		})
	}
}

// TestFetchReport_RejectsInvalidReport verifies every unusable report is refused with an error that
// names the cause, and that nothing is written to .scratch: a missing items key, truncated JSON
// (the error names the source path), a missing or incorrect "source" field, and an absent file
// (a missing-file error distinct from the JSON parse error, so an operator can tell "the agent wrote
// nothing" from "the agent wrote garbage").
func TestFetchReport_RejectsInvalidReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// body is the report text; a nil body leaves the report file absent.
		body *string
		// wantInErr is a substring the error must carry.
		wantInErr string
		// wantPathInErr additionally requires the error to name the report's path.
		wantPathInErr bool
		// wantNotInErr is a substring the error must not carry.
		wantNotInErr string
	}{
		{name: "items key absent", body: ptr(`{"source": "sandbox-report"}`), wantInErr: "items"},
		{name: "malformed JSON", body: ptr(`{"source": "sandbox-report", "items": [`), wantPathInErr: true},
		{name: "wrong source value", body: ptr(`{"source": "something-else", "items": []}`), wantInErr: "source"},
		{name: "missing source field", body: ptr(`{"items": []}`), wantInErr: "source"},
		{name: "report absent", wantInErr: "not found", wantNotInErr: "parse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoDir := t.TempDir()
			loomyardRoot := t.TempDir()
			info := fakeBinaryInfo()

			if tt.body != nil {
				writeReport(t, repoDir, *tt.body)
			}

			_, _, err := fetchReport(repoDir, loomyardRoot, info)
			if err == nil {
				t.Fatal("fetchReport() error = nil; want a rejection")
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error = %q; want it to mention %q", err.Error(), tt.wantInErr)
			}
			if wantPath := filepath.Join(repoDir, reportFileName); tt.wantPathInErr && !strings.Contains(err.Error(), wantPath) {
				t.Errorf("error = %q; want it to mention path %q", err.Error(), wantPath)
			}
			if tt.wantNotInErr != "" && strings.Contains(err.Error(), tt.wantNotInErr) {
				t.Errorf("error = %q; want it not to mention %q", err.Error(), tt.wantNotInErr)
			}
			if !scratchIsEmpty(t, loomyardRoot) {
				t.Error(".scratch was written to despite a rejected report")
			}
		})
	}
}

// ptr returns a pointer to s.
func ptr(s string) *string { return &s }

// makeFetchHubRepo builds the Hub repo layout and returns parentDir and repoDir.
func makeFetchHubRepo(t *testing.T) (parentDir, repoDir string) {
	t.Helper()
	parentDir = t.TempDir()
	repoDir = filepath.Join(parentDir, hubName, repoDirName)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("create repo dir: %v", err)
	}
	return parentDir, repoDir
}

// stubLyxLookPath stubs resolveLyx to return sourceProd.
func stubLyxLookPath(t *testing.T, fakeLyx string) func() {
	t.Helper()
	oldDevBinPath := devBinPath
	devBinPath = func() (string, error) { return filepath.Join(t.TempDir(), "lyx"), nil }
	oldLookPath := lookPath
	lookPath = func(name string) (string, error) {
		if name == "lyx" {
			return fakeLyx, nil
		}
		return "", fmt.Errorf("not found on PATH: %s", name)
	}
	return func() {
		devBinPath = oldDevBinPath
		lookPath = oldLookPath
	}
}

// TestRunFetch_StampsTheResolvedBinary verifies runFetch fetches a valid report and stamps it with
// the fingerprint of the binary resolveLyx picked: the on-PATH binary as prod, or the dev binary as
// dev without consulting PATH at all.
func TestRunFetch_StampsTheResolvedBinary(t *testing.T) {
	tests := []struct {
		name       string
		wantSource string
		// stubSeams installs the resolveLyx seams and returns the binary runFetch must resolve.
		stubSeams func(t *testing.T, parentDir string) string
	}{
		{
			name:       "on-PATH binary is prod",
			wantSource: sourceProd,
			stubSeams: func(t *testing.T, parentDir string) string {
				fakeLyx := filepath.Join(parentDir, "lyx.exe")
				if err := os.WriteFile(fakeLyx, []byte("fake lyx binary"), 0o755); err != nil {
					t.Fatalf("write fake lyx: %v", err)
				}
				t.Cleanup(stubLyxLookPath(t, fakeLyx))
				return fakeLyx
			},
		},
		{
			name:       "dev binary skips the PATH fallback",
			wantSource: sourceDev,
			stubSeams: func(t *testing.T, parentDir string) string {
				devBinDir := filepath.Join(parentDir, ".dev-bin")
				if err := os.MkdirAll(devBinDir, 0o755); err != nil {
					t.Fatalf("mkdir dev-bin dir: %v", err)
				}
				devLyx := filepath.Join(devBinDir, "lyx")
				if err := os.WriteFile(devLyx, []byte("fake dev lyx binary"), 0o755); err != nil {
					t.Fatalf("write fake dev lyx: %v", err)
				}

				oldDevBinPath := devBinPath
				t.Cleanup(func() { devBinPath = oldDevBinPath })
				devBinPath = func() (string, error) { return devLyx, nil }

				oldLookPath := lookPath
				t.Cleanup(func() { lookPath = oldLookPath })
				lookPath = func(name string) (string, error) {
					t.Errorf("unexpected lookPath call for %q; a resolvable dev binary should skip the PATH fallback", name)
					return "", fmt.Errorf("not found on PATH: %s", name)
				}
				return devLyx
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parentDir, repoDir := makeFetchHubRepo(t)
			loomyardRoot := t.TempDir()
			binPath := tt.stubSeams(t, parentDir)

			writeReport(t, repoDir, `{"source": "sandbox-report", "items": []}`)

			if err := runFetch(parentDir, loomyardRoot); err != nil {
				t.Fatalf("runFetch() error: %v", err)
			}

			// The destination name embeds the SHA256 of the binary runFetch hashed.
			info, err := binaryFingerprint(binPath, tt.wantSource)
			if err != nil {
				t.Fatalf("binaryFingerprint: %v", err)
			}
			destPath := filepath.Join(loomyardRoot, ".scratch", "sandbox-report-"+info.SHA256+".json")
			raw, err := os.ReadFile(destPath)
			if err != nil {
				t.Fatalf("fetched report not found at %s: %v", destPath, err)
			}
			var got sandboxReport
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("decode fetched report: %v", err)
			}
			if got.Meta.Fingerprint.Source != tt.wantSource {
				t.Errorf("Meta.Fingerprint.Source = %q; want %q", got.Meta.Fingerprint.Source, tt.wantSource)
			}
		})
	}
}

// TestRunFetch_HubAbsent verifies runFetch returns an error when Hub is absent.
func TestRunFetch_HubAbsent(t *testing.T) {
	parentDir := t.TempDir()
	loomyardRoot := t.TempDir()

	old := lookPath
	defer func() { lookPath = old }()
	lookPath = func(name string) (string, error) {
		t.Error("lookPath should not be called when the Hub is absent")
		return "", fmt.Errorf("unexpected")
	}

	err := runFetch(parentDir, loomyardRoot)
	if err == nil {
		t.Fatal("runFetch should return error when the Hub repo subdir is absent")
	}
	if !strings.Contains(err.Error(), "sandbox/build.cmd") {
		t.Errorf("error should mention 'sandbox/build.cmd'; got %q", err.Error())
	}
}

// TestRunFetch_MissingReport verifies runFetch errors when no report exists.
func TestRunFetch_MissingReport(t *testing.T) {
	parentDir, _ := makeFetchHubRepo(t)
	loomyardRoot := t.TempDir()

	fakeLyx := filepath.Join(parentDir, "lyx.exe")
	if err := os.WriteFile(fakeLyx, []byte("fake lyx binary"), 0o755); err != nil {
		t.Fatalf("write fake lyx: %v", err)
	}
	restore := stubLyxLookPath(t, fakeLyx)
	defer restore()

	err := runFetch(parentDir, loomyardRoot)
	if err == nil {
		t.Fatal("runFetch should return error when the report is missing")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention the report was not found; got %q", err.Error())
	}
}
