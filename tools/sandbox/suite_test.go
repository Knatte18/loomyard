// suite_test.go contains unit tests for the suite launcher functions: binary
// fingerprinting, scheme rendering, git-exclude management, and the runSuite
// orchestration. All tests use seam stubs and temp directories -- no real
// lyx, claude, or network calls are made.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBinaryFingerprint_TempFile verifies that binaryFingerprint returns the
// correct size, a 12-character hex SHA256 prefix, and the absolute path for a
// real temp file with known content.
func TestBinaryFingerprint_TempFile(t *testing.T) {
	content := []byte("fake lyx binary content for testing")
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lyx.exe")
	if err := os.WriteFile(binPath, content, 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	info, err := binaryFingerprint(binPath)
	if err != nil {
		t.Fatalf("binaryFingerprint(%s) error: %v", binPath, err)
	}

	if info.Path != binPath {
		t.Errorf("Path = %q; want %q", info.Path, binPath)
	}
	if info.Size != int64(len(content)) {
		t.Errorf("Size = %d; want %d", info.Size, len(content))
	}
	if len(info.SHA256) != 12 {
		t.Errorf("SHA256 length = %d; want 12", len(info.SHA256))
	}

	// Compute the expected digest independently to confirm correctness.
	h := sha256.New()
	h.Write(content)
	wantDigest := hex.EncodeToString(h.Sum(nil))[:12]
	if info.SHA256 != wantDigest {
		t.Errorf("SHA256 = %q; want %q", info.SHA256, wantDigest)
	}
}

// TestBinaryFingerprint_MissingPath verifies that binaryFingerprint returns an
// error when the target file does not exist.
func TestBinaryFingerprint_MissingPath(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "nonexistent.exe")
	_, err := binaryFingerprint(missingPath)
	if err == nil {
		t.Error("binaryFingerprint on missing path should return error; got nil")
	}
}

// TestRenderScheme_ContainsHeaderAndBody verifies that renderScheme embeds the
// binary fingerprint fields and the embedded SANDBOX-SUITE body in its output.
func TestRenderScheme_ContainsHeaderAndBody(t *testing.T) {
	info := binaryInfo{
		Path:    "/fake/lyx.exe",
		Size:    1234,
		ModTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		SHA256:  "abc123def456",
	}
	got := renderScheme(info)

	// Each field must appear in the rendered output.
	checks := []struct {
		label string
		want  string
	}{
		{"path", "/fake/lyx.exe"},
		{"size", "1234 bytes"},
		{"sha256", "abc123def456"},
		{"scheme heading", "SANDBOX-SUITE"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.want) {
			t.Errorf("renderScheme() missing %s: %q not found in output", c.label, c.want)
		}
	}
}

