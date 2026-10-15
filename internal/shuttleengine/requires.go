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

// startRequirementsError is the refusal of an unmet requirement at Start.
// It holds the refusal without its way forward and reads with the generic one, so a refusal no caller wraps still names a way forward.
type startRequirementsError struct{ refusal error }

func (e *startRequirementsError) Error() string {
	return e.refusal.Error() + "; way forward: give the role a model alias of an engine that meets them, or for a role that takes no alias set engine: in shuttle.yaml to one that does, then re-run the verb"
}

func (e *startRequirementsError) Unwrap() error { return e.refusal }

// ModelKey names where a role's model came from, so a Start requirements refusal can name the key to change.
// The zero ModelKey stands for a seat whose key no caller names.
type ModelKey struct {
	// Module is the config module whose YAML file holds the key.
	Module string
	// Key is the config key holding the model spec.
	Key string
	// Entry names one entry of a {name, model} list under Key, empty for a scalar model spec.
	Entry string
	// DefaultEngine marks a model that named no engine and resolved no key, so the engine came from shuttle.yaml's default.
	DefaultEngine bool
}

// Wrap returns err unchanged unless its chain holds a Start requirements refusal.
// For one that does, it returns the held refusal with the way forward for this key in place of the generic one; the result still wraps ErrRequirementsUnmet.
func (k ModelKey) Wrap(err error) error {
	var start *startRequirementsError
	if !errors.As(err, &start) {
		return err
	}
	var wayForward string
	switch {
	case k.Module != "" && k.Key != "" && k.Entry != "":
		wayForward = fmt.Sprintf("set the model of %s in %s in %s.yaml to an alias whose engine meets them with lyx config %s, then re-run the verb, or under the role's row re-step the row", k.Entry, k.Key, k.Module, k.Module)
	case k.Module != "" && k.Key != "":
		wayForward = fmt.Sprintf("set %s in %s.yaml to an alias whose engine meets them with lyx config %s --set %s=<alias>, then re-run the verb, or under the role's row re-step the row", k.Key, k.Module, k.Module, k.Key)
	case k.DefaultEngine:
		wayForward = "set engine: in shuttle.yaml to an engine that meets them, then re-run the verb"
	default:
		wayForward = "set the seat's model to an alias whose engine meets them, then re-run the verb, or under the role's row re-step the row"
	}
	return &requirementsError{message: start.refusal.Error() + "; way forward: " + wayForward}
}

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
		return &startRequirementsError{refusal: err}
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
