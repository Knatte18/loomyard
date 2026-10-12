package state_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

// sample is a small struct type used to instantiate the generics in tests.
type sample struct {
	Name string
	N    int
}

// TestWriteJSON_RoundTripOverwriteAndLayout writes a value, reads it back, overwrites it with a different one, and verifies the second read returns the new value, the file is JSON indented by two spaces, and the directory holds only the data file and its lock file at exactly path + ".lock", with no .tmp- entries left behind.
//
//testtiming:keep pins the two-space indentation and the data-plus-lock-only directory layout, which no covering test asserts
func TestWriteJSON_RoundTripOverwriteAndLayout(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")
	lockPath := path + ".lock"

	orig := sample{Name: "test", N: 42}
	if err := state.WriteJSON(path, lockPath, orig); err != nil {
		t.Fatalf("WriteJSON() error: %v", err)
	}

	got, found, err := state.ReadJSON[sample](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() error: %v", err)
	}
	if !found {
		t.Fatal("ReadJSON() found = false; want true")
	}
	if got != orig {
		t.Errorf("ReadJSON() = %+v; want %+v", got, orig)
	}

	replacement := sample{Name: "second", N: 2}
	if err := state.WriteJSON(path, lockPath, replacement); err != nil {
		t.Fatalf("second WriteJSON() error: %v", err)
	}
	got, found, err = state.ReadJSON[sample](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() after overwrite error: %v", err)
	}
	if !found {
		t.Fatal("ReadJSON() after overwrite found = false; want true")
	}
	if got != replacement {
		t.Errorf("ReadJSON() after overwrite = %+v; want %+v", got, replacement)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	wantData, _ := json.MarshalIndent(replacement, "", "  ")
	if string(data) != string(wantData) {
		t.Errorf("JSON formatting mismatch:\ngot:\n%s\nwant:\n%s", string(data), string(wantData))
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir() error: %v", err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	if want := []string{"state.json", "state.json.lock"}; !slices.Equal(names, want) {
		t.Errorf("directory entries = %v; want exactly %v", names, want)
	}
}

// TestMissingFile reads a never-written path and verifies found=false, err=nil, and that the parent dir and lock file now exist.
//
//testtiming:keep pins the parent directory and lock file ReadJSON creates for a missing path, which the UpdateJSON tests never assert
func TestMissingFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "missing.json")
	lockPath := path + ".lock"

	got, found, err := state.ReadJSON[sample](path, lockPath)
	if err != nil {
		t.Fatalf("ReadJSON() on missing file error: %v", err)
	}
	if found {
		t.Fatal("ReadJSON() found = true; want false")
	}
	if got != (sample{}) {
		t.Errorf("ReadJSON() = %+v; want zero value", got)
	}

	// Verify parent dir and lock file exist.
	parentDir := filepath.Dir(path)
	if _, err := os.Stat(parentDir); err != nil {
		t.Errorf("parent directory does not exist: %v", err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file does not exist at %s: %v", lockPath, err)
	}
}

// TestCorruptFile writes invalid JSON and verifies ReadJSON returns a non-nil error, and that UpdateJSON aborts the same way without running mutate or touching the file.
//
//testtiming:keep named by internal/loomcli's smoke test as the lenient read's decode-failure pin
func TestCorruptFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "corrupt.json")
	lockPath := path + ".lock"

	// Write corrupt JSON.
	corrupt := []byte("{not json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("setup: WriteFile() error: %v", err)
	}

	_, _, err := state.ReadJSON[sample](path, lockPath)
	if err == nil {
		t.Fatal("ReadJSON() on corrupt file error = nil; want non-nil")
	}
	// The lenient read path now wraps ErrDecode too, matching ReadJSONNoMkdir, so a caller can tell a
	// decode failure apart from a read or lock failure with errors.Is (loomshed.Seed relies on this
	// to refuse rather than escalate a present-but-undecodable status file — crucible round
	// fable5-high-r5, F3).
	if !errors.Is(err, state.ErrDecode) {
		t.Errorf("ReadJSON() on corrupt file error = %v; want errors.Is(err, ErrDecode)", err)
	}

	// UpdateJSON shares the same lenient read, so it aborts before its mutate with the same wrapped
	// error and never overwrites the corrupt file.
	mutateCalled := false
	uerr := state.UpdateJSON(path, lockPath, func(cur sample, found bool) (sample, error) {
		mutateCalled = true
		return cur, nil
	})
	if !errors.Is(uerr, state.ErrDecode) {
		t.Errorf("UpdateJSON() on corrupt file error = %v; want errors.Is(err, ErrDecode)", uerr)
	}
	if mutateCalled {
		t.Error("UpdateJSON() called mutate on a corrupt file; want it to abort before mutate so the file is left untouched")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(after) != string(corrupt) {
		t.Errorf("UpdateJSON() modified a corrupt file:\nbefore:\n%s\nafter:\n%s", corrupt, after)
	}
}

