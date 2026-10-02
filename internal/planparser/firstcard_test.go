package planparser_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestCheckFirstCard(t *testing.T) {
	t.Run("EqualHasNoFinding", func(t *testing.T) {
		if got := planparser.CheckFirstCard(&planparser.Plan{FirstCard: 5}, 5); len(got) != 0 {
			t.Errorf("CheckFirstCard() = %v; want none", got)
		}
	})
	t.Run("DifferingNamesBothNumbers", func(t *testing.T) {
		got := planparser.CheckFirstCard(&planparser.Plan{FirstCard: 3}, 7)
		if len(got) != 1 || got[0].Check != "rework-first-card" {
			t.Fatalf("CheckFirstCard() = %v; want one rework-first-card finding", got)
		}
		if !strings.Contains(got[0].Detail, "3") || !strings.Contains(got[0].Detail, "7") {
			t.Errorf("Detail = %q; want both numbers named", got[0].Detail)
		}
	})
	t.Run("AbsentMatchesToldOne", func(t *testing.T) {
		// An absent first_card parses to 1.
		if got := planparser.CheckFirstCard(&planparser.Plan{FirstCard: 1}, 1); len(got) != 0 {
			t.Errorf("CheckFirstCard() = %v; want none", got)
		}
	})
}
