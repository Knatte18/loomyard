// doc.go carries the package-level doc comment for internal/segmentcolor.

// Package segmentcolor is a stdlib-only leaf holding lyx's closed vocabulary of loom segments and palette colors.
//
// A segment is the loom phase a spawned strand belongs to, named by the spawning module on its launch spec.
// The segment constants' values are the keys of reed's segment color config, so one vocabulary serves the launch spec, the strand record and the config.
// No segment's default color lives here: defaults are reed's config template values.
//
// The palette is lyx's own vocabulary.
// This package maps it to tmux color names; a provider's names for the same colors live only inside that provider's engine.
//
// Leaf rule: production code imports the standard library only, enforced by leaf_enforcement_test.go.
// See PATTERN-leaf-packages.
package segmentcolor
