// desktoplauncher_test.go covers the launcher files Add writes, Remove takes away and Shortnames lists.
// Each test works in a temporary desktop.

package desktoplauncher_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/desktoplauncher"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// desktopFiles returns the names of the files in dir, in name order.
func desktopFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestAddRemove_KeepTheAllHubsLauncherWhileAnyHubHasOne(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		form      desktoplauncher.Form
		extension string
	}{
		{desktoplauncher.FormFreedesktop, ".desktop"},
		{desktoplauncher.FormWindowsWSL, ".cmd"},
		{desktoplauncher.FormWindows, ".cmd"},
	}
	for _, tt := range tests {
		t.Run(string(tt.form), func(t *testing.T) {
			t.Parallel()
			desktop := desktoplauncher.Desktop{Dir: t.TempDir(), Form: tt.form}
			unrelated := filepath.Join(desktop.Dir, "lym-hub-notes.txt")
			if err := os.WriteFile(unrelated, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			for _, shortname := range []string{"alpha", "beta"} {
				if err := desktop.Add("/opt/lym", shortname); err != nil {
					t.Fatalf("Add(%q) = %v", shortname, err)
				}
			}

			wantBoth := []string{"lym-hub-alpha" + tt.extension, "lym-hub-beta" + tt.extension, "lym-hub-notes.txt", "lym-hubs" + tt.extension}
			if got := desktopFiles(t, desktop.Dir); !slices.Equal(got, wantBoth) {
				t.Errorf("desktop after two adds = %v; want %v", got, wantBoth)
			}
			if got, err := desktop.Shortnames(); err != nil || !slices.Equal(got, []string{"alpha", "beta"}) {
				t.Errorf("Shortnames() = %v, %v; want [alpha beta]", got, err)
			}

			if err := desktop.Remove("alpha"); err != nil {
				t.Fatalf("Remove(alpha) = %v", err)
			}
			wantBeta := []string{"lym-hub-beta" + tt.extension, "lym-hub-notes.txt", "lym-hubs" + tt.extension}
			if got := desktopFiles(t, desktop.Dir); !slices.Equal(got, wantBeta) {
				t.Errorf("desktop after removing alpha = %v; want %v", got, wantBeta)
			}

			for _, shortname := range []string{"beta", "beta"} {
				if err := desktop.Remove(shortname); err != nil {
					t.Fatalf("Remove(%q) = %v", shortname, err)
				}
			}
			if got := desktopFiles(t, desktop.Dir); !slices.Equal(got, []string{"lym-hub-notes.txt"}) {
				t.Errorf("desktop after removing the last hub = %v; want only the unrelated file", got)
			}
		})
	}
}

