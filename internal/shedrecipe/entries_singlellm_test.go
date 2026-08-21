// entries_singlellm_test.go covers singleLLMEntry: its construction-time validations over Config
// and Env, and the composed shuttleengine.Spec its shedadapters.SpecSource closure builds, reached
// by driving the returned producer's Call through a recording fakeShuttle since the closure itself
// is unreachable from outside shedadapters.SingleLLMProducer.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// writeStencilFile writes content to the on-disk location stencilstore.Read(dir, name) resolves
// name to, creating any parent directory the family convention requires.
func writeStencilFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := stencilstore.Path(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir stencil parent %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write stencil %s: %v", path, err)
	}
}

// validSingleLLMEnv returns a fresh newTestEnv(t) with a readable, marker-free stencil already
// written into it, plus that stencil's stencilstore name.
func validSingleLLMEnv(t *testing.T) (Env, string) {
	t.Helper()
	env := newTestEnv(t)
	const name = "singlellm-valid"
	writeStencilFile(t, env.StencilsDir, name, "Body text with no markers.\n")
	return env, name
}

// validSingleLLMConfig returns a minimal Config recognised by singleLLMEntry: the required stencil
// and output_files keys only.
func validSingleLLMConfig(stencilName string) Config {
	return Config{
		"stencil":      stencilName,
		"output_files": []string{"out/a.md"},
	}
}

func TestSingleLLMEntry_ConstructionFailures(t *testing.T) {
	t.Run("StencilNameWithNoFile", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := validSingleLLMConfig("no-such-stencil")
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for a stencil with no file behind it")
		}
		if !strings.Contains(err.Error(), "no-such-stencil") {
			t.Errorf("singleLLMEntry() error = %v; want it to name the offending stencil %q", err, "no-such-stencil")
		}
	})

	for _, reserved := range []string{"worktree_root", "anchor_path", "stencils_dir", "output_files"} {
		reserved := reserved
		t.Run("ReservedToken_"+reserved, func(t *testing.T) {
			env, stencilName := validSingleLLMEnv(t)
			cfg := validSingleLLMConfig(stencilName)
			cfg["tokens"] = map[string]any{reserved: "x"}
			_, err := singleLLMEntry("Row", cfg, env)
			if err == nil {
				t.Fatalf("singleLLMEntry() error = nil; want non-nil for a tokens entry naming reserved token %q", reserved)
			}
			if !strings.Contains(err.Error(), reserved) {
				t.Errorf("singleLLMEntry() error = %v; want it to name the offending token %q", err, reserved)
			}
		})
	}

	t.Run("TokenValueEmptyString", func(t *testing.T) {
		env, stencilName := validSingleLLMEnv(t)
		cfg := validSingleLLMConfig(stencilName)
		cfg["tokens"] = map[string]any{"greeting": ""}
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for an empty-string tokens value")
		}
		if !strings.Contains(err.Error(), "greeting") {
			t.Errorf("singleLLMEntry() error = %v; want it to name the offending token %q", err, "greeting")
		}
	})

	t.Run("TokenValueWhitespaceOnly", func(t *testing.T) {
		env, stencilName := validSingleLLMEnv(t)
		cfg := validSingleLLMConfig(stencilName)
		cfg["tokens"] = map[string]any{"greeting": "   "}
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for a whitespace-only tokens value")
		}
		if !strings.Contains(err.Error(), "greeting") {
			t.Errorf("singleLLMEntry() error = %v; want it to name the offending token %q", err, "greeting")
		}
	})

	t.Run("AbsoluteOutputFilesEntry", func(t *testing.T) {
		env, stencilName := validSingleLLMEnv(t)
		cfg := validSingleLLMConfig(stencilName)
		cfg["output_files"] = []string{"/abs/out.md"}
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for an absolute output_files entry")
		}
		if !strings.Contains(err.Error(), "output_files") {
			t.Errorf("singleLLMEntry() error = %v; want it to name key %q", err, "output_files")
		}
	})

	t.Run("EscapingOutputFilesEntry", func(t *testing.T) {
		env, stencilName := validSingleLLMEnv(t)
		cfg := validSingleLLMConfig(stencilName)
		cfg["output_files"] = []string{"../escape.md"}
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for a ..-escaping output_files entry")
		}
		if !strings.Contains(err.Error(), "output_files") {
			t.Errorf("singleLLMEntry() error = %v; want it to name key %q", err, "output_files")
		}
	})

	t.Run("MissingStencilKey", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"output_files": []string{"out/a.md"}}
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for a missing stencil key")
		}
		if !strings.Contains(err.Error(), "stencil") {
			t.Errorf("singleLLMEntry() error = %v; want it to name key %q", err, "stencil")
		}
	})

	t.Run("MissingOutputFilesKey", func(t *testing.T) {
		env, stencilName := validSingleLLMEnv(t)
		cfg := Config{"stencil": stencilName}
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for a missing output_files key")
		}
		if !strings.Contains(err.Error(), "output_files") {
			t.Errorf("singleLLMEntry() error = %v; want it to name key %q", err, "output_files")
		}
	})

	t.Run("EmptyStringStencilKey", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"stencil": "", "output_files": []string{"out/a.md"}}
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for an empty-string stencil key")
		}
		if !strings.Contains(err.Error(), "stencil") {
			t.Errorf("singleLLMEntry() error = %v; want it to name key %q", err, "stencil")
		}
	})

	t.Run("EmptyOutputFilesList", func(t *testing.T) {
		env, stencilName := validSingleLLMEnv(t)
		cfg := Config{"stencil": stencilName, "output_files": []string{}}
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for an empty output_files list")
		}
		if !strings.Contains(err.Error(), "output_files") {
			t.Errorf("singleLLMEntry() error = %v; want it to name key %q", err, "output_files")
		}
	})

	t.Run("UnrecognisedConfigKey", func(t *testing.T) {
		env, stencilName := validSingleLLMEnv(t)
		cfg := validSingleLLMConfig(stencilName)
		cfg["bogus_key"] = "x"
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil for an unrecognised config key")
		}
		if !strings.Contains(err.Error(), "bogus_key") {
			t.Errorf("singleLLMEntry() error = %v; want it to name the offending key %q", err, "bogus_key")
		}
	})

	for _, field := range []string{"WorktreeRoot", "AnchorPath", "StencilsDir"} {
		field := field
		t.Run("Blank"+field, func(t *testing.T) {
			env, stencilName := validSingleLLMEnv(t)
			env = zeroEnvField(env, field)
			cfg := validSingleLLMConfig(stencilName)
			_, err := singleLLMEntry("Row", cfg, env)
			if err == nil {
				t.Fatalf("singleLLMEntry() error = nil; want non-nil when Env.%s is blank", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Errorf("singleLLMEntry() error = %v; want it to name field %q", err, field)
			}
		})
	}

	t.Run("NilShuttle", func(t *testing.T) {
		env, stencilName := validSingleLLMEnv(t)
		env.Shuttle = nil
		cfg := validSingleLLMConfig(stencilName)
		_, err := singleLLMEntry("Row", cfg, env)
		if err == nil {
			t.Fatalf("singleLLMEntry() error = nil; want non-nil when Env.Shuttle is nil")
		}
		if !strings.Contains(err.Error(), "Shuttle") {
			t.Errorf("singleLLMEntry() error = %v; want it to name field %q", err, "Shuttle")
		}
	})
}

