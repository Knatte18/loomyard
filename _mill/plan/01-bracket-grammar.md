# Batch: bracket-grammar

```yaml
task: 'modelspec: bare-effort shorthand and version-to-v rename'
batch: 'bracket-grammar'
number: 1
cards: 6
verify: go test ./internal/modelspec/...
depends-on: []
```

## Batch Scope

This batch delivers the whole task: two changes to the model-spec bracket grammar — a bare bracket token means `effort=<token>`, and the bracket's version param key is renamed from `version` to `v` — plus the tests and the two docs that pin the grammar.
It is one batch because every card operates on the same package (`internal/modelspec`) and the same mental model of one small parsing loop;
the two documentation cards restate the same grammar line the code implements and would drift if scheduled separately.
There is no external interface for a later batch to consume — this is the last batch.

Batch-local decisions that go beyond the overview's `## Shared Decisions`:

- Card order is load-bearing. The vocabulary map (card 1) must exist before the parser references it (card 2), and the parser must have its new behaviour before the tests assert it (card 3). Cards 4, 5 and 6 are independent of each other but all depend on card 1's map existing.
- The intermediate commits for cards 2 and 3 are not individually green: card 2 deletes the `no '=' separator` error that card 3's table still asserts against. The batch's `verify:` gate is the correctness boundary, not each card's commit.

## Cards

### Card 1: add the `bracketKeys` bracket-spelling vocabulary

- **Context:**
  - `contracts/specs/llm-model-spec.md`
  - `internal/modelspec/load.go`
- **Edits:**
  - `internal/modelspec/modelspec.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new package-level var `bracketKeys` immediately after the existing `knownParams` var, mapping an accepted bracket key spelling to the canonical `Params` key it normalizes to: `effort` maps to `effort`, and `v` maps to `version`.
  Give it a doc comment stating that it is the bracket-facing spelling layer, that its values are canonical `Params` keys, and that its value set must remain a subset of `knownParams`' key set.
  Leave `knownParams` itself unchanged in membership — it keeps `effort` and `version` and keeps being the vocabulary `validateAlias` gates `Entry.Defaults` against.
  Correct `knownParams`' own doc comment: its opening clause currently claims it is the closed set of keys a bracket or a registry Defaults map may use, which stops being true once the bracket is gated by `bracketKeys`. Re-scope that clause to `Entry.Defaults` and the canonical `Params` key space, and add a sentence pointing at `bracketKeys` as the bracket-facing spelling layer over it.
  Keep the existing sentence stating that it gates param KEYS ONLY and never model names or aliases — it is still true and is load-bearing for the no-value-validation decision.
  Rewrite the package doc's one-line grammar, which currently spells both shapes with a `key=value,...` bracket interior, so that the bracket interior reads as a comma-separated list of items where an item is either `key=value` or a bare effort token. Word it identically to the grammar line card 5 pins in the contract doc — the two are the same grammar stated twice and must not drift.
  Leave the package doc's consumer example untouched: the line assigning from `resolved.Params["version"]` shows the canonical key a consumer reads, which this rename does not change.
- **Commit:** `feat(modelspec): add bracketKeys spelling vocabulary`

### Card 2: bare-token and `v=` handling in `parseBracket`

- **Context:**
  - `internal/modelspec/modelspec.go`
  - `contracts/specs/llm-model-spec.md`
- **Edits:**
  - `internal/modelspec/parse.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite the per-segment loop body of `parseBracket`. `Parse` itself keeps its current body — the split into body and bracket interior, the whitespace rejection, and the alias-vs-escape-form branch are all unchanged, and one `parseBracket` call still serves both forms.
  The new per-segment order is: empty-segment check, then branch on whether the segment contains `=`, then the duplicate check, then store. The duplicate check and the unknown-key check swap relative to today — canonicalization is the `bracketKeys` lookup whose miss is the unknown-key rejection, and the duplicate check operates on canonical keys, so canonicalization must run first.
  Empty segment: a segment that is the empty string is rejected with exactly `modelspec: empty param in spec %q`, formatted with the full spec string. This replaces the coverage the deleted `no '=' separator` check used to give incidentally.
  Bare branch, no `=` in the segment: the whole segment is the value and the canonical key is `effort`. Validate the segment with `validateCharset` using the kind string `param value` and the `isModelIDChar` predicate — the same charset and the same kind string any `key=value` value gets. Do not validate it against any effort vocabulary. Remove the current `no '=' separator` error entirely; it is no longer reachable.
  Key-value branch: keep every existing check in its current relative order — reject an empty key with the existing `empty param key` error, reject an empty value with the existing `empty value for param key` error, charset-check the key as kind `param key` with `isIdentChar`, charset-check the value as kind `param value` with `isModelIDChar`. Then look the key up in `bracketKeys` to get the canonical key. A miss is the unknown-key rejection: keep the existing `modelspec: unknown param key %q in spec %q` wording for every key, and when and only when the offending key is exactly `version`, use instead `modelspec: unknown param key "version" in spec %q (the bracket version param is spelled "v")`. Every other unknown key keeps the unadorned message unchanged.
  Duplicate check: track, alongside the params map, the as-written segment that first set each canonical key. When a second segment resolves to a canonical key already present, reject with exactly `modelspec: duplicate param key %q in spec %q (segments %q and %q both set it)` — canonical key, full spec string, the first segment as written, then this segment as written. The `duplicate param key` substring is preserved deliberately; an existing reject case asserts it.
  Store under the canonical key, never the as-written spelling — `sonnet[v=4.5]` yields a `Params` map whose only key is `version`.
  Update two doc comments in this file that spell the bracket as key=value-only, so they describe the new grammar: `Parse`'s own doc comment, which lists the four recognized shapes, and `parseBracket`'s doc comment, which calls the bracket interior a comma-separated key=value list. The file's header comment needs no edit — it names the four shapes without spelling the bracket interior, so it stays true.
