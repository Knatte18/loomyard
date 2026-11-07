// profile.go defines Profile, treadle's per-block input: resolved plain data only;
// it reads no config file and parses no model spec.
// It also defines Gate and GateMode, the convergence-check vocabulary.
// Profile.validate is the fail-loud structural check run once per block before the loop starts.

package treadleengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// GateMode selects how a round's convergence is decided.
// It is safety-critical and gets no silent default;
// validate rejects any unknown value.
type GateMode string

// The three legal GateMode values.
const (
	GateLLMVerdict GateMode = "llm-verdict"
	GateCommand    GateMode = "command"
	GateBoth       GateMode = "both"
)

// Gate describes the convergence check.
// Mode selects which signal or signals decide that a round is clean.
// Command is the argv to run when Mode consults a command; it runs without a shell, so the argv is portable.
// Timeout bounds how long that command may run.
type Gate struct {
	Mode    GateMode
	Command []string
	Timeout time.Duration
}

// Profile is treadle's per-block input: resolved plain data only.
// ProfileHash is caller-computed identity;
// treadle stamps it into state.json verbatim.
// Gate/GateDir select and locate the convergence check.
// RoundCaps must be resolved and non-empty;
// treadle does no default resolution.
// Judge is the model choice, engine included, of the judge and targeting calls;
// Attempt is the model choice of each round's attempt, and Timeout tunes it.
// PreRoundTargeting gates the optional pre-round targeting capability.
type Profile struct {
	ProfileHash       string
	Gate              Gate
	GateDir           string
	RoundCaps         []int
	Judge             shuttleengine.ModelChoice
	Attempt           shuttleengine.ModelChoice
	Timeout           time.Duration
	PreRoundTargeting bool
}

// validate checks only the structural invariants treadle owns.
// It requires a non-empty ProfileHash and a non-empty, strictly increasing RoundCaps ladder.
// It checks that Gate.Mode is legal, with its command-emptiness cross-checks,
// and that GateDir is set when the mode runs a command.
// It rejects negative timeouts.
// Every message is prefixed with name, the calling engine's name, so the diagnostics name the engine that failed.
func (p *Profile) validate(name string) error {
	if p.ProfileHash == "" {
		return fmt.Errorf("%s: profile.ProfileHash must not be empty", name)
	}

	if len(p.RoundCaps) == 0 {
		return fmt.Errorf("%s: profile.RoundCaps must not be empty", name)
	}
	for i, roundCap := range p.RoundCaps {
		if roundCap < 1 {
			return fmt.Errorf("%s: profile.RoundCaps entries must all be >= 1, got %d at index %d", name, roundCap, i)
		}
		if i > 0 && roundCap <= p.RoundCaps[i-1] {
			return fmt.Errorf("%s: profile.RoundCaps must be strictly increasing, got %d at index %d following %d", name, roundCap, i, p.RoundCaps[i-1])
		}
	}

	switch p.Gate.Mode {
	case GateLLMVerdict:
		if len(p.Gate.Command) != 0 {
			return fmt.Errorf("%s: profile.Gate.Mode = %q must not set Gate.Command (got %v)", name, GateLLMVerdict, p.Gate.Command)
		}
	case GateCommand, GateBoth:
		if len(p.Gate.Command) == 0 {
			return fmt.Errorf("%s: profile.Gate.Mode = %q requires a non-empty Gate.Command", name, p.Gate.Mode)
		}
		if p.GateDir == "" {
			return fmt.Errorf("%s: profile.GateDir must not be empty when Gate.Mode runs a command", name)
		}
	default:
		return fmt.Errorf("%s: profile.Gate.Mode must be %q, %q, or %q, got %q", name, GateLLMVerdict, GateCommand, GateBoth, p.Gate.Mode)
	}

	if p.Gate.Timeout < 0 {
		return fmt.Errorf("%s: profile.Gate.Timeout must not be negative (got %s)", name, p.Gate.Timeout)
	}
	if p.Timeout < 0 {
		return fmt.Errorf("%s: profile.Timeout must not be negative (got %s)", name, p.Timeout)
	}

	return nil
}
