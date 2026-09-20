// bootstrapverb.go declares loom's bootstrap-verb capability constant. It is kept out of both
// cli.go, which builds the cobra tree, and bootstrap.go, whose own doc comment scopes that file to
// pure predicates and composers -- a bare capability declaration is neither.

package loomcli

// BootstrapVerb names loom's own bootstrap verb: the command that reads this worktree's run seed at
// startup and decides who drives.
//
// A recipe whose BootstrapVerb is non-empty has a command that reads its own run's seed at startup
// and can therefore honour driver: llm; a recipe whose BootstrapVerb is empty has no such site and
// cannot. This value must equal the Use string of the cobra command startCmd builds in start.go.
// internal/shedcli's table field (card 7) is populated from this constant so the fact is recorded
// once.
const BootstrapVerb = "start"
