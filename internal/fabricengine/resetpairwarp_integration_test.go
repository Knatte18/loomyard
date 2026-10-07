//go:build integration

// resetpairwarp_integration_test.go covers Fabric.ResetPairWarp, the gated reset of a task pair's warp checkout: a reset past later commits and own-path dirt that keeps untracked files, moves the remote task branch and records remote_branch_updated then worktree_reset, a refusal on dirt outside the own paths that precedes any fetch, the four ownership refusals, and the remote half's refusals, skips, stale lease and failed checkout rewrite.
//
// Every hub is built through hubforge.NewHub with an empty branch_prefix, so the pair's warp branch is the bare slug.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// pairFabric adds a task pair for slug to h and opens the fabric handle on the pair's warp worktree.
func pairFabric(t *testing.T, h *hubforge.Hub, slug string) (*fabricengine.Fabric, string) {
	t.Helper()

	hubforge.AddPair(t, h, slug)
	warp := h.PairCodeWorktree(slug)
	loc, err := lyxcwd.ResolveWorktree(warp)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", warp, err)
	}
	f, err := fabricengine.Open(loc)
	if err != nil {
		t.Fatalf("Open(%s): %v", warp, err)
	}
	return f, warp
}

// assertResetRefused fails the test unless err is a gate refusal on want that records nothing in rec.
func assertResetRefused(t *testing.T, err error, rec *fabricengine.Mutations, want fabricengine.Check) fabricengine.Refusal {
	t.Helper()

	refusal, ok := fabricengine.RefusalOf(err)
	if !ok {
		t.Fatalf("ResetPairWarp error = %v; want a gate refusal", err)
	}
	if refusal.Check != want {
		t.Errorf("refusal.Check = %s; want %s (reason %q)", refusal.Check, want, refusal.Reason)
	}
	if got := len(rec.Entries()); got != 0 {
		t.Errorf("refusal recorded %d entries; want 0", got)
	}
	return refusal
}

