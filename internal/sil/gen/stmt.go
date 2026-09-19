package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

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
	g.popReachable()
}

// stmt lowers one statement inside a formal scope of its own, so
// that borrows opened by its expressions close with it.
func (g *gen) stmt(s ast.Stmt) {
	g.pushFormal()
	g.stmtBody(s)
	// What the statement wrote through a subscript's temporary is set
	// back before the statement's temporaries end.
	g.flushWritebacks()
	g.popReachable()
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

	case *ast.DeferStmt:
		// Runs when its enclosing block is left, however it is left.
		s := g.lexical()
		s.cleanups = append(s.cleanups, cleanup{deferred: n.Body})

	case *ast.DoStmt:
		if len(n.Catches) > 0 {
			g.doCatch(n)
			break
		}
		// A `do` with nothing to catch is a scope: its defers run at its end.
		g.block(n.Body)

	case *ast.ThrowStmt:
		g.throwStmt(n)

	case *ast.LabeledStmt:
		g.labeled(n)

	case *ast.BreakStmt:
		g.breakStmt(n)

	case *ast.ContinueStmt:
		g.continueStmt(n)

	case *ast.CodeBlock:
		g.block(n)

	case *ast.EmptyStmt:
	case *ast.BadStmt:
	default:
		g.unsupportedStmt(s)
	}
}

// exprStmt lowers an expression evaluated for side effects or assignment.
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
		g.destroyLater(v)
	}
}

// chainedDestination lowers an assignment whose destination is an optional
// chain -- `a.next?.v = 5` -- as Swift does: the whole assignment is the
// chain, so where a step finds nothing, nothing is assigned and the value
// is not evaluated. It reports whether the destination was one; lower is
// the assignment, run inside the chain.
func (g *gen) chainedDestination(e *ast.BinaryExpr, lower func()) bool {
	if !g.info.ChainRoots[e.X] || g.chainActive[e.X] {
		return false
	}
	none, join := g.fn.Block(), g.fn.Block()
	if g.chainActive == nil {
		g.chainActive = map[ast.Expr]bool{}
	}
	g.chainActive[e.X] = true
	g.chainNone = append(g.chainNone, none)
	g.push()
	g.chainDepth = append(g.chainDepth, len(g.scopes)-1)
	lower()
	g.popReachable()
	g.chainNone = g.chainNone[:len(g.chainNone)-1]
	g.chainDepth = g.chainDepth[:len(g.chainDepth)-1]
	delete(g.chainActive, e.X)
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Br(join)
	}
	g.blk = none
	g.blk.Br(join)
	g.blk = join
	return true
}

