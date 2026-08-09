# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix rm <file>`.

### From discussion.md

# Discussion: plan-format: drop the v3 suffix and sweep every reference by script

```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
slug: plan-format-drop-v3-suffix
status: discussing
parent: main
```

## Problem

`docs/reference/plan-format-v3.md` is the pinned contract for the only plan format lyx has.
Its predecessor v2 was deleted by task A (`builder-retire`, commit `0149776a`) along with `builder`, its sole consumer, and v1 was already gone before that.
A version suffix on the sole surviving format is exactly the stale guard this repo argues against elsewhere — `discussion-format.md`'s `no-schema-version` reference to `status-schema.md` makes the same case.

**Why now:** task A freed the `plan-format.md` filename by deleting v2's doc, and left a deliberate window in which `docs/reference/plan-format.md` does not exist at all — two links currently dangle into it (`manifest/designs/loom.md:29` and `docs/reference/plan-format-v3.md:5`).
This task closes that window by renaming v3 into the freed name.
Two tasks are blocked behind it: `format-docs-name-producers` (C) and `batcher-standalone-split` (F), both of which edit the renamed file.

A half-done rename is worse than either end state, so completion is judged by a zero-hit repo grep rather than by any file count agreed in advance.

The operator additionally decided during this discussion that **every mention of plan-format v2 is removed repo-wide**, not merely the ones this task's rename would corrupt.
v2 and v1 are out of use; naming them anywhere is confusing and costs tokens for no benefit.
This is a deliberate expansion beyond the manifest's "paths and names only, never prose" scope rule for this task — see Decisions.

The authoritative task specification is `manifest/designs/shed-followups.md`, section **B — plan-format-drop-v3-suffix** (lines 156–233).
Several of its instructions are overridden by this discussion; every override is recorded below and must also be written back into that file (see `shed-followups-override-notes`).

## Scope

**In:**

