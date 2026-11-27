// mutation_test.go covers the record value and its carrier: JSON marshalling and the copies the record promises.

package fabriclayout

import (
	"encoding/json"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestMutations_MarshalJSON asserts the exact JSON byte string a record marshals to.
// Its cases cover empty, zero-value, empty-Detail and non-empty-Detail records.
func TestMutations_MarshalJSON(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	one := func(kind Kind, target, detail string) Mutations {
		return MutationsOf([]Mutation{{Kind: kind, Target: target, Detail: detail}})
	}
	tests := []struct {
		name string
		m    Mutations
		want string
	}{
		{"empty record", MutationsOf(nil), `[]`},
		{"zero-value record", Mutations{}, `[]`},
		{"one entry with empty Detail", one(KindDirCreated, "/hub/one", ""), `[{"kind":"dir_created","target":"/hub/one"}]`},
		{"one entry with non-empty Detail", one(KindWorktreeReset, "/hub/one", "deadbeef"), `[{"kind":"worktree_reset","target":"/hub/one","detail":"deadbeef"}]`},
		{"merge staged entry", one(KindMergeStaged, "/hub/warp", "deadbeef"), `[{"kind":"merge_staged","target":"/hub/warp","detail":"deadbeef"}]`},
		{"merge committed entry", one(KindMergeCommitted, "/hub/warp", "cafebabe"), `[{"kind":"merge_committed","target":"/hub/warp","detail":"cafebabe"}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(tt.m)
			if err != nil {
				t.Fatalf("json.Marshal(%+v) error = %v", tt.m, err)
			}
			if string(got) != tt.want {
				t.Errorf("json.Marshal(%+v) = %s; want %s", tt.m, got, tt.want)
			}
		})
	}
}

// TestMutationRecord_EmbedsAndMarshalsUnderMutationsKey covers a MutationRecord embedded in a local struct.
// It checks that the record marshals under the "mutations" key, and that Mutated() returns the same entries.
//
//testtiming:keep an embedded MutationRecord marshalling under the "mutations" key; coverage of its blocks by other tests does not show an assertion of this
func TestMutationRecord_EmbedsAndMarshalsUnderMutationsKey(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	type result struct {
		MutationRecord
		Name string `json:"name"`
	}

	record := MutationsOf([]Mutation{{Kind: KindDirCreated, Target: "/hub/one"}})
	r := result{MutationRecord: MutationRecord{Mutations: record}, Name: "example"}

	got, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal(%+v) error = %v", r, err)
	}
	want := `{"mutations":[{"kind":"dir_created","target":"/hub/one"}],"name":"example"}`
	if string(got) != want {
		t.Errorf("json.Marshal(%+v) = %s; want %s", r, got, want)
	}

	mutated := r.Mutated().Entries()
	if len(mutated) != 1 || mutated[0].Target != "/hub/one" {
		t.Errorf("Mutated().Entries() = %+v; want one entry targeting /hub/one", mutated)
	}
}

// TestMutationsOf covers that the record shares no slice with its input or with what Entries returns.
func TestMutationsOf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	input := []Mutation{{Kind: KindDirCreated, Target: "/hub/one"}}
	record := MutationsOf(input)
	input[0].Target = "changed input"
	first := record.Entries()
	if first[0].Target != "/hub/one" {
		t.Errorf("Entries() after changing the input = %q; want %q", first[0].Target, "/hub/one")
	}

	first[0].Target = "changed output"
	if second := record.Entries(); second[0].Target != "/hub/one" {
		t.Errorf("Entries() after changing an earlier answer = %q; want %q", second[0].Target, "/hub/one")
	}
}
