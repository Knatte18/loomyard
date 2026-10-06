// standalonestate_test.go drives both platform rows of derive through its unexported seam, so
// runtime.GOOS being a compile-time constant never leaves one row unexercised in CI.
// A test that changes the working directory with t.Chdir cannot run in parallel; the one such test
// names that process-global state, and every other test here runs in parallel.

package standalonestate

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDerive_StateDir pins which base each platform row of derive joins "lyx" and hash8 onto, and
// which environments it refuses.
// The Windows branch consults localAppData, the leaf separator being left to path/filepath rather
// than a literal backslash string; the non-Windows branch consults xdgStateHome, then home, and
// never consults localAppData.
//
// It is also the regression guard for the R4 review's R4-08: derive validated target for
// absoluteness but never the environment-supplied base it joined onto, so a relative XDG_STATE_HOME
// (or LOCALAPPDATA, or home) produced a RELATIVE stateDir -- which the standalone CLIs then resolved
// against the process working directory, i.e. the operator's own repository, writing lyx state and
// trace logs inside the very checkout standalonegeom.LogsDir exists to keep them out of.
// Reproduced live as `XDG_STATE_HOME=.relstate lyx burler run` creating <repo>/.relstate/lyx/<hash8>/.
// The three bases answer a relative value differently because their specifications do: a relative
// XDG_STATE_HOME is ignored per the XDG Base Directory specification, while a relative LOCALAPPDATA
// or home has no fallback left and is refused.
// Every surviving row asserts an ABSOLUTE stateDir, which is the property that actually matters.
func TestDerive_StateDir(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		goos         string
		localAppData string
		xdgStateHome string
		home         string
		wantErr      bool
		wantStateDir func(hash8 string) string
	}{
		{
			name:         "windows consults localAppData",
			goos:         "windows",
			localAppData: "/localappdata",
			wantStateDir: func(hash8 string) string { return filepath.Join("/localappdata", "lyx", hash8) },
		},
		{
			name:         "non-windows consults xdgStateHome and never localAppData",
			goos:         "linux",
			localAppData: "/should-be-ignored",
			xdgStateHome: "/xdgstate",
			wantStateDir: func(hash8 string) string { return filepath.Join("/xdgstate", "lyx", hash8) },
		},
		{
			name: "non-windows falls back to home/.local/state when xdgStateHome is empty",
			goos: "linux",
			home: "/home/user",
			wantStateDir: func(hash8 string) string {
				return filepath.Join("/home/user", ".local", "state", "lyx", hash8)
			},
		},
		{
			name:    "windows with an empty localAppData is an error",
			goos:    "windows",
			home:    "/home/user",
			wantErr: true,
		},
		{
			name:         "non-windows with an empty xdgStateHome and an empty home is an error",
			goos:         "linux",
			localAppData: "/localappdata",
			wantErr:      true,
		},
		{
			name:         "relative XDG_STATE_HOME ignored in favour of home",
			goos:         "linux",
			xdgStateHome: ".relstate",
			home:         "/home/user",
			wantStateDir: func(hash8 string) string {
				return filepath.Join("/home/user", ".local", "state", "lyx", hash8)
			},
		},
		{
			name:         "dot-relative XDG_STATE_HOME ignored in favour of home",
			goos:         "linux",
			xdgStateHome: "./relstate/nested",
			home:         "/home/user",
			wantStateDir: func(hash8 string) string {
				return filepath.Join("/home/user", ".local", "state", "lyx", hash8)
			},
		},
		{
			name:         "relative XDG_STATE_HOME with no home left to fall back on",
			goos:         "linux",
			xdgStateHome: ".relstate",
			wantErr:      true,
		},
		{
			name:    "relative home refused",
			goos:    "linux",
			home:    "relative/home",
			wantErr: true,
		},
		{
			name:         "relative LOCALAPPDATA refused",
			goos:         "windows",
			localAppData: "relative/appdata",
			home:         "/home/user",
			wantErr:      true,
		},
		{
			name:         "absolute XDG_STATE_HOME still honoured",
			goos:         "linux",
			xdgStateHome: "/xdgstate",
			home:         "/home/user",
			wantStateDir: func(hash8 string) string { return filepath.Join("/xdgstate", "lyx", hash8) },
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			stateDir, hash8, err := derive(c.goos, c.localAppData, c.xdgStateHome, c.home, "/abs/target")
			if c.wantErr {
				if err == nil {
					t.Fatalf("derive() error = nil, stateDir = %q; want non-nil error", stateDir)
				}
				return
			}
			if err != nil {
				t.Fatalf("derive() error = %v; want nil", err)
			}
			if !filepath.IsAbs(stateDir) {
				t.Fatalf("derive() stateDir = %q; want an absolute path", stateDir)
			}
			if want := c.wantStateDir(hash8); stateDir != want {
				t.Errorf("derive() stateDir = %q; want %q", stateDir, want)
			}
		})
	}
}

