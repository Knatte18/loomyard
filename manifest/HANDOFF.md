Load skills `mill:conversation` and `mill:prose` before reading the rest of this document.

# Handoff: producer gates landed (PR #256, `208910e71`); next = manifest cleanup, then the batten crucible

## Where this leaves off

Working directory `/home/knatte/Code/loomyard/wts/loomyard` — the **main** worktree, direct-push allowed here per `CLAUDE.md`.
The user drives design in Norwegian; reply in Norwegian.

**Working rules the user enforces (also in auto-memory): in design discussions, answer with your assessment and get explicit agreement BEFORE editing or committing anything.
And design docs in `manifest/designs/` are NORMATIVE — they describe what SHALL BE, not what is.**
The previous session misread them as descriptive; don't repeat that.
A deliberate implementation narrowing belongs in the doc's status section as a declared deviation, or as a design change agreed first — never as a silent doc rewrite to match shipped code.

## Landed this session

- **Producer gates shipped** — squash-landed on main as `208910e71` (PR #256 closed).
  Full mill pipeline with `--orch`: this session owned the Monitor wait on `_mill/discussion.md`, a fork wrote round 1's `orch-review.md` (APPROVE, two NITs, both later verified addressed).
- Scope-verified against `manifest/designs/producer-gates.md` before merge: gate contract (`GateResult`, error ≠ not-passed), gate in the producer's own control flow (spawn + attach, `SingleLLMProducer` + Burler rounds), `gate_attempts` budget (default 3, exhausted → `Stuck`), the three validate rows removed with guards re-pointed at 14 rows, docs/constraints parity sweep complete.
- Three declared narrowings, documented in the design doc's status section: no compound quiescence (async denied at gated sites, pinned by tripwire tests), findings always via file, gate signature `func()` with closure-captured paths.
- **One gap the scope check caught and the worker fixed pre-merge:** the attempt counter now reaches the row's envelope/status file — `GateAttempts *int` (`omitempty`) on `HistoryEntry`/`OutputPointer`, threaded from all six gated exit points; `nil` = ungated, `0` = passed first try.
  The worker deliberately implemented the broader-than-asked rule (every gated call carries the count, not just exhaustion/retry) — accepted, the design doc states no threshold.

## Next up, in order

1. **Manifest cleanup — user-agreed, not yet started.**
   The user's explicit instruction: what IS done or outdated in `manifest/` can be removed, so the directory again only holds what SHALL BE.
   Discuss the concrete removals with the user before deleting anything (agree-before-writing rule applies).
2. **The batten end-to-end crucible — last, per the user's explicit ordering.**
   Wiki task `crucible-batten-end-to-end` exists; brief is self-contained (run as crucible orchestrator role, disposable fixture hub with own Board, never the operator's sandbox).
   Its ordering precondition is now satisfied (producer gates landed), but the brief's text still says "after seeded-driver-choice merges" and `depends_on` was never set — tidy via the wiki client if touched.
   Sabotage ideas queued for the campaign: raw git commit in weft outside the seams; a stray file dropped into `_lyx` mid-run (stage-all lottery vs. silent loss).

## Still parked

- **Creel (mailbox):** design settled in `manifest/designs/reed-mailbox.md` (addressing `name@slug[.hub]`, guid = identity, reed owns pane titles, pull-mail + `inject`, connector domains post-v1 grammar-reserved only, no MCP layer).
  Someday — don't re-litigate the decisions listed there.
- **Fabric fragility findings:** crucible material, deliberately not tasks (warp-side direct git safe by design; real flanks are Windows junction force-adds and `git -C` into the weft; hardening ideas — PreToolUse deny-hook, untracked-in-weft warning — live only in the crucible framing).

## Loose ends

- Task worktrees on disk post-merge: `wts/seeded-shed-core`, `wts/seeded-driver-choice`, `wts/producer-gates` — `mill-cleanup` when wanted.
- No Monitor waits are armed.
- This file supersedes `.scratch/handoff.md` (deleted).

## Suggested skills for the next agent

- `mill:prose` + `mill:conversation` — load before writing anything.
- `mill:mill-status` / `mill:mill-inspect` — confirm state before acting.
- `mill:orch-review` if a task is started with `--orch` — the session owns the Monitor wait; fork only after `discussion.md` exists.
