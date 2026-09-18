# Plan: Launch ly-supervise and orchestrator via lyx reed add

```yaml
task: "Launch ly-supervise and orchestrator via lyx reed add"
slug: "ly-supervise-reed-add"
approved: true
started: "20260918-171126"
parent: "main"
root: ""
verify: null
discussion_sha: 0f65cdd1555b2c978b976d96e49e10541acae4a6
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: reed-if-absent
    file: 01-reed-if-absent.md
    depends-on: []
    verify: go test ./internal/reedengine/... ./internal/reedcli/... && go test -tags integration ./internal/reedcli/... && go vet -tags smoke ./internal/reedcli/
  - number: 2
    name: vscode-launch-chain
    file: 02-vscode-launch-chain.md
    depends-on: []
    verify: go test ./internal/vscode/... ./internal/ideengine/...
  - number: 3
    name: skill-and-roadmap
    file: 03-skill-and-roadmap.md
    depends-on: [1, 2]
    verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
```

## Shared Decisions

### Decision: docs-land-in-the-same-commit-as-the-behaviour

- **Decision:** a card that changes observable behaviour carries its own doc edit in the same card, and therefore the same commit — the sandbox scenario rides with the `--if-absent` CLI flag card, and the `docs/overview.md` **ide** bullet rides with the `WriteConfig` card.
  Only the roadmap move and the `ly-supervise` skill rewrite stand as their own cards, in the last batch.
- **Rationale:** `CLAUDE.md`'s Task-completion rule requires docs for an observable CLI change to land in the same commit, and mill's execution model is one commit per card.
  Splitting the doc into its own card would produce a second commit and break that rule.
  The roadmap is the sanctioned exception: it "moves only on completing a planned item", which is only true once every other batch has landed.
- **Applies to:** all batches

### Decision: if-absent-decision-logic-lives-in-the-engine

- **Decision:** the whole `--if-absent` branch decision — matching, the candidate set, the liveness predicate, and candidate selection — lives in `internal/reedengine`.
  `internal/reedcli/add.go` maps the flag onto `reedengine.AddSpec` and nothing more.
- **Rationale:** CONSTRAINTS.md's CLI / Cobra Invariant fixes the direction of the dependency (`reedcli` imports `reedengine`; the engine never imports cli or cobra), and the discussion's Constraints section names this placement explicitly.
  It also puts the decision table where untagged unit tests can reach it with no tmux.
- **Applies to:** reed-if-absent

### Decision: pure-classifier-so-the-decision-table-is-hermetic

- **Decision:** the four-row branch table is a pure function over `[]Strand` plus an alive-pane-id set, separate from the code that performs the branch.
  The function is what the untagged unit tests drive;
  the real relaunch (which reaches tmux) is covered by the tagged smoke test only.
- **Rationale:** CONSTRAINTS.md's Test Tier Purity Invariant bars spawns from untagged files, and `internal/reedengine`'s existing tests already work this way — `planResumeLaunches` is a pure planner tested in `lifecycle_test.go`, and `strand_test.go` drives the `*Locked` helpers against a fixture `.lyx` precisely because they never reach tmux.
  Modelling `--if-absent` the same way keeps every row of the table testable without a live server.
- **Applies to:** reed-if-absent

### Decision: liveness-predicate-is-copied-from-planResumeLaunches

- **Decision:** a candidate is alive when `s.PaneID != "" && aliveIDs[s.PaneID]`, with `aliveIDs` built by `aliveIDSet`, never `liveIDSet`.
- **Rationale:** `--if-absent` asks the same question `Resume` asks, so it must answer it the same way.
  `planResumeLaunches` uses both halves for a reason the discussion records: `Up` clears every pane binding on a freshly booted server, so a rebooted machine's persisted strand arrives with an empty `PaneID`, and a set-membership-only predicate would index on `""` and reach the wrong branch.
  `aliveIDSet` rather than `liveIDSet` because tmux keeps a session's sole dead pane present, and a strand bound to it must relaunch rather than read as live.
