// Package machinecli builds the `lym machine` group with its two verbs, `lym machine name` and `lym machine start`.
// With no argument `name` shows this machine's name, where it comes from and the former names the machine file records.
// The package writes the machine file's names, and boardengine.EnsureIdentity writes its id; both keep the other's keys.
//
// The module is not standalone-capable.
// It resolves no `--target-dir`, repository root or mode-derived state, so internal/cliwire does not serve it.
// Both verbs carry the operator audience.
// The envelope carries the result of `name` under `machine` and that of `start` under `start`.
// A refusal goes through output.ErrFields with a message that names its way forward.
//
// # Seams
//
// Every outside effect goes through a deps value that the command closes over, never a package-level variable.
// Tests build a deps by hand, so none reads or writes the developer's machine file, hubs or desktop.
//
// # Start
//
// `lym machine start [<shortname>]` brings back what this machine owns, from any directory.
// It reaches every hub hubregistry.Live lists, or the one hub hubregistry.Lookup resolves for shortname.
// Per hub it pulls the board, then reads the owners and the hold from the local board.
// A prime or beam another machine owns is skipped, naming that machine and `lym adopt`.
// One with no recorded owner is skipped too, since dobbyengine.EnsureLive would claim a prime with none.
// The hub's owned beams still start.
// The rest start through `lym beam start` in the hub's prime.
// The prime starts with `--all` when this machine owns it, which brings up every owned beam with it.
// Otherwise each owned beam starts by its slug, with the hidden `--no-prime-claim` flag,
// so no child claims a prime that has no recorded owner.
// A worktree whose dobby `lym beam start` reports refused or failed counts as failed.
// The verb never runs `lym s` or `lym adopt`, whose takeover would claim a hub another machine owns.
// It resumes no run; the orchs and dobbies take the runs up.
// In a hub where a worktree came up it opens the hub's workspace file through vscode.Launch.
// It also opens one terminal window per started worktree running `lym reed attach`, through termwindow.
// A started worktree whose window does not open carries its attach command in the result.
// So does every started worktree when no launcher resolves.
// A standing Bolt hold is printed with its machine and subjects.
// For this machine's own hold it also prints the `lym adopt <subjects>` re-run.
// It keeps the desktop launchers internal/desktoplauncher writes.
// A hub's launcher is added while dobbyengine.OwnsAnything reads that this machine owns anything in the hub.
// It is removed once this machine owns nothing there.
// The launcher of a hub hubregistry.Live no longer lists is removed, and the all-hubs launcher goes with the last hub's.
// A pull that fails, a window, a workspace or a launcher that does not open or write is a warning.
// A hub that does not read and a worktree that does not start make the verb exit non-zero, and the other hubs go on.
// The verb writes no tracked record.
//
// # Show
//
// The show reads the machine file through machinename.ReadFile and never through machinename.Name,
// so a file that does not decode or holds an invalid name is shown with its error and path, not hidden behind a fallback.
// It writes nothing and opens no hub.
//
// # Set
//
// `lym machine name <name>` checks the name, reads every hub's board, and only then writes.
// The hubs are the live hubs of the machine record and the hub of the working directory, each once.
// A board that does not read, and a name a board records as another machine's owner, refuse the verb before any write.
// A name this machine already holds, as its current name, a former name or its hostname rule's name, passes that check.
// A registration the board's registry refuses also stops the verb before any write.
// That is a name another registered machine holds as its current or former name.
// The refusal names that machine and the way forward.
// The machine file is rewritten only when it differs from what the verb would write.
// The file keeps its machine id and its forgotten names.
// It also keeps the former names, the old name and the hostname rule's name, and drops empty entries.
// The new name comes last.
// A file with no id gets a fresh one.
// A file whose id a hub's registry holds under a name that is not one of the file's names is a copy.
// A name counts as the file's when it is the current name, one of its names or one of its forgotten names.
// A copy of another machine's file gets a fresh id, and the verb is not refused for the copy.
// A current name or hostname rule's name that a hub's registry reads as another machine's is not this machine's:
// it is kept out of the written names, and a forgotten name is never added back.
// A file that did not decode loses its names, and the result says so.
// Each hub then registers the new identity, with the old name among the machine's former names in the registry.
// It then takes one owner write whose replace set is the former names.
// The owner write carries the identity, so it writes the current name.
// It takes over the prime and every open beam project recorded under one of them, after the write's own pull.
// A hub whose registration or write fails or keeps an owner is reported, and the other hubs go on.
// The verb then exits non-zero.
// A hub whose registration fails is not swept.
// The re-run with the same name finishes the registration and the sweep and leaves the machine file as it is.
//
// # Forget
//
// `lym machine name --forget-former <name>` is exclusive with a positional name.
// It drops a name the machine file holds as a former name.
// The name leaves the file's names and joins its forgotten names.
// The file keeps its id; a file with none gets one.
// A name the file holds in neither list is refused, naming the file's former names.
// A name the file already forgot passes, so the re-run of a registration that failed finishes it.
// Each swept hub then takes a registration whose forget set holds the name, so it leaves this machine's registry entry.
// A hub whose registration fails is reported with the re-run, the other hubs go on, and the verb exits non-zero.
// Before it returns, the verb lists every record it can see that still names the dropped name.
// They are the owners on each hub's local board, the prime as `root` and a beam by slug,
// and each takeover entry and unfinished cancel intent in the batten record of a run whose pair stands on this machine.
// Each carries the `lym adopt` subject that takes it back once the registration has landed.
//
// # Bounds
//
// The forget changes only this machine's file and its own registry entry.
// It writes no other machine's entry and no owner.
// A record under the dropped name reads as the other machine on every machine from that machine's registration on,
// and the listed references are the operator's to take back.
//
// The verb writes the machine file and, on each swept hub, owners recorded under this machine's former names.
// It never takes an owner recorded under any other name.
// An owner another machine pushes after the verb's pull stays for the re-run.
// A hub that is neither in the machine record nor the hub of the working directory is not swept;
// run the verb from that hub's container directory or one of its worktrees.
package machinecli
