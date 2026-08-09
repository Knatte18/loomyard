# Batch: probe and clone flip

```yaml
task: 'fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)'
batch: 'probe and clone flip'
number: 2
cards: 5
verify: go build ./... && go test -tags integration ./internal/fabricengine/
depends-on: [1]
```

## Batch Scope

This batch delivers the pre-hub probe, the `CloneHub` signature change to an options struct, the effective-URL resolution wired into the clone flow, the old-order bootstrap guard, the folding of `--reset` into the engine, and every mechanical call-site update needed to keep the tree compiling and the existing suite green.
It is one batch because the signature change is a compile-breaking edit: the engine, the CLI call site, the sandbox launcher, and thirteen test invocations must all move together or nothing builds.

The external interface batch 3 consumes: `CloneOptions` (with `ForceBootstrap`), and `CloneResult.WarpURL` / `CloneResult.WarpBindingRecorded`.
Batch 4 consumes the probe's observable behaviour (the `.lyx-clone-probe-` directory, the error prefixes, and the record written at the board root).

Batch-local decision: this batch flips `internal/fabriccli/clone.go`'s positional parsing and its usage string, but does NOT add the `--force-bootstrap` flag or touch the `Long` help text — that is batch 3's job.
Until then the CLI passes `ForceBootstrap: false`, which is the correct default and leaves the guard fully armed.

## Cards

### Card 3: the pre-hub weft probe

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/warpbinding.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/warpprobe.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New file in `package fabricengine` importing `fmt`, `os`, `path/filepath`, `strings`, and `github.com/Knatte18/loomyard/internal/gitexec`.
  File-level comment explaining why the probe exists: the hub is named after the warp repo, so in the one-argument form there is no hub path — and therefore no board worktree — until the warp URL is known, and git offers no porcelain that reads one file from a remote without a local repo directory.

  Declare `const warpProbeDirPrefix = ".lyx-clone-probe-"`.
  This is a `MkdirTemp` prefix, not a `_lyx`/`.lyx` path token, so it does not fall under the Lyxdirs Single-Declarer Invariant and `TestEnforcement_GeometryLiterals` (exact-equality matching) does not trip on it.
  Say so in the const's doc comment.

  Declare the result type:

  ```go
  type warpProbeResult struct {
  	RecordedWarpURL   string
  	Found             bool
  	WeftLooksLikeWeft bool
  }
  ```

  with per-field comments: `Found` is true only when `.lyx-warp` is present in the probe's HEAD commit; `WeftLooksLikeWeft` is true when the candidate has an unborn HEAD (a genuinely empty weft remote) or its HEAD commit carries `.lyx-anchor` at the root.

  `func probeWeftBinding(cwd, weftURL string) (warpProbeResult, error)` performs, in order:

  1. `probeDir, err := os.MkdirTemp(cwd, warpProbeDirPrefix)`, wrapping a failure as `create probe directory in %s: %w`.
     `defer func() { _ = os.RemoveAll(probeDir) }()` immediately, so the probe is removed on every exit path including error.
     The probe lives in `cwd`, never `os.TempDir()`: `cwd` is provably writable (the hub is about to be created there) and it avoids system-temp variance across platforms.
  2. Clone the weft candidate shallowly: `gitexec.RunGit([]string{"clone", "--depth", "1", "--filter=tree:0", "--no-checkout", "--single-branch", filepath.ToSlash(weftURL), filepath.ToSlash(filepath.Base(probeDir))}, cwd)`.
     A non-nil error, or a nonzero exit code, is a hard error carrying git's stderr verbatim behind the prefix defined below.
     Warnings on stderr with a zero exit code are NOT a failure — against a local bare repo path git ignores `--filter` and `--depth` and warns, and every fixture in this repo is exactly that case.
  3. `gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", "HEAD"}, probeDir)`.
     A nonzero exit is the unborn-HEAD case: return `warpProbeResult{Found: false, WeftLooksLikeWeft: true}, nil` — the genuinely empty weft remote that `ensureBoardWorktree`'s orphan-create path already supports.
     A non-nil error (git could not be run at all) is a hard error.
  4. `gitexec.RunGit([]string{"ls-tree", "HEAD", "--name-only", "--", WarpBindingFileName}, probeDir)`.
     A non-nil error or a nonzero exit is a hard error.
     Empty trimmed stdout means the record is absent; non-empty means present.
  5. When present: `gitexec.RunGit([]string{"show", "HEAD:" + WarpBindingFileName}, probeDir)`.
     Any error or nonzero exit at this point is a hard error, never an absence — presence was already proved in step 4.
     Set `RecordedWarpURL` to the `strings.TrimSpace`d stdout and `Found` to true.
     An empty-after-trim value is treated as absent (`Found: false`), matching `readWarpBinding`'s own empty-is-absent rule.
  6. When the record is absent (from step 4 or the empty-after-trim case in step 5), run the same discriminator for the anchor: `gitexec.RunGit([]string{"ls-tree", "HEAD", "--name-only", "--", lyxcwd.AnchorFileName}, probeDir)`, hard-erroring on error or nonzero exit, and set `WeftLooksLikeWeft` to whether the trimmed stdout is non-empty.
     When the record IS present, `WeftLooksLikeWeft` is irrelevant (no bootstrap can occur) and may be left false.

  Every hard error is formatted as `probe weft %s: %s` with the weft URL and git's trimmed stderr (falling back to a description of the failing git subcommand when stderr is empty).
  Add a small unexported helper for that prefixing rather than repeating the format string at each site.

  Do not import `internal/configsync` or anything outside the existing `fabricengine` import set plus `internal/lyxcwd`, which `clone.go` already imports.
