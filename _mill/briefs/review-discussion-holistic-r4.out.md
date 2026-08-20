MILL_REVIEW_BEGIN
# Review: Extract scout into its own standalone repo

```yaml
verdict: REQUEST_CHANGES
reviewer_model: fablehigh
reviewer_self_id: Claude (Fable 5, claude-fable-5)
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:decision] LICENSE choice never made
**Section:** Repo state **Issue:** "No README, `.gitignore`, or LICENSE was auto-generated — all three are still to be written" commits the task to a LICENSE, but no decision names which one; Loomyard's root `LICENSE` is Apache-2.0, so licensing ~8 900 copied lines is the owner's call a plan writer cannot make. **Fix:** add a decision naming the quarry license (and whether Loomyard's Apache-2.0 notice/attribution carries into the initial import).

### [NIT:scope] Repo scaffolding absent from Scope In
**Section:** Scope / Repo state **Issue:** the README is mandated by three decisions (toolchain cache re-key note, supported-platforms statement, windows native-strategy caveat) and `go.mod`/`.gitignore`/LICENSE are named as "still to be written", yet none appear in the Scope In list, so a plan writer batching from Scope alone omits them. **Fix:** add a scaffolding bullet (go.mod, README with the three mandated statements, .gitignore, LICENSE) to Scope In.

### [NIT:consistency] Stale Scope bullet on lyxdirs inlining
**Section:** Scope In, bullet 3 **Issue:** "inline the one `lyxdirs` constant that survives" is superseded by `state-path-ownership-moves-to-the-caller`, under which `DotLyxDirName` is deleted from the engine and nothing is inlined — verified: `daemonstate.go:40,48` are its only production uses and both segments are deleted. **Fix:** reword the bullet to say the constant is deleted with the derived segments, not inlined.

### [NIT:consistency] "59 fmt.Errorf literals" mischaracterizes the mechanism
**Section:** error-prefixes-stay-verbatim-through-the-port **Issue:** verified 59 `"scoutengine: "` string literals across exactly the nine named files with the exact per-file counts, but `errors.go`'s 12 are `errors.New`/`fmt.Sprintf` (sentinel values and `Error()` methods), not `fmt.Errorf` — a later rename batch scoped by a `fmt.Errorf` grep would miss them. **Fix:** say "59 `"scoutengine: "` string literals" and note the sentinel/`Error()` forms in `errors.go`.

## Verdict

REQUEST_CHANGES
Source-accurate throughout; one open decision (license) plus three small scope/consistency slips.
MILL_REVIEW_END
