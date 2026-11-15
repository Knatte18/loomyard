# Codex `astra[high]` as the darn writer, replayed once on `ide-beam-readme-preview` (Neve)

Bench of 2026-11-15, level 2 of `codex-support-levels` (R2): the darn writer's work only, one Codex session, nothing landed or pushed.
Input: the darn light task `ide-beam-readme-preview`, landed as `824eb2b6`; its Claude writer's work is the answer key.
The verdict on level 2 is the operator's; this report states what the one run showed.

## Result

Codex finished the work and its tree passes the gate, but it did more than the entry asked.

- The three files that carry the fix (`internal/ideconfig/template.yaml`, `config_test.go`, `doc.go`) match the landed change, bar one wording choice in `doc.go`.
  `template.yaml` and `config_test.go` are byte-identical to the landed ones.
- It added a new page, `docs/ide-markdown-preview.md` (38 lines), and a pointer to it in `docs/overview.md`.
  The landed writer changed neither the preview nor the docs about it, and recorded that the preview failure was left undiagnosed.
- Its change description passes `lym loom validate-description`, but names no design decisions and no hand check, which the landed one does.
- One session, one commit pair, no re-prompt: about 4 min of wall time against 5 min 41 s for the landed writer, whose figure includes a 2 min 17 s verify gate.
  Codex's dollar cost is unknown (`codexengine` implements no pricer); its tokens are below.

| | Landed Claude writer | Codex `astra[high]` |
|---|---|---|
| Diff | 4 files, +6 −7 | 5 files, +45 −6 |
| Files beyond the landed four | none | `docs/ide-markdown-preview.md` (new) |
| Commits | one landing commit | 2 on the scratch branch |
| Gate on `./internal/ideconfig` | passed in the run's verify gate | tier 0: 7 targets passed; tiers 1 and 2: no target |
| `validate-description` | clean | clean |
| Wall time | 5 min 41 s, 2 min 17 s of it the verify gate | about 4 min 8 s, no gate |
| Model calls | not recorded | 30 in the main session, 2 subagent sessions beside it |
| Fresh input, cache read, output tokens | 52 (+ 75,069 cache write), 1,474,055, 12,637 | 206,791, 3,718,528, 9,511 (main plus both subagents) |
| Cost | $0.9231 (`claude-opus-5-5`) | unknown |

Codex's input is the rollout's `input_tokens` less `cached_input_tokens`; it reports no cache-write count.
The main session alone: 95,796 fresh, 2,128,640 cached, 5,954 output.

## What the run did

The prompt is the Darn-Write stencil as it stood at the base `055ef994`, filled as `darnengine` fills it (`data/prompt.md`).
A stencil diff from the base to this branch's head shows no change in the darn, parent, edit or pattern stencils.
The decision record and support log are the landed run's, byte-identical to its archive tag `archive/ide-beam-readme-preview/afd97069da36` in the weft repository.

The session read the prompt, the record, the specs and a dozen files, then spawned two subagents (`preview_lookup` and `beam_defaults`) through `spawn_agent`.
It edited `template.yaml`, `config_test.go`, `doc.go` and `docs/overview.md` with `apply_patch`.
It then fetched upstream VS Code sources through its web tool, wrote the new page and a longer `overview.md` sentence, committed twice and ran `lym loom validate-description`.
`data/codex.diff` is its diff, `data/landed.diff` the landed one and `data/codex-summary.md` its change description.

## Diff against the landed one

- **Scope.**
  The fix is the same.
  The extra page is the whole difference: the record's idea says to settle whether lym's settings are at fault before changing them, and offers no documentation task.
  Codex settled it by source reading and published the reasoning as a page, where the landed writer left it open and told the operator what to check by hand.
- **Correctness.**
  The page links VS Code `1.100.0` sources by line number, while the landed run's reviewer found `1.142.0` installed on the machine.
  It also states behavior of `lym ide spawn` and `lym ide open` that this report did not audit.
  The landed review checked its own premises against the installed VS Code and the stored bisect state; Codex's summary says the affected window's state was unavailable.
- **Comments and prose.**
  The `doc.go` sentence (`The template opens no files for the prime, a beam or a task;`) is accurate and in tone.
  The new page hard-wraps mid-phrase at a fixed column, which the house prose rule forbids; the landed prose uses semantic line breaks.
  The summary says "Gate tests are reserved for the handoff verifier", which describes the session's process; the description spec says to describe the change.
  It pins an upstream version and line numbers, perishable specifics the prose rule says to avoid.
- **Review rubric.**
  The landed run's code review returned `APPROVED` with no finding.
  This report ran no review of the Codex diff, so it makes no claim that the review would pass.

## Faults on the way

- **Sandbox.**
  None seen: edits, `go build`, two commits and `lym loom validate-description` worked in the scratch worktree, which lies outside the session's own worktree.
  This report did not read which sandbox mode `sandbox_mode: auto` resolved to.
- **Skill load.**
  None to fail: the prompt asks for none, and the landed writer's skills come from its system prompt, which a Codex session has no counterpart of.
  The prompt's Agent-tool and `Edit`/`Write` wording matched no Codex tool; Codex read it as `spawn_agent` and `apply_patch`.
- **Idle wake.**
  None: the session never idled, so `codex-idle-wake-extra-reminder` did not come into play.
- **Instruction drift.**
  It read files with `cat`, `sed -n` and `rg`, which the edit directive tells a session not to use; no guard refused them.
  It ran no `lym board list` or `lym loom peers`, the first check of Step 3.
  It ran no `lym gate test`, as Step 4 tells it.

## Setup and deviations

- The scratch worktree is a detached checkout of `055ef994` on a branch of its own, under this task's `.scratch/`, with a copy of the hub's `gate.yaml` so `lym gate test` runs in it.
  It was not a `lym` worktree, so no board, drift or loom config applied there.
- The prompt differs from a production render in four places: a two-line preface naming the worktree, since `lym shuttle run` ran from this task's worktree; the no-parent variant of the parent directive, since no parent was live; the scratch paths; and `ide-beam-readme-preview` as the slug.
- The run used `lym shuttle run --model 'astra[high]'` with one output file, the change description in the scratch worktree.
- Landed-run figures come from the archive tag's `_lym/usage/session-*.json` and `status.json`.
  Codex's come from its rollout under `~/.codex/sessions/2026/11/15/`, whose first line is at 18:12:44Z.
- The scratch worktree and its branch are removed; production code and Bolt are untouched.

## Limits

One run on one small task: it shows a pass on a narrow change, and neither a rate nor how Codex fares on a change that spans packages.
The landed writer ran with a live parent and a verify gate with a cold fixer after it; Codex had neither, so a gate failure would have shown nowhere.
