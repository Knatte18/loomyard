//go:build tier0

package machinename

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestFromHostname(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		host    string
		want    string
		wantErr bool
	}{
		{"Lowercased", "Neve", "neve", false},
		{"CutAtFirstDot", "neve.lan", "neve", false},
		{"TrimmedAndLowercased", " Box ", "box", false},
		{"Empty", "", "", true},
		{"OnlyDomain", ".local", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := FromHostname(tt.host)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FromHostname(%q) error = %v; wantErr %v", tt.host, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("FromHostname(%q) = %q; want %q", tt.host, got, tt.want)
			}
		})
	}
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	t.Parallel()
	scankit.AssertImportAllowlist(t, "internal/machinename")
}

func TestSoleSource_NoOtherProductionFileReadsTheHostname(t *testing.T) {
	t.Parallel()
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal", "cmd"}, Filter: scankit.Prod}, func(f *scankit.File) {
		if strings.HasPrefix(f.Rel, "internal/machinename/") {
			return
		}
		if strings.Contains(string(f.Data), "os.Hostname(") {
			t.Errorf("%s reads the hostname; only internal/machinename does", f.Rel)
		}
	})
	scankit.RequireFloor(t, scanned, 100, "production files under internal/ and cmd/")
}
