// tmuxconf_test.go — tests for the operator tmux config feature's path validation.

package reedengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTmuxConfPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("set -g @marker on\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	plain := write("tmux.conf")
	bracket := write("a[b].conf")
	spaced := write("a b.conf")
	dollar := write("a$b.conf")

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "EmptyValue", raw: "", want: ""},
		{name: "HomeExpansion", raw: "~/tmux.conf", want: plain},
		{name: "AbsoluteRegularFile", raw: plain, want: plain},
		{name: "RelativePath", raw: "tmux.conf", wantErr: "tmux.conf"},
		{name: "MissingFile", raw: filepath.Join(dir, "missing.conf"), wantErr: filepath.Join(dir, "missing.conf")},
		{name: "Directory", raw: dir, wantErr: dir},
		{name: "BracketInName", raw: bracket, wantErr: bracket},
		{name: "SpaceInName", raw: spaced, wantErr: spaced},
		{name: "DollarInName", raw: dollar, wantErr: dollar},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tmuxConfPath(tt.raw, dir)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("tmuxConfPath(%q) error = %v, want none", tt.raw, err)
				}
				if got != tt.want {
					t.Errorf("tmuxConfPath(%q) = %q, want %q", tt.raw, got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("tmuxConfPath(%q) = %q, want an error", tt.raw, got)
			}
			for _, name := range []string{"tmux_conf", tmuxConfEnv, tt.wantErr} {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("tmuxConfPath(%q) error = %q, want it to name %q", tt.raw, err, name)
				}
			}
		})
	}
}
