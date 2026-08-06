// headerpane_test.go covers headerLaunchCmd's pure command-string composition against both real Shell implementations (posix, pwsh) with a fake exe path, and headerLaunchLine's underTest suppression branch — hermetic, no live tmux required.

package reedengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
)

func TestHeaderLaunchCmd(t *testing.T) {
	tests := []struct {
		name string
		sh   shell.Shell
		exe  string
		want string
	}{
		{
			name: "Posix",
			sh:   shell.Posix(),
			exe:  "/opt/lyx/bin/lyx",
			want: "'/opt/lyx/bin/lyx' 'reed' 'header' '--blocking'",
		},
		{
			name: "Pwsh",
			sh:   shell.Pwsh(),
			exe:  `C:\tools\lyx.exe`,
			want: `& 'C:\tools\lyx.exe' 'reed' 'header' '--blocking'`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := headerLaunchCmd(tt.sh, tt.exe); got != tt.want {
				t.Errorf("headerLaunchCmd(%s, %q) = %q, want %q", tt.name, tt.exe, got, tt.want)
			}
		})
	}
}

func TestHeaderLaunchLine(t *testing.T) {
	sh := shell.Posix()
	exe := "/opt/lyx/bin/lyx"

	if got := headerLaunchLine(sh, exe, true); got != "" {
		t.Errorf("headerLaunchLine(underTest=true) = %q, want \"\" (bare shell pane, no re-exec)", got)
	}
	if got, want := headerLaunchLine(sh, exe, false), headerLaunchCmd(sh, exe); got != want {
		t.Errorf("headerLaunchLine(underTest=false) = %q, want %q", got, want)
	}

	if !testing.Testing() {
		t.Fatalf("testing.Testing() = false inside a test binary; the boot site's underTest wiring relies on it being true here")
	}
}
