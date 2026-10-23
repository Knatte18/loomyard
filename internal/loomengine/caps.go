// caps.go — the loom review perches and their resolved soft and hard bounce caps, resolved through reviewbudget.

package loomengine

import (
	"github.com/Knatte18/loomyard/internal/reviewbudget"
)

// Perch names a loom review perch by its loom.yaml key prefix.
type Perch string

const (
	// PerchDiscussion is the Discussion-Review perch.
	PerchDiscussion Perch = "discussion"
	// PerchPlan is the Plan-Review perch.
	PerchPlan Perch = "plan"
	// PerchCode is the Code-Review perch.
	PerchCode Perch = "code"
)

// reviewPerches lists the loom review perches, the one place ConfigSpec, LoadConfig and ValidateLocalValues take them from.
var reviewPerches = []Perch{PerchDiscussion, PerchPlan, PerchCode}

// reviewPerchNames returns reviewPerches' names as reviewbudget takes them.
func reviewPerchNames() []string {
	names := make([]string, len(reviewPerches))
	for i, perch := range reviewPerches {
		names[i] = string(perch)
	}
	return names
}

// Caps resolves p's soft and hard cap: the perch's own `<perch>_max_bounces` when non-null, else `review_max_bounces`, and its own `<perch>_hard_max_bounces` when above 0, else `review_hard_max_bounces`.
func (cfg Config) Caps(p Perch) reviewbudget.Caps {
	var ownSoft *int
	var ownHard int
	switch p {
	case PerchDiscussion:
		ownSoft, ownHard = cfg.DiscussionMaxBounces, cfg.DiscussionHardMaxBounces
	case PerchPlan:
		ownSoft, ownHard = cfg.PlanMaxBounces, cfg.PlanHardMaxBounces
	case PerchCode:
		ownSoft, ownHard = cfg.CodeMaxBounces, cfg.CodeHardMaxBounces
	}
	return reviewbudget.Resolve(string(p), cfg.ReviewMaxBounces, cfg.ReviewHardMaxBounces, ownSoft, ownHard)
}
