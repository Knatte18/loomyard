// repository.go reads the repository name a hub works on, the identity creel's trusted_hubs lists a hub by.

package hubregistry

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
)

// The longest owner and repository names GitHub allows.
const (
	maxOwnerLength      = 39
	maxRepositoryLength = 100
)

// ErrNoRepository is wrapped by the refusal of a hub with no readable origin remote, and of one whose origin names no valid GitHub repository.
var ErrNoRepository = errors.New("creel: hub names no GitHub repository")

// noRepositoryError carries a refusal's own text and matches ErrNoRepository.
type noRepositoryError string

func (e noRepositoryError) Error() string { return string(e) }

func (e noRepositoryError) Is(target error) bool { return target == ErrNoRepository }

// RepositoryName answers the full name, owner/repo, of the GitHub repository the hub at hubPath works on.
// It reads the origin remote of the hub's prime worktree.
// A name reused on another machine or by a later hub therefore still names the repository, never the shortname.
// A hub with no readable origin remote, one whose origin is no GitHub repository,
// and one whose owner or repository has characters or a length GitHub does not allow, are refused with the reason.
// Each refusal matches ErrNoRepository.
func RepositoryName(hubPath string) (string, error) {
	url, err := fabricengine.HubOriginURL(hubPath)
	if err != nil {
		return "", noRepositoryError(fmt.Sprintf("the hub %s has no readable origin remote: %v", hubPath, err))
	}
	owner, repo, err := githubclient.ParseOwnerRepo(url)
	if err != nil {
		return "", noRepositoryError(fmt.Sprintf("the origin remote of the hub %s names no GitHub repository: %v", hubPath, err))
	}
	if !validName(owner, maxOwnerLength) || !validName(repo, maxRepositoryLength) {
		return "", noRepositoryError(fmt.Sprintf("the origin remote of the hub %s names no GitHub repository: %q is no owner/repo of letters, digits, hyphens, underscores and dots", hubPath, owner+"/"+repo))
	}
	return owner + "/" + repo, nil
}

// validName reports whether name is one to maxLength characters of ASCII letters, digits, hyphens, underscores and dots,
// and is neither "." nor "..", which GitHub reserves.
func validName(name string, maxLength int) bool {
	if name == "" || name == "." || name == ".." || len(name) > maxLength {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
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
