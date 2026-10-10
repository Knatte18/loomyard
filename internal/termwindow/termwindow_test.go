package termwindow

import (
	"errors"
	"slices"
	"testing"
)

func TestResolve_ChoosesLauncherPerPlatform(t *testing.T) {
	t.Parallel()

	onPath := func(names ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			if slices.Contains(names, name) {
				return "/bin/" + name, nil
			}
			return "", errors.New("not found")
		}
	}
	tests := []struct {
		name     string
		goos     string
		env      map[string]string
		interop  bool
		lookPath func(string) (string, error)
		want     string
	}{
		{name: "WSLByEnv", goos: "linux", env: map[string]string{"WSL_DISTRO_NAME": "Ubuntu", "DISPLAY": ":0"}, lookPath: onPath("wt.exe", "konsole"), want: "wt.exe"},
		{name: "WSLByInterop", goos: "linux", interop: true, lookPath: onPath("wt.exe"), want: "wt.exe"},
		{name: "WSLWithoutWindowsTerminal", goos: "linux", interop: true, env: map[string]string{"DISPLAY": ":0"}, lookPath: onPath("konsole")},
		{name: "KonsoleOnX11", goos: "linux", env: map[string]string{"DISPLAY": ":0"}, lookPath: onPath("konsole"), want: "konsole"},
		{name: "KonsoleOnWayland", goos: "linux", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, lookPath: onPath("konsole"), want: "konsole"},
		{name: "NoGraphicalSession", goos: "linux", lookPath: onPath("konsole")},
		{name: "NoKonsole", goos: "linux", env: map[string]string{"DISPLAY": ":0"}, lookPath: onPath()},
		{name: "OtherPlatform", goos: "darwin", env: map[string]string{"DISPLAY": ":0"}, lookPath: onPath("konsole")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			launcher, err := resolve(probe{
				goos:       tt.goos,
				getenv:     func(key string) string { return tt.env[key] },
				lookPath:   tt.lookPath,
				fileExists: func(string) bool { return tt.interop },
			})
			if tt.want == "" {
				if !errors.Is(err, ErrNoLauncher) {
					t.Errorf("resolve() = %q, %v; want ErrNoLauncher", launcher.Name(), err)
				}
				return
			}
			if err != nil || launcher.Name() != tt.want {
				t.Errorf("resolve() = %q, %v; want %q", launcher.Name(), err, tt.want)
			}
		})
	}
}

func TestLauncher_ArgumentsOpenTitledWindowRunningAttach(t *testing.T) {
	t.Parallel()

	line := attachLine("/opt/lyx dev/lyx")
	if want := `'/opt/lyx dev/lyx' reed attach; exec bash`; line != want {
		t.Fatalf("attachLine() = %q; want %q", line, want)
	}
	tests := []struct {
		name      string
		arguments func(dir, title, line string) []string
		want      []string
	}{
		{
			name:      "Konsole",
			arguments: konsoleArguments,
			want:      []string{"--workdir", "/hub/slug", "-p", "tabtitle=slug", "-e", "bash", "-lc", line},
		},
		{
			name:      "WindowsTerminal",
			arguments: windowsTerminalArguments,
			want:      []string{"-w", "new", "--title", "slug", "wsl.exe", "--cd", "/hub/slug", "--", "bash", "-lc", `'/opt/lyx dev/lyx' reed attach\; exec bash`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.arguments("/hub/slug", "slug", line); !slices.Equal(got, tt.want) {
				t.Errorf("arguments = %q; want %q", got, tt.want)
			}
		})
	}
}
