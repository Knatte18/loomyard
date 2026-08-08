MILL_REVIEW_BEGIN
# Review: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] lookupContext's second parameter is undefined: raw flag or defaulted dir
**Section:** `fix-assert-no-callers-literal` / Technical context → The CLI side
**Issue:** The signature is `lookupContext(cwd, targetDir string)`, but every call site today passes the *defaulted* `dir` (`dir := targetDir; if dir == "" { dir = cwd }`, `cli.go:142-145`, `:570-573`) into `resolveWorktreeRoot(cwd, dir)` — a plan writer reading "targetDir" can just as well pass the raw flag variable, which is `""` by default and makes the out-of-hub branch resolve `filepath.Abs("")` (process cwd) instead of `filepath.Abs(cwd)`.
**Fix:** State that `lookupContext` receives the already-defaulted directory (and that the `dir == "" → cwd` defaulting stays in each `RunE`, since `Options.TargetDir`/`filterWithin` still need it), or that `lookupContext` performs the defaulting and returns `dir` as a third value.

### [NOTE] lookupContext's error return contract is unspecified
**Section:** `fix-assert-no-callers-literal`
**Issue:** `lookupContext` returns an `error`, but the discussion never says what populates it — today the only failure is `LoadRegistry`, which each `RunE` emits via `output.Err(out, loadErr.Error())` and then returns nil (`cli.go:162-165`, `:579-582`); a `Resolve` failure is deliberately *not* an error (it degrades to builtin registry + synthesized `Location`).
**Fix:** State that the returned error carries `LoadRegistry` failures only, that `Resolve` failure is never an error, and that callers keep the `output.Err` + `return nil` envelope mapping unchanged.

### [NOTE] Byte-identical claim has an unstated filesystem-root edge case
**Section:** `out-of-hub-synthesized-location`
**Issue:** `WorktreePath()` = `filepath.Join(HubPath, WorktreeName)` reproduces `abs` exactly for every ordinary path, but not obviously so when `abs` is a filesystem/volume root (`filepath.Base("/")` is `"/"`, `filepath.Base("C:\\")` is `"\\"`); the discussion asserts byte-identity unconditionally.
**Fix:** Either note the degenerate root case as out of practical scope for a `--target-dir`, or say the synthesized `Location` is only claimed byte-identical for non-root directories.

## Verdict

GAPS_FOUND
One undefined parameter semantic in the new lookupContext seam; everything else verified against source.
MILL_REVIEW_END
