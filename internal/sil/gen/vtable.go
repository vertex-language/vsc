package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
)

// Class dispatch table (sil_vtable) generation.

// vtables emits one dispatch table per class declared in the module.
func (g *gen) vtables(files []*ast.File) {
	for _, f := range files {
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			var name *ast.Ident
			switch d := decl.D.(type) {
			case *ast.ClassDecl:
				name = d.Name
			case *ast.ActorDecl:
				name = d.Name
			default:
				continue
			}
			if name == nil {
				continue
			}
			sym, _ := g.info.Defs[name].(*analyzer.TypeNameSymbol)
			if sym == nil {
				continue
			}
			cl, ok := sym.Type().Underlying().(*types.Class)
			if !ok {
				continue
			}
			// A generic class has a table for each instance it is made
			// as, emitted with the initializer that makes it.
			if len(cl.TypeParams) > 0 {
				continue
			}
			t := g.m.VTable(cl.Name)
			t.Layout = sym.Type()
			if declaresDeinit(decl.D) {
				t.Deinit = deinitSymbol(g.module, sym.Type())
			}
			// Every class has metadata, so that an instance can say what
			// its dynamic type is whatever static type it is held as.
			if len(cl.TypeParams) == 0 {
				g.needMetadata(name, sym.Type())
			}
			for _, s := range g.slots(cl) {
				t.Entry(s.member, s.impl)
			}
		}
	}
}

// A slot is one row of a dispatch table.
type slot struct {
	member string
	impl   string
}

// slots returns the dispatch table slots for cl in inheritance order.
func (g *gen) slots(cl *types.Class) []slot {
	var out []slot
	at := map[string]int{}
	for _, c := range classChain(cl) {
		for _, m := range c.Methods {
			if m == nil || m.Sig == nil {
				continue
			}
			impl := g.methodSymbol(&analyzer.MethodRef{Recv: c, Method: m})
			if impl == "" {
				continue
			}
			key := m.Name + m.Sig.String()
			if i, ok := at[key]; ok {
				out[i].impl = impl
				continue
			}
			at[key] = len(out)
			out = append(out, slot{member: c.Name + "." + m.Name, impl: impl})
		}
	}
	return out
}

// classChain returns the inheritance chain from root base class to cl.
func classChain(cl *types.Class) []*types.Class {
	var chain []*types.Class
	seen := map[*types.Class]bool{}
	for c := cl; c != nil && !seen[c]; {
		seen[c] = true
		chain = append([]*types.Class{c}, chain...)
		next, _ := c.Superclass.(*types.Class)
		if next == nil && c.Superclass != nil {
			next, _ = c.Superclass.Underlying().(*types.Class)
		}
		c = next
	}
	return chain
}

// slotIndex is where a member sits in cl's table, for a call that
// knows the member and the static class.
func slotIndex(slots []slot, member string) (int, bool) {
	for i, s := range slots {
		if s.member == member {
			return i, true
		}
	}
	return 0, false
}

// declaresDeinit reports whether a class declaration has a deinit.
func declaresDeinit(d ast.Decl) bool {
	var body *ast.MemberBlock
	switch c := d.(type) {
	case *ast.ClassDecl:
		body = c.Body
	case *ast.ActorDecl:
		body = c.Body
	}
	if body == nil {
		return false
	}
	for _, m := range body.Members {
		if _, ok := m.(*ast.DeinitDecl); ok {
			return true
		}
	}
	return false
}
