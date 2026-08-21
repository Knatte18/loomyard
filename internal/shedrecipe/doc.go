// Package shedrecipe owns the engine registry: the name to shedengine.ShedProducer-constructor
// mapping a future recipe loader resolves each row's Engine field against.
// It is imported by nobody in the four producer-hosting packages it imports (internal/shedengine,
// internal/shedadapters, internal/websterengine, internal/landingshed), so the dependency runs one
// way only.
//
// This package holds the Told-Geometry Invariant in the precise form CONSTRAINTS.md pins: an
// engine is handed the absolute paths it operates on and derives none of its own, so it runs
// identically inside a lyx hub and in a bare directory that is not a git repository.
// Every root this package touches is told, none is derived, and the package's only path
// construction is joining a told root with a recipe-relative value.
//
// Env is the told bundle: the caller-filled roots and injected seams every entry may read from.
// Config is the portable per-row half: the recipe row's own static, already-decoded configuration,
// which never contains an absolute path and which this package never learns the file format of.
package shedrecipe
