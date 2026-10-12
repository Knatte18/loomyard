// Package shortcutcli mounts the top-level operator shortcuts (`lyx attach`, `lyx prime`) declared in one embedded file.
//
// # The file
//
// shortcuts.yaml maps a shortcut name to one argv, a list of words, or to a list of argvs run in order.
// An argv names lyx words only, never a shell string.
// The file is embedded in the binary, so a shortcut is added only by a commit to it; no hub, repo or user can add or change one.
// Any other shape of a value is a parse error naming the shortcut.
// Parsing rejects neither an empty argv nor an unknown command; the guard in `cmd/lyx` checks the shipped file against the real root.
//
// # Mount
//
// `newRoot` in `cmd/lyx` calls Mount, so every root it builds, the fresh roots a shortcut runs its argvs on included, carries the same shortcuts.
// Mount adds one root command per shortcut under the group GroupID, which cobra lists as `Shortcuts:` in `lyx --help`.
// A file that does not parse logs one Warn and mounts nothing.
// Shortcuts are not modules: they have no `Command()` or `RunCLI`, and the module guards in `cmd/lyx` skip their group.
//
// # Words
//
// A shortcut command takes every word as given, since its flag parsing is off, and routes them itself:
//
//   - A verbosity word (`-v`, `-vv`, `--verbose`, `--verbose=<n>`) goes to the front of every argv.
//   - A help or json word (`-h`, `--help`, `--json`, `--json=<bool>`) prints the shortcut's own help, as JSON under json, and runs nothing.
//   - Every other word, and everything after `--`, goes to the end of the last argv.
//
// A word a global flag spells with a value its parser refuses, such as `--json=bogus`, is not a global word.
// It goes to the last argv, whose root refuses it after the earlier argvs ran, as typing the expanded commands in turn would.
//
// # Dispatch
//
// Each argv runs in order on a fresh root from `newRoot`, in the shortcut's own context, which carries the cwd the outer root was given, so the shortcut resolves no cwd itself.
// Input and output writers pass through.
// A non-zero exit code stops the sequence and becomes the shortcut's own; the inner root already printed its envelope, so the shortcut prints nothing.
// Every argv runs through a full root with its own pre-run, refusals and pause holds, so a shortcut adds no path past any command's guard.
// The root's persistent pre-run therefore runs once for the shortcut and once per argv in one process, and the repeat is harmless.
package shortcutcli