// compoundOf returns the base binary operator and true for compound assignments.
func compoundOf(op string) (string, bool) {
	switch op {
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=",
		"&+=", "&-=", "&*=", "&<<=", "&>>=":
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
	if g.chainedDestination(e, func() { g.compoundAssign(e, op) }) {
		return
	}
	// A computed property is read and written through its accessors,
	// so `c.d += n` is a call, an operator and a call. The base is
	// evaluated twice, which is what the note above says about every
	// other destination -- Swift reads it once through a modify
	// accessor, which this does not have.
	if mem, ok := e.X.(*ast.MemberExpr); ok && mem.Name != nil {
		recv := g.typeOf(mem.X)
		if f, viaSetter := g.setterField(recv, g.text(mem.Name)); viaSetter {
			// Read as the property is read: a computed one through
			// its getter, and one with observers straight out of the
			// storage it has. Only the write differs for the second.
			cur := g.expr(e.X)
			if _, isComputed := g.computedField(recv, g.text(mem.Name)); isComputed {
				cur = g.getterCall(mem, recv, f, func() *sil.Value { return g.expr(mem.X) })
			}
			rhs := g.expr(e.Y)
			if cur == nil || rhs == nil {
				return
			}
			t := g.typeOf(e.X)
			v := g.operate(e, op, t, t, cur, rhs)
			if v == nil {
				g.unsupported(e)
				return
			}
			g.setterCallValue(mem, recv, f, g.consume(v))
			return
		}
	}
	// Through a subscript a type declares: read by its getter, written
	// by its setter.
	if sub, ok := e.X.(*ast.SubscriptExpr); ok {
		if ref := g.info.Subscripts[sub]; ref != nil {
			cur := g.declaredSubscriptRead(sub, ref)
			rhs := g.expr(e.Y)
			if cur == nil || rhs == nil {
				return
			}
			t := g.typeOf(e.X)
			v := g.operate(e, op, t, t, cur, rhs)
			if v == nil {
				g.unsupported(e)
				return
			}
			g.declaredSubscriptWrite(sub, ref, v)
			return
		}
	}
	// d[k, default: v] op= w reads and writes through the runtime. An
	// array's a[i] op= v is written where the element is, below.
	if sub, ok := e.X.(*ast.SubscriptExpr); ok {
		_, isDict := g.typeOf(sub.X).Underlying().(*types.Dictionary)
		if isDict && g.defaultSubscript(sub) {
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
			nv := g.optionalFor(e, g.consume(v), t, &types.Optional{Wrapped: t})
			g.subscriptAssign(e.X, collArg{value: nv})
			return
		}
	}
	if arr, ok := g.typeOf(e.X).Underlying().(*types.Array); ok && op == "+" {
		if m, found := core.LowerCollectionMethod(arr, "append", []string{"contentsOf"}); found {
			g.collectionCall(e, m, e.X, exprArgs(e.Y))
			return
		}
	}
	said := len(g.diags)
	// Through an array element, the operand comes first and the element
	// is read where it is written, once. See assign.
	var rhs *sil.Value
	through := g.writesThroughElement(e.X)
	if through {
		if rhs = g.expr(e.Y); rhs == nil {
			return
		}
	}
	addr := g.lvalue(e.X)
	if addr == nil {
		if len(g.diags) == said {
			g.refuse(e.X, "an assignment to "+g.exprKind(e.X))
		}
		return
	}
	var cur *sil.Value
	if through {
		lt := lowerType(g.typeOf(e.X))
		cur = g.blk.Load(addr, loadQualifier(lt))
		g.destroyLater(cur)
	} else {
		cur, rhs = g.expr(e.X), g.expr(e.Y)
	}
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

// assign lowers a store to a variable.
func (g *gen) assign(e *ast.BinaryExpr) {
	if g.chainedDestination(e, func() { g.assign(e) }) {
		return
	}
	if mem, ok := e.X.(*ast.MemberExpr); ok && mem.Name != nil {
		recv := g.typeOf(mem.X)
		if f, viaSetter := g.setterField(recv, g.text(mem.Name)); viaSetter {
			g.setterCall(mem, recv, f, e.Y)
			return
		}
	}
	// `_ = v` evaluates v and discards the result.
	if _, discard := e.X.(*ast.WildcardExpr); discard {
		if v := g.expr(e.Y); v != nil {
			g.destroyLater(v)
		}
		return
	}
	// Through a subscript a type declares: its setter.
	if sub, ok := e.X.(*ast.SubscriptExpr); ok {
		if ref := g.info.Subscripts[sub]; ref != nil {
			// The setter borrows the value; what made it ends with
			// the statement.
			v := g.expr(e.Y)
			if v == nil {
				return
			}
			v = g.optionalFor(e.Y, v, g.typeOf(e.Y), ref.Subscript.Result)
			g.declaredSubscriptWrite(sub, ref, v)
			return
		}
	}
	// Subscript assignment handled by runtime calls.
	if g.subscriptAssign(e.X, collArg{expr: e.Y}) {
		return
	}
	said := len(g.diags)
	// Through an array element, the value comes first: the element's
	// address is where the array's storage is once it is unique, which
	// reading or copying the array in the value would undo.
	_, toExistential := existentialOf(g.typeOf(e.X))
	var early *sil.Value
	if !toExistential && g.writesThroughElement(e.X) {
		if early = g.rvalue(e.Y); early == nil {
			return
		}
	}
	addr := g.lvalue(e.X)
	if addr == nil {
		if len(g.diags) == said {
			g.refuse(e.X, "an assignment to "+g.exprKind(e.X))
		}
		return
	}
	// Writing a concrete value into an existential initializes the container with its witness table.
	if ex, isEx := existentialOf(g.typeOf(e.X)); isEx {
		from := g.typeOf(e.Y)
		// What the existential held is let go, and the new value put in.
		if _, already := existentialOf(from); already {
			src := g.expr(e.Y)
			if src == nil {
				return
			}
			access := g.blk.BeginAccess(addr, "modify", "unknown")
			g.blk.DestroyAddr(access)
			g.intoExistential(src, access)
			g.blk.EndAccess(access)
			return
		}
		v := g.rvalue(e.Y)
		if v == nil {
			return
		}
		access := g.blk.BeginAccess(addr, "modify", "unknown")
		g.blk.DestroyAddr(access)
		g.initExistential(e, access, v, from, ex)
		g.blk.EndAccess(access)
		return
	}

	v := early
	if v == nil {
		v = g.rvalue(e.Y)
	}
	if v == nil {
		return
	}
	v = g.optionalFor(e.Y, v, g.typeOf(e.Y), g.typeOf(e.X))
	access := g.blk.BeginAccess(addr, "modify", "unknown")
	g.blk.Assign(v, access)
	g.blk.EndAccess(access)
}

// lvalue lowers an expression to its destination address for assignment.
func (g *gen) lvalue(e ast.Expr) *sil.Value {
	switch n := e.(type) {
	case *ast.ParenExpr:
		return g.lvalue(n.X)

	case *ast.IdentExpr:
		if sym := g.info.Uses[n.Name]; sym != nil {
			if l := g.locals[sym]; l != nil {
				return l.addr
			}
			if addr, _, ok := g.moduleVarAddr(sym); ok {
				return addr
			}
		}
		// In a method, bare names may resolve to receiver properties,
		// or to static ones.
		if addr := g.implicitStaticAddr(n); addr != nil {
			return addr
		}
		return g.implicitSelfAddr(n)

	// self in an initializer writes directly to instance storage.
	case *ast.SelfExpr:
		if g.self != nil {
			return g.self.addr
		}
		return nil

	case *ast.MemberExpr:
		if n.Name == nil {
			return nil
		}
		// p.pointee writes through the pointer value.
		if p, ok := g.isPointee(n); ok {
			return g.pointeeAddrForWrite(n, p)
		}
		// `Counter.made = 5`: a static stored property's storage.
		if recv, f, ok := g.staticProperty(n); ok && !f.IsComputed {
			return g.staticAddr(n, recv, f)
		}
		// Computed properties have no storage address; writes go via setter.
		if g.isComputedMember(g.typeOf(n.X), n.Name.Text(g.file)) {
			g.refuse(n, "an assignment to a computed property, which is a call to "+
				"its setter and not a write to storage")
			return nil
		}
		t := lowerType(g.typeOf(n))
		name := memberName(g.typeOf(n.X), n.Name.Text(g.file))

		// Class property addresses are computed from the object reference.
		if isClass(g.typeOf(n.X)) {
			base := g.expr(n.X)
			if base == nil {
				return nil
			}
			return g.blk.RefElementAddr(base, name, t)
		}

		// Struct property addresses require the struct base to be an address.
		addr := g.lvalue(n.X)
		if addr == nil {
			return nil
		}
		return g.blk.StructElementAddr(addr, name, t)

	case *ast.SubscriptExpr:
		if ref := g.info.Subscripts[n]; ref != nil {
			return g.declaredSubscriptAddr(n, ref)
		}
		return g.elementAddr(n)

	// `a?.x = v` writes into a's payload where a is some.
	case *ast.OptionalExpr:
		return g.chainStepAddr(n)
	}
	return nil
}

// implicitSelfAddr returns the address of a stored property accessed via implicit self.
func (g *gen) implicitSelfAddr(e *ast.IdentExpr) *sil.Value {
	if g.recv == nil || e.Name == nil {
		return nil
	}
	name := g.text(e.Name)
	owner, field, ok := storedField(g.recv, name)
	if !ok {
		return nil
	}
	// Initializer receivers are mutable storage.
	if g.self != nil && g.self.addr != nil {
		return g.blk.StructElementAddr(g.self.addr,
			memberName(owner, name), lowerType(field.Type).Address())
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
	return g.blk.RefElementAddr(self, memberName(owner, name), lowerType(field.Type))
}

// text is a node's spelling.
func (g *gen) text(n ast.Node) string {
	if n == nil {
		return ""
	}
	// A name written in backticks -- `default` -- is the name inside them.
	if id, ok := n.(*ast.Ident); ok {
		return id.Text(g.file)
	}
	return string(g.file.Slice(n.Pos(), n.End()))
}

// varDecl lowers variable bindings: let binds SSA values; var allocates a box.
func (g *gen) varDecl(d *ast.VarDecl) {
	for _, b := range d.Bindings {
		if tp, ok := untyped(b.Pat).(*ast.TuplePattern); ok {
			g.tupleDecl(tp, b, d.Kind == token.LET)
			continue
		}
		// `let _ = f()` evaluates f() for what it does and keeps nothing.
		if _, discard := untyped(b.Pat).(*ast.WildcardPattern); discard && b.Value != nil {
			g.push()
			if v := g.expr(b.Value); v != nil {
				g.destroyLater(v)
			}
			g.pop()
			continue
		}
		name, sym := g.binding(b)
		if sym == nil {
			continue
		}
		// Under a specialization, the type written for the binding --
		// `var s = Stack<Element>()` -- is the instance's.
		symType := g.substituted(sym.Type())
		t := lowerType(symType)

		// Existentials allocate stack storage for container and witness tables.
		if ex, isEx := existentialOf(symType); isEx {
			g.existentialDecl(b, sym, name, t, ex, d.Kind == token.LET)
			continue
		}

		// A let given its value later is written like a var: there is
		// somewhere to put the value each path assigns it.
		if d.Kind == token.LET && b.Value != nil {
			v := g.rvalue(b.Value)
			if v == nil {
				continue
			}
			v = g.optionalFor(b.Value, v, g.typeOf(b.Value), symType)
			bound := g.blk.MoveValue(v, "lexical", "var_decl")
			g.blk.DebugValue(bound, name, "let")
			g.destroyLater(bound)
			g.locals[sym] = &local{value: bound, typ: t}
			continue
		}

		attrs := []string{"var"}
		if b.Value == nil && !t.Trivial() {
			// Nothing is stored before the first assignment, which lets go
			// of what was there: storage that starts as zero holds nothing
			// to let go of, a box or the stack slot one becomes.
			attrs = append(attrs, "zeroed")
		}
		box := g.blk.AllocBox(t, name, attrs...)
		borrow := g.blk.BeginBorrow(box, "var_decl")
		addr := g.blk.ProjectBox(borrow, 0, t)
		g.locals[sym] = &local{addr: addr, box: box, typ: t}

		// Box lifetime and borrow scope cleanup.
		g.destroyLater(box)
		g.endBorrowLater(borrow)

		if b.Value == nil {
			continue
		}
		if v := g.rvalue(b.Value); v != nil {
			v = g.optionalFor(b.Value, v, g.typeOf(b.Value), symType)
			g.blk.Store(v, addr, storeQualifier(t))
		}
	}
}

// untyped is a binding's pattern without the type written on it.
func untyped(p ast.Pattern) ast.Pattern {
	if tp, ok := p.(*ast.TypedPattern); ok {
		return tp.Pat
	}
	return p
}

// tupleDecl lowers `let (a, b) = v` and `var (a, _) = v`: the tuple taken
// apart, and each name bound to its element as a declaration of that one
// name would bind it.
func (g *gen) tupleDecl(tp *ast.TuplePattern, b *ast.PatternBinding, isLet bool) {
	if b.Value == nil {
		g.refuse(tp, "a tuple pattern with no value to take apart")
		return
	}
	v := g.rvalue(b.Value)
	if v == nil {
		return
	}
	g.bindTuple(tp, v, g.substituted(g.typeOf(b.Value)), isLet)
}

// bindTuple binds the names of a tuple pattern to the elements of v.
func (g *gen) bindTuple(tp *ast.TuplePattern, v *sil.Value, t types.Type, isLet bool) {
	tu, ok := t.Underlying().(*types.Tuple)
	if !ok || len(tu.Elements) != len(tp.Elems) {
		g.refuse(tp, "a tuple pattern whose names do not match the value's elements")
		return
	}
	elems := make([]sil.Type, len(tu.Elements))
	for i, el := range tu.Elements {
		elems[i] = lowerType(el.Type)
	}
	parts := g.blk.DestructureTuple(v, elems...)
	for i, el := range tp.Elems {
		g.bindElement(untyped(el.Pat), parts[i], tu.Elements[i].Type, isLet)
	}
}

// bindElement binds one pattern of a destructuring declaration.
func (g *gen) bindElement(p ast.Pattern, v *sil.Value, t types.Type, isLet bool) {
	switch p := p.(type) {
	case *ast.ValueBindingPattern:
		g.bindElement(untyped(p.Pat), v, t, p.Kind == token.LET)
	case *ast.WildcardPattern:
		g.destroyLater(v)
	case *ast.TuplePattern:
		g.bindTuple(p, v, t, isLet)
	case *ast.IdentPattern:
		sym := g.info.Defs[p.Name]
		if sym == nil {
			g.destroyLater(v)
			return
		}
		name := p.Name.Text(g.file)
		lt := lowerType(t)
		if isLet {
			bound := g.blk.MoveValue(v, "lexical", "var_decl")
			g.blk.DebugValue(bound, name, "let")
			g.destroyLater(bound)
			g.locals[sym] = &local{value: bound, typ: lt}
			return
		}
		box := g.blk.AllocBox(lt, name, "var")
		borrow := g.blk.BeginBorrow(box, "var_decl")
		addr := g.blk.ProjectBox(borrow, 0, lt)
		g.locals[sym] = &local{addr: addr, box: box, typ: lt}
		g.destroyLater(box)
		g.endBorrowLater(borrow)
		g.blk.Store(v, addr, storeQualifier(lt))
	default:
		g.refuse(p, "this pattern in a declaration")
		g.destroyLater(v)
	}
}

// existentialDecl lowers let/var existential bindings into stack storage.
func (g *gen) existentialDecl(b *ast.PatternBinding, sym analyzer.Symbol, name string, t sil.Type, ex *types.Existential, isLet bool) {
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
	// One existential from another: what is inside is copied through its
	// value witness, or its box shared.
	if _, already := existentialOf(from); already {
		src := g.expr(b.Value)
		if src == nil {
			return
		}
		g.intoExistential(src, slot)
		g.destroyAddrLater(slot)
		return
	}
	v := g.rvalue(b.Value)
	if v == nil {
		return
	}
	if g.initExistential(b.Value, slot, v, from, ex) {
		g.destroyAddrLater(slot)
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
func storeQualifier(t sil.Type) string {
	if t.Trivial() {
		return "trivial"
	}
	return "init"
}

// ret lowers a return statement, unwinding scopes before the terminator.
func (g *gen) ret(s *ast.ReturnStmt) {
	if s.X == nil {
		if g.initReturn != nil {
			g.initReturn()
			return
		}
		g.unwind()
		g.blk.Return(g.result())
		return
	}
	v := g.rvalue(s.X)
	// A value returned where an optional is declared is wrapped:
	// `return .success(p)` from a function returning Result<T, E>?.
	if v != nil && g.fn != nil {
		if rs := g.fn.Type().Results; len(rs) == 1 && rs[0].Type.IsValid() {
			v = g.optionalFor(s.X, v, g.typeOf(s.X), rs[0].Type.Formal())
		}
	}
	g.unwind()
	if v == nil {
		g.blk.Unreachable()
		return
	}
	g.blk.Return(v)
}

// ifStmt lowers an if statement and joins reachable branches.
func (g *gen) ifStmt(s *ast.IfStmt) {
	// Scope condition bindings for the conditional body.
	scoped := bindsAName(s.Conds)
	if scoped {
		g.push()
	}
	miss := g.laterBlock()
	if !g.conditionChain(s.Conds, miss) {
		if scoped {
			g.pop()
		}
		return
	}
	elseBlk := miss()

	g.block(s.Body)
	if scoped {
		g.popReachable()
	}
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

// conditionChain lowers a condition list to short-circuiting branches.
func (g *gen) conditionChain(conds []ast.Node, fail func() *sil.Block) bool {
	if len(conds) == 0 {
		g.refuse(nil, "an empty condition")
		return false
	}
	// Clean up earlier bindings if a subsequent condition fails.
	var bound []*sil.Value
	failing := func() *sil.Block {
		if len(bound) == 0 {
			return fail()
		}
		b := g.fn.Block()
		for i := len(bound) - 1; i >= 0; i-- {
			b.DestroyValue(bound[i])
		}
		b.Br(fail())
		return b
	}
	for _, c := range conds {
		switch c := c.(type) {
		case *ast.OptionalBinding:
			payload, ok := g.bindCondition(c, failing)
			if !ok {
				return false
			}
			if payload != nil && payload.Ownership() == sil.Owned {
				bound = append(bound, payload)
			}
		case *ast.CaseCond:
			arm, ok := g.caseCondition(c, failing)
			if !ok {
				return false
			}
			bound = append(bound, g.armOwned[arm]...)
			g.destroyArmOwned(arm)
		case ast.Expr:
			// A condition's temporaries end with the condition, before the
			// branch -- not with the statement. `while i < a.count` reads a
			// copy of the array to count it, and a copy still held while the
			// body writes to the array makes every write copy the whole
			// array first.
			g.push()
			v := g.expr(c)
			if v == nil {
				g.scopes = g.scopes[:len(g.scopes)-1]
				return false
			}
			bit := g.machine(v, g.typeOf(c))
			g.pop()
			next := g.fn.Block()
			g.blk.CondBr(bit, next, nil, failing(), nil)
			g.blk = next
		default:
			g.refuse(c, conditionKind(c))
			return false
		}
	}
	return true
}

// caseCondition lowers `case .x(let a) = v` in a condition list: a switch
// on v whose case goes on, with what it carries bound, and whose every
// other case fails. It is the block that goes on.
func (g *gen) caseCondition(c *ast.CaseCond, fail func() *sil.Block) (*sil.Block, bool) {
	pat := c.Pat
	if bind, ok := pat.(*ast.ValueBindingPattern); ok {
		pat = bind.Pat
	}
	ep, ok := pat.(*ast.EnumCasePattern)
	t := g.typeOf(c.Value)
	if _, isEnum := underlyingEnum(t); !ok || !isEnum || ep.Name == nil || !g.simpleCasePattern(c.Pat) {
		return g.matchCondition(c, fail)
	}
	// A switch consumes what it switches on, so it is given a copy where
	// the value is a name's own -- as a switch statement is. See switchable.
	lt := lowerType(t)
	subject, own := g.switchable(c.Value, lt)
	if subject == nil {
		return nil, false
	}
	next := g.fn.Block()
	if ep.Args != nil && !g.bindCasePayload(t, ep, next) {
		return nil, false
	}
	miss := fail()
	if own == sil.Owned {
		// Every other case hands the copy back, and it is let go of on the
		// way to wherever failing goes.
		other := g.fn.Block()
		other.DestroyValue(other.Arg(lt, sil.Owned))
		other.Br(miss)
		miss = other
	}
	g.blk.SwitchEnum(subject,
		sil.Case{Member: memberName(t, g.text(ep.Name)), Dest: next},
		sil.Case{Dest: miss})
	g.blk = next
	return next, true
}

// matchCondition lowers `case P = v` in a condition list for any pattern:
// v is matched against P, going on with what P binds where it matches
// and failing otherwise. It is the block that goes on, which owns what
// the match took apart.
func (g *gen) matchCondition(c *ast.CaseCond, fail func() *sil.Block) (*sil.Block, bool) {
	t := g.typeOf(c.Value)
	// The value lives for the match: it is borrowed by the patterns,
	// which copy whatever they keep, and let go of on both ways out.
	depth := len(g.scopes)
	g.push()
	v := g.rvalue(c.Value)
	if v == nil {
		g.scopes = g.scopes[:depth]
		return nil, false
	}
	if v.Ownership() == sil.Owned {
		if !g.pendingDestroy(v) {
			g.destroyLater(v)
		}
		borrowed := g.blk.BeginBorrow(v)
		g.endBorrowLater(borrowed)
		v = borrowed
	}
	miss := func() *sil.Block {
		b := g.fn.Block()
		prev := g.blk
		g.blk = b
		g.unwindTo(depth)
		g.blk.Br(fail())
		g.blk = prev
		return b
	}
	m := &matcher{g: g, miss: miss}
	if !m.match(c.Pat, v, t) {
		g.scopes = g.scopes[:depth]
		return nil, false
	}
	g.pop()
	next := g.fn.Block()
	g.blk.Br(next)
	g.blk = next
	if len(m.owned) > 0 {
		if g.armOwned == nil {
			g.armOwned = map[*sil.Block][]*sil.Value{}
		}
		g.armOwned[next] = m.owned
	}
	return next, true
}

// laterBlock returns a lazy block constructor caching the block on first creation.
func (g *gen) laterBlock() func() *sil.Block {
	var b *sil.Block
	return func() *sil.Block {
		if b == nil {
			b = g.fn.Block()
		}
		return b
	}
}

// bindsAName reports whether any condition in the list binds a variable.
func bindsAName(conds []ast.Node) bool {
	for _, c := range conds {
		switch c.(type) {
		case *ast.OptionalBinding, *ast.CaseCond:
			return true
		}
	}
	return false
}

// conditionKind names a condition for diagnostics.
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

// whileStmt lowers while loops (header test, body, and exit).
func (g *gen) whileStmt(s *ast.WhileStmt) {
	header := g.fn.Block()
	body := g.fn.Block()
	exit := g.fn.Block()

	label := g.takeLabel()

	g.blk.Br(header)

	g.blk = header
	outer := len(g.scopes)
	scoped := bindsAName(s.Conds)
	if scoped {
		g.push()
	}
	if !g.conditionChain(s.Conds, func() *sil.Block { return exit }) {
		if scoped {
			g.popReachable()
		}
		return
	}
	g.blk.Br(body)

	g.blk = body
	g.loops = append(g.loops, loop{header: header, exit: exit, depth: outer, label: label})
	g.block(s.Body)
	g.loops = g.loops[:len(g.loops)-1]
	if g.blk != nil && g.blk.Term() == nil {
		if scoped {
			g.emitCleanups(g.scopes[outer])
		}
		g.blk.Br(header)
	}

	g.blk = exit
	if scoped {
		g.scopes = g.scopes[:len(g.scopes)-1]
	}
}

// switchStmt lowers switch statements via switch_enum for enums or comparison chains.
func (g *gen) switchStmt(s *ast.SwitchStmt) {
	subject := g.rvalue(s.Subject)
	if subject == nil {
		return
	}
	clauses := caseClauses(s)
	if len(clauses) == 0 {
		return
	}

	// Create continuation block on demand.
	var contBlk *sil.Block
	cont := func() *sil.Block {
		if contBlk == nil {
			contBlk = g.fn.Block()
		}
		return contBlk
	}

	bodies := make([]*sil.Block, len(clauses))
	for i := range clauses {
		bodies[i] = g.fn.Block()
	}

	subjectType := g.typeOf(s.Subject)
	valueScope := false
	_, isEnum := underlyingEnum(subjectType)
	simple := isEnum
	for _, cs := range clauses {
		for _, item := range cs.Items {
			if !g.simpleCasePattern(item.Pat) {
				simple = false
			}
		}
	}
	if o, isOpt := optionalOf(subjectType); isOpt && g.optionalCasesOnly(clauses) && g.simpleOptionalPatterns(clauses) {
		if !g.switchOnOptional(s, subject, o, clauses, bodies, cont) {
			return
		}
	} else if simple {
		if !g.switchOnEnum(s, subject, subjectType, clauses, bodies, cont) {
			return
		}
	} else {
		// A value switched on is matched, not consumed: it lives through
		// the switch in a scope of its own, and is let go of once, where
		// the arms join or wherever one leaves.
		g.push()
		valueScope = true
		g.destroyLater(subject)
		// The patterns read it borrowed -- a tuple's elements, a name
		// bound to it -- and the borrow ends before it is let go of.
		if subject.Ownership() == sil.Owned {
			borrowed := g.blk.BeginBorrow(subject)
			g.endBorrowLater(borrowed)
			subject = borrowed
		}
		if !g.switchOnPatterns(s, subject, subjectType, clauses, bodies) {
			g.scopes = g.scopes[:len(g.scopes)-1]
			return
		}
	}

	header := (*sil.Block)(nil)
	if l, ok := g.enclosing(""); ok {
		header = l.header
	}
	for i, cs := range clauses {
		g.blk = bodies[i]
		depth := len(g.scopes)
		g.push()
		g.destroyArmOwned(bodies[i])
		g.loops = append(g.loops, loop{header: header, lazyExit: cont, depth: depth, isSwitch: true})
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
			g.scopes = g.scopes[:len(g.scopes)-1]
		}
	}
	g.blk = contBlk
	if valueScope {
		if contBlk != nil {
			g.pop()
		} else {
			g.scopes = g.scopes[:len(g.scopes)-1]
		}
	}
}

// switchOnEnum emits the branch for an enum subject and reports success.
//
// Cases are tried in order, and a case with a where clause that does not
// hold goes on to the ones after it, as swiftc has it. One switch_enum
// cannot do that -- it names each case once and never comes back -- so the
// cases are split after each one with a where clause. Each part switches
// over the subject, keeping a copy for the next part to switch over if
// nothing in this one matched; whichever part matches lets the copy go.
func (g *gen) switchOnEnum(s *ast.SwitchStmt, subject *sil.Value, t types.Type,
	clauses []*ast.CaseClause, bodies []*sil.Block, cont func() *sil.Block) bool {

	var items []enumItem
	defaultClause := -1
	for i, cs := range clauses {
		if cs.Kind == token.DEFAULT {
			defaultClause = i
			continue
		}
		for _, item := range cs.Items {
			itemPat := item.Pat
			if bind, isBind := itemPat.(*ast.ValueBindingPattern); isBind {
				itemPat = bind.Pat
			}
			pat, ok := itemPat.(*ast.EnumCasePattern)
			if !ok || pat.Name == nil {
				g.refuse(item.Pat, "this pattern in a switch over an enum")
				return false
			}
			if len(cs.Items) > 1 && pat.Args != nil && g.bindsNames(pat.Args) {
				g.refuse(item.Pat, "a case of several patterns that binds names")
				return false
			}
			if len(cs.Items) > 1 && item.Where != nil {
				g.refuse(item.Where.Cond, "a where clause on a case of several patterns")
				return false
			}
			items = append(items, enumItem{clause: i, several: len(cs.Items) > 1, item: item, pat: pat})
		}
	}

	trivial := lowerType(t).Trivial()
	cur := subject
	for start := 0; ; {
		end := len(items)
		for j := start; j < len(items); j++ {
			if items[j].item.Where != nil || g.refutablePayload(items[j].pat) {
				end = j + 1
				break
			}
		}
		final := end == len(items) && (end == start ||
			(items[end-1].item.Where == nil && !g.refutablePayload(items[end-1].pat)))

		part := enumPart{final: final}
		if !final {
			part.next = g.fn.Block()
			part.keep = cur
			if !trivial {
				part.keep = g.blk.CopyValue(cur)
			}
			part.dropKeep = !trivial
		}
		var cases []sil.Case
		named := map[string]bool{}
		for _, it := range items[start:end] {
			member := memberName(t, g.text(it.pat.Name))
			if named[member] {
				continue // a case an earlier pattern already took
			}
			named[member] = true
			dest, ok := g.enumItemArm(t, it, bodies[it.clause], part)
			if !ok {
				return false
			}
			cases = append(cases, sil.Case{Member: member, Dest: dest})
		}
		switch {
		case !final:
			cases = append(cases, sil.Case{Dest: part.next})
		case defaultClause >= 0:
			cases = append(cases, sil.Case{Dest: bodies[defaultClause]})
		default:
			// The checker has held the cases to covering every one, so, as
			// in swiftc's SIL, nothing gets past them; lowering wants an
			// edge, and this one leads nowhere.
			cases = append(cases, sil.Case{Dest: g.neverBlock()})
		}
		g.blk.SwitchEnum(cur, cases...)
		if final {
			return true
		}
		g.blk, cur, start = part.next, part.keep, end
	}
}

// An enumItem is one pattern of a switch over an enum, in the clause it is in.
type enumItem struct {
	clause  int
	several bool // the clause has more than this one pattern
	item    *ast.CaseItem
	pat     *ast.EnumCasePattern
}

// An enumPart is one switch_enum of a switch split at its where clauses:
// unless it is the last, what to switch over next and where to do it.
type enumPart struct {
	final    bool
	keep     *sil.Value // the subject for the next part
	dropKeep bool       // keep is a copy, let go of when this part matches
	next     *sil.Block
}

// enumItemArm is the block a case's edge goes to: the body itself where
// nothing stands between them, or an arm that binds the payload, tests the
// where clause, lets go of the kept subject, and joins the body.
func (g *gen) enumItemArm(t types.Type, it enumItem, body *sil.Block, part enumPart) (*sil.Block, bool) {
	pat := it.pat
	// A clause of several patterns -- `case .resized(_), .moved(_):` --
	// has one body and a payload of a different type from each case. Each
	// case goes through an arm of its own that takes what it carries, lets
	// go of it, and joins the body, which takes nothing.
	if it.several && pat.Args != nil && g.refutablePayload(pat) {
		// `.code(1), .quit`: the payload is matched, then let go.
		arm := g.fn.Block()
		var tests []payloadTest
		g.payloadTests = &tests
		bound := g.bindCasePayload(t, pat, arm)
		g.payloadTests = nil
		if !bound {
			return nil, false
		}
		owned := g.armOwned[arm]
		delete(g.armOwned, arm)
		// The body is several arms', and owns none of what they bound.
		if !g.joinArm(arm, body, owned, nil, part, true, tests...) {
			return nil, false
		}
		return arm, true
	}
	if it.several && pat.Args != nil {
		arm := g.fn.Block()
		if k := g.caseOf(t, g.text(pat.Name)); k != nil && k.AssociatedType != nil {
			lt := lowerType(sil.CaseStorage(k))
			own := sil.Unowned
			if !lt.Trivial() {
				own = sil.Owned
			}
			payload := arm.Arg(lt, own)
			if own == sil.Owned {
				arm.DestroyValue(payload)
			}
		}
		if part.dropKeep {
			arm.DestroyValue(part.keep)
		}
		arm.Br(body)
		return arm, true
	}
	if part.final {
		if pat.Args != nil && !g.bindCasePayload(t, pat, body) {
			return nil, false
		}
		return body, true
	}

	arm := g.fn.Block()
	var tests []payloadTest
	g.payloadTests = &tests
	bound := pat.Args == nil || g.bindCasePayload(t, pat, arm)
	g.payloadTests = nil
	if !bound {
		return nil, false
	}
	owned := g.armOwned[arm]
	delete(g.armOwned, arm)
	if !g.joinArm(arm, body, owned, it.item.Where, part, false, tests...) {
		return nil, false
	}
	return arm, true
}

// A payloadTest is a part of what a case carries that its pattern matches
// rather than binds -- the 1 of `.a(1, let x)` -- tested once it is bound.
type payloadTest struct {
	pat ast.Pattern
	v   *sil.Value
	t   types.Type
}

// refutablePayload reports whether a case pattern's payload patterns can
// fail to match what the case carries, so that the case is tried and the
// ones after it are tried where it does not match, as with a where clause.
func (g *gen) refutablePayload(pat *ast.EnumCasePattern) bool {
	return pat.Args != nil && g.refutable(pat.Args)
}

func (g *gen) refutable(p ast.Pattern) bool {
	switch p := p.(type) {
	case nil, *ast.WildcardPattern, *ast.IdentPattern:
		return false
	case *ast.ValueBindingPattern:
		return g.refutable(p.Pat)
	case *ast.TuplePattern:
		for _, el := range p.Elems {
			if g.refutable(el.Pat) {
				return true
			}
		}
		return false
	case *ast.ExprPattern:
		// A name under a let -- `case let .a(n)` -- is bound.
		if ie, ok := p.X.(*ast.IdentExpr); ok && ie.Name != nil && g.info.Defs[ie.Name] != nil {
			return false
		}
	}
	return true
}

// joinArm ends an arm of a split switch that has bound owned: it tests the
// where clause, if there is one, going on to the next part without what
// was bound when it does not hold; lets go of the subject kept for the next
// part; and joins the body, which owns what was bound from then on.
//
// An arm that releases lets go of what it bound before it joins the body.
func (g *gen) joinArm(arm, body *sil.Block, owned []*sil.Value, where *ast.WhereClause, part enumPart,
	release bool, tests ...payloadTest) bool {
	prev := g.blk
	defer func() { g.blk = prev }()
	g.blk = arm
	// What the payload is matched against, part by part: where one does
	// not match, it is let go and the cases after are tried.
	for _, pt := range tests {
		m := &matcher{g: g, miss: func() *sil.Block {
			fails := g.fn.Block()
			for _, v := range owned {
				fails.DestroyValue(v)
			}
			fails.Br(part.next)
			return fails
		}}
		if !m.match(pt.pat, pt.v, pt.t) {
			return false
		}
		owned = append(owned, m.owned...)
	}
	if where != nil {
		g.push()
		cond := g.rvalue(where.Cond)
		if cond == nil {
			return false
		}
		bit := g.machine(cond, types.Typ[types.Bool])
		if bit == nil {
			g.refuse(where.Cond, "a where clause of "+g.typeOf(where.Cond).String())
			return false
		}
		g.pop()
		holds, fails := g.fn.Block(), g.fn.Block()
		g.blk.CondBr(bit, holds, nil, fails, nil)
		// Not this case after all: what it bound is let go, and the
		// cases after it are tried.
		for _, v := range owned {
			fails.DestroyValue(v)
		}
		fails.Br(part.next)
		g.blk = holds
	}
	if part.dropKeep {
		g.blk.DestroyValue(part.keep)
	}
	if release {
		for _, v := range owned {
			g.blk.DestroyValue(v)
		}
		owned = nil
	}
	g.blk.Br(body)
	if len(owned) > 0 {
		if g.armOwned == nil {
			g.armOwned = map[*sil.Block][]*sil.Value{}
		}
		g.armOwned[body] = append(g.armOwned[body], owned...)
	}
	return true
}

// optionalCase is what one case item of a switch over an optional matches:
// the case, and the pattern its payload is bound to, if any.
type optionalCase struct {
	member string      // optionalSome or optionalNone; "" for anything
	inner  ast.Pattern // what the payload is bound to, for some
}

// optionalCaseOf reads a case item of a switch over an optional as a case
// of Optional: `.some(let x)`, `let x?`, `.some`, `.none`, `nil` and `_`.
func (g *gen) optionalCaseOf(pat ast.Pattern) (optionalCase, bool) {
	if bind, ok := pat.(*ast.ValueBindingPattern); ok {
		if c, ok := g.optionalCaseOf(bind.Pat); ok {
			// The binding keyword reaches the names inside.
			if c.inner != nil {
				c.inner = &ast.ValueBindingPattern{Span: bind.Span, Kind: bind.Kind, Pat: c.inner}
			}
			return c, true
		}
		return optionalCase{}, false
	}
	switch p := pat.(type) {
	case *ast.WildcardPattern:
		return optionalCase{}, true
	case *ast.OptionalPattern:
		return optionalCase{member: optionalSome, inner: p.Pat}, true
	case *ast.ExprPattern:
		if lit, ok := p.X.(*ast.BasicLit); ok && lit.Kind == token.NIL {
			return optionalCase{member: optionalNone}, true
		}
	case *ast.EnumCasePattern:
		if p.Name == nil || p.Type != nil {
			return optionalCase{}, false
		}
		switch g.text(p.Name) {
		case "none":
			return optionalCase{member: optionalNone}, p.Args == nil
		case "some":
			if p.Args == nil {
				return optionalCase{member: optionalSome}, true
			}
			if len(p.Args.Elems) == 1 {
				return optionalCase{member: optionalSome, inner: p.Args.Elems[0].Pat}, true
			}
		}
	}
	return optionalCase{}, false
}

// optionalCasesOnly reports whether every item of a switch is a case of
// Optional, which switchOnOptional lowers as a switch on the enum it is.
func (g *gen) optionalCasesOnly(clauses []*ast.CaseClause) bool {
	for _, cs := range clauses {
		for _, item := range cs.Items {
			if _, ok := g.optionalCaseOf(item.Pat); !ok {
				return false
			}
		}
	}
	return true
}

// switchOnOptional emits the branch for an optional subject whose cases
// are all Optional's own.
func (g *gen) switchOnOptional(s *ast.SwitchStmt, subject *sil.Value, o *types.Optional,
	clauses []*ast.CaseClause, bodies []*sil.Block, cont func() *sil.Block) bool {

	wrapped := lowerType(o.Wrapped)
	own := sil.Unowned
	if !wrapped.Trivial() {
		own = sil.Owned
	}
	type optItem struct {
		clause int
		item   *ast.CaseItem
		c      optionalCase
		binds  bool
	}
	var items []optItem
	defaultClause := -1
	for i, cs := range clauses {
		if cs.Kind == token.DEFAULT {
			if defaultClause < 0 {
				defaultClause = i
			}
			continue
		}
		for _, item := range cs.Items {
			c, _ := g.optionalCaseOf(item.Pat)
			binds := c.inner != nil && patternBindsNames(&ast.TuplePattern{Elems: []*ast.TuplePatternElem{{Pat: c.inner}}})
			if binds && len(cs.Items) > 1 {
				g.refuse(item.Pat, "a case of several patterns that binds names")
				return false
			}
			if item.Where != nil && (len(cs.Items) > 1 || c.member == "") {
				g.refuse(item.Where.Cond, "a where clause on this pattern")
				return false
			}
			items = append(items, optItem{clause: i, item: item, c: c, binds: binds})
		}
	}

	// As a switch over an enum is split at its where clauses; see
	// switchOnEnum.
	trivial := lowerType(o).Trivial()
	cur := subject
	for start := 0; ; {
		end := len(items)
		for j := start; j < len(items); j++ {
			if items[j].item.Where != nil {
				end = j + 1
				break
			}
		}
		final := end == len(items) && (end == start || items[end-1].item.Where == nil)
		part := enumPart{final: final}
		if !final {
			part.next = g.fn.Block()
			part.keep = cur
			if !trivial {
				part.keep = g.blk.CopyValue(cur)
			}
			part.dropKeep = !trivial
		}

		var cases []sil.Case
		seen := map[string]bool{}
		var fallback *sil.Block
		for _, it := range items[start:end] {
			member := it.c.member
			if seen[member] || seen[""] {
				continue
			}
			seen[member] = true
			body := bodies[it.clause]
			arm := g.fn.Block()
			var owned []*sil.Value
			if member == optionalSome {
				payload := arm.Arg(wrapped, own)
				if it.binds {
					if own == sil.Owned {
						owned = append(owned, payload)
					}
					if !g.bindPatternTo(it.c.inner, payload, o.Wrapped) {
						return false
					}
				} else if own == sil.Owned {
					// Nothing is kept of what it holds.
					arm.DestroyValue(payload)
				}
			}
			if !g.joinArm(arm, body, owned, it.item.Where, part, false) {
				return false
			}
			if member == "" {
				fallback = arm
				continue
			}
			cases = append(cases, sil.Case{Member: member, Dest: arm})
		}
		// Lowering wants both cases named: whichever no pattern took goes
		// to the default, or on to the next part, or nowhere -- the checker
		// has held the cases to covering both.
		if fallback == nil {
			switch {
			case !final:
				fallback = part.next
			case defaultClause >= 0:
				fallback = bodies[defaultClause]
			case !(seen[optionalSome] && seen[optionalNone]):
				fallback = g.neverBlock()
			}
		}
		if !seen[optionalSome] && !seen[""] {
			arm := g.fn.Block()
			payload := arm.Arg(wrapped, own)
			if own == sil.Owned {
				arm.DestroyValue(payload)
			}
			arm.Br(fallback)
			cases = append(cases, sil.Case{Member: optionalSome, Dest: arm})
		}
		if !seen[optionalNone] && !seen[""] {
			cases = append(cases, sil.Case{Member: optionalNone, Dest: fallback})
		}
		if seen[""] {
			// `_` takes whichever case is left, what it holds let go of
			// on the way.
			if !seen[optionalSome] {
				arm := g.fn.Block()
				payload := arm.Arg(wrapped, own)
				if own == sil.Owned {
					arm.DestroyValue(payload)
				}
				arm.Br(fallback)
				cases = append(cases, sil.Case{Member: optionalSome, Dest: arm})
			}
			if !seen[optionalNone] {
				cases = append(cases, sil.Case{Member: optionalNone, Dest: fallback})
			}
		}
		g.blk.SwitchEnum(cur, cases...)
		if final {
			return true
		}
		g.blk, cur, start = part.next, part.keep, end
	}
}

// switchOnValue emits the comparison chain for a non-enum subject.
func (g *gen) switchOnValue(s *ast.SwitchStmt, subject *sil.Value, t types.Type,
	clauses []*ast.CaseClause, bodies []*sil.Block, cont func() *sil.Block) bool {

	fallback := (*sil.Block)(nil)
	for i, cs := range clauses {
		if cs.Kind == token.DEFAULT {
			fallback = bodies[i]
		}
	}

	for i, cs := range clauses {
		if cs.Kind == token.DEFAULT {
			continue
		}
		for _, item := range cs.Items {
			test, ok := g.caseTest(item, subject, t)
			if !ok {
				return false
			}
			if test == nil {
				g.blk.Br(bodies[i])
				return true
			}
			next := g.fn.Block()
			g.blk.CondBr(test, bodies[i], nil, next, nil)
			g.blk = next
		}
	}
	if fallback == nil {
		// Exhaustive, as the checker holds it: no value gets this far.
		g.blk.Unreachable()
		return true
	}
	g.blk.Br(fallback)
	return true
}

// patternTest is the bit that decides whether a value matches a pattern,
// or nil where it always does, binding whatever the pattern names.
func (g *gen) patternTest(p ast.Pattern, subject *sil.Value, t types.Type) (*sil.Value, bool) {
	var test *sil.Value

	switch pat := p.(type) {
	case *ast.ExprPattern:
		// `case 1...5` matches everything between its bounds, so it
		// is two comparisons rather than one equality.
		if eq := g.rangeMatch(pat, subject, t); eq != nil {
			test = eq
			break
		}
		n := len(g.diags)
		// Compared, not kept: it ends with the test's scope.
		want := g.expr(pat.X)
		if want == nil {
			// Whatever stopped it said so, unless nothing did.
			if len(g.diags) == n {
				g.refuse(p, "this pattern in a switch")
			}
			return nil, false
		}
		test = g.equals(subject, want, t)
		if test == nil {
			g.refuse(p, "a switch over "+t.String())
			return nil, false
		}

	// `case let (a, 1)`: the let reaches each element's pattern.
	case *ast.ValueBindingPattern:
		if tp, isTuple := pat.Pat.(*ast.TuplePattern); isTuple {
			elems := make([]*ast.TuplePatternElem, len(tp.Elems))
			for i, el := range tp.Elems {
				inner := *el
				inner.Pat = &ast.ValueBindingPattern{Span: pat.Span, Kind: pat.Kind, Pat: el.Pat}
				elems[i] = &inner
			}
			return g.patternTest(&ast.TuplePattern{Span: tp.Span, Elems: elems}, subject, t)
		}
		// Under a let, a name is bound; anything else -- the "x" of
		// `let (n, "x")` -- is still matched.
		if ep, isExpr := pat.Pat.(*ast.ExprPattern); isExpr {
			if _, isName := ep.X.(*ast.IdentExpr); !isName {
				return g.patternTest(ep, subject, t)
			}
		}
		if !g.bindPatternTo(p, subject, t) {
			return nil, false
		}

	// Value binding patterns: case let k.
	case *ast.IdentPattern, *ast.WildcardPattern:
		if !g.bindPatternTo(p, subject, t) {
			return nil, false
		}

	// `case Code.refused:` is a static property of Code, compared.
	case *ast.EnumCasePattern:
		recv := g.info.PatternTypes[pat]
		if recv == nil || pat.Name == nil || pat.Args != nil {
			g.refuse(p, "this pattern in a switch")
			return nil, false
		}
		name := g.text(pat.Name)
		var field *types.Field
		for _, f := range g.staticsOf(recv) {
			if f != nil && f.Name == name {
				field = f
			}
		}
		if field == nil {
			g.refuse(p, "a pattern naming something other than a static property")
			return nil, false
		}
		want := g.staticRead(&ast.MemberExpr{Name: pat.Name}, recv, field)
		if want == nil {
			return nil, false
		}
		test = g.equals(subject, want, t)
		if test == nil {
			g.refuse(p, "a switch over "+t.String())
			return nil, false
		}

	// `case (1, let s)` matches each element against its own pattern,
	// and the tuple where all of them match.
	case *ast.TuplePattern:
		tu, isTuple := t.Underlying().(*types.Tuple)
		if !isTuple || len(tu.Elements) != len(pat.Elems) {
			g.refuse(p, "a tuple pattern over "+t.String())
			return nil, false
		}
		for i, el := range pat.Elems {
			et := tu.Elements[i].Type
			part := g.blk.TupleExtract(subject, i, lowerType(et))
			elTest, ok := g.patternTest(el.Pat, part, et)
			if !ok {
				return nil, false
			}
			switch {
			case elTest == nil:
			case test == nil:
				test = elTest
			default:
				test = g.blk.Builtin("and_Int1", sil.Object(sil.BuiltinInt1), test, elTest)
			}
		}

	default:
		g.refuse(p, "this pattern in a switch")
		return nil, false
	}

	return test, true
}

// caseTest is the bit that decides whether one case item matches, or
// nil where it always does. It binds whatever the pattern names
// first, because a where clause is written about those names.
func (g *gen) caseTest(item *ast.CaseItem, subject *sil.Value, t types.Type) (*sil.Value, bool) {
	// What the test makes -- the literal compared with -- ends with it,
	// before the branch.
	g.push()
	test, ok := g.caseItemTest(item, subject, t)
	if !ok {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return nil, false
	}
	g.pop()
	return test, true
}

// caseItemTest is caseTest inside the scope its temporaries end with.
func (g *gen) caseItemTest(item *ast.CaseItem, subject *sil.Value, t types.Type) (*sil.Value, bool) {
	test, ok := g.patternTest(item.Pat, subject, t)
	if !ok {
		return nil, false
	}

	if item.Where == nil {
		return test, true
	}
	cond := g.rvalue(item.Where.Cond)
	if cond == nil {
		return nil, false
	}
	bit := g.machine(cond, types.Typ[types.Bool])
	if bit == nil {
		g.refuse(item.Where.Cond, "a where clause of "+g.typeOf(item.Where.Cond).String())
		return nil, false
	}
	if test == nil {
		return bit, true
	}
	return g.blk.Builtin("and_Int1", sil.Object(sil.BuiltinInt1), test, bit), true
}

// rangeMatch lowers a range pattern test (lo <= subject && subject <= hi).
func (g *gen) rangeMatch(pat *ast.ExprPattern, subject *sil.Value, t types.Type) *sil.Value {
	bin, ok := g.fold(pat.X).(*ast.BinaryExpr)
	if !ok || bin.Op == nil {
		return nil
	}
	upper := ""
	switch g.text(bin.Op) {
	case "...":
		upper = "<="
	case "..<":
		upper = "<"
	default:
		return nil
	}
	lo, hi := g.rvalue(bin.X), g.rvalue(bin.Y)
	if lo == nil || hi == nil {
		return nil
	}
	atLeast := g.compare("<=", lo, subject, t)
	atMost := g.compare(upper, subject, hi, t)
	if atLeast == nil || atMost == nil {
		g.refuse(pat, "a range pattern over "+t.String())
		return nil
	}
	return g.blk.Builtin("and_Int1", sil.Object(sil.BuiltinInt1), atLeast, atMost)
}

// equals tests equality of two values of the same type.
func (g *gen) equals(a, b *sil.Value, t types.Type) *sil.Value {
	return g.compare("==", a, b, t)
}

// compare produces a Builtin.Int1 comparison result.
func (g *gen) compare(op string, a, b *sil.Value, t types.Type) *sil.Value {
	// String and other extern comparisons.
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
	return g.blk.Builtin(bi.Name, sil.Object(builtinNamed(bi.Result)),
		g.machine(a, t), g.machine(b, t))
}

// increment returns v + 1 with overflow checking.
func (g *gen) increment(v *sil.Value, t types.Type) *sil.Value {
	bi, ok := core.Lower("+", t)
	if !ok || !bi.Overflows {
		return nil
	}
	one := g.blk.IntegerLiteral(sil.Object(builtinFor(t)), 1)
	want := g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), -1)
	pair := sil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: builtinNamed(bi.Result)},
		{Type: sil.BuiltinInt1},
	}})
	both := g.blk.Builtin(bi.Name, pair, g.machine(v, t), one, want)
	sum := g.blk.TupleExtract(both, 0, sil.Object(builtinNamed(bi.Result)))
	flag := g.blk.TupleExtract(both, 1, sil.Object(sil.BuiltinInt1))
	g.blk.CondFail(flag, "arithmetic overflow")
	return g.blk.Struct(lowerType(t), sum)
}

