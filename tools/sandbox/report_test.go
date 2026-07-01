// report_test.go contains unit tests for the sandbox-report.json contract and
// the fetchReport validate/stamp/fetch pipeline. All tests use t.TempDir() --
// no real lyx, claude, or network calls are made.

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

// fakeBinaryInfo returns a fixed binaryInfo used across fetchReport tests so
// the expected fingerprint fields are known and stable.
func fakeBinaryInfo() binaryInfo {
	return binaryInfo{
		Path:    "/fake/lyx.exe",
		Size:    1234,
		ModTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		SHA256:  "abc123def456",
	}
}

// writeHostReport writes body verbatim to <hostRepoDir>/sandbox-report.json.
func writeHostReport(t *testing.T, hostRepoDir, body string) {
	t.Helper()
	path := filepath.Join(hostRepoDir, reportFileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write host report: %v", err)
	}
}

// scratchIsEmpty reports whether loomyardRoot/.scratch is absent or contains
// no files, used to assert that a rejected report fetch wrote nothing.
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

// TestFetchReport_HappyPath verifies that a valid report is fetched into
// .scratch, its meta.fingerprint is stamped from the passed binaryInfo, its
// items are preserved, and any meta the agent wrote is overwritten.
func TestFetchReport_HappyPath(t *testing.T) {
	hostRepoDir := t.TempDir()
	loomyardRoot := t.TempDir()
	info := fakeBinaryInfo()

	// The agent only writes source/items, but stamp a bogus meta here to prove
	// fetchReport overwrites it rather than merging or preserving it.
	writeHostReport(t, hostRepoDir, `{
		"source": "sandbox-report",
		"meta": {"fingerprint": {"path": "stale", "sha256": "stale", "size": 0, "modtime": "stale"}},
		"items": [{"ref": "S6", "title": "bad error", "body": "verdict: WARN\n\nrepro steps"}]
	}`)

	if err := fetchReport(hostRepoDir, loomyardRoot, info); err != nil {
		t.Fatalf("fetchReport() error: %v", err)
	}

	destPath := filepath.Join(loomyardRoot, ".scratch", "sandbox-report-"+info.SHA256+".json")
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
	}
	if got.Meta.Fingerprint != wantFingerprint {
		t.Errorf("Meta.Fingerprint = %+v; want %+v", got.Meta.Fingerprint, wantFingerprint)
	}

	if got.Items == nil || len(*got.Items) != 1 {
		t.Fatalf("Items = %v; want 1 item", got.Items)
	}
	gotItem := (*got.Items)[0]
	wantItem := reportItem{Ref: "S6", Title: "bad error", Body: "verdict: WARN\n\nrepro steps"}
	if gotItem != wantItem {
		t.Errorf("Items[0] = %+v; want %+v", gotItem, wantItem)
	}
}

// TestFetchReport_EmptyItemsPresent verifies that a report with a present but
// empty items array is accepted and written, not rejected as malformed.
func TestFetchReport_EmptyItemsPresent(t *testing.T) {
	hostRepoDir := t.TempDir()
	loomyardRoot := t.TempDir()
	info := fakeBinaryInfo()

	writeHostReport(t, hostRepoDir, `{"source": "sandbox-report", "items": []}`)

	if err := fetchReport(hostRepoDir, loomyardRoot, info); err != nil {
		t.Fatalf("fetchReport() error: %v", err)
	}

	destPath := filepath.Join(loomyardRoot, ".scratch", "sandbox-report-"+info.SHA256+".json")
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("fetched report not written: %v", err)
	}
}

// TestFetchReport_ItemsKeyAbsent verifies that a report missing the items key
// entirely is rejected -- a plain []reportItem could not distinguish this
// from the empty-items case, which is why Items is decoded as a pointer.
func TestFetchReport_ItemsKeyAbsent(t *testing.T) {
	hostRepoDir := t.TempDir()
	loomyardRoot := t.TempDir()
	info := fakeBinaryInfo()

	writeHostReport(t, hostRepoDir, `{"source": "sandbox-report"}`)

	err := fetchReport(hostRepoDir, loomyardRoot, info)
	if err == nil {
		t.Fatal("fetchReport() error = nil; want error for missing items key")
	}
	if !strings.Contains(err.Error(), "items") {
		t.Errorf("error = %q; want it to mention items", err.Error())
	}
	if !scratchIsEmpty(t, loomyardRoot) {
		t.Error(".scratch was written to despite a rejected report")
	}
}

// TestFetchReport_MalformedJSON verifies that truncated/non-JSON input
// produces a parse error mentioning the source path, and writes nothing.
func TestFetchReport_MalformedJSON(t *testing.T) {
	hostRepoDir := t.TempDir()
	loomyardRoot := t.TempDir()
	info := fakeBinaryInfo()

	writeHostReport(t, hostRepoDir, `{"source": "sandbox-report", "items": [`)

	err := fetchReport(hostRepoDir, loomyardRoot, info)
	if err == nil {
		t.Fatal("fetchReport() error = nil; want parse error for malformed JSON")
	}
	wantPath := filepath.Join(hostRepoDir, reportFileName)
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error = %q; want it to mention path %q", err.Error(), wantPath)
	}
	if !scratchIsEmpty(t, loomyardRoot) {
		t.Error(".scratch was written to despite a malformed report")
	}
}

