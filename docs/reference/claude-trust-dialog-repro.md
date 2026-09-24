# Reproducing the llm-driver trust-dialog hang

Manual, operator-run verification for the fix in the startup step inside
`internal/shuttleengine.Runner.StartGated`, and `internal/loomcli`'s llm arm, which starts the
driver through `StartDriver` and no longer awaits readiness itself.
Not automated: it launches a real, billed interactive Claude session and mutates the
operator's own `~/.claude.json`, which puts it outside the Test Tier Purity boundaries any
`go test` run must stay inside.

The task is not verified until this recipe has passed on a path this host has never trusted.

## Why the path matters

The trust gate keys on the child worktree's **absolute path** in `~/.claude.json`'s `projects`
map, not on worktree freshness.
A repro that reuses an already-trusted path passes silently and proves nothing — that is
exactly how an earlier reproduction attempt lost the finding.

## Steps

1. Build `lyx` from this branch: `go build ./cmd/lyx` (cgo required).
2. Choose a hub path containing a fresh UTC timestamp so it cannot have been trusted before,
   for example `$HOME/Code/r5sandbox/trust-repro-<YYYYMMDD-HHMMSS>-LYXHUB`.
   Create a fixture hub there through lyx's own clone flow (`lyx clone`; pin the exact
   invocation from `lyx clone --help`).
   The fixture must not be hand-assembled (hubforge Fabric-Fixture Invariant spirit).
3. Before launching, prove that the child worktree's absolute path is untrusted:
   `jq --arg p "<abs worktree path>" '.projects | has($p)' ~/.claude.json` must print `false`.
   If it prints `true`, pick a new timestamp — the run would pass silently and prove nothing.
4. In that worktree, run `lyx shed seed self --recipe loom --driver llm --param parent=<recorded parent branch>`,
   then `lyx loom start --no-attach`.
   `lyx loom start` reads and writes only the `self` run (`shedrun.SelfRunID`, via
   `seedAndCommitBootstrap` and `resolveRunID`).
   `WriteSeed` refuses a seed whose params disagree with `loomSeedFor`'s `{"parent": <parent>}`,
   so the seed must be at `self`, and it must carry the same parent that `start` resolves — a
   seed under any other run-id leaves `self` defaulting to the go driver, and the live check
   would pass without exercising the llm arm.
   Confirm the llm arm ran: `lyx reed status` must list a strand named
   `driverStrandDisplayName`'s value (the ly-drive driver).
   There must be no detached go runner: the driver log named by `LoomDriverLog` is absent or
   empty for this run.
5. Pre-fix expectation (baseline, optional, from a `main` build): `start` returns success, and
   `tmux capture-pane -p -t <driver pane>` shows the trust dialog ("Yes, I trust this folder")
   indefinitely.
6. Post-fix expectation: `start` returns only after dismissal, the driver pane shows the
   ly-drive session working, and
   `jq --arg p "<abs worktree path>" '.projects[$p].hasTrustDialogAccepted' ~/.claude.json`
   prints `true`.
   That checks the acceptance field itself, not the entry's presence — Claude Code creates an
   entry for every launch directory whether or not the gate was accepted.
   A `run.json` under the driver's run dir carries `started: true`.
   Had the trust dialog instead never cleared, `start` would refuse with shuttle's `ErrNotStarted`
   message instead of returning success, and `startup-capture.txt` would be left in the driver's
   run dir as the diagnosis artifact (the last pane capture taken before the not-ready teardown).
7. Tear the fixture down afterwards (`lyx` teardown / removing the hub dir).
   The `~/.claude.json` entry it leaves is harmless: Claude Code appends an entry for every
   launch directory anyway.

## Recording the outcome

Record, in the PR or task notes: the `jq` output before and after step 6, and a
`tmux capture-pane` snippet showing the dialog appear (pre-fix baseline, optional) and the
driver pane proceed past it (post-fix).