// caseClauses returns the case clauses of a switch.
func caseClauses(s *ast.SwitchStmt) []*ast.CaseClause {
	out := make([]*ast.CaseClause, 0, len(s.Cases))
	for _, c := range s.Cases {
		if cs, ok := c.(*ast.CaseClause); ok {
			out = append(out, cs)
		}
	}
	return out
}

// underlyingEnum returns the underlying enum type if present.
func underlyingEnum(t types.Type) (*types.Enum, bool) {
	if t == nil {
		return nil, false
	}
	e, ok := t.Underlying().(*types.Enum)
	return e, ok
}

// repeatStmt lowers repeat-while loops (body executed before condition test).
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

// guardStmt lowers `guard cond else { … }`, requiring the else block to exit.
func (g *gen) guardStmt(s *ast.GuardStmt) {
	miss := g.laterBlock()
	lexical := g.lexical()
	before := len(lexical.cleanups)
	if !g.conditionChain(s.Conds, miss) {
		return
	}
	elseBlk := miss()
	cont := g.blk

	// Cleanups for variables bound in the guard are not active in the else branch.
	bound := append([]cleanup(nil), lexical.cleanups[before:]...)
	lexical.cleanups = lexical.cleanups[:before]
	g.blk = elseBlk
	g.block(s.Body)
	lexical.cleanups = append(lexical.cleanups, bound...)
	if g.blk != nil && g.blk.Term() == nil {
		g.errorAt(s, "the body of a 'guard' must not fall through: "+
			"it has to return, break, continue, or throw")
		g.blk.Unreachable()
	}

	g.blk = cont
}

