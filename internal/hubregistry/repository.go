// repository.go reads the repository name a hub works on, the identity creel's trusted_hubs lists a hub by.

package hubregistry

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
)

// RepositoryName answers the full name, owner/repo, of the GitHub repository the hub at hubPath works on.
// It reads the origin remote of the hub's prime worktree.
// A name reused on another machine or by a later hub therefore still names the repository, never the shortname.
// A hub with no readable origin remote, and one whose origin is no GitHub repository, are refused with the reason.
func RepositoryName(hubPath string) (string, error) {
	url, err := fabricengine.HubOriginURL(hubPath)
	if err != nil {
		return "", fmt.Errorf("the hub %s has no readable origin remote: %w", hubPath, err)
	}
	owner, repo, err := githubclient.ParseOwnerRepo(url)
	if err != nil {
		return "", fmt.Errorf("the origin remote of the hub %s names no GitHub repository: %w", hubPath, err)
	}
	return owner + "/" + repo, nil
}

// LookupRepository answers the repository name of the one hub in dir's record that recorded shortname.
// It refuses as Lookup does when the shortname names no hub or several, and as RepositoryName does after that.
func LookupRepository(dir, shortname string) (string, error) {
	hubPath, err := Lookup(dir, shortname)
	if err != nil {
		return "", err
	}
	return RepositoryName(hubPath)
}