func TestAdd_WritesLaunchersRunningMachineStart(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		desktop desktoplauncher.Desktop
		lymPath string
		// file is the hub launcher's file name, and want its content.
		file string
		want string
		// allFile is the all-hubs launcher's file name, and wantAll its content.
		allFile string
		wantAll string
		wantErr bool
	}{
		{
			name:    "Freedesktop",
			desktop: desktoplauncher.Desktop{Form: desktoplauncher.FormFreedesktop},
			lymPath: "/opt/lym dev/lym",
			file:    "lym-hub-ly.desktop",
			want: "[Desktop Entry]\nType=Application\nName=lym: ly\nComment=Runs lym machine start ly on this machine\n" +
				`Exec=bash -lc "'/opt/lym dev/lym' machine start ly; echo; read -r -p 'Press Enter to close this window' _"` +
				"\nTerminal=true\nIcon=utilities-terminal\n",
			allFile: "lym-hubs.desktop",
			wantAll: "[Desktop Entry]\nType=Application\nName=lym: start every hub\nComment=Runs lym machine start on this machine\n" +
				`Exec=bash -lc "'/opt/lym dev/lym' machine start; echo; read -r -p 'Press Enter to close this window' _"` +
				"\nTerminal=true\nIcon=utilities-terminal\n",
		},
		{
			name:    "FreedesktopEscapesExecSpecials",
			desktop: desktoplauncher.Desktop{Form: desktoplauncher.FormFreedesktop},
			lymPath: `/opt/100%$x\lym`,
			file:    "lym-hub-ly.desktop",
			want: "[Desktop Entry]\nType=Application\nName=lym: ly\nComment=Runs lym machine start ly on this machine\n" +
				`Exec=bash -lc "'/opt/100%%\\$x\\\\lym' machine start ly; echo; read -r -p 'Press Enter to close this window' _"` +
				"\nTerminal=true\nIcon=utilities-terminal\n",
		},
		{
			name:    "WindowsWSL",
			desktop: desktoplauncher.Desktop{Form: desktoplauncher.FormWindowsWSL, Distro: "Ubuntu-24.04"},
			lymPath: "/home/u/go/bin/lym",
			file:    "lym-hub-ly.cmd",
			want:    "@echo off\r\nrem lym: ly\r\nwsl.exe -d \"Ubuntu-24.04\" -- bash -lc \"'/home/u/go/bin/lym' machine start ly\"\r\npause\r\n",
			allFile: "lym-hubs.cmd",
			wantAll: "@echo off\r\nrem lym: start every hub\r\nwsl.exe -d \"Ubuntu-24.04\" -- bash -lc \"'/home/u/go/bin/lym' machine start\"\r\npause\r\n",
		},
		{
			name:    "WindowsWSLDefaultDistroDoublesPercent",
			desktop: desktoplauncher.Desktop{Form: desktoplauncher.FormWindowsWSL},
			lymPath: "/opt/50%/lym",
			file:    "lym-hub-ly.cmd",
			want:    "@echo off\r\nrem lym: ly\r\nwsl.exe -- bash -lc \"'/opt/50%%/lym' machine start ly\"\r\npause\r\n",
		},
		{
			name:    "WindowsWSLRefusesADoubleQuote",
			desktop: desktoplauncher.Desktop{Form: desktoplauncher.FormWindowsWSL},
			lymPath: `/opt/"x"/lym`,
			wantErr: true,
		},
		{
			name:    "WindowsWSLRefusesADollarSign",
			desktop: desktoplauncher.Desktop{Form: desktoplauncher.FormWindowsWSL},
			lymPath: "/opt/$x/lym",
			wantErr: true,
		},
		{
			name:    "WindowsWSLRefusesABackquote",
			desktop: desktoplauncher.Desktop{Form: desktoplauncher.FormWindowsWSL},
			lymPath: "/opt/`x`/lym",
			wantErr: true,
		},
		{
			name:    "WindowsWSLRefusesABackslash",
			desktop: desktoplauncher.Desktop{Form: desktoplauncher.FormWindowsWSL},
			lymPath: `/opt/\x/lym`,
			wantErr: true,
		},
		{
			name:    "NativeWindows",
			desktop: desktoplauncher.Desktop{Form: desktoplauncher.FormWindows},
			lymPath: `C:\Program Files\lym\lym.exe`,
			file:    "lym-hub-ly.cmd",
			want:    "@echo off\r\nrem lym: ly\r\n\"C:\\Program Files\\lym\\lym.exe\" machine start ly\r\npause\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			desktop := tt.desktop
			desktop.Dir = t.TempDir()

			err := desktop.Add(tt.lymPath, "ly")

			if tt.wantErr {
				if err == nil {
					t.Errorf("Add(%q) = nil; want an error", tt.lymPath)
				}
				if got := desktopFiles(t, desktop.Dir); len(got) > 0 {
					t.Errorf("desktop after a refused add = %v; want it empty", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Add(%q) = %v", tt.lymPath, err)
			}
			for file, want := range map[string]string{tt.file: tt.want, tt.allFile: tt.wantAll} {
				if file == "" {
					continue
				}
				got, err := os.ReadFile(filepath.Join(desktop.Dir, file))
				if err != nil || string(got) != want {
					t.Errorf("launcher %s = %q, %v; want %q", file, got, err, want)
				}
			}
		})
	}
}