// labeled records a statement label for break and continue.
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

// takeLabel consumes and returns any pending loop label.
func (g *gen) takeLabel() string {
	label := g.pending
	g.pending = ""
	return label
}

// breakStmt unwinds scopes and branches to the enclosing loop exit.
func (g *gen) breakStmt(s *ast.BreakStmt) {
	g.leave(s, "break", g.label(s.Label), func(l loop) *sil.Block { return l.exitBlock() })
}

// continueStmt unwinds scopes and branches to the loop header.
func (g *gen) continueStmt(s *ast.ContinueStmt) {
	g.leave(s, "continue", g.label(s.Label), func(l loop) *sil.Block { return l.header })
}

// leave resolves the target loop, unwinds intervening scopes, and branches.
func (g *gen) leave(s ast.Stmt, keyword, label string, target func(loop) *sil.Block) {
	l, ok := g.enclosingFor(label, keyword == "continue")
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
// forInBody lowers a for-in's body, which a `where` clause skips for an
// element it is false of, as `continue` would.
func (g *gen) forInBody(s *ast.ForInStmt) {
	if elem := g.loopCase; elem != nil {
		g.loopCase = nil
		g.forCaseBody(s, elem)
		return
	}
	if s.Where == nil {
		g.block(s.Body)
		return
	}
	miss := g.laterBlock()
	if !g.conditionChain([]ast.Node{s.Where.Cond}, miss) {
		return
	}
	skip := miss()
	g.block(s.Body)
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Br(skip)
	}
	g.blk = skip
}

