// remoteteardown_namespace_integration_test.go covers the manual archive-tag push a failed archive names.
// It runs against stores whose origin layout is read from real git config.

package fabricengine

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

func TestFailedRemoteSteps_DiscardTagPushNamesTheHubsArchiveNamespace(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "pair"
	taskTip := strings.Repeat("b", 40)
	tag := ArchiveTagName(slug, taskTip)
	out := remoteOutcome{err: errors.New("tag push refused")}

	for _, tt := range []struct {
		name string
		// namespaced gives the code store's records sibling the one-repository fetch refspec on the same origin.
		namespaced bool
		wantPush   string
	}{
		{"OneRepositoryHubSendsTheCodeStoresTagToItsOwnPrefix", true, "refs/tags/" + tag + ":refs/lym/code-tags/" + tag},
		{"TwoRepositoryHubSendsTheTagToItsOwnName", false, "refs/tags/" + tag + ":refs/tags/" + tag},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bare := t.TempDir()
			gitkit.Git(t, bare, "init", "--bare", "-b", "main")
			hub := t.TempDir()
			code := filepath.Join(hub, "code")
			newLayoutStore(t, code, bare, false)
			newLayoutStore(t, weftname.SiblingPath(hub, "code"), bare, tt.namespaced)
			job := remoteTeardownJob{Kind: remoteTeardownDiscard, Slug: slug, Worktree: code, TaskTip: taskTip}

			failures := failedRemoteSteps(job, out, "")

			if len(failures) != 1 {
				t.Fatalf("failedRemoteSteps() = %d failures; want 1", len(failures))
			}
			want := "`git -C " + code + " push " + originRemoteName + " " + tt.wantPush + "`"
			if !strings.Contains(failures[0].wayForward, want) {
				t.Errorf("way forward = %q; want it to hold %q", failures[0].wayForward, want)
			}
		})
	}
}

func TestFailedRemoteSteps_RecordsTagPushNamesTheStoresDestination(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "pair"
	weftTip := strings.Repeat("a", 40)
	tag := ArchiveTagName(slug, weftTip)
	job := remoteTeardownJob{Kind: remoteTeardownRemove, Slug: slug, WeftTip: weftTip}
	out := remoteOutcome{err: errors.New("tag push refused")}

	for _, tt := range []struct {
		name       string
		namespaced bool
		wantPush   string
	}{
		{"OneRepositoryStoreSendsTheTagToItsNamespace", true, "refs/tags/" + tag + ":refs/lym/tags/" + tag},
		{"TwoRepositoryStoreSendsTheTagToItsOwnName", false, "refs/tags/" + tag + ":refs/tags/" + tag},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bare := t.TempDir()
			gitkit.Git(t, bare, "init", "--bare", "-b", "main")
			weftRoot := filepath.Join(t.TempDir(), "records")
			newLayoutStore(t, weftRoot, bare, tt.namespaced)

			failures := failedRemoteSteps(job, out, weftRoot)

			if len(failures) != 1 {
				t.Fatalf("failedRemoteSteps() = %d failures; want 1", len(failures))
			}
			want := "`git -C " + weftRoot + " push " + originRemoteName + " " + tt.wantPush + "`"
			if !strings.Contains(failures[0].wayForward, want) {
				t.Errorf("way forward = %q; want it to hold %q", failures[0].wayForward, want)
			}
		})
	}
}
