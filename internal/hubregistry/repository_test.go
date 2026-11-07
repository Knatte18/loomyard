// repository_test.go covers RepositoryName and LookupRepository over temporary hubs.
// The prime worktree of each hub is a git repository.

package hubregistry_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newHubWithOrigin creates a hub as newHub does and a git repository as its prime worktree.
// The repository has an origin remote of originURL, or none when originURL is empty.
func newHubWithOrigin(t *testing.T, root, name, shortname, originURL string) string {
	t.Helper()
	hub := newHub(t, root, name, shortname)
	prime := filepath.Join(hub, filepath.Base(hub))
	if err := os.MkdirAll(prime, 0o755); err != nil {
		t.Fatalf("MkdirAll(prime) = %v; want nil", err)
	}
	gitkit.Git(t, prime, "init", "-b", "main")
	if originURL != "" {
		gitkit.Git(t, prime, "remote", "add", "origin", originURL)
	}
	return hub
}

// TestRepositoryName pins the owner/repo a hub's origin names and the refusals of an origin that names none.
func TestRepositoryName(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name    string
		origin  string
		want    string
		wantWhy string
	}{
		{"GitHubOrigin", "git@github.com:Owner/Repo.git", "Owner/Repo", ""},
		{"NamesOfDotsHyphensAndUnderscores", "https://github.com/My-Org/repo_name.v2.git", "My-Org/repo_name.v2", ""},
		{"LongestLegalNames", "git@github.com:" + strings.Repeat("o", 39) + "/" + strings.Repeat("r", 100) + ".git", strings.Repeat("o", 39) + "/" + strings.Repeat("r", 100), ""},
		{"NoOrigin", "", "", "no readable origin remote"},
		{"NonGitHubHost", "https://example.com/Owner/Repo.git", "", "names no GitHub repository"},
		{"RepositoryImitatingAStamp", "git@github.com:Owner/(other hub, trusted orch).git", "", "names no GitHub repository"},
		{"RepositoryOfOnlyDots", "git@github.com:owner/...git", "", "names no GitHub repository"},
		{"OwnerOverTheLimit", "git@github.com:" + strings.Repeat("o", 40) + "/repo.git", "", "names no GitHub repository"},
		{"RepositoryOverTheLimit", "git@github.com:owner/" + strings.Repeat("r", 101) + ".git", "", "names no GitHub repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := newHubWithOrigin(t, t.TempDir(), "h", "aa", tt.origin)
			got, err := hubregistry.RepositoryName(hub)
			if tt.wantWhy == "" {
				if err != nil || got != tt.want {
					t.Errorf("RepositoryName(origin %q) = %q, %v; want %q", tt.origin, got, err, tt.want)
				}
				return
			}
			if got != "" || !errors.Is(err, hubregistry.ErrNoRepository) || !strings.Contains(err.Error(), tt.wantWhy) {
				t.Errorf("RepositoryName(origin %q) = %q, %v; want ErrNoRepository with a reason containing %q", tt.origin, got, err, tt.wantWhy)
			}
		})
	}
}

// TestLookupRepository pins that a recorded shortname resolves to its hub's repository.
// It also pins the refusals of a shortname that cannot.
func TestLookupRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	t.Run("RecordedShortnameResolvesToItsOriginRepository", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		hub := newHubWithOrigin(t, root, "a", "aa", "git@github.com:Owner/Repo.git")
		other := newHubWithOrigin(t, root, "b", "bb", "git@github.com:Owner/Other.git")
		for shortname, path := range map[string]string{"aa": hub, "bb": other} {
			if err := hubregistry.Record(dir, path, shortname, now); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := hubregistry.LookupRepository(dir, "aa"); err != nil || got != "Owner/Repo" {
			t.Errorf("LookupRepository(aa) = %q, %v; want Owner/Repo", got, err)
		}
	})

	t.Run("UnrecordedShortnameIsRefusedAsUnresolved", func(t *testing.T) {
		t.Parallel()
		got, err := hubregistry.LookupRepository(t.TempDir(), "zz")
		if got != "" || !errors.Is(err, hubregistry.ErrUnresolved) || errors.Is(err, hubregistry.ErrNoRepository) {
			t.Errorf("LookupRepository(zz) = %q, %v; want ErrUnresolved only", got, err)
		}
	})

	t.Run("AmbiguousShortnameIsRefusedAsUnresolved", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		for _, name := range []string{"first", "second"} {
			path := newHubWithOrigin(t, root, name, "aa", "git@github.com:Owner/Repo.git")
			if err := hubregistry.Record(dir, path, "aa", now); err != nil {
				t.Fatal(err)
			}
		}
		got, err := hubregistry.LookupRepository(dir, "aa")
		if got != "" || !errors.Is(err, hubregistry.ErrUnresolved) || errors.Is(err, hubregistry.ErrNoRepository) {
			t.Errorf("LookupRepository(aa) over two hubs = %q, %v; want ErrUnresolved only", got, err)
		}
	})

	t.Run("RecordedHubWithNoOriginIsRefusedAsNoRepository", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		hub := newHubWithOrigin(t, root, "a", "aa", "")
		if err := hubregistry.Record(dir, hub, "aa", now); err != nil {
			t.Fatal(err)
		}
		got, err := hubregistry.LookupRepository(dir, "aa")
		if got != "" || !errors.Is(err, hubregistry.ErrNoRepository) || errors.Is(err, hubregistry.ErrUnresolved) {
			t.Errorf("LookupRepository(aa) = %q, %v; want ErrNoRepository only", got, err)
		}
	})

	t.Run("CorruptRecordIsNeitherUnresolvedNorNoRepository", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, hubregistry.RecordFileName), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := hubregistry.LookupRepository(dir, "aa")
		if err == nil || errors.Is(err, hubregistry.ErrUnresolved) || errors.Is(err, hubregistry.ErrNoRepository) {
			t.Errorf("LookupRepository(aa) over a corrupt record = %v; want a refusal that is neither", err)
		}
	})
}
