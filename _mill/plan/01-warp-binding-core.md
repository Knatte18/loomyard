# Batch: warp-binding core

```yaml
task: 'fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)'
batch: 'warp-binding core'
number: 1
cards: 2
verify: go test ./internal/fabricengine/ -run 'TestNormalizeWarpURL|TestResolveEffectiveWarpURL|TestWarpBindingReadWrite|TestWarpURLTransportIdentity'
depends-on: []
```

## Batch Scope

This batch delivers the git-free core of the warp binding: the filename constant, the on-disk read/write pair, the URL normalizer, and the pure resolver that encodes the whole conflict rule.
It is one batch because all four live in a single new file and are provable without spawning git — which is exactly why they are written test-first here, ahead of every git-touching batch.

The external interface batch 2 consumes: `WarpBindingFileName`, `readWarpBinding`, `writeWarpBinding`, `normalizeWarpURL`, and `resolveEffectiveWarpURL`.
Batch 5 consumes the same four helpers plus `warpURLTransportIdentity`.

Batch-local decision: the tests are written before the implementation (card 1 before card 2).
Both cards land in the same batch so the batch ends compiling and green.

## Cards

### Card 1: TDD — table tests for the warp-binding core

- **Context:**
  - `internal/lyxcwd/anchor.go`
  - `internal/fabricengine/clone_test.go`
  - `internal/fabricengine/branchname_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/warpbinding_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write this file FIRST, before card 2's implementation.
  It is `package fabricengine` (in-package, so it can exercise the unexported helpers directly) and carries NO `//go:build` line — it is a Tier 1 file and must never name `gitexec.RunGit`, `exec.Command`, `exec.CommandContext`, or any `lyxtest.Copy*` helper, not even inside a comment or a string literal (Test Tier Purity Invariant, raw substring match).
  Use only `os`, `path/filepath`, `strings`, and `testing`.

  `TestNormalizeWarpURL` — a table test over `normalizeWarpURL`, covering at minimum:
  a bare `https://github.com/u/repo` unchanged;
  one trailing `/` stripped;
  one trailing `.git` stripped;
  `https://github.com/u/repo.git/` with both present reduced to the bare form;
  `https://GitHub.COM/U/Repo` lowercasing scheme and host while leaving the `/U/Repo` path case intact;
  the scp form `git@github.com:u/repo.git` left byte-identical apart from the `.git` strip, and therefore NOT equal to the https spelling of the same repo;
  the empty string returning the empty string;
  a POSIX local filesystem path such as `/home/u/fixtures/bare.git` reduced to `/home/u/fixtures/bare` with no case change;
  a Windows-style local path such as `C:/Code/Repo/` reduced to `C:/Code/Repo` with the drive letter and path case preserved byte-for-byte.

  `TestWarpURLTransportIdentity` — a table test over `warpURLTransportIdentity` asserting that `https://github.com/u/repo.git`, `http://github.com/u/repo`, and `git@github.com:u/repo` all collapse to the same identity string, while `https://github.com/u/other` does not.

  `TestResolveEffectiveWarpURL` — a table test over `resolveEffectiveWarpURL(recorded string, found bool, supplied string)` covering every row of the conflict rule:
  (absent, absent) returns an error whose text contains `has no recorded warp binding` and `lyx fabric clone <weft-url> <warp-url>`;
  (absent, supplied) returns the supplied URL with `writeRecord == true` and no error;
  (present, absent) returns the recorded URL with `writeRecord == false` and no error;
  (present, supplied, byte-identical) returns the supplied URL, `writeRecord == false`, no error;
  (present `https://github.com/u/r`, supplied `https://github.com/u/r.git/`) returns the supplied URL, `writeRecord == false`, no error — the normalized-match row;
  (present `https://github.com/u/r`, supplied `git@github.com:u/r.git`) returns an error whose text contains `refusing to re-point` and both URLs — the transport-swap row;
  (present `https://github.com/u/r`, supplied `https://github.com/u/other`) returns the same conflict error.
  Assert that on every error row the returned effective URL is the empty string and `writeRecord` is false.

  `TestWarpBindingReadWrite` — a round-trip test using `t.TempDir()` as a stand-in board directory:
  `readWarpBinding` on a directory with no `.lyx-warp` returns `("", false)`;
  `writeWarpBinding` then `readWarpBinding` returns the written URL with `found == true`;
  the bytes on disk are exactly the URL plus one trailing newline;
  a file whose content is whitespace only is treated as absent (`("", false)`), mirroring `readRecordedAnchor`'s empty-after-trim rule;
  a second `writeWarpBinding` over an existing file replaces its content rather than appending.

  Every unbound-weft and conflict message asserted here must match the exact strings card 2 produces — quote them in the table rather than substring-matching a fragment that could drift.
