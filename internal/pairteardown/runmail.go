// runmail.go drops a removed run's unread creel mail through the process's installed run mail dropper.

package pairteardown

import (
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// RunMailDropper drops the unread creel mail of prime's hub tagged with the run slug.
// It answers how many messages it dropped.
type RunMailDropper func(prime *lymcwd.Location, slug string) (dropped int, err error)

var installedRunMailDropper atomic.Pointer[RunMailDropper]

// InstallRunMailDropper stores d as the process's run mail dropper.
// It returns a function that reinstates the previous one.
func InstallRunMailDropper(d RunMailDropper) (restore func()) {
	previous := installedRunMailDropper.Swap(&d)
	return func() { installedRunMailDropper.Store(previous) }
}

// dropPairRunMail drops the hub's unread creel mail tagged with slug's run through the installed dropper.
// With none installed it drops nothing, and a failure logs a Warn.
func (t *Teardown) dropPairRunMail(slug string) {
	dropper := installedRunMailDropper.Load()
	if dropper == nil {
		return
	}
	dropped, err := (*dropper)(t.prime, slug)
	if err != nil {
		logger.Warn("pairteardown: the removed run's unread creel mail was not dropped, so an orch may still read it", "slug", slug, "error", err)
		return
	}
	if dropped > 0 {
		logger.Info("pairteardown: unread creel mail about the removed run dropped", "slug", slug, "messages", dropped)
	}
}
