MILL_REVIEW_BEGIN
# Review: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] connKindLegacy referenced but never defined
**Location:** batch 7 card 26 (also used in batch 11 card 43's doc prose)
**Issue:** batch 5 card 17 defines `type connKind int` with only `connKindNative`/`connKindSupervised` (`iota`-based). Card 26's `acquireConnection`/`teardownConnection` return and switch on a third value, `connKindLegacy`, which no card anywhere adds to the type.
**Fix:** extend card 17 (or card 26 itself, the first consumer) to add `connKindLegacy` as the third `connKind` const.

### [BLOCKING] DetachBreakaway missing on Linux (compile break)
**Location:** batch 6 card 22 (defines it) / card 23 (`ensureSupervised` calls it)
**Issue:** card 22's Edits is only `proc_windows.go`/`proc_windows_test.go` — `proc_linux.go` never gets `DetachBreakaway`. Card 23 calls `proc.DetachBreakaway(cmd)` unconditionally from `ensureserver.go`, a cross-platform file with no GOOS split. Every existing `proc` function (`HideWindow`, `Detach`, and this same plan's own `IsAlive`, card 12) is defined on both platform files; this one isn't, so `go build`/`go vet` fails on Linux/macOS with `undefined: proc.DetachBreakaway`.
**Fix:** add `DetachBreakaway` to `proc_linux.go` too (e.g. alias to `Detach`, since `Setsid` already gives Unix the survive-parent-exit property `CREATE_BREAKAWAY_FROM_JOB` gives Windows).

### [BLOCKING] Untagged tests spawn processes, no allowlist entry
**Location:** batch 4 card 16 (`daemonstate_test.go`), batch 6 card 25 (`supervised_test.go`)
**Issue:** both files are specified "untagged, offline"/"spawn-free," yet each requires "spawn-and-wait"/"spawn-and-hold a real short-lived test subprocess" (mirroring `isalive_test.go`) for a confirmed-dead/alive PID. `internal/codeintelengine` is not on `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` (only the `internal/proc` prefix is, and it doesn't cover this package) — `TestTierPurity_UntaggedTestsSpawnNothing` fails once these land. The literal `exec.Command` substring these files (plus card 9's `toolchain_integration_test.go`) introduce also trips `hermeticenv_test.go`'s all-files git-spawn-token scan, requiring a `TestMain`+`HermeticGitEnv()` or an `allowedNonHermetic` entry that no card adds either.
**Fix:** add `internal/codeintelengine` (or the specific files) to both guards' allowlist maps with a reason, in the same commit that introduces the spawn.

### [BLOCKING] Symbol has no fake-transport test seam
**Location:** batch 8 card 30 (`Symbol`) / card 32 (`Symbol` tests)
**Issue:** card 32 requires driving `Symbol`'s candidate-handling logic against a hand-built client over `newPipeTransportPair`/`fakeServer`, "not through References's full detect/acquire machinery." Card 30 defines `Symbol(ctx, opts Options)` as one function that always calls `DetectLanguage`→`acquireConnection` (real spawn/toolchain resolve) — unlike `finalizeConnection`/`probe` (batch 5), no helper taking `*lspClient` directly is factored out, so there is no way to exercise "how Symbol interprets workspaceSymbol's result" without a real spawn.
**Fix:** factor Symbol's post-connection logic into an unexported helper (e.g. `symbolFromClient(ctx, client *lspClient, query string) ([]SymbolMatch, error)`) in card 30, mirroring `finalizeConnection`'s extraction.

### [BLOCKING] Entry fields used without registry.go in Context
**Location:** batch 5 card 19 (`ensureNative`), batch 7 card 26 (`acquireConnection`)
**Issue:** card 19's Context is `toolchain.go`/`lspclient.go`/`errors.go` and its Requirements use `entry.PinnedVersion`/`entry.Command`/`entry.InstallHint` — none of the three Context files define `Entry`. Card 26's Context is `ensureserver.go`/`errors.go` and its Requirements use `entry.HasNativeDaemon`, a field this plan's own card 1 adds to `registry.go`, likewise absent from Context/Edits there.
**Fix:** add `internal/codeintelengine/registry.go` to both cards' `Context:` lists.

### [NIT] Batch 6 card-count mismatch (says 5, has 4)
**Location:** batch 6 frontmatter (`cards: 5`)
**Issue:** batch 6 declares `cards: 5` but only 4 cards (22-25) exist in the file.
**Fix:** correct the frontmatter to `cards: 4`.

### [NIT] Parent codeintel command Short goes stale
**Location:** batch 9, cards 33-36
**Issue:** `Command()`'s parent `Short` reads "code intelligence lookups (references) across supported languages"; no card updates it after `definition`/`symbol` verbs are added, leaving it under-describing the module per CONSTRAINTS.md's CLI/Cobra help-accuracy obligation.
**Fix:** add a one-line update to the parent `Short` in card 34 or 36.

### [NIT] Card 23 forward-references card 24's error type
**Location:** batch 6 card 23 / card 24
**Issue:** card 23's Edits is only `ensureserver.go`, yet it constructs `ErrServerSpawnTimeout` and says "(add it to errors.go...)" — that addition is card 24's job, committed one card later, so card 23's own commit would not compile in isolation.
**Fix:** reorder cards 23/24, or note in card 23 that the type lands in the following card.

### [NIT] WorktreeRoot doc comment promises unwired CLI plumbing
**Location:** batch 7 card 26; batches 9-10
**Issue:** card 26's new `Options.WorktreeRoot` doc comment states "The CLI layer populates it from a resolved hubgeometry.Layout.WorktreeRoot," but no card in batch 9 or 10 actually sets `opts.WorktreeRoot` — harmless in V1 (native ignores it) but the doc comment is inaccurate the moment it lands.
**Fix:** wire `WorktreeRoot` into the CLI's `opts` construction (card 34), or soften the doc comment.

### [NIT] Roadmap Done entry inherits stale "refs" naming
**Location:** batch 11 card 46
**Issue:** `manifest/roadmap.md`'s Planned bullet, which card 46 trims into `## Done`, says `lyx codeintel references|definition|symbol`, but the shipped verb is `refs`. Card 45 already corrects this same naming in `docs/overview.md`; card 46 doesn't call out the equivalent fix for roadmap.md.
**Fix:** correct "references" to "refs" when trimming the bullet into Done.

## Verdict

REQUEST_CHANGES
Two guaranteed compile breaks (connKindLegacy, Windows-only DetachBreakaway) plus a tier-purity violation block landing as specified.
MILL_REVIEW_END
