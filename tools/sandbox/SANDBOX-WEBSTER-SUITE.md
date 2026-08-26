# SANDBOX-WEBSTER-SUITE -- lyx webster black-box suite

## What this is

A structured test-loop for exercising `lyx webster` against a **live tmux server and a logged-in claude** in the sandbox Hub's Fabric repo. `webster` is the implementer module, with its own plan format and report contract: instead of spawning a fresh reed/tmux strand per batch, one long-lived **Master** session reads the codebase and the whole flat card-list plan (plan-format, parsed by `internal/planparser`, grouped into execution batches by `internal/batcher`'s config-selected batchifier -- identity by default, one card per batch) once, then forks one implementer per batch in-session (Claude Code's Agent tool, `subagent_type: "fork"`) -- no `spawn-batch`/`poll` verbs exist here;
Master itself brackets each fork with `begin-batch`/`await-batch`/`record-batch` calls (forks are BACKGROUNDED agents on current Claude Code -- the Agent call returns immediately, so Master long-polls `await-batch` for the batch report instead of relying on a synchronous fork return, and never ends its turn while a batch is open).
This suite is deliberately narrow: two scenarios, because webster's pure Go-level mechanics (fingerprinting, `--fresh` archiving, pause, `run.lock` contention, plan validation) are webster-local code already covered by the hermetic and `-tags integration` test tiers -- what is genuinely live-only here is the fork loop itself (W1) and the one dormant-by-default, timing-sensitive mechanism unique to webster: the idempotent per-batch Master model assertion's `/model` pane injection (W2).

## Pre-conditions

Before starting a session:

1. **Deploy a fresh dev binary.**
   Run `deploy-dev` to build `lyx.exe` into `.dev-bin` as current source.
   The suite resolves `.dev-bin` itself and prepends it to the agent's PATH (the fingerprint header's `Source: dev` line confirms the dev build is under test) -- no PATH setup needed for the DRIVING agent, and production `lyx` stays untouched.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
   **Pane-resolution caveat (webster-specific, load-bearing):** the dev-PATH prepend covers only the driving agent's own calls.
   Master's bracket verbs run inside the reed pane's claude, whose Bash tool resolves `lyx` through the login-shell snapshot -- on a machine whose shell profile prepends the production bin dir (e.g. `~/.local/bin` on Linux), that is the PRODUCTION binary, and the run under test silently mixes dev (outer `run`, prompt rendering) with prod (every `begin/await/record/recover-batch` Master calls).
   Three crucible rounds hit this in a row;
   when prod is stale enough the run fails with a confusing config error from the wrong binary, and when prod is merely different the mismatch is silent and the fingerprint header attests a binary Master never ran.
   Before trusting any W1/W2 verdict, make the pane-resolved `lyx` the same build as `.dev-bin` (deploy the branch build to the production location for the session and restore it at teardown, or verify `lyx webster status` from inside a pane names no config error a dev-binary call does not).
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean);
   the session cwd is the Hub's Fabric repo root, the same operating model as the main suite.
3. **Live-tmux and claude requirement.** tmux (or the Windows tmux port) on PATH, PowerShell 7,
   and a logged-in `claude` on PATH.
   If any of these is unavailable in the session, **note that as the session outcome rather than treating it as a webster defect** -- the `**Covers:** webster` tag on W1 satisfies the sandbox coverage guard (`sandbox_coverage_test.go`) regardless of runtime availability.
4. **Wired worktree required.** `lyx webster` requires a worktree wired by `lyx fabric clone`/`lyx fabric add` -- which materializes `_lyx/config/webster.yaml`, `batcher.yaml`, plus `shuttle.yaml`/`reed.yaml` since webster branches off shuttle directly -- exactly like `lyx shuttle`/`lyx burler` do.
5. **`lyx reed up` before any spawn.** `webster run` spawns the Master session through shuttle into an existing reed session and does not boot one itself;
   without it the spawn fails loud with `no reed session; run "lyx reed up"`.
6. **Attached interactive terminal.**
   Launch `sandbox/webster-suite.cmd` from a real, attached console -- never redirected, backgrounded, or detached.
   Without a TTY the driving claude session cannot idle between turns waiting for notifications, so the process ends as soon as a turn ends and the remaining scenarios are silently abandoned.
   The launcher prints a warning when it detects non-console stdio.

## Black-box rule

