# tree-sitter-analyzer against quarry and lyx, refreshed 2026-10-24

A research report of ideas: inspiration, nothing here is a commitment to build.
It refreshes the 2026-10-18 study of tree-sitter-analyzer (TSA), a Python project, against this repository.
TSA is read as it stands at `/home/knatte/Code/tree-sitter-analyzer`, v3.0.0 (merge `01f8663d`, 2026-10-23); nothing was run.
This repository is read at the commit this branch was cut from.

## Summary

The 2026-10-18 study's framing of lyx is out of date, and two of its five ideas are already built.

- Idea 2 (bump quarry, wire `enclose`) is done: `go.mod` pins quarry v0.4.0, and `internal/planglyph/repo.go` wraps `EncloseAt`.
  The glyph-cited failure trace it hoped for exists as `internal/failuretrace`.
- Idea 1 (inline bodies in a card pack) has no target: `lyx loom card-pack` and the kick-start pack are gone, and `internal/websterengine/config.go` lists their three config keys as retired.
- Ideas 3, 4 and 5 stand, each with a narrower shape than the entry gave it.
- TSA's v3.0.0 release adds nothing that changes the "would not pay off" list; its news is call-answer honesty and index speed, the territory of idea 4.

Worth a task, in the order of how little must be assumed: idea 3 (symbol delta for reviewers), idea 5 (resolve references in docs), idea 4 (a typed reference query).
Each carries a measurement caveat below, in keeping with quarry's standing rule that nothing is built without a measured or functional reason.

## What changed in lyx since the 2026-10-18 study

