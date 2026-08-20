package loomcli

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

func TestResolveParentBranch(t *testing.T) {
	tests := []struct {
		name        string
		recorded    fabricengine.Origin
		found       bool
		parentFlag  string
		wantParent  string
		wantWrite   bool
		wantErr     bool
		wantErrText []string
	}{
		{
			name:       "RecordPresent_NonEmpty_NoFlag",
			recorded:   fabricengine.Origin{ParentBranch: "main"},
			found:      true,
			parentFlag: "",
			wantParent: "main",
			wantWrite:  false,
		},
		{
			name:       "RecordPresent_MatchesFlag",
			recorded:   fabricengine.Origin{ParentBranch: "main"},
			found:      true,
			parentFlag: "main",
			wantParent: "main",
			wantWrite:  false,
		},
		{
			name:        "RecordPresent_DisagreesWithFlag",
			recorded:    fabricengine.Origin{ParentBranch: "main"},
			found:       true,
			parentFlag:  "develop",
			wantErr:     true,
			wantErrText: []string{"main", "develop"},
		},
		{
			name:       "NoRecord_WithFlag",
			recorded:   fabricengine.Origin{},
			found:      false,
			parentFlag: "main",
			wantParent: "main",
			wantWrite:  true,
		},
		{
			name:        "NoRecord_NoFlag",
			recorded:    fabricengine.Origin{},
			found:       false,
			parentFlag:  "",
			wantErr:     true,
			wantErrText: []string{"--parent"},
		},
		{
			name:       "RecordPresentButEmpty_WithFlag",
			recorded:   fabricengine.Origin{ParentBranch: ""},
			found:      true,
			parentFlag: "main",
			wantParent: "main",
			wantWrite:  true,
		},
		{
			name:        "RecordPresentButEmpty_NoFlag",
			recorded:    fabricengine.Origin{ParentBranch: ""},
			found:       true,
			parentFlag:  "",
			wantErr:     true,
			wantErrText: []string{"--parent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotParent, gotWrite, err := resolveParentBranch(tt.recorded, tt.found, tt.parentFlag)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveParentBranch(%+v, %v, %q) = nil error; want error", tt.recorded, tt.found, tt.parentFlag)
				}
				for _, want := range tt.wantErrText {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("resolveParentBranch(%+v, %v, %q) error = %q; want it to contain %q", tt.recorded, tt.found, tt.parentFlag, err.Error(), want)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("resolveParentBranch(%+v, %v, %q) unexpected error: %v", tt.recorded, tt.found, tt.parentFlag, err)
			}
			if gotParent != tt.wantParent {
				t.Errorf("resolveParentBranch(%+v, %v, %q) parent = %q; want %q", tt.recorded, tt.found, tt.parentFlag, gotParent, tt.wantParent)
			}
			if gotWrite != tt.wantWrite {
				t.Errorf("resolveParentBranch(%+v, %v, %q) write = %v; want %v", tt.recorded, tt.found, tt.parentFlag, gotWrite, tt.wantWrite)
			}
		})
	}
}

func TestSeedSlug(t *testing.T) {
	tests := []struct {
		name         string
		worktreeName string
		wantSlug     string
	}{
		{"OrdinarySlug", "loom-session-bootstrap", "loom-session-bootstrap"},
		{"Empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := seedSlug(tt.worktreeName)
			if got != tt.wantSlug {
				t.Errorf("seedSlug(%q) = %q; want %q", tt.worktreeName, got, tt.wantSlug)
			}
		})
	}
}
