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
		switch d := n.D.(type) {
		case *ast.VarDecl:
			g.varDecl(d)
		case *ast.FuncDecl:
			g.nestedFunc(d)
		default:
			// Anything else declared inside a body -- a type, an
			// extension -- is refused rather than skipped. Skipping
			// it is what this used to do for every declaration that
			// was not a var, which is how a nested function came to
			// be declared, called, and never emitted.
			g.refuse(n, declKind(n.D))
		}

	case *ast.ExprStmt:
		g.exprStmt(n.X)

	case *ast.ReturnStmt:
		g.ret(n)

	case *ast.IfStmt:
		g.ifStmt(n)

	case *ast.WhileStmt:
		g.whileStmt(n)

	case *ast.ForInStmt:
		g.forInStmt(n)

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
	if bin, ok := e.(*ast.BinaryExpr); ok {
		if op := g.text(bin.Op); op == "=" {
			g.assign(bin)
			return
		} else if base, isCompound := compoundOf(op); isCompound {
			g.compoundAssign(bin, base)
			return
		}
	}
	if v := g.expr(e); v != nil {
		// A value computed and used for nothing is destroyed where
		// the statement's scope ends.
		g.destroyLater(v)
	}
}

// compoundOf takes an assigning operator apart: `+=` is `+` and an
// assignment, and every one of them is.
//
// `=` itself is not one -- it assigns and applies nothing -- and
// neither are the comparisons that end in `=`, which produce a value
// rather than storing one.
func compoundOf(op string) (string, bool) {
	switch op {
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
		return op[:len(op)-1], true
	}
	return "", false
}

