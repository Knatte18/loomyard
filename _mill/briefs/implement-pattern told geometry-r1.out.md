5 of 5 cards committed — all cards' `Commit:` messages match the log entries exactly, verify passes across all five package trees, and the working tree has no uncommitted tracked changes.

Summary of files touched:
- `/home/knatte/Code/loomyard/wts/pattern-told-geometry/internal/pattern/pattern.go` — `Directive`/`isActive` converted to a plain `anchorPath string`; `FileHere` deleted; `lyxcwd` import removed.
- `/home/knatte/Code/loomyard/wts/pattern-told-geometry/internal/pattern/pattern_test.go` — all fixtures converted to anchor-path strings; `TestDirective_EmptyAnchorPath` replaces `TestDirective_NilLayout`; `TestDirective_NestedAnchorSubdirectory` replaces the `RelPath`-named test.
- `/home/knatte/Code/loomyard/wts/pattern-told-geometry/internal/pattern/patternpath_test.go` — `TestLocation_PatternAccessors`/`TestFileHere_EqualsFileOfAnchorPath`/`newTestLocation` removed.
- `/home/knatte/Code/loomyard/wts/pattern-told-geometry/internal/pattern/leaf_enforcement_test.go` — `lyxcwd` dropped from the allowlist map, header comment, and failure message.
- `/home/knatte/Code/loomyard/wts/pattern-told-geometry/internal/burlerengine/engine.go`, `internal/websterengine/render.go`, `internal/loomengine/plan.go` — call sites pass `l.AnchorPath()`/`layout.AnchorPath()`/`e.layout.AnchorPath()`.
- `/home/knatte/Code/loomyard/wts/pattern-told-geometry/cmd/lyx/constructoranchoring_test.go` — `pattern.FileHere` rows rewritten to `pattern.File(l.AnchorPath())`; tautology comments amended to name the row plus the new anchoring test.
- `/home/knatte/Code/loomyard/wts/pattern-told-geometry/internal/loomengine/plan_test.go` — new `TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath` (active/inactive pair).
- `/home/knatte/Code/loomyard/wts/pattern-told-geometry/internal/burlerengine/engine_test.go` — new `TestEngine_Run_PatternDirectiveReachesInstruction1` (active/inactive pair).
- `/home/knatte/Code/loomyard/wts/pattern-told-geometry/CONSTRAINTS.md`, `internal/pattern/doc.go`, `internal/websterengine/template_test.go` — Pattern Leaf Invariant and package godoc narrowed to drop `lyxcwd`.

{"status":"success","commit_sha":"2533e59ffafff7913f79ce0a7b2aab2a962c22c4","session_id":"aeac239e-19c4-4654-a255-f4dc6b7fffd3","cards_done":[1,2,3,4,5]}
