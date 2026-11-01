package vsc

import (
	"path/filepath"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// The packages a .vsx file uses without importing them: what its markup
// lowers to (ui/component), the events its handlers are given (web/dom),
// and the box other things draw into (ui/viewport, <Viewport>). They are imported for it wherever they can be found from
// its folder; where they cannot -- a program with no ui to build on --
// the compiler's own prelude stands in (analyzer/vsx_prelude.vs).
var vsxImplicitImports = []string{"ui/component", "web/dom", "ui/viewport"}

// addVSXImports gives each .vsx file among files the implicit imports it
// does not write itself and that resolve from its folder.
func addVSXImports(files []*ast.File, units []*token.File, resolves func(path, dir string) bool) {
	for i, f := range files {
		unit := f.Unit
		if i < len(units) && units[i] != nil {
			unit = units[i]
		}
		if f == nil || unit == nil || !strings.HasSuffix(unit.Name(), MarkupExtension) {
			continue
		}
		have := map[string]bool{}
		for _, p := range importPaths([]*ast.File{f}, []*token.File{unit}) {
			have[p] = true
		}
		dir := filepath.Dir(unit.Name())
		var paths []*ast.ImportPath
		for _, p := range vsxImplicitImports {
			if have[p] || !resolves(p, dir) {
				continue
			}
			lit := &ast.StringLit{Segments: []ast.Node{&ast.StringText{Synthesized: true, Synth: p}}}
			paths = append(paths, &ast.ImportPath{Path: lit})
		}
		if len(paths) == 0 {
			continue
		}
		decl := &ast.DeclStmt{D: &ast.ImportDecl{Paths: paths}}
		// After the package clause, where imports are written.
		at := 0
		for j, s := range f.Stmts {
			if ds, ok := s.(*ast.DeclStmt); ok {
				if _, pkg := ds.D.(*ast.PackageDecl); pkg {
					at = j + 1
				}
			}
		}
		f.Stmts = append(f.Stmts[:at:at], append([]ast.Stmt{decl}, f.Stmts[at:]...)...)
	}
}

// ImportPathText is the path a string-form import names, written or
// supplied by the compiler.
func ImportPathText(lit *ast.StringLit, unit *token.File) (string, bool) {
	return importPathText(lit, unit)
}
