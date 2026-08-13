// seamsignature_test.go pins the eleven existing RunCLI(io.Writer, []string) int seam functions to
// their exact signature at compile time.
// This test has no test function and no runtime body: the assertion is that the package compiles,
// so a drifted RunCLI signature in any of these eleven modules becomes a build failure instead of a
// silent divergence from CONSTRAINTS.md's CLI/Cobra Invariant.
// The CLI/Cobra Invariant's seam clause was previously unenforced: cmd/lyx/drift_test.go asserts only
// that every command carries a non-empty Short, and no test under cmd/lyx referenced RunCLI at all.
// The RunCLIIn half of this assertion — pinning func(string, io.Writer, []string) int across the
// modules that gain it — lands in batch 2, in the same batch that creates those RunCLIIn functions.

package main

import (
	"io"

	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/burlercli"
	"github.com/Knatte18/loomyard/internal/configcli"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/idecli"
	"github.com/Knatte18/loomyard/internal/perchcli"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/scoutcli"
	"github.com/Knatte18/loomyard/internal/selfreportcli"
	"github.com/Knatte18/loomyard/internal/shuttlecli"
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
	perchcli.RunCLI,
	reedcli.RunCLI,
	scoutcli.RunCLI,
	selfreportcli.RunCLI,
	shuttlecli.RunCLI,
	webstercli.RunCLI,
}
