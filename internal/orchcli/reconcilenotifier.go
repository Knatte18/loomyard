// reconcilenotifier.go installs the process-wide route that mails the hub config reconcile's reinterpretation notice to the hub's prime orch, signed as the RECONCILE program.

package orchcli

import (
	"context"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// reconcileProgram is the program name a reinterpretation notice is signed with.
const reconcileProgram = "RECONCILE"

// InstallReconcileNotifier installs hubreconcile's notifier for this process, so a reconcile that rewrites or flags a reinterpreted config value reaches the prime orch as creel mail.
// The hub is the one the context's working directory resolves to, and outside a hub nothing is installed.
// The notifier posts only for that hub's Bolt checkout, and reports another hub's notice not posted.
func InstallReconcileNotifier(ctx context.Context) {
	location, inHub := hubLocation(ctx)
	if !inHub {
		return
	}
	hubreconcile.InstallNotifier(reconcileNotifierFor(location))
}

// reconcileNotifierFor returns the notifier that posts a notice for the hub at location to its prime orch, signed RECONCILE, and reports a notice for another Bolt checkout not posted.
func reconcileNotifierFor(location *lymcwd.Location) hubreconcile.Notifier {
	boardDir := filepath.Clean(fabricengine.BoltDir(location.HubPath))
	return func(noticeBoardDir, line string) (bool, error) {
		if filepath.Clean(noticeBoardDir) != boardDir {
			return false, nil
		}
		sender, err := drivableLocation(location)
		if err != nil {
			return false, err
		}
		return NotifyPrime(sender, reconcileProgram, line)
	}
}
