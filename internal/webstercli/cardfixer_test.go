// cardfixer_test.go covers cardFixerOn, which decides whether the fork and recovery prompts name the card-gate fixer.

package webstercli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCardFixerOn(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		gate gateslot.Config
		slug string
		want bool
	}{
		{"CardKeyWithAFixerAndASlug", gateslot.Config{Gates: map[string]gateslot.GateKey{"card": {Fixer: "sonnet[medium]"}}}, "task", true},
		{"EmptySlug", gateslot.Config{Gates: map[string]gateslot.GateKey{"card": {Fixer: "sonnet[medium]"}}}, "", false},
		{"CardKeyWithAnEmptyFixer", gateslot.Config{Gates: map[string]gateslot.GateKey{"card": {}}}, "task", false},
		{"ConfigWithNoCardKey", gateslot.Config{Gates: map[string]gateslot.GateKey{"batch": {}}}, "task", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := cardFixerOn(tt.gate, tt.slug); got != tt.want {
				t.Errorf("cardFixerOn() = %v; want %v", got, tt.want)
			}
		})
	}
}