| The entry says | The code says |
|---|---|
| lyx pins quarry v0.2.1 | `go.mod` requires `github.com/Knatte18/quarry v0.4.0`, added with C# plan glyphs (`341556201`) |
| quarry v0.3.0's `Enclose` has no lyx wrapper | `planglyph.EncloseAt` is the package's one call to `(*quarry.Repo).EncloseAt` (`internal/planglyph/repo.go`) |
| `lyx loom card-pack` lists spans only | no such verb; the implementer stencil tells the fork to run one `lyx quarry resolve <glyph>...` over a card's target and `Uses:` glyphs and read the spans (`contracts/stencils/webster/webster-body-implementer.md`) |
| `KickstartPack` in `internal/planglyph/pack.go` | neither exists; `kickstart_pack`, `kickstart_pack_per_card` and `kickstart_pack_file` are `RetiredKeys` in `internal/websterengine/config.go` |
| only `recordbatch.go` uses `delta` | it also feeds `internal/failuretrace` (a member a card deletes or moves is looked up in the batch's delta) |
| `ScopeGuard` could map edits outside any symbol | it already reports, at file level, each changed file no symbol of the delta lives in, unless a card target covers it (`internal/planglyph/scope.go`) |

Quarry's CLI in lyx offers `toc`, `glyphs`, `resolve` and `expand` (`internal/quarrycli/cli.go`), and no `enclose` verb.
That is the one piece of idea 2 still open, and it is a verb, not a capability: the facade call exists.

## What changed in TSA since 2026-10-18

The history since 2026-10-18 is 74 commits to v3.0.0, most of them in `nav action=callers` and the index.

- Call answers are stricter.
  `callers` no longer answers `complete` when a real caller exists; a bare method name now passes the same gates as `Class.method`; an unresolved receiver keeps every same-named method as possible, not only the same-file one.
  The release gate reports 0 wrong `complete` answers across seven real repositories, about 1,570 sweep answers.
- Resolution of JavaScript, TypeScript and Java improved (ES-module imports and re-exports, default-export objects, `this.m()` through the `extends` chain, typed Java receivers).
- Warm `callers` is 44 to 67 percent faster, from a call graph built once per query, and refresh is incremental with a per-file Merkle certification root (RFC-0039).
- Default outputs shrank behind honest totals: `edit action=impact` and the AST-cache listing keep the top entries and carry `*_total`, `details_truncated` and `details_next_step`.
- `docs/agent-envelope-contract.md` gained typed admission codes and evidence-site lists with totals (`name_only_sites_total`, `unresolved_sites_total`, each with a `*_truncated` flag).

What it means here: TSA's own work in the three weeks went into making a reference answer trustworthy, which is the reason quarry parks its typed reference query and the reason idea 4 below is shaped around honesty.
None of it touches ideas 1 to 3 or 5.

## The ideas, rechecked

### 1. Inline symbol bodies in the card pack: no target

- TSA: `mcp/tools/symbol_body_inline.py` still inlines a verbatim body beside each coordinate in three line-capped tiers (80 lines for a definition, 40 per neighbour, 30 per search match) with a total budget per response and a `full_at` pointer when cut.
  The argument is unchanged: cost is turns times tokens.
- Today: the pack it would extend was removed; the board entry's orch view gives the glyph audits `glyph-audit-2026-10-12` to `-17` under `docs/research/` as the reason, and this report did not re-read them.
  The fork now spends one `lyx quarry resolve` call per card and reads the spans.
- Verdict: not worth it as stated.
  The residual idea, `lyx quarry resolve --body` returning source with a cap and a pointer, is the same push-mode bet with a new name.
  It needs a new measurement, not a new design, and quarry's own results found turn savings did not reliably show.

### 2. Bump to quarry v0.3.0 and wire `enclose`: built, one verb open

- TSA: `codegraph_ast_path` and the `pr` route's diff-to-symbol mapping.
- Built: `planglyph.EncloseAt`; `failuretrace.Trace` maps `path:line` locations in a failed verify log to the plan cards whose targets hold the member (a `not_found` answer, a location inside no member, falls back to the file and the file's self glyph).
  The failure record's `cards`, `later_range` and `untraced` fields carry the result.
- Open, small: a `lyx quarry enclose` verb, so an agent can turn a reviewer's `file:line` or a `go vet` line into a glyph by hand.
  The uses are a Burler or reviewer finding citing a glyph instead of a line that goes stale, and the plan writer checking a failure's location.
  Failure trace takes only Go test and build output and keeps at most twenty locations, so a `go vet` or lint line is not traced today.
- Verdict: the entry's top idea is done; what remains is a thin verb and, separately, the finding-citation use, which is a prompt change to the review stencils rather than new code.

### 3. Give reviewers the symbol delta: stands

- TSA: `codegraph_pr_review_tool.py` classifies a change per file (semantic category, risk verdict, API surface) and adds call-graph blast radius; `ast_diff.py` has the kinds `added`, `removed`, `changed`, `moved`, `renamed`, `signature_changed` and `body_changed`, and is name-based.
- Today: quarry's `delta` is stronger on identity than TSA's differ, because a glyph is unique and the answer marks `body`, `signature`, `doc` or `file` changes with rename tiers (exact, then evidence).
  Its consumers are `recordbatch.go` (done checks, handle binding, `ScopeGuard`, drift) and `failuretrace`; no stencil under `contracts/stencils/` mentions the delta, so reviewers and Burler read the textual diff alone.
- Idea: render the batch or branch delta (created, deleted, changed with which part, renames with tier) into the review prompts, and list members whose signature changed that no plan target names, as a focus list.
- Why it might pay: a signature change is the cheapest kind of defect to miss in a long diff and the cheapest for a tool to see.
- Caveats: it informs and never gates, matching `ScopeGuard`'s informational posture; and TSA's blast radius is the part to leave out, since that is idea 4's territory.
  Measure on past Burler rounds whether the focus list names defects reviewers missed, before it becomes a default.
- Verdict: worth a task, small to medium.

### 4. A typed reference query in quarry: stands, narrower

- TSA: `nav callers`, `callees` and `impact`.
  The part worth borrowing is the honesty contract in `docs/agent-envelope-contract.md`: `truncated`, the exact total before the cap, the count listed with its cap, and a `next_step` stating the real numbers.
  TSA's v3.0.0 work makes that contract stricter, with a reason code and evidence sites for each way a caller can be missed, so "no callers found" is never read as "none exist".
- Today: lyx has a Go-only typed reference finder behind the `caller-uncovered` and `delete-before-reference` plan checks (`internal/planglyph/callercoverage.go`, `typesload.go`, `deleteorder.go`).
  The plan stencil tells the writer to list every caller of a deleted or re-signed member and defers to the plan spec for what counts (`contracts/stencils/loom/loom-template-plan.md`), but the writer has no verb that returns the callers; the gate finds them after the plan is written.
- Gain: the plan writer, Webster and reviewers could ask for callers before writing cards and skip the rework loop a `caller-uncovered` finding starts.
- Design constraints this repository already holds: no silent pick on a name match (TSA's name-based resolution is the pattern quarry forbids); the answer carries `verified` and the four-part truncation shape; reference answers stay in quarry, language-neutral (the operator ruled out an LSP server on 2026-10-18 because it is one more daemon and one language per server).
- Caveat: quarry measured reference tools no better than grep for agents, so the justification is the plan gate's function, as the quarry roadmap already says for its parked type-checker task, and not agent convenience.
  A cheap first step that needs no quarry change: expose `callercoverage`'s answer as a read-only `lyx` verb for the plan writer, then see whether `caller-uncovered` findings fall.
- Verdict: the most valuable item over time, and the largest; stays parked until that function-level case is recorded.

### 5. A doc-drift scan that resolves references in docs: stands, and TSA's version is weaker than the entry implied

- TSA: `doc_sync.py` takes backtick spans and link targets from markdown, skips fences, globs, URLs, anchors, placeholders and directories, keeps spans with a file extension from a short list, and tests that the file exists.
  It checks file paths only; it does not resolve a symbol.
- Today: `internal/lyxcwd/docslink_test.go` (`PATTERN-markdown-link-integrity`) resolves inline links and anchors under `docs/`, nothing else.
  A renamed function or deleted file named in backticks in `docs/*.md`, `PATTERN.md`, `pattern/` or a `doc.go` goes unnoticed, though `CLAUDE.md` requires docs to land in the same commit as the change.
- Idea: a `go test` scan that takes every path-shaped backtick token and every glyph-shaped one (when a unique glyph spelling can be recognised) and resolves them, the glyphs in one batched `quarry` `Resolve`.
  The glyph half goes beyond TSA and is where quarry's identity helps most; the path half is a few dozen lines.
- Caveats: false positives are the cost.
  TSA needed a skip list for placeholders, runtime output files and per-user paths, and this repository's docs name many planned or example paths; the scan needs an allow-marker and a first pass to see how noisy it is.
  Package doc comments in `.go` files are a second source and need the Go parser, not a markdown regex.
- Verdict: worth a task, small to medium; start with the path half.

### Later: TSA's per-language extractors as a cross-check for Python and C#

- Unchanged.
  Quarry now ships C# (lyx enables it with `language: csharp`); TSA's `languages/` still holds 22 plugins, with full call graphs for 13 per its own 2026-09 audit.
  Their naming is not glyph-unique, so use them to find missed cases (nested classes, `partial`, overloads, fields, properties), never as code to port.

## Would not pay off, rechecked

Every item of the 2026-10-18 list holds after v3.0.0.
Notes that sharpen a reason:

- Affected tests: `internal/impactset` derives the impacted set from the import graph and adds the `//lyx:guard` tests, exactly; TSA's `edit action=impact` returns a `verification_command`, and v3.0.0 made its full output a summary by default, a sign that unbounded impact output costs more than it earns.
- Compact and summary views: unchanged; quarry's `glyphs` view is the compact index and its measurement found a lossy view cost precision.
- TOON: unchanged; agents here copy glyphs verbatim.
- Verdict envelope and `next_step`: TSA's own contract shows the cost, a growing alphabet of reason codes (admission, completeness, freshness) that each client must learn; lyx gates already carry check IDs and a blocking or informational severity.
- Architectural constraint DSL: `PATTERN.md` entries enforced by `go test` scans do the same job in the language the code is in.
- Index, cache and incremental sync: TSA's recent work is mostly this (a Merkle certification root, scoped refresh, background graph builds) and its own value audit calls the surface and test maintenance its biggest cost.
  Quarry answers a whole repository in well under a second with no cache, and a cache adds a staleness risk a gate cannot take.
- `mutation_probe`, `test_gap`, `modification_guard`, co-change coupling, health scores, dead code, clones and UML: unchanged; each yields a probabilistic hint, and a plan or gate decision here needs a mechanical answer.
- Decision journal, partial read by line range, curated skills: lyx has the discussion record and the board, the Read tool, and per-agent tool scoping.

## What TSA does that is worth knowing about

These are not proposals; they are the project-level lessons visible in its code and docs.

- Honest truncation as a contract rather than a convention: a client can tell a complete list from a capped one without reading prose.
  Any lyx verb that caps a list (a verify log excerpt, a failure trace's twenty locations) can adopt the same four fields.
- Evidence beside every negative answer: "no callers" carries the reason it might be wrong and the sites to read.
  This is the same refusal-way-forward discipline `PATTERN-refusal-way-forward` asks of lyx refusals.
- A contract test per surface: TSA pins its MCP and CLI parity and its tool-count claims in tests.
  lyx pins its CLI and recipes the same way, so no change follows.
- Bus factor: TSA's own audit records 883 commits in 90 days from one person; a reason to borrow ideas and not depend on the project.

## Open questions

- The 2026-10-18 study text itself is not in the repository, so this report refreshes the board entry's summary of it.
- Whether glyph-cited findings (idea 2) change how Burler's fixers act is untested; a ladder run on a past review round would show it.
- Idea 3's value depends on how often reviewers miss a signature change; no number for that is recorded.
