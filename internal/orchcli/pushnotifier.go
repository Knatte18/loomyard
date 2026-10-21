// pushnotifier.go installs the process-wide route that mails a failed-push notice to the hub's prime orch, signed as the FABRIC program.

package orchcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
)

// fabricProgram is the program name a failed-push notice is signed with.
const fabricProgram = "FABRIC"

// InstallPushNotifier installs fabricengine's push notifier for this process, so a push origin refuses reaches the prime orch as creel mail.
// The hub is the one the context's working directory resolves to, as the network gate resolves it:
// a detached push child starts in the hub's board worktree, which resolves as a worktree root.
// Outside a hub nothing is installed.
// The notifier posts only for that hub, and reports a failure of another hub not posted.
func InstallPushNotifier(ctx context.Context) {
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		location, err = lyxcwd.ResolveWorktree(cwd)
	}
	if err != nil || !preflight.BoardLyxPresent(location) {
		return
	}
	hubPath := filepath.Clean(location.HubPath)
	fabricengine.InstallPushNotifier(func(failedHubPath, line string) (bool, error) {
		if filepath.Clean(failedHubPath) != hubPath {
			return false, nil
		}
		sender, err := drivableLocation(location)
		if err != nil {
			return false, err
		}
		return NotifyPrime(sender, fabricProgram, line)
	})
}

// drivableLocation returns location when a lyx verb can run from it, and otherwise the first worktree of its hub one can run from.
// The board worktree and a records sibling cannot resolve the hub's prime, since that scan reads the code repository.
func drivableLocation(location *lyxcwd.Location) (*lyxcwd.Location, error) {
	if fabricengine.RequireDrivableWorktree(location) == nil {
		return location, nil
	}
	entries, err := os.ReadDir(location.HubPath)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		candidate, err := lyxcwd.ResolveWorktree(filepath.Join(location.HubPath, entry.Name()))
		if err == nil && fabricengine.RequireDrivableWorktree(candidate) == nil {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("orch: the hub at %s has no worktree to resolve its prime from; way forward: run lyx from a worktree of the hub", location.HubPath)
}