// compoundAssign lowers `x op= y` as `x = x op y`.
//
// The destination is read as a value and written as an address, which
// is two evaluations of the same expression. That is right for
// everything this compiler can assign to today -- a local, a stored
// property of one, a field through self -- because none of them has a
// side effect to happen twice. Swift's own answer is one evaluation
// through a modify accessor, which is what this becomes the day a
// subscript or a computed property can be written to.
func (g *gen) compoundAssign(e *ast.BinaryExpr, op string) {
	said := len(g.diags)
	addr := g.lvalue(e.X)
	if addr == nil {
		if len(g.diags) == said {
			g.refuse(e.X, "an assignment to "+g.exprKind(e.X))
		}
		return
	}
	cur, rhs := g.expr(e.X), g.expr(e.Y)
	if cur == nil || rhs == nil {
		return
	}
	t := g.typeOf(e.X)
	v := g.operate(e, op, t, t, cur, rhs)
	if v == nil {
		g.unsupported(e)
		return
	}
	access := g.blk.BeginAccess(addr, "modify", "unknown")
	g.blk.Assign(g.consume(v), access)
	g.blk.EndAccess(access)
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
	// Writing a concrete value into an existential variable replaces
	// what is in it, and what is in it is a value and the table that
	// says how to reach it. Storing over the buffer alone would leave
	// the old table behind, so the next call through it would reach
	// the old type's implementation with the new type's bytes -- the
	// answer would be a number, and it would be wrong. So the storage
	// is initialized again rather than stored to.
	//
	// Nothing is destroyed first because nothing in there owns
	// anything: initExistential refuses a value that does.
	if ex, isEx := existentialOf(g.typeOf(e.X)); isEx {
		from := g.typeOf(e.Y)
		if _, already := existentialOf(from); already {
			g.refuse(e, "assigning one existential to another, which needs the "+
				"value witness that copies what is inside it")
			return
		}
		v := g.rvalue(e.Y)
		if v == nil {
			return
		}
		access := g.blk.BeginAccess(addr, "modify", "unknown")
		g.initExistential(e, access, v, from, ex)
		g.blk.EndAccess(access)
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

	// `self.x = …` in an initializer, where self is storage being
	// filled in. Writing through it is the point of the initializer,
	// and the bare-name form above is the same write said shorter.
	case *ast.SelfExpr:
		if g.self != nil {
			return g.self.addr
		}
		return nil

	case *ast.MemberExpr:
		if n.Name == nil {
			return nil
		}
		t := lowerType(g.typeOf(n))
		name := memberName(g.typeOf(n.X), n.Name.Text(g.file))

		// A class's property is inside the object the reference
		// points at, so the base is a value and the field is
		// arithmetic on it.
		if isClass(g.typeOf(n.X)) {
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
	// An initializer's receiver is storage being filled in rather
	// than a value that was handed over, so a write goes into it.
	// That is what makes `x = v` legal in an init and not in an
	// ordinary method on a value type.
	if g.self != nil && g.self.addr != nil {
		return g.blk.StructElementAddr(g.self.addr,
			memberName(g.recv, name), lowerType(field.Type).Address())
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

		// A binding whose type is an existential is storage rather
		// than a value: what it holds is a buffer with a table beside
		// it, and neither a register nor an SSA value holds that.
		// SILGen allocates one with alloc_stack whether it was written
		// let or var, and so does this.
		if ex, isEx := existentialOf(sym.Type()); isEx {
			g.existentialDecl(b, sym, name, t, ex, d.Kind == token.LET)
			continue
		}

		if d.Kind == token.LET {
			v := g.rvalue(b.Value)
			if v == nil {
				continue
			}
			v = g.optionalFor(b.Value, v, g.typeOf(b.Value), sym.Type())
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
			v = g.optionalFor(b.Value, v, g.typeOf(b.Value), sym.Type())
			g.blk.Store(v, addr, storeQualifier(t))
		}
	}
}

// existentialDecl lowers `let v: any P = x` and its var form: the
// storage the binding lives in, and the value put into it.
//
// The two kinds differ in what may be done to the binding afterwards
// and not in how it is held, which is why one function does both.
func (g *gen) existentialDecl(b *ast.PatternBinding, sym analyzer.Symbol, name string, t vil.Type, ex *types.Existential, isLet bool) {
	kind := "var"
	if isLet {
		kind = "let"
	}
	slot := g.blk.AllocStackFor(t, name, kind)
	g.locals[sym] = &local{addr: slot, typ: t, mem: true}
	if b.Value == nil {
		return
	}
	from := g.typeOf(b.Value)
	// An existential copied out of another one is a copy of whatever
	// is inside it, which is the value witness table's job and there
	// is none. Binding the same storage under a second name instead
	// would give two names to one value, and a write through either
	// would be seen through both.
	if _, already := existentialOf(from); already {
		g.refuse(b.Value, "copying one existential into another binding, which needs "+
			"the value witness that copies what is inside it")
		return
	}
	v := g.rvalue(b.Value)
	if v == nil {
		return
	}
	g.initExistential(b.Value, slot, v, from, ex)
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
	// `if let x = v` is a switch on which case the optional holds
	// rather than a test of a bit, and the arm that runs gets the
	// payload as a block argument. See optional.go.
	if g.ifLet(s) {
		return
	}
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
		return g.machine(v, g.typeOf(e))
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

	subjectType := g.typeOf(s.Subject)
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
		// A case body is a list of statements rather than a code
		// block, so nothing else pushes a scope for it -- and
		// anything it allocates would then be cleaned up by the scope
		// around the whole switch, at the continuation, which no case
		// body dominates. A `for` in a case is the shape that shows
		// it: its index is a box, and the box was destroyed where the
		// cases had already met.
		//
		// The depth is taken before the push, so that a `break` out
		// of the case unwinds this scope on its way.
		depth := len(g.scopes)
		g.push()
		g.loops = append(g.loops, loop{header: header, lazyExit: cont, depth: depth})
		for _, st := range cs.Stmts {
			g.stmt(st)
			if g.blk == nil || g.blk.Term() != nil {
				break
			}
		}
		g.loops = g.loops[:len(g.loops)-1]
		if g.blk != nil && g.blk.Term() == nil {
			g.pop()
			g.blk.Br(cont())
		} else {
			// The body left by itself and has already unwound.
			g.scopes = g.scopes[:len(g.scopes)-1]
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
			// A case that binds what it carries: the payload arrives
			// as the arm's block argument, which is where SILGen puts
			// it and where the names bind. See bindCasePayload.
			if pat.Args != nil {
				if !g.bindCasePayload(t, pat, bodies[i]) {
					return false
				}
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
	return g.compare("==", a, b, t)
}

// compare is the bit an ordering or equality test produces: the raw
// Builtin.Int1 rather than a Bool, because that is what cond_br and
// cond_fail take and wrapping it in a struct only to take it apart
// again says nothing.
func (g *gen) compare(op string, a, b *vil.Value, t types.Type) *vil.Value {
	// An operator that is a call rather than an instruction. Two
	// strings are equal when their bytes say so, which is a walk and
	// not a comparison of registers. See string.go.
	if ex, ok := core.LowerExtern(op, t); ok && !ex.Owned {
		v := g.externCompare(a, b, t, ex)
		if v == nil {
			return nil
		}
		return v
	}
	bi, ok := core.Lower(op, t)
	if !ok {
		return nil
	}
	return g.blk.Builtin(bi.Name, vil.Object(builtinNamed(bi.Result)),
		g.machine(a, t), g.machine(b, t))
}

// increment is v + 1 at v's own type, as the checked add Swift uses.
//
// The trap it carries can never fire where this package emits it: a
// for-in only increments an index it has just proved is below the
// upper bound. It is the checked builtin anyway because that is the
// one `+` on an Int is, and reaching for an unchecked add here would
// make this the one place in the compiler where Swift's arithmetic
// means something else.
func (g *gen) increment(v *vil.Value, t types.Type) *vil.Value {
	bi, ok := core.Lower("+", t)
	if !ok || !bi.Overflows {
		return nil
	}
	one := g.blk.IntegerLiteral(vil.Object(builtinFor(t)), 1)
	want := g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), -1)
	pair := vil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: builtinNamed(bi.Result)},
		{Type: vil.BuiltinInt1},
	}})
	both := g.blk.Builtin(bi.Name, pair, g.machine(v, t), one, want)
	sum := g.blk.TupleExtract(both, 0, vil.Object(builtinNamed(bi.Result)))
	flag := g.blk.TupleExtract(both, 1, vil.Object(vil.BuiltinInt1))
	g.blk.CondFail(flag, "arithmetic overflow")
	return g.blk.Struct(lowerType(t), sum)
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
	g.blk.CondBr(g.machine(cond, g.typeOf(s.Cond)), body, nil, exit, nil)

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

