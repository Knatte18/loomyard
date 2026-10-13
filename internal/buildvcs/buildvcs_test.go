// buildvcs_test.go covers the mapping from build settings onto an Identity, its Clean and Label forms, and OfFile's read of a binary on disk.

package buildvcs

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
	"time"
)

func TestIdentityOf(t *testing.T) {
	t.Parallel()
	commitTime := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     Identity
	}{
		{"both keys", []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.modified", Value: "true"}}, Identity{Revision: "abc", Modified: true}},
		{"modified false", []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.modified", Value: "false"}}, Identity{Revision: "abc"}},
		{"no vcs keys", nil, Identity{}},
		{"unrelated keys only", []debug.BuildSetting{{Key: "GOOS", Value: "linux"}, {Key: "-ldflags", Value: "-s"}}, Identity{}},
		{"parseable time", []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.time", Value: "2026-03-04T05:06:07Z"}}, Identity{Revision: "abc", Time: commitTime}},
		{"missing time", []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}}, Identity{Revision: "abc"}},
		{"malformed time", []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.time", Value: "yesterday"}}, Identity{Revision: "abc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := identityOf(tt.settings)
			if got.Revision != tt.want.Revision || got.Modified != tt.want.Modified || !got.Time.Equal(tt.want.Time) {
				t.Errorf("identityOf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOfFile(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() = _, %v; want nil", err)
	}
	if _, err := OfFile(executable); err != nil {
		t.Errorf("OfFile(%q) = _, %v; want nil for the test binary", executable, err)
	}

	text := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(text, []byte("not a binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OfFile(text); err == nil {
		t.Errorf("OfFile(%q) error = nil; want a refusal for a text file", text)
	}
}

func TestIdentity_CleanAndLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		id        Identity
		wantClean bool
		wantLabel string
	}{
		{"stamped and unmodified", Identity{Revision: "0123456789abcdef"}, true, "0123456789ab"},
		{"stamped and modified", Identity{Revision: "0123456789abcdef", Modified: true}, false, "0123456789ab-modified"},
		{"unstamped", Identity{}, false, "unknown"},
		{"unstamped and modified", Identity{Modified: true}, false, "unknown-modified"},
		{"short revision kept whole", Identity{Revision: "abc"}, true, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.id.Clean(); got != tt.wantClean {
				t.Errorf("Clean() = %v, want %v", got, tt.wantClean)
			}
			if got := tt.id.Label(); got != tt.wantLabel {
				t.Errorf("Label() = %q, want %q", got, tt.wantLabel)
			}
		})
	}
}
