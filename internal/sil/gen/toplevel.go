package gen

import (
	"path/filepath"
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
)

// Top-level code lowering for main.swift statements into the entry point.

// scriptFile is the file whose top-level statements are the program, or
// nil where the program starts at `func main()` instead.
//
// Swift's rule: the program is main.swift, or the only file there is --
// an empty one included, which is a program that does nothing. (A .vs
// file has to have top-level code to be one; see below.) Files the
// compiler wrote itself (derived conformances) are not the program's and
// are not counted. The one exception is this compiler's own entry
// point: a file of declarations alone that declares `func main()` starts
// there, as it always has.
func scriptFile(module string, files []*ast.File) *ast.File {
	if module != EntryModule {
		return nil
	}
	var written []*ast.File
	for _, f := range files {
		if f.Unit != nil && !strings.HasPrefix(f.Unit.Name(), "<") {
			written = append(written, f)
		}
	}
	var script *ast.File
	for _, f := range written {
		if filepath.Base(f.Unit.Name()) == "main.swift" {
			script = f
			break
		}
	}
	if script == nil && len(written) == 1 {
		script = written[0]
	}
	if script == nil || hasTopLevelCode(script) {
		return script
	}
	// Vertex's rule is its own: a .vs program starts at `func main()`,
	// and one with neither that nor top-level code has no entry point.
	if !strings.HasSuffix(script.Unit.Name(), ".swift") {
		return nil
	}
	for _, f := range written {
		if declaresEntry(f) {
			return nil
		}
	}
	return script
}

// declaresEntry reports whether a file declares the module-level
// `func main()` this compiler starts a program at.
func declaresEntry(f *ast.File) bool {
	for _, st := range f.Stmts {
		d, ok := st.(*ast.DeclStmt)
		if !ok {
			continue
		}
		if fn, ok := d.D.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name != nil &&
			fn.Name.Text(f.Unit) == EntryName {
			return true
		}
	}
	return false
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
	// Code that awaits is an async main's body, run as the first task.
	async := g.info.AsyncTopLevel
	name := EntryName
	if async {
		name = asyncEntryName
	}
	f := g.m.Func(name).SetSourceName(EntryName).
		SetLinkage(sil.Public).SetAttr("ossa")
	g.fn = f
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	g.self, g.recv = nil, nil
	g.entry = !async
	g.topLevel = f
	defer func() { g.topLevel = nil }()
	g.push()
	g.blk = f.Entry()
	if async {
		f.SetLinkage(sil.Hidden)
		f.Type().Convention = sil.Thin
		f.Type().Async = true
	} else {
		entrySignature(f)
	}
	for _, st := range stmts {
		if g.blk == nil || g.blk.Term() != nil {
			break
		}
		if decl, ok := st.(*ast.DeclStmt); ok {
			if d, ok := decl.D.(*ast.VarDecl); ok {
				g.scriptVarDecl(d)
				continue
			}
		}
		g.stmt(st)
	}
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		g.blk.Return(g.result())
	}
	g.fn = nil
	g.entry = false
	if async {
		g.asyncEntry(EntryName, f)
	}
}