// TestSingleLLMEntry_ComposedSpec drives Call through a recording fakeShuttle and asserts the
// composed shuttleengine.Spec carries the filled prompt, the resolved absolute OutputFiles in
// Config order, and the Model/Effort/Version/Role/Interactive values from Config. It never
// pre-creates spec.OutputFiles, since Spec.OutputFiles entries must not exist when a run starts.
func TestSingleLLMEntry_ComposedSpec(t *testing.T) {
	env := newTestEnv(t)
	writeStencilFile(t, env.StencilsDir, "singlellm-composed", "Hello {{.greeting}}!\n")

	cfg := Config{
		"stencil":      "singlellm-composed",
		"output_files": []string{"out/a.md", "out/b.md"},
		"model":        "claude-opus",
		"effort":       "high",
		"version":      "v1",
		"role":         "author",
		"interactive":  true,
		"tokens":       map[string]any{"greeting": "world"},
	}

	producer, err := singleLLMEntry("SingleLLMRow", cfg, env)
	if err != nil {
		t.Fatalf("singleLLMEntry() error = %v; want nil", err)
	}

	fake := env.Shuttle.(*fakeShuttle)
	fake.result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}

	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if len(fake.specs) != 1 {
		t.Fatalf("fake.specs has %d entries; want 1", len(fake.specs))
	}
	spec := fake.specs[0]

	wantPrompt := "Hello world!\n"
	if spec.Prompt != wantPrompt {
		t.Errorf("spec.Prompt = %q; want %q", spec.Prompt, wantPrompt)
	}

	wantOutputs := []string{
		filepath.Join(env.WorktreeRoot, "out/a.md"),
		filepath.Join(env.WorktreeRoot, "out/b.md"),
	}
	if len(spec.OutputFiles) != len(wantOutputs) {
		t.Fatalf("spec.OutputFiles = %v; want %v", spec.OutputFiles, wantOutputs)
	}
	for i, want := range wantOutputs {
		if spec.OutputFiles[i] != want {
			t.Errorf("spec.OutputFiles[%d] = %q; want %q", i, spec.OutputFiles[i], want)
		}
	}

	if spec.Model != "claude-opus" {
		t.Errorf("spec.Model = %q; want %q", spec.Model, "claude-opus")
	}
	if spec.Effort != "high" {
		t.Errorf("spec.Effort = %q; want %q", spec.Effort, "high")
	}
	if spec.Version != "v1" {
		t.Errorf("spec.Version = %q; want %q", spec.Version, "v1")
	}
	if spec.Role != "author" {
		t.Errorf("spec.Role = %q; want %q", spec.Role, "author")
	}
	if !spec.Interactive {
		t.Errorf("spec.Interactive = false; want true")
	}
}

