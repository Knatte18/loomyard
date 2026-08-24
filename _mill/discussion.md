# Discussion: loom: code-writing skills — comments, build, testing

```yaml
task: loom: code-writing skills — comments, build, testing
slug: loom-code-writing-skills
status: discussing
parent: main
```

## Problem

`manifest/designs/code-comment-conventions.md` states a self-containment rule for doc comments — a comment must be a standalone contract, never referencing another named symbol — but has no installable skill implementing it.
loomyard also lacks build- and testing-convention skills of its own.
Without these, an agent working on Go code in this repo, or in any other repo that installs loomyard's plugins, has no explicit, owned encoding of this project's actual comment/quality/testing philosophy to follow — it falls back to generic defaults instead.

## Scope

**Already in the worktree** (built during this discussion, ahead of the usual discussion-then-implementation order, on explicit operator instruction — see the Q&A log entry on sequencing):

- The `scribe` plugin at `plugins/scribe/`: seven skills (`prose`, `conversation`, `code-quality`, `testing`, `golang-comments`, `golang-build`, `golang-testing`), `.claude-plugin/plugin.json`, `settings.json`, `hooks/hooks.json`, `skills/INDEX.md`.
- `.claude-plugin/marketplace.json` registers `scribe` alongside `prowler`.
- `manifest/designs/code-comment-conventions.md` rewritten: the operative self-containment rule now lives in `code-quality`'s Comments section; the design doc keeps only rationale content and points to the skill.
- `manifest/roadmap.md`'s Wave 1 entry for this task reworded to match what was actually decided (see Decisions) and to point at this discussion; not yet moved to the `## Done` section — see the Testing section below for why.
- A real structural verification pass (not just marketplace-JSON parsing): every skill's frontmatter `name` matches its directory, `plugin.json`/`settings.json`/`hooks.json` all parse, `INDEX.md` lists all seven, `marketplace.json`'s `scribe` entry points at the right source path.
  All checks passed at the time of this writing.
- `golang-build`'s toolchain reconciled with this repo's actual test-tier scheme (Tier 1 `go test ./...`, Tier 2 `-tags integration`, separate `smoke` tag) and its `goimports`/`golangci-lint` mandate scoped as not-adopted-here.

**Still to do:**

- Discussion review sign-off — in progress; this task finalizes directly from that loop, with no separate plan-writing phase, on explicit operator instruction.
- Whatever the current review round still surfaces beyond what's already fixed (see Decisions and the Q&A log for what's already been caught and resolved).
- Moving the roadmap entry to `## Done`, once review concludes.

**Out:**

- `golang-quality` — a hypothetical Go-idiom skill (accept-interfaces/return-structs, error-value mechanics, package design, zero-value usability, concurrency ownership).
  Considered at length and explicitly rejected — see Decisions.
- C#/Python equivalents of any skill here — out of scope; this task is Go-only, per `code-comment-conventions.md`'s own stated scope.
- Wiring "load these skills" into loom's own producer stencils (Discussion-Write, Plan-Write, etc.) — a separate, later roadmap item ("loom: Discussion-Write producer"), not this one.
- Actually running `/plugin install scribe@loomyard` — the plugin's files and marketplace entry are in place; installing it into a live Claude Code environment is a manual operator step, not part of this task.
- A mechanical lint pass enforcing the self-containment rule — `code-comment-conventions.md`'s own "Wave 2" enforcement tier, which was always scoped as future/Quarry-dependent work.
  Today's enforcement is review discipline only, unchanged from the design doc's original plan.

## Decisions

### One plugin, not two

- Decision: a single plugin, `scribe`, holding all seven skills under one `skills/` directory.
- Rationale: loomyard's own marketplace precedent, `prowler`, is one plugin holding several skills.
  The operator wants very few skills per plugin;
  splitting further adds plugin-count overhead without a corresponding benefit here.
- Rejected: a general-plugin-plus-per-language-plugin split — no local precedent for it, and it doesn't reduce total skill count.

### Plugin name: scribe

- Decision: `scribe`.
- Rationale: needed a name distinct from other plugins that might be installed in the same Claude Code environment, and distinct from a separate, not-yet-built "everything needed to use loomyard" plugin (tentatively `lyxsmith`) discussed in passing — a different scope entirely, see Technical context.
  `scribe` is an agent-noun (one who writes) with no collision in this codebase's existing vocabulary — `loom`, `weft`, `reed`, `shed`, `fabric`, `hub`, `webster` are all already claimed internally.
