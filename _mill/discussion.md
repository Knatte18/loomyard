# Discussion: loom: code-writing skills — comments, build, testing

```yaml
task: loom: code-writing skills — comments, build, testing
slug: loom-code-writing-skills
status: discussing
parent: main
```

## Problem

`manifest/designs/code-comment-conventions.md` states a self-containment rule for doc comments — a comment must be a standalone contract, never referencing another named symbol — but has no installable skill implementing it.
loomyard also lacks the "build" and "testing" legs of a code-writing skill set the way millhouse has for its own consumers.
Without these, an agent working on Go code in this repo, or in any other repo that installs loomyard's plugins, has no explicit encoding of this project's actual comment/quality/testing philosophy;
it falls back to generic defaults or to millhouse's own installed skills, which are outdated in several concrete places and not owned by this project.

## Scope

**Already in the worktree** (built during this discussion, ahead of the usual mill-plan/mill-go order, on explicit operator instruction — see the Q&A log entry on sequencing):

- The `scribe` plugin at `plugins/scribe/`: seven skills (`prose`, `conversation`, `code-quality`, `testing`, `golang-comments`, `golang-build`, `golang-testing`), `.claude-plugin/plugin.json`, `settings.json`, `hooks/hooks.json`, `skills/INDEX.md`.
- `.claude-plugin/marketplace.json` registers `scribe` alongside `prowler`.
- `manifest/designs/code-comment-conventions.md` rewritten: the operative self-containment rule now lives in `code-quality`'s Comments section; the design doc keeps only rationale content and points to the skill (Producer Pointer-Rule Invariant, `CONSTRAINTS.md:567`).
- `manifest/roadmap.md`'s Wave 1 entry for this task reworded to match what was actually decided (see Decisions) and to point at this discussion; not yet moved to the `## Done` section — see the Testing section below for why.
- A real structural verification pass (not just marketplace-JSON parsing): every skill's frontmatter `name` matches its directory, `plugin.json`/`settings.json`/`hooks.json` all parse, `INDEX.md` lists all seven, `marketplace.json`'s `scribe` entry points at the right source path.
  All checks passed at the time of this writing.

**Still to do:**

- Discussion review sign-off — this is what's currently in progress.
- Whatever the sign-off surfaces beyond what's already fixed (see Decisions' two newest entries and the Q&A log for what a first review round already caught and fixed in-place).
- Moving the roadmap entry to `## Done`, once review actually concludes.
- Handoff to `/mill-plan` — normally where implementation would start; here it inherits an already-built plugin instead, so its job is closer to verification-and-polish planning than build-from-scratch planning.

**Out:**

- `golang-quality` — a hypothetical Go-idiom skill (accept-interfaces/return-structs, error-value mechanics, package design, zero-value usability, concurrency ownership).
  Considered at length and explicitly rejected — see Decisions.
- C#/Python equivalents of any skill here — out of scope; this task is Go-only, per `code-comment-conventions.md`'s own stated scope.
- Wiring "load these skills" into loom's own producer stencils (Discussion-Write, Plan-Write, etc.) — a separate, later roadmap item ("loom: Discussion-Write producer"), not this one.
  The plugin's own `hooks/hooks.json` covers default-loading in the meantime — see Decisions.
- Actually running `/plugin install scribe@loomyard` — the plugin's files and marketplace entry are in place; installing it into a live Claude Code environment is a manual operator step, not part of this task.
- A mechanical lint pass enforcing the self-containment rule — `code-comment-conventions.md`'s own "Wave 2" enforcement tier, which was always scoped as future/Quarry-dependent work.
  Today's enforcement is review discipline only, unchanged from the design doc's original plan.

## Decisions

### One plugin, not two

- Decision: a single plugin, `scribe`, not a two-plugin split mirroring millhouse's `mill` (general) + `golang` (per-language) structure.
- Rationale: loomyard's only existing precedent, `prowler`, is one plugin holding several skills under one `skills/` directory.
  The operator wants very few skills per plugin;
  splitting into general + per-language plugins adds plugin-count overhead without a corresponding benefit here.
- Rejected: mirroring millhouse's two-plugin split — no local precedent for it, and it doesn't reduce total skill count.

