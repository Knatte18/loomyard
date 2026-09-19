// launcher_content_test.go tests the pure, build-tag-free launcher content builder for both the
// Windows (.cmd) and non-Windows (.sh) branches.
// Because launcherScript and launcherExt take goos as a parameter rather than reading runtime.GOOS,
// both branches are exercised on any warp, including this Windows dev box.
// The fabric-checkout case asserts fabric's own checkout script content ("fabric checkout" lyx
// args), and the run case asserts the run launcher's own command line ("loom start" lyx args).
//
// TestWriteLaunchers_RunScriptContentAndFilename additionally drives writeLaunchers itself, pinning
// the launcher-filename-unchanged decision's two halves together: the written run launcher's filename
// stays "run"+ext while its content names "loom start".

package fabricengine

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLauncherExt(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{"windows", "windows", ".cmd"},
		{"linux", "linux", ".sh"},
		{"darwin", "darwin", ".sh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := launcherExt(tt.goos)
			if got != tt.want {
				t.Errorf("launcherExt(%q) = %q; want %q", tt.goos, got, tt.want)
			}
		})
	}
}

func TestLauncherScript(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		climbRel string
		lyxArgs  string
		want     string
		wantMode uint32
	}{
		{
			name:     "windows ide spawn empty climb",
			goos:     "windows",
			climbRel: "",
			lyxArgs:  "ide spawn myslug",
			want:     "@cd /d \"%~dp0\" && lyx ide spawn myslug\r\n",
			wantMode: 0o644,
		},
		{
			name:     "windows fabric checkout nested climb",
			goos:     "windows",
			climbRel: "../../myslug/sub",
			lyxArgs:  "fabric checkout",
			want:     "@cd /d \"%~dp0..\\..\\myslug\\sub\" && lyx fabric checkout\r\n",
			wantMode: 0o644,
		},
		{
			name:     "windows ide menu nested climb",
			goos:     "windows",
			climbRel: "../sub",
			lyxArgs:  "ide menu",
			want:     "@cd /d \"%~dp0..\\sub\" && lyx ide menu\r\n",
			wantMode: 0o644,
		},
		{
			name:     "windows run nested climb",
			goos:     "windows",
			climbRel: "../../myslug/sub",
			lyxArgs:  "loom start",
			want:     "@cd /d \"%~dp0..\\..\\myslug\\sub\" && lyx loom start\r\n",
			wantMode: 0o644,
		},
		{
			name:     "linux ide spawn empty climb",
			goos:     "linux",
			climbRel: "",
			lyxArgs:  "ide spawn myslug",
			want:     "#!/usr/bin/env bash\ncd \"$(dirname \"$0\")/\" && lyx ide spawn myslug\n",
			wantMode: 0o755,
		},
		{
			name:     "linux fabric checkout nested climb",
			goos:     "linux",
			climbRel: "../../myslug/sub",
			lyxArgs:  "fabric checkout",
			want:     "#!/usr/bin/env bash\ncd \"$(dirname \"$0\")/../../myslug/sub\" && lyx fabric checkout\n",
			wantMode: 0o755,
		},
		{
			name:     "linux ide menu nested climb",
			goos:     "linux",
			climbRel: "../sub",
			lyxArgs:  "ide menu",
			want:     "#!/usr/bin/env bash\ncd \"$(dirname \"$0\")/../sub\" && lyx ide menu\n",
			wantMode: 0o755,
		},
		{
			name:     "linux run nested climb",
			goos:     "linux",
			climbRel: "../../myslug/sub",
			lyxArgs:  "loom start",
			want:     "#!/usr/bin/env bash\ncd \"$(dirname \"$0\")/../../myslug/sub\" && lyx loom start\n",
			wantMode: 0o755,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContent, gotMode := launcherScript(tt.goos, tt.climbRel, tt.lyxArgs)

			if string(gotContent) != tt.want {
				t.Errorf("launcherScript(%q, %q, %q) content = %q; want %q",
					tt.goos, tt.climbRel, tt.lyxArgs, string(gotContent), tt.want)
			}
			if uint32(gotMode) != tt.wantMode {
				t.Errorf("launcherScript(%q, %q, %q) mode = %o; want %o",
					tt.goos, tt.climbRel, tt.lyxArgs, gotMode, tt.wantMode)
			}
		})
	}

	t.Run("sh has shebang", func(t *testing.T) {
		content, _ := launcherScript("linux", "", "ide menu")
		if !strings.HasPrefix(string(content), "#!/usr/bin/env bash\n") {
			t.Errorf("launcherScript(linux) content = %q; want shebang prefix", string(content))
		}
	})

	t.Run("cmd has no shebang", func(t *testing.T) {
		content, _ := launcherScript("windows", "", "ide menu")
		if strings.HasPrefix(string(content), "#!") {
			t.Errorf("launcherScript(windows) content = %q; want no shebang", string(content))
		}
	})

	t.Run("cmd uses backslashes and CRLF", func(t *testing.T) {
		content, _ := launcherScript("windows", "../sub", "ide menu")
		s := string(content)
		if !strings.Contains(s, "\\sub") {
			t.Errorf("launcherScript(windows) content = %q; want backslash climb", s)
		}
		if strings.Contains(s, "/sub") {
			t.Errorf("launcherScript(windows) content = %q; want no forward slash climb", s)
		}
		if !strings.HasSuffix(s, "\r\n") {
			t.Errorf("launcherScript(windows) content = %q; want CRLF ending", s)
		}
	})

	t.Run("sh uses forward slashes and LF", func(t *testing.T) {
		content, _ := launcherScript("linux", "../sub", "ide menu")
		s := string(content)
		if !strings.Contains(s, "/sub") {
			t.Errorf("launcherScript(linux) content = %q; want forward slash climb", s)
		}
		if strings.Contains(s, "\\sub") {
			t.Errorf("launcherScript(linux) content = %q; want no backslash climb", s)
		}
		if strings.HasSuffix(s, "\r\n") {
			t.Errorf("launcherScript(linux) content = %q; want no CRLF ending", s)
		}
		if !strings.HasSuffix(s, "\n") {
			t.Errorf("launcherScript(linux) content = %q; want LF ending", s)
		}
	})
}

