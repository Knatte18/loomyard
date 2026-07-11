# 04 — refactor-b: finish the mapper extraction

## Intent

Second half of the refactor chain started in batch 03: rewires every call site onto the
extracted mapper and restores a green build. This batch runs the chain's real verify:.

## Scope

- 04-refactor-b.md

## Cards

### Card 1 — rewire call sites

**What:** Point every caller at the extracted mapper; delete the old inline
implementation.
**Where:** 04-refactor-b.md

## verify:

go build ./...