// A loopElement is an element a `for case` loop has taken, owned, and not
// yet matched.
type loopElement struct {
	value *sil.Value
	typ   types.Type
}

// forCaseBody lowers the body of `for case P in s where W`: the element is
// matched against P, and one that does not match -- or whose W is false --
// is skipped, as `continue` would. What P binds lives for the one element.
func (g *gen) forCaseBody(s *ast.ForInStmt, elem *loopElement) {
	skip := g.fn.Block()
	g.push()
	if !g.matchElement(s.Pat, elem, skip) {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return
	}
	if s.Where != nil {
		g.push()
		cond := g.rvalue(s.Where.Cond)
		if cond == nil {
			g.scopes = g.scopes[:len(g.scopes)-2]
			return
		}
		bit := g.machine(cond, types.Typ[types.Bool])
		g.pop()
		body, miss := g.fn.Block(), g.fn.Block()
		g.blk.CondBr(bit, body, nil, miss, nil)
		g.blk = miss
		g.emitCleanups(g.top())
		g.blk.Br(skip)
		g.blk = body
	}
	g.block(s.Body)
	g.popReachable()
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Br(skip)
	}
	g.blk = skip
}

// matchElement matches an owned element against a for-case pattern: an
// optional's `let x?`, `.some(let x)`, `nil` or `.none`, or one case of an
// enum. Where it matches, what it binds is bound for the current scope and
// the lowering goes on; everything else goes to miss.
func (g *gen) matchElement(pat ast.Pattern, elem *loopElement, miss *sil.Block) bool {
	// The element lives for the one iteration: the pattern borrows it,
	// copying whatever it keeps, and it is let go of on both ways out.
	v := elem.value
	if v.Ownership() == sil.Owned {
		g.destroyLater(v)
		borrowed := g.blk.BeginBorrow(v)
		g.endBorrowLater(borrowed)
		v = borrowed
	}
	m := &matcher{g: g, miss: func() *sil.Block {
		b := g.fn.Block()
		prev := g.blk
		g.blk = b
		g.emitCleanups(g.top())
		g.blk.Br(miss)
		g.blk = prev
		return b
	}}
	if !m.match(pat, v, elem.typ) {
		return false
	}
	for _, o := range m.owned {
		g.destroyLater(o)
	}
	return true
}

