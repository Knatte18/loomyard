package shedrecipe

import (
	"strings"
	"testing"
)

func TestConfigString(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		key      string
		required bool
		want     string
		wantErr  bool
	}{
		{"PresentCorrectType", Config{"k": "v"}, "k", true, "v", false},
		{"PresentWrongType", Config{"k": 5}, "k", true, "", true},
		{"AbsentRequired", Config{}, "k", true, "", true},
		{"AbsentOptional", Config{}, "k", false, "", false},
		{"PresentEmptyStringRequired", Config{"k": ""}, "k", true, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := configString(tt.cfg, tt.key, tt.required)
			if (err != nil) != tt.wantErr {
				t.Fatalf("configString() error = %v; wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.key) {
					t.Errorf("configString() error = %v; want it to name key %q", err, tt.key)
				}
				return
			}
			if got != tt.want {
				t.Errorf("configString() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestConfigStringSlice(t *testing.T) {
	t.Run("PresentStringSlice", func(t *testing.T) {
		got, err := configStringSlice(Config{"k": []string{"a", "b"}}, "k", true)
		if err != nil {
			t.Fatalf("configStringSlice() error = %v; want nil", err)
		}
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("configStringSlice() = %v; want [a b]", got)
		}
	})

	t.Run("PresentAnySliceOfStrings", func(t *testing.T) {
		got, err := configStringSlice(Config{"k": []any{"a", "b"}}, "k", true)
		if err != nil {
			t.Fatalf("configStringSlice() error = %v; want nil", err)
		}
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("configStringSlice() = %v; want [a b]", got)
		}
	})

	t.Run("PresentAnySliceWithNonStringElement", func(t *testing.T) {
		_, err := configStringSlice(Config{"k": []any{"a", 5}}, "k", true)
		if err == nil {
			t.Fatalf("configStringSlice() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), "k") || !strings.Contains(err.Error(), "1") {
			t.Errorf("configStringSlice() error = %v; want it to name key %q and index 1", err, "k")
		}
	})

	t.Run("PresentWrongType", func(t *testing.T) {
		_, err := configStringSlice(Config{"k": 5}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configStringSlice() error = %v; want it to name key %q", err, "k")
		}
	})

	t.Run("AbsentRequired", func(t *testing.T) {
		_, err := configStringSlice(Config{}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configStringSlice() error = %v; want it to name key %q", err, "k")
		}
	})

	t.Run("AbsentOptional", func(t *testing.T) {
		got, err := configStringSlice(Config{}, "k", false)
		if err != nil {
			t.Fatalf("configStringSlice() error = %v; want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("configStringSlice() = %v; want empty", got)
		}
	})

	t.Run("PresentEmptySliceRequired", func(t *testing.T) {
		_, err := configStringSlice(Config{"k": []string{}}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configStringSlice() error = %v; want it to name key %q", err, "k")
		}
	})
}

func TestConfigBool(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		required bool
		want     bool
		wantErr  bool
	}{
		{"PresentCorrectType", Config{"k": true}, true, true, false},
		{"PresentWrongType", Config{"k": "x"}, true, false, true},
		{"AbsentRequired", Config{}, true, false, true},
		{"AbsentOptional", Config{}, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := configBool(tt.cfg, "k", tt.required)
			if (err != nil) != tt.wantErr {
				t.Fatalf("configBool() error = %v; wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), "k") {
					t.Errorf("configBool() error = %v; want it to name key %q", err, "k")
				}
				return
			}
			if got != tt.want {
				t.Errorf("configBool() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestConfigInt(t *testing.T) {
	t.Run("PresentInt", func(t *testing.T) {
		got, err := configInt(Config{"k": 5}, "k", true)
		if err != nil {
			t.Fatalf("configInt() error = %v; want nil", err)
		}
		if got != 5 {
			t.Errorf("configInt() = %d; want 5", got)
		}
	})

	t.Run("PresentIntegralFloat64", func(t *testing.T) {
		got, err := configInt(Config{"k": float64(5)}, "k", true)
		if err != nil {
			t.Fatalf("configInt() error = %v; want nil", err)
		}
		if got != 5 {
			t.Errorf("configInt() = %d; want 5", got)
		}
	})

	t.Run("PresentFractionalFloat64", func(t *testing.T) {
		_, err := configInt(Config{"k": 5.5}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configInt() error = %v; want it to name key %q", err, "k")
		}
	})

	t.Run("PresentWrongType", func(t *testing.T) {
		_, err := configInt(Config{"k": "x"}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configInt() error = %v; want it to name key %q", err, "k")
		}
	})

	t.Run("AbsentRequired", func(t *testing.T) {
		_, err := configInt(Config{}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configInt() error = %v; want it to name key %q", err, "k")
		}
	})

	t.Run("AbsentOptional", func(t *testing.T) {
		got, err := configInt(Config{}, "k", false)
		if err != nil {
			t.Fatalf("configInt() error = %v; want nil", err)
		}
		if got != 0 {
			t.Errorf("configInt() = %d; want 0", got)
		}
	})
}

func TestConfigStringMap(t *testing.T) {
	t.Run("PresentCorrectType", func(t *testing.T) {
		got, err := configStringMap(Config{"k": map[string]any{"a": "1"}}, "k", true)
		if err != nil {
			t.Fatalf("configStringMap() error = %v; want nil", err)
		}
		if got["a"] != "1" {
			t.Errorf("configStringMap() = %v; want map[a:1]", got)
		}
	})

	t.Run("PresentWithNonStringValue", func(t *testing.T) {
		_, err := configStringMap(Config{"k": map[string]any{"a": 5}}, "k", true)
		if err == nil {
			t.Fatalf("configStringMap() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), "k") || !strings.Contains(err.Error(), "a") {
			t.Errorf("configStringMap() error = %v; want it to name outer key %q and inner key %q", err, "k", "a")
		}
	})

	t.Run("PresentWrongType", func(t *testing.T) {
		_, err := configStringMap(Config{"k": 5}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configStringMap() error = %v; want it to name key %q", err, "k")
		}
	})

	t.Run("AbsentRequired", func(t *testing.T) {
		_, err := configStringMap(Config{}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configStringMap() error = %v; want it to name key %q", err, "k")
		}
	})

	t.Run("AbsentOptional", func(t *testing.T) {
		got, err := configStringMap(Config{}, "k", false)
		if err != nil {
			t.Fatalf("configStringMap() error = %v; want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("configStringMap() = %v; want empty", got)
		}
	})
}

func TestConfigMap(t *testing.T) {
	t.Run("PresentCorrectType", func(t *testing.T) {
		got, err := configMap(Config{"k": map[string]any{"a": "1"}}, "k", true)
		if err != nil {
			t.Fatalf("configMap() error = %v; want nil", err)
		}
		if got["a"] != "1" {
			t.Errorf("configMap() = %v; want map[a:1]", got)
		}
	})

	t.Run("PresentWrongType", func(t *testing.T) {
		_, err := configMap(Config{"k": 5}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configMap() error = %v; want it to name key %q", err, "k")
		}
	})

	t.Run("AbsentRequired", func(t *testing.T) {
		_, err := configMap(Config{}, "k", true)
		if err == nil || !strings.Contains(err.Error(), "k") {
			t.Errorf("configMap() error = %v; want it to name key %q", err, "k")
		}
	})

	t.Run("AbsentOptional", func(t *testing.T) {
		got, err := configMap(Config{}, "k", false)
		if err != nil {
			t.Fatalf("configMap() error = %v; want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("configMap() = %v; want empty", got)
		}
	})
}

func TestConfigRejectUnknown(t *testing.T) {
	t.Run("OneUnrecognizedKey", func(t *testing.T) {
		err := configRejectUnknown(Config{"bad": 1, "good": 1}, "good")
		if err == nil {
			t.Fatalf("configRejectUnknown() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), "bad") {
			t.Errorf("configRejectUnknown() error = %v; want it to name key %q", err, "bad")
		}
	})

	t.Run("TwoUnrecognizedKeysNamesSortedFirst", func(t *testing.T) {
		err := configRejectUnknown(Config{"zeta": 1, "alpha": 1})
		if err == nil {
			t.Fatalf("configRejectUnknown() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), "alpha") {
			t.Errorf("configRejectUnknown() error = %v; want the sorted-first key %q", err, "alpha")
		}
	})

	t.Run("OnlyRecognizedKeys", func(t *testing.T) {
		if err := configRejectUnknown(Config{"good": 1}, "good"); err != nil {
			t.Errorf("configRejectUnknown() error = %v; want nil", err)
		}
	})

	t.Run("EmptyConfig", func(t *testing.T) {
		if err := configRejectUnknown(Config{}, "good"); err != nil {
			t.Errorf("configRejectUnknown() error = %v; want nil", err)
		}
	})

	t.Run("NilConfig", func(t *testing.T) {
		if err := configRejectUnknown(nil, "good"); err != nil {
			t.Errorf("configRejectUnknown() error = %v; want nil", err)
		}
	})
}
