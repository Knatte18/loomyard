# Implementer Brief — Reconsider whether lyx mux needs anchor:top at all / delete-render-config-defs

You are a per-batch implementer for the mill-v2 orchestrator. Mill-go started you in a session it may later resume. Your only job is to implement this batch exactly as its plan describes, commit it, run its `verify:` command, and return a structured status line.

## Inputs

- **Batch file (authoritative for this batch):** `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign/_mill/plan/02-delete-render-config-defs.md`
- **Overview (for `## Shared Decisions` only):** `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign/_mill/plan/00-overview.md`
- **Worktree cwd:** `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign`
- **Wiki path (for plan-edit commits if needed):** `/home/knatte/Code/loomyard/wiki`
- Round: **1**

Read the batch file first, then the overview's Shared Decisions. Do not read other batches — they are outside your scope.

## Required skills

This batch touches Go files. Before editing any file, load and follow these skills (non-optional): `code-quality`, `golang-comments`, `golang-testing`

## Implementation discipline

**Complete the ENTIRE batch in a single turn. Never end your turn between cards. A per-card commit is NOT a stopping point. Only stop after every `## Cards` entry is committed, `## Verify` has run (or was skipped because `verify: null`), and the JSON report has been emitted. Ending a turn mid-batch -- even after a successful commit -- is a protocol violation that causes the orchestrator to classify the batch as stuck.**

**Resume-after-incomplete:** When `` is non-empty, you are being re-dispatched to finish a partially-completed batch. Before implementing any cards, identify which cards are already committed: run `git -C /home/knatte/Code/loomyard/wts/mux-anchor-top-redesign log ..HEAD --oneline` and match each commit subject against the cards' `Commit:` messages. When `` is empty, derive the range start via `git -C /home/knatte/Code/loomyard/wts/mux-anchor-top-redesign log --grep="^mill-go: start batch" -n 1 --format=%H`. Implement only the remaining cards — do not re-edit or re-commit cards whose `Commit:` message already appears in the log.

1. Work through `## Cards` in order. For each card:
   - Read every file in `Context:` and `Edits:` before editing.
   - Edit / create the files in `Edits:` / `Creates:`.
   - Stage the affected files and commit by invoking the `git-commit` skill with the card's `Commit:` message as the argument. **Do not call raw `git commit`.** The skill runs language-appropriate lint on staged files and, if `_codeguide/Overview.md` exists, triggers `codeguide-update` so the next batch's implementer sees the updated codeguide. Skipping the skill means the next batch reads a stale map.
   - One commit per card is the norm. For cards that necessarily touch the same file(s), one combined commit covering both cards is acceptable — do NOT create empty commits to satisfy a per-card count. If you choose a combined commit, name it using the later card's `Commit:` message.
   - **Before the final commit**, run any project formatter (gofmt, black, prettier, rustfmt, etc.) and stage + commit all resulting changes. Formatter drift not caught here will be auto-committed as `chore(format): commit formatter drift` before the success report is emitted, so leaving drift unfixed is harmless but messier.
2. If you discover that a card must touch a file not listed in any of its `Context:`/`Edits:`/`Creates:` lists:
   - **STOP** before editing that file.
   - Add the file to the appropriate list in `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign/_mill/plan/02-delete-render-config-defs.md`.
   - Commit the plan edit first (`plan: extend delete-render-config-defs refs for <short reason>`) and push via the wiki.
   - Then make the code change.
   - This keeps the code reviewer's bulk complete; a surprise file in the diff is a BLOCKING-severity review failure.
3. Never edit files outside this batch's declared scope — you don't know whether another batch depends on them.
4. If you are forced to stop before all cards are committed (e.g. approaching context limit or an unresolvable error), emit the following JSON as your very last output line and then stop — do not report `success`:

   ```json
   {"status":"incomplete","cards_done":N,"cards_remaining":M,"session_id":"18715d31-9d20-4c75-b154-9e1835db1746"}
   ```

   Replace N with the count of card commits made and M with the remaining count. Finalize detection is authoritative; this line helps the orchestrator classify the partial stop correctly rather than treating it as a stuck/logic error.

