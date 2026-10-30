//go:build tier0

// runlock_test.go — the run-status form and the lock on a run-held entry's scope (runlock.go), driven through the Board facade.

package boardengine_test

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

func strPtr(s string) *string { return &s }

// TestRunLock pins the run-status form and which writes a run-held entry refuses.
// A refused row leaves board.json byte-identical.
func TestRunLock(t *testing.T) {
	t.Parallel()

	// lockedBase is the held entry; rows override fields of it.
	lockedBase := map[string]any{
		"slug": "locked", "title": "Held", "kind": "task", "labels": []string{"bug"},
		"brief": "brief", "body": "body", "status": "running · Webster",
	}
	other := map[string]any{"slug": "other", "title": "Other", "kind": "task", "labels": []string{"bug"}}
	done := map[string]any{"slug": "fin", "title": "Fin", "kind": "task", "labels": []string{"bug"}, "status": "done"}
	grouping := map[string]any{"slug": "grp", "title": "Group", "kind": "project", "labels": []string{"bug"}}
	const awaiting = "running · Beam-Await"
	// A board write cannot make a beam, so a beam row splices the beam key in after seeding the project; heldBeam is the project's fields.
	const beamKey = `"beam":true`
	heldBeam := func(status string) map[string]any {
		return map[string]any{"kind": "project", "recipe": boardengine.ProjectRecipe, "status": status}
	}

	rows := []struct {
		name    string
		locked  map[string]any
		extra   []map[string]any
		act     func(b *boardengine.Board) error
		refused bool
		// unknown is raw JSON members spliced into the locked entry after seeding; an accepted row requires the written file to still carry each.
		unknown string
		// primeOwner is a prime_owner record spliced into board.json after seeding.
		primeOwner string
		// unchanged requires an accepted row to leave board.json byte-identical.
		unchanged bool
		// beamRefusal requires a refused row to be the *BeamRunLockedError of a beam project whose run holds nothing.
		beamRefusal bool
	}{
		{name: "upsert body", refused: true, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "upsert brief", refused: true, act: upsertLocked(map[string]any{"brief": "new"})},
		{name: "upsert labels", refused: true, act: upsertLocked(map[string]any{"labels": []string{"enhancement"}})},
		{name: "upsert issues", refused: true, act: upsertLocked(map[string]any{"issues": []int{7}})},
		{name: "upsert title", refused: true, act: upsertLocked(map[string]any{"title": "new"})},
		{name: "upsert kind", refused: true, act: upsertLocked(map[string]any{"kind": "note"})},
		{name: "upsert recipe", refused: true, act: upsertLocked(map[string]any{"recipe": "other-recipe"})},
		{name: "upsert priority on a held note", refused: true, locked: map[string]any{"kind": "note"}, act: upsertLocked(map[string]any{"priority": "high"})},
		{name: "upsert pull_request", refused: true, act: upsertLocked(map[string]any{"pull_request": "on"})},
		{name: "upsert depends_on", refused: true, extra: []map[string]any{other}, act: upsertLocked(map[string]any{"depends_on": []string{"other"}})},
		{name: "upsert status and body", refused: true, act: upsertLocked(map[string]any{"status": "done", "body": "new"})},
		{name: "status blocked", refused: true, locked: map[string]any{"status": "blocked · X"}, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "status paused", refused: true, locked: map[string]any{"status": "paused · X"}, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "status failed", refused: true, locked: map[string]any{"status": "failed · X"}, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "status awaiting", refused: true, locked: map[string]any{"status": "awaiting · X"}, act: upsertLocked(map[string]any{"body": "new"})},
		{name: "set deps", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.SetDeps("locked", []string{"other"})
			return err
		}},
		{name: "promote a locked note", refused: true, locked: map[string]any{"kind": "note"}, act: func(b *boardengine.Board) error {
			_, err := b.Promote("locked")
			return err
		}},
		{name: "remove", refused: true, act: func(b *boardengine.Board) error { return b.RemoveTask("locked") }},
		{name: "merge removing it", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, _, err := b.MergeTasks([]string{"locked"}, map[string]any{"slug": "other", "body": "merged"}, nil)
			return err
		}},
		{name: "merge upserting a change into it", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, _, err := b.MergeTasks([]string{"other"}, map[string]any{"slug": "locked", "body": "merged"}, nil)
			return err
		}},
		{name: "merge changing it and clearing its status", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, _, err := b.MergeTasks([]string{"other"}, map[string]any{"slug": "locked", "body": "merged"}, &boardengine.MergeStatusUpdate{Selector: "locked"})
			return err
		}},
		{name: "batch with one changing item", refused: true, extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.UpsertTasksBatch([]map[string]any{{"slug": "other", "body": "batched"}, {"slug": "locked", "body": "new"}})
			return err
		}},
		{name: "batch clearing the status then changing the body", refused: true, act: func(b *boardengine.Board) error {
			_, err := b.UpsertTasksBatch([]map[string]any{{"slug": "locked", "status": ""}, {"slug": "locked", "body": "new"}})
			return err
		}},

		{name: "set-status to another value", unknown: `"zz_scalar":9007199254740993`, act: func(b *boardengine.Board) error { return b.SetStatus("locked", strPtr("paused · X")) }},
		{name: "set-status to nil", act: func(b *boardengine.Board) error { return b.SetStatus("locked", nil) }},
		{name: "merge whose set_status targets it", extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, _, err := b.MergeTasks(nil, map[string]any{"slug": "other", "body": "merged"}, &boardengine.MergeStatusUpdate{Selector: "locked", Status: strPtr("done")})
			return err
		}},
		{name: "identical upsert", act: upsertLocked(map[string]any{"body": "body", "labels": []string{"bug"}, "issues": []int{}, "depends_on": []string{}})},
		{name: "status-only upsert", act: upsertLocked(map[string]any{"status": "done"})},
		{name: "run settings change", act: upsertLocked(map[string]any{"run_settings": map[string]any{"loom": map[string]any{"review_max_bounces": 2}}})},
		{name: "profile change", act: upsertLocked(map[string]any{"profile": "light"})},
		{name: "upsert naming the held entry's own effective owner", primeOwner: "box", unchanged: true, act: upsertLocked(map[string]any{"owner_machine": "box"})},
		{name: "normal priority on an entry without one", act: upsertLocked(map[string]any{"priority": "normal"})},
		{name: "batch whose only item on it is status-only", extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.UpsertTasksBatch([]map[string]any{{"slug": "other", "body": "batched"}, {"slug": "locked", "status": "done"}})
			return err
		}},
		{name: "prune dropping a done dependency", locked: map[string]any{"depends_on": []string{"fin"}}, extra: []map[string]any{done}, act: func(b *boardengine.Board) error {
			removed, err := b.Prune(true)
			if err == nil && (len(removed) != 1 || removed[0] != "fin") {
				t.Errorf("prune removed %v, want [fin]", removed)
			}
			return err
		}},
		{name: "write to another entry", extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": "other", "body": "new"})
			return err
		}},
		{name: "add a dependency on the locked entry", extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, err := b.SetDeps("other", []string{"locked"})
			return err
		}},
		{name: "body change on an entry whose status is not a run status", locked: map[string]any{"status": "active"}, act: upsertLocked(map[string]any{"body": "new"})},

		{name: "beam at Parent-Align refuses a title edit as a held entry does", refused: true, unknown: beamKey, locked: heldBeam("running · Parent-Align"), act: upsertLocked(map[string]any{"title": "new"})},
		{name: "beam at Beam-Await takes a title edit", unknown: beamKey, locked: heldBeam(awaiting), act: upsertLocked(map[string]any{"title": "new"})},
		{name: "beam at Beam-Await takes a brief edit", unknown: beamKey, locked: heldBeam(awaiting), act: upsertLocked(map[string]any{"brief": "new"})},
		{name: "beam at Beam-Await takes a body edit", unknown: beamKey, locked: heldBeam(awaiting), act: upsertLocked(map[string]any{"body": "new"})},
		{name: "beam at Beam-Await takes a labels edit", unknown: beamKey, locked: heldBeam(awaiting), act: upsertLocked(map[string]any{"labels": []string{"enhancement"}})},
		{name: "beam blocked at Beam-Await takes an edit", unknown: beamKey, locked: heldBeam("blocked · Beam-Await"), act: upsertLocked(map[string]any{"title": "new"})},
		{name: "beam at Beam-Await takes a member and its order", unknown: beamKey, locked: heldBeam(awaiting), act: func(b *boardengine.Board) error {
			member := map[string]any{"slug": "m1", "title": "Member", "kind": "task", "labels": []string{"bug"}, "project": "locked"}
			if _, err := b.UpsertTask(member); err != nil {
				return err
			}
			_, err := b.UpsertTask(map[string]any{"slug": "locked", "order": []string{"m1"}})
			return err
		}},
		{name: "beam at Beam-Await refuses a project change", refused: true, beamRefusal: true, unknown: beamKey, locked: heldBeam(awaiting), extra: []map[string]any{grouping}, act: upsertLocked(map[string]any{"project": "grp"})},
		{name: "beam at Beam-Await refuses a recipe change", refused: true, beamRefusal: true, unknown: beamKey, locked: heldBeam(awaiting), act: upsertLocked(map[string]any{"recipe": ""})},
		{name: "beam at Beam-Await refuses a removal", refused: true, beamRefusal: true, unknown: beamKey, locked: heldBeam(awaiting), act: func(b *boardengine.Board) error { return b.RemoveTask("locked") }},
		{name: "beam at Beam-Await refuses a merge", refused: true, beamRefusal: true, unknown: beamKey, locked: heldBeam(awaiting), extra: []map[string]any{other}, act: func(b *boardengine.Board) error {
			_, _, err := b.MergeTasks([]string{"locked"}, map[string]any{"slug": "other", "body": "merged"}, nil)
			return err
		}},
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			boardPath := t.TempDir()
			b := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})

			fields := maps.Clone(lockedBase)
			maps.Copy(fields, tc.locked)
			// Seeded dependencies go first, as an entry may only depend on one already stored.
			for _, seed := range append(append([]map[string]any{}, tc.extra...), fields) {
				if _, err := b.UpsertTask(seed); err != nil {
					t.Fatalf("seed %v: %v", seed["slug"], err)
				}
			}
			if tc.unknown != "" {
				addEntryKeys(t, boardPath, "locked", tc.unknown)
			}
			if tc.primeOwner != "" {
				setPrimeOwner(t, boardPath, tc.primeOwner)
			}
			if tc.unchanged {
				// The splices are re-encoded by a write, so the baseline is the file as the store itself writes it.
				if err := b.Rerender(); err != nil {
					t.Fatalf("rerender: %v", err)
				}
			}
			before, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
			if err != nil {
				t.Fatalf("read board.json: %v", err)
			}

			err = tc.act(b)

			after, readErr := os.ReadFile(filepath.Join(boardPath, "board.json"))
			if readErr != nil {
				t.Fatalf("read board.json: %v", readErr)
			}
			if !tc.refused {
				if err != nil {
					t.Fatalf("write refused: %v", err)
				}
				if tc.unchanged && string(after) != string(before) {
					t.Errorf("board.json changed on a write that names the entry's own owner:\nbefore %s\nafter  %s", before, after)
				}
				if tc.unknown != "" {
					var members map[string]json.RawMessage
					if err := json.Unmarshal([]byte("{"+tc.unknown+"}"), &members); err != nil {
						t.Fatalf("decode unknown members: %v", err)
					}
					for key, value := range members {
						if got, present := keptValue(t, after, "locked", key); !present || got != string(value) {
							t.Errorf("locked key %s = %q (present %v); want %q", key, got, present, value)
						}
					}
				}
				return
			}
			if !errors.Is(err, boardengine.ErrRunLocked) {
				t.Fatalf("err = %v, want ErrRunLocked", err)
			}
			status, _ := fields["status"].(string)
			if tc.beamRefusal {
				var beamErr *boardengine.BeamRunLockedError
				if !errors.As(err, &beamErr) {
					t.Fatalf("err = %v, want a *BeamRunLockedError", err)
				}
				for _, want := range []string{`"locked"`, status, "lym batten cancel locked"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("message %q lacks %q", err.Error(), want)
					}
				}
				if string(after) != string(before) {
					t.Error("board.json changed on a refused write")
				}
				return
			}
			for _, want := range []string{`"locked"`, status, "lym batten status locked", "prime worktree", `"kind":"note"`, "lym batten cancel locked"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("message %q lacks %q", err.Error(), want)
				}
			}
			if strings.Contains(err.Error(), "lym board set-status") {
				t.Errorf("message %q names lym board set-status; want the cancel as the way to abandon a run", err.Error())
			}
			if string(after) != string(before) {
				t.Error("board.json changed on a refused write")
			}
		})
	}

	t.Run("status write on a run-held task stored with a priority", func(t *testing.T) {
		t.Parallel()
		boardPath := t.TempDir()
		b := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
		if _, err := b.UpsertTask(lockedBase); err != nil {
			t.Fatalf("seed: %v", err)
		}
		addEntryKeys(t, boardPath, "locked", `"priority":"high"`)

		if err := b.SetStatus("locked", strPtr("paused · X")); err != nil {
			t.Fatalf("SetStatus on a held task stored with a priority: %v", err)
		}
	})

	t.Run("order of a run-held project", func(t *testing.T) {
		t.Parallel()
		boardPath := t.TempDir()
		b := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
		for _, seed := range []map[string]any{
			{"slug": "locked", "title": "Held", "kind": "project", "labels": []string{"bug"}},
			{"slug": "fin", "title": "Fin", "kind": "task", "labels": []string{"bug"}, "project": "locked", "status": "done"},
			{"slug": "open", "title": "Open", "kind": "task", "labels": []string{"bug"}, "project": "locked"},
			{"slug": "locked", "order": []string{"fin", "open"}},
		} {
			if _, err := b.UpsertTask(seed); err != nil {
				t.Fatalf("seed %v: %v", seed["slug"], err)
			}
		}
		if err := b.SetRunStatus("locked", boardengine.RunStatus("running", "Webster")); err != nil {
			t.Fatalf("SetRunStatus: %v", err)
		}

		if _, err := b.UpsertTask(map[string]any{"slug": "locked", "order": []string{"open", "fin"}}); !errors.Is(err, boardengine.ErrRunLocked) {
			t.Errorf("hand change of the order of a held project error = %v; want ErrRunLocked", err)
		}
		removed, err := b.Prune(true)
		if err != nil || len(removed) != 1 || removed[0] != "fin" {
			t.Fatalf("Prune(true) = %v, %v; want [fin] removed and no refusal, since the order rewrite only drops a done slug", removed, err)
		}
		held, _, err := b.GetTask("locked")
		if err != nil || !slices.Equal(held.Order, []string{"open"}) {
			t.Errorf("held project after prune = %+v, %v; want order [open]", held, err)
		}
	})

	t.Run("run status form", func(t *testing.T) {
		t.Parallel()
		if got := boardengine.RunStatus("running", "Webster"); got != "running · Webster" {
			t.Errorf("RunStatus = %q", got)
		}
		cases := []struct {
			status *string
			want   bool
		}{
			{nil, false},
			{strPtr(""), false},
			{strPtr("done"), false},
			{strPtr("abandoned"), false},
			{strPtr("active"), false},
			{strPtr("running · Webster"), true},
			{strPtr("blocked · X"), true},
			{strPtr("paused · X"), true},
			{strPtr("failed · X"), true},
			{strPtr("awaiting · X"), true},
		}
		for _, tc := range cases {
			if got := boardengine.IsRunStatus(tc.status); got != tc.want {
				t.Errorf("IsRunStatus(%v) = %v, want %v", tc.status, got, tc.want)
			}
		}
		if producer, ok := boardengine.RunStatusProducer(strPtr("running · Seed-Child")); producer != "Seed-Child" || !ok {
			t.Errorf("RunStatusProducer = %q, %v; want %q, true", producer, ok, "Seed-Child")
		}
	})
}

