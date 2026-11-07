// config.go implements the typed Config accessors that every entry extracts its keys through.
// It also implements configRejectUnknown, the unknown-key check every entry ends its extraction with.

package shedrecipe

import (
	"fmt"
	"sort"
)

// The string, slice and map accessors treat a present empty value as absent:
// an error when the key is required, otherwise the zero value with a nil error.

// configString extracts key from cfg as a string.
// A key that is missing, or present with an empty string, counts as absent:
// an error when required, otherwise the zero value with a nil error.
// A present value of any other type is an error naming key, the expected shape, and the actual type.
// This holds whether or not key is required.
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

// configStringSlice extracts key from cfg as a []string.
// It accepts both a []string and a []any whose every element is a string.
// A []any with a non-string element is an error naming key and the offending element's index.
// A missing key, or a present empty slice of either shape, counts as absent.
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

// configBool extracts key from cfg as a bool.
// A missing key counts as absent: an error when required, otherwise the zero value with a nil error.
// A bool has no empty-value absence case: false is a legitimate present value, not an absence signal.
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

// configInt extracts key from cfg as an int.
// It accepts an int, and a float64 with an integral value.
// Decoders for different formats yield different numeric types, so the package accepts both.
// A fractional float64 is an error naming key.
// A missing key counts as absent: an error when required, otherwise the zero value with a nil error.
// Zero is a legitimate present value, not an absence signal.
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

// configStringMap extracts key from cfg as a map[string]string.
// It accepts a map[string]any whose every value is a string.
// A non-string value is an error naming both the outer key and the offending inner key.
// A missing key, or a present empty map, counts as absent.
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

// configMap extracts key from cfg as a Config, so a nested map can be read with the same typed accessors.
// It accepts a map[string]any.
// A missing key, or a present empty map, counts as absent.
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

// configMapList extracts key from cfg as a []Config, so each element can be read with the same typed accessors.
// It accepts a []any whose every element is a map[string]any, and a []map[string]any;
// an element of any other type is an error naming key and the element's index.
// present reports whether key is in cfg.
// A present empty list is not absent: it returns an empty slice with present true,
// so a caller can tell an author mistake from a missing key.
func configMapList(cfg Config, key string) (out []Config, present bool, err error) {
	raw, ok := cfg[key]
	if !ok {
		return nil, false, nil
	}
	switch v := raw.(type) {
	case []map[string]any:
		out = make([]Config, len(v))
		for i, m := range v {
			out[i] = Config(m)
		}
	case []any:
		out = make([]Config, len(v))
		for i, elem := range v {
			m, ok := elem.(map[string]any)
			if !ok {
				return nil, true, fmt.Errorf("shedrecipe: config key %q element %d must be a map, got %T", key, i, elem)
			}
			out[i] = Config(m)
		}
	default:
		return nil, true, fmt.Errorf("shedrecipe: config key %q must be a list of maps, got %T", key, raw)
	}
	return out, true, nil
}

// configRejectUnknown errors naming the first unrecognized key in cfg, in sorted order,
// so the message is deterministic across runs.
// A cfg with only recognized keys, and a nil or empty cfg, both return nil.
//
// It is also called on nested maps, not only the outer one, so each nested level rejects its own unknown keys.
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
