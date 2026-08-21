// suite_test.go contains unit tests for the suite launcher functions: binary fingerprinting, scheme
// rendering, git-exclude management, and the runSuite orchestration.
// All tests use seam stubs and temp directories -- no real lyx, claude, or network calls are made.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBinaryFingerprint_TempFile verifies that binaryFingerprint returns the correct size, SHA256
// prefix, and path for a real temp file.
func TestBinaryFingerprint_TempFile(t *testing.T) {
	content := []byte("fake lyx binary content for testing")
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lyx.exe")
	if err := os.WriteFile(binPath, content, 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	info, err := binaryFingerprint(binPath, sourceProd)
	if err != nil {
		t.Fatalf("binaryFingerprint(%s) error: %v", binPath, err)
	}

	if info.Path != binPath {
		t.Errorf("Path = %q; want %q", info.Path, binPath)
	}
	if info.Source != sourceProd {
		t.Errorf("Source = %q; want %q", info.Source, sourceProd)
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

// TestBinaryFingerprint_MissingPath verifies that binaryFingerprint returns an error when the
// target file does not exist.
func TestBinaryFingerprint_MissingPath(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "nonexistent.exe")
	_, err := binaryFingerprint(missingPath, sourceProd)
	if err == nil {
		t.Error("binaryFingerprint on missing path should return error; got nil")
	}
}

// TestRenderScheme_ContainsHeaderAndBody verifies that renderScheme embeds the fingerprint header
// and suite body.
func TestRenderScheme_ContainsHeaderAndBody(t *testing.T) {
	info := binaryInfo{
		Path:    "/fake/lyx.exe",
		Size:    1234,
		ModTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		SHA256:  "abc123def456",
		Source:  sourceProd,
	}
	got := renderScheme(info, sandboxSuiteMD)

	checks := []struct {
		label string
		want  string
	}{
		{"path", "/fake/lyx.exe"},
		{"size", "1234 bytes"},
		{"sha256", "abc123def456"},
		{"source", "Source: prod"},
		{"scheme heading", "SANDBOX-CORE-SUITE"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.want) {
			t.Errorf("renderScheme() missing %s: %q not found in output", c.label, c.want)
		}
	}
}

// TestBinaryInfoHeader_ContainsSourceLine verifies that header() renders a "- Source: %s" line for
// both sourceDev and sourceProd.
func TestBinaryInfoHeader_ContainsSourceLine(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"dev", sourceDev},
		{"prod", sourceProd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := binaryInfo{
				Path:    "/fake/lyx.exe",
				Size:    1,
				ModTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				SHA256:  "abc123def456",
				Source:  tt.source,
			}
			got := info.header()
			want := "- Source: " + tt.source
			if !strings.Contains(got, want) {
				t.Errorf("header() = %q; want it to contain %q", got, want)
			}
		})
	}
}

// TestEnsureGitExclude covers the four behaviour scenarios for idempotent exclude-file management.
func TestEnsureGitExclude(t *testing.T) {
	const entry = "SANDBOX-CORE-SUITE.md"

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
		if err := ensureGitExclude(dir, entry); err != nil {
			t.Fatalf("first call: %v", err)
		}
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
		dir := createGitDir(t)
		if err := ensureGitExclude(dir, entry); err != nil {
			t.Fatalf("ensureGitExclude: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git", "info", "exclude")); err != nil {
			t.Errorf("exclude file not created when info/ was absent: %v", err)
		}
	})
}

// stubSuiteSeams replaces devBinPath, lookPath, launchAgent, and reedDown
// with test stubs and returns a restore function.
func stubSuiteSeams(t *testing.T, fakeLyx, fakeClaude string, launchFn func(dir, claude, instruction, binDir string) int) func() {
	t.Helper()
	oldDevBinPath := devBinPath
	oldLookPath := lookPath
	oldLaunchAgent := launchAgent
	oldReedDown := reedDown
	devBinPath = func() (string, error) {
		return filepath.Join(t.TempDir(), "lyx"), nil
	}
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
	reedDown = func(warpRepoDir, lyxPath string) error { return nil }
	return func() {
		devBinPath = oldDevBinPath
		lookPath = oldLookPath
		launchAgent = oldLaunchAgent
		reedDown = oldReedDown
	}
}

