// window_test.go pins the `window` reed verb against a recording launcher seam; it opens no window.

package reedcli

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

type windowCall struct {
	dir, title, lymPath string
}

func TestWindowCmd_OpensATerminalAttachedToTheToldWorktree(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const standInLym = "/stand-in/lym"
	const formedTitle = "T7 [ly:P3] task"
	existing := t.TempDir()
	cwd, err := lymcwd.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, existing)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		// openErr is what the launcher seam returns.
		openErr error
		// placeErr is what the window-command seam returns.
		placeErr error
		// placing is whether the launcher seam reports the window as one to place, with handErr from the hand-off.
		placing bool
		handErr error
		// defaultLookup leaves lymBinary nil, so the production lookup runs and refuses the test binary.
		defaultLookup bool
		wantCode      int
		wantTitle     string
		wantCalls     int
		// wantErr must appear in the error envelope; empty means an ok envelope.
		wantErr string
	}{
		{name: "TitleFlag", args: []string{existing, "--title", "t"}, wantTitle: "t", wantCalls: 1},
		{name: "DefaultTitleIsTheFormedTitle", args: []string{existing}, wantTitle: formedTitle, wantCalls: 1},
		{name: "MissingDirectoryRunsNoLauncher", args: []string{filepath.Join(existing, "absent")}, wantCode: 1, wantErr: "absent"},
		{name: "LauncherErrorIsCarried", args: []string{existing}, openErr: errors.New("no terminal window launcher known here"), wantCode: 1, wantTitle: formedTitle, wantCalls: 1, wantErr: "no terminal window launcher"},
		{name: "DefaultLookupRefusesTheTestBinaryAndRunsNoLauncher", args: []string{existing}, defaultLookup: true, wantCode: 1, wantErr: "window: resolve the running lym"},
		{name: "WindowCommandFailureOnlyWarns", args: []string{existing}, placeErr: errors.New("window command failed"), wantTitle: formedTitle, wantCalls: 1},
		{name: "PlacementHandsTheWindowToTheChild", args: []string{existing}, placing: true, wantTitle: formedTitle, wantCalls: 1},
		{name: "HandOffFailureRunsTheWindowCommand", args: []string{existing}, placing: true, handErr: errors.New("no child"), wantTitle: formedTitle, wantCalls: 1},
		// Every seam is told the absolute path, which the checks below compare against.
		{name: "ARelativeDirectoryIsToldOnAsAbsolute", args: []string{relative}, placing: true, handErr: errors.New("no child"), wantTitle: formedTitle, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls []windowCall
			var placed []string
			type handOff struct {
				lymPath, dir, title string
				pid                 int
			}
			var handed []handOff
			c := &reedCLI{
				openTerminal: func(dir, title, lymPath string) (int, bool, error) {
					calls = append(calls, windowCall{dir, title, lymPath})
					if !tt.placing {
						return 0, false, tt.openErr
					}
					return 4242, true, tt.openErr
				},
				handPlacement: func(lymPath, dir, title string, pid int) error {
					handed = append(handed, handOff{lymPath, dir, title, pid})
					return tt.handErr
				},
				placeWindow: func(dir string) error {
					placed = append(placed, dir)
					return tt.placeErr
				},
				windowTitle: func(dir string) string {
					if dir != existing {
						t.Errorf("window title formed for %q; want %q", dir, existing)
					}
					return formedTitle
				},
			}
			if !tt.defaultLookup {
				c.lymBinary = func() (string, error) { return standInLym, nil }
			}
			var out bytes.Buffer

			code := clihelp.Execute(c.windowCmd(), &out, tt.args)

			if code != tt.wantCode {
				t.Fatalf("exit = %d; want %d; output: %s", code, tt.wantCode, out.String())
			}
			if tt.wantErr == "" {
				envelope.RequireOK(t, out.String())
			} else {
				envelope.RequireErr(t, out.String(), tt.wantErr)
			}
			if len(calls) != tt.wantCalls {
				t.Fatalf("launcher calls = %v; want %d", calls, tt.wantCalls)
			}
			wantPlaced := []string{}
			if tt.wantCalls == 1 && tt.openErr == nil && (!tt.placing || tt.handErr != nil) {
				wantPlaced = []string{existing}
			}
			var wantHanded []handOff
			if tt.placing {
				wantHanded = []handOff{{standInLym, existing, formedTitle, 4242}}
			}
			if !slices.Equal(handed, wantHanded) {
				t.Errorf("hand-offs = %v; want %v", handed, wantHanded)
			}
			if !slices.Equal(placed, wantPlaced) {
				t.Errorf("window command runs = %v; want %v, one after each window opened", placed, wantPlaced)
			}
			if tt.wantCalls == 0 {
				return
			}
			if got := calls[0]; got.dir != existing || got.title != tt.wantTitle || got.lymPath != standInLym {
				t.Errorf("launcher call = %+v; want dir %q, title %q and lym path %q", got, existing, tt.wantTitle, standInLym)
			}
			if tt.wantErr == "" && !strings.Contains(out.String(), `"title"`) {
				t.Errorf("output = %q; want it to carry the title", out.String())
			}
		})
	}
}
