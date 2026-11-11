// seamsignature_test.go pins every module's RunCLI and RunCLIIn seam function to its exact signature at compile time.
// It has no test function, so the package compiling is the assertion.
// A drifted signature fails the build instead of diverging silently from `PATTERN-cli-cobra`.

package main

import (
	"io"

	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/burlercli"
	"github.com/Knatte18/loomyard/internal/cleanupcli"
	"github.com/Knatte18/loomyard/internal/configcli"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/hibernatecli"
	"github.com/Knatte18/loomyard/internal/idecli"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/machinecli"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/quarrycli"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reportcli"
	"github.com/Knatte18/loomyard/internal/selfreportcli"
	"github.com/Knatte18/loomyard/internal/shuttlecli"
	"github.com/Knatte18/loomyard/internal/slackcli"
	"github.com/Knatte18/loomyard/internal/stencilcli"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/webstercli"
)

// The blank identifier pins every module's RunCLI to the seam signature in `PATTERN-cli-cobra`; nothing reads it.
var _ = []func(io.Writer, []string) int{
	boardcli.RunCLI,
	burlercli.RunCLI,
	cleanupcli.RunCLI,
	configcli.RunCLI,
	fabriccli.RunCLI,
	gatecli.RunCLI,
	hibernatecli.RunCLI,
	idecli.RunCLI,
	loomcli.RunCLI,
	machinecli.RunCLI,
	orchcli.RunCLI,
	tendercli.RunCLI,
	quarrycli.RunCLI,
	reedcli.RunCLI,
	reportcli.RunCLI,
	selfreportcli.RunCLI,
	shuttlecli.RunCLI,
	slackcli.RunCLI,
	stencilcli.RunCLI,
	webstercli.RunCLI,
}

// The blank identifier pins every module's RunCLIIn to the seam signature in `PATTERN-cli-cobra`; nothing reads it.
// A module belongs in this list when its RunCLIIn reads the cwd it is given.
// internal/selfreportcli is left out: it references lymcwd nowhere, so a RunCLIIn there would accept an unread cwd.
// loomcli reads cwd throughout, so a seeded cwd is meaningful to it.
var _ = []func(string, io.Writer, []string) int{
	boardcli.RunCLIIn,
	burlercli.RunCLIIn,
	cleanupcli.RunCLIIn,
	configcli.RunCLIIn,
	fabriccli.RunCLIIn,
	gatecli.RunCLIIn,
	hibernatecli.RunCLIIn,
	idecli.RunCLIIn,
	loomcli.RunCLIIn,
	machinecli.RunCLIIn,
	orchcli.RunCLIIn,
	tendercli.RunCLIIn,
	quarrycli.RunCLIIn,
	reedcli.RunCLIIn,
	reportcli.RunCLIIn,
	shuttlecli.RunCLIIn,
	slackcli.RunCLIIn,
	stencilcli.RunCLIIn,
	webstercli.RunCLIIn,
}
