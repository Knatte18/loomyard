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
	tests := []struct {
		name string
		args []string
		// openErr is what the launcher seam returns.
		openErr error
		// placeErr is what the window-command seam returns.
		placeErr error
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls []windowCall
			var placed []string
			c := &reedCLI{
				openTerminal: func(dir, title, lymPath string) error {
					calls = append(calls, windowCall{dir, title, lymPath})
					return tt.openErr
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
			if tt.wantCalls == 1 && tt.openErr == nil {
				wantPlaced = []string{existing}
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
