// Package reportcli is the report module: `lyx report convergence [--since <date>] [--fresh-days <n>] [--format json|markdown]`, the read-only convergence report, and `lyx report timing [--since <date>] [--by day|week] [--format json|markdown]`, the read-only timing report.
// Each verb validates its flags, resolves the prime's geometry and hands it to internal/reportengine, which does the reading; the engine's package documentation describes every figure.
//
// # The verbs
//
// Both run from the hub's prime worktree only: after resolving the cwd they apply fabric's drivable-worktree check and then refuse any worktree that is not the prime, naming `lyx report <verb>` and the prime and saying to re-run it there.
// The flags are checked before any cwd resolution or read, and each refusal is a JSON error with exit 1 naming the flag, the value given and the form accepted.
//
// `convergence` takes:
//
//   - `--since` takes a `YYYY-MM-DD` date in UTC and defaults to the UTC date twelve weeks before now.
//   - `--fresh-days` is an integer of at least 1 and defaults to 3.
//   - `--format` is `json` (the default) or `markdown`.
//
// `timing` takes:
//
//   - `--since` takes a `YYYY-MM-DD` date in UTC and defaults to the UTC date seven days before today's; a date after today is accepted and gives an empty report.
//   - `--by` is `day` (the default) or `week`, the bucket width.
//   - `--format` is `json` (the default) or `markdown`.
//
// # The geometry
//
// The code checkout is the prime's worktree and its parent branch is the branch that worktree has checked out.
// The records checkout is the prime's records-side sibling, on the branch paired with the parent branch, and the board checkout is the hub's board directory on the branch it has checked out.
// `timing` also lists the hub's live pairs, the base names of its code worktrees other than the main one, so the engine can tell a pair records branch whose pair is gone from one in flight.
//
// # The sleep
//
// `timing` runs `journalctl` once per invocation for the systemd-sleep entries, when it is on PATH, logging the spawn and its end at Info, and hands the sleeps it parses to the engine, which leaves them out of every interval.
// A journalctl that is absent or fails, or a journal with no sleep, leaves nothing out, and `sleep_note` says why.
//
// # The output
//
// With `json` `convergence` prints one `{"ok":true}` envelope holding `selection`, `runs`, `partial`, `landings`, `weeks`, `absent` and `findings`, and `timing` one holding `selection`, `runs`, `phases`, `buckets`, `sleep_note` and `findings`.
// With `markdown` each prints its rendered document instead.
// Both exit 0 whatever findings the report holds, since a record that does not decode is a finding and not a failure, and a hub with no archive tags prints an empty report.
// An error from reading the repositories themselves is a JSON error with exit 1.
// The verbs write nothing, take no lock and make no commit.
package reportcli