func TestResetPairWarp_DiscardsCommitsAndOwnPathDirtKeepsUntracked(t *testing.T) {
	t.Parallel()

	const slug = "rpw-reset"
	h := hubforge.NewHub(t, ".")
	f, warp := pairFabric(t, h, slug)

	gitkit.CommitFile(t, warp, "own.txt", "base", "own base")
	older := gitkit.RevParse(t, warp, "HEAD")
	gitkit.CommitFile(t, warp, "later.txt", "later", "later commit")
	gitkit.Git(t, warp, "push", "origin", "HEAD")

	if err := os.WriteFile(filepath.Join(warp, "own.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	untracked := filepath.Join(warp, "untracked.txt")
	if err := os.WriteFile(untracked, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := fabricengine.NewMutations("")
	if err := f.ResetPairWarp(rec, older, "main", []string{"own.txt"}, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("ResetPairWarp: %v", err)
	}

	if got := gitkit.RevParse(t, warp, "HEAD"); got != older {
		t.Errorf("HEAD after reset = %q; want %q", got, older)
	}
	if _, err := os.Stat(filepath.Join(warp, "later.txt")); !os.IsNotExist(err) {
		t.Errorf("later.txt still present after reset (err=%v)", err)
	}
	if got, err := os.ReadFile(filepath.Join(warp, "own.txt")); err != nil || string(got) != "base" {
		t.Errorf("own.txt = %q, %v; want %q", got, err, "base")
	}
	if _, err := os.Stat(untracked); err != nil {
		t.Errorf("untracked file removed by reset: %v", err)
	}

	wantKinds(t, rec, fabricengine.KindRemoteBranchUpdated, fabricengine.KindWorktreeReset)
	if entry := rec.Entries()[0]; entry.Target != "origin/"+slug || entry.Detail != older {
		t.Errorf("remote update entry = %+v; want target origin/%s and detail %s", entry, slug, older)
	}
	if got := remoteTip(t, warp, slug); got != older {
		t.Errorf("remote %s = %q after the reset; want %q", slug, got, older)
	}

	gitkit.CommitFile(t, warp, "after.txt", "after", "commit after the reset")
	gitkit.Git(t, warp, "push", "origin", "HEAD")
}

func TestResetPairWarp_DirtyPathOutsideOwnPathsRefuses(t *testing.T) {
	t.Parallel()

	const slug = "rpw-foreign"
	h := hubforge.NewHub(t, ".")
	f, warp := pairFabric(t, h, slug)

	gitkit.CommitFile(t, warp, "own.txt", "base", "own base")
	gitkit.CommitFile(t, warp, "foreign.txt", "base", "foreign base")
	older := gitkit.RevParse(t, warp, "HEAD~1")
	head := gitkit.RevParse(t, warp, "HEAD")
	gitkit.Git(t, warp, "push", "origin", "HEAD")
	originURL := gitkit.Git(t, warp, "remote", "get-url", "origin")

	foreign := filepath.Join(warp, "foreign.txt")
	if err := os.WriteFile(foreign, []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(warp, "own.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	// An unreachable origin makes any fetch fail, so a gate refusal proves the local refusal came first.
	gitkit.Git(t, warp, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
	rec := fabricengine.NewMutations("")
	err := f.ResetPairWarp(rec, older, "main", []string{"own.txt"}, fabricengine.SyncOptions{})
	gitkit.Git(t, warp, "remote", "set-url", "origin", originURL)
	if got := remoteTip(t, warp, slug); got != head {
		t.Errorf("remote %s = %q after a refused reset; want it untouched at %q", slug, got, head)
	}
	refusal := assertResetRefused(t, err, rec, fabricengine.CheckDirtiness)
	if !strings.Contains(refusal.Reason, "foreign.txt") {
		t.Errorf("refusal reason %q does not name foreign.txt", refusal.Reason)
	}
	if strings.Contains(refusal.Reason, "own.txt") {
		t.Errorf("refusal reason %q names the exempt own.txt", refusal.Reason)
	}

	if got := gitkit.RevParse(t, warp, "HEAD"); got != head {
		t.Errorf("HEAD moved to %q; want %q", got, head)
	}
	if got, err := os.ReadFile(foreign); err != nil || string(got) != "dirty" {
		t.Errorf("foreign.txt = %q, %v; want the dirt untouched", got, err)
	}
}

func TestResetPairWarp_OwnershipRefusals(t *testing.T) {
	t.Parallel()

	t.Run("PrimeCheckout", func(t *testing.T) {
		t.Parallel()
		h := hubforge.NewHub(t, ".")
		f, err := fabricengine.Open(h.Location)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		head := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD")
		rec := fabricengine.NewMutations("")
		assertResetRefused(t, f.ResetPairWarp(rec, head, "other", nil, fabricengine.SyncOptions{}), rec, fabricengine.CheckOwnership)
	})

	t.Run("PairOnParentBranch", func(t *testing.T) {
		t.Parallel()
		const slug = "rpw-parent"
		h := hubforge.NewHub(t, ".")
		f, warp := pairFabric(t, h, slug)
		head := gitkit.RevParse(t, warp, "HEAD")
		rec := fabricengine.NewMutations("")
		assertResetRefused(t, f.ResetPairWarp(rec, head, slug, nil, fabricengine.SyncOptions{}), rec, fabricengine.CheckOwnership)
	})

	t.Run("WeftOnAnotherBranch", func(t *testing.T) {
		t.Parallel()
		const slug = "rpw-otherweft"
		h := hubforge.NewHub(t, ".")
		f, warp := pairFabric(t, h, slug)
		gitkit.MustRun(t, h.PairRecordsSibling(slug), "git", "checkout", "-b", "some-other-branch")
		head := gitkit.RevParse(t, warp, "HEAD")
		rec := fabricengine.NewMutations("")
		assertResetRefused(t, f.ResetPairWarp(rec, head, "main", nil, fabricengine.SyncOptions{}), rec, fabricengine.CheckOwnership)
	})

	t.Run("DetachedHead", func(t *testing.T) {
		t.Parallel()
		const slug = "rpw-detached"
		h := hubforge.NewHub(t, ".")
		f, warp := pairFabric(t, h, slug)
		gitkit.MustRun(t, warp, "git", "checkout", "--detach")
		head := gitkit.RevParse(t, warp, "HEAD")
		rec := fabricengine.NewMutations("")
		assertResetRefused(t, f.ResetPairWarp(rec, head, "main", nil, fabricengine.SyncOptions{}), rec, fabricengine.CheckOwnership)
	})
}

// remoteTip returns the SHA of branch on the origin as seen from dir, or "" when the branch is absent.
func remoteTip(t *testing.T, dir, branch string) string {
	t.Helper()

	return lsRemoteTip(t, dir, "origin", branch)
}

// lsRemoteTip returns the SHA of branch on remote (a name or a URL) as seen from dir, or "" when the branch is absent.
func lsRemoteTip(t *testing.T, dir, remote, branch string) string {
	t.Helper()

	fields := strings.Fields(gitkit.Git(t, dir, "ls-remote", remote, "refs/heads/"+branch))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// originTip returns the SHA of branch on the pair's origin read through its URL, which still works after a case renamed or removed the origin remote.
func originTip(t *testing.T, p *remoteHalfPair, branch string) string {
	t.Helper()

	return lsRemoteTip(t, p.warp, p.originURL, branch)
}

// wantKinds fails the test unless rec holds exactly the given kinds, in order.
func wantKinds(t *testing.T, rec *fabricengine.Mutations, want ...fabricengine.Kind) {
	t.Helper()

	var got []fabricengine.Kind
	for _, entry := range rec.Entries() {
		got = append(got, entry.Kind)
	}
	if !slices.Equal(got, want) {
		t.Errorf("record kinds = %v; want %v", got, want)
	}
}

// remoteHalfPair is a task pair whose warp branch holds a base and a later commit, both pushed to the origin.
type remoteHalfPair struct {
	f         *fabricengine.Fabric
	warp      string
	slug      string
	older     string
	later     string
	originURL string
}

func newRemoteHalfPair(t *testing.T, slug string) *remoteHalfPair {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	f, warp := pairFabric(t, h, slug)
	older := gitkit.CommitFile(t, warp, "own.txt", "base", "own base")
	later := gitkit.CommitFile(t, warp, "later.txt", "later", "later commit")
	gitkit.Git(t, warp, "push", "origin", "HEAD")
	return &remoteHalfPair{f: f, warp: warp, slug: slug, older: older, later: later, originURL: gitkit.Git(t, warp, "remote", "get-url", "origin")}
}

// pushRemoteOnlyCommit leaves the origin one commit ahead of the checkout, which is rewound to where it was, and returns that commit.
func (p *remoteHalfPair) pushRemoteOnlyCommit(t *testing.T) string {
	t.Helper()

	extra := gitkit.CommitFile(t, p.warp, "remoteonly.txt", "x", "remote-only commit")
	gitkit.Git(t, p.warp, "push", "origin", "HEAD")
	gitkit.Git(t, p.warp, "reset", "--hard", p.later)
	return extra
}

// TestResetPairWarp_RemoteHalf covers every outcome of the remote half of ResetPairWarp other than the update itself:
// the refusals, each leaving the remote and the checkout unchanged, and the cases that leave the remote alone.
func TestResetPairWarp_RemoteHalf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// arrange shapes the pair and returns the options to reset with, the commit to reset to, and the remote tip the case expects afterwards.
		arrange func(t *testing.T, p *remoteHalfPair) (opts fabricengine.SyncOptions, sha, wantRemote string)
		// wantErr holds the parts of the refusal; empty means the reset succeeds and leaves the checkout at sha.
		wantErr []string
		// wantKinds is the record the reset leaves.
		wantKinds []fabricengine.Kind
	}{
		{
			name: "remote holding a commit HEAD lacks refuses and lists it",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				return fabricengine.SyncOptions{}, p.older, p.pushRemoteOnlyCommit(t)
			},
			wantErr: []string{"remote-only commit", "git merge --strategy ours origin/", "someone else's work"},
		},
		{
			name: "remote still holding commits an earlier reset dropped refuses",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				gitkit.Git(t, p.warp, "reset", "--hard", p.older)
				return fabricengine.SyncOptions{}, p.older, p.later
			},
			wantErr: []string{"later commit", "git merge --strategy ours origin/"},
		},
		{
			name: "a rejected push refuses toward a reachable remote",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				hooks := filepath.Join(p.originURL, "hooks")
				if err := os.MkdirAll(hooks, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(hooks, "pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				return fabricengine.SyncOptions{}, p.older, p.later
			},
			wantErr: []string{"failed", "once the remote is reachable"},
		},
		{
			name: "an unreachable remote refuses toward a reachable remote",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				gitkit.Git(t, p.warp, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
				return fabricengine.SyncOptions{}, p.older, p.later
			},
			wantErr: []string{"cannot read the remote task branch", "once the remote is reachable"},
		},
		{
			name: "SkipGit does not skip the remote update",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				return fabricengine.SyncOptions{SkipGit: true}, p.older, p.older
			},
			wantKinds: []fabricengine.Kind{fabricengine.KindRemoteBranchUpdated, fabricengine.KindWorktreeReset},
		},
		{
			name: "SkipPush leaves the remote alone",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				return fabricengine.SyncOptions{SkipPush: true}, p.older, p.later
			},
			wantKinds: []fabricengine.Kind{fabricengine.KindWorktreeReset},
		},
		{
			name: "a remote tip equal to the target is left alone",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				return fabricengine.SyncOptions{}, p.later, p.later
			},
			wantKinds: []fabricengine.Kind{fabricengine.KindWorktreeReset},
		},
		{
			name: "a remote tip behind the target is left alone",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				gitkit.Git(t, p.warp, "push", "--force", "origin", p.older+":refs/heads/"+p.slug)
				return fabricengine.SyncOptions{}, p.later, p.older
			},
			wantKinds: []fabricengine.Kind{fabricengine.KindWorktreeReset},
		},
		{
			name: "a branch absent on the remote is left alone",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				gitkit.Git(t, p.warp, "push", "origin", "--delete", p.slug)
				return fabricengine.SyncOptions{}, p.older, ""
			},
			wantKinds: []fabricengine.Kind{fabricengine.KindWorktreeReset},
		},
		{
			name: "a repository with no remote resets the checkout alone",
			arrange: func(t *testing.T, p *remoteHalfPair) (fabricengine.SyncOptions, string, string) {
				gitkit.Git(t, p.warp, "remote", "remove", "origin")
				return fabricengine.SyncOptions{}, p.older, p.later
			},
			wantKinds: []fabricengine.Kind{fabricengine.KindWorktreeReset},
		},
	}
	for i, tc := range tests {
		slug := fmt.Sprintf("rhf-%d", i)
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := newRemoteHalfPair(t, slug)
			opts, sha, wantRemote := tc.arrange(t, p)
			head := gitkit.RevParse(t, p.warp, "HEAD")

			rec := fabricengine.NewMutations("")
			err := p.f.ResetPairWarp(rec, sha, "main", nil, opts)

			if len(tc.wantErr) == 0 {
				if err != nil {
					t.Fatalf("ResetPairWarp: %v", err)
				}
				if got := gitkit.RevParse(t, p.warp, "HEAD"); got != sha {
					t.Errorf("HEAD = %s; want %s", got, sha)
				}
				wantKinds(t, rec, tc.wantKinds...)
			} else {
				if err == nil {
					t.Fatalf("ResetPairWarp succeeded; want a refusal containing %q", tc.wantErr)
				}
				for _, part := range tc.wantErr {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("error %q does not contain %q", err, part)
					}
				}
				if got := gitkit.RevParse(t, p.warp, "HEAD"); got != head {
					t.Errorf("a refused reset moved HEAD from %s to %s", head, got)
				}
				wantKinds(t, rec)
			}

			if got := originTip(t, p, slug); got != wantRemote {
				t.Errorf("remote %s = %q; want %q", slug, got, wantRemote)
			}
		})
	}
}

