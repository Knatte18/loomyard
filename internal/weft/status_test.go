//go:build integration

// status_test.go — tests for weft status reporting.

package weft

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		modify    bool
		wantDirty bool
		wantBranch string
	}{
		{"BranchReporting_Clean", false, false, "main"},
		{"DirtyFlag_Clean", false, false, "main"},
		{"DirtyFlag_Modified", true, true, "main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := lyxtest.CopyWeft(t)
			weftRepo := fixture.WeftPath

			if tt.modify {
				lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
				if err := os.WriteFile(lyxFile, []byte("modified"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			status, err := Status(weftRepo, "", "", []string{"_lyx"})
			if err != nil {
				t.Fatalf("Status: %v", err)
			}

			branch, ok := status["branch"].(string)
			if !ok || branch == "" {
				t.Errorf("status[branch] should be a non-empty string; got %v", status["branch"])
			}
			if branch != tt.wantBranch {
				t.Errorf("branch = %q; want %q", branch, tt.wantBranch)
			}

			dirty, ok := status["dirty"].(bool)
			if !ok {
				t.Errorf("status[dirty] should be a bool; got %v", status["dirty"])
			}
			if dirty != tt.wantDirty {
				t.Errorf("dirty = %v; want %v", dirty, tt.wantDirty)
			}
		})
	}
}

func TestStatus_Junction(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*testing.T, string, string) string // returns hostLink
		wantJunctionOk bool
		wantReason     string
		parallel       bool
		skip           string
	}{
		{
			name: "Missing",
			setup: func(t *testing.T, weftRepo, weftLyxDir string) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "_lyx")
			},
			wantJunctionOk: false,
			wantReason:     "host _lyx junction missing",
			parallel:       true,
		},
		{
			name: "PlainDir",
			setup: func(t *testing.T, weftRepo, weftLyxDir string) string {
				tmpDir := t.TempDir()
				hostLink := filepath.Join(tmpDir, "_lyx")
				if err := os.MkdirAll(hostLink, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				return hostLink
			},
			wantJunctionOk: false,
			wantReason:     "host _lyx is not a junction",
			parallel:       true,
		},
		{
			name: "ValidSymlink",
			setup: func(t *testing.T, weftRepo, weftLyxDir string) string {
				tmpDir := t.TempDir()
				hostLink := filepath.Join(tmpDir, "_lyx")
				err := os.Symlink(weftLyxDir, hostLink)
				if err != nil {
					t.Skipf("os.Symlink failed: %v", err)
				}
				return hostLink
			},
			wantJunctionOk: true,
			wantReason:     "",
			parallel:       true,
			skip:           "SKIP_SYMLINK_TEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip != "" && os.Getenv(tt.skip) == "1" {
				t.Skip(tt.skip + " is set")
			}
			if tt.parallel {
				t.Parallel()
			}

			fixture := lyxtest.CopyWeft(t)
			weftRepo := fixture.WeftPath
			weftLyxDir := filepath.Join(weftRepo, "_lyx")

			hostLink := tt.setup(t, weftRepo, weftLyxDir)

			status, err := Status(weftRepo, hostLink, weftLyxDir, []string{"_lyx"})
			if err != nil {
				t.Fatalf("Status: %v", err)
			}

			junctionOk, ok := status["junction_ok"].(bool)
			if !ok {
				t.Errorf("status[junction_ok] should be a bool; got %v", status["junction_ok"])
			}
			if junctionOk != tt.wantJunctionOk {
				t.Errorf("junction_ok = %v; want %v", junctionOk, tt.wantJunctionOk)
			}

			reason, ok := status["junction_reason"].(string)
			if !ok {
				t.Errorf("status[junction_reason] should be a string; got %v", status["junction_reason"])
			}
			if reason != tt.wantReason {
				t.Errorf("junction_reason = %q; want %q", reason, tt.wantReason)
			}
		})
	}
}
