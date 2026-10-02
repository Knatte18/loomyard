// byname_test.go pins guidByName's unknown, ambiguous and unique outcomes over a plain strand list,
// touching no tmux.

package reedengine

import (
	"strings"
	"testing"
)

func TestGuidByName(t *testing.T) {
	strands := []StrandStatus{
		{GUID: "g1", Name: "driver"},
		{GUID: "g2", Name: "status"},
		{GUID: "g3", Name: "twin"},
		{GUID: "g4", Name: "twin"},
	}

	if got, err := guidByName(strands, "driver"); err != nil || got != "g1" {
		t.Errorf("guidByName(driver) = %q, %v; want g1, nil", got, err)
	}
	if _, err := guidByName(strands, "nope"); err == nil || !strings.Contains(err.Error(), "no strand named") {
		t.Errorf("unknown name error = %v; want a no-strand refusal", err)
	}
	if _, err := guidByName(strands, "twin"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("ambiguous name error = %v; want an ambiguity refusal", err)
	}
}

func TestGuidByName_RoleSegmentFullNameAndLegacyName(t *testing.T) {
	strands := []StrandStatus{
		{GUID: "g1", Name: "tc:tslug:driver"},
		{GUID: "g2", Name: "tc:tslug:orch-2"},
		{GUID: "g3", Name: "status:1:abc12345"},
	}

	tests := []struct {
		query string
		want  string
	}{
		{"driver", "g1"},
		{"tc:tslug:driver", "g1"},
		{"orch-2", "g2"},
		{"status:1:abc12345", "g3"},
	}
	for _, tt := range tests {
		got, err := guidByName(strands, tt.query)
		if err != nil || got != tt.want {
			t.Errorf("guidByName(%q) = %q, %v; want %q, nil", tt.query, got, err, tt.want)
		}
	}
	if _, err := guidByName(strands, "orch"); err == nil {
		t.Error("guidByName(orch) = nil error; want no match, since orch-2 is a different role segment")
	}
}