// TestResetPairWarp_OursMergeThenRerunConverges proves the way forward a remote-only commit's refusal names:
// after `git merge --strategy ours` takes in the remote tip, a re-run rewinds both the checkout and the remote task branch.
func TestResetPairWarp_OursMergeThenRerunConverges(t *testing.T) {
	t.Parallel()

	p := newRemoteHalfPair(t, "rhf-ours")
	p.pushRemoteOnlyCommit(t)

	rec := fabricengine.NewMutations("")
	if err := p.f.ResetPairWarp(rec, p.older, "main", nil, fabricengine.SyncOptions{}); err == nil {
		t.Fatal("ResetPairWarp succeeded over a remote-only commit; want a refusal")
	}

	gitkit.Git(t, p.warp, "merge", "--strategy", "ours", "--no-edit", "origin/"+p.slug)
	rec = fabricengine.NewMutations("")
	if err := p.f.ResetPairWarp(rec, p.older, "main", nil, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("ResetPairWarp after the ours merge: %v", err)
	}
	wantKinds(t, rec, fabricengine.KindRemoteBranchUpdated, fabricengine.KindWorktreeReset)
	if got := gitkit.RevParse(t, p.warp, "HEAD"); got != p.older {
		t.Errorf("HEAD = %s; want %s", got, p.older)
	}
	if got := originTip(t, p, p.slug); got != p.older {
		t.Errorf("remote %s = %q; want %q", p.slug, got, p.older)
	}
}

