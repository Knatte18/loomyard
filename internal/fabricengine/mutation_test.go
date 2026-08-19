// mutation_test.go covers the mutation-record vocabulary declared in mutation.go: append ordering,
// the hub-relative Target conversion, ref recording, copy/isolation semantics, JSON marshalling, and
// the nil-safety and value-receiver contracts the doc comments promise.
// It is untagged Tier 1: pure table/data tests with no git spawn and no filesystem access.

package fabricengine

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestMutations_AppendOrdering covers that three appends come back from Entries() in append order.
func TestMutations_AppendOrdering(t *testing.T) {
	t.Parallel()

	m := NewMutations("")
	m.Append(KindDirCreated, "/hub/one", "")
	m.Append(KindFileWritten, "/hub/two", "")
	m.AppendRef(KindBranchCreated, "feature-x", "")

	got := m.Entries()
	want := []Mutation{
		{Kind: KindDirCreated, Target: "/hub/one"},
		{Kind: KindFileWritten, Target: "/hub/two"},
		{Kind: KindBranchCreated, Target: "feature-x"},
	}
	if len(got) != len(want) {
		t.Fatalf("Entries() = %d entries; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Entries()[%d] = %+v; want %+v", i, got[i], want[i])
		}
	}
}

// TestMutations_Append_HubRelativeConversion covers Append's hub-relative Target conversion: a
// target below the hub root, the hub root itself, a target outside the hub root, and an empty
// hubRoot.
func TestMutations_Append_HubRelativeConversion(t *testing.T) {
	t.Parallel()

	hubRoot := filepath.Join(string(filepath.Separator), "hub", "root")

	tests := []struct {
		name    string
		hubRoot string
		target  string
		want    string
	}{
		{
			name:    "below hub root",
			hubRoot: hubRoot,
			target:  filepath.Join(hubRoot, "slug-weft"),
			want:    "slug-weft",
		},
		{
			name:    "hub root itself",
			hubRoot: hubRoot,
			target:  hubRoot,
			want:    ".",
		},
		{
			name:    "outside hub root",
			hubRoot: hubRoot,
			target:  filepath.Join(string(filepath.Separator), "elsewhere", "dir"),
			want:    filepath.ToSlash(filepath.Join(string(filepath.Separator), "elsewhere", "dir")),
		},
		{
			name:    "empty hubRoot",
			hubRoot: "",
			target:  filepath.Join(hubRoot, "slug-weft"),
			want:    filepath.ToSlash(filepath.Join(hubRoot, "slug-weft")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := NewMutations(tt.hubRoot)
			m.Append(KindPathRemoved, tt.target, "")
			got := m.Entries()
			if len(got) != 1 {
				t.Fatalf("Entries() = %d entries; want 1", len(got))
			}
			if got[0].Target != tt.want {
				t.Errorf("Append(%q).Target = %q; want %q", tt.target, got[0].Target, tt.want)
			}
		})
	}
}

// TestMutations_AppendRef covers that AppendRef records its ref verbatim and is unaffected by
// hubRoot.
func TestMutations_AppendRef(t *testing.T) {
	t.Parallel()

	hubRoot := filepath.Join(string(filepath.Separator), "hub", "root")
	m := NewMutations(hubRoot)
	m.AppendRef(KindBranchDeleted, "feature-x-weft", "cleanup")

	got := m.Entries()
	want := Mutation{Kind: KindBranchDeleted, Target: "feature-x-weft", Detail: "cleanup"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("AppendRef(...) recorded %+v; want [%+v]", got, want)
	}
}

// TestMutations_Entries_ReturnsCopy covers that Entries() returns a copy — mutating the returned
// slice does not change what a second Entries() call reports — and that an empty record returns a
// non-nil zero-length slice.
func TestMutations_Entries_ReturnsCopy(t *testing.T) {
	t.Parallel()

	empty := NewMutations("")
	got := empty.Entries()
	if got == nil {
		t.Error("Entries() on an empty record = nil; want a non-nil zero-length slice")
	}
	if len(got) != 0 {
		t.Errorf("Entries() on an empty record = %d entries; want 0", len(got))
	}

	m := NewMutations("")
	m.Append(KindDirCreated, "/hub/one", "")

	first := m.Entries()
	first[0].Target = "mutated"

	second := m.Entries()
	if second[0].Target != "/hub/one" {
		t.Errorf("second Entries() call = %q after mutating first; want unaffected %q", second[0].Target, "/hub/one")
	}
}

// TestMutations_Snapshot_Isolates covers that appending through the recorder after taking a
// snapshot does not change the snapshot's Len().
func TestMutations_Snapshot_Isolates(t *testing.T) {
	t.Parallel()

	m := NewMutations("")
	m.Append(KindDirCreated, "/hub/one", "")

	snap := m.Snapshot()
	if snap.Len() != 1 {
		t.Fatalf("Snapshot().Len() = %d; want 1", snap.Len())
	}

	m.Append(KindFileWritten, "/hub/two", "")

	if snap.Len() != 1 {
		t.Errorf("Snapshot().Len() after a later Append = %d; want unaffected 1", snap.Len())
	}
	if m.Len() != 2 {
		t.Errorf("m.Len() after Append = %d; want 2", m.Len())
	}
}

