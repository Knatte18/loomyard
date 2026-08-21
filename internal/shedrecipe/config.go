// config.go implements the typed Config accessors every entry extracts its recognised keys through,
// plus configRejectUnknown, the shared unknown-key rejector every entry ends its extraction with.

package shedrecipe

import (
	"fmt"
	"sort"
)

// absenceRule tells a present cfg value apart from an absent one: a key missing from cfg, or
// present with an empty-string value, or present with an empty slice or empty map, all count as
// absent. Absent and required is an error naming key; absent and not required returns the Go zero
// value with a nil error.

// configString extracts key from cfg as a string. A key missing, or present with an empty string,
// counts as absent -- an error when required, else the zero value with a nil error. A present value
// of any other type is an error naming key, the expected shape, and the actual %T, whether or not
// key is required.
func configString(cfg Config, key string, required bool) (string, error) {
	raw, ok := cfg[key]
	if !ok {
		if required {
			return "", fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("shedrecipe: config key %q must be a string, got %T", key, raw)
	}
	if s == "" {
		if required {
			return "", fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return "", nil
	}
	return s, nil
}

// configStringSlice extracts key from cfg as a []string. It accepts both a []string and a []any
// whose every element is a string; a []any carrying a non-string element is an error naming key and
// the offending element's index. A missing key, or a present empty slice of either shape, counts as
// absent.
func configStringSlice(cfg Config, key string, required bool) ([]string, error) {
	raw, ok := cfg[key]
	if !ok {
		if required {
			return nil, fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return nil, nil
	}

	var out []string
	switch v := raw.(type) {
	case []string:
		out = v
	case []any:
		out = make([]string, len(v))
		for i, elem := range v {
			s, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("shedrecipe: config key %q element %d must be a string, got %T", key, i, elem)
			}
			out[i] = s
		}
	default:
		return nil, fmt.Errorf("shedrecipe: config key %q must be a string list, got %T", key, raw)
	}

	if len(out) == 0 {
		if required {
			return nil, fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return nil, nil
	}
	return out, nil
}

// configBool extracts key from cfg as a bool. A key missing counts as absent -- an error when
// required, else the zero value with a nil error. There is no empty-value absence case for a bool,
// unlike the string/slice/map accessors: false is a legitimate present value, not an absence
// signal.
func configBool(cfg Config, key string, required bool) (bool, error) {
	raw, ok := cfg[key]
	if !ok {
		if required {
			return false, fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return false, nil
	}
	b, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("shedrecipe: config key %q must be a bool, got %T", key, raw)
	}
	return b, nil
}

// configInt extracts key from cfg as an int. It accepts both an int and a float64 whose value is
// integral -- a YAML decoder into any yields int while a JSON-shaped one yields float64, and piece 2
// picks the format, so accepting both without truncating keeps this package format-agnostic. A
// fractional float64 is an error naming key. A key missing counts as absent -- an error when
// required, else the zero value with a nil error; zero itself is a legitimate present value, not an
// absence signal.
func configInt(cfg Config, key string, required bool) (int, error) {
	raw, ok := cfg[key]
	if !ok {
		if required {
			return 0, fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return 0, nil
	}
	switch v := raw.(type) {
	case int:
		return v, nil
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("shedrecipe: config key %q must be an integer, got fractional value %v", key, v)
		}
		return int(v), nil
	default:
		return 0, fmt.Errorf("shedrecipe: config key %q must be an int, got %T", key, raw)
	}
}

// configStringMap extracts key from cfg as a map[string]string. It accepts a map[string]any whose
// every value is a string; a non-string value is an error naming both the outer key and the
// offending inner key. A missing key, or a present empty map, counts as absent.
func configStringMap(cfg Config, key string, required bool) (map[string]string, error) {
	raw, ok := cfg[key]
	if !ok {
		if required {
			return nil, fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("shedrecipe: config key %q must be a string map, got %T", key, raw)
	}
	if len(m) == 0 {
		if required {
			return nil, fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return nil, nil
	}
	out := make(map[string]string, len(m))
	for innerKey, v := range m {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("shedrecipe: config key %q.%q must be a string, got %T", key, innerKey, v)
		}
		out[innerKey] = s
	}
	return out, nil
}

// configMap extracts key from cfg as a Config, so a nested map can be fed straight back through
// these same accessors. It accepts a map[string]any. A missing key, or a present empty map, counts
// as absent.
func configMap(cfg Config, key string, required bool) (Config, error) {
	raw, ok := cfg[key]
	if !ok {
		if required {
			return nil, fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("shedrecipe: config key %q must be a map, got %T", key, raw)
	}
	if len(m) == 0 {
		if required {
			return nil, fmt.Errorf("shedrecipe: config key %q is required", key)
		}
		return nil, nil
	}
	return Config(m), nil
}

// configRejectUnknown errors naming the first unrecognised key in cfg, in sorted order, so the
// message is deterministic across runs. A cfg with only recognised keys, and a nil or empty cfg,
// both return nil.
//
// This runs at nested levels too, not only on the outer map -- BurlerRound's profile,
// profile.target, and profile.fasit each get their own call.
func configRejectUnknown(cfg Config, known ...string) error {
	if len(cfg) == 0 {
		return nil
	}

	knownSet := make(map[string]bool, len(known))
	for _, k := range known {
		knownSet[k] = true
	}

	var unknown []string
	for k := range cfg {
		if !knownSet[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("shedrecipe: unrecognized config key %q", unknown[0])
}
