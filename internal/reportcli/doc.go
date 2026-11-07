// Package reportcli is the report module, which serves these read-only verbs:
//
//   - `lym report convergence [--since <date>] [--fresh-days <n>] [--format json|markdown]`, the convergence report.
//   - `lym report timing [--since <date>] [--by day|week] [--format json|markdown]`, the timing report.
//   - `lym report cost [--since <date>] [--format json|markdown]`, the cost report.
//   - `lym report raddle [--format json|markdown]`, the predicted-against-actual cost of each filed Sketch.
//   - `lym report replies`, the count of creel's expected replies.
//
// Each verb validates its flags, resolves the prime's geometry and hands it to internal/reportengine.
// The engine does the computing, and its package documentation describes every figure.
//
// # The verbs
//
// All run from the hub's prime worktree only.
// After resolving the cwd they apply fabric's drivable-worktree check and refuse any worktree that is not the prime.
// The refusal names `lym report <verb>` and the prime and says to re-run it there.
// The flags are checked before any cwd resolution or read.
// Each refusal is a JSON error with exit 1 naming the flag, the value given and the form accepted.
//
// `convergence` takes:
//
//   - `--since` takes a `YYYY-MM-DD` date in UTC and defaults to the UTC date twelve weeks before now.
//   - `--fresh-days` is an integer of at least 1 and defaults to 3.
//   - `--format` is `json` (the default) or `markdown`.
//
// `timing` takes:
//
//   - `--since` takes a `YYYY-MM-DD` date in UTC and defaults to the UTC date seven days before today's.
//     A date after today is accepted and gives an empty report.
//   - `--by` is `day` (the default) or `week`, the bucket width.
//   - `--format` is `json` (the default) or `markdown`.
//
// `cost` takes:
//
//   - `--since` takes a `YYYY-MM-DD` date in UTC.
//     It defaults to the UTC date seven days before today's, as `timing` does.
//   - `--format` is `json` (the default) or `markdown`.
//
// It reads `cost_min_runs` from the hub's `report.yaml` through `internal/reportconfig`.
// That key is the run count a cell needs before it shows its figures.
//
// `raddle` takes `--format`, `json` (the default) or `markdown`, and no date.
// It lists every Sketch of the raddle store on the board branch at its tip.
//
// `replies` takes no flags.
// It reads the creel journal, which keeps seven days, as the `REPORT` program.
// The read goes through creelcli.NewProgramClient, whose `Log` the package seam `creelLog` wraps.
// It prints one `{"ok":true}` envelope holding `roles`, the engine's counts per receiver role.
// The program claim is accepted from the operator's terminal or an orch pane, the places the report verbs run from;
// creel refuses it from another agent's pane, naming the claim.
//
// # The geometry
//
// The code checkout is the prime's worktree, and its parent branch is the branch that worktree has checked out.
// The records checkout is the prime's records-side sibling, on the branch paired with the parent branch.
// The board checkout is the hub's Bolt checkout on the branch it has checked out.
// `timing` also lists the hub's live pairs, the base names of its code worktrees other than the main one,
// so the engine can tell a pair records branch whose pair is gone from one in flight.
//
// # The sleep
//
// When `journalctl` is on PATH, `timing` runs it once per invocation for the systemd-sleep entries.
// It logs the spawn and its end at Info.
// It hands the sleeps it parses to the engine, which leaves them out of every interval.
// A journal with no sleep leaves nothing out, and `sleep_note` says why.
//
// The machine's sleep is unknown in three cases:
// journalctl is absent or fails, the user may not read the journal, or the output does not parse.
// A journal the user may not read makes journalctl exit 0 with empty output and a permission hint on standard error.
// The verb then reads the hub's pause record instead.
// It hands the engine every hub-scope entry's interval, a live one running to now.
// A hub pause holds every run, and `lym hibernate` pauses the hub before the machine sleeps.
// Pair-scope entries are left out, as the engine's sleep is machine-wide.
// The `sleep_note` then names why the journal was not read and says whether pauses stood in.
// It also says the figures may include sleep.
// The stand-in covers only the hub pauses the record still keeps, within `pauserecord.Retention`;
// a sleep taken with no hub pause, or before that, counts as run time.
// An undecodable pause record leaves nothing out, and the note names its error.
//
// # The output
//
// With `json` each verb prints one `{"ok":true}` envelope:
//
//   - `convergence` holds `selection`, `runs`, `partial`, `landings`, `weeks`, `absent` and `findings`.
//   - `timing` holds `selection`, `runs`, `phases`, `buckets`, `sleep_note` and `findings`.
//   - `cost` holds `selection`, `cells` and `findings`.
//   - `raddle` holds `selection`, `rows` and `findings`.
//
// With `markdown` each prints its rendered document instead.
// The report verbs exit 0 whatever findings the report holds.
// A record that does not decode is a finding and not a failure.
// A hub with no archive tags prints an empty report.
// An error from reading the repositories themselves is a JSON error with exit 1.
// The verbs write nothing, take no lock and make no commit.
package reportcli
