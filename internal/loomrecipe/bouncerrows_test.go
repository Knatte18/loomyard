package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
)

func TestBouncerRunSubdir(t *testing.T) {
	bouncers := map[string]string{
		loomshed.NameDiscussionBouncer: "discussion",
		loomshed.NamePlanBouncer:       "plan",
		loomshed.NameWebsterBouncer:    "webster",
	}
	for row, want := range bouncers {
		got, ok, err := BouncerRunSubdir(row)
		if err != nil || !ok || got != want {
			t.Errorf("BouncerRunSubdir(%q) = %q, %v, %v; want %q, true, nil", row, got, ok, err, want)
		}
	}

	for _, row := range []string{loomshed.NameDiscussionBurler, loomshed.NamePRGate, "No-Such-Row"} {
		got, ok, err := BouncerRunSubdir(row)
		if err != nil || ok || got != "" {
			t.Errorf("BouncerRunSubdir(%q) = %q, %v, %v; want empty, false, nil", row, got, ok, err)
		}
	}
}
