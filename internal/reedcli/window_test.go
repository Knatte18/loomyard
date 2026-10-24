//go:build tier0

// window_test.go pins the `window` reed verb against a recording launcher seam; it opens no window.

package reedcli

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

type windowCall struct {
	dir, title, lyxPath string
}

func TestWindowCmd_OpensATerminalAttachedToTheToldWorktree(t *testing.T) {
	t.Parallel()

	existing := t.TempDir()
	tests := []struct {
		name string
		args []string
		// openErr is what the launcher seam returns.
		openErr   error
		wantCode  int
		wantTitle string
		wantCalls int
		// wantErr must appear in the error envelope; empty means an ok envelope.
		wantErr string
	}{
		{name: "TitleFlag", args: []string{existing, "--title", "t"}, wantTitle: "t", wantCalls: 1},
		{name: "DefaultTitleIsTheBaseName", args: []string{existing}, wantTitle: filepath.Base(existing), wantCalls: 1},
		{name: "MissingDirectoryRunsNoLauncher", args: []string{filepath.Join(existing, "absent")}, wantCode: 1, wantErr: "absent"},
		{name: "LauncherErrorIsCarried", args: []string{existing}, openErr: errors.New("no terminal window launcher known here"), wantCode: 1, wantTitle: filepath.Base(existing), wantCalls: 1, wantErr: "no terminal window launcher"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls []windowCall
			c := &reedCLI{openTerminal: func(dir, title, lyxPath string) error {
				calls = append(calls, windowCall{dir, title, lyxPath})
				return tt.openErr
			}}
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
			if tt.wantCalls == 0 {
				return
			}
			if got := calls[0]; got.dir != existing || got.title != tt.wantTitle || got.lyxPath == "" {
				t.Errorf("launcher call = %+v; want dir %q, title %q and a non-empty lyx path", got, existing, tt.wantTitle)
			}
			if tt.wantErr == "" && !strings.Contains(out.String(), `"title"`) {
				t.Errorf("output = %q; want it to carry the title", out.String())
			}
		})
	}
}
