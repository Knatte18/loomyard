// Package shuttleengines is the registry of named shuttle engines.
// It is the only production package that imports a concrete engine package.
// shuttleengine and its callers therefore stay blind to which providers exist.
//
// New builds a shuttleengine.EngineSet from a shuttle config: one engine per registered name, with the config's engine: key as the default.
// It refuses a config whose engine: key names no registered engine, whichever config layer set it.
// Names lists the registered names, which equal modelspec's closed set of engine names.
//
// Each registry entry carries its engine's config module as plain data, and ConfigModules lists them sorted by name for the config registry,
// so an engine's config module reaches that registry through this package alone.
//
// SessionNamer lives here for now: reed is provider-blind, so the provider that reads and repairs a session's own name is chosen by this wiring layer.
// The package is told its Config and never imports internal/lyxcwd.
package shuttleengines
