// recheck.go — the Drift-Recheck producer: the compare again of code committed after Drift-Verify's last compare, which hands drift back to Drift-Verify.

package driftengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// recheckOpening opens the reason of every Drift-Recheck Stuck.
const recheckOpening = "drift recheck: "

// DriftRecheck is the Drift-Recheck ShedProducer.
// It runs after the rows that may commit code once Drift-Verify passed, and the recipe routes its Stuck to Drift-Verify, which judges the drift again.
type DriftRecheck struct {
	name string
	deps Deps
}

var _ shedengine.ShedProducer = (*DriftRecheck)(nil)

// NewDriftRecheck returns the Drift-Recheck producer identified as name.
func NewDriftRecheck(name string, deps Deps) *DriftRecheck {
	return &DriftRecheck{name: name, deps: deps}
}

// Call runs one Drift-Recheck pass and commits the durable drift directory on every way out but one: with drift switched off it is Done at once, committing nothing.
// A pass with no scope record, scope off, a waiver, or HEAD still at the commit of the last compare is Done with no compare.
// Otherwise it compares the code again: against the accepted profile while tests are accepted, which checks each accepted test against the output its acceptance saw, and against the baseline while none is.
// A passing compare is Done and records the compare at HEAD.
// A pass with no compare recorded yet, a dirty tracked tree, an undeclared compare or a compare that shows drift is a Stuck.
// With tests accepted, a compare against the accepted profile that shows drift, or no accepted profile captured at the last compare's commit, first lapses every acceptance, since the compare cannot tell which accepted test changed, so Drift-Verify compares them again.
func (p *DriftRecheck) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	err := requireSeams(p.name,
		seam{"Config", d.Config != nil},
		seam{"Head", d.Head != nil},
		seam{"DirtyPaths", d.DirtyPaths != nil},
		seam{"Commit", d.Commit != nil},
		seam{"Compare", d.Compare != nil},
		seam{"CompareAccepted", d.CompareAccepted != nil},
	)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if outcome, pointer, stop := passWhenSwitchedOff(d.Config); stop {
		return outcome, pointer, nil
	}
	outcome, pointer, err := p.pass(ctx)
	return commitOnExit(p.name, d.Commit, []string{DirRel()}, outcome, pointer, err)
}

// back returns the Stuck that hands the run back to Drift-Verify, carrying why.
func back(why string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Stuck, shedengine.OutputPointer{Reason: recheckOpening + why}, nil
}

// pass is Call without the commit.
func (p *DriftRecheck) pass(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	scope, found, err := ReadScope(d.Paths)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	switch {
	case !found:
		return doneBecause("no scope is recorded, so there is nothing to compare")
	case scope.Scope == ScopeOff:
		return doneBecause("the scope is off")
	case scope.WaivedAt != "":
		return doneBecause("the compare is waived: " + scope.WaivedReason)
	case scope.ComparedAt == "":
		return back("no compare is recorded, so Drift-Verify compares the code first")
	}

	head, err := d.Head()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if head == scope.ComparedHead {
		return doneBecause(fmt.Sprintf("no commit since the compare at %s", head))
	}
	dirty, err := d.DirtyPaths()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if len(dirty) > 0 {
		return back("the tracked tree is dirty: " + strings.Join(dirty, ", "))
	}
	cfg, err := d.Config()
	if err == nil {
		_, err = cfg.Step(stepCompare)
	}
	if err != nil {
		return back(err.Error())
	}

	if len(scope.Accepted) == 0 {
		result, err := d.Compare(ctx)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if err := p.recordCompare(head, result.Passed, false); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if result.Passed {
			return doneBecause(fmt.Sprintf("the compare against the baseline passed at %s", head))
		}
		return back(fmt.Sprintf("the code changed after the compare at %s, and the compare against the baseline shows drift: %s", scope.ComparedHead, result.Reason))
	}

	tests := acceptedTests(scope)
	if scope.AcceptedProfileHead != scope.ComparedHead {
		if err := p.lapse(); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		return back(fmt.Sprintf("no accepted profile was captured at %s, the commit of the last compare, so the acceptances of %s lapse", scope.ComparedHead, tests))
	}
	result, err := d.CompareAccepted(ctx)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.recordCompare(head, result.Passed, result.Passed); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if result.Passed {
		return doneBecause(fmt.Sprintf("the compare against the accepted profile passed at %s", head))
	}
	if err := p.lapse(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return back(fmt.Sprintf("the code changed after the compare at %s, and the compare against the accepted profile shows drift: %s; the acceptances of %s lapse", scope.ComparedHead, result.Reason, tests))
}

// acceptedTests names the accepted tests of scope, quoted and comma-separated.
func acceptedTests(scope ScopeRecord) string {
	quoted := make([]string, len(scope.Accepted))
	for i, acceptance := range scope.Accepted {
		quoted[i] = "`" + acceptance.Test + "`"
	}
	return strings.Join(quoted, ", ")
}

// recordCompare records in the scope record a compare of head and whether it passed, keeping the baseline's commit.
// profileHolds also records head as the accepted profile's commit, since output equal to the profile's leaves the profile valid at head.
func (p *DriftRecheck) recordCompare(head string, passed, profileHolds bool) error {
	at := time.Now()
	if p.deps.Now != nil {
		at = p.deps.Now()
	}
	return UpdateScope(p.deps.Paths, func(cur ScopeRecord, _ bool) (ScopeRecord, error) {
		cur.ComparedHead, cur.ComparedAt, cur.ComparePassed = head, stamp(at), passed
		if profileHolds {
			cur.AcceptedProfileHead = head
		}
		return cur, nil
	})
}

// lapse removes every acceptance and the accepted profile's commit from the scope record.
func (p *DriftRecheck) lapse() error {
	return UpdateScope(p.deps.Paths, func(cur ScopeRecord, _ bool) (ScopeRecord, error) {
		cur.Accepted, cur.AcceptedProfileHead = nil, ""
		return cur, nil
	})
}
