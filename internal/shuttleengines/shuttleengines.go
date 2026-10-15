// shuttleengines.go declares the registry of named engines and builds an EngineSet from a shuttle config.

package shuttleengines

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// constructor builds one engine from the shuttle config and the Dirs of the location the engine serves.
type constructor func(cfg shuttleengine.Config, dirs configengine.Dirs) shuttleengine.Engine

// ConfigModule is one engine's config module as plain data, so the config registry can list it without importing the engine.
type ConfigModule struct {
	// Name is the module identifier and its file's base name.
	Name string
	// Template returns the module's default YAML template.
	Template func() string
	// OpenMaps are the template's dotted paths whose entries belong to the operator.
	OpenMaps []string
	// Migrate, when non-nil, rewrites a present file's bytes before reconcile compares them to the template.
	Migrate func(existing []byte) ([]byte, []string, error)
	// Spec returns the module's layered-load spec.
	Spec func() configengine.Spec
	// CheckLoad runs the module's own loader under dirs and returns its error, discarding the loaded config.
	CheckLoad func(configengine.Dirs) error
}

// entry pairs a registered engine's constructor with its config module and the environment keys it declares scrubbed.
type entry struct {
	constructor constructor
	config      ConfigModule
	scrub       reedengine.EnvScrub
}

// claudeScrub is the claude engine's declared scrub.
func claudeScrub() reedengine.EnvScrub {
	names, prefixes := claudeengine.ScrubbedEnv()
	return reedengine.EnvScrub{Names: names, Prefixes: prefixes}
}

// registry maps each registered engine name to its entry.
var registry = map[string]entry{
	"claude": {
		scrub: claudeScrub(),
		constructor: func(cfg shuttleengine.Config, dirs configengine.Dirs) shuttleengine.Engine {
			return claudeengine.NewFromConfig(cfg, dirs)
		},
		config: ConfigModule{
			Name:     "claude",
			Template: claudeengine.ConfigTemplate,
			OpenMaps: claudeengine.ConfigOpenMaps(),
			Spec:     claudeengine.ConfigSpec,
			CheckLoad: func(dirs configengine.Dirs) error {
				_, err := claudeengine.LoadConfig(dirs)
				return err
			},
		},
	},
}

// New builds the set of every registered engine from cfg, with cfg.Engine as the default.
// Every engine is handed dirs, the layers of the location it serves.
// It refuses a cfg.Engine that names no registered engine.
func New(cfg shuttleengine.Config, dirs configengine.Dirs) (shuttleengine.EngineSet, error) {
	constructors := make(map[string]constructor, len(registry))
	for name, registered := range registry {
		constructors[name] = registered.constructor
	}
	return build(cfg, dirs, constructors)
}

// ConfigModules returns every registered engine's config module, sorted by name.
func ConfigModules() []ConfigModule {
	modules := make([]ConfigModule, 0, len(registry))
	for _, registered := range registry {
		modules = append(modules, registered.config)
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Name < modules[j].Name })
	return modules
}

// build constructs one engine per entry of constructors from cfg and dirs and returns them as a set whose default is cfg.Engine.
// It refuses, before returning a set, a cfg.Engine that is not one of the entries,
// and an engine that declares fork support through shuttleengine.RequirementSupporter without implementing shuttleengine.ForkAuditor.
func build(cfg shuttleengine.Config, dirs configengine.Dirs, constructors map[string]constructor) (shuttleengine.EngineSet, error) {
	engines := make(map[string]shuttleengine.Engine, len(constructors))
	names := make([]string, 0, len(constructors))
	for name, construct := range constructors {
		engines[name] = construct(cfg, dirs)
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		supporter, declares := engines[name].(shuttleengine.RequirementSupporter)
		if !declares || !supporter.Supports(shuttleengine.RequirementForks) {
			continue
		}
		if _, audits := engines[name].(shuttleengine.ForkAuditor); !audits {
			return shuttleengine.EngineSet{}, fmt.Errorf("shuttle: engine %q declares forks but cannot audit them", name)
		}
	}
	if _, ok := engines[cfg.Engine]; !ok {
		return shuttleengine.EngineSet{}, fmt.Errorf(
			"shuttle: shuttle.yaml's engine: %q names no registered engine; registered engines: %s; way forward: set engine: in shuttle.yaml to one of them",
			cfg.Engine, strings.Join(names, ", "))
	}
	return shuttleengine.NewEngineSet(cfg.Engine, engines), nil
}

// Names returns the registered engine names, sorted.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// EnvScrub returns the environment keys every registered engine declares a spawned process must not inherit.
// It is the one route from an engine's declaration to reed's server spawn, and it is never empty while an engine is registered.
func EnvScrub() reedengine.EnvScrub {
	return scrubOf(registry)
}