// stubReedDownNoop replaces the reedDown seam with a no-op.
func stubReedDownNoop(t *testing.T) {
	t.Helper()
	old := reedDown
	reedDown = func(warpRepoDir, lyxPath string) error { return nil }
	t.Cleanup(func() { reedDown = old })
}

// captureStderr redirects os.Stderr for the duration of fn and returns
// everything written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(data)
}

// makeWarpRepo creates the full Hub/warp-repo directory structure under a
// temp dir and returns both the temp dir and the warp repo path.
func makeWarpRepo(t *testing.T) (parentDir, warpRepoDir string) {
	t.Helper()
	parentDir = t.TempDir()
	warpRepoDir = filepath.Join(parentDir, hubName, warpDirName)
	if err := os.MkdirAll(filepath.Join(warpRepoDir, ".git", "info"), 0o755); err != nil {
		t.Fatalf("create warp repo dir: %v", err)
	}
	return parentDir, warpRepoDir
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

// TestRunSuite_HubAbsent verifies that runSuite returns an error when the Hub warp subdirectory
// does not exist.
func TestRunSuite_HubAbsent(t *testing.T) {
	parentDir := t.TempDir()

	restore := stubSuiteSeams(t, "", "", func(dir, claude, instruction, binDir string) int {
		t.Error("launchAgent should not be called when Hub is absent")
		return 1
	})
	defer restore()

	err := runSuite(parentDir, "", "", mainSuite)
	if err == nil {
		t.Fatal("runSuite should return error when Hub warp subdir is absent")
	}
	if !strings.Contains(err.Error(), "sandbox/build.cmd") {
		t.Errorf("error should mention 'sandbox/build.cmd'; got %q", err.Error())
	}
}

// TestRunSuite_LaunchInvocation verifies that runSuite calls launchAgent with the correct
// arguments.
func TestRunSuite_LaunchInvocation(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	var gotDir, gotClaude, gotInstruction, gotBinDir string
	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		gotDir = dir
		gotClaude = claude
		gotInstruction = instruction
		gotBinDir = binDir
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", mainSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if gotDir != warpRepoDir {
		t.Errorf("launchAgent dir = %q; want %q", gotDir, warpRepoDir)
	}
	if gotClaude != fakeClaude {
		t.Errorf("launchAgent claude = %q; want %q", gotClaude, fakeClaude)
	}
	if gotInstruction != mainSuite.instruction {
		t.Errorf("launchAgent instruction = %q; want %q", gotInstruction, mainSuite.instruction)
	}
	if gotBinDir != "" {
		t.Errorf("launchAgent binDir = %q; want empty for a prod resolution", gotBinDir)
	}
}

// TestRunSuite_DevBinaryPrependsBinDir verifies that runSuite passes the dev binary's directory as
// launchAgent's binDir when a dev binary exists.
func TestRunSuite_DevBinaryPrependsBinDir(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	devBinDir := filepath.Join(parentDir, ".dev-bin")
	if err := os.MkdirAll(devBinDir, 0o755); err != nil {
		t.Fatalf("mkdir dev-bin dir: %v", err)
	}
	devLyx := filepath.Join(devBinDir, "lyx")
	if err := os.WriteFile(devLyx, []byte("fake dev lyx binary"), 0o755); err != nil {
		t.Fatalf("write fake dev lyx: %v", err)
	}

	oldDevBinPath := devBinPath
	defer func() { devBinPath = oldDevBinPath }()
	devBinPath = func() (string, error) { return devLyx, nil }

	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return fakeClaude, nil
		}
		t.Errorf("unexpected lookPath call for %q; a resolvable dev binary should skip the PATH fallback", name)
		return "", fmt.Errorf("not found on PATH: %s", name)
	}

	var gotBinDir string
	stubReedDownNoop(t)
	oldLaunchAgent := launchAgent
	defer func() { launchAgent = oldLaunchAgent }()
	launchAgent = func(dir, claude, instruction, binDir string) int {
		gotBinDir = binDir
		return 0
	}

	if err := runSuite(parentDir, "", "", mainSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if gotBinDir != devBinDir {
		t.Errorf("launchAgent binDir = %q; want %q", gotBinDir, devBinDir)
	}
}