- **Commit:** `feat(modelspec): bare-effort bracket shorthand and v= version key`

### Card 3: parse tests for the new grammar

- **Context:**
  - `internal/modelspec/parse.go`
  - `internal/modelspec/modelspec.go`
- **Edits:**
  - `internal/modelspec/parse_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend `TestParse_Rejects`' table struct with one optional field, `alsoSubstr string`, and assert it in the loop body only when it is non-empty, using the same `strings.Contains` shape the existing `wantSubstr` assertion uses. Every existing row stays valid with the new field left unset.
  Add these accept cases to `TestParse_Accepts`, each with a descriptive `name`: `opus[high]` yielding alias `opus` and params `effort` of `high`; `opus[high,v=4.8]` yielding params `effort` of `high` and `version` of `4.8`; `sonnet[v=4.5]` yielding params `version` of `4.5`; `claude:claude-opus-5[high]` yielding engine `claude`, model `claude-opus-5`, params `effort` of `high`; `sonnet[effort]` yielding params `effort` of `effort`; `sonnet[4.5]` yielding params `effort` of `4.5`; and `opus[v=4.8,high]` yielding the same map as the combined case above, pinning order independence.
  Rewrite two existing accept cases in place, keeping each case's `name` and its expected `Params` map exactly as they are and changing only the input string: the case named `alias multiple params` moves its input from `sonnet[effort=high,version=4.5]` to `sonnet[effort=high,v=4.5]`, and the case named `dotted version value` moves its input from `sonnet[version=4.5]` to `sonnet[v=4.5]`. The expected maps keeping their `version` key through both rewrites is the regression guard for the normalization decision.
  Delete the reject case named `param with no equals` outright. Its input moves to the accept table per the uniform-rule decision.
  Add these reject cases to `TestParse_Rejects`: `sonnet[version=4.5]` with `wantSubstr` of `unknown param key` and `alsoSubstr` of `the bracket version param is spelled "v"`; `opus[high,effort=max]` with `wantSubstr` of `duplicate param key` and `alsoSubstr` of `(segments "high" and "effort=max" both set it)`; `opus[high,max]` with `wantSubstr` of `duplicate param key`; `opus[v=4.8,v=4.5]` with `wantSubstr` of `duplicate param key` and `alsoSubstr` of `(segments "v=4.8" and "v=4.5" both set it)`; `opus[v=4.8,version=4.8]` with `wantSubstr` of `unknown param key`, which is the regression guard for the check-order inversion and must not be allowed to pass on a duplicate-key error; `opus[high,]` with `wantSubstr` of `empty param in spec`; `opus[,high]` with the same substring; `opus[a,,b]` with the same substring; `opus[high!]` with `wantSubstr` of `invalid character`; and `opus[high, v=4.8]` with `wantSubstr` of `whitespace character`, pinning that the shorthand did not soften the whitespace rule.
  Change no other pre-existing case in either table. The case named `unknown param key` in particular keeps its unadorned expectation, because the migration hint fires only for the key `version`.
  Update this file's header comment only if the added struct field makes its description of the reject table inaccurate.
- **Commit:** `test(modelspec): cover bare-effort shorthand and v= rename`

### Card 4: vocabulary-sync test

- **Context:**
  - `internal/modelspec/modelspec.go`
- **Edits:** none
- **Creates:**
  - `internal/modelspec/modelspec_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create a new test file in package `modelspec` holding one test that asserts the stated invariant between the two closed vocabularies: every value in `bracketKeys` is a key in `knownParams`.
  Range over `bracketKeys` and fail the test naming both the offending bracket spelling and the canonical key it maps to when that canonical key is absent from `knownParams`.
  Do not assert the reverse containment. A canonical param settable only through `Entry.Defaults`, with no bracket spelling at all, is a legitimate future shape and must not fail this test.
  Give the file a header comment in the repo's existing style stating what it covers.
