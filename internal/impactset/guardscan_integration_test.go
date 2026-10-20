//go:build integration

// guardscan_integration_test.go drives ModuleGuardTests over a real module, whose package listing spawns `go list`.

package impactset

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestModuleGuardTests_OverAModule walks one module through the scan and each way it can fail.
// The steps share the module and run in order, so the test is parallel as a whole and no step is.
func TestModuleGuardTests_OverAModule(t *testing.T) {
	t.Parallel()

	moduleDir := t.TempDir()
	writeRepoFile(t, moduleDir, "go.mod", "module example.com/m\n\ngo 1.26\n")
	writeRepoFile(t, moduleDir, "g/g.go", "package g\n")
	writeRepoFile(t, moduleDir, "g/g_test.go", "package g\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestGuard(t *testing.T) {}\n")
	writeRepoFile(t, moduleDir, "p/p.go", "package p\n")
	writeRepoFile(t, moduleDir, "p/p_test.go", "package p\n\nimport \"testing\"\n\nfunc TestPlain(t *testing.T) {}\n")

	t.Run("the guards of every package are keyed by directory", func(t *testing.T) {
		got, err := ModuleGuardTests(moduleDir)
		if err != nil {
			t.Fatalf("ModuleGuardTests() error = %v; want nil", err)
		}
		if !slices.Equal(got["g"], []string{"TestGuard"}) || len(got["p"]) != 0 {
			t.Errorf("ModuleGuardTests() = %v; want {g: [TestGuard]}", got)
		}
	})

	t.Run("an unreadable test file is a parse error naming it with no line", func(t *testing.T) {
		skipAsRoot(t)
		makeUnreadable(t, filepath.Join(moduleDir, "p", "p_test.go"))

		_, err := ModuleGuardTests(moduleDir)
		var scanErr *GuardScanError
		if !errors.As(err, &scanErr) {
			t.Fatalf("ModuleGuardTests() error = %v; want a *GuardScanError", err)
		}
		if scanErr.Kind != GuardParse || scanErr.File != "p/p_test.go" || scanErr.Line != 0 {
			t.Errorf("ModuleGuardTests() error = %+v; want kind %q, file p/p_test.go, line 0", scanErr, GuardParse)
		}
	})

	t.Run("an unreadable package directory is a plain error naming the module and the directory", func(t *testing.T) {
		skipAsRoot(t)
		writeRepoFile(t, moduleDir, "c/c.go", "package c\n")
		makeUnreadable(t, filepath.Join(moduleDir, "c"))

		_, err := ModuleGuardTests(moduleDir)
		if err == nil || IsGuardScanError(err) || !strings.Contains(err.Error(), moduleDir) || !strings.Contains(err.Error(), "open c:") {
			t.Errorf("ModuleGuardTests() error = %v; want a plain error naming %s and c", err, moduleDir)
		}
	})

	t.Run("a directory with no go.mod is an error naming it", func(t *testing.T) {
		bare := t.TempDir()
		if _, err := ModuleGuardTests(bare); err == nil || !strings.Contains(err.Error(), bare) {
			t.Errorf("ModuleGuardTests(%s) error = %v; want an error naming the directory", bare, err)
		}
	})
}

// skipAsRoot skips a step that relies on a permission denial, which the root user never meets and `chmod` never causes on Windows.
func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("the platform or user reads any file")
	}
}

// makeUnreadable removes every permission from path and restores them at cleanup so the temp directory can be removed.
func makeUnreadable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, info.Mode().Perm()) })
}
