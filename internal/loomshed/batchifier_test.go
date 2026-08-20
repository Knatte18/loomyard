package loomshed

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// writeBatcherConfig writes content as <anchorPath>/_lyx/config/batcher.yaml.
func writeBatcherConfig(t *testing.T, anchorPath, content string) {
	t.Helper()
	configDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "batcher.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write batcher.yaml: %v", err)
	}
}

func TestBatchifier_Call(t *testing.T) {
	t.Run("ValidConfigMapsToDone", func(t *testing.T) {
		anchorPath := t.TempDir()
		writeBatcherConfig(t, anchorPath, `active: "identity"`+"\n")

		b := newBatchifier("Batchifier", anchorPath)
		outcome, pointer, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		if pointer != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want empty", pointer)
		}
	})

	t.Run("UnknownBatchifierNameMapsToStuck", func(t *testing.T) {
		anchorPath := t.TempDir()
		writeBatcherConfig(t, anchorPath, `active: "no-such-batcher"`+"\n")

		b := newBatchifier("Batchifier", anchorPath)
		outcome, _, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
	})

	t.Run("MalformedConfigFileMapsToStuck", func(t *testing.T) {
		anchorPath := t.TempDir()
		writeBatcherConfig(t, anchorPath, "active: [not valid yaml\n")

		b := newBatchifier("Batchifier", anchorPath)
		outcome, _, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
	})

	t.Run("AbsentConfigFileMapsToDoneViaTemplateFallback", func(t *testing.T) {
		anchorPath := t.TempDir()
		if err := os.MkdirAll(filepath.Join(anchorPath, lyxdirs.LyxDirName), 0o755); err != nil {
			t.Fatalf("mkdir _lyx: %v", err)
		}

		b := newBatchifier("Batchifier", anchorPath)
		outcome, _, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q — Active falls back to the embedded template", outcome, shedengine.Done)
		}
	})

	t.Run("AbsentLyxDirMapsToDoneViaTemplateFallback", func(t *testing.T) {
		anchorPath := t.TempDir() // no _lyx/ at all

		b := newBatchifier("Batchifier", anchorPath)
		outcome, _, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q — Active falls back to the embedded template", outcome, shedengine.Done)
		}
	})

	t.Run("CancelledContextReturnsErrorNotVerdict", func(t *testing.T) {
		anchorPath := t.TempDir()
		writeBatcherConfig(t, anchorPath, `active: "identity"`+"\n")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		b := newBatchifier("Batchifier", anchorPath)
		outcome, _, err := b.Call(ctx)
		if err == nil {
			t.Fatalf("Call(cancelled) error = nil; want non-nil error")
		}
		if outcome == shedengine.Done || outcome == shedengine.Stuck {
			t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
		}
	})
}