// forInStmt lowers for-in loops over ranges and collections as counted loops.
func (g *gen) forInStmt(s *ast.ForInStmt) {
	switch {
	case s.Await.IsValid():
		g.refuse(s, "an async for-in")
		return
	case s.Try.IsValid():
		g.refuse(s, "a throwing for-in")
		return
	}
	// A sequence of the program's own: through its iterator.
	if it := g.info.Iterations[s]; it != nil {
		g.forInIterator(s, it)
		return
	}
	if s.Case.IsValid() {
		if _, isArray := g.typeOf(s.Seq).Underlying().(*types.Array); !isArray {
			g.refuse(s, "a for-in with a `case` pattern over something other than an array")
			return
		}
	}

	switch u := g.typeOf(s.Seq).Underlying().(type) {
	case *types.Set:
		g.forInHashTable(s, u.Elem, nil)
		return
	case *types.Dictionary:
		g.forInHashTable(s, u.Key, u.Value)
		return
	}

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

	// Evaluate bounds once before loop entry.
	lo, hi := g.rvalue(bin.X), g.rvalue(bin.Y)
	if lo == nil || hi == nil {
		return
	}

	// Trap if range is invalid (lowerBound > upperBound).
	if bad := g.compare("<", hi, lo, elem); bad != nil {
		g.blk.CondFail(bad, "Range requires lowerBound <= upperBound")
	}

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

	g.blk = header
	index := g.blk.Load(slot, loadQualifier(vt))
	if !rng.Closed {
		more := g.compare("<", index, hi, elem)
		if more == nil {
			g.refuse(s.Seq, "a range of "+elem.String())
			return
		}
		body := g.fn.Block()
		g.blk.CondBr(more, body, nil, exit, nil)
		g.blk = body
		index = g.blk.Load(slot, loadQualifier(vt))
	}

	depth := len(g.scopes)
	g.push()
	g.bindLoopVar(s.Pat, index, vt)

	g.loops = append(g.loops, loop{header: latch, exit: exit, depth: depth, label: label})
	g.forInBody(s)
	g.loops = g.loops[:len(g.loops)-1]
	if g.blk != nil && g.blk.Term() == nil {
		g.pop()
		g.blk.Br(latch)
	} else {
		g.scopes = g.scopes[:len(g.scopes)-1]
	}

	// Advance loop variable at latch.
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

// bindLoopVar binds a for-in pattern to the current iteration value.
func (g *gen) bindLoopVar(pat ast.Pattern, v *sil.Value, t sil.Type) {
	// `for (i, v) in pairs`: the element taken apart, each name bound
	// to its part.
	if tp, ok := untyped(pat).(*ast.TuplePattern); ok && t.Formal() != nil {
		if v.Ownership() != sil.Owned && !t.Trivial() {
			v = g.blk.CopyValue(v)
		}
		g.bindTuple(tp, v, t.Formal(), true)
		return
	}
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

// patternIdent extracts the identifier from a pattern, or returns nil.
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

// patternName returns a descriptive name for the synthetic loop index.
func (g *gen) patternName(pat ast.Pattern) string {
	if name := patternIdent(pat); name != nil {
		return g.text(name)
	}
	return "i"
}

// fold resolves sequence expressions using folded operator precedence.
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

// seqKind returns a sequence description for diagnostics.
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
	case *types.Set:
		return "a set"
	}
	return "'" + t.String() + "'"
}

