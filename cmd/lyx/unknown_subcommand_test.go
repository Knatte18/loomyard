// unknown_subcommand_test.go covers W16 unknown-subcommand rejection and bare-group listing for
// module groups mounted under the real lyx root command, exercising the GroupRunE wiring and
// PersistentPreRunE guards via the run() seam.

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestMountedUnknownSubcommand verifies "lyx <group> bogus" exits 1 with "unknown subcommand" in
// error.
func TestMountedUnknownSubcommand(t *testing.T) {
	tests := []struct {
		group string
	}{
		{"board"},
		{"ide"},
		{"reed"},
	}
	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			var out bytes.Buffer
			code := run([]string{tt.group, "bogus"}, &out)

			if code != 1 {
				t.Errorf("run([%s bogus]) = %d; want 1\noutput: %s", tt.group, code, out.String())
			}

			envelope.RequireErr(t, out.String(), "unknown subcommand")
		})
	}
}

// TestMountedBareGroupListing_NoGitRepo verifies bare "lyx <group>" exits 0 with subcommand
// listing.
func TestMountedBareGroupListing_NoGitRepo(t *testing.T) {
	tests := []struct {
		group       string
		knownSubcmd string // a subcommand name expected in the help listing
	}{
		{"board", "upsert"},
		{"ide", "spawn"},
		{"reed", "up"},
	}
	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			// Run from a temp dir that is not a git repo; the PersistentPreRunE guard
			// must fire before lyxcwd.Resolve is called, keeping the exit code at 0.
			tmpDir := t.TempDir()
			t.Chdir(tmpDir)

			var out bytes.Buffer
			code := run([]string{tt.group}, &out)
			stdout := out.String()

			if code != 0 {
				t.Errorf("run([%s]) = %d; want 0 for bare group listing\noutput: %s", tt.group, code, stdout)
			}
			if strings.Contains(stdout, `"ok":false`) {
				t.Errorf("run([%s]) emitted error envelope; want plain help text\noutput: %s", tt.group, stdout)
			}
			if strings.Contains(stdout, "not a git repository") {
				t.Errorf("run([%s]) emitted \"not a git repository\"; guard not working\noutput: %s", tt.group, stdout)
			}
			if !strings.Contains(stdout, tt.knownSubcmd) {
				t.Errorf("run([%s]) output does not contain %q; want subcommand listing\noutput: %s", tt.group, tt.knownSubcmd, stdout)
			}
		})
	}
}

// TestUpdateCommandRemoved verifies "lyx update" no longer resolves (folded into config reconcile).
func TestUpdateCommandRemoved(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"update"}, &out)

	if code != 1 {
		t.Errorf("run([update]) = %d; want 1 (update should be unknown)\noutput: %s", code, out.String())
	}

	envelope.RequireErr(t, out.String(), "")
}