// TestFetchReport_WrongSource verifies that a structurally valid report with
// a missing or incorrect "source" field is rejected by validation.
func TestFetchReport_WrongSource(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"wrong_value", `{"source": "something-else", "items": []}`},
		{"missing_field", `{"items": []}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostRepoDir := t.TempDir()
			loomyardRoot := t.TempDir()
			info := fakeBinaryInfo()

			writeHostReport(t, hostRepoDir, tt.body)

			err := fetchReport(hostRepoDir, loomyardRoot, info)
			if err == nil {
				t.Fatal("fetchReport() error = nil; want validation error for wrong source")
			}
			if !strings.Contains(err.Error(), "source") {
				t.Errorf("error = %q; want it to mention source", err.Error())
			}
		})
	}
}

// TestFetchReport_MissingReport verifies that an absent sandbox-report.json
// produces a missing-file error distinct from the JSON parse error, so an
// operator can tell "the agent wrote nothing" from "the agent wrote garbage".
func TestFetchReport_MissingReport(t *testing.T) {
	hostRepoDir := t.TempDir()
	loomyardRoot := t.TempDir()
	info := fakeBinaryInfo()

	err := fetchReport(hostRepoDir, loomyardRoot, info)
	if err == nil {
		t.Fatal("fetchReport() error = nil; want error for missing report file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q; want it to mention the report was not found", err.Error())
	}
	if strings.Contains(err.Error(), "parse") {
		t.Errorf("error = %q; missing-report error should not look like a parse error", err.Error())
	}
}

// TestFetchReport_ScratchDirCreated verifies that fetchReport creates
// loomyardRoot/.scratch when it does not already exist.
func TestFetchReport_ScratchDirCreated(t *testing.T) {
	hostRepoDir := t.TempDir()
	loomyardRoot := t.TempDir()
	info := fakeBinaryInfo()

	scratchDir := filepath.Join(loomyardRoot, ".scratch")
	if _, err := os.Stat(scratchDir); !os.IsNotExist(err) {
		t.Fatalf(".scratch unexpectedly pre-exists: %v", err)
	}

	writeHostReport(t, hostRepoDir, `{"source": "sandbox-report", "items": []}`)

	if err := fetchReport(hostRepoDir, loomyardRoot, info); err != nil {
		t.Fatalf("fetchReport() error: %v", err)
	}
	if _, err := os.Stat(scratchDir); err != nil {
		t.Errorf(".scratch was not created: %v", err)
	}
}

// makeFetchHostRepo builds the Hub host-repo layout under a fresh temp dir and
// returns both the parentDir that runFetch expects and the host repo path.
func makeFetchHostRepo(t *testing.T) (parentDir, hostRepoDir string) {
	t.Helper()
	parentDir = t.TempDir()
	hostRepoDir = filepath.Join(parentDir, hubName, hostDirName)
	if err := os.MkdirAll(hostRepoDir, 0o755); err != nil {
		t.Fatalf("create host repo dir: %v", err)
	}
	return parentDir, hostRepoDir
}

// stubLyxLookPath points lookPath at fakeLyx for "lyx" so runFetch can
// fingerprint a real temp file, and returns a restore function.
func stubLyxLookPath(t *testing.T, fakeLyx string) func() {
	t.Helper()
	old := lookPath
	lookPath = func(name string) (string, error) {
		if name == "lyx" {
			return fakeLyx, nil
		}
		return "", fmt.Errorf("not found on PATH: %s", name)
	}
	return func() { lookPath = old }
}

// TestRunFetch_HappyPath verifies that runFetch fingerprints the on-PATH lyx and
// fetches a valid host report into <loomyardRoot>/.scratch under that fingerprint.
func TestRunFetch_HappyPath(t *testing.T) {
	parentDir, hostRepoDir := makeFetchHostRepo(t)
	loomyardRoot := t.TempDir()

	// binaryFingerprint stats and hashes the file, so it must really exist.
	fakeLyx := filepath.Join(parentDir, "lyx.exe")
	if err := os.WriteFile(fakeLyx, []byte("fake lyx binary"), 0o755); err != nil {
		t.Fatalf("write fake lyx: %v", err)
	}
	restore := stubLyxLookPath(t, fakeLyx)
	defer restore()

	writeHostReport(t, hostRepoDir, `{"source": "sandbox-report", "items": []}`)

	if err := runFetch(parentDir, loomyardRoot); err != nil {
		t.Fatalf("runFetch() error: %v", err)
	}

	// The destination name embeds the SHA256 of the on-PATH binary runFetch hashed.
	info, err := binaryFingerprint(fakeLyx)
	if err != nil {
		t.Fatalf("binaryFingerprint: %v", err)
	}
	destPath := filepath.Join(loomyardRoot, ".scratch", "sandbox-report-"+info.SHA256+".json")
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("fetched report not found at %s: %v", destPath, err)
	}
}

// TestRunFetch_HubAbsent verifies that runFetch returns a clear, actionable
// error and does not touch lookPath when the Hub host subdir does not exist.
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
		t.Fatal("runFetch should return error when the Hub host subdir is absent")
	}
	if !strings.Contains(err.Error(), "sandbox build") {
		t.Errorf("error should mention 'sandbox build'; got %q", err.Error())
	}
}

// TestRunFetch_MissingHostReport verifies that runFetch surfaces fetchReport's
// missing-report error when the agent produced no sandbox-report.json.
func TestRunFetch_MissingHostReport(t *testing.T) {
	parentDir, _ := makeFetchHostRepo(t)
	loomyardRoot := t.TempDir()

	fakeLyx := filepath.Join(parentDir, "lyx.exe")
	if err := os.WriteFile(fakeLyx, []byte("fake lyx binary"), 0o755); err != nil {
		t.Fatalf("write fake lyx: %v", err)
	}
	restore := stubLyxLookPath(t, fakeLyx)
	defer restore()

	// Deliberately do not write a host report.
	err := runFetch(parentDir, loomyardRoot)
	if err == nil {
		t.Fatal("runFetch should return error when the host report is missing")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention the report was not found; got %q", err.Error())
	}
}