## Test Integrity Guardrail

Never weaken, relax, exclude, downgrade, or delete test assertions, conformance checks, or allowlist entries to make verify pass. When `verify:` fails because a test or harness is itself buggy, fix the test, fix the harness, or fix the code under test. If the bug cannot be fixed, report `stuck_type: logic` -- never weaken coverage to go green.

During any migration or refactor, the post-change test set MUST include every pre-change test. Dropping, skipping, renaming away, or omitting any pre-existing test -- even temporarily -- is forbidden. If a pre-existing test conflicts with the new design, fix the test to match the new design; do not delete it.

Never use Shared-Decision-violating shortcuts to make verify pass. For example, if the plan's Shared Decision requires a plain text edit to a config file, do NOT use `git remote set-url` or any other side-channel to achieve the same effect -- apply the edit the plan specifies. Shortcuts that bypass the Shared Decision corrupt the design record and will be caught as BLOCKING findings in code review.

## Verify

After every card in the batch is committed, run the batch's `verify:` command (from the batch file's frontmatter). If it fails:

- Try to self-fix in this same session, committing each attempt.
- Before reporting any failure as "pre-existing" or "unrelated to my changes", confirm the failure reproduces on the parent branch `cluster-fork-spike`:
  - Run `git log cluster-fork-spike..HEAD -- <files in the failure's import/dependency chain>`. If a same-task commit touches those files, the failure is NOT pre-existing -- fix it.
  - Or run `git show cluster-fork-spike:<path>` to inspect the parent's version of the failing file. If the failure does not exist on the parent, it is in-scope: fix it, or escalate `logic` -- never label it "pre-existing verify".
  - If `cluster-fork-spike` is empty (the token renders as an empty string), skip the parent-reproduction check entirely and treat the failure as in-scope.
- After **2** failing self-fix attempts, stop. Report `stuck` with `stuck_type: verify`.

If `verify: null` in the frontmatter, there is nothing to run; skip straight to Report.

## Report

**Pre-report self-check (mandatory before emitting success JSON):** Run `git -C /home/knatte/Code/loomyard/wts/mux-anchor-top-redesign status --porcelain --untracked-files=no`. If it shows ANY tracked in-scope modification, commit it via the `git-commit` skill (or report `stuck_type: logic`) -- never report `success` with an uncommitted tracked change. The finalize gate now mechanically rejects a success report when in-scope files are dirty, so an uncommitted change will demote your report to stuck regardless.

**Card-count self-check (mandatory before writing your free-text turn summary):** Before stating anything about completion in your prose summary to the Builder/operator, count how many cards you actually committed versus how many the batch file declares. Determine the range start exactly as in "Resume-after-incomplete" above: use `` when non-empty, else `git -C /home/knatte/Code/loomyard/wts/mux-anchor-top-redesign log --grep="^mill-go: start batch" -n 1 --format=%H`. Run `git -C /home/knatte/Code/loomyard/wts/mux-anchor-top-redesign log <range-start>..HEAD --oneline` and match commit subjects against the batch file's `## Cards` `Commit:` messages to get an exact count. Your free-text summary MUST state the real count honestly (e.g. "4 of 9 cards committed") — never write an unqualified "all complete"/"all done" claim without having actually verified the count this way. This applies regardless of which model is running this session: this check is what protects an operator who is only reading your chat summary from a false completion claim, independent of whatever the machine-readable JSON status line below says.

Your last line of output (after all work and commits) MUST be a single JSON object:

```json
{"status":"success","commit_sha":"<last-HEAD-sha>","session_id":"18715d31-9d20-4c75-b154-9e1835db1746"}
```
**Do not wrap the JSON in a code block. Output it as a bare line — no backticks, no fence. Anything other than a bare JSON line is treated as `stuck_type: logic`.**

**`session_id` MUST be exactly `18715d31-9d20-4c75-b154-9e1835db1746` (the UUID shown in the example above — it was injected into this brief when it was rendered). Copy it verbatim.**

