MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-12
```

## Findings

### [BLOCKING:design] SeedConfig has no defined meaning on a real hub
**Section:** Technical context / blast-radius table ("`gitkit`, unchanged"), Assertion migration.
**Issue:** `lyxtest.SeedConfig` writes to `configengine.ConfigDir(repoDir)` = `<repoDir>/_lyx/config` and then runs `git add .` + `git commit` in that repo (`internal/lyxtest/lyxtest.go:38-58`); on a real hub `<worktree>/_lyx` is a weft junction excluded from warp's index, so the commit stages nothing and `MustRun` fatals — and there are 56 `lyxtest.SeedConfig(` call sites across 24 packages (reedcli 21, perchcli 6, fabricengine 19, shuttlecli 4, …), not just the two stand-in-hub sites the discussion retires.
**Fix:** State the replacement seeding path explicitly — which base (`hub.PrimeWorktree()` through the junction, `res.BoardDir`, or `WeftBase`), which repo receives the commit, and whether `gitkit.SeedConfig` keeps its commit step at all or `hubforge` gains a `SeedConfig`-equivalent accessor.

### [BLOCKING:design] Teardown never says how junction sites are discovered
**Section:** Decision "Junction-safe teardown lives in `hubforge`"; Reusable pieces.
**Issue:** The junction inventory listed (`<hub>/_portals/<slug>`, `<hub>/_launchers/<slug>`, `<worktree>/_lyx`, `<worktree>/.lyx`, `<worktree>/_board`) is slug-parameterised, but for fabricengine's own live-state tests the pairs are created by the verb under test, so `hubforge` does not know the slug set at cleanup time; `fslink.RemoveLinksIn` (`internal/fslink/fslink.go:48`) covers only immediate children of one directory.
**Fix:** Decide and record the discovery mechanism — a generic `fslink.IsLink` walk that must not descend into a link, versus enumerating worktrees from fabric and applying `RepoWiredNames` per worktree — and state the behaviour when a worktree directory was removed by hand.

### [NIT:consistency] `CopyRepo` caller allowlist names a package with no call site
**Section:** Decision "Migrate all 132 above-fabric sites"; Testing / `internal/gitkit`.
**Issue:** The guard pins `CopyRepo`'s callers to `internal/gitrepo` and `internal/lyxcwd`, but `internal/gitrepo` has zero `lyxtest.Copy*` call sites today (18× `MustRun` only), so the allowlist pre-authorises drift the task otherwise forbids.
**Fix:** Either pin to `internal/lyxcwd` alone or state why `gitrepo` is kept as a permitted future caller.

### [NIT:scope] Bare-template temp dir lifetime unstated
**Section:** Decision "Copy the bares, clone the hub".
**Issue:** `buildBareTemplate` allocates via `os.MkdirTemp` with no cleanup (`fabrictest/hub.go:73`), so the promoted factory leaks one template pair per test binary; the teardown decision covers only per-fixture junctions.
**Fix:** Say explicitly that the template dir is deliberately left to the OS temp reaper, or assign it an owner.

## Verdict

REQUEST_CHANGES
Config seeding and teardown junction discovery on a real hub are undecided.
MILL_REVIEW_END
