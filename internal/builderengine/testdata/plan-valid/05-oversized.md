---
oversized: true
---

# 05 — oversized: rewrite boardengine's row pipeline in one atomic pass

## Intent

The row pipeline's data model changes shape end-to-end (row struct, mapper, and every
consumer) with no compiling intermediate state possible inside a normal-sized batch;
this batch is flagged `oversized: true` so the orchestrator spawns the large-context
implementer role, per plan-format.md's oversized-batch escape mechanism.

## Scope

- 05-oversized.md

## Cards

### Card 1 — rewrite the pipeline

**What:** Replace the row pipeline's data model and every consumer in one pass.
**Where:** 05-oversized.md

## verify:

go build ./... && go test ./...
