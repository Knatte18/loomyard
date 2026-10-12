// Package reportcli is the report module: `lyx report convergence [--since <date>] [--fresh-days <n>] [--format json|markdown]`, the read-only convergence report.
// It validates its flags, resolves the prime's geometry and hands it to internal/reportengine, which does the reading; the engine's package documentation describes every figure.
//
// # The verb
//
// It runs from the hub's prime worktree only: after resolving the cwd it applies fabric's drivable-worktree check and then refuses any worktree that is not the prime, naming the prime and saying to re-run it there.
// The flags are checked before any cwd resolution or read, and each refusal is a JSON error with exit 1 naming the flag, the value given and the form accepted:
//
//   - `--since` takes a `YYYY-MM-DD` date in UTC and defaults to the UTC date twelve weeks before now.
//   - `--fresh-days` is an integer of at least 1 and defaults to 3.
//   - `--format` is `json` (the default) or `markdown`.
//
// # The geometry
//
// The code checkout is the prime's worktree and its parent branch is the branch that worktree has checked out.
// The records checkout is the prime's records-side sibling, on the branch paired with the parent branch, and the board checkout is the hub's board directory on the branch it has checked out.
//
// # The output
//
// With `json` the verb prints one `{"ok":true}` envelope holding `selection`, `runs`, `partial`, `landings`, `weeks`, `absent` and `findings`.
// With `markdown` it prints the rendered document instead.
// Both exit 0 whatever findings the report holds, since a record that does not decode is a finding and not a failure, and a hub with no archive tags prints an empty report.
// An error from reading the repositories themselves is a JSON error with exit 1.
// The verb writes nothing, takes no lock and makes no commit.
package reportcli
