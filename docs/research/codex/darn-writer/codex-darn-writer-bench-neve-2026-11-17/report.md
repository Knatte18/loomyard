# Codex `astra[high]` as the darn writer through the real producer, replayed once on `ide-beam-readme-preview` (Neve)

Bench of 2026-11-17, level 2 of `codex-support-levels` (R2): the darn writer's work only, one Codex session, nothing landed or pushed.
Input: the darn light task `ide-beam-readme-preview`, landed as `824eb2b6`; its Claude writer's work is the answer key.
The earlier Neve bench (`codex-darn-writer-replay-neve-2026-11-15`) hand-built its prompt; this one ran the real `Darn-Write` row of the darn recipe.
The verdict on level 2 is the operator's; this report states what the one run showed.

## Manifests

Both records are for the `darn` seat.
`data/manifest-preview-darn.json` is the output of `lym shuttle manifest darn` before the run, both engines.
`data/manifest-codex-launch-record.json` is the record the real Codex launch wrote.

| Channel | Claude (preview) | Codex (preview) | Codex (launch record) |
|---|---|---|---|
| `prompt-engine`, `notice`, `guard`, `batching-nudge`, `edit-directive` | delivered | delivered | delivered |
| `instruction-source`: three heddle files | delivered | delivered | delivered |
| `instruction-source`: `~/.claude/CLAUDE.md` | native | delivered | delivered |
| `skill-load`: the five scribe skills | delivered | delivered | delivered |
| `skill-listing`: fifteen skills | native | delivered | delivered |
| `plugin-context`: the `scribe` conversation hook | native | omitted, hook listed unrun | delivered |
| `settings` | `settings`, 16,364 bytes | `sandbox-mode` `workspace-write`, `developer-instructions` 6,828 bytes | same, `developer-instructions` 7,051 bytes |

The verb exited 0 with `missing: []`: the Codex manifest holds every item of Claude's.
The only difference the verb reported is the unrun plugin hook, which the real launch record shows delivered.
Neither record's `verdict` is set.

## Result

Codex finished the work in one session, its tree passes the gate, and the fix is the landed fix.
It also rewrote the comments of 20 files the entry never touches; the cause is the bench's setup, not the writer (see Diff).

| | Landed Claude writer | Codex `astra[high]` |
|---|---|---|
| Diff | 4 files, +6 −7 | 24 files, +68 −48; 20 files are comment rewrites only |
| `template.yaml`, `config_test.go` | landed | byte-identical to landed |
| `doc.go`, `docs/overview.md` | landed | same fix, other wording; `overview.md` gains four preview-troubleshooting lines |
| Commits | one landing commit | 3 (`4cc19245`, `7d474afa`, `3217bd79`) |
| Verify gate | 0 attempts, one pass, 2 min 17 s | 1 attempt: gate 5 min 41 s, fix turn 2 min 49 s, gate 3 min 4 s |
| Session wall time | 5 min 41 s, gate included | 13 min 5 s from the first prompt, gate included |
| `Darn-Write` row | 6 min 17 s | 16 min 57 s |
| Fresh input, cache read, output tokens | 52 (+ 75,069 cache write), 1,474,055, 12,637 | 258,983, 4,848,640, 13,638 (main plus six subagents) |
| Cost | $0.9231 (`claude-opus-5-5`) | unknown: `codexengine` has no pricer |

Codex's fresh input is `input_tokens` less `cached_input_tokens`, summed over the rollouts; it reports no cache-write count.
The main session alone: 130,295 fresh, 4,183,552 cached, 8,801 output, 36 tool calls, 4 turns.
The turns were the skill load (14 s), the prompt (10 s, see Faults), the orch's go-ahead (4 min 3 s) and the gate-findings re-prompt (2 min 49 s).
The landed run recorded no turn or call counts.
Run cost beyond the writer is not in the table: the cold gate fixer (Claude) ran at 18:13Z and its usage was not read.

## Diff against the landed one

- **Scope, the fix.**
  The three files that carry the fix match the landed change in substance.
  `template.yaml` and `config_test.go` are byte-identical (same blob hashes).
  The `doc.go` sentence reads "The template opens no files for any place. Add entries to opt in, such as docs/{beam}.md for a beam's own file." and is accurate.
- **Scope, the extra documentation.**
  Codex added four lines to `docs/overview.md`: the preview needs the built-in Markdown Language Features extension, how to end an Extension Bisect, how to reopen a tab as preview.
  The record's idea says to settle whether lym's settings are at fault; the landed writer left it open and listed a hand check, Codex concluded from upstream docs that they are not and documented recovery.
  The menu names (`Help: Stop Extension Bisect`, `View: Reopen Editor With...`) were not audited against the installed VS Code.
- **Scope, the 20 comment rewrites.**
  The verify gate's comment lint scopes to the diff from the merge base with the pair's parent branch.
  The bench pair's parent is the T15 task branch at `3bbc92a0`, ahead of `055ef994`; the forward commit `8df0f732` that restored the base tree made the lint see 1,418 changed files.
  The gate listed comments in 20 files outside the entry as fixed-column wraps; Codex fixed them with four subagents, at one gate round's cost.
  A real darn run forks from its parent's tip and would not have this diff.
  Every hunk is a comment line: the diff changes no code or assertion.
