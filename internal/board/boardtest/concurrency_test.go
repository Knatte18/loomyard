// concurrency_test.go — concurrency correctness + read-under-write contention.
//
// Verifies that many readers run correctly alongside a writer (reads see a
// consistent wiki, never a phantom-empty one) and that concurrent writers
// serialize through the write lock without losing updates. Also benchmarks read
// latency while a writer hammers the wiki in the background. All no-git.

package boardtest

import (
	"strconv"
	"sync"
	"testing"

	"github.com/Knatte18/mhgo/internal/board"
)

// TestConcurrentReadsDuringUpserts runs many readers concurrently with a single
// writer and asserts reads never fail and always observe a consistent wiki.
// Reads bypass the write lock and writes are atomic (temp + rename), so every
// read must see a complete tasks.json — either the pre- or post-upsert state,
// never a partial one.
func TestConcurrentReadsDuringUpserts(t *testing.T) {
	t.Setenv("WIKI_SKIP_GIT", "1")
	dir := seedWiki(t, 100)
	w := wiki.New(dir)

	const (
		readers = 8
		writes  = 300
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

				tasks, err := w.ListTasksBrief()
				if err != nil {
					t.Errorf("reader ListTasksBrief: %v", err)
					return
				}
				if len(tasks) != 100 {
					t.Errorf("reader saw %d tasks, want 100", len(tasks))
					return
				}
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentUpsertsDoNotLoseWrites launches many writers at once, each adding
// a distinct task to an initially empty wiki. The write lock must serialize the
// load → mutate → save cycle so no update is lost and ids stay unique; if the
// lock failed to serialize same-process writers, we would see fewer than `writers`
// tasks or duplicate ids.
func TestConcurrentUpsertsDoNotLoseWrites(t *testing.T) {
	t.Setenv("WIKI_SKIP_GIT", "1")
	dir := seedWiki(t, 0)
	w := wiki.New(dir)

	const writers = 16
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			slug := "w-" + strconv.Itoa(n)
			if _, err := w.UpsertTask(map[string]any{"slug": slug, "title": slug}); err != nil {
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

// BenchmarkGetDuringUpsert measures read latency while a writer continuously
// upserts in the background. Reads take no lock, so this should stay close to the
// uncontended BenchmarkGet — that gap is the price reads pay for a busy writer.
func BenchmarkGetDuringUpsert(b *testing.B) {
	b.Setenv("WIKI_SKIP_GIT", "1")
	dir := seedWiki(b, 100)
	w := wiki.New(dir)

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
			// Errors (if any) are surfaced by the correctness tests, not here;
			// the writer just needs to keep the wiki busy.
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
