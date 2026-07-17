# Batch: revert-and-verify

```yaml
task: 'Spike: structured Go reference/call-graph lookup (go/packages / gopls)'
batch: revert-and-verify
number: 4
cards: 1
verify: go build ./...
depends-on: [3]
```

## Batch Scope

The mandatory final step from `_mill/discussion.md` → Testing → "Final revert-and-verify
step": remove every throwaway artifact so the branch's product diff against `main` is
**doc-only**, leaving only `docs/research/codeintel-spike.md`. This is the machine-checkable
guard that the "deleted before merge" assertion needs (Shared Decision
`throwaway-discipline`). No new behaviour — pure removal + dependency cleanup.

## Cards

### Card 8: Revert harness, `go.mod`/`go.sum`, `.lsp.json`; assert doc-only diff

- **Context:**
  - `_mill/discussion.md`
  - `docs/research/codeintel-spike.md`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:** none
- **Deletes:**
  - `tools/codeintel-poc/main.go`
  - `tools/codeintel-poc/gopackages.go`
  - `tools/codeintel-poc/callers.go`
  - `tools/codeintel-poc/gopls.go`
  - `tools/codeintel-poc/callgraph.go`
  - `.lsp.json`
- **Moves:** none
- **Requirements:** Delete the entire `tools/codeintel-poc/` directory (all five source files
  listed above) and the repo-root `.lsp.json`. Then run `go mod tidy`, which drops
  `golang.org/x/tools` from both `go.mod` and `go.sum` now that nothing imports it (revert
  both). Confirm the module still builds and vets clean: `go build ./...` and `go vet ./...`.
  Finally assert the **doc-only product diff**: `git diff main...HEAD --name-only --
  ':(exclude)_mill/'` must list **exactly** `docs/research/codeintel-spike.md` and nothing
  else (the `_mill/` task-state is tracked but excluded from this check; `.scratch/` and
  `.vscode/settings.json` are gitignored). If any other product file appears in that diff, it
  is a task failure — fix it (finish reverting) before this card is done. Do not delete or
  modify anything under `_mill/`.
- **Commit:** `chore(codeintel-poc): revert throwaway harness, deps, and .lsp.json`

## Batch Tests

`verify: go build ./...` (Go native runner, no `PYTHONPATH=` prefix) — a whole-module compile
confirming the revert left the repo in a clean, buildable state with `golang.org/x/tools`
removed. This is intentionally broader than the earlier batches' `./tools/codeintel-poc/`
scope because the harness no longer exists and the point of this batch is that the *rest of
the module* is unaffected. The doc-only diff assertion in card 8 is the batch's other
acceptance check.
