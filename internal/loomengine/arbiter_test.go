// arbiter_test.go — untagged Tier-1 tests for the arbiter's record readers and writers, over temp directories.

package loomengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestIsArbiterStrand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{"lyx:slug:arbiter", true},
		{"lyx:slug:arbiter-2", true},
		{"lyx:arbiter", true},
		{"lyx:slug:driver", false},
		{"lyx:slug:arbiter-x", false},
		{"arbiter", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsArbiterStrand(tt.name); got != tt.want {
				t.Errorf("IsArbiterStrand(%q) = %v; want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestStopKeyOf(t *testing.T) {
	t.Parallel()
	st := shedengine.Status{CurrentProducer: "Plan-Review", State: shedengine.StateBlocked, History: []shedengine.HistoryEntry{{}, {}}}
	want := ArbiterStopKey{CurrentProducer: "Plan-Review", HistoryLength: 2, State: "blocked"}
	if got := StopKeyOf(st); got != want {
		t.Errorf("StopKeyOf(...) = %+v; want %+v", got, want)
	}
}

// TestArbiterAttempts pins attempt write and read (the directory filled in and never serialized), listing in attempt order, an absent directory listing none, and a subdirectory with no attempt.json refused by name.
func TestArbiterAttempts(t *testing.T) {
	t.Parallel()
	attempt := func(n int) ArbiterAttempt {
		return ArbiterAttempt{
			Number:       n,
			StopKey:      ArbiterStopKey{CurrentProducer: "B", HistoryLength: n, State: "blocked"},
			PauseBefore:  "C",
			PauseAfter:   "A",
			SpawnedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			Deadline:     time.Date(2026, 1, 2, 3, 34, 5, 0, time.UTC),
			StrandGUID:   "guid",
			EnvelopePath: "/e",
			ReportPath:   "/r",
		}
	}

	t.Run("round trip fills Dir and serializes no Dir", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "attempt-1")
		if err := WriteArbiterAttempt(dir, attempt(1)); err != nil {
			t.Fatalf("WriteArbiterAttempt = %v; want nil", err)
		}
		got, err := ReadArbiterAttempt(dir)
		if err != nil {
			t.Fatalf("ReadArbiterAttempt = _, %v; want nil", err)
		}
		want := attempt(1)
		want.Dir = dir
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ReadArbiterAttempt = %+v; want %+v", got, want)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "attempt.json"))
		if err != nil || strings.Contains(string(raw), dir) {
			t.Errorf("attempt.json = %q, %v; want a file that does not carry the directory", raw, err)
		}
	})

	t.Run("list sorts by number, ignores files and lists none for an absent directory", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "arbiter")
		if got, err := ListArbiterAttempts(root); err != nil || got != nil {
			t.Errorf("ListArbiterAttempts(absent) = %v, %v; want nil, nil", got, err)
		}
		for _, n := range []int{10, 2, 1} {
			if err := WriteArbiterAttempt(filepath.Join(root, "attempt-"+strconv.Itoa(n)), attempt(n)); err != nil {
				t.Fatalf("WriteArbiterAttempt(%d) = %v", n, err)
			}
		}
		if err := os.WriteFile(filepath.Join(root, "stray.txt"), nil, 0o644); err != nil {
			t.Fatalf("write stray file: %v", err)
		}
		got, err := ListArbiterAttempts(root)
		if err != nil {
			t.Fatalf("ListArbiterAttempts = _, %v; want nil", err)
		}
		var numbers []int
		for _, a := range got {
			numbers = append(numbers, a.Number)
		}
		if !reflect.DeepEqual(numbers, []int{1, 2, 10}) {
			t.Errorf("ListArbiterAttempts numbers = %v; want [1 2 10]", numbers)
		}
	})

	t.Run("a subdirectory without attempt.json is refused by name", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "arbiter")
		if err := os.MkdirAll(filepath.Join(root, "attempt-1"), 0o755); err != nil {
			t.Fatalf("create attempt directory: %v", err)
		}
		if _, err := ListArbiterAttempts(root); err == nil || !strings.Contains(err.Error(), "attempt-1") {
			t.Errorf("ListArbiterAttempts = _, %v; want an error naming attempt-1", err)
		}
	})

	t.Run("ParseArbiterAttemptDir reads tolerantly and names the failing file", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			files        map[string][]byte
			wantNumber   int
			wantVerdict  string
			wantNoVerdct bool
			wantApplied  bool
			wantErr      string
		}{
			{
				name: "resume verdict with an unknown key and the applied mark",
				files: map[string][]byte{
					"attempt.json": []byte(`{"number":2}`),
					"verdict.yaml": []byte("verdict: resume\nreason: r\nbogus: x\n"),
					"applied":      nil,
					"prompt.md":    []byte("ignored"),
				},
				wantNumber:  2,
				wantVerdict: ArbiterVerdictResume,
				wantApplied: true,
			},
			{
				name:         "no verdict",
				files:        map[string][]byte{"attempt.json": []byte(`{"number":3}`)},
				wantNumber:   3,
				wantNoVerdct: true,
			},
			{
				name:         "attempt.json with an unknown key",
				files:        map[string][]byte{"attempt.json": []byte(`{"number":4,"bogus":true}`)},
				wantNumber:   4,
				wantNoVerdct: true,
			},
			{name: "missing attempt.json", files: map[string][]byte{}, wantErr: "attempt.json"},
			{name: "undecodable attempt.json", files: map[string][]byte{"attempt.json": []byte("{")}, wantErr: "attempt.json"},
			{
				name:    "undecodable verdict.yaml",
				files:   map[string][]byte{"attempt.json": []byte(`{"number":1}`), "verdict.yaml": []byte("verdict: [")},
				wantErr: "verdict.yaml",
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				got, err := ParseArbiterAttemptDir(tc.files)
				if tc.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("ParseArbiterAttemptDir = _, %v; want an error naming %s", err, tc.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatalf("ParseArbiterAttemptDir = _, %v; want nil", err)
				}
				if got.Attempt.Number != tc.wantNumber || got.Applied != tc.wantApplied {
					t.Errorf("ParseArbiterAttemptDir = %+v; want number %d, applied %v", got, tc.wantNumber, tc.wantApplied)
				}
				if tc.wantNoVerdct != (got.Verdict == nil) || (got.Verdict != nil && got.Verdict.Verdict != tc.wantVerdict) {
					t.Errorf("ParseArbiterAttemptDir verdict = %+v; want %q (nil %v)", got.Verdict, tc.wantVerdict, tc.wantNoVerdct)
				}
			})
		}
	})
}

