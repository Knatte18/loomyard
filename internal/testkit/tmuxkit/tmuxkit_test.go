//go:build tier0

package tmuxkit

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestCheckSocketPath(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		uid     int
		wantErr bool
	}{
		{"short directory", "/tmp/lym123", 1000, false},
		{"directory at the limit", "/tmp/" + strings.Repeat("d", 107-len("/tmp/")-len("/tmux-1000/")-maxKeyBytes), 1000, false},
		{"directory one byte over", "/tmp/" + strings.Repeat("d", 107-len("/tmp/")-len("/tmux-1000/")-maxKeyBytes+1), 1000, true},
		{"run directory of the widest uid", "/tmp/lym-4294967295/r0a1b2c", 4294967295, false},
		{"longer uid widens the path", "/tmp/" + strings.Repeat("d", 107-len("/tmp/")-len("/tmux-1000/")-maxKeyBytes), 1000000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSocketPath(tt.dir, tt.uid)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkSocketPath(%q, %d) error = %v, wantErr %v", tt.dir, tt.uid, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "TMPDIR") {
				t.Errorf("error %q does not name TMPDIR as the remedy", err)
			}
		})
	}
}

func TestSetEnv(t *testing.T) {
	t.Setenv("TMUX", "/outer/tmux,1,0")
	t.Setenv("TMUX_PANE", "%3")
	t.Setenv("LYM_REED_TMUX_CONF", "/operator/tmux.conf")
	t.Setenv("TMUX_TMPDIR", "/elsewhere")
	// TMP and TEMP are the Windows temp variables; setting them first has t.Setenv restore them.
	for _, name := range []string{"TMPDIR", "TMP", "TEMP", "GIT_CEILING_DIRECTORIES"} {
		t.Setenv(name, os.Getenv(name))
	}
	dir := filepath.Join(t.TempDir(), dirPrefix+"0")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	setEnv(dir)

	for _, name := range []string{"TMUX_TMPDIR", "TMPDIR", "GIT_CEILING_DIRECTORIES"} {
		if got := os.Getenv(name); got != dir {
			t.Errorf("%s = %q, want %q", name, got, dir)
		}
	}
	// A subtest's TempDir resolves its base afresh, so it sees the TMPDIR setEnv just set.
	t.Run("temp directory lands under TMPDIR", func(t *testing.T) {
		if got := t.TempDir(); !strings.HasPrefix(got, dir+string(filepath.Separator)) {
			t.Errorf("t.TempDir() = %q, want it under %q", got, dir)
		}
	})
	for _, name := range []string{"TMUX", "TMUX_PANE", "LYM_REED_TMUX_CONF"} {
		if v, ok := os.LookupEnv(name); ok {
			t.Errorf("%s still set to %q", name, v)
		}
	}
}

func TestListSockets_MissingDirectory(t *testing.T) {
	if got := listSockets(t.TempDir(), 1000); len(got) != 0 {
		t.Errorf("listSockets of a directory with no tmux subdirectory = %v, want none", got)
	}
}

func TestMaxKeyBytes_HoldsReedServerName(t *testing.T) {
	hub := "/" + strings.Repeat("h", 300) + "-LYMHUB"
	if got := len(reedengine.ServerName(hub)); got > maxKeyBytes {
		t.Errorf("reedengine.ServerName of an over-long hub path is %d bytes, over the kit's %d-byte key bound", got, maxKeyBytes)
	}
}

func TestAfterRun_ReportsFailedRemovalKeepingTheCode(t *testing.T) {
	t.Parallel()
	for _, code := range []int{0, 1} {
		dir := t.TempDir()
		removeErr := errors.New("directory in use")

		var out bytes.Buffer
		got := afterRun(&out, dir, code, 100*time.Millisecond, func() {}, func() error { return removeErr })

		if got != code {
			t.Errorf("afterRun code = %d, want the input code %d unchanged", got, code)
		}
		if want := "tmuxkit: remove run directory " + dir + ": directory in use; the next start's sweep removes it\n"; out.String() != want {
			t.Errorf("afterRun output = %q, want %q", out.String(), want)
		}
	}
}
