# Batch: fixture-empty-stage-tolerance

```yaml
task: 'fabric: clone doesn''t commit written module configs'
batch: 'fixture-empty-stage-tolerance'
number: 2
cards: 1
verify: go test -count=1 -tags integration ./internal/hubforge/... ./internal/preflight/... ./internal/preflightshed/...
depends-on: []
```

## Batch Scope

This batch makes the three fixture sites that run a bare `git commit` at the weft prime tolerate an empty stage, by giving each one `--allow-empty`.
It is one batch, and one card, because the three sites are a single mechanically-enumerated class — every `gitkit.MustRun(..., "git", "add", ...)` followed by a bare `gitkit.MustRun(..., "git", "commit", ...)` run at the weft worktree root, minus the sites that write their own file before staging — and committing them together is what makes the class disposition auditable rather than "the ones we happened to notice".

It lands ahead of batch 3 rather than with it because `gitkit.MustRun` calls `tb.Fatalf` on any non-zero exit, so the moment batch 3 leaves the weft prime clean these three become hard fixture failures.
Batch 3 depends on this batch for exactly that reason.
The change is a no-op on today's tree — the nine untracked config files still stage — so this batch is independently verifiable in its own right.

There is no external interface;
batch 3 consumes only the fact that these three commits can no longer fail on an empty stage.

## Cards

### Card 2: allow an empty stage at the three weft-prime fixture commit sites

- **Context:**
  - `internal/gitkit/gitkit.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/doc.go`
  - `internal/fabriccli/pushbypass_integration_test.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/hubforge/seed.go`
  - `internal/preflight/preflight_integration_test.go`
  - `internal/preflightshed/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/hubforge/seed.go`, change `SeedConfig`'s final call from `gitkit.MustRun(tb, h.PrimeWeft(), "git", "commit", "-m", "hubforge: seed config")` to the same call with `"--allow-empty"` inserted immediately before `"-m"`.
  Leave the preceding `gitkit.MustRun(tb, h.PrimeWeft(), "git", "add", ".")` unchanged.
  Do not add a `git diff --cached --quiet` probe or any other conditional branch — the flag is the whole remedy.

  Extend `SeedConfig`'s doc comment, which already explains where its commit runs and why, with a paragraph explaining why the commit is `--allow-empty`: once `fabriccli.CloneAndWire` commits the module configs it materialises, the base clone leaves the weft prime clean, so a seeded override that is byte-identical to the just-committed reconciled file stages nothing and a bare `git commit` exits 1 — which `gitkit.MustRun` turns into a `tb.Fatalf` in a test that did nothing wrong.
  Say plainly that the cost is an occasional empty fixture commit that nothing observes.

  In `internal/preflight/preflight_integration_test.go`, apply the same one-flag change to the `gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "-m", "seed junctions")` call in `setupFixture`, leaving its preceding `"git", "add", "-A"` call unchanged.
  Update the existing comment above that add-and-commit pair so it also records why the commit must be allowed to be empty: after the clone-commit change the weft prime already arrives clean, `.lyx` is excluded through the weft repo's `.git/info/exclude`, and the `_extra` junction target materialises as an empty directory git does not track — so this pair becomes a no-op that must be allowed to succeed rather than deleted, because deleting it would silently drop the guarantee if a future fixture change reintroduces untracked weft content.

  In `internal/preflightshed/preflight_integration_test.go`, apply the identical change and the identical comment extension to its own `setupPreflightWrapperFixture`, whose comment already states that it mirrors `internal/preflight`'s fixture.

  Do not touch `internal/fabriccli/pushbypass_integration_test.go` — it writes a placeholder file under the durable lyx directory immediately before its `git add .`, so it always has something to stage and is not a member of this class.
- **Commit:** `test(hubforge,preflight): allow an empty stage at the weft-prime fixture commits`

## Batch Tests

`verify: go test -count=1 -tags integration ./internal/hubforge/... ./internal/preflight/... ./internal/preflightshed/...` runs the three packages this batch edits.
All three fixtures are `//go:build integration` (or build hubs via `hubforge.NewHub`, which the Test Tier Purity Invariant confines to tagged files), so the `-tags integration` flag is required for the edited code to be compiled at all — an untagged run would report a vacuous pass.

These three packages are the complete set: `internal/hubforge` owns the edited `SeedConfig` helper and is the only package that exercises it directly here, and the two preflight packages own the two edited fixtures.

The batch has no new test of its own.
`--allow-empty` on today's tree is a no-op — the nine untracked config files still stage, so every one of these commits still has real content — and the failure mode the flag closes is unreachable until batch 3 lands.
The regression test that proves the `SeedConfig` half (a redundant, byte-identical seed) is card 5 in batch 3, where it can actually fail without the flag;
see the overview's `seedconfig-trap-is-latent-not-live` Decision for the measurement behind that placement.
