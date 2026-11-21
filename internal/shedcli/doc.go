// Package shedcli builds the "lym shed" subtree.
// It holds a named-recipe arming table.
// The CLI seam (Command) registers it under the lym root.
//
// The verb bodies live in internal/shedverbs, a leaf package, and the table lives here, in the composition layer.
// The split follows the import direction:
// the arming functions the table calls are built over the loomCLI and battenCLI receivers,
// so the package owning the table imports internal/loomcli and internal/battencli,
// and those two packages import the verb bodies.
//
// The table in table.go maps recipe names to arming functions.
// internal/shedrecipe's registry is a separate table that maps engine names to shedengine.ShedProducer constructors.
// The two must stay apart, since a recipe name and an engine name come from disjoint vocabularies.
//
// The table binds a recipe name to an arming function, not to a recipe file path.
// A recipe alone cannot arm a Shed: it needs injected seams and told strings no file can name.
// A recipe is a compiled Go function, never a parsed file, as internal/shedbuild's doc comment rules.
//
// Every arming function shares one positional-argument contract, cobra.MaximumNArgs(1).
// An addressed run is named by run-id, so no recipe has an argument shape of its own.
package shedcli
