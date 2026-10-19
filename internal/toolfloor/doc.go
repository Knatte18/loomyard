// Package toolfloor declares the oldest git and tmux lyx runs on, parses a tool's version line and returns the floor error.
// CheckGit reads `git --version` output and CheckTmux reads `tmux -V` output; each returns whether the line parsed and a *FloorError when the parsed version is below the floor.
// It spawns no process, keeps no cache and logs nothing: gitexec and reedengine run the probes, keep their own process caches and own the Warn.
// It imports the standard library only.
//
// # The floor
//
// GitFloor is 2.43.0 and TmuxFloor is 3.4, the versions Ubuntu 24.04 ships and the oldest lyx is proven on.
// A host below the floor fails plainly: the FloorError names the tool, the version found and the floor.
//
// # Unparseable lines
//
// A line that does not parse, such as `tmux next-3.6`, `tmux master` or an empty line, returns (false, nil).
// A development build reports no comparable version, so the caller admits the tool with one Warn instead of refusing a host that may be newer than the floor.
//
// # The feature audit
//
// The floor is only true while no code requires a feature newer than it.
// The audit covers every git subcommand, flag and config key and every tmux command, option and format feature lyx uses, in production code, test kits and `tools/`, that arrived after git 2.20 or tmux 3.0.
// Each version below is the release that introduced the feature, confirmed against git's RelNotes and tmux's upstream CHANGES file.
//
// The git features found are `branch --show-current` (2.22), the `%(worktreepath)` format atom (2.23), `cherry-pick --skip` (2.23), `init --initial-branch` (2.28), `worktree repair` (2.29), the `maintenance.auto` key (2.29), the `receive.procReceiveRefs` key a test sets (2.29), `rev-parse --path-format=absolute` (2.31), the `push.autoSetupRemote` key (2.37), `merge-tree --write-tree` and `--no-messages` (2.38), and `worktree add --orphan` (2.42).
// All are at or below the floor.
//
// The tmux features found are pane options and pane user options through `set-option -p` and `show-options -p`, the status-format list and range markup (`list=`, `range=window|`, `range=session|`, `norange`, `nolist`), `window-size latest`, the `-f` filter of `list-sessions`, `break-pane -b`, the `#{T:}`, `#{E:}`, `#{e|…}`, `#{>:}`, `#{&&:}` and `#{||:}` format modifiers and user ranges (`range=user|`).
// All are at or below the floor, tmux 3.4 being the release that added user ranges.
//
// Three forms are found above the floor.
// `cherry-pick --empty=drop` (git 2.45) is found and to be replaced.
// The two-format `#{S:}` loop, which tmux 3.4 does not run, is found and to be replaced.
// The `allow-set-title` pane option (tmux 3.5) is an optional above-floor feature lyx keeps:
// the launch of a strand pane sets it to stop the agent overwriting the pane title, and on tmux 3.4 the `set-option` fails, reed Warns and continues, so the title stays one the agent can overwrite.
// It is never met by raising TmuxFloor.
//
// # The rule for a new feature
//
// A new git or tmux feature must exist at the floor, or be kept optional with a fallback that Warns and continues.
package toolfloor