// TestRunSuite_Overrides verifies that runSuite honours -claude and -prompt override arguments.
func TestRunSuite_Overrides(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)

	customClaude := filepath.Join(parentDir, "custom-claude.exe")
	customPrompt := "Do something entirely custom."

	oldDevBinPath := devBinPath
	defer func() { devBinPath = oldDevBinPath }()
	devBinPath = func() (string, error) { return filepath.Join(t.TempDir(), "lyx"), nil }

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
	launchAgent = func(dir, claude, instruction, binDir string) int {
		gotClaude = claude
		gotInstruction = instruction
		return 0
	}

	if err := runSuite(parentDir, customClaude, customPrompt, mainSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if gotClaude != customClaude {
		t.Errorf("launchAgent claude = %q; want %q", gotClaude, customClaude)
	}
	if gotInstruction != customPrompt {
		t.Errorf("launchAgent instruction = %q; want %q", gotInstruction, customPrompt)
	}
}

// TestRunSuite_NonZeroLaunchTolerated verifies that a non-zero exit code from launchAgent is
// tolerated (interactive sessions are expected to exit manually).
func TestRunSuite_NonZeroLaunchTolerated(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		return 2
	})
	defer restore()

	if err := runSuite(parentDir, "", "", mainSuite); err != nil {
		t.Fatalf("runSuite should tolerate a non-zero interactive exit; got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parentDir, ".scratch")); !os.IsNotExist(err) {
		t.Errorf(".scratch should not be created by runSuite; stat err = %v", err)
	}
}

// TestRunSuite_ClaudeNotFound verifies that runSuite returns an error when claude cannot be
// resolved from PATH.
func TestRunSuite_ClaudeNotFound(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)

	oldDevBinPath := devBinPath
	defer func() { devBinPath = oldDevBinPath }()
	devBinPath = func() (string, error) { return filepath.Join(t.TempDir(), "lyx"), nil }

	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(name string) (string, error) {
		if name == "lyx" {
			return fakeLyx, nil
		}
		return "", fmt.Errorf("executable file not found in %%PATH%%")
	}

	oldLaunchAgent := launchAgent
	defer func() { launchAgent = oldLaunchAgent }()
	launchAgent = func(dir, claude, instruction, binDir string) int {
		t.Error("launchAgent should not be called when claude is not found")
		return 1
	}

	err := runSuite(parentDir, "", "", mainSuite)
	if err == nil {
		t.Fatal("runSuite should return error when claude is not on PATH")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should mention 'claude'; got %q", err.Error())
	}
}

// TestRunSuite_StaleReportRemoved verifies that runSuite removes a prior sandbox-report.json before
// launching the agent.
func TestRunSuite_StaleReportRemoved(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	// Pre-create a stale report from a prior run.
	stalePath := filepath.Join(warpRepoDir, reportFileName)
	if err := os.WriteFile(stalePath, []byte(`{"source": "sandbox-report", "items": [{"ref": "S0", "title": "stale", "body": "stale"}]}`), 0o644); err != nil {
		t.Fatalf("write stale report: %v", err)
	}

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
			t.Errorf("stale report should be removed before launch; stat err = %v", statErr)
		}
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", mainSuite); err != nil {
		t.Fatalf("runSuite should return nil; got error: %v", err)
	}
	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stale report should have been removed before launch; stat err = %v", statErr)
	}
}

// TestRunSuite_ExcludesReport verifies that runSuite registers sandbox-report.json in
// .git/info/exclude.
func TestRunSuite_ExcludesReport(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", mainSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}

	excludePath := filepath.Join(warpRepoDir, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read .git/info/exclude: %v", err)
	}
	for _, entry := range []string{mainSuite.fileName, reportFileName} {
		if !strings.Contains(string(content), entry) {
			t.Errorf(".git/info/exclude missing entry %q; got %q", entry, string(content))
		}
	}
}

