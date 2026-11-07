// concurrency_test.go covers concurrent board access: readers alongside a writer, and writers racing for the write lock.
//
// It verifies that readers see a consistent wiki, never a phantom-empty one,
// and that concurrent writers serialize without losing updates.
// It also benchmarks read latency while a writer hammers the wiki in the background.
// The tests run without git.

package boardtest

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestConcurrentReadsDuringUpserts runs many readers concurrently with a single writer and asserts
// reads never fail and always observe a consistent board.
// Reads bypass the write lock, and writes are atomic (temp file and rename),
// so every read sees a complete board.json: either the pre-upsert or the post-upsert state, never a partial one.
//
// The test is bound by the filesystem, not the CPU.
// Each write runs Board.boardCriticalSection, which performs an AtomicWrite of board.json, Home.md and _Sidebar.md.
// The readers loop until the writer closes the stop channel.
// Read-under-write coverage therefore depends on how long the writer runs, not on how many writes it makes.
// The writes constant therefore stays small: it bounds the filesystem work and keeps the race window open.
func TestConcurrentReadsDuringUpserts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cwd := seedWiki(t, 100)
	// seedWiki creates _lym/config/board.yaml with path: board, so the board dir is <cwd>/board
	cfg := boardengine.Config{Path: filepath.Join(cwd, "board"), Readme: "Home.md", DesignPrefix: "proposal-", Types: []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}}, Labels: []boardengine.Label{{Name: "undecided"}}, SkipGit: true}
	w := boardengine.New(cfg)

	const (
		readers = 8
		// writes is the number of upserts the writer runs.
		// Each write re-renders Home.md and _Sidebar.md, the dominant per-write cost.
		// No assertion depends on the count, which only has to keep the writer busy long enough for the readers to overlap it.
		writes = 10
	)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Single writer: repeatedly update task-0's title (never adds/removes, so the
	// task count stays 100 throughout and readers can assert on it).
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(stop)
		for i := 0; i < writes; i++ {
			if _, err := w.UpsertTask(map[string]any{
				"slug":  "task-0",
				"title": "Updated " + strconv.Itoa(i),
			}); err != nil {
				t.Errorf("writer upsert %d: %v", i, err)
				return
			}
		}
	}()

	// Readers: keep reading until the writer stops, validating each read.
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}

				task, found, err := w.GetTask("task-50")
				if err != nil {
					t.Errorf("reader GetTask: %v", err)
					return
				}
				if !found || task.Slug != "task-50" {
					t.Errorf("reader saw inconsistent task: found=%v task=%+v", found, task)
					return
				}

				tasks, err := w.ListTasksBrief(nil)
				if err != nil {
					t.Errorf("reader ListTasksBrief: %v", err)
					return
				}
				if len(tasks) != 100 {
					t.Errorf("reader saw %d tasks, want 100", len(tasks))
					return
				}

				// Yield between reads: the readers must run alongside the writer, not flat out.
				// Lock-free readers spinning with no pause peg every core and starve the single writer's upserts.
				// A 1 ms pause still lands many validated reads inside the writer's window and leaves the writer CPU to finish.
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentUpsertsDoNotLoseWrites launches many writers at once, each adding a distinct task
// to an initially empty board.
// The write lock must serialize the load, mutate and save cycle, so no update is lost and ids stay unique.
// Without that serialization, the test sees fewer than `writers` tasks or duplicate ids.
func TestConcurrentUpsertsDoNotLoseWrites(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cwd := seedWiki(t, 0)
	// seedWiki creates _lym/config/board.yaml with path: board, so the board dir is <cwd>/board
	cfg := boardengine.Config{Path: filepath.Join(cwd, "board"), Readme: "Home.md", DesignPrefix: "proposal-", Types: []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}}, Labels: []boardengine.Label{{Name: "undecided"}}, SkipGit: true}
	w := boardengine.New(cfg)

	const writers = 16
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			slug := "w-" + strconv.Itoa(n)
			if _, err := w.UpsertTask(map[string]any{"slug": slug, "title": slug, "labels": []string{"bug"}}); err != nil {
				t.Errorf("writer %d: %v", n, err)
			}
		}(i)
	}
	wg.Wait()

	tasks, err := w.ListTasksFull()
	if err != nil {
		t.Fatalf("ListTasksFull: %v", err)
	}
	if len(tasks) != writers {
		t.Fatalf("expected %d tasks after concurrent upserts, got %d (lost write?)", writers, len(tasks))
	}
	seen := make(map[int]bool)
	for _, task := range tasks {
		if seen[task.ID] {
			t.Errorf("duplicate id %d after concurrent upserts", task.ID)
		}
		seen[task.ID] = true
	}
}

// BenchmarkGetDuringUpsert measures read latency while a writer continuously upserts in the background.
// Reads take no lock, so the gap to an uncontended read is the cost a reader pays for a busy writer.
func BenchmarkGetDuringUpsert(b *testing.B) {
	tiergate.Require(b, 0)
	cwd := seedWiki(b, 100)
	// seedWiki creates _lym/config/board.yaml with path: board, so the board dir is <cwd>/board
	cfg := boardengine.Config{Path: filepath.Join(cwd, "board"), Readme: "Home.md", DesignPrefix: "proposal-", Types: []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}}, Labels: []boardengine.Label{{Name: "undecided"}}, SkipGit: true}
	w := boardengine.New(cfg)

	stop := make(chan struct{})
	var writerDone sync.WaitGroup
	writerDone.Add(1)
	go func() {
		defer writerDone.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			// Errors are surfaced by the correctness tests, not here;
			// the writer only needs to keep the wiki busy.
			_, _ = w.UpsertTask(map[string]any{"slug": "task-0", "title": "U" + strconv.Itoa(i)})
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, _, err := w.GetTask("task-50"); err != nil {
				b.Error(err) // b.Fatal is not allowed from RunParallel goroutines
				return
			}
		}
	})
	b.StopTimer()

	close(stop)
	writerDone.Wait()
}
