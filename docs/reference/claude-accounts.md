# Claude accounts on one machine

lym can spread Claude usage over the accounts logged in on one machine.
Each worktree runs its Claude sessions under one account.
The account is picked by weight and headroom when the worktree is made on the machine,
and it is switched to another account when its own runs out.
The whole policy is machine-local: it is never committed, and no other machine reads it.

## The account keys

Two keys of the `claude` module hold the policy.
They live only in the machine layer, so set them with `lym config claude --layer machine`, never in a tracked file.

- `claude.accounts` lists the accounts.
  Each has a `name` (a-z, 0-9 and `-`), a `config_dir` (absolute, or starting with `~/`),
  a `weight` (0 retires the account) and `windows`.
  `windows` gives, per `five_hour` and `seven_day`, the used percentage from 1 to 100 at or above which the account is passed over.
- `claude.account_pins` pins the hub's worktrees (`hub`) or its beams (`beams`) to one listed account of non-zero weight.
  A hub pin also keeps the worktree from switching away from it on a usage limit.

An example with two accounts, the second pinned for the hub's beams:

```yaml
accounts:
  - name: personal
    config_dir: ~/.claude
    weight: 2
    windows:
      five_hour: 90
      seven_day: 95
  - name: work
    config_dir: ~/.claude-work
    weight: 1
    windows:
      five_hour: 90
      seven_day: 95
account_pins:
  hub: ""
  beams: work
```

## Wiring a second account

The operator owns this work; lym only reads the result.

1. Log the account in under its own directory:
   run `CLAUDE_CONFIG_DIR=<dir> claude`, then `/login`.
2. Link `<dir>/plugins` to `~/.claude/plugins`.
   lym's deploy syncs plugins into `~/.claude`, so the link lets it serve both accounts.
3. Link `<dir>/settings.json` to `~/.claude/settings.json`, so both accounts carry the same `enabledPlugins`.
4. Copy the MCP servers of `~/.claude.json` into `<dir>/.claude.json`, so both accounts load the same tools.
   A transcript copied from one account into the other resumes only where the tools it used are present.
5. Link the user-level instruction file as wanted.
6. Add the account to `claude.accounts` as above.

Logging in, preparing the directories, and whether the use is within the accounts' and the organisation's terms
are the operator's to settle.

## Checking and changing the account

- `lym claude account show` prints the account the current worktree resolves to, the pins,
  and for each listed account its weight, headroom, the time a usage limit blocks it until, and each usage window.
- `lym claude account set <account>` sets the worktree's account for its next Claude launch.
  A running session keeps its account.
  It refuses under a hub pin.
- A session on a listed account runs lym's status line, which records the account's usage windows as the session works.
- An operator's own `CLAUDE_CONFIG_DIR` no longer reaches the panes lym starts, since lym sets the directory per account.
  The `env` module is the way to set one variable for every pane.

## Manual check

Run this once on OSL-1046 after the task lands on neve and reaches OSL-1046.
It is the first run of a copied transcript resuming under the second account.

1. Start two task runs.
2. Run `lym claude account show` in each worktree, and confirm each worktree's entry.
3. Confirm that a transcript lands in each account's `projects/` directory.
4. Switch one worktree with `lym claude account set <other account>`.
5. Resume a session in that worktree, and confirm it resumes under the other account with its history.