// TestRunSuite_ReedSpec_WritesReedFile verifies that runSuite(..., reedSuite) writes
// SANDBOX-REED-SUITE.md with the fingerprint header.
func TestRunSuite_ReedSpec_WritesReedFile(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", reedSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}

	reedPath := filepath.Join(warpRepoDir, reedSuite.fileName)
	content, err := os.ReadFile(reedPath)
	if err != nil {
		t.Fatalf("read %s: %v", reedSuite.fileName, err)
	}
	if !strings.Contains(string(content), "Binary under test") {
		t.Errorf("%s missing fingerprint header; got %q", reedSuite.fileName, string(content))
	}
	if !strings.Contains(string(content), reedSandboxSuiteMD) {
		t.Errorf("%s does not contain the embedded reed doc body", reedSuite.fileName)
	}

	if _, err := os.Stat(filepath.Join(warpRepoDir, mainSuite.fileName)); !os.IsNotExist(err) {
		t.Errorf("%s should not be written by a reedSuite run; stat err = %v", mainSuite.fileName, err)
	}
}

// TestRunSuite_ReedSpec_ExcludesFiles verifies that a reedSuite run registers its files in
// .git/info/exclude.
func TestRunSuite_ReedSpec_ExcludesFiles(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", reedSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}

	excludePath := filepath.Join(warpRepoDir, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read .git/info/exclude: %v", err)
	}
	for _, entry := range []string{reedSuite.fileName, reportFileName} {
		if !strings.Contains(string(content), entry) {
			t.Errorf(".git/info/exclude missing entry %q; got %q", entry, string(content))
		}
	}
}

// TestRunSuite_ReedSpec_DeletesStaleReport verifies that a reedSuite run deletes stale
// sandbox-report.json before launching the agent.
func TestRunSuite_ReedSpec_DeletesStaleReport(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	stalePath := filepath.Join(warpRepoDir, reportFileName)
	if err := os.WriteFile(stalePath, []byte(`{"source": "sandbox-report", "items": [{"ref": "M0", "title": "stale", "body": "stale"}]}`), 0o644); err != nil {
		t.Fatalf("write stale report: %v", err)
	}

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
			t.Errorf("stale report should be removed before launch; stat err = %v", statErr)
		}
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", reedSuite); err != nil {
		t.Fatalf("runSuite should return nil; got error: %v", err)
	}
	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stale report should have been removed before launch; stat err = %v", statErr)
	}
}

// TestRunSuite_ReedSpec_DefaultInstruction verifies that a reedSuite run passes the reed default
// instruction to launchAgent.
func TestRunSuite_ReedSpec_DefaultInstruction(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	var gotInstruction string
	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		gotInstruction = instruction
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", reedSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if gotInstruction != reedSuite.instruction {
		t.Errorf("launchAgent instruction = %q; want %q", gotInstruction, reedSuite.instruction)
	}
	if gotInstruction != "Read ./SANDBOX-REED-SUITE.md and follow the instructions in it exactly." {
		t.Errorf("launchAgent instruction = %q; want the literal reed default", gotInstruction)
	}
}

// TestRunSuite_ReedSpec_PromptOverride verifies that a -prompt override reaches launchAgent
// verbatim for a reedSuite run.
func TestRunSuite_ReedSpec_PromptOverride(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")
	customPrompt := "Do the reed thing entirely differently."

	var gotInstruction string
	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		gotInstruction = instruction
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", customPrompt, reedSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if gotInstruction != customPrompt {
		t.Errorf("launchAgent instruction = %q; want override %q", gotInstruction, customPrompt)
	}
}

// TestRunSuite_ShuttleSpec_WritesShuttleFile verifies that runSuite writes SANDBOX-SHUTTLE-SUITE.md
// with the fingerprint header.
func TestRunSuite_ShuttleSpec_WritesShuttleFile(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", shuttleSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}

	shuttlePath := filepath.Join(warpRepoDir, shuttleSuite.fileName)
	content, err := os.ReadFile(shuttlePath)
	if err != nil {
		t.Fatalf("read %s: %v", shuttleSuite.fileName, err)
	}
	if !strings.Contains(string(content), "Binary under test") {
		t.Errorf("%s missing fingerprint header; got %q", shuttleSuite.fileName, string(content))
	}
	if !strings.Contains(string(content), shuttleSandboxSuiteMD) {
		t.Errorf("%s does not contain the embedded shuttle doc body", shuttleSuite.fileName)
	}

	if _, err := os.Stat(filepath.Join(warpRepoDir, mainSuite.fileName)); !os.IsNotExist(err) {
		t.Errorf("%s should not be written by a shuttleSuite run; stat err = %v", mainSuite.fileName, err)
	}
	if _, err := os.Stat(filepath.Join(warpRepoDir, reedSuite.fileName)); !os.IsNotExist(err) {
		t.Errorf("%s should not be written by a shuttleSuite run; stat err = %v", reedSuite.fileName, err)
	}
}

