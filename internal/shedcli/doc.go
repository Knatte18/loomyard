// Package shedcli builds the "lym shed" subtree.
// It holds a named-recipe arming table.
// The CLI seams (Command, RunCLI, RunCLIIn) register it under the lym root.
//
// The verb bodies live in internal/shedverbs, a leaf package, and the table lives here, in the composition layer.
// The split follows the import direction:
// the arming functions the table calls are built over the loomCLI and battenCLI receivers,
// so the package owning the table imports internal/loomcli and internal/battencli,
// and those two packages import the verb bodies.
//
// The table in table.go maps recipe names to arming functions.
// shedcli's own pre-run reads it to decide which module arms a given "lym shed" invocation.
// internal/shedrecipe's registry is a separate table.
// It maps engine names to shedengine.ShedProducer constructors, and every recipe's row list reads it.
// The two must stay apart, since a recipe name and an engine name come from disjoint vocabularies.
//
// The table binds a recipe name to an arming function, not to a recipe file path, for two reasons.
// A recipe alone cannot arm a Shed:
// shedverbs.Spec's BuildShed closure needs injected seams (Shuttle, Burler, WebsterRun).
// It also needs told strings no file can name.
// A path-taking flag would also imply a run-time on-disk location for recipe files.
// internal/shedbuild's doc comment rules that out: a recipe is a compiled Go function, never a parsed file.
//
// Every arming function shares one positional-argument contract, cobra.MaximumNArgs(1).
// It is set statically on each generic verb, not read off the table.
// An addressed run is named by run-id, so no recipe has an argument shape of its own.
// The run-id is resolved once, ahead of the recipe lookup, by resolvePersistentPreRun's seed read;
// cli.go's doc comment gives the full sequence.
package shedcli