// scrubOf returns the union of the entries' scrub declarations, sorted and deduplicated.
func scrubOf(entries map[string]entry) reedengine.EnvScrub {
	names := map[string]bool{}
	prefixes := map[string]bool{}
	for _, registered := range entries {
		for _, name := range registered.scrub.Names {
			names[name] = true
		}
		for _, prefix := range registered.scrub.Prefixes {
			prefixes[prefix] = true
		}
	}
	return reedengine.EnvScrub{Names: sortedKeys(names), Prefixes: sortedKeys(prefixes)}
}

// sortedKeys returns the keys of set in ascending order.
func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// SessionNamers returns the session namers reed's watchdog repairs names through, keyed by the provider tag shuttle sets on a strand.
// It is the one place a provider is chosen for reed, which stays provider-blind.
// The empty tag maps to the claude engine's namer, whatever the default engine is:
// strands recorded before the tag existed, and strands added outside shuttle, have always been repaired as Claude's.
func SessionNamers() map[string]reedengine.SessionNamer {
	engines := make(map[string]shuttleengine.Engine, len(registry))
	for name, registered := range registry {
		engines[name] = registered.constructor(shuttleengine.Config{}, configengine.Dirs{})
	}
	namers := namersOf(engines)
	if claude, ok := namers["claude"]; ok {
		namers[""] = claude
	}
	return namers
}

// namersOf maps each engine that implements reedengine.SessionNamer to its namer, keyed by the engine's name, and skips one that does not.
func namersOf(engines map[string]shuttleengine.Engine) map[string]reedengine.SessionNamer {
	namers := make(map[string]reedengine.SessionNamer, len(engines))
	for name, engine := range engines {
		if namer, ok := engine.(reedengine.SessionNamer); ok {
			namers[name] = namer
		}
	}
	return namers
}

// RecordReaders returns each registered engine's offline read capabilities, keyed by engine name, for a reader that works from finished session records.
// The engines are built with a zero shuttle config and no config directories, as SessionNamers builds them, so no config file is needed to read a record.
// Each capability is wrapped in an adapter that exposes reading alone, so a reader cannot recover an engine that starts, opens, sends to or stops a run.
func RecordReaders() map[string]shuttleengine.RecordReads {
	engines := make(map[string]shuttleengine.Engine, len(registry))
	for name, registered := range registry {
		engines[name] = registered.constructor(shuttleengine.Config{}, configengine.Dirs{})
	}
	return readsOf(engines)
}

// readsOf maps each engine to its read capabilities, keyed by the engine's name.
// A capability the engine does not implement is nil in its entry.
func readsOf(engines map[string]shuttleengine.Engine) map[string]shuttleengine.RecordReads {
	reads := make(map[string]shuttleengine.RecordReads, len(engines))
	for name, engine := range engines {
		var entry shuttleengine.RecordReads
		if transcripts, ok := engine.(shuttleengine.TranscriptReader); ok {
			entry.Transcripts = transcriptsAdapter{transcripts: transcripts}
		}
		if pricer, ok := engine.(shuttleengine.MessagePricer); ok {
			entry.Pricer = pricerAdapter{pricer: pricer}
		}
		if instructions, ok := engine.(shuttleengine.InstructionFileLister); ok {
			entry.Instructions = instructionsAdapter{instructions: instructions}
		}
		reads[name] = entry
	}
	return reads
}

// transcriptsAdapter forwards transcript reading to an engine and exposes nothing else of it.
type transcriptsAdapter struct {
	transcripts shuttleengine.TranscriptReader
}

// LocateSessions forwards to the wrapped engine.
func (a transcriptsAdapter) LocateSessions(worktree string) ([]shuttleengine.LocatedSession, error) {
	return a.transcripts.LocateSessions(worktree)
}

// ReadSession forwards to the wrapped engine.
func (a transcriptsAdapter) ReadSession(s shuttleengine.LocatedSession) (shuttleengine.SessionRead, error) {
	return a.transcripts.ReadSession(s)
}

// pricerAdapter forwards message pricing to an engine and exposes nothing else of it.
type pricerAdapter struct {
	pricer shuttleengine.MessagePricer
}

// PriceMessage forwards to the wrapped engine.
func (a pricerAdapter) PriceMessage(model string, u shuttleengine.MessageUsage) (float64, bool) {
	return a.pricer.PriceMessage(model, u)
}

// instructionsAdapter forwards the instruction-file list to an engine and exposes nothing else of it.
type instructionsAdapter struct {
	instructions shuttleengine.InstructionFileLister
}

// InstructionFiles forwards to the wrapped engine.
func (a instructionsAdapter) InstructionFiles() []string {
	return a.instructions.InstructionFiles()
}
