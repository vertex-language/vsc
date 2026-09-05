package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
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

// stmt lowers one statement inside a formal scope of its own, so
// that the borrows its expressions opened close with it.
func (g *gen) stmt(s ast.Stmt) {
	g.pushFormal()
	g.stmtBody(s)
	if g.blk != nil && g.blk.Term() == nil {
		g.pop()
		return
	}
	// The statement left the block — a return has already unwound
	// every scope, this one included.
	g.scopes = g.scopes[:len(g.scopes)-1]
}

func (g *gen) stmtBody(s ast.Stmt) {
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

	case *ast.WhileStmt:
		g.whileStmt(n)

	case *ast.SwitchStmt:
		g.switchStmt(n)

	case *ast.RepeatWhileStmt:
		g.repeatStmt(n)

	case *ast.GuardStmt:
		g.guardStmt(n)

	case *ast.LabeledStmt:
		g.labeled(n)

	case *ast.BreakStmt:
		g.breakStmt(n)

	case *ast.ContinueStmt:
		g.continueStmt(n)

	case *ast.CodeBlock:
		g.block(n)

	case *ast.EmptyStmt:
		// A bare `;`. Nothing to lower, and nothing wrong with it.

	case *ast.BadStmt:
		// The parser already reported why. Compile stops before this
		// package on a parse error, so reaching here means a caller
		// drove the generator directly; either way, saying it again
		// helps nobody.

	default:
		// Everything this package does not lower yet, said out loud.
		// A statement that falls through here silently is a statement
		// that does not happen, in a program that compiles.
		g.unsupportedStmt(s)
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
	// Counted so that a destination which already said why it could not
	// be written to is not told off a second time in more general
	// terms. Two diagnostics for one mistake sends the reader looking
	// for two.
	said := len(g.diags)
	addr := g.lvalue(e.X)
	if addr == nil {
		// An assignment whose destination could not be lowered is an
		// assignment that does not happen, in a program that compiles
		// and runs. Whatever it was meant to change keeps its old
		// value and the answer is quietly wrong.
		if len(g.diags) == said {
			g.refuse(e.X, "an assignment to "+g.exprKind(e.X))
		}
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
		// Inside a method a bare name may be a stored property, and
		// writing to it writes through the receiver: `n = …` there is
		// `self.n = …`.
		return g.implicitSelfAddr(n)

	case *ast.MemberExpr:
		if n.Name == nil {
			return nil
		}
		t := lowerType(g.info.Types[n])
		name := memberName(g.info.Types[n.X], n.Name.Text(g.file))

		// A class's property is inside the object the reference
		// points at, so the base is a value and the field is
		// arithmetic on it.
		if isClass(g.info.Types[n.X]) {
			base := g.expr(n.X)
			if base == nil {
				return nil
			}
			return g.blk.RefElementAddr(base, name, t)
		}

		// A struct's property is inside the struct's own storage, so
		// the base has to be an address as well — `p.y = …` writes
		// into where p lives, and reading p out into a value first
		// would write into the copy.
		addr := g.lvalue(n.X)
		if addr == nil {
			return nil
		}
		return g.blk.StructElementAddr(addr, name, t)
	}
	return nil
}

