// fileops.go times small file operations in a folder: the operations a test's fixtures make many of.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sizes of the files the operations write.
const (
	smallFileBytes = 64
	blockBytes     = 4096
)

// measureFileOps repeats each file operation count times in a folder it makes inside dir.
// It adds the median time of each under fileops/<name>/.
func measureFileOps(ctx context.Context, dir NamedDir, count int, measurements *collector) error {
	workDir, err := os.MkdirTemp(dir.Path, probeFolderPrefix)
	if err != nil {
		return fmt.Errorf("create folder in %s: %w", dir.Path, err)
	}
	defer os.RemoveAll(workDir)

	smallContent := make([]byte, smallFileBytes)
	block := make([]byte, blockBytes)
	statTarget := filepath.Join(workDir, "stat-target")
	if err := os.WriteFile(statTarget, smallContent, 0o644); err != nil {
		return fmt.Errorf("write stat target: %w", err)
	}
	blockFile, err := os.Create(filepath.Join(workDir, "block"))
	if err != nil {
		return fmt.Errorf("create block file: %w", err)
	}
	defer blockFile.Close()

	operations := []struct {
		name string
		run  func() error
	}{
		{"create_write_close_delete_us", func() error {
			path := filepath.Join(workDir, "small")
			if err := os.WriteFile(path, smallContent, 0o644); err != nil {
				return err
			}
			return os.Remove(path)
		}},
		{"stat_us", func() error {
			_, err := os.Stat(statTarget)
			return err
		}},
		{"mkdir_rmdir_us", func() error {
			path := filepath.Join(workDir, "folder")
			if err := os.Mkdir(path, 0o755); err != nil {
				return err
			}
			return os.Remove(path)
		}},
		{"write_4k_us", func() error {
			_, err := blockFile.WriteAt(block, 0)
			return err
		}},
		{"write_4k_fsync_us", func() error {
			if _, err := blockFile.WriteAt(block, 0); err != nil {
				return err
			}
			return blockFile.Sync()
		}},
	}
	prefix := groupFileOps + "/" + dir.Name + "/"
	for _, operation := range operations {
		samples := make([]time.Duration, 0, count)
		for range count {
			if err := ctx.Err(); err != nil {
				return err
			}
			started := time.Now()
			if err := operation.run(); err != nil {
				return fmt.Errorf("file operation %s in %s: %w", operation.name, workDir, err)
			}
			samples = append(samples, time.Since(started))
		}
		measurements.add(prefix+operation.name, unitMicroseconds, microseconds(median(samples)))
	}
	return nil
}
