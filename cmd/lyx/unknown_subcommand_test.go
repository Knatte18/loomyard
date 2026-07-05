// unknown_subcommand_test.go covers W16 unknown-subcommand rejection and bare-group
// listing for all module groups when mounted under the real lyx root command,
// exercising the GroupRunE wiring and PersistentPreRunE guards via the run() seam.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestMountedUnknownSubcommand verifies that "lyx <group> bogus" exits 1 and emits
// a JSON error envelope with ok=false and an error string containing "unknown subcommand".
// This exercises the GroupRunE wiring applied to each mounted group command in batch 2:
// warp (no guard), and weft/board/ide/mux (with PersistentPreRunE guards).
func TestMountedUnknownSubcommand(t *testing.T) {
	tests := []struct {
		group string
	}{
		{"warp"},
		{"weft"},
		{"board"},
		{"ide"},
		{"mux"},
	}
	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			var out bytes.Buffer
			code := run([]string{tt.group, "bogus"}, &out)

			if code != 1 {
				t.Errorf("run([%s bogus]) = %d; want 1\noutput: %s", tt.group, code, out.String())
			}

			// GroupRunE must emit a well-formed JSON error envelope.
			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
				t.Fatalf("run([%s bogus]) output is not valid JSON: %v; output: %q", tt.group, err, out.String())
			}
			if ok, _ := env["ok"].(bool); ok {
				t.Errorf("run([%s bogus]) ok = true; want false", tt.group)
			}
			// Confirm GroupRunE's "unknown subcommand" message reaches the error field,
			// proving the mounted group no longer falls through to cobra's help output.
			errMsg, _ := env["error"].(string)
			if !strings.Contains(errMsg, "unknown subcommand") {
				t.Errorf("run([%s bogus]) error = %q; want \"unknown subcommand\" substring", tt.group, errMsg)
			}
		})
	}
}

// TestMountedBareGroupListing_NoGitRepo verifies that bare "lyx <group>" exits 0 and
// prints the human-readable subcommand listing without emitting an error envelope or a
// "not a git repository" message. Each test runs from a temp dir (not a git repo) so
// that a PersistentPreRunE guard regression — which would invoke hubgeometry.Resolve and fail
// — surfaces as a visible test failure.
func TestMountedBareGroupListing_NoGitRepo(t *testing.T) {
	tests := []struct {
		group       string
		knownSubcmd string // a subcommand name expected in the help listing
	}{
		{"weft", "commit"},
		{"board", "upsert"},
		{"ide", "spawn"},
		{"mux", "up"},
	}
	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			// Run from a temp dir that is not a git repo; the PersistentPreRunE guard
			// must fire before hubgeometry.Resolve is called, keeping the exit code at 0.
			tmpDir := t.TempDir()
			t.Chdir(tmpDir)

			var out bytes.Buffer
			code := run([]string{tt.group}, &out)
			stdout := out.String()

			if code != 0 {
				t.Errorf("run([%s]) = %d; want 0 for bare group listing\noutput: %s", tt.group, code, stdout)
			}
			// A JSON error envelope must not be present on a bare-group help path.
			if strings.Contains(stdout, `"ok":false`) {
				t.Errorf("run([%s]) emitted error envelope; want plain help text\noutput: %s", tt.group, stdout)
			}
			// The git-repo error from PersistentPreRunE must not appear.
			if strings.Contains(stdout, "not a git repository") {
				t.Errorf("run([%s]) emitted \"not a git repository\"; guard not working\noutput: %s", tt.group, stdout)
			}
			// The help listing must contain at least one known subcommand name.
			if !strings.Contains(stdout, tt.knownSubcmd) {
				t.Errorf("run([%s]) output does not contain %q; want subcommand listing\noutput: %s", tt.group, tt.knownSubcmd, stdout)
			}
		})
	}
}

// TestUpdateCommandRemoved verifies that "lyx update" no longer resolves after the
// command was folded into "lyx config reconcile". It must exit 1 with a JSON error
// envelope whose ok field is false.
func TestUpdateCommandRemoved(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"update"}, &out)

	if code != 1 {
		t.Errorf("run([update]) = %d; want 1 (update should be unknown)\noutput: %s", code, out.String())
	}

	// Cobra must emit a well-formed JSON error envelope via RunRoot.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("run([update]) output is not valid JSON: %v; output: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("run([update]) ok = true; want false")
	}
}

// TestMountedBareWarp verifies that bare "lyx warp" exits 0 and prints the subcommand
// listing. warp has no PersistentPreRunE so no guard is needed; GroupRunE with empty
// args delegates to cmd.Help(). This test confirms the GroupRunE-only wiring from card 5.
func TestMountedBareWarp(t *testing.T) {
	// Run from a temp dir to keep the test environment consistent with the guarded groups.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	var out bytes.Buffer
	code := run([]string{"warp"}, &out)
	stdout := out.String()

	if code != 0 {
		t.Errorf("run([warp]) = %d; want 0 for bare group listing\noutput: %s", code, stdout)
	}
	if strings.Contains(stdout, `"ok":false`) {
		t.Errorf("run([warp]) emitted error envelope; want plain help text\noutput: %s", stdout)
	}
	// Must list at least one known warp subcommand to confirm help was printed.
	if !strings.Contains(stdout, "add") && !strings.Contains(stdout, "list") {
		t.Errorf("run([warp]) output does not contain known subcommand; want listing\noutput: %s", stdout)
	}
}
