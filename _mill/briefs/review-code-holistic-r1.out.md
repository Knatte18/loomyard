MILL_REVIEW_BEGIN
# Review: modelspec: bare-effort shorthand and version-to-v rename — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-13
```

## Findings

### [BLOCKING:consistency] escape-form grammar line repeats item production instead of referencing it
**Location:** `internal/modelspec/modelspec.go:17`
**Issue:** The overview's pinned "bracket grammar line" Shared Decision states the escape-form line "is the same shape ... and states the `item` production by reference rather than repeating it." `modelspec.go:17` reads `<engine>:<model-id>[item,item,...]        where item is  key=value  |  <effort>`, fully repeating the item production two lines below the alias-form line that already states it, whereas the contract doc's escape line (`contracts/specs/llm-model-spec.md:25`) correctly does it by reference: `where item is the same shape as above`.
**Fix:** Change `modelspec.go`'s escape-form line to reference the alias-form line (e.g. `where item is the same shape as above`) instead of repeating `key=value | <effort>` verbatim.

### [NIT:scope] shorthand yaml example placed beside the wrong existing line
**Location:** `contracts/specs/llm-model-spec.md:15-19`
**Issue:** Card 5 asks for the new shorthand example line to sit "beside the existing explicit-effort reviewer line" (`reviewer: opus[effort=max]`), but the added line (`implementer: sonnet[high]`) was placed beside the `implementer: sonnet[effort=high]` line instead.
**Fix:** If the literal placement was intentional, no action needed; otherwise move/add the shorthand example beside the `reviewer:` line as the card specifies.

## Verdict

REQUEST_CHANGES
One Shared-Decision text deviation (escape-form grammar line repeats rather than references the item production).
MILL_REVIEW_END