// bindCasePayload binds enum associated values to pattern names.
func (g *gen) bindCasePayload(subject types.Type, pat *ast.EnumCasePattern, arm *sil.Block) bool {
	assoc := g.caseAssociated(subject, g.text(pat.Name))
	if assoc == nil {
		g.refuse(pat, "a pattern binding a value on a case that carries none")
		return false
	}
	if g.armOwned == nil {
		g.armOwned = map[*sil.Block][]*sil.Value{}
	}
	// What a case carries leaves the enum the switch consumed, so an arm
	// owns a counted payload and lets it go when its scope ends.
	own := sil.Unowned
	if !lowerType(assoc).Trivial() {
		own = sil.Owned
	}
	var payload *sil.Value
	if k := g.caseOf(subject, g.text(pat.Name)); k != nil && k.Indirect {
		// An indirect case hands over its box: the arm owns the box, and
		// a copy of what the box holds.
		box := arm.Arg(lowerType(sil.CaseStorage(k)), sil.Owned)
		g.armOwned[arm] = append(g.armOwned[arm], box)
		addr := arm.ProjectBox(box, 0, lowerType(assoc))
		payload = arm.Load(addr, loadQualifier(lowerType(assoc)))
	} else {
		payload = arm.Arg(lowerType(assoc), own)
	}
	if own == sil.Owned {
		g.armOwned[arm] = append(g.armOwned[arm], payload)
	}
	names := pat.Args.Elems
	if len(names) == 1 {
		return g.bindPatternTo(names[0].Pat, payload, assoc)
	}
	tu, ok := assoc.Underlying().(*types.Tuple)
	if !ok || len(tu.Elements) != len(names) {
		g.refuse(pat, "a pattern whose names do not match what the case carries")
		return false
	}
	prev := g.blk
	g.blk = arm
	// An owned tuple is taken apart, and the arm owns each counted part
	// of it instead of the whole.
	var parts []*sil.Value
	if own == sil.Owned {
		elems := make([]sil.Type, len(tu.Elements))
		for i, el := range tu.Elements {
			elems[i] = lowerType(el.Type)
		}
		parts = g.blk.DestructureTuple(payload, elems...)
		owned := g.armOwned[arm]
		g.armOwned[arm] = owned[:len(owned)-1]
		for i, part := range parts {
			if !elems[i].Trivial() {
				g.armOwned[arm] = append(g.armOwned[arm], part)
			}
		}
	}
	for i, el := range names {
		var v *sil.Value
		if parts != nil {
			v = parts[i]
		} else {
			v = g.blk.TupleExtract(payload, i, lowerType(tu.Elements[i].Type))
		}
		if !g.bindPatternTo(el.Pat, v, tu.Elements[i].Type) {
			g.blk = prev
			return false
		}
	}
	g.blk = prev
	return true
}

