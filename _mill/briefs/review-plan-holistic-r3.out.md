MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewer_self_id: Claude Opus (Anthropic); environment reports model id claude-opus-5
reviewed_file: plan/
date: 2026-08-05
```

## Findings

### [BLOCKING] Batch 1's green-tree gate cannot pass — guard rows lag owners

**Location:** batch 1 cards 1-2 vs batch 4 card 19
**Issue:** `TestEnforcement_GeometryLiterals` flags any *string const decl* of a geometry token outside the single allowlisted dir (`enforcement_test.go:224` token set, `:281-302` context (c), `:420` allowlist = `internal/hubgeometry`). Card 1 adds `weftname.Suffix = "-weft"` and card 2 adds `configengine.LyxDirName = "_lyx"`, but the ownership rows for `-weft`/`_lyx` are not registered until batch 4 card 19 — and batch 1's `verify` runs `go test ./...`, which runs that untagged guard.
**Fix:** Split card 19: register the `-weft` and `_lyx` rows in batch 1 (they need no rename), leaving only the `internal/hubgeometry` → `internal/lyxcwd` directory-literal switch in batch 4.

### [BLOCKING] Card 4 deletes `(*Layout).WorktreePath(slug)` with live in-package test callers unlisted

**Location:** batch 1, card 4
**Issue:** Seven surviving calls sit in files absent from the card's `Edits:`/`Deletes:` — `internal/hubgeometry/weft_test.go:131,299`, `internal/hubgeometry/pattern_test.go:106`, `internal/hubgeometry/hubgeometry_test.go:130,423,449,572`. The card enumerates only the four *production* in-module callers (`:459`, `:524`, `:537`, `:570`), so batch 1's `go test ./...` fails to compile package `hubgeometry`.
**Fix:** Add those three test files to card 4's `Edits:` and state the substitution (`filepath.Join(l.Hub, slug)` or `fabricengine.WorktreePath(l, slug)` per file).

### [BLOCKING] Batch 5 moves the `_pattern` declarer but not its ownership row

**Location:** batch 5 card 25 (+ card 29) vs batch 6 card 36
**Issue:** Card 25 deletes `PatternDirName` from `lyxcwd` and declares `const DirName = "_pattern"` in `internal/pattern`, while card 19's map still names `internal/lyxcwd` as `_pattern`'s owner until batch 6. Batch 5's `verify` includes `go test ./internal/lyxcwd/...`, which runs the guard — so batch 5 is red. Card 29's zero-diff gate only reasons about `_lyx` and misses this.
**Fix:** Move the `_pattern` → `internal/pattern` row into batch 5 (card 25 or 29), leaving only the `fabricengine` co-owner row for batch 6 card 35/36.

### [BLOCKING] Card 32's `HostLyxLinkHere` disposition and Edits list are wrong

**Location:** batch 6, card 32
**Issue:** The card says delete it "if card 32 finds no in-package caller", but six live callers exist in `fabricengine`'s own test files — `checkout_rollback_test.go:56,111`, `reconcile_stale_removal_test.go:130,269,379`, `reconcile_stale_registration_test.go:485` — and none of those three files is in card 32's `Edits:`. They are edited under card 31 for an unrelated reason, so the retarget is unspecified anywhere.
**Fix:** Relocate it as unexported `hostLyxLinkHere(l)` in `fabricengine/junction.go`, and add the three test files to card 32's `Edits:` with the retarget named.

### [NIT] Card 25 contradicts itself on `internal/pattern`'s import set

**Location:** batch 5, card 25
**Issue:** It states pattern imports "stdlib plus `internal/lyxcwd` and `internal/configengine`" while also asserting the Pattern Leaf Invariant needs no widening — but `pattern/leaf_enforcement_test.go:23-24` allowlists only the cwd module, and the declared constructors (`DirName`, `Dir`, `File`, `FileHere`) need no `configengine`.
**Fix:** Drop `internal/configengine` from that sentence; the card's own bodies never use it.

### [NIT] Card 31 both relocates and deletes `WeftRaddleDir`, and cites the wrong card

**Location:** batch 6, card 31
**Issue:** `weftRaddleDir(l)` appears in the re-declare list and two sentences later is "delete it outright rather than relocating dead surface". Separately, "`fabriccli/weft_verbs.go` … takes the card-24 accessor" points at scout's daemon paths; the accessor is added by card 30.
**Fix:** Remove `weftRaddleDir` from the re-declare list and correct the cross-reference to card 30.

### [NIT] ~20 files keep comment-only `hubgeometry` references no card touches

**Location:** batches 3-4, cards 8-12 and 13-17
**Issue:** Godoc/comment references survive in files no card lists, e.g. `internal/builderengine/doc.go`, `internal/builderengine/state.go`, `internal/builderengine/plan.go`, `internal/websterengine/state.go`, `internal/websterengine/doc.go`, `internal/treadleengine/engine.go`, `internal/reedengine/server.go`, `internal/scoutengine/toolchain.go`, `internal/logger/retention.go`, `internal/burlerengine/prompt.go`, `internal/gitrepo/doc.go`, `internal/lyxtest/doc.go`, `cmd/lyx/registration_test.go`. They do not break the build, but card 18's own argument (a stale package name stops meaning anything) applies.
**Fix:** Add a comment-only sweep card in batch 4 covering the non-importing files that name `hubgeometry`/`Layout`.

## Verdict

REQUEST_CHANGES
Four batch gates fail as sequenced; two edit lists omit live callers.
MILL_REVIEW_END