**The agent under test works exclusively inside the Hub's Fabric repo (`lyx-test-HUB/lyx-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx webster --help` and `lyx webster <subcommand> --help` alone -- not from documentation outside the Hub.
The plan file(s) under `_lyx/plan/` are the one artifact the agent must construct itself per each scenario's Goal below: a plan-format flat card list -- `00-overview.md` (frontmatter `format: 4` / `approved: true`, a `## Card Index`) plus one `NN-<card-slug>.md` per card carrying one or more bold type labels from the seven-name set (`**Create:**`/`**Edit:**`/`**Delete:**`/`**Rename:**`/`**Move:**`/`**Prosa:**`/`**Custom:**`), each with its own backtick-wrapped target bullets, an optional `**Uses:**`, a required `**Intent:**`, and an `**ImpactSummary:**` on cards with an `Edit`/`Delete` group -- a field with no content is omitted rather than carrying a `none` sentinel -- reason the shape out from `lyx webster validate`'s error messages, which name every violated check.
Keep every scenario's plan cards trivial -- e.g. "create `resultN.md` containing the single line `OK`" -- so a real fork finishes each batch in one card, one commit, fast.

### Controlled exceptions

Two sanctioned deviations from the pure black-box rule, mirroring the reed/shuttle/burler suites' own controlled-exception notes:

- **Direct `tmux -L <socket> list-panes`/`ls`** is allowed only to confirm Master's own strand exists (or was cleaned up), where `<socket>` is read from `lyx reed status` output -- this is also how W1 confirms no EXTRA strand appears per batch (a fork is not a new strand;
  there is exactly one implementer-bearing strand, Master's own, for the whole run).
- **One targeted `_lyx/webster/state.json` edit (W2 only).**
  The per-batch model assertion is DORMANT in the shipped flow -- `run` launches Master with the master role's model and baselines the persisted `assertedModel` to that same value at entry, so `begin-batch`'s idempotency check never finds a divergence on its own.
  W2 arms the injection by editing the `assertedModel` value in `_lyx/webster/state.json` mid-run (see W2's Goal);
  no other scenario may touch webster state by hand.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies it into the Hub's Fabric repo.
The fingerprint records the absolute path, file size, modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate fetch step (run after this session) stamps it into `meta.fingerprint` of the fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lyx webster`, `lyx webster <subcommand>`, and `--help` flags (S0 ethos).
- **Watch** what lyx does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete;
  lyx broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json` (in the Fabric-repo cwd) on this exact schema.
**Always write the file, even when there are zero `WARN`/`FAIL` findings** -- in that case `items` is an empty array.

```json
{
  "source": "sandbox-report",
  "items": [
    {
      "ref": "W2b",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`W1`, or `W2a`/`W2b`/`W2c` for W2's three separately-verdicted assertions).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps `meta` (including the binary fingerprint).
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### W1 -- Happy path (full `run`, in-session forks)

**Covers:** webster

**Goal:** Pin a tiny two-card plan whose cards each just write one fixed-content file, run `lyx webster run`, and confirm it drives itself end-to-end -- via in-session Agent-tool forks, not new reed strands -- to a `"outcome":"done"` outcome with both batches' cards committed.

**Watch:** `lyx webster run` blocks until the run reaches a terminal outcome;
the printed JSON envelope reports `"outcome":"done"` with `batches_done: 2`.
Confirm **Master grounds itself instead of refusing**: attach early (`lyx reed attach <guid>`) and confirm the freshly spawned Master's FIRST actions are the two read-only orientation checks the master template opens with (`lyx webster status` and `ls _lyx/plan/`), after which it proceeds into the loop -- it must NOT end its turn asking whether the harness/prompt is real (which the shuttle classifies asking and surfaces as a `master asked a question` run error).
This is the injection-refusal failure mode the grounding block is tuned against (rounds fable-r1 and opus-r2, crucible);
an occasional refusal here is a real, highest-priority finding, not a transient.
Confirm **one fork per batch, no extra reed strands during batches**: `lyx reed status` shows exactly one implementer-bearing strand for the entire run (Master's own) -- never a second strand appearing and disappearing per batch.
Confirm **per-batch weft commits landing**: `state.json` committed at each batch's `begin-batch` (start-SHA + batch entry durable before the fork),
and the batch report plus `state.json` committed at each batch's `record-batch` (the main per-batch sync) -- inspect the weft's own commit log for both.
Confirm **digest envelopes from `record-batch`**: each batch's fork-return is followed by a `record-batch` call whose JSON response carries webster's pinned digest fields (`batch`, `status`, `head_sha`, `deviations`, `dead_reason`, `elapsed_s` -- webster's own minimal fork-return digest, never raw report prose;
absent optional fields may be omitted).
Confirm **digest carry-forward**: batch 2's rendered fork prompt (`.lyx/webster/prompts/02-*.md`) embeds batch 1's one-line persisted digest in its "Prior-batch context" section.
Confirm **a valid `summary.md`** at exit: `_lyx/webster/summary.md` exists, its first line is `# <title>`,
and the rest is a non-empty narrative -- alongside `_lyx/webster/outcome.yaml`.
Afterward, Master's pane/run dir is cleaned up (no leftover strand;
`lyx reed status` no longer lists it).

**Verdict:** `OK` / `WARN` / `FAIL`

---

### W2 -- Idempotent per-batch `/model` assertion (tamper-armed injection timing)

