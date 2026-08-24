# Batch: loomengine seam and stencil rewrite

```yaml
task: 'loom: Discussion-Write producer'
batch: 'loomengine seam and stencil rewrite'
number: 1
cards: 5
verify: go test ./internal/loomengine/...
depends-on: []
```

## Batch Scope

This batch delivers everything `internal/loomengine` and the discussion stencil owe the rest of the task, and nothing else.
It adds `DiscussionDirRel()`, the anchor-relative counterpart of `DiscussionDir` that batch 3's commit closure uses as its pathspec;
it corrects the autonomous `modeRules` text, which today names a `lyx loom run --auto` flag that does not exist;
it corrects three stale comments (`discussion.go`'s header, and both timeout comments in `template.yaml`);
and it rewrites `contracts/stencils/loom/loom-template-discussion.md` to fold in the exploration bound, the bounded architecture category, the `scribe` skill-load step, and the closing self-check step.

The external interface batch 3 consumes is exactly one new exported symbol: `loomengine.DiscussionDirRel() string`.
Nothing else in this batch is read by a later batch.

This batch is independent of batch 2 and can run in parallel with it.
It changes no exported signature: `DiscussionSpec` keeps its `autonomous bool` parameter and both `modeRules` branches per the `autonomous-only` Shared Decision, so no existing caller or test outside `internal/loomengine` is affected.

Batch-local decision, differing from nothing in the overview: the stencil rewrite keeps everything the `loom-format-discussion.md` design doc explicitly does *not* supersede — Step 3's mode-neutral interview framing, Step 2's "before asking the operator anything" instruction, Step 1's board read, Step 5's section shapes, and the trailing `AskUserQuestion` prohibition — verbatim.
All mode-specific wording stays confined to the `{{.mode_rules}}` marker at Step 4, which is what makes the interactive follow-up item a one-argument flip.

## Cards

### Card 1: Add `loomengine.DiscussionDirRel`

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/discussionpath_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported `DiscussionDirRel() string` to `internal/loomengine/config.go`, returning `filepath.Join(lyxdirs.LyxDirName, discussionDirName)`.
  Place it immediately before `DiscussionDir`, and model its doc comment on the existing `LoomStatusRel`: state that it returns the worktree-anchor-relative form of `DiscussionDir`'s path and that it exists so a caller building a fabric commit pathspec never has to name a directory segment `loomengine` owns.
  Then refactor `DiscussionDir` to `filepath.Join(l.AnchorPath(), DiscussionDirRel())`, exactly as `LoomStatusFile` already composes `LoomStatusRel()`, so the two declarations cannot drift.
  Leave `DiscussionDecisionRecord` and `DiscussionSupportLog` unchanged — they already derive from `DiscussionDir`.
  In `internal/loomengine/discussionpath_test.go`, add a `TestDiscussionDirRel` that pins the returned value against `DiscussionDir` for a hand-built `lyxcwd.Location` whose `AnchorRel` differs from `"."`, asserting `DiscussionDir(l) == filepath.Join(l.AnchorPath(), DiscussionDirRel())`, and separately that `DiscussionDirRel()` is not an absolute path per `filepath.IsAbs`.
- **Commit:** `feat(loomengine): add DiscussionDirRel, the anchor-relative discussion dir`

### Card 2: Correct the autonomous `modeRules` text and `discussion.go`'s header comment

- **Context:**
  - `internal/loomcli/run.go`
- **Edits:**
  - `internal/loomengine/prompt.go`
  - `internal/loomengine/discussion.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomengine/prompt.go`, rewrite the autonomous branch of `modeRules` so it no longer contains the substring `--auto`.
  The current text opens "This session is running in autonomous (`--auto`) mode"; `lyx loom run` has no such flag, so replace that opening with a plain statement that the session is autonomous and that no operator will answer questions.
  Keep the rest of the branch's obligations intact and unreworded: make a best-judgment choice at every decision point, never block waiting for input, never call the `AskUserQuestion` tool, and record each self-made pick and its rationale in the support log's `## Question ledger`, marked as an auto-pick.
  Do not touch the interactive branch of `modeRules` and do not change `modeRules`' signature.
  In `internal/loomengine/discussion.go`, correct the header comment's last sentence, which currently reads "the future loom phase machine drives the returned Spec through shuttle.Run and reacts to its outcome": the phase machine is no longer in the future.
  State instead that `shedadapters.SingleLLMProducer` drives the returned Spec through the shuttle seam, reached from `internal/shedrecipe`'s `DiscussionWrite` registry entry.
  Change nothing else in `discussion.go` — `DiscussionSpec`'s body and signature are untouched by this task.
- **Commit:** `fix(loomengine): drop the nonexistent --auto flag from the autonomous mode rules`

### Card 3: Correct both timeout comments in loom's config template

- **Context:**
  - `internal/loomengine/configtemplate.go`
- **Edits:**
  - `internal/loomengine/template.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomengine/template.yaml`, keep both values unchanged — `discussion_timeout_min` stays `480` and `plan_timeout_min` stays `120` — and correct only their trailing comments.
  `discussion_timeout_min`'s comment currently ends "(interactive interviews run long)", which stops being true once the discussion run is autonomous; replace the parenthetical with one stating that an autonomous agent exploring a codebase and writing two files can legitimately run long, and that the value is a deadline rather than a wait.
  `plan_timeout_min`'s comment currently ends "(autonomous, shorter than the interview)", whose contrast no longer holds for the same reason; replace it with a plain statement that the plan run gets a shorter ceiling than the discussion run.
  Do not touch the `discussion:` or `plan:` model-spec lines.
- **Commit:** `docs(loomengine): correct both timeout comments in the loom config template`

### Card 4: Rewrite the discussion stencil

- **Context:**
  - `manifest/designs/loom-format-discussion.md`
  - `plugins/scribe/skills/INDEX.md`
  - `contracts/stencils/stencils.go`
  - `internal/loomengine/prompt.go`
  - `internal/stencil/stencil.go`
  - `internal/discussionparser/validate.go`
  - `internal/loomcli/validate.go`
- **Edits:**
  - `contracts/stencils/loom/loom-template-discussion.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `contracts/stencils/loom/loom-template-discussion.md` with exactly the six changes below, leaving every other line byte-identical.
  The file must keep all four `{{.slug}}`, `{{.mode_rules}}`, `{{.decision_record_path}}`, `{{.support_log_path}}` markers, add no fifth marker, and contain no `{{if}}` or `{{range}}` conditional — a required marker inside a conditional branch renders silently blank.
  (1) Rewrite the leading HTML comment's first paragraph to describe the real call path now that the producer is wired: the file ships as an embedded default in `contracts/stencils/stencils.go`, is seeded to the hub's stencils directory, is read from there at call time by `composePrompt` in `internal/loomengine/prompt.go` via `internal/stencil`, is wrapped into a `shuttleengine.Spec` by `loomengine.DiscussionSpec`, and is driven through the shuttle seam by `shedadapters.SingleLLMProducer` as recipe row 3.
  Keep the comment's marker paragraph and its JSON-punctuation paragraph verbatim.
  (2) Insert a new `## Step 0 — Load the writing skills` section between the H1 intro and `## Step 1`, instructing the agent to load `scribe:prose` first and then `scribe:conversation`, stating the order matters because `scribe:conversation` builds on `scribe:prose`, and stating explicitly that both are best-effort: if a skill is unavailable, continue without it rather than treating an unresolvable skill name as an error.
  Name those two skills only.
  (3) In `## Step 2 — Explore before asking`, keep the three existing lines verbatim and append the exploration bound from `loom-format-discussion.md`'s Fix 1, stated from Step 2's own angle (what to explore): at a coarse level you MAY establish which module boundary the work falls under and whether the design conflicts with an existing pattern;
  you MUST NOT gather exact signatures, `file:line` citations, interface shapes, or dependency lists, and you MUST NOT do exhaustive existing-pattern research, because that class of fact is computed fresh at Plan time.
  (4) In `## Step 3 — Conduct the interview`, replace only the `**Architecture** — modules, interfaces, dependencies.` bullet with a bounded coarse-level category carrying the same positive/negative pair from Step 3's angle (what to ask): MAY ask about module boundary and pattern conflict;
  MUST NOT ask the operator to enumerate signatures, `file:line` citations, interface shapes, or dependency lists.
  Leave the other five bullets (Scope, Constraints, Edge cases, Security, Testing) and every line of the surrounding framing untouched — "Interview relentlessly", the recommended-answer rule, the 2–3-approaches rule, "Challenge the problem itself", "Design the full scope now", and the YAGNI rule all stay verbatim.
  (5) Insert a new `## Step 6 — Self-check before ending your turn` section after `## Step 5`'s two file subsections and before the trailing `## Never use AskUserQuestion` section, instructing the agent to run `lyx loom validate-discussion`, stating that the verb takes no arguments, exits 0 on a clean gate and 1 otherwise, and puts its findings under the failure envelope's `findings` key, and instructing the agent to fix what it reports and re-run until it exits 0 before ending its turn.
  (6) Leave `## Step 1`, `## Step 4`, `## Step 5`'s seven-heading enumeration and its compaction rules, the support log's four-heading enumeration, and the trailing `## Never use AskUserQuestion` section entirely unchanged.
  The seven required H2 headings stay enumerated in Step 5: that is the output-shape carve-out to the Producer Pointer-Rule Invariant, not a restatement of `Discussion-Validate`'s checklist, and the plan's blind-bounce reasoning depends on the enumeration being present.
  Apply semantic line breaks throughout the file per the `markdown-semantic-line-breaks` Shared Decision.
- **Commit:** `feat(stencils): rewrite the discussion stencil for the redesigned Discussion format`

### Card 5: Extend the loomengine prompt tests for the rewritten stencil

- **Context:**
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/stencils/stencils.go`
  - `internal/loomengine/discussion_test.go`
- **Edits:**
  - `internal/loomengine/prompt_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `internal/loomengine/prompt_test.go` with assertions covering the card 2 and card 4 changes, keeping the file's existing stable-substring style rather than golden-file equality.
  Add to `TestComposePrompt_RendersMarkers` (or as sibling tests in the same file) three checks over the rendered autonomous output: that it contains the `scribe:prose` and `scribe:conversation` skill names from Step 0, that it contains the `lyx loom validate-discussion` self-check command from Step 6, and that the exploration bound survives the fill — assert on a stable substring of the MUST NOT clause rather than a whole sentence.
  Add a check that the rendered autonomous output does not contain the substring `--auto`, and a matching check on `modeRules(true)` itself in `TestModeRules`.
  Leave the existing marker, path, slug, board-read, and mode-difference assertions in place unchanged, and leave `newTestStencilsDir` unchanged — it already copies the shipped embedded default, so it picks up card 4's rewrite automatically.
- **Commit:** `test(loomengine): pin the rewritten stencil's Step 0, exploration bound, and self-check`

## Batch Tests

`verify: go test ./internal/loomengine/...` runs the whole `internal/loomengine` package, which is the only package this batch's production edits touch.
It covers `discussionpath_test.go` (card 1's `DiscussionDirRel` pin and the existing `DiscussionDir`/`DiscussionDecisionRecord`/`DiscussionSupportLog` tests, which card 1's refactor of `DiscussionDir` must leave passing), `prompt_test.go` (card 5's new assertions plus the existing marker and mode-difference coverage), `discussion_test.go` (which fills the rewritten stencil through the real `DiscussionSpec` for both `autonomous` values, so card 4's rewrite proves it still fills all four markers non-empty), and `config_test.go` (which loads the template card 3 edits).
The stencil rewrite has no runnable surface of its own — it is exercised only through `composePrompt` and `DiscussionSpec`, both in this package.
Every test in this batch is hermetic and untagged, seeding its stencils directory from the embedded defaults into a `t.TempDir()` and building `lyxcwd.Location` values by hand.
