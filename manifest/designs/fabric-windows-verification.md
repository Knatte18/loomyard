# fabric: Windows path behaviour is unverified after six hardening rounds

> **Status: Someday.**
> Deferred deliberately — not because the gap is small, but because closing it needs a Windows host rather than a design.
> Filed as GitHub issue #147 (`bug`) by the fabric v2 crucible campaign's orchestrator and folded into the manifest here;
> the issue is closed, pointing at this file.

This document exists so that "fabric was hardened by six review rounds" is never read without the asterisk.

## The gap

`fabric` is explicitly cross-platform — `internal/fslink` exists precisely because Windows uses directory junctions where other platforms use symlinks.
**Six crucible rounds hardened fabric without executing a single line of it on Windows.**
Every round ran on Linux, and every round named this gap and carried it forward, reasoned about rather than driven.

Named explicitly, round after round, as reasoned-but-not-executed:

- **`lyxcwd.ValidateAnchorRel`** — the volume-rooted rejection (`C:\...`, `\\server\share`).
  Linux cannot produce those inputs, so the branch has never run against a real path.
- **`excludePatternFor`** — separator handling when composing `.git/info/exclude` patterns.
  Git wants forward slashes;
  `filepath` on Windows produces backslashes.
  Never executed.
- **`lyxcwd.samePath`** — case-insensitive comparison on Windows.
  The case-insensitive branch is dead code on Linux.
- **`internal/fslink`** — the junction path itself.
  Every link fabric creates on Windows goes through code no round has run.

Beyond the named four: the entire anchor/subpath mechanism — the campaign's number one concern throughout, and the source of the largest single group of findings — has only ever been verified on one platform.

## Why this is worse than a normal coverage gap

The campaign's eight data-loss defects were **all** found by driving real git against a real filesystem with hostile or dirty state;
the hermetic suite was green throughout and found none of them (see [internal/fabricengine](../../internal/fabricengine/doc.go)'s package doc, "The destruction chokepoint" section).
That is direct evidence that fabric's defects live exactly where platform behaviour lives: in path composition, link creation, and filesystem semantics.

Windows is where all three differ.
It is the platform most likely to hold a defect and the only one never driven.

The `remove ..` BLOCKING is a concrete illustration.
Its fix rejects any slug that `filepath.Clean` would rewrite.
`.` and `..` are the same two strings on both platforms, so the fix is portable — but that is a reasoned conclusion, not an executed one, and it is reasoning about `filepath` semantics that genuinely differ between platforms in other respects.

## What would close it

Running the existing suite on a Windows host is the minimum:

```
go build ./...
go vet ./...
go test ./...
go test -tags integration ./...
```

That alone would exercise `samePath`'s case-insensitive branch, `fslink`'s junction path, and `excludePatternFor`'s separator handling under real conditions.

Beyond the suite, the scenarios worth driving by hand on Windows are the ones the campaign found defects in on Linux:

- a `--subpath` anchored hub — junction targets, `_lyx`/`.lyx`/`_board` placement, the cwd gate refusing the warp root and its subdirectories;
- `add`/`remove` slug hygiene;
- `prune` / `clone --reset` ownership refusals.

Slice 13's live-state harness is the natural vehicle: once it exists, closing this item is largely a matter of running it on Windows rather than writing anything new.
That makes this item cheaper *after* slice 13, which is one reason it sits in Someday rather than ahead of it.

## The legitimate alternative answer

If Windows support is not actually a goal, that is a valid resolution — but it must be **decided and written down**, because `internal/fslink`'s existence currently asserts the opposite, and CLAUDE.md's own filesystem-links rule states the junction path is "the only link type guaranteed everywhere".
Deciding to drop Windows means retiring that claim too, not just skipping the test run.

## Related

- [internal/fabricengine](../../internal/fabricengine/doc.go) package documentation — the four slices of the fabric crucible follow-up campaign, all landed;
  slice 13's harness inherits this gap honestly rather than closing it.
- [warp-visibility.md](warp-visibility.md) — the other open item carrying a Windows-specific caveat (Developer Mode for symlinks, with a copy fallback).
- `internal/fslink` package documentation — the junction-vs-symlink contract this item would verify.