- Rejected: `code-writing` (operator didn't like the name); `quill`, `ink` (considered alongside `scribe`, `scribe` preferred).

### code-quality and code-comments merged

- Decision: comment content is a section of `code-quality`, not a separate skill.
- Rationale: code and its comments are one craft, not two separable concerns.
  An early two-file draft forced an artificial "where's the line" boundary that had no natural answer — needing a dedicated note to explain the split was itself a symptom the split was wrong.
  Corrected to one file once that became apparent.
- Rejected: keeping them as two skills — rejected once the artificial boundary became apparent in drafting.

### prose and conversation split out of comment content

- Decision: a new `prose` skill (always-active by convention) governs the STYLE of all text an agent writes — chat replies, markdown, code comments, docstrings: terseness, no empty intensifiers, no padding, semantic line breaks.
  `code-quality`'s Comments section governs only comment CONTENT (self-containment, necessary-and-sufficient).
  A separate, thinner `conversation` skill (also always-active) carries chat-interaction rules that aren't about writing style at all — tone, no compliments, numbered-choice-list formatting, `.scratch/` conventions, no `sed`.
- Rationale: the operator observed a concrete, recurring problem: Claude writes disciplined, terse chat replies but sloppy, padded markdown and docstrings, because no style discipline applied outside chat replies.
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
  An earlier draft tied the exception to "small, single-purpose file" — a real logic bug, caught during review, not just imprecise wording.
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
- Rationale: two independent review passes both found the same class of problem: the file re-explained concepts (why a file header exists, what "necessary and sufficient" means) that already live in `code-quality`, instead of only adding Go-specific syntax.
  Went through every section on that basis, not only the flagged examples.
  Net effect across three trim rounds: roughly a 26% size reduction, with no rule content lost — only duplication and marginal examples cut.
- Rejected: the fuller, more expository version — explicitly rejected twice by the operator as still too long even after the first dedup pass.

### golang-build and golang-testing: minimal drafting, one real toolchain question surfaced

- Decision: both skills carry Go build/lint/test commands and testing-framework conventions (table-driven tests, `t.Helper()`, `t.Cleanup`, same-package vs. external test files), drafted directly and reformatted for line-break/terseness discipline — no content redesign, since neither skill's subject matter was contested during discussion.
- Rationale: comment content and code quality needed real design work because this project's actual philosophy diverges from generic defaults in specific, observed ways;
  build commands and Go testing-framework mechanics don't — there was nothing to redesign.
  Two real gaps surfaced independent of drafting quality, both now fixed: `golang-build`'s default `goimports`/`golangci-lint` mandate and its "run all tests found" default didn't reconcile with this repo's own established two-tier test scheme, and `golang-testing`'s per-project section was left as an unfilled generic placeholder even though this repo has two machine-enforced test invariants (Test Tier Purity, Hermetic Git Test Environment) an agent following the skill here would otherwise violate.
  Fixed both the same way: filled each skill's own "per-project configuration" section with this repo's actual convention, rather than changing the generic top-level defaults, which stay reasonable for other repos installing this plugin.
  `golang-build`'s generic "Tool installation" section also got one line added, noting the mandate is overridable per project — the unqualified "stop" language and the repo-specific "skip" note read as contradictory without it.
- Rejected: leaving the generic defaults unreconciled with this repo's actual practice — a real inconsistency, not a stylistic one, since an agent following either skill literally here would have run tools this repo doesn't use, missed the tier this repo actually relies on, or written a test that fails CI under a machine-enforced invariant.

### golang-quality not built

- Decision: no `golang-quality` skill in this task's scope.
- Rationale: real, non-redundant candidate content exists (accept-interfaces/return-structs, error-as-value mechanics, package design, zero-value usability, concurrency ownership), but unlike every other skill in this plugin, no concrete observed problem motivates building it now.
  Comments got its rework because of an observed verbosity/cross-referencing problem;
  testing's determinism section addressed a real, demonstrated gap;
  this would be anticipatory completeness, which `code-quality`'s own YAGNI rule argues against.
- Rejected: building it now on completeness grounds — proposed, briefly reversed when the first objection to it was identified as too weak to stand alone, then reconsidered a final time against the YAGNI test and dropped again.

### Always-active mechanism: SessionStart hook, once per session

- Decision: two mechanisms cover "always-active," for two distinct contexts.
  Inside any lyx-generated prompt, an explicit "Load these skills: ..." line (a separate, later roadmap item, still the plan for that context).
  For every other session with the plugin installed, `plugins/scribe/hooks/hooks.json` ships a `SessionStart` hook injecting a load-`prose`/`conversation` reminder once, at session start — not a `UserPromptSubmit` hook re-injecting it every turn.
- Rationale: a review round flagged that "always-active" had no mechanism behind it at all, plus a dangling pointer to a mill-transient file that wouldn't travel with the plugin;
  a `UserPromptSubmit` hook was shipped first to close that gap.
  The operator then questioned whether per-turn latency was worth it, for a benefit `prose`'s own broad `description` field might already get through Claude Code's normal relevance-matching most of the time.
  Confirmed by research that `SessionStart` fires reliably on both fresh and resumed sessions, injects its command's stdout into context the same way `UserPromptSubmit` does, and carries no documented platform caveats — a strictly better cost/benefit trade for a one-time session-level nudge than firing on every turn.
- Rejected: `UserPromptSubmit` (the first-shipped version) — unnecessary per-turn cost for a reminder that only needs to land once per session;
  leaving "always-active" as a bare, mechanism-less claim (the original gap); dropping the hook file entirely and relying on relevance-matching alone — rejected because the reviewed gap was specifically the absence of any concrete mechanism, and `SessionStart` closes it at negligible cost.

### Hook shell portability

- Decision: the hook command is a single-quoted `echo`, POSIX-style, unqualified for platform.
- Rationale: `internal/shuttleengine/claudeengine/settings.go`'s own doc comment states Claude Code hook commands run under git-bash on Windows in this environment, which is POSIX-compatible — the same quoting style lyx's own generated hooks already rely on.
  No separate Windows-specific command variant is needed.
- Rejected: writing a cross-shell-safe or PowerShell-specific variant — unnecessary given the confirmed git-bash execution environment.

### Hook discovery: declared explicitly, firing itself still unverified

- Decision: `plugin.json` now declares `"hooks": "./hooks/hooks.json"` explicitly, rather than relying on directory-convention auto-discovery alone.
- Rationale: a review round noted the structural check only confirmed `hooks.json` parses as JSON, not that Claude Code actually discovers or fires it, and that `plugin.json` named no `hooks` key while this repo's own precedent (`prowler`) ships no hook at all to compare against.
  Explicit declaration mirrors how this repo's own `mill` plugin declares its `agents` list explicitly rather than trusting directory convention alone — the safer pattern where one exists.
- Rejected: leaving discovery to directory convention only — plausible per research, but unconfirmed in this specific environment, and a one-line declaration removes the ambiguity at no cost.
  Actually verifying the hook fires (installing the plugin, starting a session, confirming the injected text lands) is still not done — see Testing;
  that requires `/plugin install`, which stays a manual step outside this task.

### Portable skills don't carry repo-specific jargon

- Decision: `conversation`'s file-writing bullet no longer names "lyx" by name — reworded to "a task orchestrator's status/plan/discussion files, if the project has one."
- Rationale: a review round pointed out this skill ships to other repos, where "lyx" means nothing — the same portability argument the design-doc decision already rests on, just not yet applied to this bullet.
- Rejected: leaving it as a loomyard-specific reference — inconsistent with every other portability call made elsewhere in this task.

### Design doc points to the skill, not the reverse

- Decision: `code-comment-conventions.md`'s operative rule text (the rule, its two exceptions, the information triage) is no longer duplicated in the design doc — it points to `code-quality`'s Comments section as the canonical text.
  The design doc keeps only content a skill shouldn't carry: the deeper rationale for why the rule is stronger than a staleness rule, and the not-yet-built Quarry query mechanism.
- Rationale: the design doc lives under `manifest/designs/` in this repo only and won't exist at all in another repo that installs the `scribe` plugin — making it the canonical source would leave the skill non-self-contained everywhere else it ships, since a skill needs to carry its own operative rule text to function on its own.
  A review round also cited `CONSTRAINTS.md`'s Producer Pointer-Rule Invariant against the duplication;
  a later round corrected that citation — the invariant's own text exempts "design docs restating the rule for a human reader," which plausibly covers this document, so the invariant doesn't cleanly mandate the outcome.
  The portability argument stands on its own regardless, and is what this decision actually rests on.
  A later review round caught that the document's own header still cited the retracted invariant while its body restated the rule anyway — a straight self-contradiction, not just a stale citation;
  the header now says only that the document is rationale, not a rule copy, with no invariant name attached.
- Rejected: making the design doc canonical and the skill a bare pointer — would break the skill's portability, independent of whether the cited invariant strictly applies.

## Technical context

- Plugin lives at `plugins/scribe/`, mirroring `plugins/prowler/`'s shape: `.claude-plugin/plugin.json`, `settings.json` (granting `Skill(scribe:*)`), `hooks/hooks.json` (a `SessionStart` hook for the always-active mechanism — see Decisions), `skills/<name>/SKILL.md` per skill, `skills/INDEX.md`.
- Registered in `.claude-plugin/marketplace.json` alongside `prowler`.
- `update-plugins.sh`'s report — `Skipped (not installed): scribe@loomyard -- run '/plugin install scribe@loomyard' first` — proves only that `marketplace.json` parses and names `scribe`;
  the script's not-installed branch returns before it ever touches `plugins/scribe/`'s own contents, so it validates nothing about the plugin's shape.
  A separate, real structural check was run instead (see Scope's "Already in the worktree") and passed;
  installing the plugin into a live Claude Code environment (`/plugin install scribe@loomyard`) remains a manual operator step outside this task.
- Cross-references between skills: `golang-comments` → `code-quality`'s Comments section;
  `golang-testing` → `testing` (a same-plugin reference);
  `conversation` → `prose`;
  `code-quality`'s Comments section → `prose` for writing style.
- A separate, unrelated future plugin — "everything needed to use loomyard" (a compiled `lyx` binary, stencils, a handful of skills) — was discussed in passing as a different, not-yet-built plugin, naming leaning toward `lyxsmith`/`lyxkit` but not finalized.
  It is out of scope here; don't conflate it with `scribe`.
- `manifest/designs/code-comment-conventions.md` now points at `code-quality`'s Comments section as the canonical rule text rather than duplicating it — see Decisions.
- No producer-stencil wiring exists yet — "load these skills" instructions in loom's own prompts are a separate, later roadmap item, out of scope here.

## Constraints

CONSTRAINTS.md's cross-cutting invariants (Cwd Resolution, gitkit Leaf, hubforge Fabric-Fixture, CLI/Cobra, Documentation Lifecycle) don't apply to this task — it adds Claude Code skill/plugin content, not a Go module or CLI command, and touches nothing under `internal/` or `cmd/`.
No new CONSTRAINTS.md entry is needed.

## Testing

Two different senses, since the deliverable is mostly prose/markdown skill content, not executable code.

**Content review:** each skill file read against its own stated rules (does `code-quality.md` avoid padding by its own standard?
does `golang-comments.md` avoid restating `code-quality`?).
Done iteratively during discussion, including two dedicated trim passes on `golang-comments` and this task's own discussion-review rounds, which caught the file-header logic bug, the always-active gap, a self-refuting rule in `prose` itself (it banned "any" as an empty intensifier while using "any" load-bearingly elsewhere in the same file — fixed by dropping it from the list), and the other findings resolved in Decisions above.

**Structural verification:** a real check (not just `update-plugins.sh`'s marketplace-JSON parse) confirming `plugin.json`/`settings.json`/`hooks.json` all parse, every skill's frontmatter `name` matches its directory, `INDEX.md` lists all seven skills, and `marketplace.json`'s `scribe` entry points at the right source path.
Passed at time of writing;
no persisted test script was added for this — it was a one-time check, not a mechanical gate this task decided the repo needs going forward.

**Explicitly not verified:** that the `SessionStart` hook actually fires and its text actually lands in context.
The structural check only confirms `hooks.json` is well-formed JSON;
confirming delivery requires `/plugin install scribe@loomyard` and a real session, which stays a manual step outside this task (see Scope Out).
This is accepted, not overlooked.

## Review guidance

For whoever reviews this discussion and the linked skill files (`plugins/scribe/skills/*/SKILL.md`):

- Focus on: can anything be shortened further, is anything redundant — within one file, or duplicated across two files — and is any worked example pulling its weight relative to its size.
- These are loomyard's own skills, evaluated on their own merits.
- You don't have access to why each decision above was made beyond what's written here.
  If something reads as a real design gap rather than a wording or length issue, flag it as a question rather than assuming the omission was accidental.

## Q&A log

- **Q:** Should implementation (the plugin scaffold, marketplace entry) happen before or after this discussion.md is written, and does this task hand off to a separate plan-writing phase afterward?
  **A:** Implementation first, discussion.md after — explicit operator instruction, the reverse of the usual order where discussion normally precedes and gates implementation.
  No separate plan-writing/execution phase either: the operator asked for everything to be written directly following the review rounds, so this task concludes from the review loop itself rather than handing off.
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
- **Q:** Is the always-active hook worth its per-turn latency cost?
  **A:** Not at the `UserPromptSubmit` cost — switched to `SessionStart`, confirmed to fire reliably once per session (fresh or resumed) rather than on every turn, closing the mechanism gap at a cost the operator found acceptable.
