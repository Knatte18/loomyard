// blockkey.go keys coverage blocks by their content, so a block keeps its key when lines above it move.

package testcover

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// PackageLevelFunc is the function a block key names for a block outside every function declaration.
const PackageLevelFunc = "(package)"

// BlockKey identifies a coverage block by its content rather than by its line and column.
type BlockKey struct {
	// File is the block's file name within its package directory.
	File string `json:"file"`
	// Func is the enclosing function, `Type.Method` for a method, or PackageLevelFunc.
	Func string `json:"func"`
	// Hash is the ContentHash of the block's source text.
	Hash string `json:"hash"`
	// Ordinal counts the blocks before this one, in source order, whose file, function and hash equal its own.
	Ordinal int `json:"ordinal"`
}

// ContentHash returns the hex SHA-256 of data.
// It is the hash a block key, and a coverage map's file and test entries, carry.
func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// KeyBlocks returns the content key of each block, in the order given.
// Every block must lie in a Go file of the package directory dir, which it reads and parses.
// Ordinals count among the blocks given, so a stable key needs every block of the package's profile, covered or not.
// A file that does not parse, or a position outside its file, is an error.
func KeyBlocks(dir string, blocks []ProfileBlock) ([]BlockKey, error) {
	type located struct {
		index int
		start int
	}
	files := map[string]*sourceFile{}
	keys := make([]BlockKey, len(blocks))
	byFile := map[string][]located{}
	for i, b := range blocks {
		name := path.Base(b.File)
		src, ok := files[name]
		if !ok {
			var err error
			if src, err = parseSourceFile(filepath.Join(dir, name)); err != nil {
				return nil, err
			}
			files[name] = src
		}
		start, err := src.offset(b.StartLine, b.StartCol)
		if err != nil {
			return nil, fmt.Errorf("block %s: %w", b.Position(), err)
		}
		end, err := src.offset(b.EndLine, b.EndCol)
		if err != nil {
			return nil, fmt.Errorf("block %s: %w", b.Position(), err)
		}
		if end < start {
			return nil, fmt.Errorf("block %s ends before it starts", b.Position())
		}
		keys[i] = BlockKey{File: name, Func: src.enclosingFunc(start), Hash: ContentHash(src.text[start:end])}
		byFile[name] = append(byFile[name], located{index: i, start: start})
	}

	for _, blocksOfFile := range byFile {
		sort.SliceStable(blocksOfFile, func(a, b int) bool { return blocksOfFile[a].start < blocksOfFile[b].start })
		seen := map[BlockKey]int{}
		for _, l := range blocksOfFile {
			base := keys[l.index]
			keys[l.index].Ordinal = seen[base]
			seen[base]++
		}
	}
	return keys, nil
}

// sourceFile is one parsed Go file with the offsets of its function declarations.
type sourceFile struct {
	text  []byte
	file  *token.File
	funcs []funcSpan
}

// funcSpan is the byte range of one function declaration and the name a block key gives it.
type funcSpan struct {
	start, end int
	name       string
}

func parseSourceFile(filename string) (*sourceFile, error) {
	text, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, filename, text, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	src := &sourceFile{text: text, file: fset.File(parsed.Pos())}
	for _, decl := range parsed.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			src.funcs = append(src.funcs, funcSpan{
				start: src.file.Offset(fn.Pos()),
				end:   src.file.Offset(fn.End()),
				name:  funcKeyName(fn),
			})
		}
	}
	return src, nil
}

// offset returns the byte offset of a profile position: a line and a byte column, both counted from 1.
func (src *sourceFile) offset(line, col int) (int, error) {
	if line < 1 || line > src.file.LineCount() {
		return 0, fmt.Errorf("line %d is outside the file's %d lines", line, src.file.LineCount())
	}
	offset := src.file.Offset(src.file.LineStart(line)) + col - 1
	if col < 1 || offset > len(src.text) {
		return 0, fmt.Errorf("column %d of line %d is outside the file", col, line)
	}
	return offset, nil
}

// enclosingFunc names the function declaration holding the byte at offset, or PackageLevelFunc.
func (src *sourceFile) enclosingFunc(offset int) string {
	for _, fn := range src.funcs {
		if fn.start <= offset && offset < fn.end {
			return fn.name
		}
	}
	return PackageLevelFunc
}

// funcKeyName spells a function's name for a block key: `Type.Method` for a method, the bare name otherwise.
func funcKeyName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// receiverTypeName strips the pointer, parentheses and type arguments from a receiver type.
func receiverTypeName(expr ast.Expr) string {
	for {
		switch x := expr.(type) {
		case *ast.StarExpr:
			expr = x.X
		case *ast.ParenExpr:
			expr = x.X
		case *ast.IndexExpr:
			expr = x.X
		case *ast.IndexListExpr:
			expr = x.X
		case *ast.Ident:
			return x.Name
		default:
			return types.ExprString(expr)
		}
	}
}
