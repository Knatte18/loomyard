// Package claudecli builds the `lym claude` group: the account verbs and the status line verb.
// The verbs are `lym claude account set <account>`, `lym claude account show` and `lym claude statusline <account>`.
// They run from a worktree of a hub and refuse anywhere else, naming that accounts belong to a hub.
// The package reaches the provider only through shuttleengines.AccountKeeper.
// The keeper is built over the worktree's config directories and the worktree geometry hubgeom tells.
//
// `set` and `show` carry the operator audience and print their result in the standard envelope.
// `set` reports the account it wrote under `set`, with the note that it takes effect at the worktree's next launch.
// `show` reports its lines under `lines`.
// `statusline` is hidden, carries the internal audience, and reads the provider's status line payload on standard input.
// It prints one short line to standard output for the provider to show, and a refusal goes through output.Err.
// The module is hub-only and not standalone-capable, so internal/cliwire does not serve it.
package claudecli
