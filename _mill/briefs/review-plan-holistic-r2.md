**You are a READ-ONLY reviewer. You MUST NOT call Edit, Write, Bash, or any
tool that modifies files or runs commands. You MUST NOT make git commits.
Your sole output is the review file in the format below. If you find issues,
REPORT them — do NOT fix them.**

You are an independent plan reviewer for **board-modul (rename fra wiki) + _mhgo-konfigurasjon**. You evaluate the complete plan (all batches) and produce a structured review.

Reviewer model: **sonnetmax**. Round **2**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: Do NOT use Write, Edit, or run git/bash. Return review as text.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Constraints


## Files included (N=40)

- C:\Code\mhgo\wts\config-layer\_mill\plan\00-overview.md
- C:\Code\mhgo\wts\config-layer\_mill\plan\01-rename-wiki-to-board.md
- C:\Code\mhgo\wts\config-layer\_mill\plan\02-config-system.md
- C:\Code\mhgo\wts\config-layer\_mill\plan\03-config-driven-render-facade.md
- C:\Code\mhgo\wts\config-layer\_mill\plan\04-cwd-activation-and-board-path.md
- C:\Code\mhgo\wts\config-layer\_mill\plan\05-mhgo-init-command.md
- C:\Code\mhgo\wts\config-layer\_mill\plan\06-docs-and-roadmap.md
- C:\Code\mhgo\wts\config-layer\go.mod
- C:\Code\mhgo\wts\config-layer\cmd\mhgo\main.go
- C:\Code\mhgo\wts\config-layer\cmd\mhgo\main_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\cli.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\cli_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\git.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\git_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\layer.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\layer_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\lock.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\lock_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\render.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\render_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\spawn_other.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\spawn_windows.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\store.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\store_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\sync.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\sync_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\task.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\task_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\wiki.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\wiki_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\bench_git_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\bench_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\concurrency_test.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\doc.go
- C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\integration_test.go
- C:\Code\mhgo\wts\config-layer\go.sum
- C:\Code\mhgo\wts\config-layer\docs\wiki.md
- C:\Code\mhgo\wts\config-layer\docs\overview.md
- C:\Code\mhgo\wts\config-layer\docs\benchmarks.md
- C:\Code\mhgo\wts\config-layer\_mill\discussion.md

## Plan files to review
- Overview: `C:\Code\mhgo\wts\config-layer\_mill\plan\00-overview.md`
- Batches:
- `C:\Code\mhgo\wts\config-layer\_mill\plan\01-rename-wiki-to-board.md`
- `C:\Code\mhgo\wts\config-layer\_mill\plan\02-config-system.md`
- `C:\Code\mhgo\wts\config-layer\_mill\plan\03-config-driven-render-facade.md`
- `C:\Code\mhgo\wts\config-layer\_mill\plan\04-cwd-activation-and-board-path.md`
- `C:\Code\mhgo\wts\config-layer\_mill\plan\05-mhgo-init-command.md`
- `C:\Code\mhgo\wts\config-layer\_mill\plan\06-docs-and-roadmap.md`

Read the overview and every batch listed above. Then read the source files referenced across all batches:
- `C:\Code\mhgo\wts\config-layer\go.mod`
- `C:\Code\mhgo\wts\config-layer\cmd\mhgo\main.go`
- `C:\Code\mhgo\wts\config-layer\cmd\mhgo\main_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\cli.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\cli_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\git.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\git_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\layer.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\layer_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\lock.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\lock_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\render.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\render_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\spawn_other.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\spawn_windows.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\store.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\store_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\sync.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\sync_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\task.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\task_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\wiki.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\wiki_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\bench_git_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\bench_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\concurrency_test.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\doc.go`
- `C:\Code\mhgo\wts\config-layer\internal\wiki\wikitest\integration_test.go`
- `C:\Code\mhgo\wts\config-layer\go.sum`
- `C:\Code\mhgo\wts\config-layer\docs\wiki.md`
- `C:\Code\mhgo\wts\config-layer\docs\overview.md`
- `C:\Code\mhgo\wts\config-layer\docs\benchmarks.md`
- `C:\Code\mhgo\wts\config-layer\_mill\discussion.md`

## Intentionally deleted (N=26)