- **Commit:** `feat(fabricengine): add the pre-hub weft binding probe`

### Card 4: CloneHub options struct, effective-URL resolution, guard, and reset folding

- **Context:**
  - `internal/fabricengine/warpbinding.go`
  - `internal/fabricengine/warpprobe.go`
  - `internal/fabricengine/weftpaths_test.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace `CloneHub`'s four positionals with an options struct.
  Declare, above `CloneResult`:

  ```go
  type CloneOptions struct {
  	WeftURL        string
  	WarpURL        string
  	Subpath        string
  	Reset          bool
  	ForceBootstrap bool
  }
  ```

  with per-field doc comments: `WeftURL` is required; `WarpURL` is optional and resolved from the recorded binding when empty; `Reset` tears down an existing hub before cloning; `ForceBootstrap` bypasses the old-order guard and is ignored outside the bootstrap path.
  Note in the struct's doc comment why this is a struct and not five positionals: two adjacent optional URL strings are exactly the shape that produces silent argument-order bugs, and the argument order is what this change is flipping.

  Add two fields to `CloneResult`: `WarpURL string` (the effective warp URL actually cloned, whether supplied or derived) and `WarpBindingRecorded bool` (true only when this clone wrote the record).

  Change the signature to `func CloneHub(cwd string, opts CloneOptions) (CloneResult, error)` and rewrite the doc comment's phase list to describe the new step 0 and the form-dependent ordering.
  Return an error when `opts.WeftURL` is empty.

  The new flow, replacing today's steps 1-3:

  - Two-argument form (`opts.WarpURL != ""`), where the hub name is derivable with no network at all:
    1. `name := DeriveWarpName(opts.WarpURL)`; empty → today's `could not derive repo name from warp URL %s` error, unchanged.
    2. `hubPath := HubPath(cwd, name)`.
    3. If `opts.Reset`, `RemoveAll(hubPath)`, returning `reset: remove hub at %s: %w` on failure.
    4. Hub-exists check — today's `hub already exists at %s`, unchanged and still offline.
    5. Only now `probeWeftBinding(cwd, opts.WeftURL)`.
    6. `resolveEffectiveWarpURL(probe.RecordedWarpURL, probe.Found, opts.WarpURL)`.
    7. When `writeRecord` is true and `probe.WeftLooksLikeWeft` is false and `opts.ForceBootstrap` is false, return the old-order guard error (see below) — before any hub directory is created.
  - One-argument form (`opts.WarpURL == ""`), where the hub name is unknowable until the record is read:
    1. `probeWeftBinding(cwd, opts.WeftURL)` first.
    2. `resolveEffectiveWarpURL(...)` with an empty `supplied`.
       On the unbound error, prefix the weft URL so the message reads `weft %s has no recorded warp binding; supply the warp URL explicitly: lyx fabric clone <weft-url> <warp-url>`.
    3. `DeriveWarpName(effective)`; empty → an error that states the URL came from the record: `could not derive repo name from warp URL %s recorded in the %s binding on weft:main`.
    4. `hubPath := HubPath(cwd, name)`, then the `Reset` teardown, then the hub-exists check.
    The asymmetry is deliberate and must be commented in the code: an offline two-argument re-clone against an existing hub still fails with `hub already exists`, exactly as today; an offline one-argument invocation fails with `probe weft <url>:` instead, which is the irreducible cost of deriving the hub name from a remote fact.

  Track whether the effective URL was derived from the record.
  On the derive path, wrap step 5's warp clone failure so it states the source: `clone warp %s (from the %s binding on weft:main): %w`.
  On the supplied path leave today's error shape unchanged.

  The old-order guard error text, verbatim:
  `refusing to bootstrap %s as a weft: its history carries neither %s nor an empty tree — check the argument order, clone now takes <weft-url> [<warp-url>]`
  with the weft URL and `lyxcwd.AnchorFileName`.
  The guard fires only on the bootstrap path (`writeRecord == true`), which is reachable only in the two-argument form, so `ForceBootstrap` is structurally ignored everywhere else — no usage error, no warning.

  Steps 4 through 9 of the existing flow (hub `MkdirAll`, `<hub>/.lyx`, warp clone, hook install, weft clone, `suffixWeftPrimaryBranch`, `ensureBoardWorktree`, the stale `.fabric-anchor` guard, anchor adopt-or-create, `lyxcwd.Resolve`, `wireBoardLink`) stay exactly as they are, reading the effective warp URL wherever `warpURL` was read before and `opts.Subpath` wherever `subpath` was read before.

  Immediately after the anchor block writes `.lyx-anchor` (both the adopt and the create branch fall through to the same point), add the record write: when `writeRecord` is true, `writeWarpBinding(boardDir, effective)` and set `WarpBindingRecorded: true` on the result; a failure routes through `teardownHub` like every other post-step-5 failure.
  This is the clone-time backfill too: a re-clone of a pre-binding hub has `.lyx-anchor` already committed but no `.lyx-warp`, so `found` is false, `writeRecord` is true, and the record is written with no special casing.

  Populate `CloneResult.WarpURL` with the effective URL on every success path.
- **Commit:** `refactor(fabricengine)!: CloneHub takes CloneOptions and resolves the warp binding`

### Card 5: flip the CLI clone handler's positionals

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `runCloneWithReset`, replace the `len(args) != 2` check with an arity check accepting one or two positionals;
  zero or three-or-more is a usage error reading
  `usage: lyx fabric clone [--reset] [--subpath <rel>] [--force-bootstrap] <weft-url> [<warp-url>]`.
  Parse `weftURL := args[0]` and set `warpURL` to `args[1]` when present and `""` otherwise.

  Delete the whole `if reset { ... }` block: the `DeriveWarpName` call, the `HubPath` computation, and the `fabricengine.RemoveAll` call all move into `CloneHub` in card 4.
  The handler no longer calls `DeriveWarpName`, `HubPath`, or `RemoveAll` at all;
  drop the `fmt` import if it becomes unused.

  Replace the `CloneHub` call with the options form, passing `WeftURL`, `WarpURL`, `Subpath: subpath`, `Reset: reset`, and `ForceBootstrap: false`.
  Add a one-line comment noting that the flag itself arrives in the next change and that `false` keeps the old-order guard armed.

  Update the function's doc comment: the handler no longer tears down the hub itself, and the reset is now driven through `CloneOptions.Reset`.
  Leave the `configsync` / `Bolt` / `WireJunctions` sequence and the returned envelope exactly as they are — the envelope's new keys are batch 3's card.
- **Commit:** `refactor(fabriccli): clone takes <weft-url> [<warp-url>] and delegates reset`

### Card 6: flip both sandbox clone call sites

- **Context:**
  - `internal/fabriccli/clone.go`
- **Edits:**
  - `tools/sandbox/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Both `exec.Command` argument lists that spell `"fabric", "clone"` are warp-first today and must flip to weft-first.
  In `cloneRun`, the arguments become `"fabric", "clone", weftURL, warpURL`.
  In `fabricCloneRun`, they become `"fabric", "clone", fabricWeftURL, fabricWarpURL`.

  These are `exec.Command` argument lists, so an argument-order flip is completely invisible to the compiler — the enumeration rule is to grep every string-literal `"fabric", "clone"` occurrence in the repo and confirm exactly these two sites exist and both were flipped.
  Missing `cloneRun` would point the shared sandbox launcher at the core sandbox warp repo as a weft, firing the old-order footgun automatically on every sandbox build.

  Leave the URL constants, the hub names, and every surrounding comment unchanged;
  this card changes argument order only.
