//go:build tier0

// owners_test.go — ParseOwners, Owners.Effective and OwnedElsewhere.

package boardengine

import (
	"errors"
	"testing"
)

func TestOwnersEffective(t *testing.T) {
	t.Parallel()

	errParent := errors.New("parent unreadable")
	parents := map[string]string{"child": "", "pair": "", "running": "", "member": "beam", "group": "beam", "grand": "group", "beam": ""}
	parentOf := func(slug string) (string, error) { return parents[slug], nil }
	failingParentOf := func(string) (string, error) { return "", errParent }

	entries := `"entries":[` +
		`{"id":0,"slug":"child","title":"c","kind":"task","labels":["bug"],"status":"idle"},` +
		`{"id":1,"slug":"pair","title":"p","kind":"task","labels":["bug"],"owner_machine":"stray"},` +
		`{"id":2,"slug":"beam","title":"b","kind":"project","labels":["bug"],"beam":true,"owner_machine":"box"},` +
		`{"id":3,"slug":"member","title":"m","kind":"task","labels":["bug"],"project":"beam"},` +
		`{"id":4,"slug":"group","title":"g","kind":"project","labels":["bug"],"project":"beam"},` +
		`{"id":5,"slug":"grand","title":"gr","kind":"task","labels":["bug"],"project":"group"},` +
		`{"id":6,"slug":"running","title":"r","kind":"task","labels":["bug"],"status":"` + RunStatus("running", "Implementer") + `"}]`
	board := func(prime string) []byte {
		if prime == "" {
			return []byte(`{"version":1,` + entries + `}`)
		}
		return []byte(`{"version":1,"prime_owner":"` + prime + `",` + entries + `}`)
	}

	tests := []struct {
		name     string
		board    []byte
		slug     string
		parentOf func(string) (string, error)
		want     string
		wantErr  error
		parseErr bool
	}{
		{name: "EmptyBytesHoldNoEntry", board: nil, slug: "child", parentOf: parentOf, want: ""},
		{name: "UndecodableBytesReturnTheError", board: []byte(`{`), slug: "child", parentOf: parentOf, parseErr: true},
		{name: "RootTaskWithNoPrimeOwnerReadsAsNoOwner", board: board(""), slug: "child", parentOf: parentOf, want: ""},
		{name: "BeamOwnerHoldsOnABoardWithNoPrimeOwner", board: board(""), slug: "member", parentOf: parentOf, want: "box"},
		{name: "BeamReadsItsRecordedOwner", board: board("neve"), slug: "beam", parentOf: parentOf, want: "box"},
		{name: "RecordedTaskOwnerIsIgnored", board: board("neve"), slug: "pair", parentOf: parentOf, want: "neve"},
		{name: "RunHeldTaskWithNoOwnerReadsItsParentsOwner", board: board("neve"), slug: "running", parentOf: parentOf, want: "neve"},
		{name: "TaskWithNoBeamReadsThePrime", board: board("neve"), slug: "child", parentOf: parentOf, want: "neve"},
		{name: "TaskReadsItsBeamsOwner", board: board("neve"), slug: "member", parentOf: parentOf, want: "box"},
		{name: "GroupingProjectReadsItsParentsOwner", board: board("neve"), slug: "group", parentOf: parentOf, want: "box"},
		{name: "AbsentSlugReadsAsNoOwner", board: board("neve"), slug: "ghost", parentOf: parentOf, want: ""},
		{name: "ParentErrorIsReturned", board: board("neve"), slug: "child", parentOf: failingParentOf, wantErr: errParent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			owners, err := ParseOwners(tt.board)
			if tt.parseErr {
				if err == nil {
					t.Fatalf("ParseOwners(%q) error = nil; want an error", tt.board)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseOwners(%q) error = %v; want nil", tt.board, err)
			}
			got, err := owners.Effective(tt.slug, tt.parentOf)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Effective(%q) error = %v; want %v", tt.slug, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Effective(%q) = %q; want %q", tt.slug, got, tt.want)
			}
		})
	}

	t.Run("AdoptSubject", func(t *testing.T) {
		t.Parallel()
		owners, err := ParseOwners(board("neve"))
		if err != nil {
			t.Fatalf("ParseOwners() error = %v; want nil", err)
		}
		subjects := []struct {
			name     string
			slug     string
			parentOf func(string) (string, error)
			want     string
			wantErr  error
		}{
			{"BeamIsItsOwnSubject", "beam", parentOf, "beam", nil},
			{"MemberThroughAGroupingProjectTakesItsBeam", "grand", parentOf, "beam", nil},
			{"RootTaskTakesRoot", "child", parentOf, RootSubject, nil},
			{"AbsentSlugIsItsOwnSubject", "ghost", parentOf, "ghost", nil},
			{"ParentErrorIsReturned", "child", failingParentOf, "", errParent},
		}
		for _, tt := range subjects {
			got, err := owners.AdoptSubject(tt.slug, tt.parentOf)
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("%s: AdoptSubject(%q) = (%q, %v); want (%q, %v)", tt.name, tt.slug, got, err, tt.want, tt.wantErr)
			}
		}
	})

	t.Run("ReadsProjectAndBeam", func(t *testing.T) {
		t.Parallel()
		owners, err := ParseOwners([]byte(`{"version":1,"entries":[` +
			`{"id":0,"slug":"beamer","title":"b","kind":"project","labels":["bug"],"beam":true,"owner_machine":"box","beam_color":"#7d2d6b"},` +
			`{"id":1,"slug":"grouping","title":"g","kind":"project","labels":["bug"]},` +
			`{"id":2,"slug":"work","title":"w","kind":"task","labels":["bug"]}]}`))
		if err != nil {
			t.Fatalf("ParseOwners() error = %v; want nil", err)
		}
		want := map[string]OwnerEntry{
			"beamer":   {Owner: "box", IsProject: true, Beam: true, BeamColor: "#7d2d6b"},
			"grouping": {IsProject: true},
			"work":     {IsTask: true},
		}
		for slug, entry := range want {
			if got := owners.Entries[slug]; got != entry {
				t.Errorf("Entries[%q] = %+v; want %+v", slug, got, entry)
			}
		}
	})

	elsewhere := []struct {
		name        string
		owner, here string
		want        bool
	}{
		{"EmptyOwnerIsOwnedByNoOther", "", "neve", false},
		{"SameOwnerIsNotElsewhere", "neve", "neve", false},
		{"OtherOwnerIsElsewhere", "box", "neve", true},
	}
	for _, tt := range elsewhere {
		t.Run("OwnedElsewhere_"+tt.name, func(t *testing.T) {
			t.Parallel()
			if got := OwnedElsewhere(tt.owner, tt.here); got != tt.want {
				t.Errorf("OwnedElsewhere(%q, %q) = %v; want %v", tt.owner, tt.here, got, tt.want)
			}
		})
	}
}