### Plugin name: scribe

- Decision: `scribe`.
- Rationale: needed to avoid colliding with millhouse's installed `mill`/`golang` plugin names, and to stay distinct from a separate, not-yet-built "everything needed to use loomyard" plugin (tentatively `lyxsmith`) discussed in passing — a different scope entirely, see Technical context.
  `scribe` is an agent-noun (one who writes) with no collision in this codebase's existing vocabulary — `loom`, `weft`, `reed`, `shed`, `fabric`, `hub`, `webster` are all already claimed internally.
- Rejected: `code-writing` (operator didn't like the name); `quill`, `ink` (considered alongside `scribe`, `scribe` preferred).

### code-quality and code-comments merged

- Decision: the roadmap's original two general skills, `code-quality` and `code-comments`, are one skill — `code-quality` — with comment-content rules in its own "Comments" section rather than a separate file.
- Rationale: code and its comments are one craft, not two separable concerns.
  Keeping them split forced an artificial "where's the line" boundary that had no natural answer — early drafts needed a dedicated note explaining the split, which is itself a symptom the split was wrong.
  Millhouse has no standalone "code-comments" skill to mirror in the first place;
  this was never a rename of an existing split, just new territory drafted as two files, then corrected to one.
- Rejected: keeping them as two skills, the roadmap's literal original text — rejected once the artificial boundary became apparent in drafting.

### prose and conversation split out of comment content

- Decision: a new `prose` skill (always-active by convention) governs the STYLE of all text an agent writes — chat replies, markdown, code comments, docstrings: terseness, no empty intensifiers, no padding, semantic line breaks.
  `code-quality`'s Comments section governs only comment CONTENT (self-containment, necessary-and-sufficient).
  A separate, thinner `conversation` skill (also always-active) carries chat-interaction rules that aren't about writing style at all — tone, no compliments, numbered-choice-list formatting, `.scratch/` conventions, no `sed`.
- Rationale: the operator observed a concrete, recurring problem — Claude writes disciplined, terse chat replies (millhouse's own `conversation` skill already governs that) but sloppy, padded markdown and docstrings, because that skill's style rules never applied outside chat.
  Splitting style (`prose`) from comment content (`code-quality`) and from chat mechanics (`conversation`) lets the same style discipline apply everywhere text is produced, not only in chat.
- Rejected: a separate `markdown` skill — considered, then dropped once `prose` was established as always-active, since a conditionally-loaded skill is a poor fit for something written this often;
  markdown's real content (semantic line breaks, single-line table cells/blockquotes, heading discipline) is folded directly into `prose` instead.

### Semantic line breaks generalized and correctly scoped

- Decision: `prose`'s "Line breaks" section applies to any multi-line prose written into a file — markdown paragraphs and list items, and multi-line code comments/docstrings alike — not only `.md` files.
- Rationale: this repo's own CLAUDE.md already had this rule, scoped to markdown files in this repo only.
  The underlying justification — a single-word edit shouldn't cascade into re-wrapping unrelated lines in a diff — applies identically to a multi-line godoc comment, since that's also line-broken text in a versioned file.
  Generalizing it also makes it portable, since this skill ships to other repos that won't have this repo's specific CLAUDE.md wording.
- Rejected: an early draft scoped it to `.md` files only, caught and corrected during discussion;
  leaving it solely in this repo's local CLAUDE.md was also rejected, since it wouldn't travel with the plugin.

### Necessary-and-sufficient doc comment bar

- Decision: a symbol's signature plus its doc comment must let a reader decide whether it fits their task without opening the file or reading the implementation — "necessary and sufficient," not merely "self-contained."
  Two named failure directions: insufficient (omits purpose/why, or leans on another symbol) and excessive (padding, tangential remarks, history).
- Rationale: self-containment alone doesn't prevent a comment from being accurate, self-contained, and bloated — the operator specifically observed Claude writing excessively long C# doc comments (`<remarks>` blocks, cross-method references) that were technically self-contained.
  The same bar, applied to file headers, is what makes mechanical per-file and per-directory table-of-contents extraction (tree-sitter or similar) possible without parsing implementation — that is the deeper motivation, not tidiness for its own sake.
- Rejected: a bare self-containment rule with no length/relevance ceiling — insufficient to prevent the observed verbosity problem.

### File-header exception is about siblings, not size

- Decision: a file header may be skipped only when the file is the sole file in its directory/module — never based on file size.
- Rationale: the header exists so a reader can disambiguate among sibling files;
  a module with only one file has nothing to disambiguate from, since the module-level doc already covers it, but a small file sharing a directory with others still needs its own header, because the reader is choosing among siblings regardless of any individual file's size.
  An earlier draft tied the exception to "small, single-purpose file" — a real logic bug, caught by an independent orchestrator review, not just imprecise wording.
- Rejected: the size-based exception (the bug); no exception at all (rejected as needless, since the sole-file case genuinely has nothing to disambiguate).

### Trust the caller; fix the defect, don't band-aid the symptom

- Decision: two related but distinct rules in `code-quality`'s "Errors and control flow":
  (1) for a non-public-API module, assume the caller uses it correctly — don't build failure handling or write tests for misuse a caller could invent, including not re-validating what the type system/compiler already guarantees (e.g. a null check on a non-nullable parameter);
  (2) when your own logic produces an invalid result (a NaN, for instance), fix the defect at its source — don't add a check that tolerates or special-cases the symptom.
- Rationale: both are explicit operator corrections to a default LLM tendency toward defensive-programming bloat — checking things the type system already rules out, or silently tolerating a symptom instead of tracing back to the actual bug.
  Kept as two separate bullets rather than merged, since they're different failure modes: caller misuse of a well-documented module versus a bug in one's own logic.
- Rejected: leaving the existing "validate at boundaries" bullet to imply this without stating it — the operator asked for it stated directly, and it has a testing-discipline consequence the boundary-validation bullet alone doesn't cover.

### Testing's Error paths qualified to trust boundaries

- Decision: `testing`'s Coverage section's "Error paths" bullet is qualified to trust boundaries — a public API, user input, an external system — not internal misuse of a well-typed, well-documented module.
  A test asserting behavior for an input the type system already rules out (wrong type, a non-nullable made null) is named explicitly as testing something the compiler already prevents, not a real path.
- Rationale: direct consequence of the trust-the-caller decision above — the original, unqualified "always test error paths" was in tension with it.
- Rejected: leaving it unqualified.

### golang-comments: dedup with code-quality, then structural consolidation

- Decision: `golang-comments` adds only Go-specific mechanics on top of a `code-quality` reference — it never restates a concept `code-quality` already states in full.
  Small single-fact sections ("Boolean-returning functions," "Methods on a type") are folded into "Exported symbol doc comments" as bullets with inline illustrations rather than full separate headings and example pairs;
  "File-level comments" and "Package doc comments" are merged into one section, contrasted by the one fact that actually distinguishes them (whether a blank line separates the comment from `package`).
- Rationale: two independent review passes — the operator's own read and a relayed orchestrator review — both found the same class of problem: the file re-explained concepts (why a file header exists, what "necessary and sufficient" means) that already live in `code-quality`, instead of only adding Go-specific syntax.
  Went through every section on that basis, not only the flagged examples.
  Net effect across three trim rounds: roughly 2,120 to 1,560 estimated tokens (~26% reduction), with no rule content lost — only duplication and marginal examples cut.
- Rejected: the fuller, more expository version — explicitly rejected twice by the operator as still too long even after the first dedup pass.

### golang-quality not built

- Decision: no `golang-quality` skill in this task's scope.
- Rationale: real, non-redundant candidate content exists (accept-interfaces/return-structs, error-as-value mechanics, package design, zero-value usability, concurrency ownership), but unlike every other skill in this plugin, no concrete observed problem motivates building it now.
  Comments got its rework because of an observed verbosity/cross-referencing problem;
  testing's determinism section addressed a real, demonstrated gap;
  this would be anticipatory completeness, which `code-quality`'s own YAGNI rule argues against.
  The candidate content is also mostly well-established Go community knowledge that a capable model already leans toward by default, unlike the self-containment rule, which is a genuine project-specific departure from default behavior.
- Rejected: building it now on completeness grounds — proposed, briefly reversed into "yes, build it" when "millhouse doesn't have it" was correctly identified as a non-argument, then reconsidered a final time against the YAGNI test and dropped again.

### Always-active mechanism: a hook nudge, plus explicit loading inside lyx's own prompts

- Decision: two distinct mechanisms cover "always-active," for two distinct contexts.
  For a lyx-generated prompt (a loom producer stencil, or anything lyx itself writes), the prompt carries an explicit "Load these skills: ..." line — the same pattern `mill-start` already uses to load `mill:conversation` unconditionally as its own first step;
  this wiring is the separate, later roadmap item named in Scope Out.
  For any other session with the plugin installed, `plugins/scribe/hooks/hooks.json` ships a `UserPromptSubmit` hook whose command injects an instruction to load `scribe:prose` (and `scribe:conversation` for a chat reply) before responding.
- Rationale: a first review round flagged that "always-active" shipped as a bare claim with no mechanism behind it, plus a dangling pointer to a mill-transient file (`discussion.md`) that wouldn't travel with the plugin.
  Researched whether Claude Code plugins can force-load a skill; confirmed answer: no — a hook can only inject prompt text asking the agent to load a skill, the same mechanism this very discussion.md's own CLAUDE.md-reading hook already relies on in every turn of this session, reliably in practice even though it isn't a platform-enforced guarantee.
  That's the best available mechanism today, so it's what shipped.
- Rejected: leaving the claim undefined (the reviewed state); waiting for the producer-stencil wiring alone to cover it, which would leave every non-lyx-driven session with no default loading at all.

### Design doc points to the skill, not the reverse

- Decision: `code-comment-conventions.md`'s operative rule text (the rule, its two exceptions, the information triage) is no longer duplicated in the design doc — it points to `code-quality`'s Comments section as the canonical text.
  The design doc keeps only content a skill shouldn't carry: the deeper rationale for why the rule is stronger than a staleness rule, and the not-yet-built Quarry query mechanism.
- Rationale: a first review round cited `CONSTRAINTS.md`'s Producer Pointer-Rule Invariant — an instruction file must never duplicate or paraphrase another file's format-contract content, only point at it — against the design doc and the skill both carrying the same rule text.
  Resolved in favor of the skill as the source, not the design doc, because the skill is the artifact an agent actually loads at runtime;
  the design doc lives under `manifest/designs/` in this repo only and won't exist at all in another repo that installs the `scribe` plugin, so making it the canonical source would leave the skill non-self-contained everywhere else it ships.
- Rejected: making the design doc canonical and the skill a bare pointer — would break the skill's portability, a worse failure than the duplication the invariant guards against.

## Technical context

- Plugin lives at `plugins/scribe/`, mirroring `plugins/prowler/`'s shape: `.claude-plugin/plugin.json`, `settings.json` (granting `Skill(scribe:*)`), `hooks/hooks.json` (the always-active nudge, see Decisions), `skills/<name>/SKILL.md` per skill, `skills/INDEX.md`.
- Registered in `.claude-plugin/marketplace.json` alongside `prowler`.
- `update-plugins.sh --dry-run`-equivalent check: it reports `Skipped (not installed): scribe@loomyard -- run '/plugin install scribe@loomyard' first`.
  That line alone proves only that `marketplace.json` parses and names `scribe` — the script's not-installed branch returns before it ever touches `plugins/scribe/`'s own contents, so it validates nothing about the plugin's shape.
  A separate, real structural check was run instead (see Scope's "Already in the worktree" for what it covers) and passed;
  installing the plugin into a live Claude Code environment (`/plugin install scribe@loomyard`) remains a manual operator step outside this task.
- Cross-references between skills: `golang-comments` → `code-quality`'s Comments section;
  `golang-testing` → `testing` (a same-plugin reference, replacing millhouse's cross-plugin `@code:testing` textual pointer, since everything here lands in one plugin);
  `conversation` → `prose`;
  `code-quality`'s Comments section → `prose` for writing style.
- A separate, unrelated future plugin — "everything needed to use loomyard" (a compiled `lyx` binary, stencils, a handful of skills) — was discussed in passing as a different, not-yet-built plugin, naming leaning toward `lyxsmith`/`lyxkit` but not finalized.
  It is out of scope here; don't conflate it with `scribe`.
- `manifest/designs/code-comment-conventions.md` now points at `code-quality`'s Comments section as the canonical rule text rather than duplicating it — see Decisions.
- No producer-stencil wiring exists yet — "load these skills" instructions in loom's own prompts are a separate, later roadmap item, out of scope here;
  `hooks/hooks.json` covers default-loading for every other context in the meantime.

## Constraints

CONSTRAINTS.md's cross-cutting invariants (Cwd Resolution, gitkit Leaf, hubforge Fabric-Fixture, CLI/Cobra, Documentation Lifecycle) don't apply to this task — it adds Claude Code skill/plugin content, not a Go module or CLI command, and touches nothing under `internal/` or `cmd/`.
No new CONSTRAINTS.md entry is needed.

## Testing

Two different senses, since the deliverable is mostly prose/markdown skill content, not executable code.

**Content review:** each skill file read against its own stated rules (does `code-quality.md` avoid padding by its own standard?
does `golang-comments.md` avoid restating `code-quality`?).
Done iteratively during discussion, including two dedicated trim passes on `golang-comments` and this task's own discussion-review round, which caught the file-header logic bug, the always-active gap, the Pointer-Rule Invariant violation, and the other findings resolved in Decisions above.

**Structural verification:** a real check (not just `update-plugins.sh`'s marketplace-JSON parse) confirming `plugin.json`/`settings.json`/`hooks.json` all parse, every skill's frontmatter `name` matches its directory, `INDEX.md` lists all seven skills, and `marketplace.json`'s `scribe` entry points at the right source path.
Passed at time of writing;
no persisted test script was added for this — it was a one-time check, not a mechanical gate this task decided the repo needs going forward.

## Review guidance

For whoever reviews this discussion and the linked skill files (`plugins/scribe/skills/*/SKILL.md`):

- Focus on: can anything be shortened further, is anything redundant — within one file, or duplicated across two files — and is any worked example pulling its weight relative to its size.
- Do not compare against millhouse's equivalent skills as a baseline.
  This plugin is deliberately not a port of millhouse's content in several places (see Decisions above) and isn't meant to match it — it's meant to improve on it.
- You don't have access to why each decision above was made beyond what's written here.
  If something reads as a real design gap rather than a wording or length issue, flag it as a question rather than assuming the omission was accidental.

## Q&A log

- **Q:** Should implementation (the plugin scaffold, marketplace entry) happen before or after this discussion.md is written?
  **A:** Before, on explicit operator instruction ("do all the rest, then write discussion.md") — the reverse of mill-start's usual order, where discussion normally precedes and gates implementation.
  This is why Scope is split into "already in the worktree" versus "still to do" rather than reading as a pre-implementation plan.
- **Q:** Does the file-header exception apply to any small file, or only the sole file in a package?
  **A:** Only the sole file in a package — size was never the actual test;
  a small file sharing a package with siblings still needs its own header.
- **Q:** Should `testing`'s "always test error paths" include internal misuse of a non-public-API module?
  **A:** No.
  Concrete example: a C# method `func(int x)` where `x` is non-nullable;
  Claude has been observed adding `if x is not null` checks against that signature, which is meaningless — the compiler already prevents a non-nullable `int` from being null.
  If nullable is intended, the signature should say `int? x`.
- **Q:** When a calculation produces NaN, should the code special-case or tolerate it?
  **A:** No — NaN usually signals an upstream bug, such as division by zero;
  fix the defect at its source, don't add a band-aid check for the symptom.
- **Q:** Are worked code examples in a skill ever just redundant restatement, given `prose`'s "say it once" rule?
  **A:** Partially disagree as a blanket claim — an example pins down a fuzzy judgment line ("how much padding is too much") that prose rules alone leave to interpretation, a different function than restating a rule.
  But the count matters: `golang-comments`' exported-symbol section had four examples for one rule, trimmed to three, then consolidated further in a later round.