- **Commit:** `test(modelspec): pin bracketKeys-to-knownParams containment`

### Card 5: pinned contract doc

- **Context:**
  - `internal/modelspec/modelspec.go`
  - `internal/modelspec/parse.go`
- **Edits:**
  - `contracts/specs/llm-model-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Edit the Grammar section in four places, and four `version` mentions further down in three places — one of the four mentions is deliberately left alone.
  The alias-form grammar line, currently the fenced one-liner spelling the bracket as a comma-separated `key=value` list, becomes a comma-separated list of items with the item production stated on the same line: an item is either `key=value` or a bare effort token. Keep it a single fenced line. Reject the alternation-inside-the-bracket form — it reads as one item of either shape and hides that the comma list mixes both. This exact wording is restated in the package doc of `internal/modelspec/modelspec.go` by card 1; the two must match.
  The bullet immediately under it, which currently says each `key=value` overrides that parameter for this spec only, is extended to say a bracket item is either `key=value` or a bare token, and that a bare token means effort.
  The escape-form grammar line, currently spelling its bracket the same `key=value` way, takes the same item-list form. State the item production once, at the alias-form line, and reference it here rather than repeating it.
  The yaml example block under the Grammar section gains one line showing the shorthand spelling beside the existing explicit-effort reviewer line, so both spellings are visibly legal.
  In the Pinning section, the per-spec bullet's example `sonnet[version=4.5]` becomes `sonnet[v=4.5]`, and the sentence two lines below it about combining `version=` with a full model id re-spells that param as `v=`. Both describe the bracket an operator types.
  Leave the sentence about the provider engine translating the generic `version` param unchanged — it names the resolved param key the provider engine reads, not a bracket spelling.
  In the Provider seam section, the list item reading `version=` id translation drops the equals sign and keeps the word, becoming `version` id translation. The provider side never sees `v`, so re-spelling it there would document a spelling that does not exist on that side of the seam.
  Follow the repo's semantic-line-break convention for every line touched: one sentence per line, breaks at internal independent-clause boundaries, never a fixed-column wrap.
- **Commit:** `docs(spec): pin bare-effort shorthand and v= bracket key`

### Card 6: module overview doc

- **Context:**
  - `contracts/specs/llm-model-spec.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the shuttle module bullet, the parenthetical stating that consumers drive the version pin via the model-spec notation's `version=` param re-spells that param as `v=`. That parenthetical is the only occurrence of the bracket spelling in this file; change nothing else in the bullet and nothing elsewhere in the file.
  The module table and the execution stack are unchanged by this task, so no other section of this file moves.
- **Commit:** `docs(overview): re-spell model-spec version bracket param as v=`

## Batch Tests

`verify:` is `go test ./internal/modelspec/...`, scoped to the one package every code card touches.
That package's test files are `parse_test.go`, `registry_test.go`, `load_test.go`, `template_test.go`, `leaf_enforcement_test.go`, and the `modelspec_test.go` card 4 creates — all under `internal/modelspec`.
Only the first and the new one change;
the other four are expected to stay green untouched and are the local regression signal.
`registry_test.go` in particular constructs `Spec` values with a literal `version` params key, bypassing `Parse` entirely, so it tests `Resolve`'s precedence against the canonical key and stays correct under the normalize-to-`version` decision.
If implementing this batch appears to require editing `registry_test.go`, the normalization was implemented wrong — treat it as a signal, not a test to adjust.
The two markdown cards have no runnable surface of their own and are covered by the same gate only in the sense that they must not break the build, which they cannot.
Cross-package coverage — the one risk being a consumer that depended on the deleted `no '=' separator` error string — comes from the overview's module-wide `go build ./...` at the batch boundary and from `pipeline.done_gate`'s repo-wide `go test ./...` before the task is marked done.
