// locate_test.go covers the choice of desktop directory and launcher form per platform, through a hand-made probe.

package desktoplauncher

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLocate_ChoosesDirectoryAndFormPerPlatform(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		goos    string
		env     map[string]string
		interop bool
		// dirs are the directories that exist.
		dirs           []string
		freedesktopErr error
		want           Desktop
		wantNone       bool
	}{
		{name: "LinuxFreedesktop", goos: "linux", dirs: []string{"/home/u/Skrivebord"}, want: Desktop{Dir: "/home/u/Skrivebord", Form: FormFreedesktop}},
		{name: "OverrideWinsOnLinux", goos: "linux", env: map[string]string{desktopEnv: "/run/desk"}, dirs: []string{"/run/desk"}, want: Desktop{Dir: "/run/desk", Form: FormFreedesktop}},
		{name: "WSLByEnv", goos: "linux", env: map[string]string{wslDistroEnv: "Ubuntu"}, dirs: []string{"/mnt/c/Users/u/Desktop"}, want: Desktop{Dir: "/mnt/c/Users/u/Desktop", Form: FormWindowsWSL, Distro: "Ubuntu"}},
		{name: "WSLByInterop", goos: "linux", interop: true, dirs: []string{"/mnt/c/Users/u/Desktop"}, want: Desktop{Dir: "/mnt/c/Users/u/Desktop", Form: FormWindowsWSL}},
		{name: "NativeWindows", goos: "windows", dirs: []string{"/mnt/c/Users/u/Desktop"}, want: Desktop{Dir: "/mnt/c/Users/u/Desktop", Form: FormWindows}},
		{name: "MissingDirectory", goos: "linux", wantNone: true},
		{name: "UnresolvedDirectory", goos: "linux", freedesktopErr: errors.New("no home"), wantNone: true},
		{name: "OtherPlatform", goos: "darwin", dirs: []string{"/home/u/Skrivebord"}, wantNone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := locate(probe{
				goos:       tt.goos,
				getenv:     func(key string) string { return tt.env[key] },
				fileExists: func(string) bool { return tt.interop },
				isDir: func(path string) bool {
					for _, dir := range tt.dirs {
						if dir == path {
							return true
						}
					}
					return false
				},
				freedesktopDir: func() (string, error) { return "/home/u/Skrivebord", tt.freedesktopErr },
				windowsDir:     func() (string, error) { return "/mnt/c/Users/u/Desktop", nil },
			})
			if tt.wantNone {
				if !errors.Is(err, ErrNoDesktop) {
					t.Errorf("locate() = %+v, %v; want ErrNoDesktop", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("locate() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestFreedesktopDesktopDir_ReadsUserDirsOrFallsBackToHome(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// plant prepares the config directory.
		plant   func(t *testing.T, configDir string)
		want    string
		wantErr bool
	}{
		{
			name: "UserDirsNamesTheDesktop",
			plant: func(t *testing.T, configDir string) {
				if err := os.WriteFile(filepath.Join(configDir, userDirsFileName), []byte("XDG_DESKTOP_DIR=\"$HOME/Skrivebord\"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "/home/u/Skrivebord",
		},
		{
			name:  "NoUserDirsFallsBackToTheHomeDesktop",
			plant: func(*testing.T, string) {},
			want:  filepath.Join("/home/u", "Desktop"),
		},
		{
			name: "UserDirsThatCannotBeReadIsAnError",
			plant: func(t *testing.T, configDir string) {
				if err := os.Mkdir(filepath.Join(configDir, userDirsFileName), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			configDir := t.TempDir()
			tt.plant(t, configDir)

			got, err := freedesktopDesktopDir(
				func() (string, error) { return configDir, nil },
				func() (string, error) { return "/home/u", nil },
			)

			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("freedesktopDesktopDir() = %q, %v; want %q, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestParseUserDirs_ReadsTheDesktopDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		data      string
		want      string
		wantFound bool
	}{
		{name: "UnderHome", data: "# written by xdg-user-dirs-update\nXDG_DOWNLOAD_DIR=\"$HOME/Nedlastinger\"\nXDG_DESKTOP_DIR=\"$HOME/Skrivebord\"\n", want: "/home/u/Skrivebord", wantFound: true},
		{name: "Absolute", data: "XDG_DESKTOP_DIR=\"/data/desk/\"\n", want: "/data/desk", wantFound: true},
		{name: "HomeItself", data: "XDG_DESKTOP_DIR=\"$HOME\"\n", want: "/home/u", wantFound: true},
		{name: "Relative", data: "XDG_DESKTOP_DIR=\"desk\"\n"},
		{name: "NotNamed", data: "XDG_MUSIC_DIR=\"$HOME/Music\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, found := parseUserDirs([]byte(tt.data), "/home/u")
			if got != tt.want || found != tt.wantFound {
				t.Errorf("parseUserDirs(%q) = %q, %v; want %q, %v", tt.data, got, found, tt.want, tt.wantFound)
			}
		})
	}
}