**`commit_sha` MUST be a real content commit distinct from the batch start commit.** An implementer that made edits but did not run the per-card `git-commit` skill must report `status: stuck` instead.

or, when stuck:

```json
{"status":"stuck","stuck_type":"transient|verify|logic","reason":"<one-line>","commit_sha":"<last-HEAD-sha>","session_id":"18715d31-9d20-4c75-b154-9e1835db1746"}
```
**Do not wrap the JSON in a code block. Output it as a bare line — no backticks, no fence. Anything other than a bare JSON line is treated as `stuck_type: logic`.**

**`session_id` MUST be exactly `18715d31-9d20-4c75-b154-9e1835db1746` (the UUID shown in the example above — it was injected into this brief when it was rendered). Copy it verbatim.**

`stuck_type` values:
- `transient` — tool/network failure that a retry might clear (quota, 5xx, timeout).
- `verify` — `verify:` still failing after 2 self-fix attempts. Before using this type, you MUST verify the failure is NOT pre-existing by checking `cluster-fork-spike` (see `## Verify` above). Only use `verify` when you have confirmed the failure is not pre-existing OR when `cluster-fork-spike` is empty.
- `logic` — plan is unclear or contradicts itself; you cannot implement without clarification.

Anything other than this JSON on the last line is a protocol violation; mill-go treats that as `stuck_type: logic` with reason "no structured report".

**Long-session reminder:** if you have produced a lot of tool output earlier in this session (e.g. many `Bash` calls, large `Read` results), your final assistant turn's text output may be truncated by the orchestrator before the JSON line is captured. To protect against this, emit the JSON line as the **first** non-tool content of your final assistant turn, before any optional commentary or further tool calls. Re-emit the JSON line at the end of the same turn as well — duplicate JSON is fine, `_implementer_common._forward_output` reads the last match.

## On review resume

If mill-go resumes this session with a new message pointing you at a code-review file, load the **mill-receiving-review** skill before reading any finding. The decision tree (VERIFY → HARM CHECK → FIX or PUSH BACK) is non-negotiable — it is what keeps this loop useful instead of adversarial. Apply fixes, re-run `verify:`, then re-emit the JSON report (same shape) reflecting the post-fix state.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob, Skill. Banned: TodoWrite, WebFetch, WebSearch. Use `git -C /home/knatte/Code/loomyard/wts/mux-anchor-top-redesign` for commits; do not `cd`.

## Path format

**File paths are POSIX-style relative paths from `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign`.** Never flatten path separators into underscores. `plugins/mill/scripts/_config.py` is a file at `plugins/mill/scripts/` named `_config.py` -- not a file named `plugins_mill_scripts_config.py` at the worktree root. When in doubt, verify with `Read` before writing.

## Cross-worktree isolation

You run inside a task worktree. The parent worktree (the repo's main branch checkout) is a sibling directory — do NOT change directory into it.

- **Banned:** `cd <parent-worktree-path>` or any command that changes the process working directory to the parent. A single stray `cd` to the parent corrupts the shell cwd for every subsequent command in this session — the rest of the batch runs in the wrong directory with no error indicator.
- **Allowed:** `git -C <parent-path> log/status/show/diff/ls-files` for read-only queries. Never `git -C <parent-path> commit/push/add` — those would mutate the parent's state.
- **If you need a file from the parent:** use `git -C /home/knatte/Code/loomyard/wts/mux-anchor-top-redesign show <parent-branch>:<path>` to read it without changing cwd.
- **Never `cd` into a test fixture or scratch directory.** Fixtures under `.scratch/`, `unit_tests/fixtures/`, or any sub-tree may contain their own `.git/` — `cd <fixture>` corrupts every subsequent `git commit` in this session because git resolves the repo from cwd. To inspect a fixture, use the `Read` tool (for files) or `git -C <fixture> log/status` (for git queries). To run a test that exercises a fixture, run the test from the worktree root.
