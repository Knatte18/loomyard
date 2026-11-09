// caps.go defines the loom review perches and resolves their soft and hard bounce caps through reviewbudget.

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
	// PerchSplit is the split perch, the Raddle-Review perch over a raddle proposal.
	PerchSplit Perch = "split"
)

// reviewPerches lists the review perches whose keys loom.yaml holds.
// Config loading, its spec and local-value validation take the perches from here.
// The plan perch's keys live in tapestry.yaml, so it is not listed here.
var reviewPerches = []Perch{PerchDiscussion, PerchCode, PerchSplit}

// reviewPerchNames returns reviewPerches' names as reviewbudget takes them.
func reviewPerchNames() []string {
	names := make([]string, len(reviewPerches))
	for i, perch := range reviewPerches {
		names[i] = string(perch)
	}
	return names
}

// Caps resolves p's soft and hard bounce caps.
// The soft cap is the perch's own `<perch>_max_bounces` when that is non-null, else `review_max_bounces`.
// The hard cap is the perch's own `<perch>_hard_max_bounces` when that is above 0, else `review_hard_max_bounces`.
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
	case PerchSplit:
		ownSoft, ownHard = cfg.SplitMaxBounces, cfg.SplitHardMaxBounces
	}
	return reviewbudget.Resolve(string(p), cfg.ReviewMaxBounces, cfg.ReviewHardMaxBounces, ownSoft, ownHard)
}