**Covers:** webster

**Goal:** Arm and observe the ONLY model-injection site in webster -- `begin-batch`'s idempotent per-batch Master model assertion -- under its real production timing: the injection types `/model <master-model>` into Master's own pane **while `begin-batch` itself is still the foreground Bash tool call executing inside that pane**.
The mechanism is dormant by default (the launch model already equals the master role's model and `assertedModel` is baselined at run entry), so arm it deliberately: pin a plan of at least three trivial cards, start `lyx webster run`, and -- from your own session, once you see an early batch's report land under `_lyx/webster/reports/` -- edit `_lyx/webster/state.json`, changing `assertedModel` to any OTHER registered model alias (e.g. `opus`).
The next `begin-batch` then finds the divergence and fires a real `/model` injection racing its own still-running foreground subprocess.
The edit races Master's own state saves;
if a save overwrites your tamper before the next `begin-batch` reads it, nothing fires -- a benign miss, re-tamper at the next batch boundary (this is why the plan needs at least three cards).

**Watch**, recorded as **three separately-verdicted assertions** -- do not fold them into one OK/WARN/FAIL;
a miss on (a) alone is benign, a hit on (b) is dangerous regardless of what (a) or (c) showed:

- **(a) Assertion lands and re-arms idempotency.**
  The injected `/model <master-model>` keystrokes reach Claude's TUI input (capture the pane around the armed `begin-batch` to see them), the armed `begin-batch`'s own envelope reports the master role's model,
  and the FOLLOWING batch's `begin-batch` does NOT re-inject (the persisted `assertedModel` is back at the master model -- the idempotency memory).
  A miss here (keystrokes never land, model never switches) is the BENIGN failure mode: the assertion seam exists for a future per-batch model policy,
  and a no-op injection leaves the run driving on the launch model exactly as if never armed.
- **(b) No corruption of the foreground call.**
  The injected keystrokes do **not** leak into the running `begin-batch` subprocess's own stdin/output: its JSON envelope parses clean in Master's transcript, the run proceeds to fork the batch normally,
  and the batch still reaches `record-batch` with a clean digest.
  **A hit here (corruption) is the DANGEROUS failure mode** -- it means pane injection cannot safely race a foreground tool call at all, regardless of what (a) showed.
- **(c) Fork-transcript flush timing.**
  By the time a fork has COMPLETED (its report file has landed -- the moment `await-batch` returns `{"report": true}` and Master calls `record-batch`), the fork's `subagents/<id>.jsonl` transcript file already exists on disk (under the session's `~/.claude/projects/<encoded-cwd>/<sessionID>/subagents/` directory) -- the incremental per-batch audit's transcript-count-before-report-presence check (`record-batch`, with its bounded settle retry) depends on this flush having already happened within seconds of the report, not merely by session end. (Forks are backgrounded on current Claude Code, so "the Agent call returning" is the spawn acknowledgment, not completion.)

**Verdict:** `OK` / `WARN` / `FAIL` for EACH of (a), (b), (c) independently;
record all three in the session log and name whichever one(s) failed.

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

W1:  <OK|WARN|FAIL> -- <one-line note if not OK>
W2a: <OK|WARN|FAIL> -- <one-line note if not OK>
W2b: <OK|WARN|FAIL> -- <one-line note if not OK>
W2c: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above -- with `items: []` when every scenario was `OK`.

## Teardown

After the session summary is recorded and `./sandbox-report.json` is written, run `lyx reed down` to tear down the tmux session/server the scenarios booted.
An orphaned tmux server holds open handles inside the Hub's Fabric repo and blocks the next `sandbox/build.cmd -reset`.
The launcher also runs `lyx reed down` itself after the session ends (deterministic backstop), but run it here anyway -- defense-in-depth,
and it keeps the Hub clean while the session is still open for inspection.

## Notes

- Warp/weft scenarios stay in `SANDBOX-CORE-SUITE.md`, reed/tmux scenarios stay in `SANDBOX-REED-SUITE.md`, shuttle black-box agent scenarios stay in `SANDBOX-SHUTTLE-SUITE.md`, burler's own review+fix round scenarios stay in `SANDBOX-BURLER-SUITE.md`;
  this suite holds only webster's fork-loop and model-assertion scenarios -- add `W` scenarios here, not in any other suite.
- This suite is a FLOOR, not a ceiling: the run-level mechanics (fingerprinting, `--fresh` archiving, pause, `run.lock` contention, plan validation, crash reclaim) are webster-local Go code exercised by the hermetic and `-tags integration` test tiers plus `internal/webstercli/smoke_test.go`'s live smoke tests -- duplicating those here would re-test covered Go paths through a slower medium.
  What is genuinely live-only is the fork loop itself (W1) and the tamper-armed `/model` pane-injection timing (W2).
  Neither scenario proves implementer quality or plan-format content richness -- those are a normal code review's job.
