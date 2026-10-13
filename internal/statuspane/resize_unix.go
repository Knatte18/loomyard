//go:build !windows

// resize_unix.go relays the terminal's resize signal to the pane loop.

package statuspane

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// NotifyResize returns a channel that receives one value per terminal resize until ctx ends.
// A resize arriving while a value is still unread is folded into it.
func NotifyResize(ctx context.Context) <-chan struct{} {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	resized := make(chan struct{}, 1)
	go func() {
		defer signal.Stop(signals)
		for {
			select {
			case <-ctx.Done():
				return
			case <-signals:
				select {
				case resized <- struct{}{}:
				default:
				}
			}
		}
	}()
	return resized
}
