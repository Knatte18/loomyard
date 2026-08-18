// doc.go carries the package-level doc comment for internal/standalonegeom.

// Package standalonegeom is the told-mode sibling of internal/hubgeom: it builds engine geometry
// structs from told strings alone, never resolving cwd and never reading the environment.
//
// It deliberately never calls standalonestate.Derive — the caller derives the standalone state
// directory and hash8 once, at the CLI argument boundary, and passes both down as plain strings.
// Derive reads live XDG_STATE_HOME/HOME/LOCALAPPDATA, so a builder that called it here would
// resolve <state> to the operator's real home directory in every test that touched it. Told
// parameters are what keep this package hermetic by construction, rather than by each test
// remembering to redirect XDG_STATE_HOME.
//
// This package is deliberately not a leaf: it imports reedengine and websterengine, so it must
// not be added to internal/buildinfo's or internal/standalonestate's leaf-enforcement allowlists.
//
// standalonegeom's contract today is ReedGeometry and WebsterGeometry, converting a told target,
// stateDir, and (for reed) hash8 into a reedengine.Geometry or websterengine.Geometry
// respectively. Neither engine imports this package back — the told direction stays one-way.
package standalonegeom
