// profile.go defines Profile, treadle's per-block input contract: resolved plain data only (no
// config file, no model-spec parsing — see the treadle-owns-no-config shared decision), plus Gate
// and GateMode, the convergence-check vocabulary, and Profile.validate, the fail-loud structural
// check treadle runs once per block before the loop starts.

package treadleengine

import (
	"fmt"
	"time"
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

// Gate describes the convergence check: which signal(s) decide a round is clean (Mode), the argv to
// run when Mode consults a command (Command — no shell, so argv is portable), and how long the
// command may run (Timeout).
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
// JudgeModel/ JudgeEffort tune judge/triage calls;
// Model/Effort/Timeout tune each round's attempt.
// PreRoundTargeting gates the optional pre-round targeting capability.
type Profile struct {
	ProfileHash       string
	Gate              Gate
	GateDir           string
	RoundCaps         []int
	JudgeModel        string
	JudgeEffort       string
	Model             string
	Effort            string
	Timeout           time.Duration
	PreRoundTargeting bool
}

// validate checks only the structural invariants treadle itself owns: a
// non-empty ProfileHash, a non-empty strictly increasing RoundCaps ladder, a
// legal Gate.Mode with its command-emptiness cross-checks, a non-empty GateDir when
// Gate.Mode runs a command, and non-negative timeouts. Every message is
// prefixed with name (the calling engine's own name, e.g. "tenter") so a
// caller's diagnostics read exactly like today's engine-specific errors —
// the name-parameterized-diagnostics shared decision.
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
