// export_test.go re-exports runCircling and judgeInputs for the package treadleengine_test file
// that needs to drive the circling-check judge seam directly: internal/treadleengine sits inside
// internal/fabriccli's dependency set, so its fixture-using test cannot live in-package without
// closing a compile cycle through internal/hubforge — the standard Go export_test.go idiom, per the
// export-test-shim decision.

package treadleengine

// RunCirclingForTest re-exports runCircling for package treadleengine_test files that spawn the
// per-round circling-check progress judge directly against a real Shuttle.
var RunCirclingForTest = runCircling

// JudgeInputsForTest re-exports judgeInputs as a type alias (not a defined type) for package
// treadleengine_test files that need to construct it with runCircling's own field names.
type JudgeInputsForTest = judgeInputs

// FramingCirclingForTest re-exports framingCircling for package treadleengine_test files that parse
// a circling judge's verdict file via the exported ParseJudgeVerdict, whose judgeFraming parameter
// type stays unexported.
// Added beyond card 37's original two identifiers because smoke_judge_test.go's move to package
// treadleengine_test (card 38) also needs this seam to compile.
var FramingCirclingForTest = framingCircling
