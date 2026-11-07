# SANDBOX-WEBSTER-SUITE -- lym webster black-box suite

## What this is

A structured test-loop for exercising `lym webster` against a **live tmux server and a logged-in claude** in the sandbox Hub's Fabric repo. `webster` is the implementer module, with its own plan format and report contract.
Instead of spawning a fresh reed/tmux strand per batch, one long-lived **Master** session reads the whole flat card-list plan and the repo's rules once, never the code.
`internal/planparser` parses the plan, and `internal/batcher`'s config-selected batchifier (the `wide` cost profile by default) groups it into execution batches.
Master then forks one implementer per batch in-session (Claude Code's Agent tool, `subagent_type: "fork"`), and each fork reads its cards' code itself.
Master brackets each fork with `begin-batch`, `await-batch` and `record-batch` calls.
Forks are BACKGROUNDED agents on current Claude Code: the Agent call returns immediately.
Master therefore long-polls `await-batch` for the batch report, and never ends its turn while a batch is open.
This suite is narrow: it holds one scenario, W1, because only the fork loop needs a live session.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lym.exe` into `.dev-bin` from current source.
   The suite resolves `.dev-bin` itself and prepends it to the driving agent's PATH, so no PATH setup is needed, and production `lym` stays untouched.
   The fingerprint header's `Source: dev` line confirms the dev build is under test.
   The deployed binary is a snapshot: re-deploy after any source change you want to test.
   **Pane-resolution caveat:** the dev-PATH prepend covers only the driving agent's own calls.
   Master's bracket verbs run inside the reed pane's claude, whose Bash tool resolves `lym` through the login-shell snapshot.
   If the shell profile prepends the production bin dir (e.g. `~/.local/bin` on Linux), that resolves the production binary.
   The run then mixes dev (outer `run`, prompt rendering) with prod (Master's `begin/await/record/recover-batch` calls).
   When prod is stale, the run fails with a confusing config error from the wrong binary.
   When prod is merely different, the mismatch is silent, and the fingerprint header attests a binary Master never ran.
   Before trusting any W1 verdict, make the pane-resolved `lym` the same build as `.dev-bin`:
   either deploy the branch build to the production location for the session and restore it at teardown,
   or verify that `lym webster status` from inside a pane names no config error a dev-binary call does not.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean);
   the session cwd is the Hub's Fabric repo root.
3. **Live-tmux and claude requirement.**
   tmux (or the Windows tmux port) on PATH, PowerShell 7, and a logged-in `claude` on PATH.
   If any of these is unavailable in the session, **note that as the session outcome rather than treating it as a webster defect**.
   The `**Covers:** webster` tag on W1 satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless of runtime availability.
4. **Wired worktree required.**
   `lym webster` requires a worktree wired by `lym fabric clone` or `lym fabric add`.
   Wiring materializes `_lym/config/webster.yaml` and `batcher.yaml`, plus `shuttle.yaml` and `reed.yaml` because webster branches off shuttle directly.
5. **`lym reed up` is optional before a spawn.**
   `webster run` spawns the Master session through shuttle, which reaches `AddStrand`.
   `AddStrand` boots a cold worktree's session with `up`'s own semantics before adding the strand,
   so a bare reed session is not a precondition.
   An operator may run `lym reed up` first to watch the session come up in a separate pane.
6. **Attached interactive terminal.**
   Launch `sandbox/webster-suite.cmd` from a real, attached console -- never redirected, backgrounded, or detached.
   Without a TTY the driving claude session cannot idle between turns waiting for notifications,
   so the process ends as soon as a turn ends and the remaining scenarios are silently abandoned.
   The launcher prints a warning when it detects non-console stdio.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lym-test-LYMHUB/lym-test`).
It tests `lym.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lym source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

The agent discovers the command surface via `lym webster --help` and `lym webster <subcommand> --help` alone,
not from documentation outside the Hub.
The plan files under `_lym/plan/` are the one artifact the agent must construct itself, per each scenario's Goal below.
The plan is a plan-format flat card list: `00-overview.md` (frontmatter `format: 5` / `approved: true`, a `## Card Index`)
plus one `NN-<card-slug>.md` per card.
Each card carries one or more bold type labels, each with its own backtick-wrapped target bullets:
`**Create:**`, `**Edit:**`, `**Extends:**`, `**Delete:**`, `**Rename:**`, `**Move:**`, `**Prosa:**`, `**Custom:**`.
A card may carry an optional `**Uses:**`, and requires an `**Intent:**`.
A card with an `Edit`, `Extends` or `Delete` group also requires an `**ImpactSummary:**`.
A field with no content is omitted rather than carrying a `none` sentinel.
The agent reasons the shape out from `lym webster validate`'s error messages, which name every violated check.
Keep every scenario's plan cards trivial, e.g. "create `resultN.md` containing the single line `OK`".
A real fork then finishes each batch in one card and one commit, fast.

### Controlled exceptions

One sanctioned deviation from the pure black-box rule:

- **Direct `tmux -L <socket> list-panes`/`ls`** is allowed only to confirm Master's own strand exists (or was cleaned up).
  Read `<socket>` from `lym reed status` output.
  W1 also uses it to confirm no extra strand appears per batch:
  a fork is not a new strand, so the whole run has exactly one implementer-bearing strand, Master's own.

## Fingerprint header

