// Package shuttleengines is the registry of named shuttle engines.
// It is the only production package that imports a concrete engine package.
// shuttleengine and its callers therefore stay blind to which providers exist.
//
// New builds a shuttleengine.EngineSet from a shuttle config and the configengine.Dirs of the location it serves: one engine per registered name, with the config's engine: key as the default.
// New hands every engine those Dirs, so an engine loads its own config module lazily where it reads it.
// It refuses a config whose engine: key names no registered engine, whichever config layer set it.
// Names lists the registered names, which equal modelspec's closed set of engine names.
//
// Each registry entry carries its engine's config module as plain data, and ConfigModules lists them sorted by name for the config registry,
// so an engine's config module reaches that registry through this package alone.
//
// Each entry also declares the environment keys its engine's process injects, and EnvScrub returns their union for reed's server spawn,
// so an engine's scrub keys reach reed through this package alone.
//
// SessionNamers returns each registered engine's session namer keyed by the provider tag shuttle sets on its strands, and also maps the empty tag to the claude engine's namer:
// reed is provider-blind, so the namer that reads and repairs a session's own name is chosen by this wiring layer, per strand tag.
// The package is told its Config and never imports internal/lyxcwd.
package shuttleengines
