// labels_test.go — unit tests for label validation (labels.go).

package boardengine

import (
	"strings"
	"testing"
)

func TestValidateLabels(t *testing.T) {
	vocab := Vocabulary{Types: []string{"bug", "enhancement"}, Labels: []string{"undecided", "area"}}

	tests := []struct {
		name    string
		task    Task
		wantErr []string // substrings the message must contain; nil means valid
	}{
		{
			name: "valid note",
			task: Task{Slug: "n", Kind: KindNote, Labels: []string{"bug", "undecided"}},
		},
		{
			name: "valid task with two type labels",
			task: Task{Slug: "t", Kind: KindTask, Labels: []string{"enhancement", "bug", "area"}},
		},
		{
			name:    "label in neither list",
			task:    Task{Slug: "n", Kind: KindNote, Labels: []string{"bug", "mystery"}},
			wantErr: []string{`"n"`, `"mystery"`, "board.yaml"},
		},
		{
			name:    "duplicate label",
			task:    Task{Slug: "n", Kind: KindNote, Labels: []string{"bug", "area", "area"}},
			wantErr: []string{`"n"`, `"area"`, "once"},
		},
		{
			name:    "note with no type label",
			task:    Task{Slug: "n", Kind: KindNote, Labels: []string{"undecided"}},
			wantErr: []string{`"n"`, "exactly one type label", "board.yaml", "none"},
		},
		{
			name:    "note with two type labels names them",
			task:    Task{Slug: "n", Kind: KindNote, Labels: []string{"bug", "enhancement"}},
			wantErr: []string{`"n"`, "exactly one type label", "board.yaml", "bug, enhancement"},
		},
		{
			name:    "task with no type label",
			task:    Task{Slug: "t", Kind: KindTask, Labels: []string{"area"}},
			wantErr: []string{`"t"`, "type label", "board.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLabels(tt.task, vocab)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected a refusal containing %v, got nil", tt.wantErr)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

// newVocabStore builds a store whose writes are label-checked, which only store-level tests do.
func newVocabStore(v Vocabulary) *Store {
	s := NewStore("")
	s.vocab = &v
	return s
}

func TestStoreValidatesLabelsWithVocabulary(t *testing.T) {
	vocab := Vocabulary{Types: []string{"bug"}, Labels: []string{"area"}}

	t.Run("upsert refused on an unconfigured label", func(t *testing.T) {
		s := newVocabStore(vocab)
		_, err := s.UpsertTask(map[string]any{"slug": "x", "labels": []string{"bug", "mystery"}})
		if err == nil || !strings.Contains(err.Error(), `"mystery"`) || !strings.Contains(err.Error(), "board.yaml") {
			t.Errorf("expected refusal naming mystery and board.yaml, got %v", err)
		}
		if len(s.Tasks()) != 0 {
			t.Errorf("a refused upsert changed the store: %+v", s.Tasks())
		}
	})

	t.Run("batch refused as a whole", func(t *testing.T) {
		s := newVocabStore(vocab)
		err := s.UpsertTasksBatch([]map[string]any{
			{"slug": "ok", "labels": []string{"bug"}},
			{"slug": "bad", "labels": []string{"area"}},
		})
		if err == nil || !strings.Contains(err.Error(), `"bad"`) {
			t.Errorf("expected refusal naming bad, got %v", err)
		}
		if len(s.Tasks()) != 0 {
			t.Errorf("a refused batch changed the store: %+v", s.Tasks())
		}
	})

	t.Run("status write refused while the entry carries an unconfigured label", func(t *testing.T) {
		s := newVocabStore(vocab)
		if _, err := s.UpsertTask(map[string]any{"slug": "x", "labels": []string{"bug"}}); err != nil {
			t.Fatal(err)
		}
		s.vocab = &Vocabulary{Types: []string{"enhancement"}}
		done := "done"
		if err := s.SetStatus("x", &done); err == nil || !strings.Contains(err.Error(), `"bug"`) {
			t.Errorf("expected refusal naming bug, got %v", err)
		}
		if got, _ := s.GetTask("x"); got.Status != nil {
			t.Errorf("a refused status write changed the entry: %v", *got.Status)
		}
	})

	t.Run("promote keeps a valid entry valid", func(t *testing.T) {
		s := newVocabStore(vocab)
		if _, err := s.UpsertTask(map[string]any{"slug": "x", "labels": []string{"bug"}}); err != nil {
			t.Fatal(err)
		}
		got, changed, err := s.Promote("x")
		if err != nil || !changed || got.Kind != KindTask {
			t.Errorf("got %+v changed %v err %v; want x promoted", got, changed, err)
		}
	})
}
