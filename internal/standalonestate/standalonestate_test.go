// standalonestate_test.go drives both platform rows of derive through its unexported seam, so
// runtime.GOOS being a compile-time constant never leaves one row unexercised in CI.

package standalonestate

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDerive_WindowsRow pins that the Windows branch consults localAppData for stateDir, leaving
// the exact separator to path/filepath rather than a literal backslash string.
func TestDerive_WindowsRow(t *testing.T) {
	stateDir, hash8, err := derive("windows", "/localappdata", "", "", "/abs/target")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	want := filepath.Join("/localappdata", "lyx", hash8)
	if stateDir != want {
		t.Errorf("derive() stateDir = %q; want %q", stateDir, want)
	}
}

// TestDerive_NonWindowsRow_XDGStateHomeSet pins that the non-Windows branch consults
// xdgStateHome for stateDir and never consults localAppData.
func TestDerive_NonWindowsRow_XDGStateHomeSet(t *testing.T) {
	stateDir, hash8, err := derive("linux", "/should-be-ignored", "/xdgstate", "", "/abs/target")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	want := filepath.Join("/xdgstate", "lyx", hash8)
	if stateDir != want {
		t.Errorf("derive() stateDir = %q; want %q", stateDir, want)
	}
}

// TestDerive_NonWindowsRow_XDGStateHomeUnset pins the fallback to home/.local/state/lyx/hash8
// when xdgStateHome is empty.
func TestDerive_NonWindowsRow_XDGStateHomeUnset(t *testing.T) {
	stateDir, hash8, err := derive("linux", "", "", "/home/user", "/abs/target")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	want := filepath.Join("/home/user", ".local", "state", "lyx", hash8)
	if stateDir != want {
		t.Errorf("derive() stateDir = %q; want %q", stateDir, want)
	}
}

// TestDerive_WindowsRow_MissingLocalAppData pins that an empty localAppData on the Windows
// branch is an error.
func TestDerive_WindowsRow_MissingLocalAppData(t *testing.T) {
	_, _, err := derive("windows", "", "", "/home/user", "/abs/target")
	if err == nil {
		t.Error("derive() error = nil; want non-nil for empty localAppData on windows")
	}
}

// TestDerive_NonWindowsRow_MissingStateHome pins that an empty xdgStateHome and an empty home
// together are an error on the non-Windows branch.
func TestDerive_NonWindowsRow_MissingStateHome(t *testing.T) {
	_, _, err := derive("linux", "/localappdata", "", "", "/abs/target")
	if err == nil {
		t.Error("derive() error = nil; want non-nil for empty xdgStateHome and empty home")
	}
}

// TestDerive_CaseFold pins that two spellings of one path differing only in case hash identically
// under goos == "windows" and differently under goos == "linux".
func TestDerive_CaseFold(t *testing.T) {
	_, lowerWin, err := derive("windows", "/localappdata", "", "", "/abs/Target")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	_, upperWin, err := derive("windows", "/localappdata", "", "", "/abs/TARGET")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	if lowerWin != upperWin {
		t.Errorf("derive() hash8 windows = %q, %q; want equal (case-folded)", lowerWin, upperWin)
	}

	_, lowerLinux, err := derive("linux", "", "", "/home/user", "/abs/Target")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	_, upperLinux, err := derive("linux", "", "", "/home/user", "/abs/TARGET")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	if lowerLinux == upperLinux {
		t.Errorf("derive() hash8 linux = %q, %q; want different (case-sensitive)", lowerLinux, upperLinux)
	}
}

// TestDerive_RelativeTargetRejected pins that a relative target is rejected under both goos
// values, and that the outcome does not vary with the test process' working directory -- the
// function must never consult it.
func TestDerive_RelativeTargetRejected(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			_, _, err := derive(goos, "/localappdata", "/xdgstate", "/home/user", "relative/target")
			if err == nil {
				t.Errorf("derive(%q) error = nil; want non-nil for relative target", goos)
			}

			originalWD, wdErr := os.Getwd()
			if wdErr != nil {
				t.Fatalf("os.Getwd() error = %v", wdErr)
			}
			t.Chdir(t.TempDir())
			_, _, errAfterChdir := derive(goos, "/localappdata", "/xdgstate", "/home/user", "relative/target")
			t.Chdir(originalWD)
			if (errAfterChdir == nil) != (err == nil) {
				t.Errorf("derive(%q) outcome changed with cwd: before=%v, after=%v", goos, err, errAfterChdir)
			}
		})
	}
}

// TestDerive_Hash8Shape pins that hash8 is exactly 8 characters, every one a lowercase hex digit.
func TestDerive_Hash8Shape(t *testing.T) {
	_, hash8, err := derive("linux", "", "", "/home/user", "/abs/target")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	if len(hash8) != 8 {
		t.Fatalf("derive() hash8 = %q; want length 8, got %d", hash8, len(hash8))
	}
	for _, r := range hash8 {
		isLowerHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isLowerHex {
			t.Errorf("derive() hash8 = %q; contains non-lowercase-hex character %q", hash8, r)
		}
	}
}

// TestDerive_Stability pins that the same input yields the same hash8 across repeated calls,
// the property the whole resumability story rests on.
func TestDerive_Stability(t *testing.T) {
	_, first, err := derive("linux", "", "", "/home/user", "/abs/target")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	_, second, err := derive("linux", "", "", "/home/user", "/abs/target")
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	if first != second {
		t.Errorf("derive() hash8 not stable: %q != %q", first, second)
	}
}

// TestDerive_CreatesNothingOnDisk pins that derive creates nothing on disk -- no directory, no
// file -- by pointing it at a path under t.TempDir() and asserting the returned stateDir does
// not exist afterward.
func TestDerive_CreatesNothingOnDisk(t *testing.T) {
	target := t.TempDir()
	home := t.TempDir()
	stateDir, _, err := derive("linux", "", "", home, target)
	if err != nil {
		t.Fatalf("derive() error = %v; want nil", err)
	}
	if _, statErr := os.Stat(stateDir); !os.IsNotExist(statErr) {
		t.Errorf("derive() stateDir %q exists on disk or stat failed unexpectedly: %v", stateDir, statErr)
	}
}