// TestMutations_MarshalJSON covers the empty, zero-value, empty-Detail, and non-empty-Detail
// marshalling cases, asserting the exact JSON byte string.
func TestMutations_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    Mutations
		want string
	}{
		{
			name: "empty record",
			m:    *NewMutations(""),
			want: `[]`,
		},
		{
			name: "zero-value record",
			m:    Mutations{},
			want: `[]`,
		},
		{
			name: "one entry with empty Detail",
			m: func() Mutations {
				m := NewMutations("")
				m.Append(KindDirCreated, "/hub/one", "")
				return m.Snapshot()
			}(),
			want: `[{"kind":"dir_created","target":"/hub/one"}]`,
		},
		{
			name: "one entry with non-empty Detail",
			m: func() Mutations {
				m := NewMutations("")
				m.Append(KindWorktreeReset, "/hub/one", "deadbeef")
				return m.Snapshot()
			}(),
			want: `[{"kind":"worktree_reset","target":"/hub/one","detail":"deadbeef"}]`,
		},
		{
			name: "merge staged entry",
			m: func() Mutations {
				m := NewMutations("")
				m.Append(KindMergeStaged, "/hub/warp", "deadbeef")
				return m.Snapshot()
			}(),
			want: `[{"kind":"merge_staged","target":"/hub/warp","detail":"deadbeef"}]`,
		},
		{
			name: "merge committed entry",
			m: func() Mutations {
				m := NewMutations("")
				m.Append(KindMergeCommitted, "/hub/warp", "cafebabe")
				return m.Snapshot()
			}(),
			want: `[{"kind":"merge_committed","target":"/hub/warp","detail":"cafebabe"}]`,
		},
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

// TestMutationRecord_EmbedsAndMarshalsUnderMutationsKey covers that a MutationRecord embedded in a
// throwaway local struct marshals its record under the "mutations" key, and that Mutated() returns
// the same entries.
func TestMutationRecord_EmbedsAndMarshalsUnderMutationsKey(t *testing.T) {
	t.Parallel()

	type result struct {
		MutationRecord
		Name string `json:"name"`
	}

	m := NewMutations("")
	m.Append(KindDirCreated, "/hub/one", "")

	r := result{MutationRecord: MutationRecord{Mutations: m.Snapshot()}, Name: "example"}

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

// TestMutations_NilReceiver_DoesNotPanic covers that Append and AppendRef are safe on a nil
// *Mutations receiver.
func TestMutations_NilReceiver_DoesNotPanic(t *testing.T) {
	t.Parallel()

	var m *Mutations
	m.Append(KindDirCreated, "/hub/one", "")
	m.AppendRef(KindBranchCreated, "feature-x", "")
}

// TestMutations_Extend covers that Extend appends other's entries verbatim in order, that it is a
// no-op on an empty source, and that it is safe on a nil receiver.
func TestMutations_Extend(t *testing.T) {
	t.Parallel()

	m := NewMutations("")
	m.Append(KindDirCreated, "/hub/one", "")

	other := NewMutations("")
	other.Append(KindFileWritten, "/hub/two", "")
	other.AppendRef(KindBranchCreated, "feature-x", "")

	m.Extend(other.Snapshot())

	got := m.Entries()
	want := []Mutation{
		{Kind: KindDirCreated, Target: "/hub/one"},
		{Kind: KindFileWritten, Target: "/hub/two"},
		{Kind: KindBranchCreated, Target: "feature-x"},
	}
	if len(got) != len(want) {
		t.Fatalf("Entries() = %d entries; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Entries()[%d] = %+v; want %+v", i, got[i], want[i])
		}
	}

	empty := NewMutations("")
	m2 := NewMutations("")
	m2.Append(KindDirCreated, "/hub/one", "")
	m2.Extend(empty.Snapshot())
	if m2.Len() != 1 {
		t.Errorf("Extend(empty).Len() = %d; want unaffected 1", m2.Len())
	}

	var nilRec *Mutations
	nilRec.Extend(other.Snapshot())
}

// TestMutations_Snapshot_NilReceiver covers that Snapshot is safe on a nil *Mutations receiver,
// returning the zero Mutations rather than panicking — CloneHub and Unwire both install their
// populating defer before the recorder exists, so the defer can observe a nil recorder on the
// earliest failure paths.
func TestMutations_Snapshot_NilReceiver(t *testing.T) {
	t.Parallel()

	var m *Mutations
	got := m.Snapshot()
	if got.Len() != 0 {
		t.Errorf("nil.Snapshot().Len() = %d; want 0", got.Len())
	}
}

// mutationsFromFunc returns a non-addressable Mutations value, mirroring the shape batches 5-7 rely
// on: a result type's Mutated() accessor returns Mutations, not *Mutations.
func mutationsFromFunc() Mutations {
	m := NewMutations("")
	m.Append(KindDirCreated, "/hub/one", "")
	return m.Snapshot()
}

// TestMutations_EntriesAndLen_CallableOnNonAddressableValue covers that Entries() and Len() are
// callable directly on the result of a function returning Mutations — the construct batches 5-7
// rely on, and the reason those two methods carry value receivers.
func TestMutations_EntriesAndLen_CallableOnNonAddressableValue(t *testing.T) {
	t.Parallel()

	if got := mutationsFromFunc().Len(); got != 1 {
		t.Errorf("mutationsFromFunc().Len() = %d; want 1", got)
	}
	if got := mutationsFromFunc().Entries(); len(got) != 1 {
		t.Errorf("mutationsFromFunc().Entries() = %d entries; want 1", len(got))
	}
}