// TestEnsureGitExclude covers the four behaviour scenarios for idempotent
// exclude-file management.
func TestEnsureGitExclude(t *testing.T) {
	const entry = "SANDBOX-SUITE.md"

	// createGitDir sets up a minimal <dir>/.git directory (without info/).
	createGitDir := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir .git: %v", err)
		}
		return dir
	}

	t.Run("creates_missing_exclude_file", func(t *testing.T) {
		dir := createGitDir(t)
		if err := ensureGitExclude(dir, entry); err != nil {
			t.Fatalf("ensureGitExclude: %v", err)
		}
		content, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if !strings.Contains(string(content), entry) {
			t.Errorf("exclude file does not contain entry; got %q", string(content))
		}
	})

	t.Run("idempotent_on_second_call", func(t *testing.T) {
		dir := createGitDir(t)
		// First call creates the file with the entry.
		if err := ensureGitExclude(dir, entry); err != nil {
			t.Fatalf("first call: %v", err)
		}
		// Second call must not duplicate the entry.
		if err := ensureGitExclude(dir, entry); err != nil {
			t.Fatalf("second call: %v", err)
		}
		content, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		count := strings.Count(string(content), entry)
		if count != 1 {
			t.Errorf("entry appears %d times; want exactly 1", count)
		}
	})

	t.Run("preserves_existing_content", func(t *testing.T) {
		dir := createGitDir(t)
		// Pre-populate the exclude file with unrelated content.
		infoDir := filepath.Join(dir, ".git", "info")
		if err := os.MkdirAll(infoDir, 0o755); err != nil {
			t.Fatalf("mkdir info: %v", err)
		}
		existing := "# git/info/exclude\n*.log\nbuild/\n"
		excludePath := filepath.Join(infoDir, "exclude")
		if err := os.WriteFile(excludePath, []byte(existing), 0o644); err != nil {
			t.Fatalf("write existing content: %v", err)
		}

		if err := ensureGitExclude(dir, entry); err != nil {
			t.Fatalf("ensureGitExclude: %v", err)
		}
		content, err := os.ReadFile(excludePath)
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		for _, preserved := range []string{"# git/info/exclude", "*.log", "build/"} {
			if !strings.Contains(string(content), preserved) {
				t.Errorf("existing content %q was not preserved", preserved)
			}
		}
		if !strings.Contains(string(content), entry) {
			t.Errorf("new entry %q was not appended", entry)
		}
	})

	t.Run("creates_info_dir_when_absent", func(t *testing.T) {
		// .git exists but .git/info/ does not -- ensureGitExclude must create it.
		dir := createGitDir(t)
		if err := ensureGitExclude(dir, entry); err != nil {
			t.Fatalf("ensureGitExclude: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git", "info", "exclude")); err != nil {
			t.Errorf("exclude file not created when info/ was absent: %v", err)
		}
	})
}

// stubSuiteSeams replaces lookPath and launchAgent with test stubs and returns
// a restore function. fakeLyx must be a real file path so binaryFingerprint
// can stat and hash it. fakeClaude is returned as the "claude" resolution.
func stubSuiteSeams(t *testing.T, fakeLyx, fakeClaude string, launchFn func(dir, claude, instruction string) int) func() {
	t.Helper()
	oldLookPath := lookPath
	oldLaunchAgent := launchAgent
	lookPath = func(name string) (string, error) {
		switch name {
		case "lyx":
			return fakeLyx, nil
		case "claude":
			return fakeClaude, nil
		default:
			return "", fmt.Errorf("not found on PATH: %s", name)
		}
	}
	launchAgent = launchFn
	return func() {
		lookPath = oldLookPath
		launchAgent = oldLaunchAgent
	}
}

// makeHostRepo creates the full Hub/host-repo directory structure under a temp
// dir and returns both the temp dir (parentDir for runSuite) and the host repo
// path. It also creates .git/info/ so ensureGitExclude has somewhere to write.
func makeHostRepo(t *testing.T) (parentDir, hostRepoDir string) {
	t.Helper()
	parentDir = t.TempDir()
	hostRepoDir = filepath.Join(parentDir, hubName, hostDirName)
	if err := os.MkdirAll(filepath.Join(hostRepoDir, ".git", "info"), 0o755); err != nil {
		t.Fatalf("create host repo dir: %v", err)
	}
	return parentDir, hostRepoDir
}

// makeFakeLyx writes a small binary file to tmpDir and returns its path.
func makeFakeLyx(t *testing.T, tmpDir string) string {
	t.Helper()
	fakeLyx := filepath.Join(tmpDir, "lyx.exe")
	if err := os.WriteFile(fakeLyx, []byte("fake lyx binary"), 0o755); err != nil {
		t.Fatalf("write fake lyx: %v", err)
	}
	return fakeLyx
}