// forInStmt lowers `for i in a..<b` and `for i in a...b` as the
// counting loop they are.
//
// SILGen does not do this. It emits the desugaring the language
// defines — `makeIterator()`, then `next()` in a loop until it returns
// none — and every piece of that is generic stdlib: Range, Collection,
// IndexingIterator, Optional. None of it exists here, so lowering the
// desugaring would mean lowering calls to functions that are declared
// nowhere.
//
// What is emitted instead is what `swiftc -O` produces once it has
// specialized and inlined all of that away, which is the same program:
// two bounds evaluated once, an index, and a comparison. The oracle is
// `swiftc -O -emit-sil`, and three of its facts are load-bearing:
//
//   - the bounds are evaluated once, before the loop, not per
//     iteration;
//   - a reversed range traps, and says so in those words. It is not an
//     empty loop, which is what a naive `i < hi` test would silently
//     make it;
//   - `a..<b` stops before b and `a...b` includes it.
//
// The two spellings need two shapes. A half-open range is tested at
// the top, because it may run zero times. A closed one cannot — the
// trap above guarantees a <= b — so it is tested at the bottom, after
// the body, against the upper bound itself. That is not a style
// choice: `for i in 0...Int.max` with a top test would have to
// evaluate i + 1 past the end of the type, and the increment is only
// safe below because it is reached having just proved i is not yet the
// upper bound.
//
// Anything that is not a range of integers is refused. An array, a
// dictionary, a type of one's own conforming to Sequence — each needs
// the iterator protocol this cannot lower, and each says so by name.
func (g *gen) forInStmt(s *ast.ForInStmt) {
	switch {
	case s.Await.IsValid():
		g.refuse(s, "an async for-in")
		return
	case s.Try.IsValid():
		g.refuse(s, "a throwing for-in")
		return
	case s.Case.IsValid():
		g.refuse(s, "a for-in with a `case` pattern")
		return
	case s.Where != nil:
		g.refuse(s, "a for-in with a `where` clause")
		return
	}

	// An array is walked by index rather than through an iterator,
	// which is the same thing `swiftc -O` arrives at. See forInArray.
	if arr, ok := g.typeOf(s.Seq).Underlying().(*types.Array); ok {
		g.forInArray(s, arr)
		return
	}
	rng, ok := g.typeOf(s.Seq).Underlying().(*types.Range)
	if !ok {
		g.refuse(s, "a for-in over "+g.seqKind(s.Seq))
		return
	}
	elem := rng.Element
	if _, ok := core.Lower("<", elem); !ok {
		g.refuse(s, "a for-in over a range of "+elem.String())
		return
	}

	bin, ok := g.fold(s.Seq).(*ast.BinaryExpr)
	if !ok {
		g.refuse(s.Seq, "this range")
		return
	}

	// Once, before anything else: a bound that calls a function calls
	// it one time, however many times the body runs.
	lo, hi := g.rvalue(bin.X), g.rvalue(bin.Y)
	if lo == nil || hi == nil {
		return
	}

	// `cond_fail (hi < lo)`, in Swift's own words. A range that runs
	// backwards is a mistake about the program rather than an empty
	// loop, and this is the only place that can still say so — by the
	// time the index is being compared, the two are indistinguishable.
	if bad := g.compare("<", hi, lo, elem); bad != nil {
		g.blk.CondFail(bad, "Range requires lowerBound <= upperBound")
	}

	// The index is a box for the same reason a `var` is: it is written
	// to, and an SSA value is not. Allocating it through the scope
	// machinery is what gets it released on every way out of the loop,
	// a `break` and a `return` from inside the body included.
	vt := lowerType(elem)
	name := g.patternName(s.Pat)
	box := g.blk.AllocBox(vt, "$"+name+"$index", "var")
	borrow := g.blk.BeginBorrow(box, "var_decl")
	slot := g.blk.ProjectBox(borrow, 0, vt)
	g.destroyLater(box)
	g.endBorrowLater(borrow)
	g.blk.Store(lo, slot, storeQualifier(vt))

	header := g.fn.Block()
	latch := g.fn.Block()
	exit := g.fn.Block()
	label := g.takeLabel()

	g.blk.Br(header)

	// The head of a half-open loop tests before the body; a closed one
	// falls straight in, having already proved it has an iteration to
	// run.
	g.blk = header
	index := g.blk.Load(slot, loadQualifier(vt))
	if !rng.Closed {
		more := g.compare("<", index, hi, elem)
		if more == nil {
			g.refuse(s.Seq, "a range of "+elem.String())
			return
		}
		// The body is a block of its own only here. A closed range
		// runs its body straight out of the header, and allocating a
		// block it would never branch to would leave an empty one
		// behind.
		body := g.fn.Block()
		g.blk.CondBr(more, body, nil, exit, nil)
		g.blk = body
		index = g.blk.Load(slot, loadQualifier(vt))
	}

	// The loop variable is a fresh `let` each time round, bound to the
	// index as it stands. That is SILGen's shape too — a move_value
	// [var_decl] of what next() produced — and it is why writing to
	// the index inside the body is not a thing the language offers.
	depth := len(g.scopes)
	g.push()
	g.bindLoopVar(s.Pat, index, vt)

	g.loops = append(g.loops, loop{header: latch, exit: exit, depth: depth, label: label})
	g.block(s.Body)
	g.loops = g.loops[:len(g.loops)-1]
	if g.blk != nil && g.blk.Term() == nil {
		g.pop()
		g.blk.Br(latch)
	} else {
		// The body left by itself and has already unwound.
		g.scopes = g.scopes[:len(g.scopes)-1]
	}

	// `continue` lands here, which is why the bottom test lives in
	// this block rather than at the end of the body: a continue in a
	// closed-range loop still has to ask whether that was the last
	// iteration.
	g.blk = latch
	at := g.blk.Load(slot, loadQualifier(vt))
	step := latch
	if rng.Closed {
		last := g.compare("==", at, hi, elem)
		if last == nil {
			g.refuse(s.Seq, "a range of "+elem.String())
			return
		}
		step = g.fn.Block()
		g.blk.CondBr(last, exit, nil, step, nil)
		g.blk = step
		at = g.blk.Load(slot, loadQualifier(vt))
	}
	next := g.increment(at, elem)
	if next == nil {
		g.refuse(s.Seq, "a range of "+elem.String())
		return
	}
	g.blk.Store(next, slot, storeQualifier(vt))
	g.blk.Br(header)

	g.blk = exit
}

