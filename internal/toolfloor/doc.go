// Package toolfloor declares the oldest git and tmux lym runs on.
// It parses a tool's version line and returns the floor error.
// CheckGit reads `git --version` output and CheckTmux reads `tmux -V` output;
// each returns whether the line parsed, and a *FloorError when the parsed version is below the floor.
// TmuxHasAllowSetTitle reads `tmux -V` output for the one optional above-floor tmux feature.
// It spawns no process, keeps no cache and logs nothing:
// gitexec and reedengine run the probes, keep their own process caches and own the Warn.
// It imports the standard library only.
//
// # The floor
//
// GitFloor is 2.43.0 and TmuxFloor is 3.4, the versions Ubuntu 24.04 ships and the oldest lym is proven on.
// A host below the floor fails plainly: the FloorError names the tool, the version found and the floor.
//
// # Unparseable lines
//
// A line that does not parse, such as `tmux next-3.6`, `tmux master` or an empty line, returns (false, nil).
// A development build reports no comparable version.
// The caller therefore admits the tool with one Warn instead of refusing a host that may be newer than the floor.
//
// # The feature audit
//
// The floor holds only while no code requires a feature newer than it.
// The audit covers production code, test kits and `tools/`.
// It lists each git subcommand, flag and config key lym uses that arrived after git 2.20.
// It lists each tmux command, option and format feature lym uses that arrived after tmux 3.0.
// Each version below is the release that introduced the feature.
// It is confirmed against git's RelNotes and tmux's upstream CHANGES file.
//
// The git features found:
//
//   - `branch --show-current` (2.22)
//   - the `%(worktreepath)` format atom (2.23)
//   - `cherry-pick --skip` (2.23)
//   - `init --initial-branch` (2.28)
//   - `worktree repair` (2.29)
//   - the `maintenance.auto` key (2.29)
//   - the `receive.procReceiveRefs` key a test sets (2.29)
//   - `rev-parse --path-format=absolute` (2.31)
//   - the `push.autoSetupRemote` key (2.37)
//   - `merge-tree --write-tree` and `--no-messages` (2.38)
//   - `worktree add --orphan` (2.42)
//
// All are at or below the floor.
//
// The tmux features found:
//
//   - pane options and pane user options through `set-option -p` and `show-options -p`
//   - the status-format list and range markup (`list=`, `range=window|`, `range=session|`, `norange`, `nolist`)
//   - `window-size latest`
//   - the `-f` filter of `list-sessions`
//   - `break-pane -b`
//   - the `#{T:}`, `#{E:}`, `#{e|…}`, `#{>:}`, `#{&&:}`, `#{||:}` and `#{m:}` format modifiers
//   - user ranges (`range=user|`)
//   - `confirm-before -p`
//
// All are at or below the floor, tmux 3.4 being the release that added user ranges.
//
// No feature lym requires is newer than the floor.
// `cherry-pick --empty=drop` (git 2.45) is not used.
// A plain pick skips a change HEAD already carries and refuses a commit empty from the start.
// The two-format `#{S:}` loop, which tmux 3.4 does not run, is not used.
// Two single-format loops run instead, the prime's session first.
// The one above-floor feature lym uses is the `allow-set-title` pane option (tmux 3.5, TmuxAllowSetTitle).
// It stays optional:
// the launch of a strand pane sets it to stop the agent overwriting the pane title.
// TmuxHasAllowSetTitle reads the same `tmux -V` line as CheckTmux.
// On a tmux it knows is older, reed skips the `set-option` and logs that once at Info.
// The title then stays one the agent can overwrite.
// On a version line that does not parse, reed still tries the `set-option`, and a failure Warns and continues.
// The floor is never raised to meet this feature.
//
// # The rule for a new feature
//
// A new git or tmux feature must exist at the floor, or be kept optional with a fallback that Warns and continues.
package toolfloor
