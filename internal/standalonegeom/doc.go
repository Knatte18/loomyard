// doc.go carries the package-level doc comment for internal/standalonegeom.

// Package standalonegeom builds engine geometry structs from told strings alone.
// It never resolves cwd and never reads the environment.
//
// It never calls standalonestate.Derive.
// The caller derives the standalone state directory and hash8 once, at the CLI argument boundary,
// and passes both down as plain strings.
// Derive reads the live XDG_STATE_HOME, HOME and LOCALAPPDATA,
// so a builder that called it would resolve <state> to the operator's real home directory in every test that touched it.
//
// The one standalonestate call this package makes is standalonestate.Normalize, from ReedGeometry.
// It resolves only the symlinks along a told absolute path,
// so a builder stays a pure function of its told arguments plus the filesystem's own shape.
// The tmux session name's readable half and the socket key's hash must name the same directory.
// Only Derive's normalization rule makes them agree.
// A told path that does not exist on disk normalizes to itself.
//
// This package is not a leaf: it imports reedengine, websterengine and burlerengine.
// It must not be added to the leaf-enforcement allowlists of internal/buildinfo or internal/standalonestate.
//
// ReedGeometry, WebsterGeometry and BurlerGeometry convert a told target, stateDir and (for reed) hash8.
// StencilsDir, LogsDir and SpecsDir convert a told stateDir alone into the standalone directory paths.
// PauseGeometry converts a told stateDir alone into the pauserecord.Geometry of the standalone pause record.
// That geometry carries an empty slug.
// No engine imports this package back, so the told direction stays one-way.
package standalonegeom