// bindLoopVar binds a for-in's pattern to one iteration's value.
//
// A wildcard binds nothing and is not an omission: `for _ in 0..<3`
// says the count is what matters, and there is no name to give the
// value to.
func (g *gen) bindLoopVar(pat ast.Pattern, v *vil.Value, t vil.Type) {
	name := patternIdent(pat)
	if name == nil {
		return
	}
	sym := g.info.Defs[name]
	if sym == nil {
		return
	}
	bound := g.blk.MoveValue(v, "lexical", "var_decl")
	g.blk.DebugValue(bound, g.text(name), "let")
	g.destroyLater(bound)
	g.locals[sym] = &local{value: bound, typ: t}
}

// patternIdent is the name a pattern binds, or nil for one that binds
// nothing.
//
// Two spellings reach here for the same thing. The parser reads a
// for-in's pattern in matching mode, where a bare name is a value to
// compare with rather than a name to bind, so `for i in …` arrives as
// an expression; `for i: Int in …` arrives wrapped in its annotation.
// The analyzer declares a symbol for both, so both have to be found.
func patternIdent(pat ast.Pattern) *ast.Ident {
	switch p := pat.(type) {
	case *ast.TypedPattern:
		return patternIdent(p.Pat)
	case *ast.ValueBindingPattern:
		return patternIdent(p.Pat)
	case *ast.IdentPattern:
		return p.Name
	case *ast.ExprPattern:
		if id, ok := p.X.(*ast.IdentExpr); ok {
			return id.Name
		}
	}
	return nil
}

