// originseams_test.go drives the origin reconcile's seams through the fake substrates, so it spawns nothing.

package pairteardown

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// removeRecorder replaces a teardown's removal with one that keeps the request it was given and answers with result and err.
type removeRecorder struct {
	request Request
	result  fabricengine.RemoveResult
	err     error
}

func (r *removeRecorder) install(f *fakeSubstrates, td *Teardown) {
	td.remove = func(req Request) (fabricengine.RemoveResult, error) {
		r.request = req
		f.calls = append(f.calls, "remove-local")
		return r.result, r.err
	}
}

// mutationRecord returns a record holding one created tag.
func mutationRecord(tag string) fabricengine.MutationRecord {
	rec := fabricengine.NewMutations("/hub")
	rec.AppendRef(fabricengine.KindTagCreated, tag, "")
	return fabricengine.MutationRecord{Mutations: rec.Snapshot()}
}

// TestTeardown_OriginSeams pins what the origin reconcile's seams read and how they drive a local removal.
func TestTeardown_OriginSeams(t *testing.T) {
	t.Parallel()

	t.Run("A quiet pair is not live and a busy one is", func(t *testing.T) {
		t.Parallel()
		for _, row := range []struct {
			name  string
			state quietState
			want  bool
		}{
			{"quiet", quietState{quiet: true}, false},
			{"busy", busy, true},
		} {
			f := &fakeSubstrates{quiets: []quietState{row.state}}
			seams := f.teardown().OriginSeams(SeamOptions{})
			if got, err := seams.LivePair("slug"); err != nil || got != row.want {
				t.Errorf("LivePair() over a %s pair = %v, %v; want %v", row.name, got, err, row.want)
			}
		}
	})

	t.Run("A busy pair under a live hub pause entry is not live, and reads live once the entry is lifted", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{quiets: []quietState{busy}}
		paused := true
		seams := f.teardown().OriginSeams(SeamOptions{HubPaused: func(string) (bool, error) { return paused, nil }})

		if got, err := seams.LivePair("slug"); err != nil || got {
			t.Errorf("LivePair() under a pause = %v, %v; want not live", got, err)
		}
		paused = false
		if got, err := seams.LivePair("slug"); err != nil || !got {
			t.Errorf("LivePair() after the pause was lifted = %v, %v; want live", got, err)
		}
	})

	t.Run("The prime parent lock seam is the options' own", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{}
		seams := f.teardown().OriginSeams(SeamOptions{PrimeParentLockFree: func() (bool, error) { return false, nil }})
		if free, err := seams.PrimeParentLockFree(); err != nil || free {
			t.Errorf("PrimeParentLockFree() = %v, %v; want held", free, err)
		}
	})

	t.Run("The teardown seam removes locally with a zero wait and refuses a busy driver", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{quiets: []quietState{{quiet: true}}}
		td := f.teardown()
		removal := &removeRecorder{}
		removal.install(f, td)

		report, err := td.OriginSeams(SeamOptions{}).Teardown(fabricengine.EndedTeardown{Slug: "slug"})

		if err != nil || report.Busy {
			t.Fatalf("Teardown() = %+v, %v; want a removed pair", report, err)
		}
		if !removal.request.Local || !removal.request.RefuseWhenBusy || removal.request.QuietWait != 0 || removal.request.Force || removal.request.Snapshot {
			t.Errorf("removal request = %+v; want local, refusing a busy driver, with a zero wait and no force", removal.request)
		}
	})

	t.Run("A busy driver comes back as the busy flag with nothing removed", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{quiets: []quietState{busy}}
		td := f.teardown()
		removal := &removeRecorder{}
		removal.install(f, td)

		report, err := td.OriginSeams(SeamOptions{}).Teardown(fabricengine.EndedTeardown{Slug: "slug"})

		if err != nil || !report.Busy || report.BusyOn != fabricengine.BusyOnDriver {
			t.Errorf("Teardown() = %+v, %v; want busy on the driver and no error", report, err)
		}
		if slices.Contains(f.calls, "remove-local") || slices.Contains(f.calls, "end") {
			t.Errorf("calls = %v; want nothing ended and nothing removed", f.calls)
		}
	})

	t.Run("A held teardown or records write lock comes back as the busy flag on a lock", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{quiets: []quietState{{quiet: true}}}
		td := f.teardown()
		removal := &removeRecorder{err: fmt.Errorf("the records write lock is held: %w", fabricengine.ErrPairBusy)}
		removal.install(f, td)

		report, err := td.OriginSeams(SeamOptions{}).Teardown(fabricengine.EndedTeardown{Slug: "slug"})

		if err != nil || !report.Busy || report.BusyOn != fabricengine.BusyOnLock {
			t.Errorf("Teardown() = %+v, %v; want busy on a lock and no error", report, err)
		}
	})

	t.Run("A removal's recorded mutation and any other error come back", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{quiets: []quietState{{quiet: true}}}
		td := f.teardown()
		boom := errors.New("boom")
		removal := &removeRecorder{result: fabricengine.RemoveResult{MutationRecord: mutationRecord("set-aside/slug/abc"), SetAsideTag: "set-aside/slug/abc"}, err: boom}
		removal.install(f, td)

		report, err := td.OriginSeams(SeamOptions{}).Teardown(fabricengine.EndedTeardown{Slug: "slug"})

		if !errors.Is(err, boom) || report.Busy {
			t.Errorf("Teardown() error = %v, busy = %v; want the removal's error", err, report.Busy)
		}
		if got := report.Mutations.Entries(); len(got) != 1 || got[0].Kind != fabricengine.KindTagCreated {
			t.Errorf("report mutations = %v; want the removal's one tag_created entry", got)
		}
		if !slices.Equal(report.Tags, []string{"set-aside/slug/abc"}) {
			t.Errorf("report Tags = %v; want the set-aside tag", report.Tags)
		}
	})

	t.Run("A wake hold forces the removal with a snapshot and lifts the hold once it is kept", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{quiets: []quietState{busy}}
		td := f.teardown()
		removal := &removeRecorder{}
		removal.install(f, td)
		opts := SeamOptions{
			WakeHeld:     func(slug string) (bool, error) { return slug == "slug", nil },
			LiftWakeHold: func(string) error { f.calls = append(f.calls, "lift"); return nil },
		}

		report, err := td.OriginSeams(opts).Teardown(fabricengine.EndedTeardown{Slug: "slug"})

		if err != nil || report.Busy {
			t.Fatalf("Teardown() = %+v, %v; want the busy driver ended and the pair removed", report, err)
		}
		if removal.request.RefuseWhenBusy || !removal.request.Force || !removal.request.Snapshot || !removal.request.Local {
			t.Errorf("removal request = %+v; want local, forced, with a snapshot and without refusing a busy driver", removal.request)
		}
		wantOrder := []string{"snapshot", "lift", "remove-local"}
		var got []string
		for _, call := range f.calls {
			if slices.Contains(wantOrder, call) {
				got = append(got, call)
			}
		}
		if !slices.Equal(got, wantOrder) {
			t.Errorf("calls = %v; want the snapshot, then the lift, then the removal", f.calls)
		}
	})

	t.Run("An earlier snapshot without a hold takes the force case with nothing to lift", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{quiets: []quietState{{quiet: true}}}
		td := f.teardown()
		removal := &removeRecorder{}
		removal.install(f, td)
		lifted := false
		opts := SeamOptions{LiftWakeHold: func(string) error { lifted = true; return nil }}

		_, err := td.OriginSeams(opts).Teardown(fabricengine.EndedTeardown{Slug: "slug", EarlierSnapshot: true})

		if err != nil {
			t.Fatalf("Teardown() error = %v; want nil", err)
		}
		if removal.request.RefuseWhenBusy || !removal.request.Force || !removal.request.Snapshot {
			t.Errorf("removal request = %+v; want forced with a snapshot and without refusing a busy driver", removal.request)
		}
		if lifted || !slices.Contains(f.calls, "snapshot") {
			t.Errorf("lifted = %v, calls = %v; want a snapshot and no lift", lifted, f.calls)
		}
	})
}
