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
