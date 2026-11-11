// runmail.go installs the process-wide route that drops a removed run's unread creel mail at its pair's teardown.

package orchcli

import (
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
)

// teardownProgram is the program name the drop of a removed run's mail is signed with.
const teardownProgram = "TEARDOWN"

// InstallRunMailDropper installs pairteardown's run mail dropper for this process,
// so a pair's teardown drops the unread creel mail about its run, signed as the TEARDOWN program.
// A hub with no creel journal holds no mail, so the drop starts no daemon there.
// Creel refuses a program claim from the pane of a strand that is no orch's, so a teardown run there drops nothing.
func InstallRunMailDropper() {
	pairteardown.InstallRunMailDropper(dropRunMail)
}

// dropRunMail drops the unread creel mail of prime's hub tagged with slug as its run, signed as the TEARDOWN program.
// It answers how many messages it dropped.
func dropRunMail(prime *lymcwd.Location, slug string) (int, error) {
	client, err := creelcli.NewProgramClient(prime, teardownProgram)
	if err != nil {
		return 0, err
	}
	dropped, err := client.DropRun(slug)
	return len(dropped), err
}