// TestRunLock_BeamKindAndBeamChange pins the refusal of a kind or beam change on a beam project whose run holds nothing, which no board write reaches since the write's own check refuses it first.
func TestRunLock_BeamKindAndBeamChange(t *testing.T) {
	t.Parallel()

	status := "running · Beam-Await"
	before := boardengine.Task{Slug: "locked", Kind: boardengine.KindProject, Beam: true, Recipe: boardengine.ProjectRecipe, Status: &status}
	tests := []struct {
		name  string
		after boardengine.Task
	}{
		{"KindChange", boardengine.Task{Slug: "locked", Kind: boardengine.KindNote, Status: &status}},
		{"BeamChange", boardengine.Task{Slug: "locked", Kind: boardengine.KindProject, Recipe: boardengine.ProjectRecipe, Status: &status}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := boardengine.CheckRunLocks([]boardengine.Task{before}, []boardengine.Task{tt.after})
			var beamErr *boardengine.BeamRunLockedError
			if !errors.As(err, &beamErr) || !errors.Is(err, boardengine.ErrRunLocked) || !strings.Contains(err.Error(), "lym batten cancel locked") {
				t.Errorf("CheckRunLocks error = %v; want a *BeamRunLockedError matching ErrRunLocked and naming lym batten cancel locked", err)
			}
		})
	}
}