// implicitSelfAddr is where a stored property of the receiver lives, for
// a name in a method body being written to.
//
// A class's property is inside the object the reference names, so its
// address is arithmetic on the receiver. A struct's is inside the
// receiver's own storage — and a struct receiver arrives by value, so
// there is nothing to write into that the caller would see. Swift says
// the same by requiring `mutating` on such a method and giving it an
// inout self; neither is modelled, so this refuses rather than writing
// to a copy.
func (g *gen) implicitSelfAddr(e *ast.IdentExpr) *vil.Value {
	if g.recv == nil || e.Name == nil {
		return nil
	}
	name := g.text(e.Name)
	field, ok := storedField(g.recv, name)
	if !ok {
		return nil
	}
	self := g.selfValue()
	if self == nil {
		return nil
	}
	if !isClass(g.recv) {
		g.errorAt(e, "cannot assign to '"+name+"': the receiver is a value, and a "+
			"method that changes one has to be declared 'mutating'")
		return nil
	}
	return g.blk.RefElementAddr(self, memberName(g.recv, name), lowerType(field.Type))
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
	if s.X == nil {
		// `return` with nothing to return: the empty tuple, or the
		// exit status in the entry point, which says zero.
		g.unwind()
		g.blk.Return(g.result())
		return
	}
	v := g.rvalue(s.X)
	g.unwind()
	if v == nil {
		// The expression was there and could not be lowered, and
		// whatever refused it has already said so. What must not
		// happen is returning something else in its place: the
		// substitute type-checks, the module verifies, and the
		// program runs and gives the wrong answer. `unreachable`
		// terminates the block without inventing a value, and the
		// diagnostic already stopped the compilation.
		g.blk.Unreachable()
		return
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

// condition lowers an if's condition list to the bit a branch tests.
//
// A Bool is a struct around a Builtin.Int1 and cond_br takes the bit,
// so the condition is reached through exactly as an operand of an
// operator is.
//
// Only a plain boolean expression is lowered so far; a binding
// condition needs optionals, which need a library.
func (g *gen) condition(conds []ast.Node) *vil.Value {
	for _, c := range conds {
		e, ok := c.(ast.Expr)
		if !ok {
			// A binding condition, an availability check, a `case`
			// pattern. Each is a condition this package does not
			// lower, and each has to say so: a condition that
			// silently produced nothing took its whole statement with
			// it, body and all.
			g.refuse(c, conditionKind(c))
			return nil
		}
		v := g.expr(e)
		if v == nil {
			return nil
		}
		return g.machine(v, g.info.Types[e])
	}
	g.refuse(nil, "an empty condition")
	return nil
}

// conditionKind names a condition the way a person would say it.
func conditionKind(c ast.Node) string {
	switch c.(type) {
	case *ast.OptionalBinding:
		return "a binding condition"
	case *ast.AvailabilityCond:
		return "an availability condition"
	case *ast.CaseCond:
		return "a case condition"
	}
	return "this condition"
}

// whileStmt lowers a while loop to the three blocks SILGen gives it:
// a header that tests, a body that branches back to it, and the block
// after.
//
//	bb0: ... br header
//	header: <condition> cond_br %c, body, exit
//	body: ... br header
//	exit: ...
//
// The condition is emitted into the header rather than before it,
// because it is tested once per iteration and not once.
//
// A borrow opened by the condition would want ending on each pass
// through the header, and this does not arrange that — the formal
// scope around the statement closes after the loop, which is the
// wrong place. It is not reachable yet: the only conditions that
// lower are comparisons of trivial types, which own nothing and leave
// nothing to close. A condition that owns something needs the scope
// handling before it needs anything else here.
func (g *gen) whileStmt(s *ast.WhileStmt) {
	header := g.fn.Block()
	body := g.fn.Block()
	exit := g.fn.Block()

	label := g.takeLabel()

	g.blk.Br(header)

	g.blk = header
	cond := g.condition(s.Conds)
	if cond == nil {
		// Reported. The block is left open and the compilation stops
		// on the diagnostic; inventing a branch here would only
		// decide which way an untestable condition went.
		return
	}
	g.blk.CondBr(cond, body, nil, exit, nil)

	g.blk = body
	g.loops = append(g.loops, loop{header: header, exit: exit, depth: len(g.scopes), label: label})
	g.block(s.Body)
	g.loops = g.loops[:len(g.loops)-1]
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Br(header)
	}

	g.blk = exit
}

// switchStmt lowers a switch.
//
// Two shapes, and SILGen picks between them the same way. A subject that
// is an enum branches on the tag:
//
//	switch_enum %e, case #E.a: bb1, case #E.b: bb2, default: bb3
//
// Anything else is a chain: each pattern is compared with the subject
// and a failed test falls into the next one, which is what Swift's
// pattern match on a literal is — `~=` over Equatable, and not a jump
// table.
//
//	%c = <subject == first>   cond_br %c, body1, test2
//	test2: …
//
// A case body does not fall into the next: Swift breaks implicitly at
// the end of one, so a body that runs off its end goes to the
// continuation. `break` inside a switch goes there too, and `continue`
// past it to the enclosing loop — which is why the entry pushed here
// keeps whatever header was already on the stack.
func (g *gen) switchStmt(s *ast.SwitchStmt) {
	subject := g.rvalue(s.Subject)
	if subject == nil {
		return
	}
	clauses := caseClauses(s)
	if len(clauses) == 0 {
		return
	}

	// The continuation is made on demand rather than up front: where
	// every case returns, nothing branches past the switch, and a
	// block with no predecessors is not a well formed one. Everything
	// that would branch there goes through cont(), so the block exists
	// exactly when something reaches it.
	var contBlk *vil.Block
	cont := func() *vil.Block {
		if contBlk == nil {
			contBlk = g.fn.Block()
		}
		return contBlk
	}

	bodies := make([]*vil.Block, len(clauses))
	for i := range clauses {
		bodies[i] = g.fn.Block()
	}

	subjectType := g.info.Types[s.Subject]
	if _, isEnum := underlyingEnum(subjectType); isEnum {
		if !g.switchOnEnum(s, subject, subjectType, clauses, bodies, cont) {
			return
		}
	} else if !g.switchOnValue(s, subject, subjectType, clauses, bodies, cont) {
		return
	}

	// The bodies, each ending at the continuation unless it left by
	// itself.
	header := (*vil.Block)(nil)
	if l, ok := g.enclosing(""); ok {
		header = l.header
	}
	for i, cs := range clauses {
		g.blk = bodies[i]
		g.loops = append(g.loops, loop{header: header, lazyExit: cont, depth: len(g.scopes)})
		for _, st := range cs.Stmts {
			g.stmt(st)
			if g.blk == nil || g.blk.Term() != nil {
				break
			}
		}
		g.loops = g.loops[:len(g.loops)-1]
		if g.blk != nil && g.blk.Term() == nil {
			g.blk.Br(cont())
		}
	}
	// Nil rather than an empty block when nothing reached the end:
	// what follows the switch is unreachable, and the callers already
	// read a nil block as saying so.
	g.blk = contBlk
}

// switchOnEnum emits the branch for an enum subject, and reports
// whether it could.
func (g *gen) switchOnEnum(s *ast.SwitchStmt, subject *vil.Value, t types.Type,
	clauses []*ast.CaseClause, bodies []*vil.Block, cont func() *vil.Block) bool {

	var cases []vil.Case
	seenDefault := false
	for i, cs := range clauses {
		if cs.Kind == token.DEFAULT {
			cases = append(cases, vil.Case{Dest: bodies[i]})
			seenDefault = true
			continue
		}
		for _, item := range cs.Items {
			if item.Where != nil {
				g.refuse(s, "a switch with a `where` clause")
				return false
			}
			pat, ok := item.Pat.(*ast.EnumCasePattern)
			if !ok || pat.Name == nil {
				g.refuse(item.Pat, "this pattern in a switch over an enum")
				return false
			}
			if pat.Args != nil {
				g.refuse(item.Pat, "a pattern that binds an enum case's value")
				return false
			}
			cases = append(cases, vil.Case{
				Member: memberName(t, g.text(pat.Name)),
				Dest:   bodies[i],
			})
		}
	}
	// The checker holds a switch over an enum to covering every case,
	// so a missing default means every case is named. Sending the
	// unnamed remainder to the continuation rather than leaving the
	// branch short is what keeps the block well formed either way.
	if !seenDefault {
		cases = append(cases, vil.Case{Dest: cont()})
	}
	g.blk.SwitchEnum(subject, cases...)
	return true
}

// switchOnValue emits the comparison chain for a subject that is not an
// enum, and reports whether it could.
func (g *gen) switchOnValue(s *ast.SwitchStmt, subject *vil.Value, t types.Type,
	clauses []*ast.CaseClause, bodies []*vil.Block, cont func() *vil.Block) bool {

	// The default is where a subject that matched nothing goes, and the
	// continuation is that when there is none.
	fallback := (*vil.Block)(nil)
	for i, cs := range clauses {
		if cs.Kind == token.DEFAULT {
			fallback = bodies[i]
		}
	}
	if fallback == nil {
		fallback = cont()
	}

	for i, cs := range clauses {
		if cs.Kind == token.DEFAULT {
			continue
		}
		for _, item := range cs.Items {
			if item.Where != nil {
				g.refuse(s, "a switch with a `where` clause")
				return false
			}
			pat, ok := item.Pat.(*ast.ExprPattern)
			if !ok {
				g.refuse(item.Pat, "this pattern in a switch")
				return false
			}
			n := len(g.diags)
			want := g.rvalue(pat.X)
			if want == nil {
				// Whatever stopped it said so, unless nothing did.
				if len(g.diags) == n {
					g.refuse(item.Pat, "this pattern in a switch")
				}
				return false
			}
			eq := g.equals(subject, want, t)
			if eq == nil {
				g.refuse(item.Pat, "a switch over "+t.String())
				return false
			}
			next := g.fn.Block()
			g.blk.CondBr(eq, bodies[i], nil, next, nil)
			g.blk = next
		}
	}
	g.blk.Br(fallback)
	return true
}

// equals is the bit that says whether two values of the same type are
// the same one, which is what a case pattern tests.
func (g *gen) equals(a, b *vil.Value, t types.Type) *vil.Value {
	bi, ok := core.Lower("==", t)
	if !ok {
		return nil
	}
	return g.blk.Builtin(bi.Name, vil.Object(builtinNamed(bi.Result)),
		g.machine(a, t), g.machine(b, t))
}

// caseClauses is the clauses of a switch, in order, skipping anything
// that is not one — a conditional case is an #if the parser kept.
func caseClauses(s *ast.SwitchStmt) []*ast.CaseClause {
	out := make([]*ast.CaseClause, 0, len(s.Cases))
	for _, c := range s.Cases {
		if cs, ok := c.(*ast.CaseClause); ok {
			out = append(out, cs)
		}
	}
	return out
}

// underlyingEnum is the enum a type is, if it is one.
func underlyingEnum(t types.Type) (*types.Enum, bool) {
	if t == nil {
		return nil, false
	}
	e, ok := t.Underlying().(*types.Enum)
	return e, ok
}

// repeatStmt lowers `repeat { … } while cond`, whose difference from
// a while loop is which end the test is at:
//
//	  br body
//	body: … br cond
//	cond: <condition> cond_br %c, body, exit
//	exit:
//
// The body runs before anything is tested, which is the whole point
// of the form. `continue` goes to the condition rather than to the
// body — the iteration is over, and what remains is deciding whether
// there is another — and SILGen agrees: its continue branches to the
// block holding the test.
func (g *gen) repeatStmt(s *ast.RepeatWhileStmt) {
	label := g.takeLabel()

	body := g.fn.Block()
	test := g.fn.Block()
	exit := g.fn.Block()

	g.blk.Br(body)

	g.blk = body
	g.loops = append(g.loops, loop{header: test, exit: exit, depth: len(g.scopes), label: label})
	g.block(s.Body)
	g.loops = g.loops[:len(g.loops)-1]
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Br(test)
	}

	g.blk = test
	cond := g.expr(s.Cond)
	if cond == nil {
		return
	}
	g.blk.CondBr(g.machine(cond, g.info.Types[s.Cond]), body, nil, exit, nil)

	g.blk = exit
}

