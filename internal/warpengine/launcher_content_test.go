// launcher_content_test.go tests the pure, build-tag-free launcher content
// builder for both the Windows (.cmd) and non-Windows (.sh) branches. Because
// launcherScript and launcherExt take goos as a parameter rather than reading
// runtime.GOOS, both branches are exercised on any host, including this
// Windows dev box.

package warpengine

import (
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
			name:     "windows warp checkout nested climb",
			goos:     "windows",
			climbRel: "../../myslug/sub",
			lyxArgs:  "warp checkout",
			want:     "@cd /d \"%~dp0..\\..\\myslug\\sub\" && lyx warp checkout\r\n",
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
			name:     "linux ide spawn empty climb",
			goos:     "linux",
			climbRel: "",
			lyxArgs:  "ide spawn myslug",
			want:     "#!/usr/bin/env bash\ncd \"$(dirname \"$0\")/\" && lyx ide spawn myslug\n",
			wantMode: 0o755,
		},
		{
			name:     "linux warp checkout nested climb",
			goos:     "linux",
			climbRel: "../../myslug/sub",
			lyxArgs:  "warp checkout",
			want:     "#!/usr/bin/env bash\ncd \"$(dirname \"$0\")/../../myslug/sub\" && lyx warp checkout\n",
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
