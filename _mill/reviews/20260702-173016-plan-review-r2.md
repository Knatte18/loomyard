MILL_REVIEW_BEGIN
# Review: Build internal/mux: the window to the world (overlay + strands + render) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-02
```

## Findings

### [BLOCKING] Batch 4: overlay card precedes its parsers
**Location:** 04-muxengine-carrier, cards 10 (overlay.go) → 11 (parse.go)
**Issue:** Card 10's `listPanes`/`windowSize`/`paneIDsTopToBottom` methods call `parsePaneList`/`parseWindowSize`/`parsePaneOrder`, but those are only created in card 11 — so `go build ./internal/muxengine/...` fails at the card-10 boundary (a forward reference the plan's own batch-5 discipline forbids). Card 10's Context also omits parse.go.
**Fix:** Emit the pure parsers (and `LivePane`) before the overlay methods that call them — reorder card 11 ahead of card 10, or move the parser-calling methods into the parse card.

### [BLOCKING] Batch 3: height card forward-references focus + layout symbols
**Location:** 03-render, card 7 (height.go)
**Issue:** `stackHeights` returns `[]placement` (defined in card 5's layout.go) and the shrink-ancestor rule needs `isAncestor` (defined in card 8's focus.go, which is sequenced *after* card 7). Card 7 therefore both forward-references card 8 and lists neither layout.go nor focus.go in its Context — it won't compile at the card-7 boundary and the implementer can't read the referenced types.
**Fix:** Order focus.go (card 8) before height.go (card 7), and add render/layout.go + render/focus.go to card 7's Context (or define the ancestor check inline in card 7).

### [NIT] Rules named return shadows focusTarget function
**Location:** 03-render, card 9 (rules.go)
**Issue:** The signature `(layout string, focusTarget string, err error)` names a return `focusTarget`, which shadows the package function `focusTarget(ordered)` the card tells Rules to call — `focusTarget(ordered)` then calls a string and won't compile.
**Fix:** Rename the return (e.g. `focus string`) or the helper (`resolveFocus`).

### [NIT] Batch 4 depends-on lists logger unnecessarily
**Location:** 00-overview batches block / 04-muxengine-carrier
**Issue:** Batch 4 declares `depends-on: [1, 2, 3]`, but no card in it consumes `internal/logger` (batch 2).
**Fix:** Drop 2 from batch 4's `depends-on` (or add a logger use if intended).

## Verdict

REQUEST_CHANGES
Two intra-batch forward references (batches 3 and 4) break per-card build; reorder and fix Context.
MILL_REVIEW_END