- **Commit:** `fix(sandbox): flip both fabric clone call sites to weft-first`

### Card 7: update the existing CloneHub test invocations

- **Context:**
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/fabricengine/clone_test.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/boardjunction_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Thirteen `CloneHub` invocations across these three files move to the options form.
  The mapping is purely mechanical: `CloneHub(parent, warp, weft, subpath)` becomes `CloneHub(parent, fabricengine.CloneOptions{WeftURL: weft, WarpURL: warp, Subpath: subpath})` — note that the warp and weft arguments swap position as well as becoming named fields, so a careless edit that keeps the old order silently inverts the test.
  In `clone_test.go` the call is unqualified (`package fabricengine`), so the struct literal is `CloneOptions{...}` with no package prefix; the other two files are `package fabricengine_test` and need the `fabricengine.` qualifier.

  Do not change any assertion, fixture, or test name in this card — the behaviour under test is unchanged and every one of these tests must still pass for exactly the reason it passed before.
  The one substantive consequence to check: these fixtures clone a weft bare repo that carries a committed readme file and no anchor marker, so the new old-order guard would reject them.
  Each of these call sites therefore needs `ForceBootstrap: true` UNLESS its weft fixture is empty (`makeEmptyBareRemote`) or already carries `.lyx-anchor` (seeded via `commitFileOnBranch`).
  Work through the call sites one at a time and set the field from the fixture actually used;
  add a short comment at each `ForceBootstrap: true` site saying the fixture is a seeded bare repo standing in for a weft, not a repo that has ever been a weft.

  Two files in this package mention `CloneHub` in comments only and call nothing — `internal/configcli/configcli_integration_test.go` and `internal/fabricengine/add_rollback_adopt_test.go` — do not touch them.
- **Commit:** `test(fabricengine): move CloneHub invocations to the options form`

## Batch Tests

`verify:` is `go build ./... && go test -tags integration ./internal/fabricengine/`.

The `go build ./...` half is the real gate for cards 4-6: the signature change touches `internal/fabricengine`, `internal/fabriccli`, and `tools/sandbox`, and a missed call site is a compile error rather than a test failure.

The `-tags integration` half is required because card 7 edits `internal/fabricengine/clone_adopt_test.go` and `internal/fabricengine/boardjunction_integration_test.go`, both of which carry `//go:build integration`;
without the tag those files would not be compiled at all and the card's edits would go unverified.
The same invocation also compiles and runs the untagged `internal/fabricengine/clone_test.go`.

Scope is limited to `internal/fabricengine` because no test outside it asserts on clone behaviour yet — `internal/fabriccli`'s clone tests are rewritten in batch 3, which is where they are verified.
The overview's module-wide `go build ./...` runs at the batch boundary as a second, whole-repo compile gate.
