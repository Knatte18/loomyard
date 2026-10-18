// entries_settings.go implements what the review entries share to read a row's segment settings and the config settings a row's Config names through Env.Setting.

package shedrecipe

import (
	"fmt"
	"strings"
)

// settingReference is a row's reference to one config value, parsed from a `module.key` string.
type settingReference struct {
	// Module is the config module the value lives in.
	Module string
	// Key is the template key within Module.
	Key string
	// Text is the reference as the recipe wrote it.
	Text string
}

// String renders the reference as the citation a refusal gives for where a value came from: `<key> in <module>.yaml`.
func (r settingReference) String() string {
	return r.Key + " in " + r.Module + ".yaml"
}

// rowSegment returns the SegmentSettings of the segment row sits in.
// It refuses a row with no segment, and a segment with no env.Segments entry, naming the entry, the row and the segment.
func rowSegment(entry string, row RowIdentity, env Env) (SegmentSettings, error) {
	if row.Segment == "" {
		return SegmentSettings{}, fmt.Errorf("shedrecipe: %s: row %q has no segment; way forward: put the row in a review segment with segment: in the recipe", entry, row.Name)
	}
	settings, ok := env.Segments[row.Segment]
	if !ok {
		return SegmentSettings{}, fmt.Errorf("shedrecipe: %s: row %q: segment %q has no Env.Segments entry; way forward: wire segment %q's settings in the caller's Env.Segments", entry, row.Name, row.Segment, row.Segment)
	}
	return settings, nil
}

// settingRef reads the `module.key` reference cfg holds under key, a string.
// An absent or empty value is not named.
// A named value is split at its first dot, and one with no dot, an empty module or an empty key is refused, as is a nil env.Setting.
func settingRef(entry string, row RowIdentity, cfg Config, key string, env Env) (ref settingReference, named bool, err error) {
	text, err := configString(cfg, key, false)
	if err != nil || text == "" {
		return settingReference{}, false, err
	}
	module, settingKey, hasDot := strings.Cut(text, ".")
	if !hasDot || module == "" || settingKey == "" {
		return settingReference{}, false, fmt.Errorf("shedrecipe: %s: row %q: config key %q value %q is not a module.key setting reference; way forward: set %s to a module.key such as %q", entry, row.Name, key, text, key, "loom.discussion_reviewer_fan")
	}
	if env.Setting == nil {
		return settingReference{}, false, fmt.Errorf("shedrecipe: %s: row %q: config key %q names setting %q but Env.Setting is nil; way forward: fill Env.Setting in the caller that builds this recipe", entry, row.Name, key, text)
	}
	return settingReference{Module: module, Key: settingKey, Text: text}, true, nil
}

// readSetting reads the setting cfg's key names through env.Setting.
// A key that names none reports named false; a lookup error is wrapped with the entry, the row, the key and the reference.
func readSetting(entry string, row RowIdentity, cfg Config, key string, env Env) (value any, ref settingReference, named bool, err error) {
	ref, named, err = settingRef(entry, row, cfg, key, env)
	if err != nil || !named {
		return nil, settingReference{}, false, err
	}
	value, err = env.Setting(ref.Module, ref.Key)
	if err != nil {
		return nil, settingReference{}, false, fmt.Errorf("shedrecipe: %s: row %q: config key %q setting %q: %w", entry, row.Name, key, ref.Text, err)
	}
	return value, ref, true, nil
}

// fanSetting reads the setting cfg's key names as a per-round fan list.
// A string is a one-entry list and a list of strings is the list, a null entry of it counting as ""; a null, an empty string and a list whose every entry is "" give a nil list with named still true.
// Any other shape is refused naming the entry, the row, the key, the reference and the shape.
func fanSetting(entry string, row RowIdentity, cfg Config, key string, env Env) (fans []string, ref settingReference, named bool, err error) {
	value, ref, named, err := readSetting(entry, row, cfg, key, env)
	if err != nil || !named {
		return nil, ref, named, err
	}
	refuse := func(shape string) error {
		return fmt.Errorf("shedrecipe: %s: row %q: config key %q setting %q (%s) holds %s, want a fan name or a list of fan names; way forward: point %s at a setting holding a fan name or a list of fan names", entry, row.Name, key, ref.Text, ref, shape, key)
	}
	switch v := value.(type) {
	case nil:
		return nil, ref, true, nil
	case string:
		if v == "" {
			return nil, ref, true, nil
		}
		return []string{v}, ref, true, nil
	case []any:
		list := make([]string, len(v))
		for i, element := range v {
			switch name := element.(type) {
			case nil:
				list[i] = ""
			case string:
				list[i] = name
			default:
				return nil, ref, true, refuse(fmt.Sprintf("a list whose entry %d is a %T", i, element))
			}
		}
		if !hasFanName(list) {
			return nil, ref, true, nil
		}
		return list, ref, true, nil
	default:
		return nil, ref, true, refuse(fmt.Sprintf("a %T", value))
	}
}

// boolSetting reads the setting cfg's key names as a bool: false for a key that names none and for a null, and a refusal for any other shape.
func boolSetting(entry string, row RowIdentity, cfg Config, key string, env Env) (bool, error) {
	value, ref, named, err := readSetting(entry, row, cfg, key, env)
	if err != nil || !named {
		return false, err
	}
	switch v := value.(type) {
	case nil:
		return false, nil
	case bool:
		return v, nil
	default:
		return false, fmt.Errorf("shedrecipe: %s: row %q: config key %q setting %q (%s) holds a %T, want a bool; way forward: point %s at a setting holding a bool", entry, row.Name, key, ref.Text, ref, value, key)
	}
}
