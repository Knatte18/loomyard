// engineblock.go resolves engine blocks, the include blocks whose text differs by the engine a stencil is rendered for.
// It implements ReadSet, which reads a stencil with every block it includes.
// It also implements ReadForEngine, which reads a stencil with its engine blocks written in place.

package stencilstore

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// EngineBlockPrefix opens the include name of an engine block.
// A stencil pulls in block <block> of the engine it is rendered for with the bare {{template "engine-<block>"}} action.
const EngineBlockPrefix = "engine-"

// EngineBlocksDir is the directory under a stencils baseDir that holds one directory of engine blocks per engine.
const EngineBlocksDir = "engine"

// bareIncludePattern matches a bare include action, engine block or plain fragment, and captures the include's name.
var bareIncludePattern = regexp.MustCompile(`\{\{template "([^"]+)"\}\}`)

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
// any other include is read as the stencil of its name, with the engine blocks it includes written in place.
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

// ReadForEngine reads the stencil name from baseDir.
// Each engine block it includes is written in place, in engine's copy.
// Each bare plain include is written in place too, as the stencil of that name with its leading comment dropped.
// The engine blocks that stencil includes are written in place as well.
// Its own plain includes are left as they stand.
// A producer that fills a stencil with no include map calls it, so the text it fills is already worded for engine.
// A plain include of a stencil that does not exist is an error naming both stencils and the way forward.
// Its other errors are ReadSet's.
func ReadForEngine(baseDir, engine, name string) ([]byte, error) {
	body, err := Read(baseDir, name)
	if err != nil {
		return nil, err
	}
	return writeEngineBlocksInPlace(baseDir, engine, name, body, true)
}

// writeEngineBlocksInPlace returns body with each bare engine-block include action replaced.
// An engine block is replaced by engine's copy of it, its leading comment dropped.
// With plainIncludes, each bare plain include is replaced too.
// It is replaced by the stencil of its name read through readInclude, its leading comment dropped.
// Without plainIncludes, a plain include is left as it stands.
// A plain include of a stencil that does not exist is refused, naming stencilName and the missing stencil.
// A leading comment of body itself is kept as it stands, so a header that spells an include action is not read.
// An engine-block include left unresolved is refused:
// one written in a form other than the bare action, or one an engine block itself carries.
func writeEngineBlocksInPlace(baseDir, engine, stencilName string, body []byte, plainIncludes bool) ([]byte, error) {
	text := string(body)
	rest := stencil.StripLeadingComment(text)
	header := text[:len(text)-len(rest)]

	var readErr error
	expanded := bareIncludePattern.ReplaceAllStringFunc(rest, func(action string) string {
		if readErr != nil {
			return action
		}
		include := bareIncludePattern.FindStringSubmatch(action)[1]
		isEngineBlock := strings.HasPrefix(include, EngineBlockPrefix)
		if !isEngineBlock && !plainIncludes {
			return action
		}
		block, err := readInclude(baseDir, engine, stencilName, include)
		if errors.Is(err, os.ErrNotExist) && !isEngineBlock {
			err = fmt.Errorf("stencilstore: stencil %q includes stencil %q, which does not exist; way forward: run lym stencil diff, then lym stencil sync to seed the missing stencil: %w", stencilName, include, err)
		}
		if err != nil {
			readErr = err
			return action
		}
		return stencil.StripLeadingComment(string(block))
	})
	if readErr != nil {
		return nil, readErr
	}

	includes, err := stencil.IncludeNames([]byte(expanded))
	if err != nil {
		return nil, fmt.Errorf("stencilstore: stencil %q: %w", stencilName, err)
	}
	for _, include := range includes {
		if strings.HasPrefix(include, EngineBlockPrefix) {
			return nil, fmt.Errorf("stencilstore: stencil %q leaves engine block %q unresolved; way forward: write its include as the bare {{template %q}}, outside any engine block", stencilName, strings.TrimPrefix(include, EngineBlockPrefix), include)
		}
	}
	return []byte(header + expanded), nil
}

// readInclude reads the block that include names, for the stencil called stencilName.
// An engine block is read from engine's copy, without its file's final newline,
// so a block can stand inside a line as well as on lines of its own.
// Any other include is read as the stencil of its name, with the engine blocks it includes written in place.
func readInclude(baseDir, engine, stencilName, include string) ([]byte, error) {
	block, isEngineBlock := strings.CutPrefix(include, EngineBlockPrefix)
	if !isEngineBlock {
		content, err := Read(baseDir, include)
		if err != nil {
			return nil, err
		}
		return writeEngineBlocksInPlace(baseDir, engine, include, content, false)
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
	if err := refuseNewerBoardCopy(EngineBlockName(engine, block), content); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(content, []byte("\n")), nil
}