// TestRunSuite_ShuttleSpec_ExcludesFiles verifies that a shuttleSuite run registers its files in
// .git/info/exclude.
func TestRunSuite_ShuttleSpec_ExcludesFiles(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", shuttleSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}

	excludePath := filepath.Join(warpRepoDir, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read .git/info/exclude: %v", err)
	}
	for _, entry := range []string{shuttleSuite.fileName, reportFileName} {
		if !strings.Contains(string(content), entry) {
			t.Errorf(".git/info/exclude missing entry %q; got %q", entry, string(content))
		}
	}
}

// TestRunSuite_ShuttleSpec_DeletesStaleReport verifies that a shuttleSuite run deletes stale
// sandbox-report.json before launching the agent.
func TestRunSuite_ShuttleSpec_DeletesStaleReport(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	stalePath := filepath.Join(warpRepoDir, reportFileName)
	if err := os.WriteFile(stalePath, []byte(`{"source": "sandbox-report", "items": [{"ref": "SH0", "title": "stale", "body": "stale"}]}`), 0o644); err != nil {
		t.Fatalf("write stale report: %v", err)
	}

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
			t.Errorf("stale report should be removed before launch; stat err = %v", statErr)
		}
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", shuttleSuite); err != nil {
		t.Fatalf("runSuite should return nil; got error: %v", err)
	}
	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stale report should have been removed before launch; stat err = %v", statErr)
	}
}

// TestRunSuite_ShuttleSpec_DefaultInstruction verifies that a shuttleSuite run passes the shuttle
// default instruction to launchAgent.
func TestRunSuite_ShuttleSpec_DefaultInstruction(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	var gotInstruction string
	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		gotInstruction = instruction
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", "", shuttleSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if gotInstruction != shuttleSuite.instruction {
		t.Errorf("launchAgent instruction = %q; want %q", gotInstruction, shuttleSuite.instruction)
	}
	if gotInstruction != "Read ./SANDBOX-SHUTTLE-SUITE.md and follow the instructions in it exactly." {
		t.Errorf("launchAgent instruction = %q; want the literal shuttle default", gotInstruction)
	}
}

// TestRunSuite_ShuttleSpec_PromptOverride verifies that a -prompt override reaches launchAgent
// verbatim for a shuttleSuite run.
func TestRunSuite_ShuttleSpec_PromptOverride(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")
	customPrompt := "Do the shuttle thing entirely differently."

	var gotInstruction string
	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		gotInstruction = instruction
		return 0
	})
	defer restore()

	if err := runSuite(parentDir, "", customPrompt, shuttleSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if gotInstruction != customPrompt {
		t.Errorf("launchAgent instruction = %q; want override %q", gotInstruction, customPrompt)
	}
}

// TestSuiteSpecs_ReedTeardownFlag verifies which suites have the reedTeardown flag.
func TestSuiteSpecs_ReedTeardownFlag(t *testing.T) {
	if mainSuite.reedTeardown {
		t.Error("mainSuite.reedTeardown = true; the core suite boots no reed substrate")
	}
	for _, spec := range []suiteSpec{reedSuite, shuttleSuite, burlerSuite} {
		if !spec.reedTeardown {
			t.Errorf("%s: reedTeardown = false; live-reed suites must tear their substrate down", spec.fileName)
		}
	}
}

