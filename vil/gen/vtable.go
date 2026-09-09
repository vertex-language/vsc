package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
)

// Dispatch tables.
//
// A class's table is what makes `a.get()` reach B's body when a is a B
// held in an A. swiftc prints one per class:
//
//	sil_vtable A {
//	  #A.get: (A) -> () -> Int32 : @$s2p61AC3gets5Int32VyF
//	}
//	sil_vtable B {
//	  #A.get: (A) -> () -> Int32 : @$s2p61BC3gets5Int32VyF [override]
//	}
//
// Two things in that output are the whole design. A slot is named for
// the class that *introduced* the method -- B's table says #A.get, not
// #B.get -- so a call site that knows only the static type A can name
// the slot it wants. And B's table repeats A's slots in A's order,
// with the overridden ones pointing elsewhere, so the same slot is at
// the same index in every table down the chain. That is what lets a
// call load a table it has never seen and index it correctly.
//
// Slots are keyed by name and signature rather than by name alone: two
// methods that differ only in their parameters are two methods, and
// merging them would make an overload silently dispatch to its
// neighbour.

// vtables emits one dispatch table per class declared in the module.
//
// Emitted for every class, not only the ones with a superclass, the
// way swiftc does: whether a table is ever loaded is a property of the
// call, and a class with no subclasses today may be a base tomorrow.
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
			t := g.m.VTable(cl.Name)
			for _, s := range g.slots(cl) {
				t.Entry(s.member, s.impl)
			}
		}
	}
}

// A slot is one row of a dispatch table: the method as the class that
// introduced it names it, and the symbol that implements it here.
type slot struct {
	member string
	impl   string
}

// slots is cl's table, in index order.
//
// The chain is walked base first so that an inherited slot keeps the
// index it had in the superclass. A method whose name and signature
// match one already in the table is an override and replaces that
// row's implementation, keeping the row where it is and keeping the
// name the base gave it.
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

// classChain is cl and its superclasses, the base first.
//
// A cycle would be a checker error rather than something to loop
// forever on, so the walk stops at a class it has already seen.
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