// TestSingleLLMEntry_ReservedTokenValues drives Call against a stencil declaring all four reserved
// markers and asserts the filled prompt carries env.WorktreeRoot, env.AnchorPath, env.StencilsDir,
// and the newline-joined resolved output paths.
func TestSingleLLMEntry_ReservedTokenValues(t *testing.T) {
	env := newTestEnv(t)
	writeStencilFile(t, env.StencilsDir, "singlellm-reserved",
		"{{.worktree_root}}\n{{.anchor_path}}\n{{.stencils_dir}}\n{{.output_files}}\n")

	cfg := Config{
		"stencil":      "singlellm-reserved",
		"output_files": []string{"out/a.md", "out/b.md"},
	}
	producer, err := singleLLMEntry("Row", cfg, env)
	if err != nil {
		t.Fatalf("singleLLMEntry() error = %v; want nil", err)
	}

	fake := env.Shuttle.(*fakeShuttle)
	fake.result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}

	prompt := fake.specs[0].Prompt
	for _, want := range []string{env.WorktreeRoot, env.AnchorPath, env.StencilsDir} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt = %q; want it to contain %q", prompt, want)
		}
	}
	wantOutputsJoined := strings.Join([]string{
		filepath.Join(env.WorktreeRoot, "out/a.md"),
		filepath.Join(env.WorktreeRoot, "out/b.md"),
	}, "\n")
	if !strings.Contains(prompt, wantOutputsJoined) {
		t.Errorf("prompt = %q; want it to contain the newline-joined resolved output paths %q", prompt, wantOutputsJoined)
	}
}

// TestSingleLLMEntry_CallTimeUnfilledMarker pins stencil.Fill's hard-error behaviour: a stencil
// naming a marker that is neither reserved nor present in Config.tokens makes Call return an error
// rather than filling it empty.
func TestSingleLLMEntry_CallTimeUnfilledMarker(t *testing.T) {
	env := newTestEnv(t)
	writeStencilFile(t, env.StencilsDir, "singlellm-unfilled", "{{.not_reserved_and_not_in_tokens}}\n")

	cfg := Config{
		"stencil":      "singlellm-unfilled",
		"output_files": []string{"out/a.md"},
	}
	producer, err := singleLLMEntry("Row", cfg, env)
	if err != nil {
		t.Fatalf("singleLLMEntry() error = %v; want nil", err)
	}

	fake := env.Shuttle.(*fakeShuttle)
	fake.result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	if _, _, err := producer.Call(context.Background()); err == nil {
		t.Fatalf("Call() error = nil; want non-nil for a marker neither reserved nor in tokens")
	}
}

// TestSingleLLMEntry_NilClock asserts an Env with Now nil constructs successfully -- a nil clock
// defaults inside shedadapters.NewSingleLLMProducer.
func TestSingleLLMEntry_NilClock(t *testing.T) {
	env, stencilName := validSingleLLMEnv(t)
	env.Now = nil
	cfg := validSingleLLMConfig(stencilName)
	producer, err := singleLLMEntry("Row", cfg, env)
	if err != nil {
		t.Fatalf("singleLLMEntry() error = %v; want nil for a nil Env.Now", err)
	}
	if producer == nil {
		t.Fatalf("singleLLMEntry() = nil producer; want non-nil")
	}
}
