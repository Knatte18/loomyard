// headerpane_test.go covers headerLaunchCmd's pure command-string
// composition against both real Shell implementations (posix, pwsh) with a
// fake exe path — hermetic, no live tmux required.

package muxengine

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
			want: "'/opt/lyx/bin/lyx' 'mux' 'header' '--blocking'",
		},
		{
			name: "Pwsh",
			sh:   shell.Pwsh(),
			exe:  `C:\tools\lyx.exe`,
			want: `& 'C:\tools\lyx.exe' 'mux' 'header' '--blocking'`,
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
