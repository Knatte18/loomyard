// owners_test.go — ParseOwners, Owners.Effective and OwnedElsewhere.

package boardengine

import (
	"errors"
	"testing"
)

func TestOwnersEffective(t *testing.T) {
	t.Parallel()

	errParent := errors.New("parent unreadable")
	parents := map[string]string{"child": "", "grand": "pair", "pair": ""}
	parentOf := func(slug string) (string, error) { return parents[slug], nil }
	failingParentOf := func(string) (string, error) { return "", errParent }

	entries := `"entries":[` +
		`{"id":0,"slug":"child","title":"c","kind":"task","labels":["bug"],"status":"idle"},` +
		`{"id":1,"slug":"pair","title":"p","kind":"task","labels":["bug"],"owner_machine":"box"},` +
		`{"id":2,"slug":"grand","title":"g","kind":"task","labels":["bug"]},` +
		`{"id":3,"slug":"running","title":"r","kind":"task","labels":["bug"],"status":"` + RunStatus("running", "Implementer") + `"}]`
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
		{name: "NoPrimeOwnerReadsAsNoOwner", board: board(""), slug: "pair", parentOf: parentOf, want: ""},
		{name: "RecordedOwnerStands", board: board("neve"), slug: "pair", parentOf: parentOf, want: "box"},
		{name: "RunHeldWithoutOwnerReadsAsNoOwner", board: board("neve"), slug: "running", parentOf: parentOf, want: ""},
		{name: "UnownedTaskFollowsThePrime", board: board("neve"), slug: "child", parentOf: parentOf, want: "neve"},
		{name: "UnownedTaskFollowsAPairParentsOwner", board: board("neve"), slug: "grand", parentOf: parentOf, want: "box"},
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
