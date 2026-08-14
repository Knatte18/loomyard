# `fabric` — independent review, round 8 (`opus-medium-r8`)

> Clean-room, unprimed review of the `fabric` module per `_mill/fabric-review-prompt.md`.
> This is the operator's stated FINAL round of the campaign, with no seeded residual — a general
> confidence sweep rather than a hunt for a known gap.
> Model/effort: Opus/medium.

## Status

Job 1 (review) IN PROGRESS. Findings and test observations are appended incrementally, per the
prompt's crash-resilience rule.

## What was tested

(appended incrementally as each command/scenario returns)

### Hermetic gates

- `go build ./...` — **PASS** (exit 0, no output).
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...` — **PASS** (exit 0, no output).

- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5` — **PASS**, all five packages `ok`.
- `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... -count=1` — **PASS** (exit 0), no FAIL and no substrate-corruption marker.

## Findings

### M1 (MEDIUM, CONFIRMED-by-trace) — `removeLaunchers`' launcher-directory removal is check-then-act on a nominal path, the exact R3 window the gate's two arbitrary-path executors closed

`internal/fabricengine/launchers.go:284-293`.

`removeLaunchers` runs the gate's full pipeline for the launcher DIRECTORY via a direct
`checkPathRequest(dirReq)` call, and then performs the act itself as a raw, nominal-path
`os.Remove(launcherDir)`:

```go
if err := checkPathRequest(dirReq); err != nil { ... }
removeErr := os.Remove(launcherDir)
```

It deliberately does not route through `removePath`, and the stated reason is sound — `removePath`'s
directory branch is `RemoveAll`, which would destroy foreign content the operator put beside the
launchers (`TestRemoveLaunchers_PreservesForeignContent`). But avoiding `removePath` also avoided
`removeContainedPath`, and with it the entire R3 fix. This site is therefore the one surviving
arbitrary-path removal in the package that still resolves its containment at one instant and unlinks
at a later one — precisely the window `destroy.go`'s own doc comment says the check alone cannot
close ("a symlink at an intermediate segment, dangling when the check ran and flipped
live-and-escaping before the executor acted, carried a gated removal outside the hub anyway").

**Failure scenario.** `AnchorRel` is a non-`.` subpath (say `backend`), so
`launcherDir = <hub>/_launchers/backend/<slug>`. A same-UID process toggles
`<hub>/_launchers/backend` between a dangling symlink and a symlink to `/victim` during
`lyx fabric remove <slug>`. With the link dangling, `refuseUncontainedPath` and `checkPathRequest`
both pass (`checkPathRequest` short-circuits to `nil` on an absent target via `os.Lstat`; the
ancestor-walk fallback in `resolveAncestorSymlinks` resolves the dangling tail lexically). The link
then flips live-and-escaping, and `os.Remove(<hub>/_launchers/backend/<slug>)` resolves `backend`
and unlinks `/victim/<slug>` — an out-of-hub deletion. Because `os.Remove` also removes plain files
and symlinks, the escaped target need not be an empty directory. On success the site appends
`KindPathRemoved` naming the **hub-relative** path the inode removed never was — the identical
false-success shape as R2's M3 and R3's M1.

Static (no-race) exploitation is refused: a pre-planted escaping symlink is caught by
`refuseUncontainedPath`. So this needs the same toggle race R3 reproduced, against the same threat
model R3 treated as real rather than theoretical.

**Suggested fix.** Route the act through the existing `removeContainedPath(launchersDir(l),
launcherDir, false)`. The non-recursive branch is `os.Root.Remove`, which the OS refuses on a
non-empty directory exactly as `os.Remove` does — so the preserve-foreign-content property is kept
verbatim — while component resolution and the unlink become one `openat` chain rooted at the
container. This also retires `launchers.go`'s `os.Remove(` entry from the destructive guard's
allowlist, which is the correct end state: the file should no longer need one.

### L1 (LOW, CONFIRMED-by-trace) — `pruneEmptyAncestors` relates and removes purely lexically, so a planted intermediate symlink walks it out of the hub

`internal/fabricengine/ancestors.go:103-129`, reached from `launchers.go:296` and `portals.go:112`.

The sweeper's boundary guard is `filepath.Rel(stop, cur)` over **nominal** strings — the lexical
comparison `containmentFailure` was rewritten away from in R2 — and its act is a raw
`os.Remove(cur)` on the nominal path. Both call sites walk the two hub-level structural containers
R7 identified as the attacker-plantable ones (`<hub>/_launchers`, `<hub>/_portals`).

**Failure scenario.** A multi-segment `AnchorRel` (e.g. `services/api`) makes
`start = <hub>/_launchers/services/api`, `stop = <hub>/_launchers`. A symlink planted at `services`
pointing to `/victim` makes the first `os.Remove(cur)` resolve `services` and remove
`/victim/api` — an out-of-hub removal. The loop then removes the `services` link itself and halts at
`stop`.

Materially weaker than M1 and correctly graded LOW: `os.Remove` on a directory is refused by the OS
the moment it is non-empty (the reason the guard allowlist gives), so only an EMPTY out-of-hub
directory can be destroyed; every error is swallowed; and nothing is appended to the mutation
record, so there is no false-success claim. It is nevertheless the last lexical-containment +
raw-nominal-act pair left in the package, and it sits on the same two containers the campaign has
now hardened on both the write and the delete side.

**Suggested fix.** Root the sweep: open an `os.Root` at `stop` and walk with `root.Remove(rel)`, so
both the boundary and the act resolve through the container's own handle. The lexical `Rel` guard
can stay as the loop's termination condition; what changes is that the removal itself can no longer
traverse an escaping component.
