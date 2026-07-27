# Discussion: Crucible review spawn as effort-selectable Agent profiles

```yaml
task: Crucible review spawn as effort-selectable Agent profiles
slug: crucible-effort-agent-profiles
status: discussing
parent: main
```

## Problem

Crucible round agents (`crucible/orchestrator-prompt.md`) spawn today as `subagent_type: general-purpose` with only a per-round `model:` override (rule 2, loop step 2, and the "Model rotation" section — lines 18/30/55). The operator can choose the **model** per round but has no lever on reasoning **effort**. For a review+fix loop where depth is the entire point, effort is a valuable independent knob: a cheap low-effort wide sweep early, a max-effort correctness pass for the final safety round. The Claude Code `Agent` tool's call site has no `effort` parameter (only `model` + `subagent_type`) — the only place reasoning effort can be pinned per spawn is the `effort:` frontmatter key on a custom agent definition file (`.claude/agents/*.md`), so offering an effort choice means one file per effort level, selected via `subagent_type`.

This is explicitly a **V0 stopgap**: crucible's paste-prompt method executes in mill-spawned worktrees (not lyx/Fabric-initialized hubs), so lyx's own `modelspec` effort plumbing (`opus[effort=high]` → `shuttleengine.Spec.Effort` → `claudeengine --effort`) does not exist in this context — crucible bypasses lyx's launch entirely via Claude Code's own `Agent` tool. Once Hardener (crucible's V1, a Go engine) drives the loop inside a Fabric-initialized environment, effort comes from `modelspec` and these per-effort agent-definition files become obsolete (or at most a Claude-Code-Agent-tool fallback). Do not extend this mechanism beyond what V0 needs.

## Scope

**In:**
- Five new agent definition files, one per effort level: `.claude/agents/crucible-reviewer-low.md`, `-medium.md`, `-high.md`, `-xhigh.md`, `-max.md`.
- Rewiring `crucible/orchestrator-prompt.md` rule 2 and loop step 2 to spawn via `subagent_type: crucible-reviewer-<effort>` instead of `general-purpose`, with `model:` staying an orthogonal per-call override.
- Renaming the "Model rotation" section to "Model + effort selection", documenting that the operator picks both per round.
- Updating `crucible/README.md` to describe the effort choice and how it composes with model rotation.
- A cheap live smoke check (see Testing) confirming Claude Code accepts each new agent file's frontmatter and spawns cleanly.
- A one-line amendment to `crucible/review-prompt-template.md`'s opening line (currently: the per-module review prompt "is the round agent's *entire* instruction set") — no longer literally true once the new agent-file body also carries clean-room/commit-per-fix/summary-only contract text; see Technical context for the exact wording.

**Out:**
- No `.claude-plugin/plugin.json` registration step — loomyard is a plain repo, not a Claude Code plugin (verified: no `.claude-plugin/` directory exists in this repo, unlike millhouse). `.claude/agents/*.md` files are project-level and auto-discovered from a worktree's own checkout; nothing else needs to reference them by path — see the "Placement" decision below for the merge-visibility caveat that follows from this.
- No changes to `internal/*` Go code, `CONSTRAINTS.md`, or any module doc — this task touches only `crucible/*.md` and `.claude/agents/*.md`.
- No read-only "crucible reviewer" variant (a review-only, non-fixing profile) — out of scope; the round agent is always a reviewer-**fixer** with the full implementer tool set.
- No automated test suite analogous to millhouse's `test-agents-defs.py` — loomyard has no Python/plugin test harness for agent-definition files; verification is the live smoke check below plus proofreading.
- No mill-agents.yaml-style dispatch config or `_agent_dispatch.py`-equivalent script — crucible is a manually-run human+Claude orchestrator prompt, not an automated dispatch layer; the operator picks `subagent_type` by hand each round exactly as they already pick `model:`.

## Decisions

### Effort tiers offered: all five

- Decision: Ship all five effort levels — `low`, `medium`, `high`, `xhigh`, `max`.
- Rationale: The orchestrator's own stated use case explicitly wants both ends of the spectrum ("a cheap low-effort wide sweep early, a max-effort correctness pass for the final safety round"). Cost of shipping all five is just five near-identical files.
- Rejected: A trimmed set (e.g. `medium/high/max` only, mirroring millhouse's own precedent — see Technical context) — rejected because millhouse's precedent serves a different use case (automated per-batch implementation/review dispatch, where a `low` tier risks quality on real autonomous work); crucible's explicit intent calls for the cheap-sweep end too.
- **Caveat (unresolved by direct evidence):** the millhouse precedent files (`origin/hanf/linux-port-more`, unmerged) prove the `effort: medium|high|max` key was written and accepted as valid frontmatter — they do NOT prove a spawn was independently observed to succeed; the checked-out millhouse worktree itself has only the untiered `mill-implementer.md`/`mill-reviewer.md`. So in practice NONE of the five tiers has a confirmed live spawn yet — `medium`/`high`/`max` merely have one extra data point (a real prior author wrote and shipped that frontmatter) that `low`/`xhigh` lack. The observable failure mode of an unsupported/invalid `effort:` value (reject the file at load, error at spawn time, or silently ignore it) is unknown for all five. This task's live smoke check (see Testing) is the first real confirmation point for every tier: if `low` or `xhigh` fails to spawn cleanly, drop that tier rather than ship a broken file; if `medium`, `high`, or `max` also fails, treat it as a real blocker — halt the task and report to the operator rather than silently shipping a docs-only change, since a failure on the better-evidenced tiers suggests a deeper problem with the whole mechanism (e.g. the `effort:` key itself no longer being honored), not just an unverified edge case.

### No unsuffixed "inherit" fallback file

- Decision: Every offered tier gets its own explicit `crucible-reviewer-<effort>.md` file. No bare `crucible-reviewer.md` (session-default effort) is shipped.
- Rationale: The entire point of this task is a per-round *explicit* effort choice by the operator, mirroring how they already explicitly pick `model:` each round. An implicit "inherit" option would be an easy-to-forget silent default that undermines that discipline.
- Rejected: Also shipping a 6th unsuffixed file as a "don't care" option.

### Description states the effort level in prose

- Decision: Each tier file's `description:` frontmatter explicitly states its effort level (e.g. "...at high reasoning effort...") AND an explicit "only when invoked by name via `subagent_type` (the crucible orchestrator) — not for automatic delegation" qualifier.
- Rationale: Self-explanatory in an agent picker/listing without needing to inspect the `effort:` frontmatter key. The auto-delegation qualifier is needed because `description:` is what Claude Code matches on for automatic delegation — five near-identical crucible descriptions (the first agent files ever added to this repo's `.claude/agents/`) could otherwise get picked up for unrelated main-thread work that never intended to invoke a crucible round.
- Rejected: Byte-identical `description:` across all tiers (which is what millhouse's own precedent files do, relying on `name` + `effort:` alone to convey the tier) — rejected here specifically per operator request; the two codebases are allowed to diverge on this cosmetic point. Also rejected: omitting the auto-delegation qualifier — leaves a real, verified risk (empty `.claude/agents/` today, so nothing currently guards against it) unaddressed.

### `model:` omitted entirely from every tier file

- Decision: No tier file sets a `model:` frontmatter key (default is `inherit`).
- Rationale: Keeps effort (via the file/`subagent_type`) and model (via the orchestrator's per-call `model:` override on the same `Agent` call) fully orthogonal — no N-effort × M-model file explosion. This is also exactly what millhouse's own precedent files do. The operator picks both per round: `subagent_type: crucible-reviewer-<effort>` and `model: <pick>` on the same `Agent` call.
- Rejected: Setting `model: inherit` explicitly — same effective behavior, adds a line for no benefit.

### Verification: cheap live smoke check, not a real crucible round

- Decision: After creating the five files, spawn one throwaway `Agent` call per new `subagent_type` with a trivial no-op prompt (e.g. "Reply with the single word OK.") to confirm Claude Code accepts the file's frontmatter (including the `effort:` key) and the agent starts and replies without error. Discard the result; do not run a real review+fix round as part of verification.
- Rationale: Confirms the mechanism actually works (catches a bad effort value, YAML typo, or malformed frontmatter) at near-zero token cost, before an operator hits a broken profile mid-round.
- Rejected: No live check at all (docs-only, proofreading only) — rejected because the frontmatter `effort:` key was unverified in this repo until this task's own research confirmed it via Claude Code's docs; a cheap live check is worth the small cost given real breakage would otherwise surface mid-round, when it's disruptive.
- **Explicit limitation:** this check proves only that Claude Code accepts the file's frontmatter and the agent starts/replies — it does NOT prove the `effort:` value was actually honored by the underlying model call versus silently ignored. No observable signal (agent listing, startup banner, session log) distinguishes honored-vs-ignored effort. This is accepted as a known V0 limitation, not solved by this task.

### Tool set: omit `tools:` entirely (inherit the full default set)

- Decision: None of the five files set a `tools:` frontmatter key.
- Rationale: Crucible rounds spawn as `general-purpose` today, which gets the full default tool set — including `BashOutput`/`KillShell` (needed for live-substrate driving and the "zero stray processes" teardown discipline in `crucible/README.md`) and `TodoWrite`/`WebFetch`/`WebSearch`. The task's original proposal copied millhouse's narrower `mill-implementer` list (`Read, Edit, Write, Bash, Grep, Glob, Skill`), which would have silently narrowed round-agent capability versus today's spawn — caught in discussion review. Omitting `tools:` keeps these profiles doing exactly one job: selecting an effort tier. Nothing else about the spawn changes.
- Rejected: Explicitly enumerating an expanded tool list matching `general-purpose`'s actual grant — more brittle (has to be kept in sync with whatever tool set `general-purpose` happens to include) for no benefit over just omitting the key. Also rejected: the original narrow 7-tool list, once the capability regression was identified.

### Placement: project-level `.claude/agents/`, not user-level

- Decision: Ship these five files under this repo's `.claude/agents/`, not `~/.claude/agents/` (user-level).
- Rationale: The profiles are specific to this repo's crucible method and its `crucible/<module>-review-prompt.md` convention; project-level scoping keeps them versioned with the method they serve and visible to any operator who checks out this repo, not just one machine. Tradeoff: a worktree branched before this task merges to `main` will not see these files until it syncs (`mill-merge-in` or equivalent) — an in-flight crucible campaign in an older worktree gets an unrecognized `subagent_type` if the operator tries to use an effort tier before merging. This is a one-time, self-correcting gap (closes the moment that worktree merges in), not a persistent one.
- Rejected: User-level `~/.claude/agents/` — works across every worktree on one machine without a merge step, but ties the profiles to one operator's machine instead of the repo, orphaning them from the crucible method they exist to serve.

### Orchestrator must ask for an explicit effort pick every round

- Decision: `orchestrator-prompt.md` states as a hard rule that the orchestrator must obtain an explicit effort-tier pick from the operator before spawning any round — if the operator names only a model ("next round, Opus") and no effort, the orchestrator asks for the missing pick rather than defaulting to a tier or falling back to `general-purpose`.
- Rationale: Mirrors the "no unsuffixed inherit fallback file" decision above — the whole point is a per-round explicit choice; a silent default (or a `general-purpose` fallback) would quietly reintroduce the exact gap this task exists to close.
- Rejected: Defaulting silently to a specific tier (e.g. `medium`); falling back to `general-purpose` when unspecified.

### Round tag form: `<model>-<effort>-r<N>`

- Decision: The round tag used in commit messages, deliverable filenames (`review-prompt-template.md`'s `<module>-review-<tag>.md` / `<module>-review-<tag>-fixer-report.md`), and the campaign table changes from `<model>-r<N>` to `<model>-<effort>-r<N>` (e.g. `opus-high-r3`).
- Rationale: Effort is now a second independent spawn axis alongside model. With the old `<model>-r<N>` tag, two rounds at the same model but different effort collide in tag and in deliverable filenames, and `README.md`'s "Round | Model | What it closed" campaign table (see the worked reed-campaign example) loses which effort produced which result. `review-prompt-template.md`'s deliverable filenames are derived mechanically from whatever tag string the orchestrator supplies, so they inherit the new form automatically — no template edit needed there.
- Rejected: Keeping `<model>-r<N>` and tracking effort only in the seed commit message — cheaper but not searchable from tag/filename. Also rejected: not recording effort per round at all — undoes the same traceability the campaign table already provides for models.

### Valid tier names must be enumerated where the operator picks

- Decision: The renamed "Model + effort selection" section in `orchestrator-prompt.md` explicitly lists the shipped tier names (e.g. "Available effort tiers: `low`, `medium`, `high`, `xhigh`, `max` — see `.claude/agents/crucible-reviewer-<effort>.md`"). Dropping a tier (per the effort-tiers Decision's smoke-check caveat) requires removing it from this list in the same commit.
- Rationale: Every edit spec elsewhere in this document uses the placeholder `crucible-reviewer-<effort>` — nothing states the five concrete values anywhere the operator actually reads before spawning. Co-locating the enumeration with the spawn instruction (rather than `README.md`) keeps it a single source of truth next to where it's used.
- Rejected: Enumerating in `README.md` instead, or in both files (a second place to keep in sync for no benefit).

## Technical context

**Precedent (crib from, do not copy verbatim — see divergences under Decisions above):** the user already built this exact pattern in millhouse, on branch `origin/hanf/linux-port-more` (fetched into the local millhouse clone at `c:/Code/millhouse/wts/millhouse`; **not yet merged to millhouse's own `main`**). Relevant commits: `fcf6b549` (`feat(mill): add mill-reviewer-medium/high/max agent definitions`) and `0ca27381` (`feat(mill): add mill-implementer-medium/high/max agent definitions`), plus the follow-up `8a87f13b` ("Agent-tool dispatch discards the effort tier already encoded in mill-agents.yaml") which also touched `plugins/mill/.claude-plugin/plugin.json`'s `agents` array — that registration step is millhouse-plugin-specific and does NOT apply here (see Scope/Out).

Millhouse's file shape (one example, `mill-reviewer-high.md`, fetched via `git show origin/hanf/linux-port-more:plugins/mill/agents/mill-reviewer-high.md`):
```markdown
---
name: mill-reviewer-high
description: Sub-agent for code review — validates findings without modifying files or running commands, but writes its report to the file named in its brief.
tools: Read, Grep, Glob, Write
effort: high
---

# mill-reviewer
...(body byte-identical to the base mill-reviewer.md)...
```
Confirms: `effort:` is a documented, real frontmatter key (source: `https://code.claude.com/docs/en/sub-agents.md`, "Supported frontmatter fields" — full supported key list is `name`, `description` (required), `tools`, `disallowedTools`, `model` (default `inherit`), `permissionMode`, `maxTurns`, `skills`, `mcpServers`, `hooks`, `memory`, `background`, `effort`, `isolation`, `color`), separate from `model:`, honored when the file is selected via `subagent_type`, and overrides the session effort level for that spawn only.

**Existing files to read before implementing:**
- `crucible/orchestrator-prompt.md` — rule 2 (line ~18: "Rounds are FRESH agents, never forks... Spawn `subagent_type: general-purpose` with a `model:` override"), loop step 2 "Spawn" (line ~30: `Agent` tool → `subagent_type: general-purpose`, `model: <the operator's pick this round>`, prompt = "Read `crucible/<module>-review-prompt.md` and do exactly what it says."), and the "Model rotation" section (line ~55).
- `crucible/README.md` — the "2. **Spawn.**" step in "The loop" section (~line 40): "One fresh `general-purpose` Agent with a `model:` override..."; and the "Why rotate the model" subsection (~line 48).
- `crucible/review-prompt-template.md` — the exact "Commit per fix" contract (section "Commit per fix (BLOCKING...)") and the "Deliverables" section's item 3 (final chat message = concise executive summary only, never paste full reports) — both must be echoed in the new agent files' body so a crucible round agent knows the commit/never-push/summary-only contract even before it opens the per-module review prompt.
- `plugins/mill/agents/mill-implementer.md` and `plugins/mill/agents/mill-reviewer.md` (in the millhouse clone, `c:/Code/millhouse/wts/millhouse/plugins/mill/agents/`) — the base (non-tiered) frontmatter shape/style to match for tone/format consistency, though crucible's content is original (there is no existing untiered `crucible-reviewer.md` to copy from — this is a new agent family, not an existing one being tiered).

**Draft body for the five new files** (identical across all five except `name`, `description`, and `effort`) — mill-plan may refine wording but should preserve every substantive point. **Sync discipline:** all five bodies must stay byte-identical except `name`, `description`, and `effort`; if `review-prompt-template.md`'s contract wording (commit-per-fix, sequencing rule, clean-room constraint) changes later, update all five bodies in the same commit — there is no automated drift guard (see Testing), so this is a manual discipline point for whoever touches these files next.
```markdown
---
name: crucible-reviewer-<effort>
description: A crucible reviewer-fixer round agent at <effort> reasoning effort — reads the per-module review prompt named in its brief and drives one independent review+fix round. Only invoked by name via subagent_type (the crucible orchestrator) — not a candidate for automatic delegation.
effort: <effort>
---

# crucible-reviewer-<effort>

You are a crucible round agent — a fresh, clean-room reviewer-fixer for one round of the crucible review+fix loop (see `crucible/README.md`). Read the per-module review prompt named in your brief (`crucible/<module>-review-prompt.md`) and do exactly what it says: form your own independent review findings first, save the review report to disk, THEN fix every recorded finding, all severities including NIT.

- **Clean-room: form your own findings first.** Do not read any prior round's review/fixer-report material before your own findings list is complete (see "Clean-room review constraint" in `crucible/review-prompt-template.md`).
- **Commit per fix, never push.** As each individual fix lands green, commit it on the current branch with a message identifying the finding it closes (see "Commit per fix" in `crucible/review-prompt-template.md`). This is a **host-repo** commit on the crucible worktree, never a weft-repo operation. Never push unless explicitly told to.
- **Final chat message is an executive summary only.** Reply with a concise executive summary, counts by severity, the two report file paths (review + fixer report), and an explicit merge-readiness verdict. Do not paste the full reports into chat.

This file exists solely to select a reasoning-effort tier via `subagent_type` at spawn time. It carries no `model:` key (orchestrator's per-call `model:` override stays independent) and no `tools:` key (inherits the full default tool set, same as `general-purpose` gets today) — effort is the only thing this profile changes. This mechanism is a **V0 stopgap**: once Hardener (crucible's V1, a Go engine) drives the loop inside a Fabric-initialized environment, effort comes from lyx's own `modelspec` instead and this file becomes obsolete.
```
Replace `<effort>` with each of `low`, `medium`, `high`, `xhigh`, `max` to produce the five files, named `crucible-reviewer-low.md` … `crucible-reviewer-max.md` under `.claude/agents/`.

**`crucible/orchestrator-prompt.md` edits:**
- Rule 2: change the spawn instruction from `Spawn subagent_type: general-purpose with a model: override` to `Spawn subagent_type: crucible-reviewer-<effort> (the operator's pick this round) with a model: override (also the operator's pick, independent of effort)`. State as a hard rule: the orchestrator must obtain an explicit effort-tier pick from the operator before spawning any round; if the operator names only a model, the orchestrator asks for the missing effort pick — it never defaults to a tier and never falls back to `general-purpose`.
- Loop step 2 "Spawn": change `Agent tool → subagent_type: general-purpose, model: <the operator's pick this round>` to `Agent tool → subagent_type: crucible-reviewer-<effort> (the operator's pick this round), model: <the operator's pick this round>`; also change the round tag example from `<model>-r<N>` to `<model>-<effort>-r<N>` (e.g. `opus-high-r3`), per the "Round tag form" Decision.
- Rename the "Model rotation" section (~line 55) to "Model + effort selection" and rewrite its body to: document that the operator picks both per round; keep the existing rationale about rotating models across rounds and add the effort-tier rationale (cheap wide sweep early vs. max-effort safety pass late) from the Problem section above; and explicitly enumerate the shipped tier names (per the "Valid tier names must be enumerated" Decision) — update this list in the same commit if the smoke check drops a tier.
- Loop step 5 "Decide" (~line 34): change "...Re-seed the prompt (step 1) with the new finding, and spawn the next full round with a **different** model" to also name effort — e.g. "...spawn the next full round with a different model and/or effort tier" — since the hard rule (rule 2) now requires an explicit effort pick on every spawn, not just a rotated model.
- State explicitly (near rule 2's hard-rule statement) the recovery path for a pre-merge worktree where a `crucible-reviewer-<effort>` profile does not yet exist: sync the worktree (`mill-merge-in` or equivalent) to pull in the profiles, then retry the spawn. This is the defined action when the operator is between an unusable tier and the forbidden `general-purpose` fallback — it is not a silent fallback, it is a required remediation step before the round can proceed at all.

**`crucible/README.md` edits:**
- In "The loop" step 2 (~line 40), update the spawn description to mention the effort-tiered `subagent_type` alongside the existing `model:` override, and update its round-tag example to `<model>-<effort>-r<N>`.
- Update the ASCII loop-box line (~line 29, "SPAWN a fresh clean-room round agent (rotate the model)") to "rotate the model and/or effort tier" — same reason as the `orchestrator-prompt.md` loop-step-5 edit above: the hard rule now requires an explicit effort pick on every spawn.
- Update "Re-seed + rotate" step 4 (~line 42, "Spawn the next round with a **different** model") to also name effort, matching the loop-box wording change.
- Add a short paragraph (near "Why rotate the model", ~line 48) explaining the effort choice and how it composes with model rotation — cross-reference `orchestrator-prompt.md`'s renamed "Model + effort selection" section (including its tier-name enumeration) rather than duplicating the rationale.
- Also update line ~22 ("a `general-purpose` Agent, *not* a fork") in "The two roles" section — this describes the round-agent spawn mechanism directly and would otherwise be left as a stale reference to the old undifferentiated `general-purpose` spawn; rephrase to describe the effort-tiered `crucible-reviewer-<effort>` spawn while keeping the "not a fork" independence point intact.
- Add an "Effort" column to the worked-example campaign table (~line 112, "Round | Model | What it closed") so future campaign write-ups can record which effort tier produced which result, per the "Round tag form" Decision. The existing reed-campaign example rows predate effort tiers and are historical; leave their Effort cells blank or "n/a" rather than fabricating a value.

**`crucible/review-prompt-template.md` edit:**
- Amend the opening line (currently: "It is the round agent's *entire* instruction set") additively, e.g.: "It is the round agent's instruction set for the review+fix work itself; the `crucible-reviewer-<effort>` agent-file preamble (`.claude/agents/`) also carries the clean-room/commit-per-fix/summary-only contract, but this file remains the authoritative statement of it — its "Commit per fix," "Sequencing rule," and "Clean-room review constraint" sections stay exactly as they are." This is additive, not a reason to trim any of those three sections: the agent-file preamble is a convenience restatement, and for any worktree that has not yet merged the new profiles, this file is still the sole carrier of that contract.

## Constraints

Most of `CONSTRAINTS.md`'s invariants are `internal/*` Go package/import-boundary rules (Hub Geometry, lyxtest Leaf, CLI/Cobra, Shuttle/Shell seams, sandbox/test-tier guards, etc.) and do not apply — this task touches only `crucible/*.md` and `.claude/agents/*.md`, no Go code. Two invariants DO place review obligations on agent *prompt text*, which is exactly what the new file bodies are:

- **Weft Git Invariant** — "agent prompt templates never instruct a weft git op... An LLM agent never drives weft git." The new bodies instruct `git commit` on the current branch — this is a **host-repo** commit (normal dev git on the crucible worktree), never a weft-repo operation; the invariant's own carve-out ("an agent does commit its own code to the host repo... the weft, never") is exactly what applies here. State this explicitly in the body so a reader doesn't mistake "commit per fix" for a weft-git instruction.
- **Review Round Invariant** — "A-before-B..., every recorded finding is fixed in B, all severities including LOW/NIT; no self-grading...; commit-per-fix on host source, never push." This is precisely the contract the new bodies restate (via `review-prompt-template.md`'s "Commit per fix" + "Sequencing rule" sections) — the bodies satisfy this invariant by construction; they don't newly need to comply with it.

The draft body's "Commit per fix" bullet (see Technical context) now includes an explicit "this is a host-repo commit... never a weft-repo operation" clause, satisfying the Weft Git Invariant point directly in the shipped text; the Review Round Invariant point is satisfied by construction (the body's clean-room/commit-per-fix/fix-everything-severities content restates that invariant's own requirements) and needs no separate clause.

One process constraint from root `CLAUDE.md` does apply: **markdown one-line-per-paragraph** — every new/edited `.md` file (the five agent files' bodies, plus the `orchestrator-prompt.md`/`README.md` edits) must be written with one continuous line per paragraph/list item, no hard-wrapping. Low collision risk noted in the original task body with the separate `markdown-unwrap` task (which only reflows whitespace in these same two crucible files) — write everything one-line-per-paragraph from the start so no reflow is ever needed; whichever task merges second runs `mill-merge-in`.

## Testing

This task has no Go code and no existing automated test harness for `.claude/agents/*.md` content (unlike millhouse's Python-based `test-agents-defs.py`, which is specific to millhouse's plugin packaging). Verification is:

1. **Frontmatter sanity (static):** every one of the five files parses as valid YAML frontmatter with exactly `name`, `description`, `effort` set (no `model` key, no `tools` key), and `effort` matches one of the five intended values with no typos.
2. **Live smoke check (per the Verification decision above):** for each of the five new `subagent_type` values, one throwaway `Agent` call with a trivial no-op prompt ("Reply with the single word OK."), confirming it spawns and replies without a tool/frontmatter error. **Pass criterion is spawn success, independent of reply content:** the agent starts and produces ANY reply with no frontmatter/tool-load error — the draft body instructs it to read a review prompt named in "your brief," which this throwaway prompt doesn't provide, so a tier may reasonably reply with a question or a clarifying refusal instead of literally "OK"; that is still a PASS. Only a tool-schema/frontmatter-load error (the agent fails to start at all) counts as a FAIL. **This proves only that the frontmatter parses and the agent starts/replies — it does NOT prove the `effort:` value was actually honored** (no observable signal distinguishes honored-vs-ignored effort; see the Verification decision's explicit limitation). If `low` or `xhigh` fails to spawn cleanly, drop that tier (see the effort-tiers Decision's caveat) rather than ship a broken file. This is NOT a real crucible review round — no module code is touched, no real review prompt is read, cost is minimal. Discard results after confirming success; do not commit anything from this step.
3. **Doc-consistency read-through:** after editing `orchestrator-prompt.md` and `README.md`, re-read both files end-to-end to confirm every remaining reference to the old undifferentiated spawn was actually updated. Grep for the bare token `general-purpose` across `crucible/` — this CANNOT return zero hits after the edits, since rule 2 is required to state "...never falls back to `general-purpose`" (see the "Orchestrator must ask for an explicit effort pick" Decision). Instead, treat it as an **expected-hit inventory**: after the edits, the token should appear only at the deliberate never-fall-back mention in `orchestrator-prompt.md` rule 2, and nowhere else — any hit outside that one deliberate mention (e.g. a description still saying "general-purpose Agent" as if it were the current spawn mechanism) is the real stale-reference signal.
4. **Round-tag / tier-enumeration consistency:** confirm the renamed "Model + effort selection" section actually lists all five (or however many survive the smoke check) tier names, and that every example round tag in both edited files uses the `<model>-<effort>-r<N>` form consistently (no leftover `<model>-r<N>` example).

## Q&A log

- **Q:** Which effort levels should the new agent profiles cover — all five (low/medium/high/xhigh/max), match millhouse's own precedent (medium/high/max only), or a trimmed low/high/max? **A:** All five — the task's own stated use case explicitly wants the cheap low-effort end, which millhouse's precedent (serving a different, automated-dispatch use case) didn't need.
- **Q:** Should every tier get its own explicit file with no unsuffixed "inherit" fallback, or also ship a bare `crucible-reviewer.md` as a "don't care" option? **A:** Every tier explicit, no fallback file — the whole point is an explicit per-round choice, mirroring how `model:` is already chosen explicitly each round.
- **Q:** Should each tier's `description:` state its effort level in prose, or stay byte-identical across tiers like millhouse's own precedent files? **A:** State it in prose — self-explanatory in a listing without inspecting frontmatter; deliberate divergence from the millhouse convention.
- **Q:** Should `model:` be omitted from every tier file (relying on the orchestrator's per-call override), or set explicitly to `inherit`? **A:** Omitted entirely, confirmed against the fact that the `Agent` tool's call site has no `effort` parameter at all (only `model` + `subagent_type`) — that gap is precisely why the file-based effort mechanism is needed in the first place, and it's also exactly what the millhouse precedent files do.
- **Q:** How should this be verified, given no automated test harness exists for these files? **A:** A live smoke check — one throwaway `Agent` call per new `subagent_type` with a trivial no-op prompt — cheap on tokens, not a real crucible round.
- **Q:** (Discussion review r1, gap 1) `low`/`xhigh` are asserted valid `effort:` values with no direct evidence — keep all five with the gap stated, drop to the millhouse-verified `medium/high/max`, or leave unchanged? **A:** Keep all five, but state explicitly that `low`/`xhigh` are unverified and that the live smoke check is their first real confirmation; drop a tier if its smoke check fails.
- **Q:** (r1, gap 2) The smoke check can't distinguish honored-vs-ignored effort — state the limitation explicitly, have the agent self-report its effort, or drop the check? **A:** State the limitation explicitly; this is an accepted V0 gap, not solved by this task.
- **Q:** (r1, gap 3) The copied 7-tool list silently narrows the round agent vs. today's `general-purpose` spawn (missing `BashOutput`/`KillShell`/`TodoWrite`/`WebFetch`/`WebSearch`) — omit `tools:` to inherit the full default set, enumerate an expanded list, or keep the narrow list? **A:** Omit `tools:` entirely from all five files.
- **Q:** (r1, gap 4) `crucible/README.md:22`'s stale `general-purpose` mention was missed by the edit list and by the proposed greps (backtick breaks phrase match) — add it to the edit list and fix the grep, or leave line 22 and only fix the grep? **A:** Add `README.md:22` to the edit list (rephrase to describe the effort-tiered spawn) and change the grep to the bare token `general-purpose`.
- **Q:** (r1, gap 5) Nothing defines orchestrator behavior when the operator names a model but no effort — must-ask, silent default, or fall back to `general-purpose`? **A:** Orchestrator must ask for the missing effort pick; never default, never fall back to `general-purpose`.
- **Q:** (r1, gap 6) "No CONSTRAINTS.md entry applies" is overstated — the Weft Git Invariant and Review Round Invariant both place review obligations on agent prompt text. Replace the blanket claim or leave it? **A:** Replace it with an explicit acknowledgment of both invariants and how the new bodies satisfy them.
- **Q:** (Discussion review r2, gap 1) The round tag `<model>-r<N>` collides across effort tiers and the campaign table loses which effort produced which result — adopt `<model>-<effort>-r<N>`, track effort only in the seed commit message, or don't record it per round? **A:** Adopt `<model>-<effort>-r<N>`; add an Effort column to the campaign table.
- **Q:** (r2, gap 2) The five concrete tier names are never enumerated anywhere the operator reads before spawning — enumerate in the renamed orchestrator-prompt.md section, in README.md, or both? **A:** Enumerate in the renamed "Model + effort selection" section of `orchestrator-prompt.md` only; update it in the same commit if a tier is dropped.
- **Q:** (r2, gap 3) The Constraints section promised a host-repo-only clause in the draft body that was never actually added — add the clause to the body, or drop the promise from Constraints? **A:** Add the clause to the body's "Commit per fix" bullet.
- **Q:** (r2, gap 4) The "zero stale hits" grep for `general-purpose` is unsatisfiable once rule 2 must contain that literal token — restate as an expected-hit inventory, or drop the check? **A:** Restate as an expected-hit inventory (exactly one deliberate mention expected, in rule 2).
- **Q:** (Discussion review r3, gap 1) Three "rotate the model" lines (`orchestrator-prompt.md` loop step 5, `README.md`'s ASCII loop box and "Re-seed + rotate" step 4) still describe rounds as model-only, contradicting the new explicit-effort-pick hard rule — add all three to the edit list, or leave them deliberately model-only? **A:** Add all three to the edit list.
- **Q:** (r3, gap 2) The template-amendment wording ("this file does not need to restate that scaffolding") is self-contradictory since the template still carries those sections — reword additively, or drop the amendment? **A:** Reword additively; the template remains authoritative and its three sections stay.
- **Q:** (r3, NOTE) No recovery path is stated for a pre-merge worktree where the tier profile doesn't exist yet — state one, or leave undefined? **A:** State it: sync via `mill-merge-in` (or equivalent), then retry — not a silent fallback, a required remediation step.
- **Q:** (r3, NOTE) The smoke check's pass criterion is ambiguous since the throwaway prompt doesn't match what the body expects (a brief) — define pass as exact-reply-match or as spawn-success-independent-of-content? **A:** Spawn-success independent of reply content; only a frontmatter/tool-load failure counts as FAIL.
- **Q:** (Ongoing, r3) Operator asked to auto-apply the recommended option for all remaining discussion-review rounds rather than being asked gap-by-gap. **A:** Confirmed — subsequent rounds resolve gaps by auto-picking the recommended option and proceeding.
