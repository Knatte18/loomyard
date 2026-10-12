// pausegeom.go implements PauseGeometry, the told-mode builder of the pause record's geometry.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// PauseGeometry builds the pauserecord.Geometry for a standalone run whose state directory is the told stateDir.
// The slug is empty: a standalone run belongs to no hub pair, so only a hub-scope entry in this record would cover it.
func PauseGeometry(stateDir string) pauserecord.Geometry {
	return pauserecord.Geometry{
		RecordPath: filepath.Join(stateDir, lyxdirs.DotLyxDirName, lyxdirs.PauseDirName, pauserecord.RecordFileName),
	}
}
