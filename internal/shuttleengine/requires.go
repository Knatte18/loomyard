// requires.go checks a run's Spec.Requires against the engine that would drive it, at Start and at attach.
// The checks only refuse runs: an empty list with no ForkSubagents passes every engine.

package shuttleengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrRequirementsUnmet is wrapped by every refusal of a run whose Spec.Requires names an unknown requirement or one its engine does not meet.
var ErrRequirementsUnmet = errors.New("shuttle: the engine does not meet the run's requirements")

// requirementsError is a refusal that reads as its message and matches ErrRequirementsUnmet.
type requirementsError struct{ message string }

func (e *requirementsError) Error() string { return e.message }

func (e *requirementsError) Unwrap() error { return ErrRequirementsUnmet }

// knownRequirements lists the closed set of requirements, in the order a refusal names them.
var knownRequirements = []Requirement{RequirementForks, RequirementPeerMessaging, RequirementBackgroundWake}

// knownRequirement reports whether req is one of the Requirement constants.
func knownRequirement(req Requirement) bool {
	return slices.Contains(knownRequirements, req)
}

// requiredOf returns listed plus RequirementForks when forks is true, as a deduplicated set in first-seen order.
func requiredOf(listed []Requirement, forks bool) []Requirement {
	var required []Requirement
	for _, req := range listed {
		if !slices.Contains(required, req) {
			required = append(required, req)
		}
	}
	if forks && !slices.Contains(required, RequirementForks) {
		required = append(required, RequirementForks)
	}
	return required
}

// meetsAll reports whether engine meets every requirement in required through RequirementSupporter.
func meetsAll(engine Engine, required []Requirement) bool {
	supporter, ok := engine.(RequirementSupporter)
	for _, req := range required {
		if !ok || !supporter.Supports(req) {
			return false
		}
	}
	return true
}

// unmetRequirements returns nil when engine meets every requirement in required.
// Otherwise it returns an error wrapping ErrRequirementsUnmet that names engineName, each unmet requirement and the registered engines that meet them all, "none" when no engine does.
func unmetRequirements(engines EngineSet, engineName string, engine Engine, required []Requirement) error {
	supporter, _ := engine.(RequirementSupporter)
	var unmet []string
	for _, req := range required {
		if supporter == nil || !supporter.Supports(req) {
			unmet = append(unmet, string(req))
		}
	}
	if len(unmet) == 0 {
		return nil
	}
	var meeting []string
	for _, name := range engines.Names() {
		if candidate, err := engines.Lookup(name); err == nil && meetsAll(candidate, required) {
			meeting = append(meeting, name)
		}
	}
	meetingList := "none"
	if len(meeting) > 0 {
		meetingList = strings.Join(meeting, ", ")
	}
	return &requirementsError{message: fmt.Sprintf("shuttle: engine %q does not meet the required %s; engines that meet them all: %s", engineName, strings.Join(unmet, ", "), meetingList)}
}

// refuseUnknownRequirement returns a refusal naming the first requirement in listed that is not a known one, nil when all are known.
// verb names the operation (start or attach) in the message.
func refuseUnknownRequirement(verb string, listed []Requirement) error {
	for _, req := range listed {
		if knownRequirement(req) {
			continue
		}
		names := make([]string, len(knownRequirements))
		for i, known := range knownRequirements {
			names[i] = string(known)
		}
		return &requirementsError{message: fmt.Sprintf("shuttle: %s: unknown requirement %q; known requirements: %s", verb, string(req), strings.Join(names, ", "))}
	}
	return nil
}

// checkRequirementsAtStart refuses a start whose spec lists an unknown requirement or names one the resolved engine does not meet.
func (r *Runner) checkRequirementsAtStart(spec Spec, engine Engine, engineName string) error {
	if err := refuseUnknownRequirement("start", spec.Requires); err != nil {
		return err
	}
	err := unmetRequirements(r.engines, engineName, engine, requiredOf(spec.Requires, spec.ForkSubagents))
	if err != nil {
		return fmt.Errorf("%w; way forward: give the role a model alias of an engine that meets them", err)
	}
	return nil
}

// checkRequirementsAtAttach refuses an attach of the run candidate records when spec lists an unknown requirement or the engine the record names does not meet one.
// The record's ForkSubagents joins the spec's list; a ForkSubagents on the spec alone adds nothing, since the live run keeps its recorded engine.
func (r *Runner) checkRequirementsAtAttach(spec Spec, candidate attachCandidate, minAge time.Duration) error {
	if err := refuseUnknownRequirement("attach", spec.Requires); err != nil {
		return err
	}
	engine, engineName, err := r.engineFor(candidate.state)
	if err != nil {
		return err
	}
	err = unmetRequirements(r.engines, engineName, engine, requiredOf(spec.Requires, candidate.state.ForkSubagents))
	if err != nil {
		return fmt.Errorf("%w; way forward: end the run's live strand with \"lyx reed remove %s\", then re-step the row once the run record in %s is older than twice startup_timeout_s (%s)", err, candidate.state.StrandGUID, candidate.runDir, minAge)
	}
	return nil
}
