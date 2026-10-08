package burlermarker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// readySuffix is appended to the review file's name to form the marker's name.
const readySuffix = ".ready"

// Path returns the ready marker path of the round whose review is reviewPath.
// A relative reviewPath is resolved against root.
// The result must lie inside root, else the error names both paths and says to place the review path under root.
// The marker sits under base's ephemeral lyx directory at the review's path relative to root.
// A leading durable lyx directory segment is dropped, and ".ready" is appended to the file name.
func Path(root, base, reviewPath string) (string, error) {
	if !filepath.IsAbs(reviewPath) {
		reviewPath = filepath.Join(root, reviewPath)
	}
	relative, err := filepath.Rel(root, reviewPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("burlermarker: review path %s is not inside %s; place the review path under %s", reviewPath, root, root)
	}
	segments := strings.Split(relative, string(filepath.Separator))
	if segments[0] == lyxdirs.LyxDirName {
		segments = segments[1:]
	}
	markerParts := append([]string{base, lyxdirs.DotLyxDirName}, segments...)
	return filepath.Join(markerParts...) + readySuffix, nil
}
