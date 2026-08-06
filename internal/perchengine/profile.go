// profile.go defines Profile, the content contract for one perch block: the embedded burler content
// fields (what to review and how a round may fix it) plus the perch-owned gate/caps/tuning keys
// that drive the loop itself.
// It also defines Gate and GateMode, the convergence-check vocabulary, and Profile.validate, the
// fail-loud default-resolution and check pass that runs once per block before the loop starts.

package perchengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// GateMode selects how a round's convergence is decided.
type GateMode string

// The three legal GateMode values.
const (
	// GateLLMVerdict treats a fresh round's burler verdict as the sole convergence signal: clean means
	// the round's review came back burlerengine.VerdictApproved.
	GateLLMVerdict GateMode = "llm-verdict"
	// GateCommand ignores the burler verdict for convergence and instead runs Gate.Command after each
	// round's fix phase;
	// a zero exit is clean.
	// The burler review still drives what the fix phase changes — only convergence is decided
	// elsewhere.
	GateCommand GateMode = "command"
	// GateBoth requires both signals: the burler verdict must be VerdictApproved AND Gate.Command must
	// exit zero.
	GateBoth GateMode = "both"
)

// Gate describes the convergence check for a perch block.
type Gate struct {
	Mode    GateMode
	Command []string
	Timeout time.Duration
}

// defaultRoundCaps is the built-in milestone ladder when neither profile nor Config sets one.
var defaultRoundCaps = []int{5, 8, 10}

// The built-in defaults validate falls back to when a field is left
// unresolved by both the profile and Config.
const (
	defaultJudgeModel  = "haiku"
	defaultGateTimeout = 10 * time.Minute
)

// Profile is the content contract for one perch block: burler content fields plus perch-owned
// fields driving the round loop.
type Profile struct {
	// Target, Fasit, Rubric, FixScope, ToolUse, and ClusterFan are the
	// burler content fields, carried 1:1 into every round's
	// burlerengine.Profile by buildRoundProfile. validate below does not
	// check these — they are validated by burlerengine.Profile.validate
	// inside the first round's Engine.Run, which is the single place that
	// already enforces them (including ClusterFan's fan resolution).
	Target     burlerengine.FileSet
	Fasit      burlerengine.FileSet
	Rubric     string
	FixScope   burlerengine.FixScope
	ToolUse    bool
	ClusterFan string

	// Gate selects and configures the block's convergence check.
	Gate Gate
	// RoundCaps is the milestone ladder: strictly increasing round numbers,
	// the last of which is the hard cap. Nil (unset) resolves through Config
	// then defaultRoundCaps; a non-nil EMPTY ladder is an explicit operator
	// mistake and fails validation loud rather than silently defaulting.
	RoundCaps []int
	// JudgeModel and JudgeEffort tune the ephemeral progress-judge and
	// asking-triage calls. Empty JudgeModel resolves through Config then
	// defaultJudgeModel; empty JudgeEffort resolves through Config only (no
	// built-in default — an empty effort means "provider default").
	JudgeModel  string
	JudgeEffort string
	// Model, Effort, and Timeout tune every burler round uniformly (mapped
	// onto each round's burlerengine.RunOpts). Timeout of zero defers to the
	// shuttle config default, exactly like RunOpts.Timeout already does.
	Model   string
	Effort  string
	Timeout time.Duration
}

// validate resolves perch-owned defaults and reports fail-loud error if the profile is not runnable.
func (p *Profile) validate(cfg Config) error {
	// A nil RoundCaps means "unset — resolve through the default chain"; a
	// non-nil EMPTY ladder is an explicit `round-caps: []` the operator
	// wrote, and silently substituting the default for it would hide a
	// mangled profile — fail loud instead (the discussion pins empty as a
	// profile error, distinct from absent).
	if p.RoundCaps != nil && len(p.RoundCaps) == 0 {
		return fmt.Errorf("perch: profile.RoundCaps must not be an explicit empty list; omit the key to use the default ladder")
	}
	if len(p.RoundCaps) == 0 {
		p.RoundCaps = cfg.RoundCaps
	}
	if len(p.RoundCaps) == 0 {
		p.RoundCaps = defaultRoundCaps
	}
	if p.JudgeModel == "" {
		p.JudgeModel = cfg.JudgeModel
	}
	if p.JudgeModel == "" {
		p.JudgeModel = defaultJudgeModel
	}
	if p.JudgeEffort == "" {
		p.JudgeEffort = cfg.JudgeEffort
	}
	if p.Gate.Timeout == 0 {
		p.Gate.Timeout = defaultGateTimeout
	}

	// RoundCaps entries must all be positive round numbers and strictly
	// increasing — a non-increasing or non-positive entry would let the
	// loop stall or skip milestone rungs silently. A single-element ladder
	// trivially satisfies both conditions (plain hard cap).
	for i, roundCap := range p.RoundCaps {
		if roundCap < 1 {
			return fmt.Errorf("perch: profile.RoundCaps entries must all be >= 1, got %d at index %d", roundCap, i)
		}
		if i > 0 && roundCap <= p.RoundCaps[i-1] {
			return fmt.Errorf("perch: profile.RoundCaps must be strictly increasing, got %d at index %d following %d", roundCap, i, p.RoundCaps[i-1])
		}
	}

	// Gate.Mode selects safety-critical behavior (what decides convergence)
	// and gets no silent default, mirroring burlerengine.FixScope's posture.
	// Command-mode fields are cross-checked here too: a command mode with no
	// argv can never run, and an llm-verdict mode with an argv is very
	// likely an operator's leftover value from a mode they switched away
	// from — both are profile errors, not silently-ignored fields.
	switch p.Gate.Mode {
	case GateLLMVerdict:
		if len(p.Gate.Command) != 0 {
			return fmt.Errorf("perch: profile.Gate.Mode = %q must not set Gate.Command (got %v)", GateLLMVerdict, p.Gate.Command)
		}
	case GateCommand, GateBoth:
		if len(p.Gate.Command) == 0 {
			return fmt.Errorf("perch: profile.Gate.Mode = %q requires a non-empty Gate.Command", p.Gate.Mode)
		}
	default:
		return fmt.Errorf("perch: profile.Gate.Mode must be %q, %q, or %q, got %q", GateLLMVerdict, GateCommand, GateBoth, p.Gate.Mode)
	}

	if p.Gate.Timeout < 0 {
		return fmt.Errorf("perch: profile.Gate.Timeout must not be negative (got %s)", p.Gate.Timeout)
	}
	if p.Timeout < 0 {
		return fmt.Errorf("perch: profile.Timeout must not be negative (got %s)", p.Timeout)
	}

	return nil
}