// TestWriteLaunchers_RunScriptContentAndFilename pins the launcher-filename-unchanged decision's two
// halves together: writeLaunchers still names the run launcher file "run"+ext, and that file's
// content still invokes "loom start", not "loom drive" or the retired "loom run" bootstrap sense.
//
// A scenario in launcherScript/launcherExt's own shape cannot catch a rename of the file, since
// neither of those constructs a path — runPath is built by filepath.Join(launcherDir, "run"+ext)
// inside writeLaunchers alone. So this scenario calls writeLaunchers directly, against a hand-built
// *lyxcwd.Location whose HubPath is a t.TempDir(), the same pattern portallauncher_test.go's other
// tier 1 suites use to avoid a real hub. The menu launcher is pre-seeded at menuLauncherPath(l) so
// writeLaunchers' never-clobber early return fires before it ever reaches PrimeName(l), which a
// hand-built Location cannot satisfy.
func TestWriteLaunchers_RunScriptContentAndFilename(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	l := newPortalLauncherTestLocation(hub, filepath.Join(hub, "prime"), ".")
	const slug = "test-slug"

	menuPath := menuLauncherPath(l)
	if err := os.MkdirAll(filepath.Dir(menuPath), 0o755); err != nil {
		t.Fatalf("mkdir menu launcher dir: %v", err)
	}
	if err := os.WriteFile(menuPath, []byte("preexisting menu launcher\n"), 0o755); err != nil {
		t.Fatalf("seed menu launcher: %v", err)
	}

	if err := writeLaunchers(NewMutations(hub), l, slug); err != nil {
		t.Fatalf("writeLaunchers() error = %v; want nil", err)
	}

	ext := launcherExt(runtime.GOOS)
	launcherDir := LauncherDir(l, slug)

	runPath := filepath.Join(launcherDir, "run"+ext)
	runContent, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatalf("read %s: %v; want the run launcher written at run%s", runPath, err, ext)
	}
	if !strings.Contains(string(runContent), "loom start") {
		t.Errorf("run launcher content = %q; want it to contain %q", string(runContent), "loom start")
	}

	startPath := filepath.Join(launcherDir, "start"+ext)
	if _, err := os.Stat(startPath); !os.IsNotExist(err) {
		t.Errorf("start%s exists at %s; want the launcher filename to stay run%s per the "+
			"launcher-filename-unchanged decision", ext, startPath, ext)
	}
}