// TestUpdateRemoteBranch_StaleLeaseRefuses drives the executor with a lease stale against the origin, the state a remote tip that moves after the fetch produces.
func TestUpdateRemoteBranch_StaleLeaseRefuses(t *testing.T) {
	t.Parallel()

	p := newRemoteHalfPair(t, "rhf-lease")

	rec, err := fabricengine.UpdateRemoteBranchWithLeaseForTest(p.f, p.older, "main", p.older)
	if err == nil {
		t.Fatal("update succeeded under a stale lease; want a refusal")
	}
	for _, part := range []string{"moved while this reset ran", "git merge --strategy ours origin/" + p.slug, "someone else's work"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %q does not contain %q", err, part)
		}
	}
	wantKinds(t, &rec)
	if got := originTip(t, p, p.slug); got != p.later {
		t.Errorf("remote %s = %q; want it unchanged at %q", p.slug, got, p.later)
	}
}

// TestResetPairWarp_CheckoutFailureAfterRemoteUpdateConverges holds the checkout's index lock so the rewrite fails once the remote has moved.
func TestResetPairWarp_CheckoutFailureAfterRemoteUpdateConverges(t *testing.T) {
	t.Parallel()

	p := newRemoteHalfPair(t, "rhf-lock")
	lock := gitkit.Git(t, p.warp, "rev-parse", "--path-format=absolute", "--git-path", "index.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	rec := fabricengine.NewMutations("")
	err := p.f.ResetPairWarp(rec, p.older, "main", nil, fabricengine.SyncOptions{})
	if err == nil {
		t.Fatal("ResetPairWarp succeeded with the index locked; want the checkout rewrite to fail")
	}
	for _, part := range []string{"remote task branch was already updated", "re-run this reset"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %q does not contain %q", err, part)
		}
	}
	wantKinds(t, rec, fabricengine.KindRemoteBranchUpdated)
	if got := originTip(t, p, p.slug); got != p.older {
		t.Errorf("remote %s = %q; want %q", p.slug, got, p.older)
	}

	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	rec = fabricengine.NewMutations("")
	if err := p.f.ResetPairWarp(rec, p.older, "main", nil, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	wantKinds(t, rec, fabricengine.KindWorktreeReset)
	if got := gitkit.RevParse(t, p.warp, "HEAD"); got != p.older {
		t.Errorf("HEAD = %s; want %s", got, p.older)
	}
}
