# Batch: spawn-observability-guard

```yaml
task: "Audit internal/logger coverage across spawn/hard-error paths"
batch: "spawn-observability-guard"
number: 5
cards: 1
verify: go test ./cmd/lyx/
depends-on: [1, 3]
```

## Batch Scope

This batch converts the audit's spawn table from a snapshot that rots into an invariant that holds: a new tree-wide guard test at `cmd/lyx/spawnobservability_test.go` that fails any production file containing a real `exec.Command`/`exec.CommandContext` call which neither imports `internal/logger` nor carries an allowlist entry with a written reason.
It is one batch and one card because the guard file and the `cmd/lyx/tierpurity_test.go` allowlist entry it requires must land together — the guard carries the banned `exec.Command`/`exec.CommandContext` tokens as its own scan data, so the Test Tier Purity Invariant guard fails the moment the file exists without the entry.

It depends on batch 1 (the audit document records the allowlist's reasons, and the guard's header comment is where the mechanism and its blind spot are documented rather than in CONSTRAINTS.md) and on batch 3 (until every `add`-verdict spawn site has its `logger` import, the guard fails by design, and a batch whose own `verify:` cannot pass is not a landable batch).

Batch-local decision differing from `## Shared Decisions`: this batch adds no production log line at all, so `log-line-style` and `level-policy` do not bind it. What it does add is the only mechanical enforcement in the task — the hard-error half of the audit gets none, deliberately.

## Cards

### Card 14: Add the spawn-observability guard and its tier-purity allowlist entry

- **Context:**
  - `cmd/lyx/checkedcall_test.go`
  - `cmd/lyx/ghguard_test.go`
  - `cmd/lyx/uncontainedwrite_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `cmd/lyx/testmain_test.go`
  - `internal/githubclient/leaf_enforcement_test.go`
  - `internal/gitexec/gitexec.go`
  - `internal/gitkit/gitkit.go`
  - `internal/githubclient/token.go`
  - `internal/githubclient/doc.go`
  - `internal/reedengine/doc.go`
  - `internal/reedengine/attach.go`
  - `internal/hubforge/hub.go`
  - `cmd/testtiming/main.go`
  - `manifest/designs/logger-coverage.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