// TestDerive_Hash8 pins the identity half of derive: hash8 is eight lowercase hex digits, stable
// across calls, taken over exactly what Normalize returns, and case-folded under windows only.
//
//testtiming:keep pins hash8's shape, stability, Normalize input and per-platform case folding, which the state-directory table covering its blocks never reads
func TestDerive_Hash8(t *testing.T) {
	t.Parallel()

	t.Run("shape is eight lowercase hex digits", func(t *testing.T) {
		t.Parallel()

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
	})

	t.Run("stable across repeated calls", func(t *testing.T) {
		t.Parallel()

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
	})

	// This is the property R4-24's fix rests on: a caller that spells the target through Normalize
	// to build a session name is spelling it the way derive spelled it for the socket key and state
	// directory, so the two halves of a standalone identity can never disagree.
	t.Run("hash input is exactly what Normalize returns", func(t *testing.T) {
		t.Parallel()

		target := filepath.Join(string(filepath.Separator), "abs", "..", "abs", "target")
		_, rawHash8, err := derive("linux", "", "", "/home/user", target)
		if err != nil {
			t.Fatalf("derive(%q) error = %v; want nil", target, err)
		}
		_, normalizedHash8, err := derive("linux", "", "", "/home/user", Normalize(target))
		if err != nil {
			t.Fatalf("derive(Normalize(%q)) error = %v; want nil", target, err)
		}
		if rawHash8 != normalizedHash8 {
			t.Errorf("derive() hash8 raw = %q, pre-normalized = %q; want equal", rawHash8, normalizedHash8)
		}
	})

	t.Run("case folds under windows and stays case-sensitive under linux", func(t *testing.T) {
		t.Parallel()

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
	})
}

// TestNormalize_AbsentTargetFallsBackToClean pins that a target that does not exist on disk yet
// normalizes to its cleaned self rather than failing: an unborn directory still needs a stable
// identity, and this is the branch every hermetic test of the geometry builders relies on.
//
//testtiming:keep pins that Normalize cleans a target absent from disk instead of failing, which the state-directory table covering its blocks never reads
func TestNormalize_AbsentTargetFallsBackToClean(t *testing.T) {
	t.Parallel()

	absent := filepath.Join(t.TempDir(), "never", "..", "never", "created")

	if got, want := Normalize(absent), filepath.Clean(absent); got != want {
		t.Errorf("Normalize(%q) = %q; want %q", absent, got, want)
	}
}

// TestRelativeInputs_NeverConsultTheWorkingDirectory pins that a relative input's outcome does not
// vary with the test process' working directory, because "never consults the cwd" is only
// demonstrated by the answer not moving when the cwd does.
// A relative target is rejected under both goos values; a relative environment base is ignored
// rather than resolved; and Normalize cleans a relative target instead of resolving it, the one
// branch of Normalize that exists purely to protect the Standalonestate Leaf Invariant
// (filepath.EvalSymlinks resolves a relative path against the process working directory).
// It changes the working directory with t.Chdir, which is process-global state, so neither it nor
// its subtests run in parallel.
func TestRelativeInputs_NeverConsultTheWorkingDirectory(t *testing.T) {
	const relative = "relative/./target"

	cases := []struct {
		name    string
		eval    func() (string, error)
		wantErr bool
		want    string
	}{
		{
			name: "relative target rejected under windows",
			eval: func() (string, error) {
				stateDir, _, err := derive("windows", "/localappdata", "/xdgstate", "/home/user", "relative/target")
				return stateDir, err
			},
			wantErr: true,
		},
		{
			name: "relative target rejected under linux",
			eval: func() (string, error) {
				stateDir, _, err := derive("linux", "/localappdata", "/xdgstate", "/home/user", "relative/target")
				return stateDir, err
			},
			wantErr: true,
		},
		{
			name: "relative environment base ignored",
			eval: func() (string, error) {
				stateDir, _, err := derive("linux", "", ".relstate", "/home/user", "/abs/target")
				return stateDir, err
			},
		},
		{
			name: "relative target cleaned by Normalize, not resolved",
			eval: func() (string, error) { return Normalize(relative), nil },
			want: filepath.Clean(relative),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before, beforeErr := c.eval()
			if (beforeErr != nil) != c.wantErr {
				t.Fatalf("outcome error = %v; want error %v", beforeErr, c.wantErr)
			}
			if c.want != "" && before != c.want {
				t.Errorf("outcome = %q; want %q", before, c.want)
			}

			t.Chdir(t.TempDir())
			after, afterErr := c.eval()
			if (afterErr != nil) != (beforeErr != nil) || after != before {
				t.Errorf("outcome changed with cwd: before=(%q, %v), after=(%q, %v)", before, beforeErr, after, afterErr)
			}
		})
	}
}

// TestDerive_SymlinkNormalization pins that derive creates nothing on disk -- no directory, no file
// -- and that a symlink and its real target produce the same hash8, skipping the symlink half when
// os.Symlink fails with a permission error so a Windows host without Developer Mode does not fail
// the suite.
func TestDerive_SymlinkNormalization(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("os.Mkdir(%q) error = %v", realDir, err)
	}

	realStateDir, realHash8, err := derive("linux", "", "", base, realDir)
	if err != nil {
		t.Fatalf("derive(realDir) error = %v; want nil", err)
	}
	if _, statErr := os.Stat(realStateDir); !os.IsNotExist(statErr) {
		t.Errorf("derive() stateDir %q exists on disk or stat failed unexpectedly: %v", realStateDir, statErr)
	}

	linkPath := filepath.Join(base, "link")
	if err := os.Symlink(realDir, linkPath); err != nil {
		if os.IsPermission(err) {
			t.Skip("os.Symlink not permitted on this host; skipping")
		}
		t.Fatalf("os.Symlink(%q, %q) error = %v", realDir, linkPath, err)
	}

	_, linkHash8, err := derive("linux", "", "", base, linkPath)
	if err != nil {
		t.Fatalf("derive(linkPath) error = %v; want nil", err)
	}
	if realHash8 != linkHash8 {
		t.Errorf("derive() hash8 real = %q, symlink = %q; want equal", realHash8, linkHash8)
	}
}