- **Applies to:** reed-if-absent

### Decision: matched-branches-never-rewrite-persisted-spec

- **Decision:** neither matched branch writes any field of the persisted strand from this invocation's flags.
  The no-op mutates nothing at all — not `Cmd`, not `ResumeCmd`, not `Parent`, not `Display`.
  The relaunch writes only the new `PaneID` binding, and launches the **stored** `ResumeCmd`-else-`Cmd`, exactly as `Resume` does.
- **Rationale:** an `add` that silently rewrote an existing strand's recorded command is an update verb wearing `add`'s name, and it would let a stale generated task file redefine a strand the operator configured by hand.
  `UpdateStrand` is where a deliberate mutation belongs, and it is engine-API-only in v1.
  Not rewriting also keeps a name-matched relaunch identical to what `resume` would have done for the same strand, so the two entry points cannot diverge.
- **Applies to:** reed-if-absent

### Decision: vscode-tasks-json-is-not-a-pane-shell-command

- **Decision:** the generated `tasks.json` builds its commands as a VS Code `command` string plus an `args` array, and does **not** route through `internal/shell`.
- **Rationale:** CONSTRAINTS.md's Shell Mechanics Seam governs *pane-shell* command strings — the strings reed hands to tmux to run inside a pane.
  A `tasks.json` entry is consumed by VS Code's own task runner, which applies its own per-platform quoting to `command` + `args`;
  pre-quoting with `internal/shell` would double-quote it.
  Keeping the arguments in an `args` array rather than concatenating them into one string is also what keeps a stamped path containing spaces correct on both platforms.
- **Applies to:** vscode-launch-chain

### Decision: stamping-a-path-is-not-a-re-exec

- **Decision:** `internal/ideengine`'s `Spawn` calls `os.Executable()` and `exec.LookPath` to resolve the two binary paths it stamps into the generated file, including under `go test`.
- **Rationale:** CONSTRAINTS.md's Live-Substrate Spawn Observability rule bars **re-exec**ing `os.Executable()` under `go test`, not reading its value.
  Nothing here starts a process: the resolved strings are written into a JSON file in a temp dir.
  The discussion records this reading explicitly.
- **Applies to:** vscode-launch-chain

### Decision: writeconfig-owns-the-bare-name-fallback

- **Decision:** `WriteConfig` substitutes the bare name (`lyx`, `claude`) for any empty path it is handed.
  `Spawn` passes whatever resolution produced, empty string included, and never substitutes first.
- **Rationale:** one owner, in the layer that writes the file, so the assertion has one obvious home and both fallbacks are testable against `WriteConfig` itself.
  `Spawn` may still log a resolution failure;
  it must not paper over it with a second copy of the rule.
- **Applies to:** vscode-launch-chain

### Decision: no-guard-for-concurrent-windows

- **Decision:** two VS Code windows open on one worktree each run the chain, `--if-absent` no-ops on the second, and both attach.
  No detection, no guard, no new state.
- **Rationale:** two tmux clients whose terminal sizes differ clamp the layout to the smaller, which is tmux's ordinary multi-client behaviour and no worse than today's editor terminal plus a side `reed attach`.
  The operator resolves it by closing one window.
- **Applies to:** all batches

## All Files Touched

- `docs/overview.md`
- `internal/ideengine/spawn.go`
- `internal/ideengine/spawn_test.go`
- `internal/reedcli/add.go`
- `internal/reedcli/cli_integration_test.go`
- `internal/reedcli/smoke_ifabsent_test.go`
- `internal/reedengine/strand.go`
- `internal/reedengine/strand_test.go`
- `internal/vscode/config.go`
- `internal/vscode/config_test.go`
- `manifest/designs/reed-header-selvage.md`
- `manifest/roadmap.md`
- `plugins/ly/skills/ly-supervise/SKILL.md`
- `tools/sandbox/SANDBOX-REED-SUITE.md`
