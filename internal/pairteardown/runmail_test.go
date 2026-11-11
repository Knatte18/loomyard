// runmail_test.go pins how a teardown drops a removed run's unread creel mail through the installed dropper.

package pairteardown

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDropPairRunMail_InstalledDropper pins that the installed dropper is called with the teardown's prime and the slug.
// It also pins that nothing happens with none installed, and that a dropper error is logged and never propagates.
// The dropper is process-wide state, so this test is not parallel, and each case restores the previous one.
func TestDropPairRunMail_InstalledDropper(t *testing.T) {
	tiergate.Require(t, 0)

	prime := &lymcwd.Location{HubPath: "/hub", AnchorRel: "."}
	td := &Teardown{prime: prime}

	t.Run("InstalledDropperGetsThePrimeAndTheSlug", func(t *testing.T) {
		var gotPrimes []*lymcwd.Location
		var gotSlugs []string
		restore := InstallRunMailDropper(func(p *lymcwd.Location, slug string) (int, error) {
			gotPrimes = append(gotPrimes, p)
			gotSlugs = append(gotSlugs, slug)
			return 2, nil
		})
		t.Cleanup(restore)

		td.dropPairRunMail("slug")

		if len(gotPrimes) != 1 || gotPrimes[0] != prime || len(gotSlugs) != 1 || gotSlugs[0] != "slug" {
			t.Errorf("dropper saw primes %v and slugs %v; want one call with the teardown's prime and %q", gotPrimes, gotSlugs, "slug")
		}
	})

	t.Run("NoDropperInstalledDoesNothing", func(t *testing.T) {
		previous := installedRunMailDropper.Swap(nil)
		t.Cleanup(func() { installedRunMailDropper.Store(previous) })

		td.dropPairRunMail("slug")
	})

	t.Run("DropperErrorIsLoggedAndNotPropagated", func(t *testing.T) {
		var buf strings.Builder
		logger.SetOutput(&buf)
		t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })
		restore := InstallRunMailDropper(func(*lymcwd.Location, string) (int, error) {
			return 0, errors.New("creel unreachable")
		})
		t.Cleanup(restore)

		td.dropPairRunMail("slug")

		for _, want := range []string{"slug", "creel unreachable"} {
			if !strings.Contains(buf.String(), want) {
				t.Errorf("log = %q, want it to name %q", buf.String(), want)
			}
		}
	})
}
