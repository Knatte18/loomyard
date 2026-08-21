// seamsignature_test.go pins the eleven existing RunCLI(io.Writer, []string) int seam functions,
// and the ten RunCLIIn(string, io.Writer, []string) int seam functions built alongside them, to
// their exact signatures at compile time.
// This test has no test function and no runtime body: the assertion is that the package compiles,
// so a drifted RunCLI/RunCLIIn signature in any of these modules becomes a build failure instead of
// a silent divergence from CONSTRAINTS.md's CLI/Cobra Invariant.
// The CLI/Cobra Invariant's seam clause was previously unenforced: cmd/lyx/drift_test.go asserts only
// that every command carries a non-empty Short, and no test under cmd/lyx referenced RunCLI at all.

package main

import (
	"io"

	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/burlercli"
	"github.com/Knatte18/loomyard/internal/configcli"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/idecli"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/selfreportcli"
	"github.com/Knatte18/loomyard/internal/shuttlecli"
	"github.com/Knatte18/loomyard/internal/stencilcli"
	"github.com/Knatte18/loomyard/internal/webstercli"
)

// The blank identifier below pins every module's RunCLI to the eleven-module RunCLI seam shape
// declared by the CLI/Cobra Invariant. Nothing reads it; the compile itself is the assertion.
var _ = []func(io.Writer, []string) int{
	boardcli.RunCLI,
	burlercli.RunCLI,
	configcli.RunCLI,
	fabriccli.RunCLI,
	idecli.RunCLI,
	loomcli.RunCLI,
	reedcli.RunCLI,
	selfreportcli.RunCLI,
	shuttlecli.RunCLI,
	stencilcli.RunCLI,
	webstercli.RunCLI,
}

// The blank identifier below pins every module's RunCLIIn to the ten-module RunCLIIn seam shape
// declared by the CLI/Cobra Invariant. internal/selfreportcli is the one seam module deliberately
// absent: it references lyxcwd nowhere, so a RunCLIIn there would accept a cwd argument nothing
// reads. internal/loomcli is on this slice rather than joining that exception: loom resolves cwd
// throughout, so a seeded cwd is meaningful to it, which is the exact criterion the existing
// exception rests on. Nothing reads this slice; the compile itself is the assertion.
var _ = []func(string, io.Writer, []string) int{
	boardcli.RunCLIIn,
	burlercli.RunCLIIn,
	configcli.RunCLIIn,
	fabriccli.RunCLIIn,
	idecli.RunCLIIn,
	loomcli.RunCLIIn,
	reedcli.RunCLIIn,
	shuttlecli.RunCLIIn,
	stencilcli.RunCLIIn,
	webstercli.RunCLIIn,
}