// patternName is what to call the hidden index, so that a reader of
// the IR can tell which loop it belongs to.
func (g *gen) patternName(pat ast.Pattern) string {
	if name := patternIdent(pat); name != nil {
		return g.text(name)
	}
	return "i"
}

// fold resolves a sequence expression to the tree the analyzer built
// from it. The parser leaves `a..<b` as a flat sequence of operands
// and operators; which of them binds tighter is the analyzer's
// answer, and this is where it is read.
func (g *gen) fold(e ast.Expr) ast.Expr {
	if seq, ok := e.(*ast.SequenceExpr); ok {
		if folded, ok := g.info.Folded[seq]; ok {
			return folded
		}
	}
	if p, ok := e.(*ast.ParenExpr); ok {
		return g.fold(p.X)
	}
	return e
}

// seqKind names what a for-in was asked to walk, for the refusal.
func (g *gen) seqKind(e ast.Expr) string {
	t := g.typeOf(e)
	if t == nil {
		return "this"
	}
	switch t.Underlying().(type) {
	case *types.Array:
		return "an array"
	case *types.Dictionary:
		return "a dictionary"
	}
	return "'" + t.String() + "'"
}

// bindCasePayload gives an arm a block argument for what its case
// carries and binds the pattern's names to it.
//
// A case carrying one value binds one name to the argument. A case
// carrying several carries a tuple of them, and each name binds to an
// element of it -- which is a read from the argument rather than a
// second argument, because the tuple is what the case holds.
func (g *gen) bindCasePayload(subject types.Type, pat *ast.EnumCasePattern, arm *vil.Block) bool {
	assoc := g.caseAssociated(subject, g.text(pat.Name))
	if assoc == nil {
		g.refuse(pat, "a pattern binding a value on a case that carries none")
		return false
	}
	payload := arm.Arg(lowerType(assoc), vil.Unowned)
	names := pat.Args.Elems
	if len(names) == 1 {
		return g.bindPatternTo(names[0].Pat, payload, assoc)
	}
	tu, ok := assoc.Underlying().(*types.Tuple)
	if !ok || len(tu.Elements) != len(names) {
		g.refuse(pat, "a pattern whose names do not match what the case carries")
		return false
	}
	// The arm is where the reads go: this is emitted before the body,
	// which is what the block is for.
	prev := g.blk
	g.blk = arm
	for i, el := range names {
		v := g.blk.TupleExtract(payload, i, lowerType(tu.Elements[i].Type))
		if !g.bindPatternTo(el.Pat, v, tu.Elements[i].Type) {
			g.blk = prev
			return false
		}
	}
	g.blk = prev
	return true
}

// bindPatternTo binds one name in a case pattern to a value.
func (g *gen) bindPatternTo(p ast.Pattern, v *vil.Value, t types.Type) bool {
	if bind, ok := p.(*ast.ValueBindingPattern); ok {
		p = bind.Pat
	}
	id, ok := p.(*ast.IdentPattern)
	if !ok || id.Name == nil {
		g.refuse(p, "this pattern inside a case")
		return false
	}
	sym := g.info.Defs[id.Name]
	if sym == nil {
		g.refuse(p, "a name in a case this compiler did not resolve")
		return false
	}
	g.locals[sym] = &local{value: v, typ: lowerType(t)}
	return true
}

// caseAssociated is what a named case of an enum carries.
func (g *gen) caseAssociated(subject types.Type, name string) types.Type {
	if meta, ok := subject.(*types.Metatype); ok {
		subject = meta.Instance
	}
	e, ok := subject.Underlying().(*types.Enum)
	if !ok {
		return nil
	}
	for _, c := range e.Cases {
		if c != nil && c.Name == name {
			return c.AssociatedType
		}
	}
	return nil
}
