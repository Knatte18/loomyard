// export_test.go re-exports runCircling, judgeInputs and framingCircling for package treadleengine_test.
// That package needs them to drive the circling-check judge directly.
// A test that needs the hubforge fixture cannot live in this package, because that import would close a compile cycle.

package treadleengine

// RunCirclingForTest re-exports runCircling for package treadleengine_test files that spawn the
// per-round circling-check progress judge directly against a real Shuttle.
var RunCirclingForTest = runCircling

// JudgeInputsForTest is a type alias for judgeInputs.
// It lets package treadleengine_test files build the judge's inputs with the same field names.
type JudgeInputsForTest = judgeInputs

// FramingCirclingForTest re-exports framingCircling for package treadleengine_test files.
// They parse a circling judge's verdict file via the exported ParseJudgeVerdict.
// Its judgeFraming parameter type is unexported, so this value supplies it.
var FramingCirclingForTest = framingCircling
