package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/vil"
)

// Statements, and the scopes they open.
//
// A block of statements is a scope: what it declares is destroyed
// where it ends, and a return unwinds every scope between it and the
// function's edge. That is the whole of lifetime management in raw
// VIL, and it is why the output verifies.

// block lowers a code block in a scope of its own.
func (g *gen) block(b *ast.CodeBlock) {
	if b == nil {
		return
	}
	g.push()
	for _, s := range b.Stmts {
		g.stmt(s)
		if g.blk == nil || g.blk.Term() != nil {
			break // the rest of the block is unreachable
		}
	}
	if g.blk != nil && g.blk.Term() == nil {
		g.pop()
		return
	}
	// The block ended in a return, which already unwound.
	g.scopes = g.scopes[:len(g.scopes)-1]
}

func (g *gen) stmt(s ast.Stmt) {
	switch n := s.(type) {
	case *ast.DeclStmt:
		if d, ok := n.D.(*ast.VarDecl); ok {
			g.varDecl(d)
		}

	case *ast.ExprStmt:
		g.exprStmt(n.X)

	case *ast.ReturnStmt:
		g.ret(n)

	case *ast.IfStmt:
		g.ifStmt(n)

	case *ast.CodeBlock:
		g.block(n)
	}
}

// exprStmt lowers an expression written as a statement. An
// assignment is the one that does something rather than produces
// something, and it is written as an operator, so it is recognised
// here rather than among the expressions.
func (g *gen) exprStmt(e ast.Expr) {
	if seq, ok := e.(*ast.SequenceExpr); ok {
		if folded, ok := g.info.Folded[seq]; ok {
			e = folded
		}
	}
	if bin, ok := e.(*ast.BinaryExpr); ok && g.text(bin.Op) == "=" {
		g.assign(bin)
		return
	}
	if v := g.expr(e); v != nil {
		// A value computed and used for nothing is destroyed where
		// the statement's scope ends.
		g.destroyLater(v)
	}
}

// assign lowers a store to a variable. In raw VIL it is `assign`
// rather than `store`: whether the destination already held something
// is what definite initialization decides, and it has not run yet.
func (g *gen) assign(e *ast.BinaryExpr) {
	addr := g.lvalue(e.X)
	if addr == nil {
		return
	}
	v := g.rvalue(e.Y)
	if v == nil {
		return
	}
	access := g.blk.BeginAccess(addr, "modify", "unknown")
	g.blk.Assign(v, access)
	g.blk.EndAccess(access)
}

// lvalue is an expression lowered as somewhere to write: the address
// a store goes to.
func (g *gen) lvalue(e ast.Expr) *vil.Value {
	switch n := e.(type) {
	case *ast.ParenExpr:
		return g.lvalue(n.X)

	case *ast.IdentExpr:
		if sym := g.info.Uses[n.Name]; sym != nil {
			if l := g.locals[sym]; l != nil {
				return l.addr
			}
		}

	case *ast.MemberExpr:
		base := g.expr(n.X)
		if base == nil || n.Name == nil {
			return nil
		}
		t := lowerType(g.info.Types[n])
		name := memberName(g.info.Types[n.X], n.Name.Text(g.file))
		if isClass(g.info.Types[n.X]) {
			return g.blk.RefElementAddr(base, name, t)
		}
	}
	return nil
}

// text is a node's spelling.
func (g *gen) text(n ast.Node) string {
	if n == nil {
		return ""
	}
	return string(g.file.Slice(n.Pos(), n.End()))
}

// varDecl lowers a `let` or a `var`.
//
// A let is a value: copied if it binds something owned elsewhere,
// moved to mark where the binding begins, and destroyed where its
// scope ends. A var is a box, because a variable can be written to
// and an SSA value cannot: allocated, borrowed for the variable's
// lifetime, and projected to get the address its stores go to.
func (g *gen) varDecl(d *ast.VarDecl) {
	for _, b := range d.Bindings {
		name, sym := g.binding(b)
		if sym == nil {
			continue
		}
		t := lowerType(sym.Type())

		if d.Kind == token.LET {
			v := g.rvalue(b.Value)
			if v == nil {
				continue
			}
			bound := g.blk.MoveValue(v, "lexical", "var_decl")
			g.blk.DebugValue(bound, name, "let")
			g.destroyLater(bound)
			g.locals[sym] = &local{value: bound, typ: t}
			continue
		}

		box := g.blk.AllocBox(t, name, "var")
		borrow := g.blk.BeginBorrow(box, "var_decl")
		addr := g.blk.ProjectBox(borrow, 0, t)
		g.locals[sym] = &local{addr: addr, box: box, typ: t}

		// The box is released and its borrow closed where the scope
		// ends, in that order.
		g.destroyLater(box)
		g.endBorrowLater(borrow)

		if v := g.rvalue(b.Value); v != nil {
			g.blk.Store(v, addr, storeQualifier(t))
		}
	}
}

// binding is a binding's name and the symbol it declares.
func (g *gen) binding(b *ast.PatternBinding) (string, analyzer.Symbol) {
	pat := b.Pat
	if tp, ok := pat.(*ast.TypedPattern); ok {
		pat = tp.Pat
	}
	id, ok := pat.(*ast.IdentPattern)
	if !ok || id.Name == nil {
		return "", nil
	}
	return id.Name.Text(g.file), g.info.Defs[id.Name]
}

// storeQualifier says what a store does to the ownership of what was
// there: nothing, for a type that owns nothing.
func storeQualifier(t vil.Type) string {
	if t.Trivial() {
		return "trivial"
	}
	return "init"
}

// ret lowers a return: the value first, then every open scope's
// cleanups, then the terminator. The order matters — a value computed
// inside a scope must be produced before the scope is torn down, and
// must not be destroyed by it.
func (g *gen) ret(s *ast.ReturnStmt) {
	var v *vil.Value
	if s.X != nil {
		v = g.rvalue(s.X)
	}
	g.unwind()
	if v == nil {
		v = g.void()
	}
	g.blk.Return(v)
}

// ifStmt lowers a branch. Both arms join at a block that continues
// the function, unless both of them returned, in which case there is
// nothing to join.
func (g *gen) ifStmt(s *ast.IfStmt) {
	cond := g.condition(s.Conds)
	if cond == nil {
		return
	}

	thenBlk := g.fn.Block()
	elseBlk := g.fn.Block()
	g.blk.CondBr(cond, thenBlk, nil, elseBlk, nil)

	g.blk = thenBlk
	g.block(s.Body)
	thenOpen := g.blk != nil && g.blk.Term() == nil
	thenEnd := g.blk

	g.blk = elseBlk
	if s.Else != nil {
		g.stmt(s.Else)
	}
	elseOpen := g.blk != nil && g.blk.Term() == nil
	elseEnd := g.blk

	switch {
	case !thenOpen && !elseOpen:
		// Both arms left; nothing follows the branch.
		g.blk = elseEnd
	default:
		join := g.fn.Block()
		if thenOpen {
			thenEnd.Br(join)
		}
		if elseOpen {
			elseEnd.Br(join)
		}
		g.blk = join
	}
}

// condition lowers an if's condition list. Only a plain boolean
// expression is lowered so far; a binding condition needs optionals,
// which need a library.
func (g *gen) condition(conds []ast.Node) *vil.Value {
	for _, c := range conds {
		if e, ok := c.(ast.Expr); ok {
			return g.expr(e)
		}
	}
	return nil
}
