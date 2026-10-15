// driver.go implements ResolveDriver, the driver segment's config-to-settings resolver: a pure
// composer that parses and resolves the driver role's model-spec, modelled on ResolveReview in
// internal/loomengine/review.go.
// Unlike ReviewSettings, the driver's resolved choice carries no Timeout: shuttleengine.Spec.Timeout feeds
// Run.deadline, which only Wait reads, and the driver path deliberately never calls Wait, so a
// timeout key would configure nothing.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// LoomDriverStrandName is the loom driver session's own strand's stable identity, the role the hub addresses as "<slug>:driver".
// It is pinned because reed's add has no upsert semantics and refuses an add whose explicit role another strand already holds.
// Every add and every lookup must use this exact constant, or a re-entrant bootstrap misses the driver already running and its add is refused.
// It lives here rather than in loomcli so batten can look the driver up without importing another CLI package.
const LoomDriverStrandName = agentname.RoleDriver

// LegacyLoomDriverStrandName names a driver strand recorded before agent names existed.
// No add ever uses it.
// It exists because agentname.Matches reaches a legacy name only by equality, so a "driver" query alone never sees it,
// and a run in flight across the deploy would otherwise miss its own live driver and spawn a second one.
const LegacyLoomDriverStrandName = "loom-driver"

// IsDriverStrand reports whether a strand recorded under name is the run's driver:
// it matches the driver role through agentname.Matches, or is exactly the legacy literal.
func IsDriverStrand(name string) bool {
	return agentname.Matches(name, LoomDriverStrandName) || name == LegacyLoomDriverStrandName
}

// arbiterDefaultModelSpec is the model-spec an empty arbiter key takes.
const arbiterDefaultModelSpec = "opus[high]"

// arbiterDefaultTimeout is the timeout a zero arbiter_timeout_min takes.
const arbiterDefaultTimeout = 30 * time.Minute

// ArbiterSettings is the arbiter role's resolved model choice, timeout and spawn budget, threaded onto the arbiter session's launch spec.
type ArbiterSettings struct {
	shuttleengine.ModelChoice
	// Timeout is how long one arbiter spawn may run.
	Timeout time.Duration
	// MaxSpawns is how many arbiter spawns a run may use; zero turns the arbiter off.
	MaxSpawns int
}

// ResolveArbiter parses and resolves the arbiter role's model-spec from cfg through reg.
// An empty cfg.Arbiter takes opus[high] and a zero cfg.ArbiterTimeoutMin takes 30 minutes;
// cfg.ArbiterMaxSpawns passes through.
// A spec that does not parse or resolve is an error naming the arbiter key.
func ResolveArbiter(cfg Config, reg modelspec.Registry) (ArbiterSettings, error) {
	specText := cfg.Arbiter
	if specText == "" {
		specText = arbiterDefaultModelSpec
	}
	spec, err := modelspec.Parse(specText)
	if err != nil {
		return ArbiterSettings{}, fmt.Errorf("loom: ResolveArbiter: arbiter role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return ArbiterSettings{}, fmt.Errorf("loom: ResolveArbiter: arbiter role model-spec: %w", err)
	}

	return ArbiterSettings{
		ModelChoice: shuttleengine.NewModelChoice(resolved),
		Timeout:     ArbiterTimeout(cfg),
		MaxSpawns:   cfg.ArbiterMaxSpawns,
	}, nil
}

// ArbiterTimeout returns how long one arbiter spawn may run: arbiter_timeout_min in minutes, or 30 minutes when it is 0.
func ArbiterTimeout(cfg Config) time.Duration {
	if cfg.ArbiterTimeoutMin > 0 {
		return time.Duration(cfg.ArbiterTimeoutMin) * time.Minute
	}
	return arbiterDefaultTimeout
}

// alignmentDefaultModelSpec is the model-spec an empty alignment key takes.
const alignmentDefaultModelSpec = "opus[high]"

// alignmentDefaultTimeoutMin is the minutes a zero alignment_timeout_min takes.
const alignmentDefaultTimeoutMin = 20

// AlignmentSettings is the alignment agent's resolved model choice and timeout, threaded onto its session's launch spec.
type AlignmentSettings struct {
	shuttleengine.ModelChoice
	// Timeout is how long one alignment spawn may run.
	Timeout time.Duration
}

// ResolveAlignment parses and resolves the alignment agent's model-spec from cfg through reg.
// An empty cfg.Alignment takes opus[high] and a zero cfg.AlignmentTimeoutMin takes 20 minutes.
// A spec that does not parse or resolve is an error naming the alignment key.
func ResolveAlignment(cfg Config, reg modelspec.Registry) (AlignmentSettings, error) {
	specText := cfg.Alignment
	if specText == "" {
		specText = alignmentDefaultModelSpec
	}
	spec, err := modelspec.Parse(specText)
	if err != nil {
		return AlignmentSettings{}, fmt.Errorf("loom: ResolveAlignment: alignment model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return AlignmentSettings{}, fmt.Errorf("loom: ResolveAlignment: alignment model-spec: %w", err)
	}

	timeoutMin := alignmentDefaultTimeoutMin
	if cfg.AlignmentTimeoutMin > 0 {
		timeoutMin = cfg.AlignmentTimeoutMin
	}
	return AlignmentSettings{
		ModelChoice: shuttleengine.NewModelChoice(resolved),
		Timeout:     time.Duration(timeoutMin) * time.Minute,
	}, nil
}

// ResolveDriver parses and resolves the driver role's model-spec from cfg.
// It returns the ModelChoice the caller threads onto the driver session's launch spec.
// An empty cfg.Driver resolves to the zero ModelChoice (default engine, provider defaults) with a nil error.
// An empty value means "defer to the engine default", not a mistake to reject.
func ResolveDriver(cfg Config, reg modelspec.Registry) (shuttleengine.ModelChoice, error) {
	if cfg.Driver == "" {
		return shuttleengine.ModelChoice{}, nil
	}

	spec, err := modelspec.Parse(cfg.Driver)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("loom: ResolveDriver: driver role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("loom: ResolveDriver: driver role model-spec: %w", err)
	}

	return shuttleengine.NewModelChoice(resolved), nil
}
