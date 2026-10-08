// segmentcolor.go resolves a loom segment to its configured palette color and validates the segment color config at boot.

package reedengine

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// validateSegmentColors fails when any configured value is outside the palette.
// The error names the key, the value and the palette as the way forward; keys are checked in sorted order so the message is deterministic.
func validateSegmentColors(colors map[string]string) error {
	keys := make([]string, 0, len(colors))
	for key := range colors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := segmentcolor.ParseColor(colors[key]); err != nil {
			return fmt.Errorf("invalid segment_colors.%s: %w; set segment_colors.%s to one of the palette colors", key, err, key)
		}
	}
	return nil
}

// segmentColor returns the palette color configured for a segment.
// An empty segment yields no color silently; a segment with no key, or a value outside the palette, yields no color and a logged warning.
// Only palette names leave it, so no config value carries format syntax into a tmux format or a typed command.
func (e *Engine) segmentColor(segment string) (segmentcolor.Color, bool) {
	if segment == "" {
		return "", false
	}
	raw, ok := e.cfg.SegmentColors[segment]
	if !ok {
		logger.Warn("reed: no color configured for segment", "segment", segment)
		return "", false
	}
	color, err := segmentcolor.ParseColor(raw)
	if err != nil {
		logger.Warn("reed: segment color is outside the palette", "segment", segment, "value", raw)
		return "", false
	}
	return color, true
}