// TestRunHolds pins which entries a run status holds: all of them, bar a beam project at a hold-free producer.
func TestRunHolds(t *testing.T) {
	t.Parallel()

	task := func(kind string, beam bool, status *string) boardengine.Task {
		return boardengine.Task{Slug: "e", Kind: kind, Beam: beam, Status: status}
	}
	tests := []struct {
		name string
		in   boardengine.Task
		want bool
	}{
		{"TaskRunning", task(boardengine.KindTask, false, strPtr("running · Webster")), true},
		{"TaskAtAHoldFreeProducerName", task(boardengine.KindTask, false, strPtr("running · Beam-Await")), true},
		{"GroupingProjectRunning", task(boardengine.KindProject, false, strPtr("running · Beam-Await")), true},
		{"BeamAtWorktreeCreate", task(boardengine.KindProject, true, strPtr("running · Worktree-Create")), false},
		{"BeamAtPreflight", task(boardengine.KindProject, true, strPtr("running · Preflight")), false},
		{"BeamAtBeamUp", task(boardengine.KindProject, true, strPtr("running · Beam-Up")), false},
		{"BeamAtBeamAwaitRunning", task(boardengine.KindProject, true, strPtr("running · Beam-Await")), false},
		{"BeamAtBeamAwaitBlocked", task(boardengine.KindProject, true, strPtr("blocked · Beam-Await")), false},
		{"BeamAtParentAlign", task(boardengine.KindProject, true, strPtr("running · Parent-Align")), true},
		{"BeamAtFinalize", task(boardengine.KindProject, true, strPtr("running · Finalize")), true},
		{"BeamWithNoStatus", task(boardengine.KindProject, true, nil), false},
		{"BeamWithADoneStatus", task(boardengine.KindProject, true, strPtr("done")), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := boardengine.RunHolds(tt.in); got != tt.want {
				t.Errorf("RunHolds(%+v) = %v; want %v", tt.in, got, tt.want)
			}
		})
	}
}