- **Correctness of the rewrites.**
  Most are clean one-sentence-per-line splits.
  A few read worse than the original: `TestRegisteredBattenAlive_ReadsADobbyDrivenRun` now says "It does so only while a dobby of the run's parent stands", whose subject is lost; `reviewTable` gained "The seat is the reviewer as the chair".
  `ErrFingerprintMismatch` lost its note that `BeginBatch` returns it.
- **Change description.**
  It passed the run's description gate.
  It narrates verification ("`go build ./internal/ideconfig`, `go vet ...` ... passed", "The `darn-write` gate reported tier0, tier1, and tier2 passing"), which the description spec says to leave out, and links an upstream VS Code file on `main`, a perishable pointer.
  It has no `Design decisions` or `To check by hand` section, which the landed one has; it states the preview failure "was not reproduced" in prose instead.
- **Review.**
  The landed run's code review returned `APPROVED` with no finding.
  This bench stopped before `Code-Review` and ran none on the Codex diff, so it claims nothing about that outcome.

## Gate

`lym gate test` on the touched packages (`internal/ideconfig` and the ten comment-only ones) answered `carried`: this exact tree had already passed each tier, so the call ran no test.
The runs that passed on it are the writer's last verify gate, over the parent-diff scope:

| Tier | Discovered | Passed | Failed | Skipped | Wall |
|---|---|---|---|---|---|
| 0 | 28,784 | 27,777 | 0 | 1,007 | 46.9 s |
| 1 | 6,725 | 4,292 | 0 | 2,433 | 68.8 s |
| 2 | 959 | 280 | 0 | 679 | 46.5 s |

The `drift` and `description` gate entries also passed.
The landed run's gate passed once, with no re-prompt.

## Faults on the way

- **Skill load.**
  None.
  The first turn read the five skill files and answered `ok scribe:prose, ...` in 14 s, as the launch's loader instruction asks.
  The `scribe` plugin context shows delivered in the launch record.
- **Idle wake.**
  None in the run that counts: each of its four turns began from a delivered message or a gate re-prompt, and no extra reminder text appears in its rollout.
  The first Codex session, started on the wrong tree, went `idle-stalled` twice (17:33:37Z, 17:34:21Z) and was ended; its shuttle history is the only idle record read, and `codex-idle-wake-extra-reminder` is not shown by it either way.
- **Prompt noise from the setup, which the Claude writer would also have met.**
  The prompt names `_lym/drift/expected.yaml`, which this bench never sealed, so the session reported the file absent and carried on.
  The orch's earlier "stop and wait" line was carried into the fresh session's prompt; its second turn ended "Stopped. No edits made", and the orch's go-ahead started the work.
  The 10 s turn and the extra message are the only cost; the prompt otherwise ran as rendered.
- **Subagents.**
  The launch carries a `multi_agent_mode` line telling the session not to spawn subagents unless asked, and the prompt asks it to.
  It spawned six: two lookups, then four comment fixers.
- **Instruction drift.**
  It read files with `cat` and `rg`, which the shell guard did not refuse, and ran `lym board get` from Step 3's checks.
  It ran no `lym gate test`, as Step 4 tells it.

## Setup and deviations

- The bench pair `cwb-codex` was made by the orch from this task's worktree; the run's driver was `go`, recipe `darn`, profile `docs` in `usage/run.json`, writer `astra[high]` through the pair's local `darn.writer`.
- The board has no entry for the pair's slug, so `lym loom start` first seeded `tapestry` and ran its `Discussion-Write` before a Discussion agent stopped; that run was cleared, the record restored byte-identical to the landed run's record, and the run re-seeded with `lym shed seed self --recipe darn`.
  Its two small Claude `tackler` sessions cost $0.0103 in all.
  The record is the landed run's, so it carries the task's body as it stood then; the board entry itself is gone.
- The first reset of the pair to `055ef994` was undone by the run's sync, which took origin's `3bbc92a0`; the first Codex session therefore read the wrong tree and wrote nothing.
  The branch was not rewritten: `8df0f732` is a forward commit whose tree equals `055ef994`, and the run restarted from `Darn-Write` with a fresh session.
- The pause `--before Drift-Verify` stopped the run after the writer; `Describe` and `Finalize` never ran and nothing was pushed.
- Landed-run figures come from the archive tag `archive/ide-beam-readme-preview/afd97069da36` in the weft repository.
  Codex's come from the rollouts under `~/.codex/sessions/2026/11/17/` that begin at 19:09:10 to 19:20:51 local time.
- Production code and Bolt are untouched.

## Limits

One run on one small task: it shows the writer finishing a narrow change on the real producer path, and neither a rate nor how Codex fares on a change that spans packages.
The extra comment work comes from the bench's parent branch, so the table's diff size and gate scope say little about Codex itself; the fix's own files are the comparison.
The Claude run had no gate re-prompt; whether Codex alone would have triggered one is not shown.
