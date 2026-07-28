# Batch: stencil-optional

```yaml
task: 'PATTERN wiring: conditional constraint-injection into every agent'
batch: stencil-optional
number: 1
cards: 2
verify: go test ./internal/stencil/...
depends-on: []
```

## Batch Scope

This batch adds the one primitive every later batch depends on: an optional-marker mode for `internal/stencil`, so a `{{.X}}` marker can render as nothing without tripping either of `Fill`'s two guards. It is one batch because `internal/stencil` is a 129-line standard-library-only leaf with a single exported function and a single black-box test file — there is nothing to split. The external interface batch 7 consumes is `stencil.FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error)`; `Fill` survives unchanged in signature and behaviour so all ten existing call sites are untouched.

Batch-local decision: `Fill` is reimplemented as exactly `return FillOptional(template, values, nil)` rather than kept as a parallel implementation. One code path means the two can never drift, and it is what makes the "byte-identical on the same input, including the error path" test meaningful rather than tautological.

## Cards

### Card 1: add `FillOptional` with two-guard exemption and whitespace normalisation

- **Context:**
  - `docs/shared-libs/stencil.md`
- **Edits:**
  - `internal/stencil/stencil.go`
  - `internal/stencil/stencil_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add exported `FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error)` to `internal/stencil/stencil.go`, and reduce `Fill(template, values)` to `return FillOptional(template, values, nil)`. A name listed in `optional` must be exempt from **both** of the package's guards, which are separate mechanisms and each need their own change: (a) `unfilledTopLevelMarkers` must not report it — extend that function (or wrap its result) with the optional set so a listed name is skipped even when its value is absent, empty, or whitespace-only; (b) `Option("missingkey=error")`, set on the parsed template, fires at execution time on an **absent** key, so `FillOptional` must build a **copy** of `values` and seed every listed optional name that is absent from it with `""` before calling `Execute`. The caller's map is never mutated. The copy step must additionally apply the same `strings.TrimSpace(v) == ""` test `unfilledTopLevelMarkers` already uses and write `""` for any optional entry that fails it — seeding absent names alone is not sufficient, because a value of `"   "` would otherwise render its three spaces verbatim while the contract says it renders as nothing. Keeping that one `TrimSpace` definition of "empty" shared across both guards is the point. An optional name listed but present nowhere in the template is a harmless no-op. Update `Fill`'s and `FillOptional`'s godoc so the documented guarantee names the optional exemption and states that whitespace-only optional values normalise to the empty string; do not weaken `Fill`'s existing stated guarantee for non-optional markers. Extend the existing black-box table-driven `stencil_test.go` (do not start a new file) to cover: an optional marker absent from `values` renders as nothing with no error; an optional marker present but empty renders as nothing; an optional marker present but whitespace-only renders as nothing rather than as its whitespace; an optional marker present and non-empty renders its value; a **non**-optional empty marker still produces the existing `unfilled top-level marker(s)` error; a mix of one optional-and-empty plus one required-and-empty reports only the required one; `Fill(t, v)` and `FillOptional(t, v, nil)` are byte-identical on the same input including on the error path; an optional name absent from the template is a no-op; the caller's `values` map is not mutated by the seeding step; and repeated calls produce identical output and identical error text.
- **Commit:** `stencil: add FillOptional for markers that may render as nothing`

### Card 2: document the optional-marker mode in stencil's shared-lib doc

- **Context:**
  - `internal/stencil/stencil.go`
- **Edits:**
  - `docs/shared-libs/stencil.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `docs/shared-libs/stencil.md` so it documents `FillOptional` alongside `Fill`: the signature, that `Fill` is now defined as `FillOptional(t, v, nil)`, that a listed optional name is exempt from both the top-level emptiness check and `missingkey=error`, and that absent, empty and whitespace-only optional values all render as nothing. State explicitly that optionality is a property of the **caller's argument list**, not of the template text — a marker is optional because a Go call site says so, which is testable per call site, unlike a `{{if}}` wrapper in markdown. Write every paragraph and list item as a single unwrapped line.
- **Commit:** `docs: document stencil's optional-marker mode`

## Batch Tests

`verify: go test ./internal/stencil/...` covers `internal/stencil/stencil_test.go`, the package's only test file and the sole consumer of both cards' surface. The scope is deliberately narrow: this batch touches one leaf package plus one markdown file, and `stencil`'s ten existing call sites are protected by the fact that `Fill`'s signature and behaviour do not change — a compile break elsewhere would be caught by the repo-wide `go test -tags integration ./...` that batch 7 runs. No test file in this batch spawns a process or copies a fixture tree, so nothing here is `//go:build integration`-tagged and the Test Tier Purity Invariant is satisfied without change.