The launcher copies this file into the Hub's Fabric repo with a "binary under test" fingerprint block prepended.
The fingerprint records the absolute path, file size, modification time, and a short SHA-256 of the `lym.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance.
A separate fetch step, run after this session, stamps it into `meta.fingerprint` of the fetched `sandbox-report.json`,
so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal**: it names the task, not the commands.
  Discover the commands via `lym webster`, `lym webster <subcommand>`, and `--help` flags.
- **Watch** what lym does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete;
  lym broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write every `WARN`/`FAIL` finding to `./sandbox-report.json` in the Fabric-repo cwd.
Use this exact schema.
**Always write the file, even when there are zero `WARN`/`FAIL` findings** -- in that case `items` is an empty array.

```json
{
  "source": "sandbox-report",
  "items": [
    {
      "ref": "W1b",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`W1`, or `W1b` for the separately-verdicted fork-transcript flush assertion).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items`.
A separate fetch step, run after the session, stamps `meta`, including the binary fingerprint.
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### W1 -- Happy path (full `run`, in-session forks)

**Covers:** webster

**Goal:** Pin a tiny two-card plan whose cards each write one fixed-content file, and run `lym webster run`.
Confirm it reaches a `"outcome":"done"` outcome with both batches' cards committed.
The run must drive itself end-to-end through in-session Agent-tool forks, not new reed strands.

**Watch:** `lym webster run` blocks until the run reaches a terminal outcome;
the printed JSON envelope reports `"outcome":"done"` with `batches_done: 2`.
Confirm **Master grounds itself instead of refusing**: attach early (`lym reed attach <guid>`).
The freshly spawned Master's first actions must be the two read-only orientation checks the master template opens with
(`lym webster status` and `ls _lym/plan/`).
It then proceeds into the loop, and must NOT end its turn asking whether the harness or prompt is real.
The grounding block is tuned against this injection-refusal failure mode;
an occasional refusal here is a highest-priority finding, not a transient.
Confirm **one fork per batch, no extra reed strands during batches**:
`lym reed status` shows exactly one implementer-bearing strand for the entire run, Master's own.
No second strand appears and disappears per batch.
Confirm **per-batch weft commits landing** by inspecting the weft's own commit log.
Each batch's `begin-batch` commits `state.json`, making the start-SHA and batch entry durable before the fork.
Each batch's `record-batch` (the main per-batch sync) commits the batch report plus `state.json`.
Confirm **digest envelopes from `record-batch`**: each batch's fork-return is followed by a `record-batch` call.
Its JSON response carries webster's pinned digest fields: `batch`, `status`, `head_sha`, `deviations`, `dead_reason`, `elapsed_s`.
The digest is the minimal fork-return summary, never raw report prose, and absent optional fields may be omitted.
Confirm **digest carry-forward**.
Batch 2's rendered fork prompt is `.lym/webster/prompts/02-*.md`.
Its "Prior-batch context" section embeds batch 1's one-line persisted digest.
Confirm **a valid `summary.md`** at exit: `_lym/webster/summary.md` exists, its first line is `# <title>`,
and the rest is a non-empty narrative.
`_lym/webster/outcome.yaml` must exist alongside it.
Afterward, Master's pane and run dir must be cleaned up: no strand is left, and `lym reed status` no longer lists it.

Confirm **fork-transcript flush timing** as its own assertion (`W1b`) with its own verdict.
By the time a fork has COMPLETED, its `subagents/<id>.jsonl` transcript file must already exist on disk,
under the session's `~/.claude/projects/<encoded-cwd>/<sessionID>/subagents/` directory.
A fork has completed when its report file has landed, the moment `await-batch` returns `{"report": true}` and Master calls `record-batch`.
`record-batch`'s incremental per-batch audit checks the transcript count before report presence, with a bounded settle retry.
It depends on the flush happening within seconds of the report, not merely by session end.
Forks are backgrounded on current Claude Code, so the Agent call returning is the spawn acknowledgment, not completion.

**Verdict:** `OK` / `WARN` / `FAIL` for W1 and for W1b independently;
record both in the session log and name whichever one failed.

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

W1:  <OK|WARN|FAIL> -- <one-line note if not OK>
W1b: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, with `items: []` when every scenario was `OK`.

## Teardown

When the session summary and `./sandbox-report.json` are written, run `lym reed down`.
It tears down the tmux session and server the scenarios booted.
An orphaned tmux server holds open handles in the Hub's Fabric repo and blocks the next `sandbox/build.cmd -reset`.
The launcher also runs `lym reed down` after the session ends as a backstop.
Run it here anyway: it keeps the Hub clean while the session is still open for inspection.

## Notes

- This suite holds only webster's fork-loop scenarios: add `W` scenarios here, not in any other suite.
  Warp/weft scenarios belong in `SANDBOX-CORE-SUITE.md`, reed/tmux scenarios in `SANDBOX-REED-SUITE.md`.
  Shuttle black-box agent scenarios belong in `SANDBOX-SHUTTLE-SUITE.md`, burler's review+fix rounds in `SANDBOX-BURLER-SUITE.md`.
- This suite is a floor, not a ceiling.
  The run-level mechanics (fingerprinting, `--fresh` archiving, `run.lock` contention, plan validation, crash reclaim) are webster-local Go code.
  The hermetic and `-tags integration` test tiers and `internal/webstercli/smoke_test.go`'s live smoke tests exercise them;
  duplicating them here would re-test covered Go paths through a slower medium.
  Only the fork loop (W1) is live-only.
  The suite does not prove implementer quality or plan-format content richness; a normal code review covers those.
