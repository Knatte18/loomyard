// record_test.go pins the pause record's read, locked update, coverage and interval behavior.

package pauserecord

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var epoch = time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

func TestRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	undecodable := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(valid, []byte(`{"live":[{"scope":"hub","caller":"EXTERNAL","paused_at":"2026-01-10T12:00:00Z"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(undecodable, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		path      string
		wantLive  int
		wantError string
	}{
		{"missing file reads empty", filepath.Join(dir, "missing.json"), 0, ""},
		{"valid file decodes", valid, 1, ""},
		{"undecodable file names its path", undecodable, 0, undecodable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			record, err := Read(tt.path)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("Read error = %v, want one naming %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if len(record.Live) != tt.wantLive {
				t.Fatalf("live entries = %d, want %d", len(record.Live), tt.wantLive)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	t.Run("creates the file and its directory", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "nested", RecordFileName)
		_, err := Update(path, epoch, func(r *Record) error {
			r.Live = append(r.Live, Entry{Scope: ScopeHub, PausedAt: epoch})
			return nil
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		record, err := Read(path)
		if err != nil || len(record.Live) != 1 || record.Live[0].Scope != ScopeHub {
			t.Fatalf("Read after Update = %+v, %v; want the hub entry", record, err)
		}
	})

	t.Run("a mutate error writes nothing", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), RecordFileName)
		if _, err := Update(path, epoch, func(r *Record) error {
			r.Live = append(r.Live, Entry{Scope: "a"})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		wantErr := errors.New("refused")
		_, err = Update(path, epoch, func(r *Record) error {
			r.Live = nil
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("Update error = %v, want %v", err, wantErr)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("file changed on a mutate error:\nbefore %s\nafter  %s", before, after)
		}
	})

	t.Run("prunes a resumed entry past retention and keeps a younger one", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), RecordFileName)
		record, err := Update(path, epoch, func(r *Record) error {
			r.Resumed = []Entry{
				{Scope: "old", ResumedAt: epoch.Add(-Retention - time.Hour)},
				{Scope: "young", ResumedAt: epoch.Add(-Retention + time.Hour)},
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if len(record.Resumed) != 1 || record.Resumed[0].Scope != "young" {
			t.Fatalf("resumed = %+v, want only the young entry", record.Resumed)
		}
	})

	t.Run("keeps keys the record types do not declare", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), RecordFileName)
		seed := `{"future_top":1,"live":[{"scope":"a","future_entry":"x"}]}`
		if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Update(path, epoch, func(r *Record) error {
			r.Live = append(r.Live, Entry{Scope: "b"})
			return nil
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{`"future_top"`, `"future_entry"`} {
			if !strings.Contains(string(data), key) {
				t.Errorf("record after Update lost %s:\n%s", key, data)
			}
		}
	})

	t.Run("keeps keys of an entry that shifts index when an earlier one is removed", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			seed   string
			mutate func(r *Record)
		}{
			{
				name: "live entry behind a lifted one",
				seed: `{"live":[{"scope":"a"},{"scope":"b","future_entry":"x"}]}`,
				mutate: func(r *Record) {
					r.Live = r.Live[1:]
				},
			},
			{
				name: "resumed entry behind an expired one",
				seed: `{"resumed":[{"scope":"old","resumed_at":"2000-01-01T00:00:00Z"},{"scope":"young","resumed_at":"2030-01-01T00:00:00Z","future_entry":"x"}]}`,
				mutate: func(r *Record) {
					r.Live = nil
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				path := filepath.Join(t.TempDir(), RecordFileName)
				if err := os.WriteFile(path, []byte(tt.seed), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := Update(path, epoch, func(r *Record) error {
					tt.mutate(r)
					return nil
				}); err != nil {
					t.Fatalf("Update: %v", err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), `"future_entry"`) {
					t.Errorf("record after Update lost the shifted entry's key:\n%s", data)
				}
			})
		}
	})

	t.Run("concurrent updates each keep their entry", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), RecordFileName)
		var wg sync.WaitGroup
		for _, scope := range []string{"a", "b"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := Update(path, epoch, func(r *Record) error {
					r.Live = append(r.Live, Entry{Scope: scope})
					return nil
				}); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		record, err := Read(path)
		if err != nil || len(record.Live) != 2 {
			t.Fatalf("Read = %+v, %v; want both entries", record, err)
		}
	})
}

func TestCovering(t *testing.T) {
	t.Parallel()
	hub := Entry{Scope: ScopeHub, PausedAt: epoch}
	slugEntry := Entry{Scope: "a", PausedAt: epoch.Add(time.Minute)}
	olderSlugEntry := Entry{Scope: "a", PausedAt: epoch.Add(-time.Minute)}

	tests := []struct {
		name      string
		live      []Entry
		slug      string
		wantScope string
		wantFound bool
	}{
		{"hub entry covers a slug", []Entry{hub}, "a", ScopeHub, true},
		{"slug entry covers its slug", []Entry{slugEntry}, "a", "a", true},
		{"slug entry leaves another slug", []Entry{slugEntry}, "b", "", false},
		{"oldest of hub and slug entries wins", []Entry{hub, olderSlugEntry}, "a", "a", true},
		{"hub is oldest when it was first", []Entry{slugEntry, hub}, "a", ScopeHub, true},
		{"no entry covers nothing", nil, "a", "", false},
		{"empty slug is covered by a hub entry", []Entry{hub}, "", ScopeHub, true},
		{"empty slug is not covered by a slug entry", []Entry{slugEntry}, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, found := Record{Live: tt.live}.Covering(tt.slug)
			if found != tt.wantFound || got.Scope != tt.wantScope {
				t.Fatalf("Covering(%q) = %q, %v; want %q, %v", tt.slug, got.Scope, found, tt.wantScope, tt.wantFound)
			}
		})
	}
}

func TestOwner(t *testing.T) {
	t.Parallel()
	record := Record{Live: []Entry{{Scope: "a", Sessions: []Session{{GUID: "g1"}, {GUID: "g2"}}}}}
	if entry, ok := record.Owner("g2"); !ok || entry.Scope != "a" {
		t.Fatalf("Owner(g2) = %+v, %v; want entry a", entry, ok)
	}
	if _, ok := record.Owner("other"); ok {
		t.Fatal("Owner(other) found an entry")
	}
}

func TestCoverageOf(t *testing.T) {
	t.Parallel()
	now := epoch.Add(time.Hour)
	live := Entry{Scope: "a", PausedAt: epoch}
	resumed := Entry{Scope: ScopeHub, PausedAt: epoch.Add(-3 * time.Hour), ResumedAt: epoch.Add(-2 * time.Hour)}
	elsewhere := Entry{Scope: "b", PausedAt: epoch, ResumedAt: epoch.Add(time.Minute)}
	record := Record{Live: []Entry{live}, Resumed: []Entry{resumed, elsewhere}}

	got := record.CoverageOf("a", now)
	if !got.Covered || got.Entry.Scope != "a" {
		t.Fatalf("coverage = %+v, want covered by a", got)
	}
	want := []Interval{{epoch, now}, {resumed.PausedAt, resumed.ResumedAt}}
	if len(got.Intervals) != len(want) || got.Intervals[0] != want[0] || got.Intervals[1] != want[1] {
		t.Fatalf("intervals = %+v, want %+v", got.Intervals, want)
	}

	resumedOnly := Record{Resumed: []Entry{resumed}}.CoverageOf("a", now)
	if resumedOnly.Covered || len(resumedOnly.Intervals) != 1 {
		t.Fatalf("resumed-only coverage = %+v, want uncovered with its one interval", resumedOnly)
	}
}

func TestReadCoverage(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), RecordFileName)
	if _, err := Update(path, epoch, func(r *Record) error {
		r.Live = append(r.Live, Entry{Scope: "a", PausedAt: epoch})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	covered, err := ReadCoverage(path, "a", epoch.Add(time.Minute))
	if err != nil || !covered.Covered {
		t.Fatalf("ReadCoverage(a) = %+v, %v; want covered", covered, err)
	}
	other, err := ReadCoverage(path, "b", epoch.Add(time.Minute))
	if err != nil || other.Covered {
		t.Fatalf("ReadCoverage(b) = %+v, %v; want uncovered", other, err)
	}
}

func TestExcluded(t *testing.T) {
	t.Parallel()
	at := func(minutes int) time.Time { return epoch.Add(time.Duration(minutes) * time.Minute) }

	tests := []struct {
		name      string
		intervals []Interval
		want      time.Duration
	}{
		{"none", nil, 0},
		{"disjoint intervals add", []Interval{{at(0), at(10)}, {at(20), at(30)}}, 20 * time.Minute},
		{"overlapping intervals count once", []Interval{{at(0), at(20)}, {at(10), at(30)}}, 30 * time.Minute},
		{"contained interval adds nothing", []Interval{{at(0), at(30)}, {at(10), at(20)}}, 30 * time.Minute},
		{"parts outside the window are clipped", []Interval{{at(-10), at(10)}, {at(50), at(70)}}, 20 * time.Minute},
		{"an interval wholly outside adds nothing", []Interval{{at(-20), at(-10)}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Excluded(tt.intervals, at(0), at(60)); got != tt.want {
				t.Fatalf("Excluded = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoveredError(t *testing.T) {
	t.Parallel()
	pausedAt := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		entry Entry
		slug  string
		want  string
	}{
		{
			"hub entry",
			Entry{Scope: ScopeHub, PausedAt: pausedAt},
			"a",
			`pause: a hub-wide pause has held this run since 2026-01-10T12:00:00Z; way forward: run "lyx resume", then retry`,
		},
		{
			"slug entry",
			Entry{Scope: "a", PausedAt: pausedAt},
			"a",
			`pause: the pause of a has held this run since 2026-01-10T12:00:00Z; way forward: run "lyx resume a", then retry`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := CoveredError(tt.entry, tt.slug, "retry").Error(); got != tt.want {
				t.Fatalf("CoveredError = %q, want %q", got, tt.want)
			}
		})
	}
}
