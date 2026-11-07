// bootstrapverb.go declares BootstrapVerb, the name of loom's bootstrap verb.

package loomcli

// BootstrapVerb names loom's own bootstrap verb:
// the command that reads this worktree's run seed at startup and decides who drives.
//
// A recipe with a non-empty BootstrapVerb has a command that reads its own run's seed at startup,
// so it can honour driver: llm.
// A recipe with an empty BootstrapVerb has no such site and cannot.
// This value must equal the `Use` string of the cobra command that `startCmd` builds in `start.go`.
// The recipe table in `internal/shedcli` takes the value from this constant instead of repeating it.
const BootstrapVerb = "start"
