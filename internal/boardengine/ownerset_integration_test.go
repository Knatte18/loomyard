//go:build tier1

// ownerset_integration_test.go covers SetOwners against a real board repository, a bare origin and a second clone of it:
// the outcomes of an owner write, the ways it refuses, a competing write that wins at origin, a deferred push and the BOARD_SKIP_GIT bypass.

package boardengine

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lock"
)

// unreachableOrigin is an origin URL nothing answers.
const unreachableOrigin = "http://127.0.0.1:1/board.git"

// ownerFixture is a board seeded with a few entries and pushed to a bare origin, beside a second clone of that origin.
// The board is the hub's _board worktree of a weft repository, the shape a rejected push's replay requires.
// The seed holds alpha and beta, unowned; away, owned by box; and held, run-held and owned by box.
type ownerFixture struct {
	bare     string
	boardDir string
	otherDir string
	seedHead string
}

// newOwnerFixture builds the fixture, with primeOwner as the prime owner the seed records, empty for none.
func newOwnerFixture(t *testing.T, primeOwner string) *ownerFixture {
	t.Helper()
	hub := t.TempDir()
	side := t.TempDir()
	bare := newBareRemote(t, side)
	// The hub's records repository sits beside _board under a name that pairs with a warp branch; fabric alone composes that name.
	weftDir := newBoardRepo(t, hub, fabricengine.RecordsBranchName("hub"), bare)

	store := NewStore(weftDir)
	for _, fields := range []map[string]any{
		{"slug": "alpha", "title": "Alpha", "kind": "task", "labels": []string{"bug"}},
		{"slug": "beta", "title": "Beta", "kind": "task", "labels": []string{"bug"}},
		{"slug": "away", "title": "Away", "kind": "task", "labels": []string{"bug"}},
		{"slug": "held", "title": "Held", "kind": "task", "labels": []string{"bug"}, "status": RunStatus("running", "Webster")},
	} {
		if _, err := store.UpsertTask(fields); err != nil {
			t.Fatalf("seed %v: %v", fields["slug"], err)
		}
	}
	for i := range store.tasks {
		if store.tasks[i].Slug == "away" || store.tasks[i].Slug == "held" {
			store.tasks[i].OwnerMachine = "box"
		}
	}
	store.primeOwner = primeOwner
	if err := store.Save(); err != nil {
		t.Fatalf("save seed board: %v", err)
	}
	writeBoardFile(t, weftDir, ".gitignore", "*.lock\n*.swaplock\n"+renderManifestFile+"\n")
	// The board config is tracked, since a replay of a rejected push validates the entries against it.
	configFile := configengine.ConfigFile(weftDir, "board")
	if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
		t.Fatalf("mkdir board config dir: %v", err)
	}
	if err := os.WriteFile(configFile, []byte(ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("write board config: %v", err)
	}
	if err := os.Mkdir(filepath.Join(weftDir, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	writeBoardFile(t, weftDir, "notes/x.md", "one\n")
	gitkit.Git(t, weftDir, "add", "-A")
	gitkit.Git(t, weftDir, "commit", "-m", "seed board")
	gitkit.Git(t, weftDir, "push", "-u", "origin", "main")

	// The weft repository's own checkout moves off main, which the hub's _board worktree then holds.
	boardDir := filepath.Join(hub, fabricengine.BoardDirName)
	gitkit.Git(t, weftDir, "checkout", "-b", "home")
	gitkit.Git(t, weftDir, "worktree", "add", boardDir, "main")
	gitkit.MustRun(t, side, "git", "clone", bare, "other")

	return &ownerFixture{bare: bare, boardDir: boardDir, otherDir: filepath.Join(side, "other"), seedHead: gitkit.RevParse(t, boardDir, "HEAD")}
}

// originOwners returns the owners the bare origin's board.json records.
func (f *ownerFixture) originOwners(t *testing.T) Owners {
	t.Helper()
	owners, err := ParseOwners([]byte(gitkit.Git(t, f.bare, "show", "main:board.json")))
	if err != nil {
		t.Fatalf("parse origin board.json: %v", err)
	}
	return owners
}

// pushFromOther changes the second clone's board through mutate and pushes the result to origin.
func (f *ownerFixture) pushFromOther(t *testing.T, mutate func(store *Store)) {
	t.Helper()
	store := NewStore(f.otherDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load second clone: %v", err)
	}
	mutate(store)
	if err := store.Save(); err != nil {
		t.Fatalf("save second clone: %v", err)
	}
	gitkit.Git(t, f.otherDir, "add", "board.json")
	gitkit.Git(t, f.otherDir, "commit", "-m", "other machine writes")
	gitkit.Git(t, f.otherDir, "push", "origin", "main")
}

// setOwnerOnOther returns a mutation of the second clone's store that records owner as slug's owner.
func setOwnerOnOther(slug, owner string) func(store *Store) {
	return func(store *Store) {
		for i := range store.tasks {
			if store.tasks[i].Slug == slug {
				store.tasks[i].OwnerMachine = owner
			}
		}
	}
}

// failOnSleep is a Sleep that fails the test, for a row that must not wait.
func failOnSleep(t *testing.T) func(time.Duration) {
	return func(d time.Duration) { t.Errorf("SetOwners slept %v; want no wait", d) }
}

func TestSetOwners(t *testing.T) {
	// Not parallel: the SkipGit row sets the environment, which a parallel ancestor forbids; the rows that can run in parallel say so themselves.

	// A test binary never runs the lyx root pre-run that installs the board replay a rejected push is retried through.
	fabricengine.InstallBoardReplay(FabricReplay{})

	outcomes := []struct {
		name         string
		primeOwner   string
		req          OwnerRequest
		wantRecorded []string
		wantLost     map[string]string
		// wantOwners maps a slug to the owner origin records for it afterwards.
		wantOwners map[string]string
		// wantPrime is the prime owner origin records afterwards.
		wantPrime string
		wantClaim string
		// wantSubject is the subject of origin's newest commit; empty requires origin's head to stay at the seed.
		wantSubject string
	}{
		{
			name: "RecordedAndPushed", primeOwner: "neve",
			req:          OwnerRequest{Machine: "neve", Slugs: []string{"alpha"}},
			wantRecorded: []string{"alpha"}, wantOwners: map[string]string{"alpha": "neve"}, wantPrime: "neve",
			wantSubject: "board: owner neve for alpha",
		},
		{
			name: "TakeoverChangesTheOwnerOfARunHeldEntry", primeOwner: "neve",
			req:          OwnerRequest{Machine: "neve", Slugs: []string{"held"}, Takeover: true},
			wantRecorded: []string{"held"}, wantOwners: map[string]string{"held": "neve"}, wantPrime: "neve",
			wantSubject: "board: owner neve for held",
		},
		{
			name: "OwnedElsewhereIsSkippedWithoutTakeover", primeOwner: "neve",
			req:          OwnerRequest{Machine: "neve", Slugs: []string{"away", "alpha"}},
			wantRecorded: []string{"alpha"}, wantLost: map[string]string{"away": "box"}, wantOwners: map[string]string{"away": "box", "alpha": "neve"}, wantPrime: "neve",
			wantSubject: "board: owner neve for away, alpha",
		},
		{
			name: "AbsentEntryWritesNothing", primeOwner: "neve",
			req:       OwnerRequest{Machine: "neve", Slugs: []string{"ghost"}},
			wantPrime: "neve",
		},
		{
			name: "PrimeIsClaimedWhenNoneIsRecorded", primeOwner: "",
			req:          OwnerRequest{Machine: "neve", Prime: true},
			wantRecorded: []string{OwnerWritePrime}, wantPrime: "neve",
			wantSubject: "board: owner neve for prime",
		},
		{
			name: "PrimeOwnedElsewhereIsSkippedWithoutTakeover", primeOwner: "box",
			req:      OwnerRequest{Machine: "neve", Prime: true},
			wantLost: map[string]string{OwnerWritePrime: "box"}, wantPrime: "box",
		},
		{
			name: "ClaimStatusLandsInTheSameCommit", primeOwner: "neve",
			req:          OwnerRequest{Machine: "neve", Slugs: []string{"alpha"}, Claim: RunStatus("running", "Worktree-Create")},
			wantRecorded: []string{"alpha"}, wantOwners: map[string]string{"alpha": "neve"}, wantPrime: "neve", wantClaim: RunStatus("running", "Worktree-Create"),
			wantSubject: "board: owner neve for alpha",
		},
	}
	for _, tt := range outcomes {
		t.Run("Outcome_"+tt.name, func(t *testing.T) {
			t.Parallel()
			f := newOwnerFixture(t, tt.primeOwner)
			tt.req.Sleep = failOnSleep(t)

			got, err := SetOwners(f.boardDir, tt.req)
			if err != nil {
				t.Fatalf("SetOwners() error = %v; want nil", err)
			}
			if !slices.Equal(got.Recorded, tt.wantRecorded) {
				t.Errorf("SetOwners() Recorded = %v; want %v", got.Recorded, tt.wantRecorded)
			}
			if !maps.Equal(got.Lost, tt.wantLost) {
				t.Errorf("SetOwners() Lost = %v; want %v", got.Lost, tt.wantLost)
			}

			origin := f.originOwners(t)
			for slug, want := range tt.wantOwners {
				if origin.Entries[slug].Owner != want {
					t.Errorf("origin owner of %q = %q; want %q", slug, origin.Entries[slug].Owner, want)
				}
			}
			if origin.Prime != tt.wantPrime {
				t.Errorf("origin prime owner = %q; want %q", origin.Prime, tt.wantPrime)
			}
			if tt.wantSubject != "" {
				readme, err := os.ReadFile(filepath.Join(f.boardDir, "README.md"))
				if err != nil {
					t.Fatalf("read README.md: %v", err)
				}
				if want := "Prime owner: `" + tt.wantPrime + "`.\n"; !strings.Contains(string(readme), want) {
					t.Errorf("README.md = %q; want it to contain %q", readme, want)
				}
			}
			if tt.wantClaim != "" {
				tasks, err := ParseBoardTasks([]byte(gitkit.Git(t, f.bare, "show", "main:board.json")))
				if err != nil {
					t.Fatalf("parse origin tasks: %v", err)
				}
				i := slices.IndexFunc(tasks, func(task Task) bool { return task.Slug == "alpha" })
				if i < 0 {
					t.Fatalf("origin board.json holds no entry alpha")
				}
				if tasks[i].Status == nil || *tasks[i].Status != tt.wantClaim {
					t.Errorf("origin status of alpha = %v; want %q", tasks[i].Status, tt.wantClaim)
				}
			}
			head := gitkit.RevParse(t, f.bare, "main")
			if tt.wantSubject == "" {
				if head != f.seedHead {
					t.Errorf("origin head = %s; want the seed %s, since nothing was written", head, f.seedHead)
				}
				return
			}
			if subject := gitkit.Git(t, f.bare, "log", "-1", "--format=%s", "main"); subject != tt.wantSubject {
				t.Errorf("origin head subject = %q; want %q", subject, tt.wantSubject)
			}
		})
	}

	refusals := []struct {
		name string
		// setup runs after the fixture is built and before SetOwners, and may return a BeforePush.
		setup      func(t *testing.T, f *ownerFixture) (beforePush func())
		machine    string
		wantIs     error
		wantSubstr string
		// wantKept requires the owner commit to stay in the local board, ahead of origin.
		wantKept bool
	}{
		{
			name:       "EmptyMachineIsRefused",
			setup:      func(t *testing.T, f *ownerFixture) func() { return nil },
			machine:    "",
			wantSubstr: "internal/machinename",
		},
		{
			name: "UnreachableOriginAtPull",
			setup: func(t *testing.T, f *ownerFixture) func() {
				gitkit.Git(t, f.boardDir, "remote", "set-url", "origin", unreachableOrigin)
				return nil
			},
			machine: "neve",
			wantIs:  ErrOwnerOriginUnreachable,
		},
		{
			name: "TransportFailureAtPush",
			setup: func(t *testing.T, f *ownerFixture) func() {
				return func() { gitkit.Git(t, f.boardDir, "remote", "set-url", "origin", unreachableOrigin) }
			},
			machine:  "neve",
			wantIs:   ErrOwnerOriginUnreachable,
			wantKept: true,
		},
		{
			name: "DivergedPull",
			setup: func(t *testing.T, f *ownerFixture) func() {
				gitkit.CommitFile(t, f.boardDir, "notes/c.md", "mine\n", "local note")
				gitkit.CommitFile(t, f.otherDir, "notes/c.md", "theirs\n", "other note")
				gitkit.Git(t, f.otherDir, "push", "origin", "main")
				return nil
			},
			machine:    "neve",
			wantSubstr: "holds commits its upstream lacks",
		},
		{
			name: "DirtyPull",
			setup: func(t *testing.T, f *ownerFixture) func() {
				writeBoardFile(t, f.boardDir, "notes/x.md", "mine\n")
				gitkit.CommitFile(t, f.otherDir, "notes/x.md", "theirs\n", "other edit")
				gitkit.Git(t, f.otherDir, "push", "origin", "main")
				return nil
			},
			machine:    "neve",
			wantSubstr: "lyx board sync",
		},
	}
	for _, tt := range refusals {
		t.Run("Refusal_"+tt.name, func(t *testing.T) {
			t.Parallel()
			f := newOwnerFixture(t, "neve")
			beforePush := tt.setup(t, f)
			headBefore := gitkit.RevParse(t, f.boardDir, "HEAD")
			boardBefore, err := os.ReadFile(filepath.Join(f.boardDir, boardFile))
			if err != nil {
				t.Fatalf("read board.json: %v", err)
			}

			got, err := SetOwners(f.boardDir, OwnerRequest{Machine: tt.machine, Slugs: []string{"alpha"}, BeforePush: beforePush, Sleep: failOnSleep(t)})
			if err == nil {
				t.Fatalf("SetOwners() error = nil; want a refusal")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("SetOwners() error = %v; want it to match %v", err, tt.wantIs)
			}
			if tt.wantSubstr != "" && !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("SetOwners() error = %v; want it to name %q", err, tt.wantSubstr)
			}
			if len(got.Recorded) != 0 {
				t.Errorf("SetOwners() Recorded = %v; want none", got.Recorded)
			}

			headAfter := gitkit.RevParse(t, f.boardDir, "HEAD")
			boardAfter, err := os.ReadFile(filepath.Join(f.boardDir, boardFile))
			if err != nil {
				t.Fatalf("read board.json: %v", err)
			}
			if tt.wantKept {
				if headAfter == headBefore {
					t.Errorf("local head = %s; want the owner commit kept on top of it", headAfter)
				}
				if origin := gitkit.RevParse(t, f.bare, "main"); origin != f.seedHead {
					t.Errorf("origin head = %s; want the seed %s, since the push failed", origin, f.seedHead)
				}
				return
			}
			if headAfter != headBefore || string(boardAfter) != string(boardBefore) {
				t.Errorf("a refused write changed the local board: head %s -> %s", headBefore, headAfter)
			}
		})
	}

	t.Run("CompetingWriteLosesToOrigins", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")

		got, err := SetOwners(f.boardDir, OwnerRequest{
			Machine: "neve", Slugs: []string{"alpha"}, Sleep: failOnSleep(t),
			BeforePush: func() { f.pushFromOther(t, setOwnerOnOther("alpha", "box")) },
		})
		if err != nil {
			t.Fatalf("SetOwners() error = %v; want nil", err)
		}
		if len(got.Recorded) != 0 || !maps.Equal(got.Lost, map[string]string{"alpha": "box"}) {
			t.Errorf("SetOwners() = Recorded %v, Lost %v; want none recorded and alpha lost to box", got.Recorded, got.Lost)
		}
		if owner := f.originOwners(t).Entries["alpha"].Owner; owner != "box" {
			t.Errorf("origin owner of alpha = %q; want origin's own value box to stand", owner)
		}
	})

	deferrals := []struct {
		name string
		// releaseOnSleep releases the held board lock on the first wait, so the retried push lands.
		releaseOnSleep bool
	}{
		{name: "RetriedPushLands", releaseOnSleep: true},
		{name: "PushStillAheadIsRefused", releaseOnSleep: false},
	}
	for _, tt := range deferrals {
		t.Run("DeferredPush_"+tt.name, func(t *testing.T) {
			t.Parallel()
			f := newOwnerFixture(t, "neve")

			var held *lock.FileLock
			release := func() {
				if held != nil {
					_ = held.Release()
					held = nil
				}
			}
			t.Cleanup(release)
			var waits []time.Duration
			req := OwnerRequest{
				Machine: "neve", Slugs: []string{"alpha"},
				// The other clone moves origin by a change to another entry, so the first push is rejected, and the held board lock defers the replay that would retry it.
				BeforePush: func() {
					f.pushFromOther(t, func(store *Store) {
						for i := range store.tasks {
							if store.tasks[i].Slug == "beta" {
								store.tasks[i].Brief = "changed elsewhere"
							}
						}
					})
					lockHeld, acquired, err := lock.TryAcquireWriteLock(filepath.Join(f.boardDir, fabricengine.BoardWriteLockFile))
					if err != nil || !acquired {
						t.Fatalf("take the board write lock: acquired %v, error %v", acquired, err)
					}
					held = lockHeld
				},
				Sleep: func(d time.Duration) {
					waits = append(waits, d)
					if tt.releaseOnSleep {
						release()
					}
				},
			}

			got, err := SetOwners(f.boardDir, req)
			if len(waits) != 1 || waits[0] != time.Second {
				t.Errorf("SetOwners() waited %v; want one wait of %v", waits, time.Second)
			}
			if !tt.releaseOnSleep {
				if err == nil || !strings.Contains(err.Error(), "lyx board sync") {
					t.Fatalf("SetOwners() error = %v; want a refusal naming `lyx board sync`", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetOwners() error = %v; want nil", err)
			}
			if !slices.Equal(got.Recorded, []string{"alpha"}) {
				t.Errorf("SetOwners() Recorded = %v; want [alpha]", got.Recorded)
			}
			if owner := f.originOwners(t).Entries["alpha"].Owner; owner != "neve" {
				t.Errorf("origin owner of alpha = %q; want neve", owner)
			}
		})
	}

	t.Run("SkipGitWritesTheOwnerWithNoCommit", func(t *testing.T) {
		t.Setenv("BOARD_SKIP_GIT", "1")
		f := newOwnerFixture(t, "neve")

		got, err := SetOwners(f.boardDir, OwnerRequest{Machine: "neve", Slugs: []string{"alpha"}, Sleep: failOnSleep(t)})
		if err != nil {
			t.Fatalf("SetOwners() error = %v; want nil", err)
		}
		if !slices.Equal(got.Recorded, []string{"alpha"}) {
			t.Errorf("SetOwners() Recorded = %v; want [alpha]", got.Recorded)
		}
		owners, err := ReadOwners(f.boardDir)
		if err != nil {
			t.Fatalf("ReadOwners() error = %v", err)
		}
		if owners.Entries["alpha"].Owner != "neve" {
			t.Errorf("local owner of alpha = %q; want neve", owners.Entries["alpha"].Owner)
		}
		if head := gitkit.RevParse(t, f.boardDir, "HEAD"); head != f.seedHead {
			t.Errorf("local head = %s; want the seed %s, since BOARD_SKIP_GIT commits nothing", head, f.seedHead)
		}
	})
}
