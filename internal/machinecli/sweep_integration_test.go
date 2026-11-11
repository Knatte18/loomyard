// sweep_integration_test.go covers `lym machine name <name>` against real hubs:
// the collision check and the sweep over real Bolts, origin and the working directory's hub.

package machinecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// sweepFixture is two hubs and the deps that reach them.
// Hub a is in the machine record; hub b only holds the working directory.
type sweepFixture struct {
	a, b *hubforge.Hub
	d    deps
	file string
	// cwd is a subdirectory of hub b's prime worktree.
	cwd string
}

// openBoard opens the board at boardDir for seeding.
func openBoard(t *testing.T, boardDir string) *boardengine.Board {
	t.Helper()
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: boardDir}, "board")
	if err != nil {
		t.Fatalf("load the board config of %s: %v", boardDir, err)
	}
	cfg.Path = boardDir
	return boardengine.New(cfg)
}

// seedBeams upserts a project entry per slug and records machine as the owner of each as a beam.
func seedBeams(t *testing.T, board *boardengine.Board, boardDir, machine string, slugs ...string) {
	t.Helper()
	for _, slug := range slugs {
		if _, err := board.UpsertTask(map[string]any{"slug": slug, "kind": "project", "title": slug, "labels": []string{"bug"}}); err != nil {
			t.Fatalf("seed the entry %s: %v", slug, err)
		}
	}
	if _, err := boardengine.SetOwners(boardDir, boardengine.OwnerRequest{Machine: machine, Slugs: slugs, Beam: true, Takeover: true}); err != nil {
		t.Fatalf("record %s as the owner of %v: %v", machine, slugs, err)
	}
}

// recordPrime records machine as the prime owner of the board at boardDir.
func recordPrime(t *testing.T, boardDir, machine string) {
	t.Helper()
	if _, err := boardengine.SetOwners(boardDir, boardengine.OwnerRequest{Machine: machine, Prime: true, Takeover: true}); err != nil {
		t.Fatalf("record %s as the prime owner of %s: %v", machine, boardDir, err)
	}
}

// newSweepFixture builds the hubs, the registry and the machine file's directory of one subtest.
// Hub a holds an open beam, a done beam and a beam owned by far, and the prime.
// All but the far beam are under osl-1046.
// Hub b holds the prime under osl-1046.
// A second clone of hub a pushes a beam under osl-1046 and one under neve to origin only.
func newSweepFixture(t *testing.T) sweepFixture {
	t.Helper()
	boardkit.InstallReplay(t)
	a := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	b := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	boardA := openBoard(t, a.BoardDir())
	seedBeams(t, boardA, a.BoardDir(), "osl-1046", "a-open", "a-done")
	seedBeams(t, boardA, a.BoardDir(), "far", "a-far")
	done := boardengine.StatusDone
	if err := boardA.SetStatus("a-done", &done); err != nil {
		t.Fatalf("mark a-done done: %v", err)
	}
	// The prime write is last, so its push carries every earlier commit of hub a to origin before the second clone pushes.
	recordPrime(t, a.BoardDir(), "osl-1046")
	recordPrime(t, b.BoardDir(), "osl-1046")

	second := hubforge.CloneSecond(t, a)
	boardSecond := openBoard(t, second.BoardDir())
	seedBeams(t, boardSecond, second.BoardDir(), "osl-1046", "o-old")
	seedBeams(t, boardSecond, second.BoardDir(), "neve", "o-neve")

	registryDir := t.TempDir()
	if err := hubregistry.Record(registryDir, a.Path, hubforge.TestShortname, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("record hub a in the machine record: %v", err)
	}
	cwd := filepath.Join(b.PrimeWorktree(), "sub")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("make the working directory %s: %v", cwd, err)
	}
	file := filepath.Join(t.TempDir(), "machine.json")
	d := productionDeps()
	d.locate = func() (string, string, bool, error) { return file, file + ".lock", true, nil }
	d.hostName = hostNamed("osl-1046")
	d.hubs = func() ([]string, error) { return liveHubs(registryDir) }
	return sweepFixture{a: a, b: b, d: d, file: file, cwd: cwd}
}

func TestName_SweepsHubs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("CollisionWithAnOriginOnlyOwnerIsRefused", func(t *testing.T) {
		t.Parallel()
		f := newSweepFixture(t)

		code, envelope := runName(t, f.d, f.cwd, "neve")

		message, _ := envelope["error"].(string)
		if code == 0 || !strings.Contains(message, "o-neve") || !strings.Contains(message, `"neve"`) {
			t.Errorf("set exit %d, envelope %v; want a refusal naming o-neve and neve, held only by origin before the verb ran", code, envelope)
		}
		if state := fileState(t, f.file); state != "absent" {
			t.Errorf("machine file after the refusal = %q; want none written", state)
		}
	})

	t.Run("RenameSweepsEveryHub", func(t *testing.T) {
		t.Parallel()
		f := newSweepFixture(t)

		code, envelope := runName(t, f.d, f.cwd, "osl46")

		if code != 0 {
			t.Fatalf("set exit %d, envelope %v; want ok", code, envelope)
		}
		if outcomes := hubOutcomes(t, envelope); len(outcomes) != 2 {
			t.Errorf("hub outcomes = %v; want hub a from the record and hub b from the working directory", outcomes)
		}
		ownersA, err := boardengine.ReadOwners(f.a.BoardDir())
		if err != nil {
			t.Fatalf("read hub a's owners: %v", err)
		}
		for slug, want := range map[string]string{"a-open": "osl46", "o-old": "osl46", "a-done": "osl-1046", "a-far": "far", "o-neve": "neve"} {
			if got := ownersA.Entries[slug].Owner; got != want {
				t.Errorf("hub a owner of %s = %q; want %q", slug, got, want)
			}
		}
		if ownersA.Prime != "osl46" {
			t.Errorf("hub a prime owner = %q; want osl46", ownersA.Prime)
		}
		ownersB, err := boardengine.ReadOwners(f.b.BoardDir())
		if err != nil {
			t.Fatalf("read hub b's owners: %v", err)
		}
		if ownersB.Prime != "osl46" {
			t.Errorf("hub b prime owner = %q; want osl46, reached through the working directory's subdirectory", ownersB.Prime)
		}
	})
}