// destroyArmOwned lets go of what an arm's case carried when the scope
// just pushed for the arm ends.
func (g *gen) destroyArmOwned(arm *sil.Block) {
	for _, v := range g.armOwned[arm] {
		g.destroyLater(v)
	}
	delete(g.armOwned, arm)
}

// bindsNames reports whether a case's argument patterns bind a name.
func (g *gen) bindsNames(args *ast.TuplePattern) bool {
	found := false
	ast.Inspect(args, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IdentPattern:
			found = true
		case *ast.ExprPattern:
			if ie, ok := n.X.(*ast.IdentExpr); ok && ie.Name != nil && g.info.Defs[ie.Name] != nil {
				found = true
			}
		}
		return !found
	})
	return found
}

// patternBindsNames reports whether a case's argument patterns bind any
// name, rather than only matching with `_`.
func patternBindsNames(args *ast.TuplePattern) bool {
	found := false
	ast.Inspect(args, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IdentPattern:
			found = true
		case *ast.ExprPattern:
			found = true
		}
		return !found
	})
	return found
}

// bindPatternTo binds a single pattern name to a value.
func (g *gen) bindPatternTo(p ast.Pattern, v *sil.Value, t types.Type) bool {
	if bind, ok := p.(*ast.ValueBindingPattern); ok {
		p = bind.Pat
	}
	// `_` binds nothing. What the case carries is let go with the arm's
	// scope all the same.
	if _, ok := p.(*ast.WildcardPattern); ok {
		return true
	}
	// Under a let written before the case -- `case let .rect(w, h)` -- a
	// name is written as an expression.
	if ep, ok := p.(*ast.ExprPattern); ok {
		if ie, isIdent := ep.X.(*ast.IdentExpr); isIdent && ie.Name != nil {
			if sym := g.info.Defs[ie.Name]; sym != nil {
				g.locals[sym] = &local{value: v, typ: lowerType(t)}
				return true
			}
		}
	}
	id, ok := p.(*ast.IdentPattern)
	if !ok || id.Name == nil {
		// Matched rather than bound: tested where the arm joins its body.
		if g.payloadTests != nil && g.refutable(p) {
			*g.payloadTests = append(*g.payloadTests, payloadTest{pat: p, v: v, t: t})
			return true
		}
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

// caseAssociated returns the associated type for a named enum case.
func (g *gen) caseAssociated(subject types.Type, name string) types.Type {
	if c := g.caseOf(subject, name); c != nil {
		return c.AssociatedType
	}
	return nil
}

// caseOf is the case of the enum subject named name, or nil.
func (g *gen) caseOf(subject types.Type, name string) *types.EnumCase {
	if meta, ok := subject.(*types.Metatype); ok {
		subject = meta.Instance
	}
	e, ok := subject.Underlying().(*types.Enum)
	if !ok {
		return nil
	}
	for _, c := range e.Cases {
		if c != nil && c.Name == name {
			return c
		}
	}
	return nil
}

// neverBlock is a block no path reaches, for an edge lowering needs.
func (g *gen) neverBlock() *sil.Block {
	b := g.fn.Block()
	b.Unreachable()
	return b
}
