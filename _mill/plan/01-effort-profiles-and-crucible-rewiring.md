# Batch: effort-profiles-and-crucible-rewiring

```yaml
task: Crucible review spawn as effort-selectable Agent profiles
batch: effort-profiles-and-crucible-rewiring
number: 1
cards: 4
verify: null
depends-on: []
```

## Batch Scope

This batch delivers the whole task: the five `.claude/agents/crucible-reviewer-<effort>.md` profiles that make reasoning effort selectable per crucible round, plus the three `crucible/*.md` documents that tell an operator how to use them. It is one batch because it is a single documentation-and-prompt-text change with no runnable surface — card 1 creates the files the other three cards' text refers to by name, and cards 2–4 share almost all of their `Context:` with card 1, so splitting would force the same three crucible files to be re-read in a second session for no isolation benefit. There is no external interface for a later batch to consume; this is the only batch.

Batch-local discipline beyond `## Shared Decisions` in the overview: **the five bodies must stay byte-identical except `name:`, the effort word in `description:`, `effort:`, and the H1 heading.** Nothing enforces this automatically. Write card 1's five files from one template string with three substitutions rather than editing each by hand, and if `crucible/review-prompt-template.md`'s contract wording ever changes, update all five bodies in the same commit.

## Cards

### Card 1: Create the five effort-tiered crucible reviewer profiles

