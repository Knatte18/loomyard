MILL_REVIEW_BEGIN
# Review: modelspec: bare-effort shorthand and version-to-v rename

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [NIT:consistency] parse.go header does not say key=value
**Section:** Scope → `internal/modelspec/parse.go` **Issue:** The bullet claims three doc comments "describe the bracket as key=value-only", but `parse.go:1-4` only lists the four shapes as `alias[bracket]` / `engine:model[bracket]` — it carries no key=value prose, so it needs no edit. **Fix:** Narrow the claim to `Parse`'s comment (`:37`) and `parseBracket`'s (`:122`), or state that the header is touched only if its "rejects everything else" wording is re-read.

### [NIT:consistency] knownParams doc comment becomes false, not just incomplete
**Section:** Decisions → "Bracket key vocabulary is separate…" **Issue:** `modelspec.go:109-113` currently reads "the closed set of parameter keys **a bracket** or a registry Defaults map may use"; after the split the bracket is gated by `bracketKeys`, so the clause is wrong, yet the discussion prescribes only "a one-line pointer" rather than correcting it. **Fix:** State that the existing clause is re-scoped to `Defaults`/canonical `Params` in addition to gaining the `bracketKeys` pointer.

### [NIT:decision] Package-doc grammar line disposition is self-contradictory
**Section:** Scope → `internal/modelspec/modelspec.go` **Issue:** "package doc's grammar line … stay canonical but the grammar prose gains the shorthand" reads both ways for `modelspec.go:11-12` (`<alias>[key=value,...]`); a plan writer cannot tell whether that line is rewritten. **Fix:** Say explicitly whether the one-line grammar in the package doc admits the bare form, separately from the `Params["version"]` example staying canonical.

### [NIT:decision] Three spellings deferred to mill-plan
**Section:** Scope `:8` production; duplicate-key message punctuation; `TestParse_Rejects` second-substring mechanism **Issue:** Three open choices are delegated without a pick, one of them on a line the discussion itself calls "the pinned grammar" of a pinned contract doc. **Fix:** Pin the `:8`/`:24` production spelling here (the other two are test/message cosmetics with stated observable contracts and are fine to delegate).

### [NIT:scope] CONSTRAINTS.md disposition not stated for the new sync obligation
**Section:** Constraints **Issue:** The discussion declares a `bracketKeys` ⊆ `knownParams` **invariant** while CLAUDE.md/CONSTRAINTS.md require recording new cross-cutting invariants in the same commit; the Documentation Lifecycle bullet names only `contracts/specs/llm-model-spec.md` and `docs/overview.md`, leaving the omission implicit. **Fix:** State outright that the obligation is package-internal (test-enforced) and CONSTRAINTS.md's Modelspec Leaf Invariant is unchanged.

## Verdict

APPROVE
Decisions complete and source-accurate; five documentation-precision nits, none blocking plan writing.
MILL_REVIEW_END
