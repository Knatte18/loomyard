//go:build tier0

package shedadapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestGateFailedReason_NamesAttemptsAndFindingsPath pins that both the attempt count and the findings path are interpolated.
// The producers' own gate-failed fixtures (zero attempts, often no findings path) cannot tell that apart from a fixed string.
// It also pins that a told findings path receives the findings.
//
//testtiming:keep pins that the attempt count and the findings path are both interpolated into the gate-failed reason
func TestGateFailedReason_NamesAttemptsAndFindingsPath(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", gateFindingsReasonLimit+50)
	tests := []struct {
		name string
		gate *shuttleengine.GateOutcome
		// path builds the told findings path under dir, and nil tells none.
		path func(dir string) string
		// before plants files under dir before the call.
		before func(t *testing.T, dir string)
		// wantReason may hold {path}, which stands for the told path.
		wantReason string
		// wantReasonPrefix is the start of a reason that holds an OS error text, and "" means wantReason is exact.
		wantReasonPrefix string
		// wantReasonSuffix is the end of that reason.
		wantReasonSuffix string
		// wantFile is the text the told path holds afterwards, and "" means no file is left.
		wantFile string
	}{
		{
			name:       "empty path names the gate's own findings path",
			gate:       &shuttleengine.GateOutcome{Attempts: 3, FindingsPath: "/run/gate-findings.md", Findings: "ignored"},
			wantReason: "gate did not pass after 3 attempts; findings: /run/gate-findings.md",
		},
		{
			name:       "told path receives the findings and is named",
			gate:       &shuttleengine.GateOutcome{Attempts: 3, FindingsPath: "/run/gate-findings.md", Findings: "first findings"},
			path:       func(dir string) string { return filepath.Join(dir, "gate-findings", "row.md") },
			wantReason: "gate did not pass after 3 attempts; findings: {path}",
			wantFile:   "first findings",
		},
		{
			name: "a second exhausted outcome overwrites the file",
			gate: &shuttleengine.GateOutcome{Attempts: 2, Findings: "second findings"},
			path: func(dir string) string { return filepath.Join(dir, "row.md") },
			before: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "row.md"), []byte("first findings that are longer"), 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			},
			wantReason: "gate did not pass after 2 attempts; findings: {path}",
			wantFile:   "second findings",
		},
		{
			name:       "terminal outcome returns its reason and writes no file",
			gate:       &shuttleengine.GateOutcome{Attempts: 1, Reason: "verify is red", Findings: "never written"},
			path:       func(dir string) string { return filepath.Join(dir, "row.md") },
			wantReason: "verify is red",
		},
		{
			name: "a path under a regular file carries the findings text instead",
			gate: &shuttleengine.GateOutcome{Attempts: 1, Findings: "line one\n\n  line two " + long},
			path: func(dir string) string { return filepath.Join(dir, "blocker", "row.md") },
			before: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "blocker"), []byte("a file"), 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			},
			wantReasonPrefix: "findings could not be kept at {path}: ",
			wantReasonSuffix: "; findings: line one line two " + strings.Repeat("x", gateFindingsReasonLimit-len("line one line two ")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := ""
			if tt.path != nil {
				path = tt.path(dir)
			}
			if tt.before != nil {
				tt.before(t, dir)
			}

			got := gateFailedReason(tt.gate, path)

			wantReason := strings.ReplaceAll(tt.wantReason, "{path}", path)
			if tt.wantReasonPrefix != "" {
				prefix := strings.ReplaceAll(tt.wantReasonPrefix, "{path}", path)
				if !strings.HasPrefix(got, prefix) || !strings.HasSuffix(got, tt.wantReasonSuffix) {
					t.Errorf("gateFailedReason(%+v) = %q; want prefix %q and suffix %q", tt.gate, got, prefix, tt.wantReasonSuffix)
				}
			} else if got != wantReason {
				t.Errorf("gateFailedReason(%+v) = %q; want %q", tt.gate, got, wantReason)
			}
			if path == "" {
				return
			}
			content, err := os.ReadFile(path)
			switch {
			case tt.wantFile == "" && err == nil:
				t.Errorf("findings file holds %q; want none", content)
			case tt.wantFile != "" && string(content) != tt.wantFile:
				t.Errorf("findings file = %q (%v); want %q", content, err, tt.wantFile)
			}
		})
	}
}

// TestClearGateFindings pins that an existing file is removed, that an absent file or an empty path is no error,
// and that a removal failure leaves the target in place without panicking.
func TestClearGateFindings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// setup builds the path under dir, creating what the case needs.
		setup func(t *testing.T, dir string) string
		// wantGone reports whether the path must not exist afterwards.
		wantGone bool
	}{
		{
			name: "an existing file is removed",
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				path := filepath.Join(dir, "row.md")
				if err := os.WriteFile(path, []byte("findings"), 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
				return path
			},
			wantGone: true,
		},
		{
			name:     "an absent file is no error",
			setup:    func(t *testing.T, dir string) string { return filepath.Join(dir, "absent.md") },
			wantGone: true,
		},
		{
			name:  "an empty path does nothing",
			setup: func(t *testing.T, dir string) string { return "" },
		},
		{
			name: "a non-empty directory fails the removal and stays",
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				path := filepath.Join(dir, "row.md")
				if err := os.MkdirAll(filepath.Join(path, "inner"), 0o755); err != nil {
					t.Fatalf("MkdirAll() error = %v", err)
				}
				return path
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.setup(t, t.TempDir())

			clearGateFindings(path)

			if path == "" {
				return
			}
			_, err := os.Stat(path)
			if gone := os.IsNotExist(err); gone != tt.wantGone {
				t.Errorf("after clearGateFindings(%q) path gone = %v; want %v", path, gone, tt.wantGone)
			}
		})
	}
}