// keepBase is embedded in keepRecord, so its Shared field is promoted to a top-level key.
type keepBase struct {
	Shared string `json:"shared,omitempty"`
}

// keepEntry is the element type of keepRecord's slice, map and pointer fields.
type keepEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// keepGuarded encodes and decodes itself, so a write treats it as opaque.
type keepGuarded struct {
	Value string
}

func (g keepGuarded) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Guarded string `json:"guarded"`
	}{g.Value})
}

func (g *keepGuarded) UnmarshalJSON(data []byte) error {
	var fields struct {
		Guarded string `json:"guarded"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	g.Value = fields.Guarded
	return nil
}

// keepRecord is the fixture record of TestWriteJSON_KeepsUnknownKeys: one field of each shape a write merges into or treats as opaque.
type keepRecord struct {
	keepBase
	Title   string               `json:"title"`
	Entries []keepEntry          `json:"entries,omitempty"`
	ByName  map[string]keepEntry `json:"by_name,omitempty"`
	Pointer *keepEntry           `json:"pointer,omitempty"`
	Free    any                  `json:"free,omitempty"`
	Guarded *keepGuarded         `json:"guarded,omitempty"`
	Raw     json.RawMessage      `json:"raw,omitempty"`
}

// TestWriteJSON_KeepsUnknownKeys seeds a file, writes a keepRecord over it through UpdateJSON or WriteJSON, and compares the written file to its exact expected text.
// A key the record's type does not declare survives with its exact digits after the known keys, wherever a declared struct holds it and the element it sits in is unchanged, and no key survives inside an opaque field or an element that was updated, removed or moved.
func TestWriteJSON_KeepsUnknownKeys(t *testing.T) {
	t.Parallel()

	updateWith := func(mutate func(record *keepRecord)) func(path, lockPath string) error {
		return func(path, lockPath string) error {
			return state.UpdateJSON(path, lockPath, func(current keepRecord, _ bool) (keepRecord, error) {
				mutate(&current)
				return current, nil
			})
		}
	}
	tests := []struct {
		name string
		// old is the compact file text the write replaces; want is the compact text it must become.
		old, want string
		apply     func(path, lockPath string) error
	}{
		{
			name: "an appended entry keeps the unknown keys of the file and of the entry left alone",
			old:  `{"title":"T","entries":[{"name":"a","count":1,"zz_entry":9007199254740993}],"zz_top":{"k":[1,2]}}`,
			want: `{"title":"T","entries":[{"name":"a","count":1,"zz_entry":9007199254740993},{"name":"b","count":2}],"zz_top":{"k":[1,2]}}`,
			apply: updateWith(func(record *keepRecord) {
				record.Entries = append(record.Entries, keepEntry{Name: "b", Count: 2})
			}),
		},
		{
			name: "a write of the value read back with a known field changed keeps both unknown keys",
			old:  `{"title":"T","entries":[{"name":"a","count":1,"zz_entry":1}],"zz_top":true}`,
			want: `{"title":"U","entries":[{"name":"a","count":1,"zz_entry":1}],"zz_top":true}`,
			apply: func(path, lockPath string) error {
				record, _, err := state.ReadJSON[keepRecord](path, lockPath)
				if err != nil {
					return err
				}
				record.Title = "U"
				return state.WriteJSON(path, lockPath, record)
			},
		},
		{
			name: "an entry updated in place drops its unknown key and its neighbour keeps its own",
			old:  `{"title":"T","entries":[{"name":"a","count":1,"zz":1},{"name":"b","count":2,"zz":2}]}`,
			want: `{"title":"T","entries":[{"name":"a","count":5},{"name":"b","count":2,"zz":2}]}`,
			apply: updateWith(func(record *keepRecord) {
				record.Entries[0].Count = 5
			}),
		},
		{
			name: "removing entry 0 drops the unknown key of the entry that moved to index 0",
			old:  `{"title":"T","entries":[{"name":"a","count":1,"zz":1},{"name":"b","count":2,"zz":2}]}`,
			want: `{"title":"T","entries":[{"name":"b","count":2}]}`,
			apply: updateWith(func(record *keepRecord) {
				record.Entries = record.Entries[1:]
			}),
		},
		{
			name: "a map value replaced under the same key keeps its unknown key and a new key gets none",
			old:  `{"title":"T","by_name":{"x":{"name":"x","count":1,"zz":1}}}`,
			want: `{"title":"T","by_name":{"x":{"name":"x2","count":3,"zz":1},"y":{"name":"y","count":0}}}`,
			apply: updateWith(func(record *keepRecord) {
				record.ByName["x"] = keepEntry{Name: "x2", Count: 3}
				record.ByName["y"] = keepEntry{Name: "y"}
			}),
		},
		{
			name: "a case-variant of a declared key is written once under the declared name",
			old:  `{"Title":"Old"}`,
			want: `{"title":"New"}`,
			apply: updateWith(func(record *keepRecord) {
				record.Title = "New"
			}),
		},
		{
			name: "a promoted field of the embedded struct is declared and written once",
			old:  `{"shared":"s1","title":"T","zz":1}`,
			want: `{"shared":"s2","title":"T","zz":1}`,
			apply: updateWith(func(record *keepRecord) {
				record.Shared = "s2"
			}),
		},
		{
			name: "extra keys inside the any, Marshaler and RawMessage fields are not carried",
			old:  `{"title":"T","free":{"a":1,"zz":2},"guarded":{"guarded":"g","zz":3},"raw":{"a":1,"zz":4}}`,
			want: `{"title":"T","free":{"a":2},"guarded":{"guarded":"h"},"raw":{"a":2}}`,
			apply: updateWith(func(record *keepRecord) {
				record.Free = map[string]any{"a": 2}
				record.Guarded = &keepGuarded{Value: "h"}
				record.Raw = json.RawMessage(`{"a":2}`)
			}),
		},
		{
			name: "the pointer-to-struct field keeps its unknown key",
			old:  `{"title":"T","pointer":{"name":"p","count":1,"zz":1}}`,
			want: `{"title":"T","pointer":{"name":"p","count":2,"zz":1}}`,
			apply: updateWith(func(record *keepRecord) {
				record.Pointer.Count = 2
			}),
		},
		{
			name: "a write over a malformed file writes the new value alone",
			old:  `{"title":`,
			want: `{"title":"T"}`,
			apply: func(path, lockPath string) error {
				return state.WriteJSON(path, lockPath, keepRecord{Title: "T"})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "record.json")
			lockPath := path + ".lock"
			if err := os.WriteFile(path, []byte(tt.old), 0o644); err != nil {
				t.Fatalf("seed record: %v", err)
			}

			if err := tt.apply(path, lockPath); err != nil {
				t.Fatalf("write: %v", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read record: %v", err)
			}
			var want bytes.Buffer
			if err := json.Indent(&want, []byte(tt.want), "", "  "); err != nil {
				t.Fatalf("indent want: %v", err)
			}
			if string(got) != want.String() {
				t.Errorf("written file:\n%s\nwant:\n%s", got, want.String())
			}
		})
	}
}