- `git mv docs/reference/plan-format-v3.md docs/reference/plan-format.md`, as its own step, so git records a rename.
- A scripted find/replace over a **six**-pattern set (the manifest's five plus `plan format v3`), executed by a temporary Go program, never a hand-edit pass and never `sed`.
- Every resulting content edit across 30 files (see **Technical context** for the per-file inventory).
- Removal of every **plan-format-v2** mention repo-wide, including the nine by-contrast lines inside the renamed doc and the "**Dropped from v2, and why:**" subsection.
- Renaming the two v2-named test guards to name the concepts they actually assert, and dropping the now-meaningless `"v2"` token from `internal/webstercli/cli_test.go:100`'s `forbidden` list.
- The four surviving bare-`v3` format labels in `internal/planparser` comments (see `bare-v3-label-in-scope`).
- Hand-written override notes appended to `manifest/designs/shed-followups.md`, outside the scripted sweep.
- `go build ./...` and `go test ./...` green.

**Out:**

- **Every `v1`/`V1` in the tree.** All of them belong to unrelated vocabularies — scout V1, reed v1, the shuttle v1 engine, crucible V1, `hn.algolia.com/api/v1`. There is no `plan-format v1` reference anywhere; the class is empty. Do not touch them.
- **`gopkg.in/yaml.v3`** — 32 Go files import it, including `internal/planparser/parse.go:21`, the file this task is most certain to edit. Hard exclusion. Also present at `CONSTRAINTS.md:66` and `:77`.
- **The `format: 3` frontmatter key** in `00-overview.md` and everything that validates it (`internal/planparser`'s `format-unsupported` check, the worked example, `tools/sandbox/SANDBOX-WEBSTER-SUITE.md:44`). This is the plan schema's own version field, not the document's filename. Renaming the doc does not renumber the schema.
- **`internal/planparser`'s API.** No exported or unexported Go identifier anywhere in the repo contains `V3` — verified by `grep -E '\b\w*[Vv]3\w*\b' --include='*.go'`, whose every hit outside `yaml.v3` falls inside a comment. There is no code rename to perform.
- **`CONSTRAINTS.md`.** Its only `v3` hits are the two `gopkg.in/yaml.v3` import-allowlist lines. See `constraints-needs-no-change`.
- **`manifest/designs/loom.md` beyond the pattern sweep plus `:29`.** Rows 2–7 of the producer table are task C's; everything else in the file is task E's.
- **The concept assertions in the two test guards** (`oversized`, `chain`, `## Scope`, `out_of_scope:`, `tests: green`). Only their names, comments, and the literal `"v2"` token change.
- **Task C's and task F's work** on the renamed file. This task does not rewrite `plan-format.md:5`'s framing beyond removing the v2 claim, does not add producer-model vocabulary, and does not touch the "Batch is gone / the card is the unit" section.

## Decisions

### rename-is-a-separate-git-mv

- Decision: `git mv docs/reference/plan-format-v3.md docs/reference/plan-format.md` runs as its own step before the content sweep.
- Rationale: git records a rename rather than a delete-plus-add, so `git log --follow` and review diffs stay readable across the boundary.
- Rejected: letting the sweep script perform the rename inline — the script would write a new file and unlink the old one, which git reports as an unrelated add/delete pair.

### six-pattern-set

- Decision: the sweep and the completion grep both use six case-insensitive patterns, not the manifest's five: `plan-format-v3`, `plan_format_v3`, `plan-format v3`, `plan format v3`, `plan-v3`, `Plan-format v3`.
- Rationale: `docs/reference/plan-format-v3.md:1` is `# Plan format v3 — flat card list` — a **space** between "Plan" and "format", so `plan-format v3` does not match it. Under the manifest's five-pattern set the zero-hit grep passes with the renamed doc still titled "v3", which defeats the task. `Plan-format v3` is redundant under case-insensitivity but is kept because the manifest names it and its presence costs nothing.
- Rejected: leaving the title to task C — C's brief never mentions the title, so it would survive. Leaving the title permanently — it is the single most visible "v3" in the repo.

Replacement mapping (case-preserving on the leading letter):

| pattern | replacement |
| --- | --- |
| `plan-format-v3` | `plan-format` |
| `plan_format_v3` | `plan_format` |
| `plan-format v3` | `plan-format` |
| `plan format v3` | `plan format` |
| `plan-v3` | `plan` |

`plan-format-v3.md` therefore becomes `plan-format.md` in every link and prose reference, and `plan-v3's card contract` becomes `plan's card contract`.

### exclusion-set

- Decision: three paths are excluded from **both** the sweep and the completion grep — `manifest/designs/shed-followups.md`, `manifest/roadmap.md:18`, and `_mill/`. The grep gate is defined as zero hits *outside* this set, and the set is part of the acceptance criterion rather than a caveat on it.
- Rationale: each of these carries the pattern as quoted spec text that a blind replacement destroys.
  - `shed-followups.md:228` **is** the sentence "The pattern set is: `plan-format-v3`, `plan_format_v3`, `plan-format v3`, `plan-v3`, and `Plan-format v3`." Sweeping it yields "`plan-format`, `plan_format`, `plan-format `, `plan`" — this task's own acceptance criterion, destroyed. `:74` holds a genuine `plan-format.md` / `plan-format-v3.md` pair that collapses to a self-duplicate, and `:53`/`:120`/`:192`/`:214`/`:230` are descriptions *of* this rename. The file is a versioned historical spec; it should keep saying what it said.
  - `manifest/roadmap.md:18` reads "mechanical rename sweep, `plan-format-v3.md` → `plan-format.md`". Sweeping yields "`plan-format.md` → `plan-format.md`". Only line 18 is excluded; the file's other five hits are swept normally.
  - `_mill/` holds this very file, which quotes the patterns literally, plus `status.md`. It is task-state, torn down on merge, and never part of the repo's prose.
- Rejected: sweeping everything and hand-repairing afterwards — violates the "never a hand-edit pass" discipline the task exists to honour. Excluding only `_mill/` — nobody owns `shed-followups.md` downstream, so the corruption would be permanent.

**Accepted consequence: `shed-followups.md` keeps stale references.** Excluding it leaves `:53`, `:74`, `:192` and `:214` citing `docs/reference/plan-format-v3.md`, a path that will not exist after this task. This is accepted, not overlooked. The file is a historical record of what each task was told to do at scoping time, and those four citations are accurate *as of that moment* — rewriting them would make the record claim the scoping task knew the post-rename name. The override notes (`shed-followups-override-notes`) are where a reader learns the file moved. Acceptance gate 8's link-check therefore covers links **touched by the sweep** only, and explicitly exempts this file; do not file its stale citations as a regression.

### v2-is-erased-repo-wide

- Decision: every reference to plan-format **v2** is removed from the repo, not merely the ones this rename would break.
- Rationale: operator decision. v2 (and v1 before it) is out of use; naming a format that no longer exists is confusing and buys nothing. This supersedes `shed-followups.md:216`'s "This task changes paths and names only, never prose" and takes `plan-format-v3.md:5` from task C and `manifest/designs/loom.md:29` from task E.
- Rejected: fixing only `plan-format-v3.md:5`'s self-link and leaving the by-contrast prose — leaves nine sentences describing a format the reader cannot find. Leaving `loom.md:29` self-contradicting for task E as the manifest directs — the operator overrode this; see `loom-29-in-scope`.

The complete v2 site list is in **Technical context** under *v2 erasure inventory*.

### bare-v3-label-in-scope

- Decision: the four surviving bare-`v3` format labels in `internal/planparser` comments are rewritten too — `validate.go:11`, `validate_test.go:240`, `parse_test.go:36`, `parse_test.go:53`. Each drops the version word and names the format directly.
- Rationale: `shed-followups.md:169` explicitly rejects "renaming the file but keeping in-text `v3` as a historical label — the suffix is exactly what is being retired". Declaring that class out of scope contradicts the task's own spec. The original out-of-scope call rested on "this task changes paths and names, not prose", a premise `v2-is-erased-repo-wide` has already discarded — that decision rewrites nine prose lines in the renamed doc. With only one plan format left, "v3" as a label denotes nothing.
- Rejected: overriding `:169` and keeping the labels — would leave `parse_test.go:36`'s "a syntactically complete v3 overview" describing a format whose doc no longer names a version. Sweeping every bare `v3` mechanically — the four sites need their sentences rewritten, not a token replaced.

The exact sites and their problem:

| site | current text | issue |
| --- | --- | --- |
| `internal/planparser/validate.go:11` | "not batch — v3 has no batch concept —" | also a v2-era contrast; fold into `v2-is-erased-repo-wide` |
| `internal/planparser/validate_test.go:240` | "the plan-wide scope v3 uses since batch is gone" | version label with no referent |
| `internal/planparser/parse_test.go:36` | "a syntactically complete v3 overview" | version label with no referent |
| `internal/planparser/parse_test.go:53` | "a syntactically complete v3 card file body" | version label with no referent |

**The renamed doc needs no separate bare-`v3` pass.** Every `v3` in its body is already reached by another decision: `:1`, `:3`, `:221` are pattern hits swept by `six-pattern-set`; `:5` is deleted by `plan-format-5-in-scope`; `:28`, `:69`, `:196`, `:212–213` are rewritten by `v2-is-erased-repo-wide`. After both passes the file contains no `v3` at all — verify this rather than assuming it.

### loom-29-in-scope

- Decision: `manifest/designs/loom.md:29` is rewritten by this task, not left self-contradicting for task E.
- Rationale: the line currently reads "today's pinned [plan-format.md v2](../../docs/reference/plan-format.md) (batch-based) is being replaced by [plan-format v3](../../docs/reference/plan-format-v3.md) (a flat card list)". After the sweep both links point at the same file and the sentence claims a replacement that already happened. The manifest assigned this to task E precisely because a find/replace cannot repair it — but under `v2-is-erased-repo-wide` the v2 half must go regardless, and repairing half a sentence is worse than repairing all of it.
- Rejected: the manifest's original chain-order assignment (`shed-followups.md:209–210`, `:406`) — overridden by the operator. Task E remains `loom.md`'s final owner for everything else; only `:29` moves.

Target wording: state that the pinned plan format is `plan-format.md`, a flat card list, and point at `internal/websterengine`'s package documentation for the consumer — with no reference to a predecessor and no "is changing" framing.

### plan-format-5-in-scope

- Decision: `docs/reference/plan-format-v3.md:5` — the blockquote `> **v3 is the live plan format.** [plan-format.md v2](plan-format.md) is retired now that builder, its sole consumer, is gone. v3 — consumed by webster — is the sole plan format.` — is deleted outright by this task, **together with `:4`**.
- `:4` is a bare `>` separating the two blockquote paragraphs. Deleting only `:5` leaves it as a trailing empty quote line. Delete `:4` and `:5` as a pair, leaving `:3`'s Status paragraph as the sole blockquote.
- Rationale: the line contains no pattern hit, so the sweep leaves it untouched; after the rename its link points at the file itself. This task creates that defect, so this task fixes it. The manifest assigned `:5` to task C as the "Coexistence, not replacement" section, but task A already rewrote that section away — what remains is the self-link, which is ours.
- Rejected: leaving it as a recorded knowingly-broken site — a doc linking to itself as its own retired predecessor is not a defensible interim state.

### roadmap-203-in-scope

- Decision: `manifest/roadmap.md:203` ("v3 is the live plan format now that its predecessor is retired.") is deleted, and the Done item's `:201` heading is swept normally.
- Rationale: after the sweep `:201` reads "**plan-format: flat card list**", making `:203`'s "v3 is the live plan format" incoherent inside an item no longer titled v3; and "its predecessor is retired" is a v2 reference without the token. Both problems are created by this task's own edit, which is the same "owns every site whose claim it itself falsifies" rule the manifest applies to task A at `shed-followups.md:119`. This is the sole exception to the bare-`v3`-is-out-of-scope rule.
- Rejected: leaving it for task E as `roadmap.md`'s last owner — E would inherit a sentence this task broke.

### v2-guards-keep-assertions-lose-the-label

- Decision: keep every concept assertion in the two v2-named test guards; rename the tests and rewrite their comments to name the concepts rather than the version; drop the literal `"v2"` string from `internal/webstercli/cli_test.go:100`'s `forbidden` slice.
- Rationale: `TestTemplates_NoV2TokensRemain` (`internal/websterengine/template_test.go:510`) never asserts on `"v2"` at all — it asserts `oversized`, `chain`, `## Scope`. `TestForkTemplate_PinsReportSchemaKeys` (`:398`) asserts `out_of_scope:` and `tests: green`. In both, "v2" appears only in the name and comments. Those assertions guard concrete banned constructs in LLM-authored prompt templates — files agents edit — and stand on their own with no reference to a version number. The one genuinely asserted `"v2"` token, in `cli_test.go:100`, guards against reintroducing a label whose referent no longer exists, which is precisely the stale guard this task's rationale rejects.
- Rejected: keeping the `"v2"` token (no case for it once v2 is erased); deleting the guards entirely (removes the only machine check stopping batch-era language from creeping back into CLI help and the embedded templates).

Renames: `TestTemplates_NoV2TokensRemain` → `TestTemplates_NoDroppedBatchConceptsRemain`; `TestCommand_LongStringsHaveNoStaleV2Language` → `TestCommand_LongStringsHaveNoStaleBatchLanguage`. `internal/webstercli/cli_test.go:106`'s error message and `internal/websterengine/template_test.go:72–77`'s `requireNotContains` doc comment lose their "v2" wording too.

### temporary-go-sweeper-in-scratch

- Decision: the sweep is performed by a temporary Go program at `.scratch/sweep/main.go`, run via `go run .scratch/sweep/main.go`, and never committed.
- Rationale: operator chose Go over Python, and temporary over permanent. Verified during exploration: both `go run .scratch/sweep/main.go` and `go run ./.scratch/sweep` execute correctly; `go build ./...` does **not** pick the file up (the go tool skips dot-prefixed directories during pattern matching); and `.gitignore:19`'s `**/.scratch/` entry means it cannot reach a commit. So it neither pollutes the build nor lands in history. `sed` is banned by this repo's tooling rules (`CLAUDE.md`, `mill:conversation`).
- Rejected: a committed Go program under `tools/` — permanent maintenance cost for a one-shot rename, and an invitation to re-run it against a repo where the patterns are already gone. A Python script — operator preference.

Note: a probe file from exploration may already exist at `.scratch/sweep/main.go` containing a `fmt.Println("sweep-probe-ok")` stub. Overwrite it.

### shed-followups-override-notes

- Decision: hand-write override notes into `manifest/designs/shed-followups.md`, outside the scripted sweep, recording every decision above that contradicts it.
- Rationale: three of that file's explicit instructions are overridden here, and downstream tasks C and E read it as their spec. Without notes they would either redo work already done or look for a knowingly-broken site that no longer exists. The file already carries four "**Override recorded 2026-08-09 (task A, as landed)**" blocks as precedent for exactly this.
- Rejected: recording the overrides only in `_mill/discussion.md` — `_mill/` is torn down on merge, so C and E would never see them.

Required notes:

1. **In section B** (after `:216`, the "paths and names only, never prose" sentence) — an "**Override recorded 2026-08-09 (task B, as landed)**" block recording every one of B's own instructions this task departed from:
   - **`:228`'s five-pattern set** → six patterns. The five missed `:1`'s space-variant title `# Plan format v3`, so the stated criterion passed with the renamed doc still titled v3.
   - **`:227`'s unqualified "repo grep"** → scoped to an exclusion set (`shed-followups.md`, `roadmap.md:18`, `_mill/`), because `:228` is itself a pattern-bearing line and a blind sweep destroys the criterion it defines. Names the accepted consequence that `:53`, `:74`, `:192`, `:214` keep citing the old path.
   - **`:216`'s "paths and names only, never prose"** → superseded. The repo-wide v2 erasure rewrites prose.
   - **`:169`'s rejected alternative** → honoured rather than overridden, and extended: the four bare-`v3` labels in `internal/planparser` comments are rewritten too. Recorded because `:169` is a *rejection*, not an instruction, and a reader could otherwise think B left the class alone.
   - **`:209–210`'s "deliberately leaves `loom.md:29` self-contradicting"** → no longer holds; B rewrote `:29` in full.
   - **`:188`'s claim that `CONSTRAINTS.md`'s Planparser Sole-Parser Invariant needs rewording** → stale. That invariant (`CONSTRAINTS.md:293–300`) carries no version reference and no link to the doc. B edited nothing there.
   - **`:197`'s "It appears in ten Go files"** → the verified count is **32**. Correct the figure so the next reader is not misled about the blast radius of a broad `v3` replace.
   - **`:199`'s "This task's script names the exclusion explicitly"** → the script names no exclusion. All six patterns require a `plan` prefix, so `gopkg.in/yaml.v3` is unmatchable by construction; the exclusion is verified by a post-sweep count rather than implemented.
2. **In section C** (at `:265–266`, the "Rewrite `plan-format.md:5`'s Coexistence, not replacement section" item) — a note that task A already rewrote that section and task B then deleted the surviving v2 blockquote, so C's obligation there is discharged; C's remaining work on the file is the producer-model rewrite only.
3. **In section E** (at `:406`, the `loom.md:29` bullet) — a note that task B rewrote `:29` in full rather than leaving it self-contradicting, so E should verify rather than repair it; and at Part four's `roadmap.md` list, that task B deleted `:203`.

### constraints-needs-no-change

- Decision: `CONSTRAINTS.md` is not edited.
- Rationale: `shed-followups.md:188` lists "`CONSTRAINTS.md`'s Planparser Sole-Parser Invariant, whose wording changes for the renamed format" in this task's starting inventory. That instruction is stale. The invariant (`CONSTRAINTS.md:293–300`) says "the on-disk plan format (`_lyx/plan/`)" with no version reference and no link to the doc; the file's only `v3` occurrences are the two `gopkg.in/yaml.v3` import-allowlist entries at `:66` and `:77`, which are the hard exclusion. Nothing to change.
- Rejected: editing it anyway to match the inventory — the inventory is explicitly "a starting inventory only" (`:176`), bounded by grep, and grep says no.

### yaml-v3-exclusion-is-structural

- Decision: the `gopkg.in/yaml.v3` exclusion is enforced structurally by the pattern set, and verified by a post-sweep assertion rather than implemented as a script feature.
- Rationale: none of the six patterns can match `gopkg.in/yaml.v3` — every one requires a `plan` prefix. The manifest's warning (`:194–199`) targets a broad bare-`v3` replace, which this task never performs. Still worth an explicit check, because the failure mode it describes is real and silent.
- Rejected: adding a skip-list of the 32 importing files — redundant complexity guarding against a replacement the script does not do.

Verification: `grep -rl 'gopkg.in/yaml.v3' --include='*.go' . | wc -l` must return **32** after the sweep, unchanged.

**The manifest's count is wrong.** `shed-followups.md:197` says the import "appears in ten Go files". The verified figure is 32 — more than three times the stated blast radius. The correction goes into the override notes so the next reader of `:197` is not misled about how much a broad `v3` replace would have destroyed.

## Technical context

### The rename target

`docs/reference/plan-format-v3.md` — 350+ lines, `> **Status: Contract — pinned.**`, the flat card-list schema `internal/planparser` implements and `internal/websterengine` consumes.
Its `## Related` section at `:343` was already re-pointed by task A away from the deleted `builder-contract.md` to `webster-contract.md`, so no dangling anchor remains there.

### Pattern-hit inventory (30 files, sweep scope)

Derived by grep at discussion time, not from any prior count. Re-derive it as the plan's first action — `shed-followups.md:173` makes this explicit and it is the step that bounds the work.

| file | hits |
| --- | --- |
| `manifest/designs/loom.md` | 8 |
| `manifest/roadmap.md` | 6 (one, `:18`, excluded) |
| `manifest/designs/scout-plan-symbol-fields.md` | 5 |
| `internal/planparser/validate.go` | 5 |
| `internal/planparser/parse.go` | 5 |
| `internal/planparser/validate_test.go` | 4 |
| `internal/planparser/normalize.go` | 4 |
| `manifest/designs/webster-parallel-execution.md` | 3 |
| `internal/websterengine/doc.go` | 3 |
| `internal/webstercli/validate.go` | 3 |
| `internal/planparser/plan.go` | 3 |
| `internal/planparser/parse_test.go` | 3 |
| `internal/loomengine/plan-template.md` | 3 |
| `docs/reference/plan-format-v3.md` | 3 (incl. the space-variant title) |
| `docs/overview.md` | 3 |
| `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` | 2 |
| `manifest/designs/review-finding-classification.md` | 2 |
| `internal/planparser/sections.go` | 2 |
| `internal/planparser/doc.go` | 2 |
| `internal/loomengine/plan.go` | 2 |
| `internal/batcher/doc.go` | 2 |
| `docs/reference/webster-contract.md` | 2 |
| `internal/websterengine/runlevel_test.go` | 1 |
| `internal/websterengine/master-template.md` | 1 |
| `internal/websterengine/integration-template.md` | 1 |
| `internal/webstercli/cli_test.go` | 1 |
| `internal/webstercli/cli.go` | 1 |
| `internal/loomengine/plantemplate.go` | 1 |
| `docs/reference/model-spec.md` | 1 |
| `README.md` | 1 |

Excluded from both sweep and grep: `manifest/designs/shed-followups.md` (8 hits), `manifest/roadmap.md:18`, `_mill/`, `.scratch/`.

### Sites needing more than replacement

Seven sites the script cannot finish on its own. Each is a separate, hand-written edit after the sweep:

1. `docs/reference/plan-format.md:1` — swept to `# Plan format — flat card list`. Verify it reads well; no further edit expected.
2. `docs/reference/plan-format.md:4–5` — delete both lines (`plan-format-5-in-scope`).
3. `manifest/designs/loom.md:29` — rewrite (`loom-29-in-scope`).
4. `manifest/roadmap.md:203` — delete the sentence (`roadmap-203-in-scope`).
5. The four bare-`v3` labels in `internal/planparser` (`bare-v3-label-in-scope`).
6. The v2 erasure inventory below.
7. `manifest/designs/shed-followups.md` — the three override-note blocks (`shed-followups-override-notes`).

### v2 erasure inventory

Every surviving plan-format-v2 reference. All are prose or comments; none carry a sweep pattern, so all are hand edits.

**Inside `docs/reference/plan-format-v3.md`** (becomes `plan-format.md`):

- `:4–5` — the bare `>` separator and the retired-v2 blockquote. **Delete both.**
- `:28` — "v2's per-batch `## Scope` concept is **removed entirely** — there is no batch-level 'declared ownership' list in v3." → state the property directly: there is no batch-level declared-ownership list and no `## Scope` section.
- `:69` — "v3 keeps lyx's own established `What:` name, playing the same role it played in v2." → drop the trailing clause.
- `:120` — "(unlike v2, where `root:` was per-batch)" → drop the parenthetical.
- `:126` — "(v2's `scope-malformed`, renamed because Scope is gone)" → drop the parenthetical.
- `:196` — "This check **absorbs** v2's `card-count-mismatch` — v3 has no `(C cards)` segment to cross-check separately" → restate as a property of the check.
- `:198` — "(v2's `scope-malformed`, renamed because Scope is gone.)" → drop the parenthetical.
- `:203` — "(now plan-level, was per-batch in v2)" → "(plan-level)".
- `:212–216` — the whole "**Dropped from v2, and why:**" block (five entries: `verify-missing`, `chain-end-dangling`, `batch-oversized`, `card-outside-scope`, `card-count-mismatch`). It exists solely to describe a delta against a format that no longer exists. **Delete the block.**

**Go comments:**

- `internal/websterengine/classify.go:3` — "The v2 Scope field is dropped entirely — the flat plan format carries no `## Scope`…" → state the second half only.
- `internal/websterengine/template_test.go:75` — `requireNotContains`'s doc comment, "every dropped v2 concept (oversized, chain, ## Scope)".
- `internal/websterengine/template_test.go:398` — `TestForkTemplate_PinsReportSchemaKeys`'s comment, "never the v2 report's tests/stuck_reason/out_of_scope grammar".
- `internal/websterengine/template_test.go:417` — "The v2 report grammar (done/stuck/tests/out_of_scope) must be gone".
- `internal/websterengine/template_test.go:510–511` — `TestTemplates_NoV2TokensRemain`'s name and comment.
- `internal/planparser/parse_test.go:215` — "Unlike the frozen v2 parser, a missing format:/approved: key is not a…".
- `internal/webstercli/cli_test.go:99` — test name; `:100` — the `"v2"` token in `forbidden`; `:106` — the error message's "stale v2/chain/oversized language".
- `internal/planparser/validate.go:11–12` — "not batch — v3 has no batch concept — … the oversized-batch cap dies with batch itself". Carries both a bare `v3` label and a v2-era contrast, so it is reached by `bare-v3-label-in-scope` and this decision at once; rewrite it once, covering both.

**Docs:**

- `manifest/designs/loom.md:29` — already covered by `loom-29-in-scope`.
- `manifest/roadmap.md:203` — already covered by `roadmap-203-in-scope`.

**Deliberately untouched** (unrelated to plan-format): `internal/state/state_test.go:139–152`, `internal/gitrepo/reset_test.go`, `internal/yamlengine/reconcile_test.go:406`, `internal/shuttleengine/claudeengine/command.go:71`, `internal/burlerengine/doc.go:178`, `manifest/designs/fabric-unified-view.md:16,223`, `manifest/designs/webster-parallel-execution.md:17,53`, `docs/research/*`, `manifest/designs/shed-followups.md` (excluded), `docs/reference/plan-format.md`'s own worked example if it contains sample content.

### The sweeper program

`.scratch/sweep/main.go`, `package main`. Shape:

- Walk the repo from the git root; skip `.git/`, `_mill/`, `.scratch/`, and the excluded paths.
- Text files only (skip binaries; `go.sum` and `plugins/prowler/go.sum` have no hits but should be skipped anyway).
- For each file, apply the six ordered replacements, longest-pattern-first so `plan-format-v3` is consumed before `plan-v3` could partially match.
- Case-insensitive matching with case-preserved output on the leading character, so `Plan-format v3` → `Plan-format` and `plan-format v3` → `plan-format`.
- `manifest/roadmap.md` needs line-level exclusion of `:18`, not whole-file exclusion — either special-case that line or match on its distinctive substring.
- Print every changed file and hit count so the run is auditable in the commit message.

Ordering matters: run `git mv` **first**, then the sweep, so the sweep also rewrites references inside the renamed file itself.

### Downstream tasks that read this work

- **Task C** (`format-docs-name-producers`) rewrites `plan-format.md` in producer-model terms and adds the `Discussion-Review-Gate` producer. It must not find the file still named v3, and must not redo the v2 removal.
- **Task F** (`batcher-standalone-split`) edits the renamed file's "Batch is gone / the card is the unit" section.
- **Task E** (`shed-model-contradiction-sweep`) is `loom.md`'s and `roadmap.md`'s final owner and must be told which lines this task already wrote.

## Constraints

From `CONSTRAINTS.md`:

- **Planparser Sole-Parser Invariant** (`:293–300`) — `internal/planparser` stays the sole parser of `_lyx/plan/`. This task edits only comments in that package, so the invariant is untouched. Its wording needs no change (`constraints-needs-no-change`).
- **The `gopkg.in/yaml.v3` import allowlists** at `:66` (modelspec) and `:77` — both are the hard-exclusion token. Neither may be altered.
- **Documentation Lifecycle** — `plan-format.md` is a durable pinned reference doc under `docs/reference/`, kept rather than deleted on landing (`docs/overview.md:93`, which itself carries a hit and must keep listing the file under its new name).
- **CLI / Cobra Invariant** — `internal/webstercli/cli.go:90` and `validate.go:46–47` are cobra `Long` strings inside this sweep. Every command keeps its `Short`; the help-tree tests must stay green.

Repo tooling rules (`CLAUDE.md`, `mill:conversation`):

- **No `sed`.** The sweeper is Go.
- **Markdown: semantic line breaks**, one sentence per line, no fixed-column hard-wrap. Every hand-written markdown edit — the override notes especially — follows this. A pure find/replace preserves existing line structure, so the scripted pass is safe by construction.
- **Scratch files go in `.scratch/`**, never `/tmp`.
- **Worktree isolation** — all work stays in `wts/plan-format-drop-v3-suffix`.

## Testing

No new tests. The existing suite plus the grep gate is the acceptance criterion; `shed-followups.md:232–233` is explicit that "the meaningful failure mode is incompleteness, checked by grep rather than by an assertion in a test file".

**Behaviour-preservation coverage already in place:**

- `internal/planparser/parse_test.go` and `validate_test.go` — full parse/validate coverage, including a golden fixture materialized from the renamed doc's worked example. If the sweep corrupts the doc's example, these fail.
- `internal/webstercli/cli_test.go` — CLI surface, including the `Long`-string guard this task renames.
- `internal/websterengine/template_test.go` — the embedded prompt templates, two of which (`master-template.md`, `integration-template.md`) carry sweep hits.
- `cmd/lyx/helptree_test.go` — guards the cobra help tree against the `Long`-string edits.

**Acceptance gate, in order:**

1. `git mv` recorded as a rename (`git status` shows `R`).
2. Six-pattern case-insensitive repo grep returns **zero hits** outside the exclusion set:
   ```
   grep -rniE 'plan-format-v3|plan_format_v3|plan-format v3|plan format v3|plan-v3' . \
     --exclude-dir=.git --exclude-dir=_mill --exclude-dir=.scratch \
     --exclude=shed-followups.md \
     | grep -v '^\./manifest/roadmap\.md:18:'
   ```
   **The exclusions must anchor on the path field, never on line content.** A bare `grep -v 'shed-followups.md'` drops every output line whose *text* mentions that filename — so a genuine, unfixed pattern hit in some other file that happens to cite `shed-followups.md` would be silently exempted, and the gate would pass on an incomplete sweep. `--exclude=` filters by filename before matching; the `roadmap.md:18` filter is anchored with `^\./…:18:` so it can only ever match that one file's line 18.
3. Zero plan-format-v2 references remain (`grep -rni 'v2' --include='*.md' --include='*.go'` reviewed against the deliberately-untouched list above).
4. `grep -ni 'v3' docs/reference/plan-format.md` returns **nothing** — the renamed doc names no version anywhere, per `bare-v3-label-in-scope`.
5. `grep -rni 'v3' internal/planparser/` returns only `gopkg.in/yaml.v3` import lines — the four bare labels are gone.
4. `grep -rl 'gopkg.in/yaml.v3' --include='*.go' . | wc -l` returns 32.
5. `go build ./...` clean.
6. `go test ./...` green.
7. No file under `.scratch/` is staged.
8. Every relative markdown link and anchor touched by the sweep resolves — in particular `docs/reference/plan-format.md` now exists, closing the dangling links from `loom.md:29` and the deleted `:5` blockquote.

**The one thing tests cannot catch:** whether the six hand-written prose edits and the three override notes read correctly. That is review's job, not the suite's.

## Q&A log

- **Q:** How do we stop the spec doc from destroying its own acceptance criterion when swept? **A:** Exclude `manifest/designs/shed-followups.md`, `manifest/roadmap.md:18`, and `_mill/` from both the sweep and the grep; the exclusion set becomes part of the criterion.
- **Q:** The pattern set misses `# Plan format v3` (space, not hyphen) — extend it? **A:** Yes, add `plan format v3` as a sixth pattern. Otherwise the zero-hit grep passes with the renamed doc still titled v3.
- **Q:** Bare-`v3` prose residue in Go comments and `roadmap.md:203` — in scope? **A:** Answered "out of scope; this task changes paths and names, not prose." **Superseded twice since:** `roadmap.md:203` came into scope because this task's own edit to `:201` breaks it, and round-1 review then showed the whole out-of-scope call rests on a premise the v2-erasure decision had already discarded — plus `shed-followups.md:169` explicitly rejects keeping in-text `v3` as a historical label. All four remaining sites are now in scope (`bare-v3-label-in-scope`). **Operator has not re-confirmed this reversal; flag it at handoff.**
- **Q:** The rename turns `plan-format-v3.md:5` into a link to itself — leave it for task C? **A:** No. v2 (and v1) are out of use; mentioning them anywhere is confusing and wasted tokens. Erase every plan-format-v2 reference repo-wide.
- **Q:** How far does that reach — just the self-link, the whole file, or the Go comments and `loom.md:29` too? **A:** All of it. v3 supersedes v2 the way v2 superseded v1; nothing should name the dead formats. The deliverable stays the rename.
- **Q:** Script form and location? **A:** A Go program, temporary, not committed — `.scratch/sweep/main.go`.
- **Q:** File rename mechanism? **A:** `git mv` as a separate step before the content sweep.
- **Q:** Keep the two v2-named test guards? **A:** Challenged — "why keep this?" Correct challenge: `TestTemplates_NoV2TokensRemain` never asserts on `"v2"` at all, only on `oversized`/`chain`/`## Scope`. Keep the concept assertions (they guard agent-edited prompt templates and stand alone), rename both tests to name the concepts, and drop the genuinely meaningless `"v2"` token from `cli_test.go:100`.
- **Q:** Append override notes to `shed-followups.md`? **A:** Yes — three blocks, in sections B, C and E, hand-written outside the sweep. `_mill/` is torn down on merge, so downstream tasks would otherwise never learn what changed.


### From _mill/plan/00-overview.md


```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
slug: plan-format-drop-v3-suffix
approved: true
started: '20260809-123134'
parent: main
root: ""
verify: go build ./...
```

### From _mill/plan/01-rename-and-sweep.md


```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
batch: rename-and-sweep
number: 1
cards: 3
verify: go build ./... && go test -tags integration ./internal/planparser/... ./internal/webstercli/... ./internal/websterengine/... ./internal/loomengine/... ./internal/batcher/... ./cmd/lyx/...
depends-on: []
```



- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `README.md`
  - `docs/overview.md`
  - `docs/reference/model-spec.md`
  - `docs/reference/plan-format.md`
  - `docs/reference/webster-contract.md`
  - `internal/batcher/doc.go`
  - `internal/loomengine/plan-template.md`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/plantemplate.go`
  - `internal/planparser/doc.go`
  - `internal/planparser/normalize.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/parse_test.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/sections.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/validate.go`
  - `internal/websterengine/doc.go`
  - `internal/websterengine/integration-template.md`
  - `internal/websterengine/master-template.md`
  - `internal/websterengine/runlevel_test.go`
  - `manifest/designs/loom.md`
  - `manifest/designs/review-finding-classification.md`
  - `manifest/designs/scout-plan-symbol-fields.md`
  - `manifest/designs/webster-parallel-execution.md`
  - `manifest/roadmap.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:** none
- **Deletes:** none

### From _mill/plan/02-doc-prose-v2-erasure.md


```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
batch: doc-prose-v2-erasure
number: 2
cards: 5
verify: null
depends-on: [1]
```



- **Edits:**
  - `docs/reference/plan-format.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `docs/reference/plan-format.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:** none
- **Deletes:** none

### From _mill/plan/03-go-comments-and-guards.md


```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
batch: go-comments-and-guards
number: 3
cards: 4
verify: go test ./internal/planparser/... ./internal/webstercli/... ./internal/websterengine/...
depends-on: [1]
```



- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
  - `internal/planparser/parse_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/websterengine/classify.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/webstercli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:** none
- **Deletes:** none

### From _mill/plan/04-override-notes-and-acceptance.md


```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
batch: override-notes-and-acceptance
number: 4
cards: 2
verify: go build ./... && go test ./...
depends-on: [2, 3]
```



- **Edits:**
  - `manifest/designs/shed-followups.md`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `manifest/designs/webster-parallel-execution.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides.
   When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure.
   Do NOT pick one side wholesale just because the region overlaps syntactically;
   picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values).
   Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Before keeping content from either side inside a conflict hunk, search the rest of the file (outside the hunk) for that same content.
   This judgment call is scoped narrowly — it applies only when a hunk's content might be a moved duplicate of content living elsewhere in the file;
   it does NOT apply to every ordinary step-3 disjoint-region combine (e.g. the column-A/column-B worked example above), which remains today's silent, high-confidence success path.
   Two branches:
   - **Confident case:** if the content clearly already exists elsewhere and the surrounding context makes it unambiguous that this is the same item having been moved (not two independent, separately-intended copies) — do not re-add it in the hunk;
     keep only the other side's unrelated edit.
     Worked example: one side moves a roadmap item from `## Planned` to `## Done`, while the other side makes an unrelated edit elsewhere in the file.
     The resolution keeps the item only under `## Done`;
     it is not re-added under `## Planned`.
   - **Ambiguous case:** if you cannot confidently tell whether this is the same moved content or a legitimate independent duplication — fall back to step 3's default (keep both) rather than guessing, and report the ambiguity via the `discarded` field (see Report section) with the description `"kept both sides of a conflict, ambiguous move-vs-duplicate"`.
     Worked example: a similarly-worded item appears in two different sections and you cannot tell whether it is the same item moved or a legitimate second, independently-added item.
     The resolution keeps both occurrences and reports the ambiguity via `discarded`.
5. Run `git -C /home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
   Do NOT silently keep the modification.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost.
If anything was discarded, you MUST list it;
an empty list when content was actually dropped is a protocol violation. `discarded` also carries the step 4 ambiguous-case entry `"kept both sides of a conflict, ambiguous move-vs-duplicate"` — even though nothing was technically dropped in that case, the field's purpose is to surface anything the operator should double-check before `git merge --continue`, which covers both a genuine drop and a kept-both ambiguity.
The `mill-merge-in` frontend reads this field and surfaces any losses (or ambiguities) to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix`.