// TestRunSuite_BurlerSpec_ReedTeardownAfterAgent verifies that a burlerSuite run calls reedDown
// exactly once, after the agent session ends.
func TestRunSuite_BurlerSpec_ReedTeardownAfterAgent(t *testing.T) {
	parentDir, warpRepoDir := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	agentDone := false
	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		agentDone = true
		return 0
	})
	defer restore()

	var gotDir, gotLyx string
	teardownCalls := 0
	reedDown = func(dir, lyx string) error {
		if !agentDone {
			t.Error("reedDown called before launchAgent returned")
		}
		teardownCalls++
		gotDir, gotLyx = dir, lyx
		return nil
	}

	if err := runSuite(parentDir, "", "", burlerSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if teardownCalls != 1 {
		t.Fatalf("reedDown called %d times; want exactly 1", teardownCalls)
	}
	if gotDir != warpRepoDir {
		t.Errorf("reedDown dir = %q; want %q", gotDir, warpRepoDir)
	}
	if gotLyx != fakeLyx {
		t.Errorf("reedDown lyx = %q; want %q", gotLyx, fakeLyx)
	}
}

// TestRunSuite_MainSpec_NoReedTeardown verifies that a mainSuite run never calls reedDown.
func TestRunSuite_MainSpec_NoReedTeardown(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		return 0
	})
	defer restore()

	reedDown = func(dir, lyx string) error {
		t.Error("reedDown should not be called for mainSuite")
		return nil
	}

	if err := runSuite(parentDir, "", "", mainSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
}

// TestRunSuite_ReedTeardownFailureTolerated verifies that a reedDown error does not turn a
// completed session into a launcher failure.
func TestRunSuite_ReedTeardownFailureTolerated(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		return 0
	})
	defer restore()

	reedDown = func(dir, lyx string) error {
		return fmt.Errorf("lyx reed down: exit status 1")
	}

	if err := runSuite(parentDir, "", "", burlerSuite); err != nil {
		t.Fatalf("runSuite should tolerate a reedDown failure; got error: %v", err)
	}
}

// TestRunSuite_ReedTeardownRunsOnNonZeroAgentExit verifies that teardown runs regardless of the
// agent's exit code.
func TestRunSuite_ReedTeardownRunsOnNonZeroAgentExit(t *testing.T) {
	parentDir, _ := makeWarpRepo(t)
	fakeLyx := makeFakeLyx(t, parentDir)
	fakeClaude := filepath.Join(parentDir, "claude.exe")

	restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
		return 2
	})
	defer restore()

	teardownCalls := 0
	reedDown = func(dir, lyx string) error {
		teardownCalls++
		return nil
	}

	if err := runSuite(parentDir, "", "", burlerSuite); err != nil {
		t.Fatalf("runSuite error: %v", err)
	}
	if teardownCalls != 1 {
		t.Errorf("reedDown called %d times after non-zero agent exit; want exactly 1", teardownCalls)
	}
}

// TestIsCharDevice_RegularFile verifies that isCharDevice reports false for a regular file.
func TestIsCharDevice_RegularFile(t *testing.T) {
	f, err := os.Open(makeFakeLyx(t, t.TempDir()))
	if err != nil {
		t.Fatalf("open temp file: %v", err)
	}
	defer f.Close()
	if isCharDevice(f) {
		t.Error("isCharDevice(regular file) = true; want false")
	}
}

// TestLaunchAgent_NonInteractiveWarning verifies that launchAgent prints nonInteractiveWarning when
// stdio is not attached to a console.
func TestLaunchAgent_NonInteractiveWarning(t *testing.T) {
	missingClaude := filepath.Join(t.TempDir(), "claude.exe")

	oldInteractive := interactiveStdio
	defer func() { interactiveStdio = oldInteractive }()

	t.Run("warns_when_detached", func(t *testing.T) {
		interactiveStdio = func() bool { return false }
		got := captureStderr(t, func() {
			if code := launchAgent(t.TempDir(), missingClaude, "instr", ""); code != 1 {
				t.Errorf("launchAgent = %d; want 1 for a missing binary", code)
			}
		})
		if !strings.Contains(got, "not an attached console") {
			t.Errorf("stderr missing the non-interactive warning; got %q", got)
		}
	})

	t.Run("silent_when_attached", func(t *testing.T) {
		interactiveStdio = func() bool { return true }
		got := captureStderr(t, func() {
			launchAgent(t.TempDir(), missingClaude, "instr", "")
		})
		if strings.Contains(got, "not an attached console") {
			t.Errorf("stderr should not carry the warning when stdio is attached; got %q", got)
		}
	})
}
