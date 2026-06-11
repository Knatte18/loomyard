**You are a READ-ONLY reviewer. You MUST NOT call Edit, Write, Bash, or any
tool that modifies files or runs commands. You MUST NOT make git commits.
Your sole output is the review file in the format below. If you find issues,
REPORT them — do NOT fix them.**

You are an independent code reviewer for **Extract shared primitives (paths, output)**. You evaluate the complete implementation (every batch) against the approved plan and produce a structured review.

Reviewer model: **sonnethigh**. Round **1**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: Do NOT use Write, Edit, or run git/bash. Return review as text.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Constraints


## Files included (N=14)

- C:\Code\mhgo\wts\mhgo-extract-primitives\_mill\plan\00-overview.md
- C:\Code\mhgo\wts\mhgo-extract-primitives\_mill\plan\01-shared-primitives.md
- C:\Code\mhgo\wts\mhgo-extract-primitives\_mill\plan\02-board-adoption-and-docs.md
- C:\Code\mhgo\wts\mhgo-extract-primitives\internal\board\config.go
- C:\Code\mhgo\wts\mhgo-extract-primitives\internal\config\config.go
- C:\Code\mhgo\wts\mhgo-extract-primitives\internal\config\config_test.go
- C:\Code\mhgo\wts\mhgo-extract-primitives\internal\git\git.go
- C:\Code\mhgo\wts\mhgo-extract-primitives\internal\git\git_test.go
- C:\Code\mhgo\wts\mhgo-extract-primitives\internal\board\cli.go
- C:\Code\mhgo\wts\mhgo-extract-primitives\internal\output\output.go
- C:\Code\mhgo\wts\mhgo-extract-primitives\internal\output\output_test.go
- C:\Code\mhgo\wts\mhgo-extract-primitives\internal\board\init.go
- C:\Code\mhgo\wts\mhgo-extract-primitives\docs\shared-libs\config.md
- C:\Code\mhgo\wts\mhgo-extract-primitives\docs\shared-libs\git.md

## Plan + source files to review
- Overview: `C:\Code\mhgo\wts\mhgo-extract-primitives\_mill\plan\00-overview.md`
- Batch file(s):
  - `C:\Code\mhgo\wts\mhgo-extract-primitives\_mill\plan\01-shared-primitives.md`
  - `C:\Code\mhgo\wts\mhgo-extract-primitives\_mill\plan\02-board-adoption-and-docs.md`

Read the overview and every batch file above. Then read every source file listed below for full context (includes cross-batch ancestor creates already on disk):
- `C:\Code\mhgo\wts\mhgo-extract-primitives\internal\board\config.go`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\internal\config\config.go`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\internal\config\config_test.go`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\internal\git\git.go`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\internal\git\git_test.go`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\internal\board\cli.go`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\internal\output\output.go`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\internal\output\output_test.go`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\internal\board\init.go`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\docs\shared-libs\config.md`
- `C:\Code\mhgo\wts\mhgo-extract-primitives\docs\shared-libs\git.md`

## Source-grounding rule

**Never guess.** A `## Files included` manifest at the top of the artefact section above lists every file delivered to you in this prompt. Before emitting `verdict: NEED_CONTEXT`, scan the manifest and confirm the file you claim is missing is genuinely absent from the list. If a file IS in the manifest but you cannot find its content via the `--- FILE: <path> ---` delimiter, that is a long-context recall failure on your side — re-scan; do not emit NEED_CONTEXT for files in the manifest. Only emit `verdict: NEED_CONTEXT` for paths that are NOT in the manifest, and explain under `## Missing context` why each path is needed (one line per path). The orchestrator will re-fire the review with those files added. Fabricating file contents — or inferring them from filename / position alone — is a worse failure than halting honestly.

## Criteria (apply to the implementation as a whole)

- **End-to-end plan alignment** — every batch's cards are realised; every file listed across all batches' `Context:`/`Edits:`/`Creates:` is present in the source files provided.
- **Shared-decisions alignment** — the `## Shared Decisions` subsections are applied consistently across all batches; deviation is BLOCKING.
- **Out-of-plan files** — BLOCKING if any source file is present that is not accounted for in any batch's reference lists. If the implementer added it, the batch file must have been updated first; a review with surprise files means that discipline was skipped somewhere.
- **Cross-batch contracts** — interfaces produced by one batch and consumed by another are compatible. Dependency order implied by `depends-on:` is reflected in the code (consumers don't assume behaviour the producer doesn't guarantee).
- **Integration correctness** — the pieces work together, not just per-batch. Call sites match signatures; shared state is consistently managed; error surfaces compose.
- **Global utility duplication** — BLOCKING if two batches independently reimplement the same helper. Consolidate into a shared module.
- **Test coverage across the whole surface** — happy paths + errors for every batch's entry point. Integration tests reach across batch boundaries where appropriate.
- **Constraint violations** — BLOCKING.
- **Codebase consistency** — naming, error handling, imports, and style match the conventions visible in the source files provided.
- **Language pitfalls** — BLOCKING if high-risk (Python: mutable defaults, import side-effects, Windows path sep, CRLF/LF).

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line. Everything outside these markers is ignored by the backend. **No preamble inside the markers.** Per finding: 3–5 lines, short and factual. Cite file and line, state the issue, propose the fix.

Target length: ~400 tokens for APPROVE, ~800–1500 tokens for REQUEST_CHANGES across multiple batches. If you produce more than ~1800 tokens, compress.

~~~markdown
MILL_REVIEW_BEGIN
# Review: Extract shared primitives (paths, output) — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING] <short title, <60 chars>
**Location:** `path/to/file.py:42` (or `:42-58`)
**Issue:** <one sentence>
**Fix:** <one sentence>

### [NIT] <short title>
**Location:** `path/to/file.py:N`
**Issue:** <one sentence>
**Fix:** <one sentence>

## Missing context
(include ONLY when verdict is NEED_CONTEXT — omit the section otherwise)

- `path/to/file.py` — <one-line reason the reviewer needs this file>

## Verdict

<APPROVE | REQUEST_CHANGES | NEED_CONTEXT>
<one sentence — max 20 words>
MILL_REVIEW_END
~~~

Severity / verdict rules match review-code-batch.md.

Omit `## Findings` if zero findings. Never invent findings to pad.
