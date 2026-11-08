// reexecguard_test.go pins which arguments make Main refuse to run the suite.

package tmuxkit

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRefusesReexec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		args        []string
		wantRefused bool
	}{
		{"NoArguments", nil, false},
		{"TestFlagsOnly", []string{"-test.run=TestFoo", "-test.v"}, false},
		{"TwoTokenFlagValue", []string{"-test.run", "TestFoo"}, false},
		{"LymVerb", []string{"slack", "bar"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer

			refused := refusesReexec(&stderr, tt.args)

			if refused != tt.wantRefused {
				t.Errorf("refusesReexec(%q) = %v; want %v", tt.args, refused, tt.wantRefused)
			}
			if wrote := stderr.Len() != 0; wrote != tt.wantRefused {
				t.Errorf("refusesReexec(%q) wrote stderr %q; want a refusal only when it refuses", tt.args, stderr.String())
			}
		})
	}
}
