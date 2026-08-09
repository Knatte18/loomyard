MILL_REVIEW_BEGIN
# Review: Scope the Shed producer-model rewrite into buildable tasks

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: claude-opus-5 (self-assessed)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [GAP] A's builder inventory misses repo-root and tooling sites
**Section:** A — `builder-retire`
**Issue:** `README.md:25,86,87,94,115` still documents `lyx builder` as a module and asserts "`builder` stays frozen in-tree as the plan-format-v2 consumer"; `docs/sandbox-howto.md:8,141–147,190` documents the builder suite and its launcher; `sandbox/builder-suite.cmd` invokes the `"builder-suite"` case A deletes from `tools/sandbox/main.go`; `.gitattributes:7–9` pins three `internal/builderengine/*` files. None appear in A's inventory or any other task's.
**Fix:** Add all four to A's inventory (the `.cmd` launcher is an orphan-by-construction once the suite case goes; README's "frozen in-tree" claim is directly falsified by A).

### [GAP] A's `docs/overview.md` list is incomplete
**Section:** A — `builder-retire`, *Doc retirement*
**Issue:** A enumerates `:92`, `:227`, `:264`, `:268`, `:375` but not `:292`, which names "builder implementer" among `internal/pattern`'s prompt consumers — the same phrase A explicitly owns at `roadmap.md:42` — nor `:265`, whose webster entry is defined as "fork-based sibling of builder".
**Fix:** Add `overview.md:292` and `:265` to A's inventory, or state that E's later sweep owns them.

### [GAP] F's batcher inventory misses webster's config template and package doc
**Section:** F — `batcher-standalone-split`
**Issue:** The `batcher:` key F removes physically lives in `internal/websterengine/template.yaml:3`, and `internal/websterengine/doc.go:12,25–27` documents "webster.yaml's `batcher:` key" — neither is listed. Worse, `internal/webstercli/verbs_test.go:702` does `strings.Replace(websterengine.ConfigTemplate(), 'batcher: ""', 'batcher: "bogus"', 1)`; both `TestPersistentPreRunE_UnknownBatcherFailsFast` and `TestPersistentPreRunE_DefaultBatcherResolves` break once the key leaves the template, and F names only `:221–223` and the `:697` comment.
**Fix:** Add `template.yaml:3`, `websterengine/doc.go:12,25–27`, `verbs_test.go:696–732` (whole gate-test pair, not just the comment), and `docs/overview.md:267` to F's inventory.

### [GAP] `shed.md:63`'s "this task is still pending" claim has no owner
**Section:** E — `shed-model-contradiction-sweep`
**Issue:** `shed.md:63` says "Wiki task `shed-producer-model-scoping` is the dedicated pass that reconciles any remaining detail mismatch" — false once this task lands. E's `shed.md` inventory is an explicit line list (`:7`, `:13`, `:18`, `:19`, `:41`) with no "everything else" catch-all for that file, so `:63` (and the surrounding "Why this doc doesn't rewrite loom.md's full detail" section) is unclaimed. `loom.md:76` carries the identical stale claim and is covered only by E's unenumerated catch-all.
**Fix:** Name `shed.md:63` and `loom.md:76` explicitly in E's inventory, alongside the `loom.md:91–94` claim E already owns.

### [NOTE] Open question 2 has no owner or gate
**Section:** Surfaced open questions
**Issue:** Question 1 is gated as a named precondition on roadmap's Planned `Shed` item and question 3/4 land in files E edits; question 2 (`Discussion-Write` has no Input) is stated with no owner and no recording site.
**Fix:** Say where E records it — the same roadmap precondition, or `shed.md`'s contract section.

### [NOTE] Comment-level builder residue unclaimed
**Section:** A / Technical context
**Issue:** `internal/perchengine/doc.go:13` ("builder-review"), `internal/modelspec/modelspec.go:7,35` ("builder's roles", "builder, perch/burler/loom configs"), `internal/loomengine/configtemplate.go:4` ("mirroring builderengine's ... embed-and-accessor") survive A's deletion as stale prose in package docs.
**Fix:** Note them in A's body as comment-only, non-compiling residue to sweep opportunistically.

## Verdict

GAPS_FOUND
Four inventory/ownership gaps; the model, decisions, and dependency graph themselves hold.
MILL_REVIEW_END
