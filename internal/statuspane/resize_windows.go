//go:build windows

// resize_windows.go is the pane loop's resize signal on Windows, where none is relayed.

package statuspane

import "context"

// NotifyResize returns nil on Windows, which signals no resize, so the pane loop keeps its poll.
func NotifyResize(ctx context.Context) <-chan struct{} {
	return nil
}
