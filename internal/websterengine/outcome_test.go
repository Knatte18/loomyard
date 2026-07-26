// outcome_test.go exercises parseOutcome's accept/reject table and
// archiveStaleOutcome's rename/preserve/no-op/collision behavior. Tier 1:
// no git, only t.TempDir() plus an injected now.

package websterengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// outcomeWriteFile writes raw content to path, creating its parent
// directory first, failing the test on any error.
func outcomeWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// TestParseOutcome_AcceptReject tables every outcome.yaml accept/reject
// case parseOutcome's fail-loud discipline pins: a well-formed
// done/stuck/paused file parses; an unrecognized outcome value, a stuck
// file missing stuck_reason, and an unknown extra key are all rejected
// loudly.
func TestParseOutcome_AcceptReject(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
		want    outcome
	}{
		{
			name:    "done accepted",
			content: "outcome: done\nstuck_reason: null\nbatches_done: 3\n",
			want:    outcome{Outcome: "done", StuckReason: "", BatchesDone: 3},
		},
		{
			name:    "stuck with reason accepted",
			content: "outcome: stuck\nstuck_reason: \"batch 03 red after 2 self-fix attempts\"\nbatches_done: 2\n",
			want:    outcome{Outcome: "stuck", StuckReason: "batch 03 red after 2 self-fix attempts", BatchesDone: 2},
		},
		{
			name:    "paused accepted",
			content: "outcome: paused\nstuck_reason: null\nbatches_done: 1\n",
			want:    outcome{Outcome: "paused", StuckReason: "", BatchesDone: 1},
		},
		{
			name:    "unrecognized outcome value rejected",
			content: "outcome: bogus\nstuck_reason: null\nbatches_done: 0\n",
			wantErr: true,
		},
		{
			name:    "stuck without stuck_reason rejected",
			content: "outcome: stuck\nstuck_reason: null\nbatches_done: 0\n",
			wantErr: true,
		},
		{
			name:    "stuck with blank stuck_reason rejected",
			content: "outcome: stuck\nstuck_reason: \"   \"\nbatches_done: 0\n",
			wantErr: true,
		},
		{
			name:    "unknown key rejected",
			content: "outcome: done\nstuck_reason: null\nbatches_done: 1\nbogus_extra_key: true\n",
			wantErr: true,
		},
		{
			name:    "unparseable yaml rejected",
			content: "outcome: [this is not a mapping\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "outcome.yaml")
			outcomeWriteFile(t, path, tt.content)

			got, err := parseOutcome(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseOutcome() error = nil; want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOutcome() error = %v; want nil", err)
			}
			if *got != tt.want {
				t.Errorf("parseOutcome() = %+v; want %+v", *got, tt.want)
			}
		})
	}
}

// TestParseOutcome_MissingFile asserts a missing outcome.yaml is a wrapped
// error, not a guessed nil result.
func TestParseOutcome_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outcome.yaml")

	if _, err := parseOutcome(path); err == nil {
		t.Fatalf("parseOutcome() error = nil; want an error for a missing file")
	}
}

// TestArchiveStaleOutcome_AbsentFileIsNoOp asserts archiving a webster dir
// with no outcome.yaml at all returns ("", nil) — not an error.
func TestArchiveStaleOutcome_AbsentFileIsNoOp(t *testing.T) {
	dir := t.TempDir()

	got, err := archiveStaleOutcome(dir, time.Now)
	if err != nil {
		t.Fatalf("archiveStaleOutcome() error = %v; want nil", err)
	}
	if got != "" {
		t.Errorf("archiveStaleOutcome() = %q; want \"\" for an absent file", got)
	}
}

// TestArchiveStaleOutcome_RenamesAndPreservesContent asserts a present
// outcome.yaml is renamed (never copied-and-left, never deleted) to
// outcome-<UTC-compact-timestamp>.yaml in the same directory, with its
// content preserved byte-for-byte, and the original path no longer exists.
func TestArchiveStaleOutcome_RenamesAndPreservesContent(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "outcome.yaml")
	content := "outcome: stuck\nstuck_reason: \"batch 04 red\"\nbatches_done: 3\n"
	outcomeWriteFile(t, original, content)

	clk := archiveFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))
	got, err := archiveStaleOutcome(dir, clk)
	if err != nil {
		t.Fatalf("archiveStaleOutcome() error = %v; want nil", err)
	}

	wantPath := filepath.Join(dir, "outcome-20260711T134500Z.yaml")
	if got != wantPath {
		t.Errorf("archiveStaleOutcome() = %q; want %q", got, wantPath)
	}

	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Errorf("original outcome.yaml still exists after archiving; want it renamed away")
	}

	archived, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", got, err)
	}
	if string(archived) != content {
		t.Errorf("archived content = %q; want %q", archived, content)
	}
}

// TestArchiveStaleOutcome_SameSecondCollisionAppendsSuffix asserts a second
// archive call whose now() truncates to the same compact timestamp does not
// clobber the first archive: it appends a numeric suffix instead.
func TestArchiveStaleOutcome_SameSecondCollisionAppendsSuffix(t *testing.T) {
	dir := t.TempDir()
	clk := archiveFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))

	outcomeWriteFile(t, filepath.Join(dir, "outcome.yaml"), "outcome: done\nstuck_reason: null\nbatches_done: 1\n")
	first, err := archiveStaleOutcome(dir, clk)
	if err != nil {
		t.Fatalf("first archiveStaleOutcome() error = %v; want nil", err)
	}

	// A fresh outcome.yaml, written after the first was archived away, is
	// itself archived a second time within the same clock-second.
	outcomeWriteFile(t, filepath.Join(dir, "outcome.yaml"), "outcome: paused\nstuck_reason: null\nbatches_done: 2\n")
	second, err := archiveStaleOutcome(dir, clk)
	if err != nil {
		t.Fatalf("second archiveStaleOutcome() error = %v; want nil", err)
	}

	if first == second {
		t.Fatalf("second archiveStaleOutcome() = %q; want a distinct path from the first %q", second, first)
	}

	wantSecond := filepath.Join(dir, "outcome-20260711T134500Z-1.yaml")
	if second != wantSecond {
		t.Errorf("second archiveStaleOutcome() = %q; want %q", second, wantSecond)
	}

	firstContent, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("ReadFile(first %q): %v", first, err)
	}
	if !strings.Contains(string(firstContent), "outcome: done") {
		t.Errorf("first archive content = %q; want it to still read \"outcome: done\"", firstContent)
	}

	secondContent, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("ReadFile(second %q): %v", second, err)
	}
	if !strings.Contains(string(secondContent), "outcome: paused") {
		t.Errorf("second archive content = %q; want it to read \"outcome: paused\"", secondContent)
	}
}
