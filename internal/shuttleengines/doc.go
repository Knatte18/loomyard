// Package shuttleengines is the registry of named shuttle engines: claude, for Claude Code, and codex, for the Codex CLI.
// It is the only production package that imports a concrete engine package.
// shuttleengine and its callers therefore stay blind to which providers exist.
//
// New builds a shuttleengine.EngineSet from a shuttle config and the configengine.Dirs of the location it serves.
// It builds one engine per registered name, with the config's engine: key as the default.
// New hands every engine those Dirs, so an engine loads its own config module lazily where it reads it.
// It refuses a config whose engine: key names no registered engine, whichever config layer set it.
// Names lists the registered names, which equal modelspec's closed set of engine names.
//
// Each registry entry carries its engine's config module as plain data.
// ConfigModules lists them sorted by name for the config registry.
// An engine's config module therefore reaches that registry through this package alone.
// The claude module's load check also validates the keep-warm caps and the batching nudge.
// A config write therefore refuses a value a run start would refuse.
//
// Each entry also declares the environment keys its engine's process injects.
// EnvScrub returns their union for reed's server spawn, so an engine's scrub keys reach reed through this package alone.
//
// SessionNamers returns each registered engine's session namer keyed by the provider tag shuttle sets on its strands.
// It also maps the empty tag to the claude engine's namer.
// Reed is provider-blind, so this wiring layer chooses the namer that reads and repairs a session's own name.
// It chooses per strand tag.
// RecordReaders returns each registered engine's offline read capabilities keyed by engine name.
// It builds them with a zero shuttle config and no config directories, as SessionNamers builds its engines.
// A capability is set only when the engine implements it.
// Each set one is its own adapter, one per capability rather than one per engine:
// a reader of finished session records gets reading alone.
// No type assertion on a value in the set recovers an engine that can start, open, send to or stop a run.
// ToolBoundaryRenderer returns one registered engine's creel hook-delivery capability by name, built the same way:
// the hook-input parse, the hook output and the wake output the creel hook verbs render.
// BatchingNudger returns one registered engine's batching-nudge capability by name, built the same way.
// The shuttle hook verb that judges a session's tool call uses it.
// KeepWarm is the one keep-warm resolver.
// It answers a strand's keep-warm cap, the long cap and shuttle.yaml's break-even.
// It reads them through the default engine's KeepWarmCaps.
// A numbered role such as implementer-2 resolves as its base role, implementer.
// When the strand does not parse, it answers a fallback.
// It answers the same fallback when the shuttle config, the engine set or the engine's caps cannot be resolved.
// The fallback is shuttleengine.DefaultAwaitCap for both caps, no break-even and the cause.
// The package is told its Config and never imports internal/lymcwd.
package shuttleengines
