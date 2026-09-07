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
// The one standalonestate call this package does make is standalonestate.Normalize, from
// ReedGeometry, and it is a different kind of call: it reads no environment variable and resolves no
// working directory, only the symlinks along a told absolute path, so a builder stays a pure function
// of its told arguments plus the filesystem's own shape. It exists because the tmux session name's
// readable half and the socket key's hash must agree about which directory they name, and only
// Derive's normalization rule can make them agree (R4 review finding R4-24). A told path that does
// not exist on disk normalizes to itself, so a test driving these builders with fictional absolute
// paths stays deterministic and needs no fixture.
//
// This package is deliberately not a leaf: it imports reedengine, websterengine and burlerengine,
// so it must not be added to internal/buildinfo's or internal/standalonestate's
// leaf-enforcement allowlists.
//
// standalonegeom's contract today is ReedGeometry, WebsterGeometry, and BurlerGeometry,
// converting a told target, stateDir, and (for reed) hash8 into a reedengine.Geometry,
// websterengine.Geometry, or burlerengine.Geometry
// respectively, plus the StencilsDir helper, which converts a told stateDir alone into the
// standalone stencils directory path, and the LogsDir helper, which converts a told stateDir
// alone into the standalone trace-log directory path. Neither engine imports this package back —
// the told direction stays one-way.
package standalonegeom
