package planparser

import "fmt"

// CheckFirstCard reports one rework-first-card finding when plan's first_card differs from told, the number Go told the rework session to start numbering at.
// An absent first_card reads as 1, so it matches a told 1.
// It is outside both ValidateFormat and Validate; the rework gate alone runs it.
func CheckFirstCard(plan *Plan, told int) []ValidationError {
	if plan.FirstCard == told {
		return nil
	}
	return []ValidationError{{
		Check:  "rework-first-card",
		Detail: fmt.Sprintf("first_card is %d, but the rework round's first card number is %d; set first_card: %d in the overview", plan.FirstCard, told, told),
	}}
}