// addEntryKeys splices the raw JSON members into the entry slug of the board.json under boardPath, leaving every value byte-for-byte as decoded raw.
func addEntryKeys(t *testing.T, boardPath, slug, members string) {
	t.Helper()
	path := filepath.Join(boardPath, "board.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("decode board.json: %v", err)
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(top["entries"], &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	var added map[string]json.RawMessage
	if err := json.Unmarshal([]byte("{"+members+"}"), &added); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	found := false
	for _, entry := range entries {
		if string(entry["slug"]) == strconv.Quote(slug) {
			maps.Copy(entry, added)
			found = true
		}
	}
	if !found {
		t.Fatalf("no entry %q in board.json", slug)
	}
	if top["entries"], err = json.Marshal(entries); err != nil {
		t.Fatalf("encode entries: %v", err)
	}
	if raw, err = json.Marshal(top); err != nil {
		t.Fatalf("encode board.json: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
}

// setPrimeOwner splices the prime_owner record into the board.json under boardPath.
func setPrimeOwner(t *testing.T, boardPath, owner string) {
	t.Helper()
	path := filepath.Join(boardPath, "board.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("decode board.json: %v", err)
	}
	if top["prime_owner"], err = json.Marshal(owner); err != nil {
		t.Fatalf("encode prime_owner: %v", err)
	}
	if raw, err = json.Marshal(top); err != nil {
		t.Fatalf("encode board.json: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
}

// upsertLocked returns an action upserting patch onto the entry "locked".
func upsertLocked(patch map[string]any) func(b *boardengine.Board) error {
	return func(b *boardengine.Board) error {
		fields := map[string]any{"slug": "locked"}
		maps.Copy(fields, patch)
		_, err := b.UpsertTask(fields)
		return err
	}
}
