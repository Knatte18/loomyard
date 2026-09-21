# Plan: Spawned agent panes resolve the spawning lyx binary

```yaml
task: "Spawned agent panes resolve the spawning lyx binary"
slug: "lyx-bin-pane-path"
approved: false
started: "20260921-114353"
parent: "main"
root: ""
verify: null
discussion_sha: "c2df456416a72822120930bd170bee78d77e7d1f"
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shell-prelude-primitives
    file: 01-shell-prelude-primitives.md
    depends-on: []
    verify: go test ./internal/shell/
  - number: 2
    name: reed-pane-binary-chokepoint
    file: 02-reed-pane-binary-chokepoint.md
    depends-on: [1]
    verify: go test ./internal/reedengine/ && go test -tags integration ./internal/reedengine/
  - number: 3
    name: docs-and-sandbox-preconditions
    file: 03-docs-and-sandbox-preconditions.md
    depends-on: [2]
    verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks && go test ./cmd/lyx/ -run TestSandboxCoverage_AllModulesCoveredOrExcluded
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: prelude-dialect-is-ForGOOS

- **Decision:** the prelude is built with `shell.ForGOOS()`, never `e.cfg.Shell`, and no batch adds a dialect selector, a `validateToldPaneShell` refusal, or any config-driven shell plumbing.
- **Rationale:** the prelude is `;`-joined onto a launch command that both production builders (`internal/shuttleengine/claudeengine/claudeengine.go`, `internal/loomcli/sharedbootstrap.go`) already build with `shell.ForGOOS()`; the two are one line typed into one shell, so the only property that matters is that they agree with each other. `e.cfg.Shell` does not govern a strand pane at all — it governs the `new-session` first pane and Selvage, neither of which has anything typed into it.
- **Applies to:** all batches

### Decision: pane-start-mode-is-untouched

- **Decision:** no batch changes how a strand pane's shell is started. `launchStrandLocked` keeps splitting with no trailing shell-command; the `split-window` argv, `new-session`'s argv, and Selvage's own split are all byte-identical to today.
- **Rationale:** a commandless `split-window` has tmux start `default-shell` as a login shell, which sources `~/.profile` / `~/.bash_profile`. A trailing shell-command is handed to `/bin/sh -c` and execs a non-login shell that skips them, changing the pane's inherited `PATH`. That pane resolves `claude` by bare name, so a start-mode change would make a task about resolving the right binary stop resolving the agent binary at all.
- **Applies to:** all batches

### Decision: chain-separator-is-semicolon

- **Decision:** `Chain` joins with `"; "` in both dialects and never `&&`.
- **Rationale:** the prelude is env decoration and must not be able to suppress the agent's own launch command. A shell that rejects the prelude leaves that pane on the ambient `PATH` while the agent still runs — strictly better than a dead agent pane. `&&` is additionally a pwsh 7.0+ pipeline-chain operator with semantics that differ from POSIX's.
- **Applies to:** shell-prelude-primitives, reed-pane-binary-chokepoint

### Decision: executable-error-warns-and-degrades

- **Decision:** when the executable path cannot be resolved, log one `logger.Warn` naming the strand and the cause, then launch the pane with the unmodified launch command. Never fail the strand launch.
- **Rationale:** the branch is near-unreachable on Linux and Windows, and on any machine that could reach it the prelude would have resolved to the production binary anyway — the exact `PATH` the pane already had. A logged, named degradation is not a silent fallback. This mirrors `SpawnWatchdog`'s own handling of the same error in the same package.
- **Applies to:** reed-pane-binary-chokepoint

### Decision: claude-resolution-check-is-shuttle-only

- **Decision:** the two binary-resolution pre-condition checks (`lyx` resolves to the spawning binary; `LYX_BIN` names the same binary) go into both sandbox suite docs, while the third check — `claude` still resolves inside a spawned agent pane — goes into `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` only, with a one-line pointer from the reed suite.
- **Rationale:** the reed suite's strands run operator-supplied commands and it spawns no agent pane, so a `claude`-resolution check has nothing to exercise there — the same reasoning that put `SANDBOX-REED-WATCH-SUITE.md` out of scope entirely. The shuttle suite is where an agent pane actually exists.
- **Applies to:** docs-and-sandbox-preconditions

### Decision: no-new-tmux-capability

- **Decision:** no batch adds a verb or flag to `requiredSubcommands`, and no batch touches the minimum-version pins.
- **Rationale:** the prelude is a `send-keys` payload, which is dialect-level rather than multiplexer-level. `split-window -e` and `set-environment` were both rejected partly because psmux's support for them is unverified and this package declines unverified psmux capabilities.
- **Applies to:** all batches

### Decision: markdown-semantic-line-breaks

- **Decision:** every `.md` file any batch touches uses semantic line breaks — one sentence per line, an extra break at an internal independent-clause boundary, plain newline only.
- **Rationale:** `CLAUDE.md` mandates it repo-wide, and it applies to existing prose being edited, not only to newly written prose. Table cells and blockquotes stay on one line.
- **Applies to:** docs-and-sandbox-preconditions, reed-pane-binary-chokepoint

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `crucible/README.md`
- `docs/sandbox-howto.md`
- `internal/reedengine/doc.go`
- `internal/reedengine/emptycmd_integration_test.go`
- `internal/reedengine/panebin.go`
- `internal/reedengine/panebin_enforcement_test.go`
- `internal/reedengine/panebin_test.go`
- `internal/reedengine/spawn.go`
- `internal/reedengine/spawn_test.go`
- `internal/shell/posix.go`
- `internal/shell/pwsh.go`
- `internal/shell/shell.go`
- `internal/shell/shell_test.go`
- `tools/sandbox/SANDBOX-REED-SUITE.md`
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