// TestReadArbiterVerdict pins both valid shapes, an absent file, and each refusal.
func TestReadArbiterVerdict(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    ArbiterVerdict
		wantErr string
	}{
		{name: "resume", content: "verdict: resume\nreason: the stop is a flake\n", want: ArbiterVerdict{Verdict: "resume", Reason: "the stop is a flake"}},
		{
			name:    "resume with a goto and verbs",
			content: "verdict: resume\nreason: bounce back\nverbs:\n  - lyx loom decision add\ngoto: Plan-Write\n",
			want:    ArbiterVerdict{Verdict: "resume", Reason: "bounce back", Verbs: []string{"lyx loom decision add"}, Goto: "Plan-Write"},
		},
		{name: "hand-on", content: "verdict: hand-on\nreason: a design call\nquestion: which scope?\n", want: ArbiterVerdict{Verdict: "hand-on", Reason: "a design call", Question: "which scope?"}},
		{name: "unparseable", content: "verdict: [", wantErr: "parse arbiter verdict"},
		{name: "unknown key", content: "verdict: resume\nreason: r\nbogus: 1\n", wantErr: "parse arbiter verdict"},
		{name: "unknown verdict word", content: "verdict: shrug\nreason: r\n", wantErr: `verdict "shrug"`},
		{name: "empty reason", content: "verdict: resume\nreason: \"\"\n", wantErr: "reason is empty"},
		{name: "hand-on without a question", content: "verdict: hand-on\nreason: r\n", wantErr: "needs a question"},
		{name: "goto on a hand-on", content: "verdict: hand-on\nreason: r\nquestion: q\ngoto: B\n", wantErr: "carries no goto"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "verdict.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write verdict: %v", err)
			}
			got, found, err := ReadArbiterVerdict(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || found {
					t.Errorf("ReadArbiterVerdict = _, %v, %v; want a refusal containing %q", found, err, tt.wantErr)
				}
				return
			}
			if err != nil || !found || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadArbiterVerdict = %+v, %v, %v; want %+v, true, nil", got, found, err, tt.want)
			}
		})
	}

	t.Run("absent file", func(t *testing.T) {
		t.Parallel()
		if _, found, err := ReadArbiterVerdict(filepath.Join(t.TempDir(), "verdict.yaml")); found || err != nil {
			t.Errorf("ReadArbiterVerdict(absent) = _, %v, %v; want false, nil", found, err)
		}
	})
}

