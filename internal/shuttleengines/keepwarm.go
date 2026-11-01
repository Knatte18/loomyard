// keepwarm.go declares the one keep-warm resolver: the cap an agent's blocking wait takes for its role, answered through the engine seam.

package shuttleengines

import (
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// KeepWarmAnswer is what KeepWarm resolves for a strand: the caps its engine answers and the gate's break-even.
type KeepWarmAnswer struct {
	// Cap is the longest one keep-warm wait of the strand's role; zero means the engine keeps nothing warm.
	Cap time.Duration
	// LongCap is the cap a long gate wait uses.
	LongCap time.Duration
	// MaxReruns is shuttle.yaml's keep_warm_max_reruns, the number of re-runs one long wait is worth; 0 takes no long wait.
	MaxReruns int
	// Fallback is why the answer is the fallback, and nil when the engine answered.
	Fallback error
}

// KeepWarm resolves the keep-warm caps of the strand named strand, the full agent name of the caller, over the layers dirs locate.
// It answers the fallback, never an error, when the strand does not parse or the shuttle config, the engine set or the engine's caps cannot be resolved:
// both caps are shuttleengine.DefaultAwaitCap, MaxReruns is 0 and Fallback holds the cause.
// It never logs; the caller logs a fallback at its own level.
func KeepWarm(dirs configengine.Dirs, strand string) KeepWarmAnswer {
	constructors := make(map[string]constructor, len(registry))
	for name, registered := range registry {
		constructors[name] = registered.constructor
	}
	return keepWarmWith(dirs, strand, constructors)
}

// keepWarmWith is KeepWarm over the given engine constructors.
// A numbered role such as implementer-2 resolves as its base role.
func keepWarmWith(dirs configengine.Dirs, strand string, constructors map[string]constructor) KeepWarmAnswer {
	fallback := func(cause error) KeepWarmAnswer {
		return KeepWarmAnswer{Cap: shuttleengine.DefaultAwaitCap, LongCap: shuttleengine.DefaultAwaitCap, Fallback: cause}
	}
	name, err := agentname.Parse(strand)
	if err != nil {
		return fallback(err)
	}
	cfg, err := shuttleengine.LoadConfig(dirs, "shuttle")
	if err != nil {
		return fallback(err)
	}
	set, err := build(cfg, dirs, constructors)
	if err != nil {
		return fallback(err)
	}
	engine, err := set.Lookup(cfg.Engine)
	if err != nil {
		return fallback(err)
	}
	capWait, longCap, err := engine.KeepWarmCaps(baseRole(name.Role))
	if err != nil {
		return fallback(err)
	}
	return KeepWarmAnswer{Cap: capWait, LongCap: longCap, MaxReruns: cfg.KeepWarmMaxReruns}
}

// baseRole drops a numbered suffix from role: for a role of the form <base>-<N> that agentname numbers from base it answers base, and any other role answers itself.
func baseRole(role string) string {
	index := strings.LastIndex(role, "-")
	if index < 0 {
		return role
	}
	if base := role[:index]; agentname.MatchesRole(role, base) {
		return base
	}
	return role
}
