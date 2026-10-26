//go:build tier0

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

	t.Run("The owner seam reads an entry's effective owner against this machine's name and returns every seam error", func(t *testing.T) {
		t.Parallel()
		const board = `{"version":1,"prime_owner":"other","entries":[` +
			`{"slug":"mine","kind":"project","beam":true,"owner_machine":"here"},` +
			`{"slug":"theirs","kind":"project","beam":true,"owner_machine":"other"},` +
			`{"slug":"unowned","kind":"task"},` +
			`{"slug":"child","kind":"task"},` +
			`{"slug":"held","kind":"task","status":"running · Implementer"}]}`
		const noPrimeOwner = `{"version":1,"entries":[{"slug":"unowned","kind":"task"}]}`
		errAny := errors.New("any error")
		machine := func() (string, error) { return "here", nil }
		parentOf := func(_, slug string) (string, error) {
			if slug == "child" {
				return "theirs", nil
			}
			return "", nil
		}
		boom := errors.New("boom")
		for _, row := range []struct {
			name          string
			machine       func() (string, error)
			parentOf      func(hubPath, slug string) (string, error)
			board         string
			slug          string
			wantOwner     string
			wantSubject   string
			wantElsewhere bool
			wantErr       error
		}{
			{"recorded for this machine", machine, parentOf, board, "mine", "here", "mine", false, nil},
			{"recorded for another machine", machine, parentOf, board, "theirs", "other", "theirs", true, nil},
			{"unowned under another machine's prime owner", machine, parentOf, board, "unowned", "other", "root", true, nil},
			{"unowned under a parent owned elsewhere", machine, parentOf, board, "child", "other", "theirs", true, nil},
			{"held by a run with no owner reads its parent's owner", machine, parentOf, board, "held", "other", "root", true, nil},
			{"a hub with no prime owner", machine, parentOf, noPrimeOwner, "unowned", "", "root", false, nil},
			{"a machine error", func() (string, error) { return "", boom }, parentOf, board, "mine", "", "", false, boom},
			{"a parent resolver error", machine, func(_, _ string) (string, error) { return "", boom }, board, "unowned", "", "", false, boom},
			{"undecodable bytes", machine, parentOf, "{", "mine", "", "", false, errAny},
		} {
			owner, subject, elsewhere, err := OwnerReader(row.machine, row.parentOf)("/hub", []byte(row.board), row.slug)
			switch {
			case row.wantErr == errAny:
				if err == nil {
					t.Errorf("%s: OwnerReader error = nil; want an error", row.name)
				}
			case !errors.Is(err, row.wantErr):
				t.Errorf("%s: OwnerReader error = %v; want %v", row.name, err, row.wantErr)
			case owner != row.wantOwner || subject != row.wantSubject || elsewhere != row.wantElsewhere:
				t.Errorf("%s: OwnerReader = %q, %q, %v; want %q, %q, %v", row.name, owner, subject, elsewhere, row.wantOwner, row.wantSubject, row.wantElsewhere)
			}
		}

		f := &fakeSubstrates{}
		if f.teardown().OriginSeams(SeamOptions{}).OwnerOf != nil {
			t.Error("OriginSeams(SeamOptions{}).OwnerOf != nil; want no owner seam without a Machine")
		}
		seams := f.teardown().OriginSeams(SeamOptions{Machine: machine, ParentOf: func(slug string) (string, error) { return parentOf("", slug) }})
		if owner, subject, elsewhere, err := seams.OwnerOf("/hub", []byte(board), "child"); err != nil || owner != "other" || subject != "theirs" || !elsewhere {
			t.Errorf("OwnerOf over the options = %q, %q, %v, %v; want other, adopt subject theirs, owned elsewhere", owner, subject, elsewhere, err)
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

	t.Run("The teardown seam carries the ended verdict, so a busy driver is skipped past and the removal runs", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{quiets: []quietState{busy}}
		td := f.teardown()
		removal := &removeRecorder{}
		removal.install(f, td)

		report, err := td.OriginSeams(SeamOptions{}).Teardown(fabricengine.EndedTeardown{Slug: "slug"})

		if err != nil || report.Busy {
			t.Fatalf("Teardown() = %+v, %v; want a removed pair", report, err)
		}
		if removal.request.Verdict != fabricengine.EntryEnded || !slices.Contains(f.calls, "remove-local") || slices.Contains(f.calls, "quiet") {
			t.Errorf("verdict = %q, calls = %v; want the ended verdict, no quiet probe and the local removal", removal.request.Verdict, f.calls)
		}
	})

	t.Run("A driver lock still held after the owned end comes back as the busy flag with nothing removed", func(t *testing.T) {
		t.Parallel()
		f := &fakeSubstrates{quiets: []quietState{{quiet: true}}, driverLockHeld: true}
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