// guardStmt lowers `guard cond else { … }`: an if whose arms are the
// other way round, and whose else may not fall through.
//
//	cond_br %c, cont, else
//	else: … (returns, breaks, continues, or throws)
//	cont: …
//
// The rule about falling through is the language's, and it is checked
// here because nothing before this models it. It is what makes a
// guard worth writing: everything after one may assume the condition
// held, and that assumption is only sound because the else cannot
// reach it.
func (g *gen) guardStmt(s *ast.GuardStmt) {
	cond := g.condition(s.Conds)
	if cond == nil {
		return
	}
	elseBlk := g.fn.Block()
	cont := g.fn.Block()
	g.blk.CondBr(cond, cont, nil, elseBlk, nil)

	g.blk = elseBlk
	g.block(s.Body)
	if g.blk != nil && g.blk.Term() == nil {
		g.errorAt(s, "the body of a 'guard' must not fall through: "+
			"it has to return, break, continue, or throw")
		// Terminated so the block is well formed for whatever reads
		// the module next. The diagnostic has already stopped the
		// compilation, and branching to cont would be asserting the
		// very thing that is wrong.
		g.blk.Unreachable()
	}

	g.blk = cont
}

// labeled lowers `name: while …`, which is the only thing a label is
// for here: break and continue naming which loop they mean.
//
// The label is held rather than emitted. Nothing in VIL carries it —
// SIL has no labels either, because by the time there are basic
// blocks a label is just which block a branch goes to.
func (g *gen) labeled(s *ast.LabeledStmt) {
	if s.Label == nil {
		g.stmtBody(s.Stmt)
		return
	}
	saved := g.pending
	g.pending = g.text(s.Label)
	g.stmtBody(s.Stmt)
	g.pending = saved
}

