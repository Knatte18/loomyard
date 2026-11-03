// front.go — the front task's two board fields: the payload and fixed-fields guards, and the one write that sets them.
//
// A front task is a task lym files ahead of its worktree's other starts; only FileFrontTask sets its front and on_land fields.

package boardengine

import "fmt"

// frontPayloadRefusal is the refusal of a payload that carries front or on_land with any value but the entry's stored one.
func frontPayloadRefusal(slug string) error {
	return fmt.Errorf("entry %q: only lym files a front task and writes its front and on_land fields; way forward: drop the front and on_land keys from the payload", slug)
}

// frontFixedRefusal is the refusal of a change to a front task's project, kind or slug.
func frontFixedRefusal(slug string) error {
	return fmt.Errorf("entry %q is a front task, and a front task's project, kind and slug are fixed; way forward: remove it with `%s`, preceded by `lym batten cancel %s --remove` while a run holds the entry", slug, RemoveCommand(slug), slug)
}

// checkFrontPayload judges the front and on_land keys of an upsert payload against the entry fields["slug"] names, the entry being new when the store holds none.
// A value equal to the stored one, false and empty for a new entry, is dropped from fields; any other value, or a front that is not a bool or an on_land that is not a string, is refused.
// On an entry whose stored Front is true, a project or kind value other than the stored one is refused too.
func checkFrontPayload(s *Store, fields map[string]any) error {
	slug, _ := fields["slug"].(string)
	stored, _ := s.GetTask(slug)
	if value, carried := fields["front"]; carried {
		if proposed, isBool := value.(bool); !isBool || proposed != stored.Front {
			return frontPayloadRefusal(slug)
		}
		delete(fields, "front")
	}
	if value, carried := fields["on_land"]; carried {
		if proposed, isString := value.(string); !isString || proposed != stored.OnLand {
			return frontPayloadRefusal(slug)
		}
		delete(fields, "on_land")
	}
	if !stored.Front {
		return nil
	}
	for key, storedValue := range map[string]string{"project": stored.Project, "kind": stored.Kind} {
		if value, carried := fields[key]; carried && value != storedValue {
			return frontFixedRefusal(slug)
		}
	}
	return nil
}

// FileFrontTask creates or updates the task fields describe under the write lock and stores it with Front true and OnLand set to onLand.
// The payload carries neither front nor on_land, which the payload refusal of an upsert would also name; the board's draft rule makes a new task draft, so a caller releases it with a later SetStatus(slug, nil).
// Only the front-task package calls it: no board edit can set either field.
func (b *Board) FileFrontTask(fields map[string]any, onLand string) (Task, error) {
	slug, _ := fields["slug"].(string)
	for _, key := range []string{"front", "on_land"} {
		if _, carried := fields[key]; carried {
			return Task{}, frontPayloadRefusal(slug)
		}
	}
	result, _, err := b.writeReportingSiblings(func(store *Store) (any, error) {
		if _, err := store.UpsertTask(fields); err != nil {
			return nil, err
		}
		i, _ := store.index(slug)
		store.tasks[i].Front = true
		store.tasks[i].OnLand = onLand
		return store.tasks[i], nil
	})
	if err != nil {
		return Task{}, err
	}
	return result.(Task), nil
}
