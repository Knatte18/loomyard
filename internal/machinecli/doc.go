// Package machinecli builds the `lym machine` group with its one verb, `lym machine name`.
// With no argument the verb shows this machine's name, where it comes from and the former names the machine file records.
// The package is the only writer of the machine file.
//
// The module is not standalone-capable.
// It resolves no `--target-dir`, repository root or mode-derived state, so internal/cliwire does not serve it.
// `name` carries the operator audience.
// The envelope carries the result under `machine`.
// A refusal goes through output.ErrFields with a message that names its way forward.
//
// # Seams
//
// Every outside effect goes through a deps value that the command closes over, never a package-level variable.
// The file seam locates the machine file and its lock; the hostname seam gives the hostname rule's name of this host.
// Tests build a deps by hand, so each hands in a file path under its own temporary directory.
// None reads or writes the developer's machine file.
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
// The machine file is rewritten only when it differs from what the verb would write.
// The file keeps the former names, the old name and the hostname rule's name, and drops empty entries.
// The new name comes last.
// A file that did not decode loses its names, and the result says so.
// Each hub then takes one owner write whose replace set is the former names.
// It takes over the prime and every open beam project recorded under one of them, after the write's own pull.
// A hub whose write fails or keeps an owner is reported, the other hubs go on, and the verb exits non-zero.
// The re-run with the same name finishes the sweep and leaves the machine file as it is.
// The result lists the recoveries each state needs.
//
// # Bounds
//
// The verb writes the machine file and, on each swept hub, owners recorded under this machine's former names.
// It never takes an owner recorded under any other name.
// An owner another machine pushes after the verb's pull stays for the re-run.
// A hub that is neither in the machine record nor the hub of the working directory is not swept;
// run the verb from that hub's container directory or one of its worktrees.
package machinecli