- **Commit:** `test(fabricengine): table tests for the warp-binding core`

### Card 2: implement the warp-binding core

- **Context:**
  - `internal/lyxcwd/anchor.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/warpbinding_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/warpbinding.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New file in `package fabricengine`, stdlib-only (`fmt`, `os`, `path/filepath`, `strings`).
  Open it with a file-level comment in the shape of `clone.go`'s, stating that this file owns the `.lyx-warp` warp-URL binding recorded once on `weft:main` beside `.lyx-anchor`, that it holds the warp URL only and never the subpath, and that the record is written to disk here but committed by the CLI layer through `Bolt`.

  Declare `WarpBindingFileName = ".lyx-warp"` as an exported const with a doc comment modelled on `lyxcwd.AnchorFileName`'s: a structural per-repo record, never a config or env override, distinct from the anchor (the anchor says *where in warp* lyx is rooted, this says *which warp repo* the weft pairs with).
  Exported because integration tests in `package fabricengine_test` and batch 5's reconcile code both need to name the file.

  `func readWarpBinding(boardDir string) (warpURL string, found bool)` — `os.ReadFile(filepath.Join(boardDir, WarpBindingFileName))`, `strings.TrimSpace`, returning `("", false)` on any read error and on an empty-after-trim result.
  Mirrors `readRecordedAnchor`.

  `func writeWarpBinding(boardDir, warpURL string) error` — `os.WriteFile` of `warpURL + "\n"` at mode `0o644`, wrapping any failure as `write %s: %w` with the full path.

  `func normalizeWarpURL(raw string) string` — trim surrounding whitespace, then strip exactly one trailing `/`, then exactly one trailing `.git`, then, only when the string begins with a `<scheme>://` prefix, lowercase the scheme and the host segment up to the first `/` after `://` and leave the remainder untouched.
  A string with no `<scheme>://` prefix (local paths, scp-form URLs) is returned with only the two trailing strips applied and no case change at all — a Windows drive letter must survive byte-for-byte.
  Order matters: strip the slash before the `.git`, so `repo.git/` reduces to `repo`.

  `func warpURLTransportIdentity(raw string) string` — used only to word a reconcile divergence detail, never to decide equality.
  Start from `normalizeWarpURL`, drop a leading `<scheme>://`, rewrite a leading scp-form `<user>@<host>:` into `<host>/`, and lowercase the whole result.
  Two spellings of the same repo over different transports must collapse to the same string.

  `func resolveEffectiveWarpURL(recorded string, found bool, supplied string) (effective string, writeRecord bool, err error)` — the whole conflict rule, pure and git-free:
  - `!found && supplied == ""` → `("", false, fmt.Errorf(...))` with the message `weft has no recorded warp binding; supply the warp URL explicitly: lyx fabric clone <weft-url> <warp-url>`.
    The caller prefixes the weft URL (see batch 2), so this function's own text must not attempt to name it.
  - `!found && supplied != ""` → `(supplied, true, nil)`.
  - `found && supplied == ""` → `(recorded, false, nil)`.
  - `found && supplied != "" && normalizeWarpURL(recorded) == normalizeWarpURL(supplied)` → `(supplied, false, nil)`.
    The supplied spelling is returned rather than the recorded one so the hub name and the URL actually cloned are derived from the same string; the record itself is left untouched either way.
  - otherwise → `("", false, fmt.Errorf("recorded warp binding %s does not match %s; refusing to re-point. If the warp repo moved, edit %s in the hub's _board worktree and commit.", recorded, supplied, WarpBindingFileName))`.

  Every one of these five identifiers stays unexported except `WarpBindingFileName`; batches 2 and 5 are in the same package.
  Do not add a `Bolt` method — `Bolt` stays a narrow git-verb handle.
- **Commit:** `feat(fabricengine): add the .lyx-warp warp-binding record helpers`

## Batch Tests

`verify:` runs `go test ./internal/fabricengine/` filtered to this batch's four test functions (`TestNormalizeWarpURL`, `TestResolveEffectiveWarpURL`, `TestWarpBindingReadWrite`, `TestWarpURLTransportIdentity`), all of which live in the single new file `internal/fabricengine/warpbinding_test.go`.
The scope is deliberately narrow: this batch adds a new file that nothing else references yet, so no other package's behaviour can change.
No `-tags integration` is needed — every test in this batch is git-free by construction, which is the point of writing them first.
