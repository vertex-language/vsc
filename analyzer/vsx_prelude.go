package analyzer

import (
	_ "embed"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

//go:embed vsx_prelude.vs
var vsxPreludeSrc []byte

// loadVSXPrelude injects the built-in .vsx v2 prelude into a module that
// contains .vsx files and does not import an external ui/component module.
func (c *checker) loadVSXPrelude(files []*ast.File, pkgScope *Scope) []*ast.File {
	hasVSX := false
	for _, f := range files {
		if f.Unit != nil && strings.HasSuffix(f.Unit.Name(), ".vsx") {
			hasVSX = true
			break
		}
	}
	if !hasVSX {
		return files
	}
	if c.modules["ui"] == nil {
		c.modules["ui"] = pkgScope
	}
	if c.modules["dom"] == nil {
		c.modules["dom"] = pkgScope
	}
	declared := map[string]bool{}
	for _, f := range files {
		if f.Unit == nil {
			continue
		}
		for _, n := range topDeclNames(f.Stmts, f.Unit) {
			declared[n] = true
		}
	}
	if c.modules["component"] != nil {
		if !declared["Element"] {
			tf := token.NewFile("<vsx_element.vs>", []byte("typealias Element = component.Node\n"))
			if parsed, diags := parser.ParseFile(tf, 0); parsed != nil && len(diags) == 0 {
				parsed.Unit = tf
				c.info.VSXPrelude = parsed
				return append(files[:len(files):len(files)], parsed)
			}
		}
		return files
	}
	if declared["component"] {
		return files
	}

	tf := token.NewFile("<vsx_prelude.vs>", vsxPreludeSrc)
	parsed, diags := parser.ParseFile(tf, 0)
	if parsed == nil || len(diags) > 0 {
		return files
	}
	parsed.Unit = tf

	var kept []ast.Stmt
	for _, st := range parsed.Stmts {
		ds, ok := st.(*ast.DeclStmt)
		if !ok {
			kept = append(kept, st)
			continue
		}
		if ext, isExt := ds.D.(*ast.ExtensionDecl); isExt {
			_ = ext
			if declared["Renderable"] {
				continue
			}
			kept = append(kept, st)
			continue
		}
		names := declNames(ds.D, tf)
		conflict := false
		for _, n := range names {
			if declared[n] {
				conflict = true
				break
			}
		}
		if !conflict {
			kept = append(kept, st)
		}
	}
	parsed.Stmts = kept
	c.info.VSXPrelude = parsed
	return append(files[:len(files):len(files)], parsed)
}

func topDeclNames(stmts []ast.Stmt, unit *token.File) []string {
	var out []string
	for _, s := range stmts {
		if ds, ok := s.(*ast.DeclStmt); ok {
			out = append(out, declNames(ds.D, unit)...)
		}
	}
	return out
}

func declNames(d ast.Decl, unit *token.File) []string {
	switch d := d.(type) {
	case *ast.StructDecl:
		if d.Name != nil {
			return []string{d.Name.Text(unit)}
		}
	case *ast.ClassDecl:
		if d.Name != nil {
			return []string{d.Name.Text(unit)}
		}
	case *ast.EnumDecl:
		if d.Name != nil {
			return []string{d.Name.Text(unit)}
		}
	case *ast.ProtocolDecl:
		if d.Name != nil {
			return []string{d.Name.Text(unit)}
		}
	case *ast.TypealiasDecl:
		if d.Name != nil {
			return []string{d.Name.Text(unit)}
		}
	case *ast.FuncDecl:
		if d.Name != nil && d.Recv == nil {
			return []string{d.Name.Text(unit)}
		}
	case *ast.VarDecl:
		var out []string
		for _, b := range d.Bindings {
			pat := b.Pat
			if tp, ok := pat.(*ast.TypedPattern); ok {
				pat = tp.Pat
			}
			if id, ok := pat.(*ast.IdentPattern); ok && id.Name != nil {
				out = append(out, id.Name.Text(unit))
			}
		}
		return out
	}
	return nil
}
