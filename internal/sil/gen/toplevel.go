package gen

import (
	"path/filepath"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
)

// Top-level code lowering for main.swift statements into the entry point.

// scriptFile is the file whose top-level statements are the program, or
// nil where no file has any.
func scriptFile(module string, files []*ast.File) *ast.File {
	if module != EntryModule {
		return nil
	}
	for _, f := range files {
		if !hasTopLevelCode(f) || f.Unit == nil {
			continue
		}
		if len(files) == 1 || filepath.Base(f.Unit.Name()) == "main.swift" {
			return f
		}
	}
	return nil
}

// hasTopLevelCode reports whether a file has a statement that is not a
// declaration.
func hasTopLevelCode(f *ast.File) bool {
	for _, st := range f.Stmts {
		switch st.(type) {
		case *ast.DeclStmt, *ast.IfConfigStmt, *ast.EmptyStmt:
			continue
		}
		return true
	}
	return false
}

// topLevelMain emits the entry point from main.swift's statements.
func (g *gen) topLevelMain(stmts []ast.Stmt) {
	f := g.m.Func(EntryName).SetSourceName(EntryName).
		SetLinkage(sil.Public).SetAttr("ossa")
	g.fn = f
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	g.self, g.recv = nil, nil
	g.entry = true
	g.push()
	g.blk = f.Entry()
	entrySignature(f)
	for _, st := range stmts {
		if g.blk == nil || g.blk.Term() != nil {
			break
		}
		g.stmt(st)
	}
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		g.blk.Return(g.result())
	}
	g.fn = nil
	g.entry = false
}
