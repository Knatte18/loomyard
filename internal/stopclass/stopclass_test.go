// stopclass_test.go pins each class's verdicts and bound per autonomy level and halt count, and Settle's reading order.
// It also pins the strings of the levels and verdicts.

package stopclass_test

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/stopclass"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	lym      = stopclass.VerdictLym
	orch     = stopclass.VerdictOrch
	operator = stopclass.VerdictOperator
)

// verdictGrid holds a class's verdicts at the first, second and third halt at a row, per level.
type verdictGrid map[stopclass.Level][3]stopclass.Verdict

var (
	// retriedOnce is a class lym retries once, then hands to the orch, and that turns severe at the third halt.
	retriedOnce = verdictGrid{
		stopclass.LevelOff:        {lym, orch, orch},
		stopclass.LevelOn:         {lym, orch, operator},
		stopclass.LevelUnattended: {lym, orch, operator},
	}
	// handedOn is a class the orch takes from the first halt and that turns severe at the third halt.
	handedOn = verdictGrid{
		stopclass.LevelOff:        {orch, orch, orch},
		stopclass.LevelOn:         {orch, orch, operator},
		stopclass.LevelUnattended: {orch, orch, operator},
	}
	// waited is a wait lym settles every time.
	waited = verdictGrid{
		stopclass.LevelOff:        {lym, lym, lym},
		stopclass.LevelOn:         {lym, lym, lym},
		stopclass.LevelUnattended: {lym, lym, lym},
	}
	// orchOnly is a class the orch takes at every halt, with no counted operator case.
	orchOnly = verdictGrid{
		stopclass.LevelOff:        {orch, orch, orch},
		stopclass.LevelOn:         {orch, orch, orch},
		stopclass.LevelUnattended: {orch, orch, orch},
	}
	// alwaysSevere is a class that reaches the operator at every halt under autonomy.
	alwaysSevere = verdictGrid{
		stopclass.LevelOff:        {orch, orch, orch},
		stopclass.LevelOn:         {operator, operator, operator},
		stopclass.LevelUnattended: {operator, operator, operator},
	}
)

// TestSettle_TableRows pins each class's verdicts and bound.
// The nth halt at a row is settled with lym's n-1 earlier settles counted, as when lym settled every earlier halt.
func TestSettle_TableRows(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		class stopclass.Class
		bound int
		want  verdictGrid
	}{
		{stopclass.ClassBoardClaim, 1, retriedOnce},
		{stopclass.ClassPause, stopclass.BoundUnlimited, waited},
		{stopclass.ClassAgentFailure, 1, retriedOnce},
		{stopclass.ClassStaleBuild, 0, handedOn},
		{stopclass.ClassReviewEscalation, 0, handedOn},
		{stopclass.ClassBudgetEscalation, 0, handedOn},
		{stopclass.ClassBounceBudget, 1, retriedOnce},
		{stopclass.ClassGateAttempts, 1, retriedOnce},
		{stopclass.ClassRowDefect, 0, handedOn},
		{stopclass.ClassLanding, 0, handedOn},
		{stopclass.ClassDriverNotParked, stopclass.BoundUnlimited, waited},
		{stopclass.ClassDeadDriver, 0, orchOnly},
		{stopclass.ClassRunQuestion, 0, orchOnly},
		{stopclass.ClassOwnerHalt, 0, alwaysSevere},
		{stopclass.ClassOperatorStep, 0, alwaysSevere},
		{stopclass.ClassUnnamed, 0, handedOn},
	}
	for _, tt := range tests {
		t.Run(string(tt.class), func(t *testing.T) {
			t.Parallel()
			for level, verdicts := range tt.want {
				for index, want := range verdicts {
					halts := index + 1
					got := stopclass.Settle(tt.class, level, halts, halts-1)
					if got.Verdict != want || got.Bound != tt.bound {
						t.Errorf("Settle(%s, %s, %d, %d) = %+v; want {Verdict:%s Bound:%d}",
							tt.class, level, halts, halts-1, got, want, tt.bound)
					}
				}
			}
			if got := stopclass.Settle(tt.class, stopclass.LevelOff, 9, 9).Verdict; got == operator {
				t.Errorf("Settle(%s, false, 9, 9).Verdict = %s; want a verdict other than operator", tt.class, got)
			}
		})
	}

	listed := make(map[stopclass.Class]bool, len(tests))
	for _, tt := range tests {
		listed[tt.class] = true
	}
	held := stopclass.TableClasses()
	for _, class := range held {
		if !listed[class] {
			t.Errorf("TableClasses() holds %s; want a row for it in tests", class)
		}
	}
	for class := range listed {
		if !slices.Contains(held, class) {
			t.Errorf("TableClasses() lacks %s, which tests lists", class)
		}
	}
}

// TestLevelAndVerdict_HoldTheirRecordedStrings pins the strings a config level converts from and a halt records.
func TestLevelAndVerdict_HoldTheirRecordedStrings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"LevelOff", string(stopclass.LevelOff), "false"},
		{"LevelOn", string(stopclass.LevelOn), "true"},
		{"LevelUnattended", string(stopclass.LevelUnattended), "unattended"},
		{"VerdictLym", string(stopclass.VerdictLym), "lym"},
		{"VerdictOrch", string(stopclass.VerdictOrch), "orch"},
		{"VerdictOperator", string(stopclass.VerdictOperator), "operator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("string(%s) = %q; want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestSettle_ReadsOperatorThenLymThenOrch pins the order Settle reads a row in:
// the operator case before lym's bound, and lym's bound before the orch.
func TestSettle_ReadsOperatorThenLymThenOrch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		class      stopclass.Class
		level      stopclass.Level
		haltsAtRow int
		lymSettles int
		want       stopclass.Verdict
	}{
		{"ThirdHaltOutranksLymBound", stopclass.ClassAgentFailure, stopclass.LevelUnattended, 3, 0, operator},
		{"FourthHaltStaysSevere", stopclass.ClassGateAttempts, stopclass.LevelOn, 4, 3, operator},
		{"ThirdHaltAtLevelOffLeavesLymBound", stopclass.ClassAgentFailure, stopclass.LevelOff, 3, 0, lym},
		{"LymBoundOutranksOrch", stopclass.ClassBoardClaim, stopclass.LevelOn, 2, 0, lym},
		{"PastLymBoundGoesToOrch", stopclass.ClassBounceBudget, stopclass.LevelUnattended, 1, 1, orch},
		{"WaitIgnoresHaltCount", stopclass.ClassPause, stopclass.LevelUnattended, 5, 5, lym},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := stopclass.Settle(tt.class, tt.level, tt.haltsAtRow, tt.lymSettles).Verdict; got != tt.want {
				t.Errorf("Settle(%s, %s, %d, %d).Verdict = %s; want %s",
					tt.class, tt.level, tt.haltsAtRow, tt.lymSettles, got, tt.want)
			}
		})
	}
}