// TestArbiterJSONRecords pins the envelope and marker round trip, absence as found false, and an unparseable file as an error.
func TestArbiterJSONRecords(t *testing.T) {
	t.Parallel()
	envelope := ArbiterEnvelope{Verdict: "resume", Reason: "r", Goto: "B", Verbs: []string{"v"}, Attempt: 2, AttemptDir: "/d", VerdictPath: "/d/verdict.yaml", Relaunch: "lyx loom ..."}
	marker := ArbiterMarker{Attempt: 2, StrandGUID: "guid", Deadline: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	tests := []struct {
		name  string
		write func(path string) error
		read  func(path string) (any, bool, error)
		want  any
	}{
		{
			name:  "envelope",
			write: func(path string) error { return WriteArbiterEnvelope(path, envelope) },
			read:  func(path string) (any, bool, error) { return ReadArbiterEnvelope(path) },
			want:  envelope,
		},
		{
			name:  "marker",
			write: func(path string) error { return WriteArbiterMarker(path, marker) },
			read:  func(path string) (any, bool, error) { return ReadArbiterMarker(path) },
			want:  marker,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "nested", "record.json")
			if _, found, err := tt.read(path); found || err != nil {
				t.Errorf("read(absent) = _, %v, %v; want false, nil", found, err)
			}
			if err := tt.write(path); err != nil {
				t.Fatalf("write = %v; want nil", err)
			}
			if got, found, err := tt.read(path); err != nil || !found || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("read = %+v, %v, %v; want %+v, true, nil", got, found, err, tt.want)
			}
			if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
				t.Fatalf("corrupt record: %v", err)
			}
			if _, _, err := tt.read(path); err == nil {
				t.Error("read(unparseable) = nil error; want an error")
			}
		})
	}
}

// TestArbiterAppliedAndMarkerRemoval pins the applied mark's absence and presence, and that removing an absent marker is not an error.
func TestArbiterAppliedAndMarkerRemoval(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "attempt-1")
	if applied, err := ArbiterApplied(dir); applied || err != nil {
		t.Errorf("ArbiterApplied(absent) = %v, %v; want false, nil", applied, err)
	}
	if err := MarkArbiterApplied(dir); err != nil {
		t.Fatalf("MarkArbiterApplied = %v; want nil", err)
	}
	if applied, err := ArbiterApplied(dir); !applied || err != nil {
		t.Errorf("ArbiterApplied = %v, %v; want true, nil", applied, err)
	}

	path := filepath.Join(t.TempDir(), "arbiter-inflight.json")
	if err := RemoveArbiterMarker(path); err != nil {
		t.Errorf("RemoveArbiterMarker(absent) = %v; want nil", err)
	}
	if err := WriteArbiterMarker(path, ArbiterMarker{Attempt: 1}); err != nil {
		t.Fatalf("WriteArbiterMarker = %v; want nil", err)
	}
	if err := RemoveArbiterMarker(path); err != nil {
		t.Fatalf("RemoveArbiterMarker = %v; want nil", err)
	}
	if _, found, err := ReadArbiterMarker(path); found || err != nil {
		t.Errorf("ReadArbiterMarker after removal = _, %v, %v; want false, nil", found, err)
	}
}

// TestArbiterPaths pins the four file names inside an attempt directory.
func TestArbiterPaths(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("a", "attempt-1")
	for got, want := range map[string]string{
		ArbiterPromptPath(dir):   "prompt.md",
		ArbiterVerdictPath(dir):  "verdict.yaml",
		ArbiterEnvelopePath(dir): "envelope.json",
		ArbiterAppliedPath(dir):  "applied",
	} {
		if got != filepath.Join(dir, want) {
			t.Errorf("path = %q; want %q under the attempt directory", got, want)
		}
	}
}