- **Context:**
  - `_mill/discussion.md`
  - `crucible/review-prompt-template.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `.claude/agents/crucible-reviewer-low.md`
  - `.claude/agents/crucible-reviewer-medium.md`
  - `.claude/agents/crucible-reviewer-high.md`
  - `.claude/agents/crucible-reviewer-xhigh.md`
  - `.claude/agents/crucible-reviewer-max.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the `.claude/` and `.claude/agents/` directories (neither exists in this repo yet; `.gitignore` was verified not to exclude either, so all five files are committed as normal tracked files with no `.gitignore` change). Write all five files from the single template body recorded verbatim in `_mill/discussion.md` under "**Draft body for the five new files**" — copy that block as the authoritative source rather than paraphrasing it. Substitute `<effort>` with `low`, `medium`, `high`, `xhigh`, `max` respectively in exactly three places per file: the `name:` value (`crucible-reviewer-<effort>`), the effort word inside `description:` ("at `<effort>` reasoning effort"), and the `effort:` value. The H1 heading likewise becomes `# crucible-reviewer-<effort>`. Every other byte is identical across the five. Frontmatter carries exactly three keys — `name`, `description`, `effort` — with **no** `model:` key and **no** `tools:` key (see the overview's frontmatter Decision). Each `description:` must retain the auto-delegation qualifier ("Only invoked by name via subagent_type (the crucible orchestrator) — not a candidate for automatic delegation"), because `description:` is what Claude Code matches on for automatic delegation and five near-identical crucible descriptions could otherwise be picked up for unrelated main-thread work. The body's three bullets must each survive intact: the clean-room "form your own findings first" bullet, the "Commit per fix, never push" bullet **including its explicit "this is a host-repo commit on the crucible worktree, never a weft-repo operation" clause** (this clause is what satisfies `CONSTRAINTS.md`'s Weft Git Invariant in the shipped text — do not drop or soften it), and the "final chat message is an executive summary only" bullet. The closing paragraph must retain both the "no `model:` / no `tools:` — effort is the only thing this profile changes" statement and the V0-stopgap sentence naming Hardener and lyx's `modelspec` as the obsolescence trigger, so a future reader knows these files are disposable. Write every paragraph and bullet as one continuous line — no hard-wrapping.
- **Commit:** `feat(crucible): add five effort-tiered crucible-reviewer agent profiles`

### Card 2: Rewire orchestrator-prompt.md to spawn via the effort-tiered profiles

- **Context:**
  - `_mill/discussion.md`
  - `crucible/README.md`
- **Edits:**
  - `crucible/orchestrator-prompt.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make five edits, all preserving the file's existing tone and one-line-per-paragraph formatting.
  1. **Opening line (~7).** "You are the **orchestrator** of a serial, model-rotating **review+fix loop**" → name both axes, e.g. "a serial, model- and effort-rotating **review+fix loop**", so the document's own framing sentence is not left single-axis.
  2. **Hard rule 2 (~18).** Replace "Spawn `subagent_type: general-purpose` with a `model:` override." with the tiered form: spawn `subagent_type: crucible-reviewer-<effort>` (the operator's pick this round) with a `model:` override that is also the operator's pick and independent of effort. Keep the existing "A fork would inherit *your* context and destroy the clean-room independence" sentence. Add, in the same rule, the hard requirement that the orchestrator must obtain an **explicit** effort-tier pick from the operator before spawning any round — if the operator names only a model ("next round, Opus"), the orchestrator asks for the missing effort pick; it never defaults to a tier and never falls back to `general-purpose`. Also add, adjacent to that hard-rule statement, the pre-merge recovery path: in a worktree branched before these profiles merged to `main`, the `crucible-reviewer-<effort>` profile does not exist yet and the spawn will not resolve — sync the worktree (`mill-merge-in` or equivalent) to pull the profiles in, then retry. State this as a required remediation step before the round can proceed, explicitly *not* a licence to fall back to `general-purpose`.
  3. **Loop step 2 "Spawn" (~30).** Change `` `Agent` tool → `subagent_type: general-purpose`, `model: <the operator's pick this round>` `` to `` `subagent_type: crucible-reviewer-<effort>` `` (the operator's pick this round) alongside the unchanged `model:` pick. In the same sentence, change the round tag from `` `<model>-r<N>` `` to `` `<model>-<effort>-r<N>` `` and give a concrete example (`opus-high-r3`). Leave the rest of the step — commit-each-fix-as-it-lands, never push, executive-summary-only reply — untouched.
  4. **Loop step 5 "Decide" (~34).** "Re-seed the prompt (step 1) with the new finding, and spawn the next full round with a **different** model" → also name effort, e.g. "with a **different** model and/or effort tier", since rule 2 now requires an explicit effort pick on every spawn rather than just a rotated model.
  5. **Section rename (~55).** Rename the `## Model rotation` heading to `## Model + effort selection` and rewrite its one-paragraph body to: keep the existing model-rotation rationale verbatim in substance (rotate across Opus / Fable / Sonnet; different models miss different things; convergence across different models beats N passes from one; use the more capable model for the final safety pass); add the effort rationale (a cheap low-effort wide sweep early, a max-effort correctness pass for the final safety round); state that the operator picks both axes independently per round; and **explicitly enumerate the shipped tier names** — `low`, `medium`, `high`, `xhigh`, `max` — with a pointer to `.claude/agents/crucible-reviewer-<effort>.md`. This enumeration is the single place an operator learns what is pickable, so if a tier is ever dropped it must be removed from this list in the same commit that deletes the file.
- **Commit:** `docs(crucible): spawn rounds via effort-tiered subagent_type in orchestrator prompt`

### Card 3: Update crucible/README.md for the effort axis

- **Context:**
  - `_mill/discussion.md`
  - `crucible/orchestrator-prompt.md`
- **Edits:**
  - `crucible/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make eight edits.
  1. **Orchestrator role bullet (~21).** "seeds the prompt, spawns each round, **independently verifies** the round's work, re-seeds, rotates the model, and decides when it has converged" → name effort alongside model in the rotation clause.
  2. **Round agent bullet (~22).** "a fresh, **clean-room** sub-agent spawned per round (a `general-purpose` Agent, *not* a fork — a fork would inherit the orchestrator's context and destroy independence)" → describe the effort-tiered `crucible-reviewer-<effort>` Agent instead of `general-purpose`, keeping the "*not* a fork" independence point and its parenthetical rationale fully intact.
  3. **ASCII loop box (~27–36).** Replace the box's content lines with these eight, in order, and pad every line to one uniform inner width so the right `│` border forms a straight rectangle (the current box is ragged; the replacement should not be). Keep the `┌`/`└ ... until converged ... ┘` frame style and the existing left indentation.
     - `1. SEED the prompt with the current known state`
     - `2. SPAWN a fresh clean-room round agent (rotate model + effort)`
     - `      A — review (independent findings, drive real substrate)`
     - `      B — fix (implement + test + docs, commit each fix as it lands)`
     - `3. ORCHESTRATOR independently VERIFIES (never trust the`
     - `      round's own "merge-ready" verdict)`
     - `4. RE-SEED with what verification found; go to 2 with the`
     - `      next model + effort tier`
     This is four changes in one box: the two rotation lines gain the effort axis, and the two commit lines — currently "B — fix (implement + test + docs, do NOT commit)" and "4. COMMIT the round's work" — are corrected to the commit-per-fix discipline. Those two are a **pre-existing contradiction** with the rest of this same file (see its "Why commit per fix, not one commit for the whole round" section), with `crucible/review-prompt-template.md`'s "Commit per fix (BLOCKING)" section, and with the new agent bodies from card 1 — and the new bodies point round agents straight at this README, so leaving them would actively mislead the very agents this task creates. Fix them here.
  4. **"The loop" step 2 (~40).** "One fresh `general-purpose` Agent with a `model:` override" → describe the effort-tiered `subagent_type: crucible-reviewer-<effort>` spawn alongside the still-present `model:` override, and change the tag in the same sentence from `` `<model>-r<N>` `` to `` `<model>-<effort>-r<N>` ``. The two `.scratch/` deliverable filenames in that step derive mechanically from the tag and need no separate edit.
  5. **"Re-seed + rotate" step 4 (~42).** "Spawn the next round with a **different** model." → name effort alongside model, matching the loop-box wording.
  6. **New effort paragraph (~48–50).** Add a short subsection or paragraph adjacent to the existing `### Why rotate the model` section explaining the effort axis and how it composes with model rotation, cross-referencing `orchestrator-prompt.md`'s renamed "Model + effort selection" section (including its tier enumeration) rather than duplicating the rationale or the tier list. Leave the existing "Why rotate the model" heading and its paragraph as they are — they explain *model* diversity specifically and are correct as written.
  7. **"Instantiating this for a new module" step 3 (~106).** "Run the loop: seed → spawn (rotate model) → independently verify → re-seed → repeat" → name effort in the spawn step.
  8. **Campaign table (~112–118).** Add an `Effort` column between `Model` and `What it closed`, so the header becomes `| Round | Model | Effort | What it closed |` with a matching separator row. Every existing row (R3–R7) is historical and predates effort tiers: write the literal `n/a` in each of their Effort cells — never blank (a blank reads as an oversight rather than a deliberate non-applicability) and never a fabricated effort value. Leave the surrounding prose at ~110 and ~120 describing that campaign untouched; it is deliberately model-only because that campaign was.
- **Commit:** `docs(crucible): document the effort axis and fix the loop box's commit lines`

### Card 4: Amend the review-prompt-template's "entire instruction set" claim

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `crucible/review-prompt-template.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Amend the opening blockquote's claim at line 3 — "It is the round agent's *entire* instruction set" — which stops being literally true once the `crucible-reviewer-<effort>` agent-file preamble also carries clean-room / commit-per-fix / summary-only contract text. Reword **additively**, along the lines of: it is the round agent's instruction set for the review+fix work itself; the `crucible-reviewer-<effort>` agent-file preamble under `.claude/agents/` also carries the clean-room / commit-per-fix / summary-only contract, but this file remains the authoritative statement of it. This is a one-line change and is explicitly **not** a reason to trim anything: the `## Commit per fix (BLOCKING ...)`, `## Sequencing rule (BLOCKING ...)`, and `## Clean-room review constraint` sections all stay exactly as they are. The agent-file preamble is a convenience restatement, and for any worktree that has not yet merged the new profiles this template is still the sole carrier of that contract. Change nothing else in this file.
- **Commit:** `docs(crucible): note the agent-file preamble alongside the review prompt template`

## Batch Tests

`verify: null` — this is a pure documentation and prompt-text batch. It creates five Markdown agent definitions and edits three Markdown documents; it touches no Go code, no build tags, and no test files, so there is no runnable surface for `go test` or `go vet` to exercise. loomyard also has no test harness for `.claude/agents/*.md` content (millhouse's `test-agents-defs.py` is specific to millhouse's plugin packaging and has no counterpart here). Running the repo suite would compile and test unrelated packages with zero chance of detecting a defect in this batch.

Verification is four manual checks, the first, third, and fourth performed by the implementer before the final commit, and the second deferred to the operator:

1. **Frontmatter sanity (static, implementer).** Each of the five files parses as valid YAML frontmatter carrying exactly `name`, `description`, and `effort` — no `model` key, no `tools` key — and each `effort` value is one of `low`, `medium`, `high`, `xhigh`, `max` with no typo and matching its own filename. Diff the five bodies pairwise to confirm they differ only in the three substituted tokens and the H1 heading.

2. **Live smoke check (operator, post-commit — see the overview's Decision on who runs it).** One throwaway `Agent` call per new `subagent_type`, confirming Claude Code accepts the frontmatter and the agent starts. **Precondition:** run from a session started *after* the five files exist on disk, since it is unknown whether a running session picks up newly written agent definitions. **Side-effect containment is required** — these profiles carry the full default tool set and a body instructing the agent to review, then fix and commit, so a throwaway spawn must be explicitly fenced. Use a prompt such as: *"This is a startup smoke test only. Reply with the single word OK and do nothing else — read no files, edit no files, run no commands, make no commits."* After all five spawns, confirm `git status --short` is clean and `git log` shows no new commits; a throwaway spawn that modified anything is itself a failure worth reporting, independent of whether it started cleanly. **Three outcomes:** **PASS** — the agent starts and produces *any* reply with no frontmatter or tool-load error (the body tells it to read a review prompt named in "your brief", which this prompt does not supply, so a clarifying question instead of literally "OK" is still a PASS; the criterion is spawn success, independent of reply content). **INCONCLUSIVE** — the call fails with an unknown or unrecognized `subagent_type`, which most likely means session staleness rather than bad frontmatter; re-run from a fresh session and never escalate on this outcome. **FAIL** — the profile is recognized but the agent fails to start, a genuine frontmatter or tool-load error such as an unsupported `effort:` value. Only FAIL acts: drop the tier (for `low` or `xhigh`) in a small follow-up commit that also removes it from the `orchestrator-prompt.md` enumeration, or halt and report to the operator (for `medium`, `high`, or `max`, whose failure would suggest the `effort:` key itself is no longer honored — a problem with the whole mechanism, not one unverified edge case). **Known limitation:** this proves only that the frontmatter parses and the agent starts — it does **not** prove the `effort:` value was honored rather than silently ignored. No observable signal distinguishes the two. Accepted as a V0 limitation. The implementer's handoff must state that this check is pending and name all five `subagent_type` values.

3. **Doc-consistency read-through (implementer).** Re-read `crucible/orchestrator-prompt.md` and `crucible/README.md` end to end after the edits, then grep `crucible/` for the bare token `general-purpose`. This **cannot** return zero hits, because rule 2 is required to state that the orchestrator never falls back to `general-purpose`. Treat it as an expected-hit inventory instead: after the edits the token should appear only in one or two deliberate places, both inside `orchestrator-prompt.md` rule 2 — the never-fall-back statement and the pre-merge recovery-path text, which naturally names the forbidden fallback. Any hit outside rule 2 — for instance a description still presenting "a `general-purpose` Agent" as the current spawn mechanism — is the real stale-reference signal and must be fixed.

4. **Round-tag and tier-enumeration consistency (implementer).** Confirm the renamed "Model + effort selection" section lists all five tier names, and that every example round tag in both edited files uses the `<model>-<effort>-r<N>` form with no leftover `<model>-r<N>`. Then grep `crucible/` for `rotate` and `next model` as a second expected-hit inventory. Legitimate remaining hits: `README.md`'s `### Why rotate the model` heading and its paragraph (~48, deliberately kept — it explains model diversity specifically, and the new adjacent paragraph covers the effort axis), and the historical reed-campaign prose (~110 and ~120), which describes a campaign that predates effort tiers and must not be retroactively reworded. Any other line still framing round-to-round rotation as model-only is stale and must be updated.
