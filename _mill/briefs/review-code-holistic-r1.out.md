MILL_REVIEW_BEGIN
# Review: Build internal/stencil: fill markdown prompt templates — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-07
```

## Verification notes

- **Leaf discipline:** `internal/stencil/stencil.go` imports only `bytes`, `fmt`, `sort`, `strings`, `tmpl "text/template"`, `text/template/parse` — no cobra, no I/O, no domain words. The `tmpl` alias is justified (avoids shadowing by the `template []byte` parameter), not a stray deviation.
- **`Fill` algorithm matches the pinned ordering exactly:** leading-comment strip → `Option("missingkey=error")` parse (`%w`-wrapped) → depth-0-only walk of `Root.Nodes` for bare `{{.X}}` ActionNodes → collect/dedup/sort/fail-before-execute → `Execute` (`%w`-wrapped). Nil `Tree`/`Root` guarded. Verified against `internal/yamlengine/resolve.go`'s godoc density/error-style convention (plain `fmt.Errorf` for the domain guard, `%w` for parse/execute wraps) — matches the Shared Decision precisely.
- **Tests** (`internal/stencil/stencil_test.go`) are black-box `package stencil_test`, table-driven, and cover all 14 scenarios from the batch card: happy path, missing/empty top-level marker, multi-offender sort+dedup, branch-internal incremental catch (including the top-level-wins-over-branch-offender race), malformed template, conditional taken/not-taken, forgotten discriminator, unused-values, leading-comment strip (including comment-only→empty), whitespace-only template, idempotence/determinism, and no-HTML-escaping. Traced each test's expected Go `text/template` semantics (map missingkey=error behavior, untaken-branch non-evaluation) against the implementation and found them consistent.
- **Docs (Card 3):** `docs/shared-libs/stencil.md` status blockquote, signature (`map[string]string`), marker-syntax grammar, and load-bearing-guarantee wording all updated to the built contract; `docs/shared-libs/README.md`'s stencil bullet drops the 🚧 marker in the sibling style. `docs/overview.md` correctly left untouched (it doesn't enumerate individual shared libs).
- **No out-of-plan files:** `internal/stencil/` contains only `stencil.go` and `stencil_test.go`; no other file in the repo references `internal/stencil` outside docs/mill artifacts, consistent with "consumers are out of scope for this batch."
- **Constraint check:** stencil is never a registered cobra module, so CLI/Cobra and Sandbox-Coverage invariants are correctly inapplicable; no hubgeometry/lyxtest-leaf conflicts (stencil doesn't touch either).

No BLOCKING or NIT findings — implementation, tests, and docs align precisely with the plan and Shared Decisions.

## Verdict

APPROVE
Implementation, tests, and docs fully match the plan's pinned algorithm, Shared Decisions, and card requirements.
MILL_REVIEW_END