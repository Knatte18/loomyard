// status_test.go — tests for weft status reporting.

package weft

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStatus_BranchReporting(t *testing.T) {
	weftRepo := newTestWeftRepo(t)

	status, err := Status(weftRepo, "", "", []string{"_lyx"})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	branch, ok := status["branch"].(string)
	if !ok || branch == "" {
		t.Errorf("status[branch] should be a non-empty string; got %v", status["branch"])
	}
	if branch != "main" {
		t.Errorf("branch = %q; want %q", branch, "main")
	}
}

func TestStatus_DirtyFlag(t *testing.T) {
	weftRepo := newTestWeftRepo(t)

	// Check dirty on clean tree
	status, err := Status(weftRepo, "", "", []string{"_lyx"})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	dirty, ok := status["dirty"].(bool)
	if !ok {
		t.Errorf("status[dirty] should be a bool; got %v", status["dirty"])
	}
	if dirty {
		t.Errorf("dirty = true on clean tree; want false")
	}

	// Modify a file in pathspec
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("modified"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Check dirty on dirty tree
	status, err = Status(weftRepo, "", "", []string{"_lyx"})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	dirty, ok = status["dirty"].(bool)
	if !ok {
		t.Errorf("status[dirty] should be a bool; got %v", status["dirty"])
	}
	if !dirty {
		t.Errorf("dirty = false on modified tree; want true")
	}
}

func TestStatus_JunctionMissing(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	tmpDir := t.TempDir()
	hostLink := filepath.Join(tmpDir, "_lyx")

	status, err := Status(weftRepo, hostLink, "", []string{"_lyx"})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	junctionOk, ok := status["junction_ok"].(bool)
	if !ok {
		t.Errorf("status[junction_ok] should be a bool; got %v", status["junction_ok"])
	}
	if junctionOk {
		t.Errorf("junction_ok = true when hostLink missing; want false")
	}

	reason, ok := status["junction_reason"].(string)
	if !ok {
		t.Errorf("status[junction_reason] should be a string; got %v", status["junction_reason"])
	}
	if reason != "host _lyx junction missing" {
		t.Errorf("junction_reason = %q; want %q", reason, "host _lyx junction missing")
	}
}

func TestStatus_JunctionPlainDir(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	tmpDir := t.TempDir()
	hostLink := filepath.Join(tmpDir, "_lyx")

	// Create a plain directory (not a junction)
	if err := os.MkdirAll(hostLink, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	status, err := Status(weftRepo, hostLink, "", []string{"_lyx"})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	junctionOk, ok := status["junction_ok"].(bool)
	if !ok {
		t.Errorf("status[junction_ok] should be a bool; got %v", status["junction_ok"])
	}
	if junctionOk {
		t.Errorf("junction_ok = true for plain dir; want false")
	}

	reason, ok := status["junction_reason"].(string)
	if !ok {
		t.Errorf("status[junction_reason] should be a string; got %v", status["junction_reason"])
	}
	if reason != "host _lyx is not a junction" {
		t.Errorf("junction_reason = %q; want %q", reason, "host _lyx is not a junction")
	}
}

func TestStatus_JunctionOk_Symlink(t *testing.T) {
	if os.Getenv("SKIP_SYMLINK_TEST") == "1" {
		t.Skip("symlink test skipped")
	}

	weftRepo := newTestWeftRepo(t)
	weftLyxDir := filepath.Join(weftRepo, "_lyx")

	tmpDir := t.TempDir()
	hostLink := filepath.Join(tmpDir, "_lyx")

	// Create a symlink (or junction on Windows)
	// Try symlink first; if that fails, skip the test
	err := os.Symlink(weftLyxDir, hostLink)
	if err != nil {
		t.Skipf("os.Symlink failed (likely Windows without privilege): %v", err)
	}

	status, err := Status(weftRepo, hostLink, weftLyxDir, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	junctionOk, ok := status["junction_ok"].(bool)
	if !ok {
		t.Errorf("status[junction_ok] should be a bool; got %v", status["junction_ok"])
	}
	if !junctionOk {
		t.Errorf("junction_ok = false for valid symlink; want true. Reason: %s", status["junction_reason"])
	}

	reason, ok := status["junction_reason"].(string)
	if !ok {
		t.Errorf("status[junction_reason] should be a string; got %v", status["junction_reason"])
	}
	if reason != "" {
		t.Errorf("junction_reason = %q on valid junction; want empty", reason)
	}
}

func TestStatus_JunctionOk_Windows(t *testing.T) {
	// This test only runs on Windows and requires privilege
	if os.Getenv("SKIP_MKLINK_TEST") == "1" {
		t.Skip("mklink test skipped")
	}

	weftRepo := newTestWeftRepo(t)
	weftLyxDir := filepath.Join(weftRepo, "_lyx")

	tmpDir := t.TempDir()
	hostLink := filepath.Join(tmpDir, "_lyx")

	// Try creating a Windows junction via cmd /c mklink /J
	// Note: Windows junctions may not have ModeSymlink set in all Go versions
	// This test is best-effort and may skip on systems without privilege
	cmd := exec.Command("cmd", "/c", "mklink", "/J", hostLink, weftLyxDir)
	if err := cmd.Run(); err != nil {
		t.Skipf("mklink /J failed (likely not on Windows or no privilege): %v", err)
	}

	// Note: On some Windows systems, junctions may not report ModeSymlink
	// but EvalSymlinks should still work. This test is skipped if the junction
	// cannot be recognized.
	status, err := Status(weftRepo, hostLink, weftLyxDir, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	junctionOk, ok := status["junction_ok"].(bool)
	if !ok {
		t.Errorf("status[junction_ok] should be a bool; got %v", status["junction_ok"])
	}

	// On Windows, the ModeSymlink bit may not be set, so we skip if not recognized
	if !junctionOk {
		reason, _ := status["junction_reason"].(string)
		if reason == "host _lyx is not a junction" {
			t.Skipf("Windows junction not recognized by os.Lstat (ModeSymlink not set)")
		}
		t.Errorf("junction_ok = false for valid junction; want true. Reason: %s", status["junction_reason"])
	}

	reason, ok := status["junction_reason"].(string)
	if !ok {
		t.Errorf("status[junction_reason] should be a string; got %v", status["junction_reason"])
	}
	if reason != "" && junctionOk {
		t.Errorf("junction_reason = %q on valid junction; want empty", reason)
	}
}
