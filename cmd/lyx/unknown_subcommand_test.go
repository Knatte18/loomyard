// unknown_subcommand_test.go covers W16 unknown-subcommand rejection and bare-group listing for
// module groups mounted under the real lyx root command, exercising the GroupRunE wiring and
// PersistentPreRunE guards via the run() seam.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
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

			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
				t.Fatalf("run([%s bogus]) output is not valid JSON: %v; output: %q", tt.group, err, out.String())
			}
			if ok, _ := env["ok"].(bool); ok {
				t.Errorf("run([%s bogus]) ok = true; want false", tt.group)
			}
			errMsg, _ := env["error"].(string)
			if !strings.Contains(errMsg, "unknown subcommand") {
				t.Errorf("run([%s bogus]) error = %q; want \"unknown subcommand\" substring", tt.group, errMsg)
			}
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

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("run([update]) output is not valid JSON: %v; output: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("run([update]) ok = true; want false")
	}
}
