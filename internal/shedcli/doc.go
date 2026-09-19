// Package shedcli builds the "lyx shed" subtree: a named-recipe arming table plus the three CLI
// seams (Command, RunCLI, RunCLIIn) that register it under the lyx root.
//
// This package is a separate package from internal/shedverbs because one package cannot hold both
// halves — the arming functions this package's table calls are built over the loomCLI and
// lifecycleCLI receivers, so whichever package owns the table must import internal/loomcli and
// internal/lifecyclecli, and those two packages import internal/shedverbs' own verb bodies.
// Splitting them puts the bodies at a leaf (internal/shedverbs) and the table at the composition
// layer (this package), which is what the dependency direction already demands.
//
// The table this package declares (table.go) is a different table from internal/shedrecipe's own
// registry, and the two must never be merged: this package's table maps recipe names ("loom",
// "lifecycle") to arming functions, while internal/shedrecipe's registry maps engine names to
// shedengine.ShedProducer constructors. The two tables answer different questions for different
// callers — this one is read by shedcli's own pre-run to decide which module arms a given "lyx shed"
// invocation, the other is read by every recipe's own row list to decide which producer constructor
// backs a given row — and collapsing them would conflate a recipe name with an engine name, which
// are drawn from disjoint vocabularies today and have no reason to start colliding.
//
// The table binds a recipe name to an arming *function* rather than to a recipe file path, for two
// reasons. First, a recipe alone cannot arm a Shed: shedverbs.Spec's BuildShed closure needs
// injected seams (Shuttle, Burler, WebsterRun) and told strings no file can name — see spec.go's own
// build-time-texts-run-time-spec Shared Decision. Second, a path-taking flag would imply a runtime
// on-disk location for recipe files, which internal/shedbuild's own doc comment and the
// Recipe-Format Sole-Parser Invariant both forbid: a recipe is a compiled Go function, never a
// parsed file, anywhere in this tree.
//
// Each table entry also declares its own positional-arg contract (Args), rather than sharing one
// contract across every recipe, because the two shipped recipes disagree: "loom" takes no
// positional argument and "lifecycle" takes exactly one slug. Each entry's Args value is the exact
// cobra.PositionalArgs value lifecyclecli and loomcli already assign to their own commands, which is
// what makes the two invocation paths refuse a wrong argument count byte-identically rather than by
// luck.
package shedcli