// writeValidSuiteReport writes a minimal valid sandbox-report.json into
// hostRepoDir, as a clean-exit launchAgent stub must do so the post-launch
// fetchReport call inside runSuite finds something to fetch.
func writeValidSuiteReport(t *testing.T, hostRepoDir string) {
	t.Helper()
	path := filepath.Join(hostRepoDir, reportFileName)
	body := `{"source": "sandbox-report", "items": []}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write valid suite report: %v", err)
	}
}

// TestRunSuite_HubAbsent verifies that runSuite returns a clear error and does
// not call launchAgent when the Hub host subdirectory does not exist.
func TestRunSuite_HubAbsent(t *testing.T) {
	parentDir := t.TempDir()
	loomyardRoot := t.TempDir()

	restore := stubSuiteSeams(t, "", "", func(dir, claude, instruction string) int {
		t.Error("launchAgent should not be called when Hub is absent")
		return 1
	})
	defer restore()

	err := runSuite(parentDir, loomyardRoot, "", "")
	if err == nil {
		t.Fatal("runSuite should return error when Hub host subdir is absent")
	}
	if !strings.Contains(err.Error(), "sandbox build") {
		t.Errorf("error should mention 'sandbox build'; got %q", err.Error())
	}
}

// TestRunSuite_LaunchInvocation verifies that runSuite calls launchAgent with
// the correct host-repo directory, claude binary path, and default instruction.
func TestRunSuite_LaunchInvocation(t *testing.T) {
	parentDir, hostRepoDir := makeHostRepo(t)
	loomyardRoot := t.TempDir()
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	var gotDir, gotClaude, gotInstruction string
	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction string) int {
		gotDir = dir
		gotClaude = claude
		gotInstruction = instruction
		// runSuite fetches the report after a clean exit; write one so the
		// fetch succeeds and runSuite returns nil as this test expects.
		writeValidSuiteReport(t, hostRepoDir)
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, loomyardRoot, "", ""); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if gotDir != hostRepoDir {
		t.Errorf("launchAgent dir = %q; want %q", gotDir, hostRepoDir)
	}
	if gotClaude != fakeClaude {
		t.Errorf("launchAgent claude = %q; want %q", gotClaude, fakeClaude)
	}
	if gotInstruction != defaultInstruction {
		t.Errorf("launchAgent instruction = %q; want %q", gotInstruction, defaultInstruction)
	}
}

// TestRunSuite_Overrides verifies that runSuite honours the -claude and -prompt
// override arguments, passing them straight to launchAgent without PATH lookup.
func TestRunSuite_Overrides(t *testing.T) {
	parentDir, hostRepoDir := makeHostRepo(t)
	loomyardRoot := t.TempDir()
	fakeLyx := makeFakeLyx(t, parentDir)

	customClaude := filepath.Join(parentDir, "custom-claude.exe")
	customPrompt := "Do something entirely custom."

	// lookPath should only be called for "lyx" when a claude override is supplied.
	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(name string) (string, error) {
		if name == "lyx" {
			return fakeLyx, nil
		}
		t.Errorf("unexpected lookPath call for %q; claude override should skip PATH lookup", name)
		return "", fmt.Errorf("not found")
	}

	var gotClaude, gotInstruction string
	oldLaunchAgent := launchAgent
	defer func() { launchAgent = oldLaunchAgent }()
	launchAgent = func(dir, claude, instruction string) int {
		gotClaude = claude
		gotInstruction = instruction
		// runSuite fetches the report after a clean exit; write one so the
		// fetch succeeds and runSuite returns nil as this test expects.
		writeValidSuiteReport(t, hostRepoDir)
		return 0
	}

	if err := runSuite(parentDir, loomyardRoot, customClaude, customPrompt); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if gotClaude != customClaude {
		t.Errorf("launchAgent claude = %q; want %q", gotClaude, customClaude)
	}
	if gotInstruction != customPrompt {
		t.Errorf("launchAgent instruction = %q; want %q", gotInstruction, customPrompt)
	}
}

// TestRunSuite_NonZeroLaunchCode verifies that a non-zero exit code from
// launchAgent is propagated as an error by runSuite.
func TestRunSuite_NonZeroLaunchCode(t *testing.T) {
	parentDir, _ := makeHostRepo(t)
	loomyardRoot := t.TempDir()
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction string) int {
		return 2
	})
	defer restore()

	err := runSuite(parentDir, loomyardRoot, "", "")
	if err == nil {
		t.Fatal("runSuite should return error when launchAgent returns non-zero")
	}
	if !strings.Contains(err.Error(), "2") {
		t.Errorf("error should mention exit code 2; got %q", err.Error())
	}
	// A non-zero exit must not fetch a report -- .scratch should stay absent.
	if _, err := os.Stat(filepath.Join(loomyardRoot, ".scratch")); !os.IsNotExist(err) {
		t.Errorf(".scratch should remain absent after a non-zero launch; stat err = %v", err)
	}
}

// TestRunSuite_ClaudeNotFound verifies that runSuite returns a clear error when
// claude cannot be resolved from PATH and no override is given.
func TestRunSuite_ClaudeNotFound(t *testing.T) {
	parentDir, _ := makeHostRepo(t)
	loomyardRoot := t.TempDir()
	fakeLyx := makeFakeLyx(t, parentDir)

	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(name string) (string, error) {
		if name == "lyx" {
			return fakeLyx, nil
		}
		// claude is not on PATH.
		return "", fmt.Errorf("executable file not found in %%PATH%%")
	}

	oldLaunchAgent := launchAgent
	defer func() { launchAgent = oldLaunchAgent }()
	launchAgent = func(dir, claude, instruction string) int {
		t.Error("launchAgent should not be called when claude is not found")
		return 1
	}

	err := runSuite(parentDir, loomyardRoot, "", "")
	if err == nil {
		t.Fatal("runSuite should return error when claude is not on PATH")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should mention 'claude'; got %q", err.Error())
	}
}

// TestRunSuite_FetchesReport verifies that runSuite fetches the agent-written
// report into <loomyardRoot>/.scratch on a clean exit.
func TestRunSuite_FetchesReport(t *testing.T) {
	parentDir, hostRepoDir := makeHostRepo(t)
	loomyardRoot := t.TempDir()
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction string) int {
		writeValidSuiteReport(t, hostRepoDir)
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, loomyardRoot, "", ""); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}

	info, err := binaryFingerprint(fakeLyx)
	if err != nil {
		t.Fatalf("binaryFingerprint: %v", err)
	}
	destPath := filepath.Join(loomyardRoot, ".scratch", "sandbox-report-"+info.SHA256+".json")
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("fetched report not found at %s: %v", destPath, err)
	}
}

// TestRunSuite_StaleReportRemoved verifies that runSuite removes a prior
// sandbox-report.json before launching the agent, so a stub that writes
// nothing surfaces the missing-report error rather than fetching stale
// findings under a fresh fingerprint.
func TestRunSuite_StaleReportRemoved(t *testing.T) {
	parentDir, hostRepoDir := makeHostRepo(t)
	loomyardRoot := t.TempDir()
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	// Pre-create a stale report from a prior run.
	stalePath := filepath.Join(hostRepoDir, reportFileName)
	if err := os.WriteFile(stalePath, []byte(`{"source": "sandbox-report", "items": [{"ref": "S0", "title": "stale", "body": "stale"}]}`), 0o644); err != nil {
		t.Fatalf("write stale report: %v", err)
	}

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction string) int {
		// Writes nothing: simulates an agent session that ends cleanly without
		// rewriting the report.
		return 0
	})
	defer restore()

	err := runSuite(parentDir, loomyardRoot, "", "")
	if err == nil {
		t.Fatal("runSuite should return the missing-report error when no fresh report is written")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention the report was not found; got %q", err.Error())
	}
	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stale report should have been removed before launch; stat err = %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(loomyardRoot, ".scratch")); !os.IsNotExist(statErr) {
		t.Errorf(".scratch should remain absent; no stale report should be fetched")
	}
}

// TestRunSuite_ExcludesReport verifies that runSuite registers
// sandbox-report.json in the host repo's .git/info/exclude, alongside the
// existing SANDBOX-SUITE.md entry.
func TestRunSuite_ExcludesReport(t *testing.T) {
	parentDir, hostRepoDir := makeHostRepo(t)
	loomyardRoot := t.TempDir()
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction string) int {
		writeValidSuiteReport(t, hostRepoDir)
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, loomyardRoot, "", ""); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}

	excludePath := filepath.Join(hostRepoDir, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read .git/info/exclude: %v", err)
	}
	for _, entry := range []string{suiteFileName, reportFileName} {
		if !strings.Contains(string(content), entry) {
			t.Errorf(".git/info/exclude missing entry %q; got %q", entry, string(content))
		}
	}
}
