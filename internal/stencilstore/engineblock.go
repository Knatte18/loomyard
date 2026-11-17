// engineblock.go resolves engine blocks, the include blocks whose text differs by the engine a stencil is rendered for.
// It implements ReadSet, which reads a stencil with every block it includes.

package stencilstore

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// EngineBlockPrefix opens the include name of an engine block.
// A stencil pulls in block <block> of the engine it is rendered for with the bare {{template "engine-<block>"}} action.
const EngineBlockPrefix = "engine-"

// EngineBlocksDir is the directory under a stencils baseDir that holds one directory of engine blocks per engine.
const EngineBlocksDir = "engine"

// EngineBlockName returns the stencil name of engine's copy of block, `engine/<engine>/<block>`.
// That copy is the file `engine/<engine>/<block>.md` under the stencils base directory.
func EngineBlockName(engine, block string) string {
	return path.Join(EngineBlocksDir, engine, block)
}

// MissingEngineBlockError reports that an engine has no copy of an engine block a stencil includes.
// Path is the absolute path the copy was looked for at.
type MissingEngineBlockError struct {
	Block  string
	Engine string
	Path   string
}

// Error names the block, the engine and the file, and the way forward.
func (e *MissingEngineBlockError) Error() string {
	return fmt.Sprintf("stencilstore: engine %q has no engine block %q: %s does not exist; way forward: add the block at that path, or run the seat on an engine that has it", e.Engine, e.Block, e.Path)
}

// ReadSet reads the stencil name from baseDir with every block it includes, keyed by include name.
// An engine block, an include whose name opens with EngineBlockPrefix, is read from engine's copy of the block;
// any other include is read as the stencil of its name.
// An engine block engine has no copy of is a *MissingEngineBlockError.
// An engine block with an empty engine is refused, since no engine names the copy to read.
// A stencil that includes no engine block reads the same whatever engine is given.
func ReadSet(baseDir, engine, name string) ([]byte, map[string][]byte, error) {
	body, err := Read(baseDir, name)
	if err != nil {
		return nil, nil, err
	}
	includes, err := stencil.IncludeNames(body)
	if err != nil {
		return nil, nil, fmt.Errorf("stencilstore: stencil %q: %w", name, err)
	}
	blocks := make(map[string][]byte, len(includes))
	for _, include := range includes {
		block, err := readInclude(baseDir, engine, name, include)
		if err != nil {
			return nil, nil, err
		}
		blocks[include] = block
	}
	return body, blocks, nil
}

// readInclude reads the block that include names, for the stencil called stencilName.
// An engine block is read from engine's copy; any other include is read as the stencil of its name.
func readInclude(baseDir, engine, stencilName, include string) ([]byte, error) {
	block, isEngineBlock := strings.CutPrefix(include, EngineBlockPrefix)
	if !isEngineBlock {
		return Read(baseDir, include)
	}
	if engine == "" {
		return nil, fmt.Errorf("stencilstore: stencil %q includes engine block %q, but no engine was named to read it for; way forward: name the seat's engine in its model spec, or set the engine key of shuttle.yaml", stencilName, block)
	}
	blockPath := Path(baseDir, EngineBlockName(engine, block))
	content, err := os.ReadFile(blockPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, &MissingEngineBlockError{Block: block, Engine: engine, Path: blockPath}
	}
	if err != nil {
		return nil, fmt.Errorf("stencilstore: read engine block %q of engine %q: %w", block, engine, err)
	}
	return content, nil
}