- **Creates:**
  - `cmd/lyx/spawnobservability_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `cmd/lyx/spawnobservability_test.go` in `package main`, following the walk-plus-allowlist shape every sibling guard in `cmd/lyx/` uses.

  **Header comment.** Open with a file header naming the invariant it enforces (CONSTRAINTS.md's **Live-Substrate Spawn Observability**) and covering three things a later reader must not undo:

  - *AST, not substring, and why that diverges from the siblings.* Every other guard in this package is a raw-substring scan. This one must not be: three production files hit the `exec.Command` substring with zero real calls — `internal/githubclient/doc.go`, `internal/reedengine/doc.go`, and `internal/reedengine/attach.go`, all prose in doc comments. A substring guard would demand permanent allowlist entries for files that spawn nothing, which reads as "these spawn without logging, and we accepted it" — the opposite of the truth. State that `internal/githubclient/leaf_enforcement_test.go` is the in-repo `go/parser` precedent, and that `cmd/lyx/tierpurity_test.go` deliberately does *not* strip comments because there a mention genuinely is the scan data. Say explicitly that "harmonising" this guard into a substring scan would silently reintroduce three phantom violations.
  - *The known blind spot.* File-level import presence is coarse: a file that imports `internal/logger` for an unrelated line and spawns a process unlogged still passes. The guard catches the regression shape that actually occurs — a brand-new spawn in a file with no logging at all — and does not claim more. Match the candour of `cmd/lyx/checkedcall_test.go`'s own blind-spot section.
  - *Why the hard-error half gets no guard.* The audit's outcome-switch table is document-only. A new unlogged `exec.Command` is nearly always a real miss, so a file-level check has high signal; a new outcome-switch branch may legitimately return normally for the caller to branch on, so the same check would fire on correct code and train the next author to reach for the allowlist. Point at `manifest/designs/logger-coverage.md` for the full argument.

  **Detection.** Resolve the module root via `go env GOMOD`, mirroring `cmd/lyx/checkedcall_test.go` and `cmd/lyx/ghguard_test.go`, and skip cleanly (`t.Skip`) when the `go` toolchain is not on PATH or `go env GOMOD` is empty, exactly as those two do.
  Walk `internal/` and `cmd/` for non-`_test.go` `.go` files.
  For each file, parse it with `go/parser` and detect a spawn as an `*ast.CallExpr` whose `Fun` is an `*ast.SelectorExpr` whose `X` is an `*ast.Ident` matching the file's own `os/exec` import name (respecting an import alias, defaulting to `exec`) and whose `Sel.Name` is `Command` or `CommandContext`.
  A file with no `os/exec` import has no spawn site by construction.
  Detect the `internal/logger` import from the same parsed file's import list, matching the full path `github.com/Knatte18/loomyard/internal/logger`.

  **Allowlist.** Declare a package-level `map[string]string` keyed by module-relative, slash-separated file path, value a one-line reason, in the style of `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` and `cmd/lyx/sandbox_coverage_test.go`'s `excludedModules`.
  Normalise walked paths with `filepath.ToSlash` before any comparison — `filepath.WalkDir` yields backslash paths on Windows, the primary dev OS.
  It carries exactly five entries, and the reasons must distinguish the two kinds:

  - `internal/gitexec/gitexec.go` — structurally barred: `logger` imports `lyxcwd`, which imports `gitexec`, so the import is a cycle; `gitexec.Run` already returns a `*GitError` carrying args, dir, exit code and stderr;
  - `internal/gitkit/gitkit.go` — structurally barred by the gitkit Leaf Invariant's pinned import list;
  - `internal/githubclient/token.go` — structurally barred by the GitHub Auth Invariant's leaf allowlist, enforced by `internal/githubclient/leaf_enforcement_test.go`; logged at both callers instead;
  - `internal/hubforge/hub.go` — not governed: test-fixture builder, not a path reachable from a `lyx` command;
  - `cmd/testtiming/main.go` — not governed: test-timing harness.

  State in a comment above the map that the first three are exemptions inside the rule (they owe a written reason) and the last two are outside it (the walk reaches them, but they were never governed) — the asymmetry CONSTRAINTS.md's sharpened invariant text draws.
  `tools/` sites need no entry: they lie outside the walk.

  **Vacuous-scan floor.** Fail if fewer than 200 files were scanned, mirroring `checkedCallMinScannedFiles` in `cmd/lyx/checkedcall_test.go`, so a misconfigured walk fails loudly rather than passing on a near-empty scan.

  **Failure message.** Name the offending file and instruct the author to either import `internal/logger` and log the spawn, or add an allowlist entry in this file with a reason.

  **Vacuity self-test.** Add a second test function in the same file that exercises the detection helper directly against small in-memory Go sources rather than the real tree: one source with a genuine `exec.Command` call and no `logger` import (must be reported), one with the same call plus the `logger` import (must not), one whose only `exec.Command` occurrence is inside a doc comment (must not — this is the phantom-violation case the AST choice exists to avoid), and one using an aliased `os/exec` import (must be reported).
  Structure the detection as a helper taking source bytes or a parsed `*ast.File` so this test can call it without touching the filesystem.

  In `cmd/lyx/tierpurity_test.go`, add a fourteenth entry to the `allowedSpawners` map, keyed `"cmd/lyx/spawnobservability_test.go"`, with a reason in the style of the thirteen existing entries: it contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data and resolves its scan root via `go env GOMOD` (Live-Substrate Spawn Observability guard).
  Change nothing else in that file — not `bannedTokens`, not `knownTierTags`, not the walk.
- **Commit:** `test(cmd/lyx): add the spawn-observability guard`

## Batch Tests

`verify: go test ./cmd/lyx/` runs the whole `cmd/lyx` untagged guard suite.
The scope is the single package this card touches, and it is the right scope rather than a narrower `-run` filter because the card's two halves are checked by two different tests in that package: the new `TestSpawnObservability…` functions in `cmd/lyx/spawnobservability_test.go`, and `cmd/lyx/tierpurity_test.go`'s own walk, which fails the moment the new file exists without its `allowedSpawners` entry.
Running the package also picks up every sibling guard, which is the point — `cmd/lyx/` is where a new test file most plausibly trips an unrelated tree-wide check (`registration_test.go`, `helptree_test.go`, `tiersleep_test.go`), and this is the batch that adds one.

This card is the task's clearest TDD case, and `_mill/discussion.md`'s Testing section makes it explicit: the test *is* the deliverable.
Because this batch depends on batch 3, the guard is written against a tree where every `add`-verdict site already logs, so it is expected green on first run.
The completeness check the discussion asks for is therefore performed the other way round: after the guard passes, temporarily revert one batch-3 log-import (or add a bare `exec.Command` to a package with no logging) and confirm the guard fails on exactly that file, then restore.
The vacuity self-test required above pins the same property permanently, without a manual step — including the doc-comment case, which is the one a substring guard would get wrong.

Any site the guard flags that `manifest/designs/logger-coverage.md` does not mention is a row the survey missed; treat that as a plan defect to surface, not as a new allowlist entry to write.

The module-wide `verify:` in the overview (`go build ./... && GOOS=windows go build ./...`) runs at the batch boundary.
The repo-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) is what proves the guard does not disagree with any package outside `cmd/lyx/`, and what runs `cmd/lyx/crosscompile_test.go` over batch 3's two Windows-tagged edits.