- docs/wiki.md
- internal/wiki/cli.go
- internal/wiki/cli_test.go
- internal/wiki/git.go
- internal/wiki/git_test.go
- internal/wiki/layer.go
- internal/wiki/layer_test.go
- internal/wiki/lock.go
- internal/wiki/lock_test.go
- internal/wiki/render.go
- internal/wiki/render_test.go
- internal/wiki/spawn_other.go
- internal/wiki/spawn_windows.go
- internal/wiki/store.go
- internal/wiki/store_test.go
- internal/wiki/sync.go
- internal/wiki/sync_test.go
- internal/wiki/task.go
- internal/wiki/task_test.go
- internal/wiki/wiki.go
- internal/wiki/wiki_test.go
- internal/wiki/wikitest/bench_git_test.go
- internal/wiki/wikitest/bench_test.go
- internal/wiki/wikitest/concurrency_test.go
- internal/wiki/wikitest/doc.go
- internal/wiki/wikitest/integration_test.go

## Source-grounding rule

**Never guess.** A `## Files included` manifest at the top of the artefact section above lists every file delivered to you in this prompt. Before emitting `verdict: NEED_CONTEXT`, scan the manifest and confirm the file you claim is missing is genuinely absent from the list. If a file IS in the manifest but you cannot find its content via the `--- FILE: <path> ---` delimiter, that is a long-context recall failure on your side — re-scan; do not emit NEED_CONTEXT for files in the manifest. Only emit `verdict: NEED_CONTEXT` for paths that are NOT in the manifest, and explain under `## Missing context` why each path is needed (one line per path). The orchestrator will re-fire the review with those files added. Fabricating file contents — or inferring them from filename / position alone — is a worse failure than halting honestly.

## Criteria (apply to the plan as a whole)

- **Constraint violations** — BLOCKING.
- **Alignment** — plan covers all task requirements.
- **Decision alignment** — every `### Decision:` in `## Shared Decisions` faithfully implemented.
- **Completeness** — every card has `Creates`/`Edits`, `Context`, `Requirements`, `Commit`.
- **Sequencing + batch dependencies** — correct order within and across batches; `batch-depends` accurate; no forward deps.
- **Batch Index DAG integrity** — BLOCKING if the `batches:` block in `00-overview.md` has a cycle, references a batch name not declared, or names a `file:` not present in the plan directory.
- **Edge cases + risks** — failures, empty states, boundaries addressed.
- **Over-engineering** — unneeded abstractions or unrequested features.
- **Codebase consistency** — follows patterns in the source files provided.
- **Test coverage** — error paths + edges.
- **Language pitfalls** — BLOCKING if high-risk (Python: mutable defaults, import side-effects, Windows path sep, CRLF/LF).
- **Integration test reachability** — BLOCKING if integration tests added but `verify:` doesn't run them.
- **Explore targets** — purpose-driven; subset of `Context:`.
- **Step granularity + atomicity** — each card small and self-contained.
- **Requirements specificity** — BLOCKING if `Requirements:` uses vague prose ("refactor X", "update to use helper") without naming the specific function, class, or constant being changed. Stable identifiers are required.
- **Context field** — non-empty per card; Edits: files are implicitly read.
- **Context completeness** — BLOCKING if `Requirements:` mentions a function, class, or constant from a file not listed in `Context:` or `Edits:`. The implementer may only read files in `Context:`; a missing entry means cold-start exploration.
- **Global step numbering** — unique, sequential, no gaps across batches.

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line. Everything outside these markers is ignored by the backend. **No preamble inside the markers.** Per finding: 3–5 lines, short and factual. The consumer has full context of the plan; do NOT explain background. Cite the batch/card, state what's wrong, propose the fix.

Target length: ~300 tokens for APPROVE, ~600–1200 tokens for REQUEST_CHANGES across multiple batches. If you produce more than ~1500 tokens, compress.

```
MILL_REVIEW_BEGIN
# Review: board-modul (rename fra wiki) + _mhgo-konfigurasjon — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: sonnetmax
reviewed_file: plan/
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING] <short title, <60 chars>
**Location:** <batch / card number>
**Issue:** <one sentence>
**Fix:** <one sentence>

### [NIT] <short title>
**Location:** <batch / card>
**Issue:** <one sentence>
**Fix:** <one sentence>

## Missing context
(include ONLY when verdict is NEED_CONTEXT — omit the section otherwise)

- `path/to/file.py` — <one-line reason the reviewer needs this file>

## Verdict

<APPROVE | REQUEST_CHANGES | NEED_CONTEXT>
<one sentence — max 20 words>
MILL_REVIEW_END
```

Severity / verdict rules match review-plan-batch.md.

Omit `## Findings` if zero findings. Never invent findings to pad.