// takeLabel is the label the loop being lowered was written with, and
// clears it so that a loop nested inside it does not inherit one.
func (g *gen) takeLabel() string {
	label := g.pending
	g.pending = ""
	return label
}

// breakStmt leaves the loop: its scopes are unwound and control goes
// to the block after it.
//
// SILGen writes one more block than this does. Its loop exit branches
// to a continuation, and a break branches to the same continuation,
// so there are two blocks where this has one. The graph is the same
// either way — an empty block whose only instruction is a branch —
// and the extra one is an artifact of how SILGen emits cleanups
// rather than something the program says.
func (g *gen) breakStmt(s *ast.BreakStmt) {
	g.leave(s, "break", g.label(s.Label), func(l loop) *vil.Block { return l.exitBlock() })
}

// continueStmt goes back to the header, where the condition is tested
// again.
func (g *gen) continueStmt(s *ast.ContinueStmt) {
	g.leave(s, "continue", g.label(s.Label), func(l loop) *vil.Block { return l.header })
}

// leave is the half break and continue share: find the loop, run the
// cleanups of everything inside it, and branch.
func (g *gen) leave(s ast.Stmt, keyword, label string, target func(loop) *vil.Block) {
	l, ok := g.enclosing(label)
	if !ok {
		// The checker does not model loop nesting, so this is where a
		// break outside a loop, or one naming a label no enclosing
		// loop has, is caught. Saying which is worth the two cases: a
		// misspelled label and a misplaced break read very
		// differently to whoever wrote one.
		msg := "'" + keyword + "' is not inside a loop"
		if label != "" {
			msg = "no enclosing loop is labelled '" + label + "'"
		}
		g.errorAt(s, msg)
		return
	}
	g.unwindTo(l.depth)
	g.blk.Br(target(l))
}

// label is the name a break or a continue wrote, or "" for the
// innermost loop.
func (g *gen) label(id *ast.Ident) string {
	if id == nil {
		return ""
	}
	return g.text(id)
}
