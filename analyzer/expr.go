package analyzer

import (
	"fmt"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// checkExpr checks expression expr against an optional expected type and returns its semantic type.
func (c *checker) checkExpr(expr ast.Expr, expected types.Type, scope *Scope) types.Type {
	if expr == nil {
		return types.Typ[types.Invalid]
	}

	// The root of an optional chain -- `a?.b.c()` -- is evaluated with every
	// step seeing the unwrapped value, and is the optional of what the last
	// step gives.
	if c.chainRoot(expr, scope) {
		c.markChain(expr)
		inner := c.evalExpr(expr, nil, scope)
		if inner == nil {
			inner = types.Typ[types.Invalid]
		}
		c.info.ChainInner[expr] = inner
		var typ types.Type = inner
		if _, already := inner.(*types.Optional); !already && !isInvalid(inner) && !types.Identical(inner, types.Typ[types.Void]) {
			typ = &types.Optional{Wrapped: inner}
		}
		if types.Identical(inner, types.Typ[types.Void]) {
			typ = &types.Optional{Wrapped: inner}
		}
		c.info.ChainRoots[expr] = true
		c.info.Types[expr] = typ
		return typ
	}
	// Inside an extension of a built-in type, a member of it named alone
	// -- `count`, `removeLast()` -- is that member of self.
	if synth := c.implicitSelfMember(expr, scope); synth != nil {
		c.info.ImplicitSelf[expr] = synth
		typ := c.checkExpr(synth, expected, scope)
		c.info.Types[expr] = typ
		return typ
	}
	typ := c.evalExpr(expr, expected, scope)
	if typ == nil {
		typ = types.Typ[types.Invalid]
	}
	if o, ok := typ.(*types.Optional); ok && o.Implicit {
		typ = c.implicitlyUnwrapped(expr, o, expected)
	}
	c.info.Types[expr] = typ
	return typ
}

// implicitlyUnwrapped is the type of a value of an implicitly unwrapped
// optional. As Swift does, it is the optional wherever the optional will do,
// and is unwrapped only where it would not and the type it wraps would. Taken
// as it is, it is a plain optional: `let p = f()` makes p a T?.
func (c *checker) implicitlyUnwrapped(expr ast.Expr, o *types.Optional, expected types.Type) types.Type {
	plain := &types.Optional{Wrapped: o.Wrapped}
	if expected == nil || types.AssignableTo(plain, expected) || !types.AssignableTo(o.Wrapped, expected) {
		c.implicit[expr] = true
		return plain
	}
	c.info.Unwrapped[expr] = plain
	return o.Wrapped
}

// unwrapImplicit unwraps a value already checked as an implicitly unwrapped
// optional, where only the wrapped type can be used: as the base of a member.
func (c *checker) unwrapImplicit(expr ast.Expr, t types.Type) types.Type {
	o, ok := t.(*types.Optional)
	if !ok || !c.implicit[expr] {
		return t
	}
	c.info.Unwrapped[expr] = o
	c.info.Types[expr] = o.Wrapped
	return o.Wrapped
}

// reconcileLiterals settles the type of a numeric literal against what it is combined with.
func (c *checker) reconcileLiterals(x ast.Expr, lhs types.Type, y ast.Expr, rhs types.Type, scope *Scope) (types.Type, types.Type) {
	if types.Identical(lhs, rhs) {
		return lhs, rhs
	}
	// A literal beside a value of a type expressible by it is one of that
	// type: `c == "a"` with c a Character.
	if t, ok := c.adoptExpressed(x, rhs, scope); ok {
		return t, rhs
	}
	if t, ok := c.adoptExpressed(y, lhs, scope); ok {
		return lhs, t
	}
	// A literal beside an optional is of what the optional wraps:
	// `Double("2") == -1000` compares with a Double.
	if o, ok := lhs.(*types.Optional); ok && !types.Identical(o.Wrapped, rhs) {
		if _, isOpt := rhs.(*types.Optional); !isOpt {
			if t, ok := c.adopt(y, o.Wrapped); ok {
				return lhs, t
			}
			if t, ok := c.adoptTree(y, o.Wrapped, scope); ok {
				return lhs, t
			}
		}
	}
	if o, ok := rhs.(*types.Optional); ok && !types.Identical(o.Wrapped, lhs) {
		if _, isOpt := lhs.(*types.Optional); !isOpt {
			if t, ok := c.adopt(x, o.Wrapped); ok {
				return t, rhs
			}
			if t, ok := c.adoptTree(x, o.Wrapped, scope); ok {
				return t, rhs
			}
		}
	}
	if t, ok := c.adopt(x, rhs); ok {
		return t, rhs
	}
	if t, ok := c.adopt(y, lhs); ok {
		return lhs, t
	}
	if t, ok := c.adoptTree(x, rhs, scope); ok {
		return t, rhs
	}
	if t, ok := c.adoptTree(y, lhs, scope); ok {
		return lhs, t
	}
	return lhs, rhs
}

// adoptExpressed re-reads a literal as want, where want is a type of a
// program's own, or the core's, that is expressible by it.
func (c *checker) adoptExpressed(e ast.Expr, want types.Type, scope *Scope) (types.Type, bool) {
	if want == nil {
		return nil, false
	}
	switch want.Underlying().(type) {
	case *types.Basic, *types.Optional:
		return nil, false
	}
	var kind types.BasicKind
	switch lit := unparen(e).(type) {
	case *ast.StringLit:
		kind = types.UntypedString
	case *ast.BasicLit:
		switch lit.Kind {
		case token.INT_LIT:
			kind = types.UntypedInt
		case token.FLOAT_LIT:
			kind = types.UntypedFloat
		case token.TRUE, token.FALSE:
			kind = types.UntypedBool
		default:
			return nil, false
		}
	default:
		return nil, false
	}
	for _, p := range types.LiteralProtocols(kind) {
		if types.ConformsToNamed(want, p) {
			return c.checkExpr(e, want, scope), true
		}
	}
	return nil, false
}

// adoptTree re-evaluates an expression made only of literals against want.
func (c *checker) adoptTree(e ast.Expr, want types.Type, scope *Scope) (types.Type, bool) {
	if want == nil || !c.isLiteralTree(e) {
		return nil, false
	}
	b, ok := want.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsNumeric == 0 {
		return nil, false
	}
	return c.checkExpr(e, want, scope), true
}

// isLiteralTree reports whether an expression consists entirely of numeric literals and operators.
func (c *checker) isLiteralTree(e ast.Expr) bool {
	switch n := e.(type) {
	case *ast.BasicLit:
		return n.Kind == token.INT_LIT || n.Kind == token.FLOAT_LIT
	// #line and #column are integer literals too.
	case *ast.MagicLit:
		return n.Kind == token.POUND_LINE || n.Kind == token.POUND_COLUMN
	case *ast.ParenExpr:
		return c.isLiteralTree(n.X)
	case *ast.PrefixExpr:
		return c.isLiteralTree(n.X)
	// `c ? 1 : 0` is a literal whichever way it goes, and takes its type
	// from what it is combined with: `year + (m <= 2 ? 1 : 0)`.
	case *ast.ConditionalExpr:
		return c.isLiteralTree(n.Then) && c.isLiteralTree(n.Else)
	// An operator sequence is whatever it folded into.
	case *ast.SequenceExpr:
		if folded, ok := c.info.Folded[n]; ok && folded != nil {
			return c.isLiteralTree(folded)
		}
		return false
	case *ast.BinaryExpr:
		if n.Op == nil {
			return false
		}
		op := n.Op.Text(c.file)
		return sharesOperandType(op) && c.isLiteralTree(n.X) && c.isLiteralTree(n.Y)
	}
	return false
}

// adopt assigns want type to a numeric literal if compatible, returning success.
func (c *checker) adopt(e ast.Expr, want types.Type) (types.Type, bool) {
	if want == nil {
		return nil, false
	}
	lit, ok := literalUnder(e)
	if !ok {
		return nil, false
	}
	var untyped types.Type
	switch lit.Kind {
	case token.INT_LIT:
		untyped = types.Typ[types.UntypedInt]
	case token.FLOAT_LIT:
		untyped = types.Typ[types.UntypedFloat]
	default:
		return nil, false
	}
	b, ok := want.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsNumeric == 0 || !types.AssignableTo(untyped, want) {
		return nil, false
	}
	c.info.Types[e] = want
	c.info.Types[lit] = want
	return want, true
}

// literalUnder unwraps an optional leading sign to extract a basic literal.
func literalUnder(e ast.Expr) (*ast.BasicLit, bool) {
	if p, ok := e.(*ast.PrefixExpr); ok && p.Op != nil {
		if inner, ok := p.X.(*ast.BasicLit); ok {
			e = inner
		}
	}
	lit, ok := e.(*ast.BasicLit)
	return lit, ok
}

// checkPostfix is a postfix operator: one the program declares, or
// `a...`, a range with no upper bound.
func (c *checker) checkPostfix(e *ast.PostfixExpr, expected types.Type, scope *Scope) types.Type {
	op := ""
	if e.Op != nil {
		op = e.Op.Text(c.file)
	}
	inner := c.checkExpr(e.X, c.rangeElement(unwrappedContext(expected)), scope)
	if isInvalid(inner) {
		return inner
	}
	if t, ok := c.resolveOperator(scope, op, e, []ast.Expr{e.X}, []types.Type{inner}); ok {
		return t
	}
	if op == "..." {
		return c.rangeOf("PartialRangeFrom", literalDefault(inner))
	}
	c.typeErrorf(e.Pos(), "cannot find operator '%s' in scope", op)
	return types.Typ[types.Invalid]
}

// checkPrefix type-checks a unary prefix expression.
func (c *checker) checkPrefix(e *ast.PrefixExpr, expected types.Type, scope *Scope) types.Type {
	expected = unwrappedContext(expected)
	op := ""
	if e.Op != nil {
		op = e.Op.Text(c.file)
	}
	if lit, ok := e.X.(*ast.BasicLit); ok && (op == "-" || op == "+") {
		c.negated[lit] = op == "-"
	}
	// `..<b` and `...b` are ranges with no lower bound, whose operand is
	// the bound.
	if op == "..<" || op == "..." {
		inner := literalDefault(c.checkExpr(e.X, c.rangeElement(expected), scope))
		if isInvalid(inner) {
			return inner
		}
		if op == "..<" {
			return c.rangeOf("PartialRangeUpTo", inner)
		}
		return c.rangeOf("PartialRangeThrough", inner)
	}
	inner := c.checkExpr(e.X, expected, scope)
	// The signed value belongs to the expression, so that whatever
	// reads a constant finds one here rather than an operator applied
	// to a magnitude. This used to be recorded in c.negated and never
	// read again, which left `-128` as a negation of 128 all the way
	// down to lowering, where it was refused.
	c.foldSign(e, op)
	if t, ok := c.resolveOperator(scope, op, e, []ast.Expr{e.X}, []types.Type{inner}); ok {
		return t
	}
	switch op {
	case "-", "+":
		if isInvalid(inner) {
			return inner
		}
		if b, ok := inner.Underlying().(*types.Basic); ok && b.Info()&types.IsNumeric != 0 {
			return inner
		}
		c.typeErrorf(e.Pos(), "unary operator '%s' cannot be applied to an operand of type '%s'", op, inner)
		return types.Typ[types.Invalid]
	case "!":
		if isInvalid(inner) {
			return inner
		}
		if types.Identical(inner, types.Typ[types.Bool]) {
			return types.Typ[types.Bool]
		}
		c.typeErrorf(e.Pos(), "unary operator '!' cannot be applied to an operand of type '%s'", inner)
		return types.Typ[types.Invalid]
	}
	return types.Typ[types.Invalid]
}

// checkStmtExpr checks an if or a switch used as a value (SE-0380). It
// is checked as the statement it is -- conditions binding names for their
// branch, a switch covering its subject -- with the expression each branch
// consists of checked for what the context wants; the branches must agree
// on a type, which is the expression's.
func (c *checker) checkStmtExpr(e *ast.StmtExpr, expected types.Type, scope *Scope) types.Type {
	vals, ok := BranchValues(e.Stmt)
	if !ok {
		c.checkStmt(e.Stmt, scope)
		c.typeErrorf(e.Pos(), "each branch of an 'if' or 'switch' expression must be a single expression, and an 'if' must have an 'else'")
		return types.Typ[types.Invalid]
	}
	if c.branchValues == nil {
		c.branchValues = map[*ast.ExprStmt]types.Type{}
	}
	for _, v := range vals {
		c.branchValues[v] = expected
	}
	c.checkStmt(e.Stmt, scope)

	var result types.Type
	for _, v := range vals {
		t := c.info.Types[v.X]
		switch {
		case t == nil || isInvalid(t):
			return types.Typ[types.Invalid]
		case types.Identical(t, types.Typ[types.Never]):
			// A branch that does not come back gives no value.
		case expected != nil:
			if !types.AssignableTo(t, expected) {
				c.typeErrorf(v.X.Pos(), "cannot convert value of type '%s' to specified type '%s'", t, expected)
				return types.Typ[types.Invalid]
			}
			result = expected
		case result == nil:
			result = literalDefault(t)
		case !types.Identical(result, literalDefault(t)) && !types.AssignableTo(t, result):
			c.typeErrorf(v.X.Pos(), "branches have mismatching types '%s' and '%s'", result, t)
			return types.Typ[types.Invalid]
		}
	}
	if result == nil {
		return types.Typ[types.Never]
	}
	return result
}

// BranchValues is the expression statement each branch of an if or a
// switch used as a value consists of, in order; a branch that is itself an
// if or a switch gives its own branches'. A branch may instead be a throw,
// which gives nothing. It reports false where s cannot be a value: a
// branch of more than one statement, or an if without an else.
func BranchValues(s ast.Stmt) ([]*ast.ExprStmt, bool) {
	var out []*ast.ExprStmt
	var walk func(ast.Stmt) bool
	branch := func(stmts []ast.Stmt) bool {
		if len(stmts) != 1 {
			return false
		}
		switch st := stmts[0].(type) {
		case *ast.ExprStmt:
			out = append(out, st)
			return true
		case *ast.ThrowStmt:
			return true
		case *ast.IfStmt, *ast.SwitchStmt:
			return walk(st)
		}
		return false
	}
	walk = func(s ast.Stmt) bool {
		switch n := s.(type) {
		case *ast.IfStmt:
			if n.Body == nil || n.Else == nil || !branch(n.Body.Stmts) {
				return false
			}
			switch e := n.Else.(type) {
			case *ast.IfStmt:
				return walk(e)
			case *ast.CodeBlock:
				return branch(e.Stmts)
			}
		case *ast.SwitchStmt:
			for _, cs := range n.Cases {
				clause, ok := cs.(*ast.CaseClause)
				if !ok || !branch(clause.Stmts) {
					return false
				}
			}
			return len(n.Cases) > 0
		}
		return false
	}
	if !walk(s) {
		return nil, false
	}
	return out, true
}

// checkInterpolation checks string interpolation expressions in scope.
func (c *checker) checkInterpolation(in *ast.Interpolation, scope *Scope) {
	if in.X != nil {
		c.checkExpr(in.X, nil, scope)
	}
	for _, arg := range in.Args {
		c.checkExpr(arg.X, nil, scope)
	}
}

// adopts reports whether an untyped literal can take the expected type.
func adopts(want, untyped types.Type) bool {
	if want == nil {
		return false
	}
	// Nor where an optional of one is: `let a: Any? = 3` is an Int in it.
	inner := want
	for {
		o, ok := inner.(*types.Optional)
		if !ok {
			break
		}
		inner = o.Wrapped
	}
	switch inner.(type) {
	case *types.Existential, *types.Protocol:
		return false
	}
	return types.AssignableTo(untyped, want)
}

func (c *checker) evalExpr(expr ast.Expr, expected types.Type, scope *Scope) types.Type {
	switch e := expr.(type) {
	case *ast.BasicLit:
		c.valueOf(e)
		switch e.Kind {
		case token.INT_LIT:
			if t, ok := c.expressedLiteral(e, expected, types.UntypedInt); ok {
				return t
			}
			if adopts(expected, types.Typ[types.UntypedInt]) {
				return expected
			}
			return types.Typ[types.Int]
		case token.FLOAT_LIT:
			if t, ok := c.expressedLiteral(e, expected, types.UntypedFloat); ok {
				return t
			}
			if adopts(expected, types.Typ[types.UntypedFloat]) {
				return expected
			}
			return types.Typ[types.Double]
		case token.TRUE, token.FALSE:
			if t, ok := c.expressedLiteral(e, expected, types.UntypedBool); ok {
				return t
			}
			return types.Typ[types.Bool]
		case token.NIL:
			if expected != nil {
				if _, ok := expected.(*types.Optional); ok {
					return expected
				}
			}
			if t, ok := c.expressedLiteral(e, expected, types.UntypedNil); ok {
				return t
			}
			return types.Typ[types.UntypedNil]
		default:
			// A regular expression literal. Its type is Regex, which
			// is a library type this compiler does not have.
			return types.Typ[types.Invalid]
		}

	case *ast.StringLit:
		c.valueOf(e)
		for _, seg := range e.Segments {
			if in, ok := seg.(*ast.Interpolation); ok {
				c.checkInterpolation(in, scope)
			}
		}
		if adopts(expected, types.Typ[types.UntypedString]) {
			if t, ok := c.expressedLiteral(e, expected, types.UntypedString); ok {
				return t
			}
			return expected
		}
		return types.Typ[types.String]

	// Parentheses group; they do not change what is inside them.
	case *ast.ParenExpr:
		return c.checkExpr(e.X, expected, scope)

	// `self` and `Self` inside a member: the type the member is
	// written in, and its metatype.
	case *ast.SelfExpr:
		// Inside a closure that captured `[weak self]`, self is that.
		if v, ok := scope.Lookup("self").(*VarSymbol); ok {
			c.info.SelfVars[e] = v
			return v.Type()
		}
		if c.currType == nil {
			c.errorf(e.Pos(), "'self' is only available in a member")
			return types.Typ[types.Invalid]
		}
		return c.currType

	case *ast.SuperExpr:
		if cl, ok := c.currType.(*types.Class); ok && cl.Superclass != nil {
			return cl.Superclass
		}
		c.errorf(e.Pos(), "'super' is only available in a class with a superclass")
		return types.Typ[types.Invalid]

	// `self.init`, `super.init`, `T.init`: the type's initializers, called
	// as the type is -- `T.init(x)` is `T(x)` -- which picks among them.
	case *ast.InitRefExpr:
		var t types.Type
		switch e.X.(type) {
		case *ast.SelfExpr, *ast.SuperExpr:
			t = c.checkExpr(e.X, nil, scope)
			if !isInvalid(t) {
				t = &types.Metatype{Instance: t}
			}
		default:
			t = c.checkExpr(e.X, nil, scope)
			// Named and not called: the closure making one.
			if meta, ok := t.(*types.Metatype); ok && !c.callees[e] {
				want, _ := expected.(*types.Signature)
				if rt, ok := c.initReference(e, e.X, e.Lparen.IsValid(), e.Names, meta.Instance, want, scope); ok {
					return rt
				}
			}
		}
		return t

	// A key path where a function is wanted -- `people.map(\.name)` -- is
	// the function that reads through it, `{ $0.name }` (SE-0249).
	case *ast.KeyPathExpr:
		if closure := c.keyPathClosure(e); closure != nil && wantsFunctionOfOne(expected) {
			return c.checkExpr(closure, expected, scope)
		}
		return c.keyPathValue(e, expected, scope)

	// `X.self` is the value X names, which for a type is its
	// metatype; `T.Type` written as an expression is the same thing.
	case *ast.PostfixSelfExpr:
		t := c.checkExpr(e.X, nil, scope)
		// `[Int].self` and `[String: Int].self`: a collection literal of
		// types is the collection type.
		switch u := t.(type) {
		case *types.Array:
			if m, ok := u.Elem.(*types.Metatype); ok {
				return &types.Metatype{Instance: &types.Array{Elem: m.Instance}}
			}
		case *types.Dictionary:
			k, kok := u.Key.(*types.Metatype)
			v, vok := u.Value.(*types.Metatype)
			if kok && vok {
				return &types.Metatype{Instance: &types.Dictionary{Key: k.Instance, Value: v.Instance}}
			}
		case *types.Tuple:
			// `(Int, Int).self`: a tuple of types is the tuple type.
			elems := make([]*types.TupleElement, 0, len(u.Elements))
			for _, el := range u.Elements {
				m, ok := el.Type.(*types.Metatype)
				if !ok {
					return t
				}
				elems = append(elems, &types.TupleElement{Name: el.Name, Type: m.Instance})
			}
			return &types.Metatype{Instance: &types.Tuple{Elements: elems}}
		}
		return t

	case *ast.TypeExpr:
		return &types.Metatype{Instance: c.resolveType(e.Type, scope)}

	// `&x` in an argument: the type is the operand's, and what the
	// ampersand says is about how it is passed.
	case *ast.InOutExpr:
		// Unless a pointer is wanted, in which case it is Swift's
		// inout-to-pointer conversion: `&x` where the parameter is an
		// `UnsafeMutablePointer<T>` hands over where x lives. The
		// storage is x's either way -- what differs is that the
		// callee is given an address rather than an inout binding,
		// which is what a C function takes.
		//
		// A nullable C pointer is an optional here, and `&x` is never
		// null, so the expectation is unwrapped first and the
		// injection happens above this the way it does for any other
		// value written where an optional is wanted.
		if p, ok := unwrappedContext(expected).(*types.Pointer); ok && p.Dereferenceable() {
			got := c.checkExpr(e.X, p.Elem, scope)
			if types.Identical(got, p.Elem) {
				return p
			}
			return got
		}
		return c.checkExpr(e.X, expected, scope)

	// The prefix operators Swift declares on its numeric types. Any
	// other spelling is a declared operator, which needs a
	// declaration to resolve against.
	case *ast.PrefixExpr:
		return c.checkPrefix(e, expected, scope)

	case *ast.PostfixExpr:
		return c.checkPostfix(e, expected, scope)

	// An if or a switch standing where a value goes: its type is the
	// type its branches agree on.
	case *ast.StmtExpr:
		return c.checkStmtExpr(e, expected, scope)

	case *ast.MagicLit:
		switch e.Kind {
		case token.POUND_LINE, token.POUND_COLUMN:
			// The place the literal is written, known here; as an integer
			// literal it takes the integer type the context wants.
			p := c.file.Position(e.Pos())
			n := p.Line
			if e.Kind == token.POUND_COLUMN {
				n = p.Column
			}
			c.info.Values[e] = Value{Kind: IntValue, Int: uint64(n)}
			if adopts(expected, types.Typ[types.UntypedInt]) {
				return expected
			}
			return types.Typ[types.Int]
		case token.POUND_DSOHANDLE:
			return types.Typ[types.Invalid]
		}
		// The file forms are spelled by lowering, which knows the module.
		if e.Kind == token.POUND_FUNCTION {
			c.info.Values[e] = Value{Kind: StringValue, Str: c.currFuncName}
		}
		if adopts(expected, types.Typ[types.UntypedString]) {
			return expected
		}
		return types.Typ[types.String]

	case *ast.IdentExpr:
		name := e.Name.Text(c.file)
		// `UnsafeMutablePointer<Int32>(p)` names a type and is a
		// conversion. The typed pointers are spelled with a type
		// argument, so the bare name is not a type and has no symbol
		// -- with an argument it names one, and this is where that
		// argument is in hand.
		if e.Args != nil && len(e.Args.Args) == 1 {
			if p, ok := pointerType(name, c.resolveType(e.Args.Args[0], scope)); ok {
				return &types.Metatype{Instance: p}
			}
		}
		sym := c.lookupValue(scope, name)
		if c.inPropertyInit && c.instanceMember(scope, name) {
			c.errorf(e.Name.Pos(), "cannot use instance member '%s' within property initializer; "+
				"property initializers run before 'self' is available", name)
			return types.Typ[types.Invalid]
		}
		if sym == nil {
			// A builtin type's name: it is in no scope, and in
			// expression position it denotes its own metatype.
			if u := types.LookupUniverse(name); u != nil {
				return &types.Metatype{Instance: u}
			}
			c.errorf(e.Name.Pos(), "cannot find '%s' in scope", name)
			return types.Typ[types.Invalid]
		}
		// A name an `async let` bound is its child task's value,
		// awaited: `a` reads as `a.value` on the Task it holds.
		if v, ok := sym.(*VarSymbol); ok && c.asyncLets[v] && !c.asyncLetReads[e] {
			at := ast.Span{Lo: e.Pos(), Hi: e.End()}
			inner := &ast.IdentExpr{Span: at, Name: e.Name}
			c.asyncLetReads[inner] = true
			read := &ast.MemberExpr{Span: at, X: inner, Dot: e.Pos(), Name: &ast.Ident{Span: at, Synth: "value"}}
			t := c.checkExpr(read, expected, scope)
			c.info.ImplicitSelf[e] = read
			c.info.Uses[e.Name] = sym
			return t
		}
		if v, ok := sym.(*VarSymbol); ok {
			if !v.IsInitialized() {
				c.errorf(e.Name.Pos(), "'%s' used before being initialized", name)
			}
			if v.IsConsumed() {
				c.errorf(e.Name.Pos(), "'%s' used after consume", name)
			}
			if v.isolated {
				c.checkIsolatedAccess(e, "var", name, false)
			}
		}
		c.info.Uses[e.Name] = sym
		c.checkAccess(e.Name.Pos(), e.Name.Text(c.file), sym)
		// A type's name in expression position denotes the type, not
		// a value of it: `Int` is `Int.Type`, which is what makes
		// `Int.self` a metatype and `Box(v: 3)` an initializer call.
		if _, ok := sym.(*TypeNameSymbol); ok {
			instance := sym.Type()
			// `Stack<Int>()` says which instance is being made
			// outright, rather than leaving it to be inferred.
			if e.Args != nil && len(e.Args.Args) > 0 {
				args := make([]types.Type, len(e.Args.Args))
				for i, a := range e.Args.Args {
					args[i] = c.resolveType(a, scope)
				}
				if c.isCoreTask(instance) {
					instance = taskOfArgs(instance, args)
				} else {
					instance = &types.GenericInstance{Base: instance, Args: args}
				}
			}
			return &types.Metatype{Instance: instance}
		}
		return sym.Type()

	case *ast.SequenceExpr:
		folded, err := FoldSequence(c.file, e, c.pg)
		if err != nil {
			c.errorf(e.Pos(), "operator precedence error: %v", err)
			return types.Typ[types.Invalid]
		}
		c.info.Folded[e] = folded
		return c.checkExpr(folded, expected, scope)

	case *ast.BinaryExpr:
		opName := e.Op.Text(c.file)
		if opName == "=" {
			// `_ = x` discards x. There is nothing on the left to give the
			// right a type, so x is checked on its own -- which is what lets
			// a closure written there infer its result from its body.
			if _, discard := unparen(e.X).(*ast.WildcardExpr); discard {
				c.info.Types[e.X] = types.Typ[types.Void]
				c.checkExpr(e.Y, nil, scope)
				return types.Typ[types.Void]
			}
			// The destination is read first, so that its type is the
			// context the source is checked in. That is what makes
			// `n = 1` an Int32 one when n is an Int32, and `s = .red`
			// name a case of whatever s is — neither expression has a
			// type of its own to fall back on, and a checker that
			// looked at the source first would have nothing to give
			// them.
			var lhs types.Type
			if id, ok := e.X.(*ast.IdentExpr); ok {
				name := id.Name.Text(c.file)
				sym := c.lookupValue(scope, name)
				if sym != nil {
					c.info.Uses[id.Name] = sym
					lhs = sym.Type()
					if v, ok := sym.(*VarSymbol); ok {
						if v.isolated {
							c.checkIsolatedAccess(id, "var", name, true)
						}
						if v.IsConst() {
							if v.IsInitialized() && !c.initializesOwnProperty(sym, name) {
								c.errorf(e.Op.Pos(), "cannot assign to value: '%s' is a 'let' constant", name)
							} else {
								if v.IsDeferred() {
									c.initLog = append(c.initLog, v)
								}
								v.SetInitialized(true)
								v.SetConsumed(false)
							}
						} else {
							v.SetInitialized(true)
							v.SetConsumed(false)
						}
					}
				} else {
					c.errorf(id.Name.Pos(), "cannot find '%s' in scope", name)
					lhs = types.Typ[types.Invalid]
				}
				c.info.Types[id] = lhs
			} else if mem, ok := e.X.(*ast.MemberExpr); ok {
				c.assigning = mem
				lhs = c.checkExpr(mem, nil, scope)
				c.assigning = nil
				baseType := c.info.Types[mem.X]
				propName := mem.Name.Text(c.file)
				if IsolatedField(baseType, propName) {
					c.checkIsolatedAccess(mem, "property", propName, true)
				}
				// `self.x = …` in an initializer is how a `let` property
				// gets its value.
				ownInit := c.inInit && string(c.file.Slice(mem.X.Pos(), mem.X.End())) == "self"
				if baseType != nil && !ownInit {
					if st, ok := baseType.Underlying().(*types.Struct); ok {
						for _, f := range st.Fields {
							if f.Name == propName && f.IsConst {
								c.errorf(e.Op.Pos(), "cannot assign to property: '%s' is a 'let' constant", propName)
							}
						}
					}
					if cl, ok := baseType.Underlying().(*types.Class); ok {
						for _, f := range cl.Fields {
							if f.Name == propName && f.IsConst {
								c.errorf(e.Op.Pos(), "cannot assign to property: '%s' is a 'let' constant", propName)
							}
						}
					}
					if c.builtinGetOnly(baseType, propName) {
						c.errorf(e.Op.Pos(), "cannot assign to property: '%s' is a get-only property", propName)
					}
				}
			} else {
				lhs = c.checkExpr(e.X, nil, scope)
				// `d[k] = v` writes through the collection, which has to be
				// a variable.
				if sub, ok := e.X.(*ast.SubscriptExpr); ok {
					switch c.info.Types[sub.X].Underlying().(type) {
					case *types.Array, *types.Dictionary:
						c.checkMutableReceiver(sub.X, sub.Lsquare, "cannot assign through subscript", scope)
					}
					c.checkSubscriptWrite(sub, scope)
				}
			}

			// `a.next?.v = 5` assigns inside the chain: the value is
			// what the last step is, not the optional the chain makes.
			if c.info.ChainRoots[e.X] {
				if inner, ok := c.info.ChainInner[e.X]; ok && inner != nil {
					lhs = inner
				}
			}
			rhs := c.checkExpr(e.Y, lhs, scope)
			if t, ok := c.adopt(e.Y, lhs); ok {
				rhs = t
			}
			// `(a, _) = pair`: the `_` takes whatever is there.
			lhs = wildcardsTake(e.X, lhs, rhs)
			if !types.AssignableTo(rhs, lhs) {
				c.typeErrorf(e.Op.Pos(), "cannot assign value of type '%s' to type '%s'", rhs, lhs)
			}
			return types.Typ[types.Void]
		}

		// An annotation reaches through an arithmetic operator to its
		// operands. `let a: Int32 = 2 + 3 * 4` is an Int32 sum of
		// Int32 literals, because the result of `+` and its operands
		// are one type -- so the context the whole expression is in is
		// the context each part of it is in. A comparison's result
		// says nothing about what it compared, and a logical
		// operator's operands are Bools whatever the result is used
		// for, so for those the context stops here and the operands
		// fall back on their own defaults.
		var operandCtx types.Type
		if expected != nil && sharesOperandType(opName) {
			// The wrapped type where an optional is wanted, for the
			// reason checkPrefix gives: `let a: Int32? = 1 + 2` is an
			// Int32 sum injected, and reading the operands as
			// `Int32?`s left them at Int's default and failed to
			// convert.
			operandCtx = unwrappedContext(expected)
		}
		// A range's operands are its bounds, so what they take from
		// the context is the element rather than the range. In a
		// pattern the context is the subject's own type -- `case
		// 1...5` over an Int32 matches Int32s -- and in an ordinary
		// expression it is the range being asked for, whose element
		// is the same answer one level in.
		if opName == "..." || opName == "..<" {
			operandCtx = c.rangeElement(expected)
		}
		// A leading-dot member has no type of its own: `k.Code == .escape`
		// names a case of whatever the other side is, and `x ?? .none`
		// one of what the optional holds. So the operand that does have a
		// type is read first and is the context for the one that does not.
		var lhs, rhs types.Type
		if implicitOperand(e.Y) && (operandCtx == nil || sharesOperandType(opName)) && implicitTakesOther(opName) {
			lhs = c.checkExpr(e.X, operandCtx, scope)
			ctx := lhs
			if opName == "??" {
				ctx = unwrappedContext(lhs)
			} else {
				ctx = c.implicitOperandContext(scope, opName, lhs, e.Y, 1)
			}
			rhs = c.checkExpr(e.Y, ctx, scope)
		} else if implicitOperand(e.X) && (operandCtx == nil || sharesOperandType(opName)) && implicitTakesOther(opName) && opName != "??" {
			rhs = c.checkExpr(e.Y, operandCtx, scope)
			lhs = c.checkExpr(e.X, c.implicitOperandContext(scope, opName, rhs, e.X, 0), scope)
		} else if opName == "??" {
			// The right of `??` is what the left wraps: `a ?? nil` for a
			// String?? is a String?. Where what it wraps is no optional,
			// `nil` there is the optional itself.
			lhs = c.checkExpr(e.X, operandCtx, scope)
			ctx := unwrappedContext(lhs)
			if lit, isLit := unparen(e.Y).(*ast.BasicLit); isLit && lit.Kind == token.NIL && !isOptionalType(ctx) {
				ctx = lhs
			}
			rhs = c.checkExpr(e.Y, ctx, scope)
		} else if _, isClosure := unparen(e.Y).(*ast.ClosureExpr); isClosure {
			// A closure operand takes its type from the operator the
			// other operand chooses.
			lhs = c.checkExpr(e.X, operandCtx, scope)
			rhs = c.checkExpr(e.Y, c.closureOperandContext(scope, opName, lhs, 1), scope)
		} else if _, isClosure := unparen(e.X).(*ast.ClosureExpr); isClosure {
			rhs = c.checkExpr(e.Y, operandCtx, scope)
			lhs = c.checkExpr(e.X, c.closureOperandContext(scope, opName, rhs, 0), scope)
		} else if _, lit := unparen(e.Y).(*ast.ArrayLit); lit && (opName == "==" || opName == "!=") {
			// An array literal compared with what one makes -- an option
			// set -- is one of those: `p == [.read, .write]`.
			lhs = c.checkExpr(e.X, operandCtx, scope)
			ctx := operandCtx
			if takesArrayLiteral(lhs) && !isArrayType(lhs) {
				ctx = lhs
			}
			rhs = c.checkExpr(e.Y, ctx, scope)
		} else {
			lhs = c.checkExpr(e.X, operandCtx, scope)
			rhs = c.checkExpr(e.Y, operandCtx, scope)
		}
		ownL, ownR := lhs, rhs
		lhs, rhs = c.reconcileLiterals(e.X, lhs, e.Y, rhs, scope)
		// A literal read as the other operand's type where no operator
		// takes two of those is its own type again, where one takes that:
		// `"set " + name.dropFirst(4)` is a String and a Substring.
		if (lhs != ownL || rhs != ownR) && !c.hasOperator(scope, opName, []ast.Expr{e.X, e.Y}, []types.Type{lhs, rhs}) {
			if ch, ok := c.pickOperator(scope, opName, []ast.Expr{e.X, e.Y}, []types.Type{ownL, ownR}); ok && len(ch.subst) == 0 {
				delete(c.info.LiteralInits, unparen(e.X))
				delete(c.info.LiteralInits, unparen(e.Y))
				lhs = c.checkExpr(e.X, ch.sig().Params[0].Type, scope)
				rhs = c.checkExpr(e.Y, ch.sig().Params[1].Type, scope)
			}
		}
		// An array literal compared with or added to an array is an array
		// of that array's elements: `bytes == [109, 115]` with bytes a
		// [UInt8], `xs + []`, `bytes += [0x80]`.
		// So is one compared with a type an array literal makes: an
		// option set, `p == [.read, .write]`.
		if opName == "==" || opName == "!=" || opName == "+" || opName == "+=" {
			if _, lit := unparen(e.Y).(*ast.ArrayLit); lit && takesArrayLiteral(lhs) && !types.Identical(lhs, rhs) {
				rhs = c.checkExpr(e.Y, lhs, scope)
			} else if opName == "+=" {
				// The left of += is what is assigned to; it is never retyped.
			} else if _, lit := unparen(e.X).(*ast.ArrayLit); lit && takesArrayLiteral(rhs) && !types.Identical(lhs, rhs) {
				lhs = c.checkExpr(e.X, rhs, scope)
			}
		}
		// So is a dictionary literal compared with a dictionary: `d == [:]`.
		if opName == "==" || opName == "!=" {
			_, ld := lhs.Underlying().(*types.Dictionary)
			_, rd := rhs.Underlying().(*types.Dictionary)
			if _, lit := unparen(e.Y).(*ast.DictLit); lit && ld && !types.Identical(lhs, rhs) {
				rhs = c.checkExpr(e.Y, lhs, scope)
			} else if _, lit := unparen(e.X).(*ast.DictLit); lit && rd && !types.Identical(lhs, rhs) {
				lhs = c.checkExpr(e.X, rhs, scope)
			}
		}

		// An operator is a function, and core declares them. Where
		// one resolves, the call decides the type and the rules
		// below are not consulted — they are what answers for the
		if t, ok := c.resolveOperator(scope, opName, e, []ast.Expr{e.X, e.Y}, []types.Type{lhs, rhs}); ok {
			return t
		}

		switch opName {
		case "===", "!==":
			// Whether two references are to one object: each a class
			// instance, an optional one, AnyObject, or nil.
			if !isReferenceOperand(lhs) || !isReferenceOperand(rhs) {
				c.typeErrorf(e.Op.Pos(), "binary operator '%s' cannot be applied to operands of type '%s' and '%s'", opName, lhs, rhs)
			}
			return types.Typ[types.Bool]
		case "==", "!=", "<", "<=", ">", ">=":
			if (opName == "==" || opName == "!=") &&
				(isUntypedNil(lhs) && isOptionalType(rhs) ||
					isUntypedNil(rhs) && isOptionalType(lhs)) {
				return types.Typ[types.Bool]
			}
			if opName == "==" || opName == "!=" {
				if c.optionalOperands(e, lhs, rhs, scope) {
					c.payloadOperator(scope, opName, e, lhs, rhs)
					return types.Typ[types.Bool]
				}
			}
			if !c.comparable(lhs) {
				c.typeErrorf(e.Op.Pos(), "type '%s' is not comparable", lhs)
			} else if opName == "==" || opName == "!=" {
				c.collectionEquals(e, lhs, scope)
			}
			if !types.AssignableTo(rhs, lhs) && !types.AssignableTo(lhs, rhs) {
				c.typeErrorf(e.Op.Pos(), "binary operator '%s' cannot be applied to operands of type '%s' and '%s'", opName, lhs, rhs)
			}
			return types.Typ[types.Bool]

		case "&&", "||":
			if !types.Identical(lhs, types.Typ[types.Bool]) || !types.Identical(rhs, types.Typ[types.Bool]) {
				c.typeErrorf(e.Op.Pos(), "logical operator '%s' requires Boolean operands", opName)
			}
			return types.Typ[types.Bool]

		case "??":
			if opt, ok := lhs.(*types.Optional); ok {
				if t, adopted := c.adoptTree(e.Y, opt.Wrapped, scope); adopted {
					rhs = t
				}
				switch {
				case types.AssignableTo(rhs, opt.Wrapped):
					return opt.Wrapped
				case types.AssignableTo(rhs, lhs):
					return lhs
				}
				c.typeErrorf(e.Op.Pos(),
					"binary operator '??' cannot be applied to operands of type '%s' and '%s'",
					lhs, rhs)
				return opt.Wrapped
			}
			return lhs

		case "+", "-", "*", "/", "%":
			// A pointer moved by a count of its elements -- of bytes, for
			// a raw pointer -- is a pointer.
			if opName == "+" || opName == "-" {
				if t, ok := c.pointerOffset(e, lhs, rhs, opName == "+"); ok {
					return t
				}
			}
			// Arithmetic: deduce common numeric type
			if types.AssignableTo(rhs, lhs) {
				return lhs
			}
			if types.AssignableTo(lhs, rhs) {
				return rhs
			}
			c.typeErrorf(e.Op.Pos(), "binary operator '%s' cannot be applied to operands of type '%s' and '%s'", opName, lhs, rhs)
			return lhs

		case "+=", "-=", "*=", "/=":
			if id, ok := e.X.(*ast.IdentExpr); ok {
				name := id.Name.Text(c.file)
				if sym := scope.Lookup(name); sym != nil {
					if v, ok := sym.(*VarSymbol); ok && v.IsConst() {
						c.errorf(e.Op.Pos(), "left side of mutating operator isn't mutable: '%s' is a 'let' constant", name)
					}
				}
			} else if sub, ok := e.X.(*ast.SubscriptExpr); ok {
				switch c.info.Types[sub.X].Underlying().(type) {
				case *types.Array, *types.Dictionary:
					c.checkMutableReceiver(sub.X, e.Op.Pos(), "left side of mutating operator isn't mutable", scope)
				}
				c.checkSubscriptWrite(sub, scope)
			} else if mem, ok := e.X.(*ast.MemberExpr); ok {
				baseType := c.info.Types[mem.X]
				propName := mem.Name.Text(c.file)
				if baseType != nil {
					if st, ok := baseType.Underlying().(*types.Struct); ok {
						for _, f := range st.Fields {
							if f.Name == propName && f.IsConst {
								c.errorf(e.Op.Pos(), "cannot assign to property: '%s' is a 'let' constant", propName)
							}
						}
					}
					if cl, ok := baseType.Underlying().(*types.Class); ok {
						for _, f := range cl.Fields {
							if f.Name == propName && f.IsConst {
								c.errorf(e.Op.Pos(), "cannot assign to property: '%s' is a 'let' constant", propName)
							}
						}
					}
				}
			}
			if t, ok := c.adopt(e.Y, lhs); ok {
				rhs = t
			}
			if !types.AssignableTo(rhs, lhs) {
				c.typeErrorf(e.Op.Pos(), "cannot assign value of type '%s' to type '%s'", rhs, lhs)
			}
			return types.Typ[types.Void]

		case "...", "..<":
			if !types.Identical(lhs, rhs) {
				if !isInvalid(lhs) && !isInvalid(rhs) {
					c.typeErrorf(e.Op.Pos(), "cannot form a range from '%s' to '%s'", lhs, rhs)
				}
				return types.Typ[types.Invalid]
			}
			if opName == "..." {
				return c.rangeOf("ClosedRange", literalDefault(lhs))
			}
			return c.rangeOf("Range", literalDefault(lhs))

		default:
			return lhs
		}

	case *ast.ConditionalExpr:
		condT := c.checkExpr(e.Cond, types.Typ[types.Bool], scope)
		if !types.Identical(condT, types.Typ[types.Bool]) {
			c.typeErrorf(e.Cond.Pos(), "condition must be of type 'Bool', got '%s'", condT)
		}
		thenT := c.checkExpr(e.Then, expected, scope)
		elseT := c.checkExpr(e.Else, expected, scope)
		// A literal on one side is of the other side's type: `c ? n - 1 : 0`
		// with n a UInt64 is a UInt64.
		thenT, elseT = c.reconcileLiterals(e.Then, thenT, e.Else, elseT, scope)
		// `c ? x : nil` is an optional of what x is, the nil read again
		// as one of that type.
		if isUntypedNil(elseT) && !isUntypedNil(thenT) && !isOptionalType(thenT) && !isInvalid(thenT) {
			elseT = &types.Optional{Wrapped: thenT}
			c.checkExpr(e.Else, elseT, scope)
		} else if isUntypedNil(thenT) && !isUntypedNil(elseT) && !isOptionalType(elseT) && !isInvalid(elseT) {
			thenT = &types.Optional{Wrapped: elseT}
			c.checkExpr(e.Then, thenT, scope)
		}
		if expected != nil && types.AssignableTo(thenT, expected) &&
			types.AssignableTo(elseT, expected) {
			return expected
		}
		if types.AssignableTo(elseT, thenT) {
			return thenT
		}
		if types.AssignableTo(thenT, elseT) {
			return elseT
		}
		c.typeErrorf(e.Colon, "result values in '? :' expression have mismatching types '%s' and '%s'", thenT, elseT)
		return thenT

	case *ast.CallExpr:
		// What an earlier look at this call inferred is not this one's:
		// a closure argument is read once with no type to pick among
		// overloads, when `captured.append(s)` in it, with `s` still
		// unknown, is the generic append(contentsOf:), and then again as
		// the parameter says, when it is append(_:), which has none.
		delete(c.info.Specializations, e)
		// A key path given where a function goes -- `xs.map(\.name)` --
		// is the closure `{ $0.name }`, and is checked and lowered as one.
		// No parameter takes a KeyPath yet, so every argument is that.
		if e.Args != nil {
			for _, a := range e.Args.Args {
				if kp, ok := unparen(a.X).(*ast.KeyPathExpr); ok {
					if cl := c.keyPathClosure(kp); cl != nil {
						a.X = cl
					}
				}
			}
		}
		if t, ok := c.optionalSome(e, expected, scope); ok {
			return t
		}
		// `k.Launch(...)` on a kernel, and a kernel called as a function.
		if t, ok := c.kernelCall(e, scope); ok {
			return t
		}
		// `Task.detached { … }` is a Task of what its operation returns,
		// as `Task { … }` is; see the initializer below.
		if mem, ok := e.Fun.(*ast.MemberExpr); ok && mem.Name != nil && mem.Name.Text(c.file) == "detached" &&
			e.Args != nil && len(e.Args.Args) == 1 {
			if meta, isMeta := c.checkExpr(mem.X, nil, scope).(*types.Metatype); isMeta && c.isCoreTask(meta.Instance) {
				want := &types.Signature{Results: nil, Async: true}
				// Detached from where it is made: the operation runs on
				// the pool, not on the main thread.
				prevIsolated := c.currIsolated
				c.currIsolated = false
				sig, isFunc := c.checkExpr(e.Args.Args[0].X, want, scope).(*types.Signature)
				c.currIsolated = prevIsolated
				base := meta.Instance
				if gi, isInst := base.(*types.GenericInstance); isInst {
					base = gi.Base
				}
				if _, m := c.findMethod(meta, "detached"); m != nil {
					c.info.Methods[mem] = &MethodRef{Recv: base, Method: m}
					c.info.Types[mem] = m.Sig
				}
				if isFunc && len(sig.Params) == 0 {
					return taskOf(base, sig.Results, sig.Throws)
				}
				return base
			}
		}
		if mem, ok := e.Fun.(*ast.MemberExpr); ok && !c.namesModule(mem.X, scope) {
			baseType := c.checkExpr(mem.X, nil, scope)
			if cl, ok := baseType.Underlying().(*types.Class); ok && cl.IsActor && c.currActor != cl && !c.inAwait {
				memberName := mem.Name.Text(c.file)
				c.errorf(e.Pos(), "actor-isolated method '%s' cannot be called synchronously without 'await'", memberName)
			}
			if mem.Name != nil {
				var args []*ast.CallArg
				if e.Args != nil {
					args = e.Args.Args
				}
				labels := make([]string, len(args))
				for i, a := range args {
					if a.Label != nil {
						labels[i] = a.Label.Text(c.file)
					}
				}
				name := mem.Name.Text(c.file)
				if t, ok := c.withUnsafeBytesCall(mem, baseType, args, expected, scope); ok {
					return t
				}
				if m, ok := core.LowerCollectionMethod(baseType, name, labels); ok && closuresFit(args, m.Params) &&
					c.valuesFit(args, m.Params, scope) {
					if m.Mutating {
						c.checkMutableReceiver(mem.X, mem.Name.Pos(), "cannot use mutating member on immutable value", scope)
					}
					sig := m.Signature()
					c.info.Types[mem] = sig
					c.info.Types[e.Fun] = sig
					// The core's method, lowered as such: not a method an
					// earlier look at the call chose, with its arguments'
					// types still unknown.
					delete(c.info.Methods, mem)
					return c.checkCallArguments(e, sig, args, scope).Results
				}
				// A method an extension gives a built-in type, called:
				// chosen by the arguments among those of its name, ahead of
				// a property of the same name -- `first(where:)` beside
				// `first` -- since a property is not called.
				if t, ok := c.builtinMethodCall(e, mem, baseType, args, scope); ok {
					return t
				}
			}
		}

		if t, ok := c.coreInitCall(e, scope); ok {
			return t
		}
		// `Array(repeating: v, count: n)`: n copies of v, as
		// `[T](repeating:count:)` makes them with T what v is.
		if id, ok := e.Fun.(*ast.IdentExpr); ok && id.Name != nil && id.Args == nil && id.Name.Text(c.file) == "Array" &&
			scope.LookupType("Array") == nil && e.Args != nil && len(e.Args.Args) == 2 &&
			e.Args.Args[0].Label != nil && e.Args.Args[0].Label.Text(c.file) == "repeating" &&
			e.Args.Args[1].Label != nil && e.Args.Args[1].Label.Text(c.file) == "count" {
			var elem types.Type
			if arr, isArray := expected.(*types.Array); isArray {
				elem = arr.Elem
			}
			vt := literalDefault(c.checkExpr(e.Args.Args[0].X, elem, scope))
			if nt := c.checkExpr(e.Args.Args[1].X, types.Typ[types.Int], scope); !types.AssignableTo(nt, types.Typ[types.Int]) {
				c.typeErrorf(e.Args.Args[1].Pos(), "cannot convert value of type '%s' to expected argument type 'Int'", nt)
			}
			if isInvalid(vt) {
				return vt
			}
			c.info.ArrayRepeats[e] = true
			return &types.Array{Elem: vt}
		}
		// `Array(xs)`: an array of what a sequence holds. Only an array's
		// own elements so far -- `Array(s.utf8)`, whose utf8 is one.
		if id, ok := e.Fun.(*ast.IdentExpr); ok && id.Name != nil && id.Args == nil && id.Name.Text(c.file) == "Array" &&
			scope.LookupType("Array") == nil && e.Args != nil && len(e.Args.Args) == 1 && e.Args.Args[0].Label == nil {
			t := c.checkExpr(e.Args.Args[0].X, expected, scope)
			if _, isArray := t.Underlying().(*types.Array); isArray {
				c.info.ArrayCopies[e] = t
				return t
			}
			// Any other sequence: its elements, as for-in reads them.
			if it := c.iteration(t); it != nil {
				c.info.ArraySequences[e] = it
				return &types.Array{Elem: it.Element}
			}
			if !isInvalid(t) {
				c.typeErrorf(e.Pos(), "no initializer of 'Array' takes a '%s' yet", t)
			}
			return types.Typ[types.Invalid]
		}
		// `Set<Point>()`, `Dictionary<String, Int>()`, `Array<Int>()`: an
		// empty collection of the type named, where nothing of the
		// program's own has that name.
		if id, ok := e.Fun.(*ast.IdentExpr); ok && id.Name != nil && id.Args != nil &&
			(e.Args == nil || len(e.Args.Args) == 0) {
			switch name := id.Name.Text(c.file); name {
			case "Set", "Array", "Dictionary":
				if scope.LookupType(name) == nil {
					t := c.resolveType(&ast.IdentType{Span: id.Span, Name: id.Name, Args: id.Args}, scope)
					switch t.(type) {
					case *types.Set, *types.Array, *types.Dictionary:
						c.info.EmptyCollections[e] = t
						return t
					}
				}
			}
		}

		if c.callees == nil {
			c.callees = map[ast.Expr]bool{}
		}
		c.callees[unparen(e.Fun)] = true
		if t, ok := c.typeOf(e, scope); ok {
			return t
		}
		// A function with parameter packs: the call is of its expansion.
		if d := c.packDeclOf(e, scope); d != nil {
			if t, ok := c.packCall(e, d, expected, scope); ok {
				return t
			}
		}
		var calleeWant types.Type
		if _, ok := e.Fun.(*ast.ImplicitMemberExpr); ok {
			calleeWant = expected
		}
		// `{ ...; return x }()` where an Int is wanted is a closure that
		// returns one.
		if _, ok := unparen(e.Fun).(*ast.ClosureExpr); ok && expected != nil && !isInvalid(expected) &&
			(e.Args == nil || len(e.Args.Args) == 0) && len(e.Trailing) == 0 {
			calleeWant = &types.Signature{Results: expected}
		}
		var calleeType types.Type
		// `.init(...)` where a T is wanted is T(...).
		if im, ok := e.Fun.(*ast.ImplicitMemberExpr); ok && im.Name != nil && im.Name.Text(c.file) == "init" && expected != nil {
			calleeType = &types.Metatype{Instance: unwrappedContext(expected)}
			c.info.Types[im] = calleeType
		} else {
			calleeType = c.checkExpr(e.Fun, calleeWant, scope)
		}
		var args []*ast.CallArg
		if e.Args != nil {
			args = e.Args.Args
		}
		if sig, ok := calleeType.Underlying().(*types.Signature); ok {
			if im, ok := e.Fun.(*ast.ImplicitMemberExpr); ok && c.info.ImplicitMethods[im] != nil {
				ref := c.info.ImplicitMethods[im]
				if ms := staticsMaking(ref.Recv, ref.Method.Name); len(ms) > 1 {
					if m := c.methodByArguments(ms, args, scope); m != nil {
						c.info.ImplicitMethods[im] = &MethodRef{Recv: ref.Recv, Method: m}
						c.info.Types[im] = m.Sig
						sig = m.Sig
					}
				}
			}
			if chosen := c.resolveOverload(e.Fun, args, expected, scope); chosen != nil {
				sig = chosen
			} else if mem, ok := e.Fun.(*ast.MemberExpr); ok {
				if chosen := c.resolveMethodOverload(mem, args, scope); chosen != nil {
					sig = chosen
				}
			}
			return c.checkCallArguments(e, sig, args, scope).Results
		}
		// An initializer call.
		//
		// A struct that declares no initializer of its own gets the
		// memberwise one, and that is a real signature the arguments
		// can be checked against: one parameter per stored property,
		// in declaration order, labelled with the property's name. A
		// type that declares its own initializers is not checked
		// here — which of them was meant is overload resolution, and
		// what an initializer body promises is not modelled.
		if meta, ok := calleeType.(*types.Metatype); ok {
			// `E(rawValue: x)` is the case whose raw value is x, or nil:
			// the initializer every enum with a raw type has, and a
			// failable one.
			if en, isEnum := meta.Instance.Underlying().(*types.Enum); isEnum && en.RawType != nil &&
				len(args) == 1 && args[0].Label != nil && args[0].Label.Text(c.file) == "rawValue" {
				got := c.checkExpr(args[0].X, en.RawType, scope)
				if !types.AssignableTo(got, en.RawType) {
					c.typeErrorf(args[0].Pos(), "cannot convert value of type '%s' to expected argument type '%s'", got, en.RawType)
				}
				c.info.RawInits[e] = en
				return &types.Optional{Wrapped: meta.Instance}
			}
			// `Task { ... }` is a Task of what its operation returns.
			if c.isCoreTask(meta.Instance) && len(args) == 1 {
				// The operation is async whatever it returns -- core
				// declares it `() async -> Void` and this is the same
				// call with a result -- so it is checked against a
				// signature that says so. Without that the closure is
				// typed as an ordinary function, and an await in it
				// would not be one. See core's Task.
				want := &types.Signature{Results: nil, Async: true}
				if sig, isFunc := c.checkExpr(args[0].X, want, scope).(*types.Signature); isFunc &&
					(!returnsNothing(sig.Results) || sig.Throws) && len(sig.Params) == 0 {
					base := meta.Instance
					if gi, isInst := base.(*types.GenericInstance); isInst {
						base = gi.Base
					}
					return taskOf(base, sig.Results, sig.Throws)
				}
			}
			inst := c.inferInstance(meta.Instance, e, scope)
			// Where the arguments leave the parameters open, the type
			// wanted says them: `let d: C<String> = C()`.
			if _, open := inst.(*types.GenericInstance); !open && len(typeParamsOf(inst)) > 0 {
				if want, ok := unwrappedContext(expected).(*types.GenericInstance); ok && types.Identical(want.Base, inst) {
					inst = want
				} else if ok {
					// Or the superclass instance wanted says them: a
					// Named<T>: Container<T> where a Container<Int> goes
					// is a Named<Int>.
					inst = inferFromSuperclass(inst, want)
				}
			}
			if st, ok := inst.Underlying().(*types.Struct); ok {
				// The memberwise initializer is read off the instance's own
				// properties, which are its arguments already.
				// Beside initializers an extension declares, the
				// memberwise one is the call whose labels are its own.
				if sig := st.Memberwise(); sig != nil && (len(st.Inits) == 0 || c.labelsFit(sig, args)) {
					// A memberwise initializer is internal: another
					// module's struct is made only by one it made public.
					if c.isImportedType(inst) {
						c.typeErrorf(e.Pos(), "'%s' initializer is inaccessible due to 'internal' protection level", inst)
					}
					c.checkCallArguments(e, sig, args, scope)
					return inst
				}
				if sig := c.pickInitializer(st.Inits, args); sig != nil {
					c.info.Inits[e] = sig
					c.checkImportedInit(e, inst, sig)
					c.checkCallArguments(e, initializerFor(inst, sig), args, scope)
					return initResult(inst, sig)
				}
				if sig := c.soleInitializerOfArity(st.Inits, len(args)); sig != nil {
					c.info.Inits[e] = sig
					c.checkImportedInit(e, inst, sig)
					c.checkCallArguments(e, initializerFor(inst, sig), args, scope)
					return initResult(inst, sig)
				}
				// Several with the same labels are told apart by type.
				if sig := c.pickInitializerByType(st.Inits, args, scope); sig != nil {
					c.info.Inits[e] = sig
					c.checkImportedInit(e, inst, sig)
					c.checkCallArguments(e, initializerFor(inst, sig), args, scope)
					return initResult(inst, sig)
				}
				if sig := c.pickInitializerFitting(st.Inits, args, scope); sig != nil {
					c.info.Inits[e] = sig
					c.checkImportedInit(e, inst, sig)
					c.checkCallArguments(e, initializerFor(inst, sig), args, scope)
					return initResult(inst, sig)
				}
				if len(st.Inits) > 0 {
					c.typeErrorf(e.Pos(),
						"no initializer of '%s' takes these arguments", inst)
				}
			}
			if en, ok := inst.Underlying().(*types.Enum); ok && len(en.Inits) > 0 {
				for _, pick := range []func() *types.Signature{
					func() *types.Signature { return c.pickInitializer(en.Inits, args) },
					func() *types.Signature { return c.soleInitializerOfArity(en.Inits, len(args)) },
					func() *types.Signature { return c.pickInitializerByType(en.Inits, args, scope) },
					func() *types.Signature { return c.pickInitializerFitting(en.Inits, args, scope) },
				} {
					if sig := pick(); sig != nil {
						c.info.Inits[e] = sig
						c.checkCallArguments(e, initializerFor(inst, sig), args, scope)
						return initResult(inst, sig)
					}
				}
				c.typeErrorf(e.Pos(), "no initializer of '%s' takes these arguments", inst)
				return inst
			}
			if cl, ok := inst.Underlying().(*types.Class); ok && len(cl.Inits) > 0 {
				if sig := c.pickInitializer(cl.Inits, args); sig != nil {
					c.info.Inits[e] = sig
					c.checkCallArguments(e, initializerFor(inst, sig), args, scope)
					return initResult(inst, sig)
				}
				if sig := c.soleInitializerOfArity(cl.Inits, len(args)); sig != nil {
					c.info.Inits[e] = sig
					c.checkCallArguments(e, initializerFor(inst, sig), args, scope)
					return initResult(inst, sig)
				}
				if sig := c.pickInitializerFitting(cl.Inits, args, scope); sig != nil {
					c.info.Inits[e] = sig
					c.checkCallArguments(e, initializerFor(inst, sig), args, scope)
					return initResult(inst, sig)
				}
				c.typeErrorf(e.Pos(), "no initializer of '%s' takes these arguments", inst)
				return inst
			}
			if b, ok := inst.Underlying().(*types.Basic); ok {
				// An initializer an extension declares -- the core's
				// `Int32(clamping:)`, or a program's own -- is called as a
				// struct's is.
				if bm := c.builtinOf(inst); bm != nil && len(bm.Inits) > 0 {
					if sig := c.pickInitializerByType(bm.Inits, args, scope); sig != nil {
						c.info.Inits[e] = sig
						c.checkCallArguments(e, initializerFor(inst, sig), args, scope)
						return initResult(inst, sig)
					}
				}
				if t, handled := c.basicInit(e, b, inst, args, scope); handled {
					return t
				}
			}
			// `UnsafeMutablePointer<T>(bitPattern: n)` is the pointer at
			// address n, or nil at zero: Swift's failable init?(bitPattern:)
			// of an Int or a UInt.
			if _, isPtr := inst.Underlying().(*types.Pointer); isPtr && len(args) == 1 &&
				args[0].Label != nil && args[0].Label.Text(c.file) == "bitPattern" {
				t := c.checkExpr(args[0].X, types.Typ[types.Int], scope)
				if !isWord(t) && !isInvalid(t) {
					c.typeErrorf(args[0].Pos(), "cannot convert value of type '%s' to expected argument type 'Int'", t)
				}
				return &types.Optional{Wrapped: inst}
			}
			for _, arg := range args {
				t := c.checkExpr(arg.X, nil, scope)
				// String(cString:) takes a pointer, not an optional one, so
				// a pointer imported from C is unwrapped for it.
				if isString(inst) && arg.Label != nil && arg.Label.Text(c.file) == "cString" {
					c.unwrapImplicit(arg.X, t)
				}
			}
			return inst
		}
		// `[K: V]()`: a dictionary literal of two types is the dictionary
		// type, and a call of it with nothing makes an empty one.
		if d, ok := calleeType.(*types.Dictionary); ok && len(args) == 0 {
			k, kok := d.Key.(*types.Metatype)
			v, vok := d.Value.(*types.Metatype)
			if kok && vok {
				inst := &types.Dictionary{Key: k.Instance, Value: v.Instance}
				c.info.EmptyCollections[e] = inst
				return inst
			}
		}
		// `[T](...)`: an array literal of one type is the array type, and
		// a call of it makes an array -- empty, or of count copies.
		if arr, ok := calleeType.(*types.Array); ok {
			if meta, ok := arr.Elem.(*types.Metatype); ok {
				inst := &types.Array{Elem: meta.Instance}
				if len(args) == 0 {
					return inst
				}
				// `[T](xs)` of an array of T -- `[UInt8](s.utf8)` -- is a
				// copy of it, as `Array(xs)` is.
				if len(args) == 1 && args[0].Label == nil {
					t := c.checkExpr(args[0].X, inst, scope)
					if a, isArray := t.Underlying().(*types.Array); isArray && types.Identical(a.Elem, meta.Instance) {
						c.info.ArrayCopies[e] = inst
						return inst
					}
					if !isInvalid(t) {
						c.typeErrorf(e.Pos(), "no initializer of '%s' takes a '%s' yet", inst, t)
					}
					return inst
				}
				if len(args) == 2 && args[0].Label != nil && args[1].Label != nil &&
					args[0].Label.Text(c.file) == "repeating" && args[1].Label.Text(c.file) == "count" {
					if vt := c.checkExpr(args[0].X, meta.Instance, scope); !types.AssignableTo(vt, meta.Instance) {
						c.typeErrorf(args[0].Pos(), "cannot convert value of type '%s' to expected argument type '%s'", vt, meta.Instance)
					}
					if ct := c.checkExpr(args[1].X, types.Typ[types.Int], scope); !types.AssignableTo(ct, types.Typ[types.Int]) {
						c.typeErrorf(args[1].Pos(), "cannot convert value of type '%s' to expected argument type 'Int'", ct)
					}
					return inst
				}
				for _, arg := range args {
					c.checkExpr(arg.X, nil, scope)
				}
				c.typeErrorf(e.Pos(), "no initializer of '%s' takes these arguments", inst)
				return inst
			}
		}
		// A value of a type that declares callAsFunction is called
		// through it (SE-0253): `p(2)` is `p.callAsFunction(2)`.
		if t, ok := c.callAsFunction(e, calleeType, expected, scope); ok {
			return t
		}
		for _, arg := range args {
			c.checkExpr(arg.X, nil, scope)
		}
		c.typeErrorf(e.Pos(), "cannot call value of non-function type '%s'", calleeType)
		return types.Typ[types.Invalid]

	// Implicit member expression (e.g. `.caseName`).
	case *ast.ImplicitMemberExpr:
		if e.Name == nil {
			return types.Typ[types.Invalid]
		}
		if expected == nil {
			c.typeErrorf(e.Dot, "reference to member '%s' cannot be resolved without a contextual type", e.Name.Text(c.file))
			return types.Typ[types.Invalid]
		}
		name := e.Name.Text(c.file)
		if t, ok := c.optionalNone(e, expected, name); ok {
			return t
		}
		sym := c.enumCaseSymbol(expected, name)
		// An optional wants what it wraps: `.success(v)` where a
		// Result<T, E>? goes is a case of Result<T, E>.
		if sym == nil {
			if o, ok := expected.(*types.Optional); ok {
				if wrapped := c.enumCaseSymbol(o.Wrapped, name); wrapped != nil {
					sym, expected = wrapped, o.Wrapped
				}
			}
		}
		if sym == nil {
			// A static property whose type is the contextual type --
			// `.default` for `ListenerOptions.default` -- is named the
			// way a case is.
			base := expected
			if o, ok := base.(*types.Optional); ok {
				base = o.Wrapped
			}
			if t, ok := c.integerBoundNamed(e, name, &types.Metatype{Instance: base}); ok {
				return t
			}
			if t := c.lookupMember(&types.Metatype{Instance: base}, name); t != nil {
				if _, isFunc := t.(*types.Signature); !isFunc && types.AssignableTo(t, base) {
					return t
				}
			}
			// A static method that makes one is called the same way:
			// `.seconds(1)` for `Span.seconds(1)`. Which of several it is
			// is the call's to say, by its arguments.
			if ms := staticsMaking(base, name); len(ms) > 0 {
				recv, _ := methodsNamed(&types.Metatype{Instance: base}, name)
				c.info.ImplicitMethods[e] = &MethodRef{Recv: recv, Method: ms[0]}
				return ms[0].Sig
			}
			if !isInvalid(expected) {
				c.typeErrorf(e.Name.Pos(), "type '%s' has no member '%s'", expected, name)
			}
			return types.Typ[types.Invalid]
		}
		c.info.Uses[e.Name] = sym
		c.checkAccess(e.Name.Pos(), e.Name.Text(c.file), sym)
		if assoc := sym.AssociatedType(); assoc != nil {
			// A case of a generic enum carries the instance's arguments:
			// .failure of Outcome<Int, NetError> takes a NetError.
			if subst := types.InstanceSubst(expected); subst != nil {
				assoc = types.Substitute(assoc, subst)
			}
			return &types.Signature{
				Params:  caseParams(assoc, sym.Label()),
				Results: expected,
			}
		}
		return expected

	case *ast.MemberExpr:
		if t, ok := c.moduleMemberValue(e, scope); ok {
			return t
		}
		if opt, ok := c.optionalNamed(e.X, expected, scope); ok {
			if t, ok := c.optionalNone(e, opt, e.Name.Text(c.file)); ok {
				return t
			}
		}
		baseType := c.checkExpr(e.X, nil, scope)
		if t, ok := c.integerBound(e, baseType); ok {
			return t
		}
		if t, ok := c.memoryLayout(e, baseType); ok {
			return t
		}
		memberName := e.Name.Text(c.file)
		if _, chained := e.X.(*ast.OptionalExpr); !chained {
			baseType = c.unwrapImplicit(e.X, baseType)
		}
		chained := false

		// A method or an initializer named and not called.
		if t, ok := c.memberReference(e, baseType, expected, scope); ok {
			return t
		}

		if cl, ok := baseType.Underlying().(*types.Class); ok && cl.IsActor && c.currActor != cl && !c.inAwait {
			for _, f := range cl.Fields {
				if f.Name == memberName {
					c.errorf(e.Name.Pos(), "actor-isolated property '%s' cannot be referenced synchronously without 'await'", memberName)
				}
			}
		}

		if t := c.lookupMemberFor(e, baseType, memberName); t != nil {
			c.checkImportedMember(e, baseType, memberName)
			c.checkMemberConditions(e, baseType, memberName)
			if c.assigning != e && IsolatedField(baseType, memberName) {
				c.checkIsolatedAccess(e, "property", memberName, false)
			}
			if chained {
				if _, already := t.(*types.Optional); !already {
					return &types.Optional{Wrapped: t}
				}
			}
			return t
		}
		if t, ok := c.dynamicMember(e, baseType, expected, scope); ok {
			return t
		}
		if !isInvalid(baseType) && c.membersKnown(baseType) {
			c.typeErrorf(e.Name.Pos(), "value of type '%s' has no member '%s'", baseType, memberName)
		}
		return types.Typ[types.Invalid]

	case *ast.SubscriptExpr:
		// `x[keyPath: k]`, which every type has: what k reads from an x.
		if len(e.Args) == 1 && e.Args[0].Label != nil && e.Args[0].Label.Text(c.file) == "keyPath" {
			if t, ok := c.keyPathSubscript(e, scope); ok {
				return t
			}
		}
		baseType := c.checkExpr(e.X, nil, scope)
		var index, fallback types.Type
		var result types.Type = types.Typ[types.Invalid]
		switch b := baseType.Underlying().(type) {
		case *types.Array:
			// `a[lo..<hi]` is the elements between, as an ArraySlice.
			if len(e.Args) == 1 {
				at := c.checkExpr(e.Args[0].X, types.Typ[types.Int], scope)
				if _, _, isRange := c.info.AnyRangeOf(at); isRange {
					if s := c.arraySliceOf(b.Elem, scope); s != nil {
						return s
					}
				}
				return b.Elem
			}
			index, result = types.Typ[types.Int], b.Elem
		// `p[i]` is what Swift's pointer subscript is: `(p + i).pointee`,
		// read or written.
		case *types.Pointer:
			if b.Dereferenceable() && len(e.Args) == 1 && e.Args[0].Label == nil {
				span := ast.Span{Lo: e.Pos(), Hi: e.End()}
				sum := &ast.BinaryExpr{Span: span, X: e.X, Op: &ast.OperatorExpr{Span: span, Kind: token.OPER_BINARY, Synth: "+"}, Y: e.Args[0].X}
				read := &ast.MemberExpr{Span: span, X: &ast.ParenExpr{Span: span, X: sum}, Dot: e.Pos(),
					Name: &ast.Ident{Span: span, Synth: "pointee"}}
				c.info.ImplicitSelf[e] = read
				return c.checkExpr(read, expected, scope)
			}
		case *types.Dictionary:
			index, result = b.Key, &types.Optional{Wrapped: b.Value}
			if len(e.Args) == 2 && e.Args[1].Label != nil && e.Args[1].Label.Text(c.file) == "default" {
				result, fallback = b.Value, b.Value
			}
		default:
			if t, ok := c.declaredSubscript(e, baseType, scope); ok {
				return t
			}
		}
		for i, arg := range e.Args {
			want := index
			if i > 0 {
				want = fallback
			}
			c.checkExpr(arg.X, want, scope)
		}
		return result

	case *ast.ArrayLit:
		if t, ok := c.collectionLiteralInit(e, expected, scope); ok {
			return t
		}
		if setT, ok := expected.(*types.Set); ok {
			for _, el := range e.Items {
				c.checkExpr(el, setT.Elem, scope)
			}
			return setT
		}
		var elemType types.Type
		if arrT, ok := expected.(*types.Array); ok {
			elemType = arrT.Elem
		}
		// `[nil, 0, -3]` with nothing to say what it is: an array of
		// optionals of what its other elements are.
		if elemType == nil && len(e.Items) > 1 {
			var sawNil bool
			var other types.Type
			for _, el := range e.Items {
				if lit, ok := unparen(el).(*ast.BasicLit); ok && lit.Kind == token.NIL {
					sawNil = true
				} else if other == nil {
					other = c.checkExpr(el, nil, scope)
				}
			}
			if sawNil && other != nil && !isInvalid(other) {
				if _, isOpt := other.(*types.Optional); isOpt {
					elemType = other
				} else {
					elemType = &types.Optional{Wrapped: literalDefault(other)}
				}
			}
		}
		for _, el := range e.Items {
			et := c.checkExpr(el, elemType, scope)
			if elemType == nil {
				elemType = et
			}
		}
		if elemType == nil {
			elemType = types.Typ[types.Invalid]
		}
		return &types.Array{Elem: elemType}

	case *ast.DictLit:
		if t, ok := c.collectionLiteralInit(e, expected, scope); ok {
			return t
		}
		var keyType, valType types.Type
		if dictT, ok := expected.(*types.Dictionary); ok {
			keyType, valType = dictT.Key, dictT.Value
		}
		for _, item := range e.Items {
			kt := c.checkExpr(item.Key, keyType, scope)
			vt := c.checkExpr(item.Value, valType, scope)
			if keyType == nil {
				keyType = kt
			}
			if valType == nil {
				valType = vt
			}
		}
		if keyType == nil {
			keyType = types.Typ[types.String]
		}
		if valType == nil {
			valType = types.Typ[types.Invalid]
		}
		return &types.Dictionary{Key: keyType, Value: valType}

	case *ast.TupleExpr:
		var want *types.Tuple
		if expected != nil {
			want, _ = expected.Underlying().(*types.Tuple)
		}
		elems := make([]*types.TupleElement, len(e.Elems))
		for i, el := range e.Elems {
			var label string
			if el.Label != nil {
				label = el.Label.Text(c.file)
			}
			var elemWant types.Type
			if want != nil && i < len(want.Elements) && want.Elements[i] != nil {
				elemWant = want.Elements[i].Type
				// `(1.5, 2)` where an `(x: Float, y: Float)` is wanted
				// takes its labels.
				if label == "" && len(want.Elements) == len(e.Elems) {
					label = want.Elements[i].Name
				}
			}
			t := c.checkExpr(el.X, elemWant, scope)
			// An element given where an optional of it is wanted --
			// `(true, 1)` for a `(Bool?, Int)` -- is the tuple's as the
			// optional, wrapped when the tuple is made.
			if elemWant != nil && !isInvalid(t) && !types.Identical(t, elemWant) {
				if _, isOpt := elemWant.(*types.Optional); isOpt && types.AssignableTo(t, elemWant) {
					t = elemWant
				}
			}
			elems[i] = &types.TupleElement{Name: label, Type: t}
		}
		return &types.Tuple{Elements: elems}

	case *ast.CastExpr:
		targetT := c.resolveType(e.Type, scope)
		// `1 as Double` reads the literal as the type it names.
		var want types.Type
		if e.Kind != token.IS && !e.Question.IsValid() && !e.Exclaim.IsValid() {
			want = targetT
		}
		c.checkExpr(e.X, want, scope)
		if e.Kind == token.IS {
			c.info.CastTargets[e] = targetT
			return types.Typ[types.Bool]
		}
		if e.Question != token.NoPos {
			return &types.Optional{Wrapped: targetT}
		}
		return targetT

	case *ast.TryExpr:
		if e.Question != token.NoPos {
			var want types.Type
			if opt, ok := expected.(*types.Optional); ok {
				want = opt.Wrapped
			}
			got := c.checkExpr(e.X, want, scope)
			if isInvalid(got) || types.Identical(got, types.Typ[types.Void]) {
				return got
			}
			if _, already := got.(*types.Optional); already {
				return got
			}
			return &types.Optional{Wrapped: got}
		}
		return c.checkExpr(e.X, expected, scope)

	case *ast.AwaitExpr:
		prevAwait := c.inAwait
		c.inAwait = true
		defer func() { c.inAwait = prevAwait }()
		return c.checkExpr(e.X, expected, scope)

	case *ast.ConsumeExpr:
		inner := c.checkExpr(e.X, expected, scope)
		if id, ok := e.X.(*ast.IdentExpr); ok {
			name := id.Name.Text(c.file)
			if sym := scope.Lookup(name); sym != nil {
				if v, ok := sym.(*VarSymbol); ok {
					if v.IsConsumed() {
						c.errorf(e.Pos(), "'%s' used after consume", name)
					}
					v.SetConsumed(true)
				}
			}
		}
		return inner

	case *ast.BorrowExpr:
		inner := c.checkExpr(e.X, expected, scope)
		if id, ok := e.X.(*ast.IdentExpr); ok {
			name := id.Name.Text(c.file)
			if sym := scope.Lookup(name); sym != nil {
				if v, ok := sym.(*VarSymbol); ok && v.IsConsumed() {
					c.errorf(e.Pos(), "'%s' used after consume", name)
				}
			}
		}
		return inner

	case *ast.CopyExpr:
		inner := c.checkExpr(e.X, expected, scope)
		if id, ok := e.X.(*ast.IdentExpr); ok {
			name := id.Name.Text(c.file)
			if sym := scope.Lookup(name); sym != nil {
				if v, ok := sym.(*VarSymbol); ok && v.IsConsumed() {
					c.errorf(e.Pos(), "'%s' used after consume", name)
				}
			}
		}
		return inner

	case *ast.ForceExpr:
		inner := c.checkExpr(e.X, nil, scope)
		if opt, ok := inner.(*types.Optional); ok {
			return opt.Wrapped
		}
		return inner

	case *ast.OptionalExpr:
		inner := c.checkExpr(e.X, nil, scope)
		// Inside a chain, `a?` is what a holds.
		if c.inChain[e] {
			if o, ok := inner.(*types.Optional); ok {
				return o.Wrapped
			}
			if !isInvalid(inner) {
				c.typeErrorf(e.Question, "cannot use optional chaining on non-optional value of type '%s'", inner)
			}
			return inner
		}
		// `Int?` written where a value goes is the type Optional<Int>.
		if meta, ok := inner.(*types.Metatype); ok {
			return &types.Metatype{Instance: &types.Optional{Wrapped: meta.Instance}}
		}
		return &types.Optional{Wrapped: inner}

	// An operator named as a value -- `reduce(0, +)` -- is the operator
	// function the context's type picks out.
	case *ast.OperatorExpr:
		return c.operatorReference(e, expected, scope)

	case *ast.ClosureExpr:
		closureScope := NewScope(scope, e.Pos(), e.End())
		c.info.Scopes[e] = closureScope
		if e.Sig != nil && e.Sig.Captures != nil {
			for _, item := range e.Sig.Captures.Items {
				c.checkCaptureItem(item, scope, closureScope)
			}
		}

		var expSig *types.Signature
		if expected != nil {
			if s, ok := expected.Underlying().(*types.Signature); ok {
				expSig = s
			} else if o, ok := expected.Underlying().(*types.Optional); ok && o.Wrapped != nil {
				// A closure where an optional function is wanted is the
				// function the optional holds.
				if s, ok := o.Wrapped.Underlying().(*types.Signature); ok {
					expSig = s
				}
			}
		}

		var params []*types.Param
		if tu := c.splatTuple(e, expSig); tu != nil {
			// One tuple wanted, its elements named: the closure takes the
			// tuple, and its names are the elements.
			var syms []Symbol
			for i, el := range tu.Elements {
				name, pos := fmt.Sprintf("$%d", i), e.Pos()
				if e.Sig != nil && e.Sig.Params != nil {
					p := e.Sig.Params.Params[i]
					name, pos = p.Name.Text(c.file), p.Name.Pos()
				}
				v := NewVar(name, el.Type, pos, true, types.DefaultOwnership)
				closureScope.Insert(v)
				if e.Sig != nil && e.Sig.Params != nil {
					c.info.Defs[e.Sig.Params.Params[i].Name] = v
				}
				syms = append(syms, v)
			}
			c.info.Splats[e] = syms
			params = []*types.Param{{Type: expSig.Params[0].Type, Ownership: expSig.Params[0].Ownership}}
		} else if e.Sig != nil && e.Sig.Params != nil {
			for i, p := range e.Sig.Params.Params {
				name := p.Name.Text(c.file)
				var paramType types.Type
				ownership := types.DefaultOwnership
				if p.Type != nil {
					paramType = c.resolveType(p.Type, scope)
				} else if expSig != nil && i < len(expSig.Params) {
					// An `inout` parameter of the function wanted is one
					// of the closure's too: `{ acc, x in acc += x }`.
					paramType = expSig.Params[i].Type
					ownership = expSig.Params[i].Ownership
				}
				if paramType == nil {
					paramType = types.Typ[types.Invalid]
				}
				params = append(params, &types.Param{Name: name, Type: paramType, Ownership: ownership})
				v := NewVar(name, paramType, p.Name.Pos(), ownership != types.InOut, ownership)
				closureScope.Insert(v)
				c.info.Defs[p.Name] = v
			}
		} else if expSig != nil {
			for i, p := range expSig.Params {
				shorthandName := fmt.Sprintf("$%d", i)
				v := NewVar(shorthandName, p.Type, e.Pos(), p.Ownership != types.InOut, p.Ownership)
				closureScope.Insert(v)
				params = append(params, &types.Param{Name: shorthandName, Type: p.Type, Ownership: p.Ownership})
			}
		}

		var retType types.Type = types.Typ[types.Void]
		if expSig != nil && expSig.Results != nil {
			retType = expSig.Results
		}
		if e.Sig != nil && e.Sig.Result != nil {
			retType = c.resolveType(e.Sig.Result.Type, scope)
		}
		// Nothing says what it returns: its returns do.
		unstated := (expSig == nil || expSig.Results == nil) && (e.Sig == nil || e.Sig.Result == nil)

		// A closure is async where it says so or where its body awaits.
		// Where an async function is merely expected, one that does not
		// await is a synchronous closure converted to it, and chooses
		// among overloads as a synchronous function does: swiftc runs the
		// blocking `load()` in `let f: () async -> Int = { load() }`.
		// An await around the closure does not cover the calls in its body.
		prevRet, prevAsync, prevAwait, prevInfer := c.currFuncRet, c.currAsync, c.inAwait, c.inferRet
		defer func() { c.inferRet = prevInfer }()
		c.inferRet = nil
		c.currFuncRet = retType
		c.currAsync = (e.Sig != nil && e.Sig.Async.IsValid()) || awaitsIn(e.Stmts)
		c.inAwait = false
		defer func() { c.currFuncRet, c.currAsync, c.inAwait = prevRet, prevAsync, prevAwait }()
		// A closure runs where it is made -- on the main thread inside
		// @MainActor code, as Swift's closures inherit their context's
		// isolation -- or where its attribute or the function type it is
		// given says. Task.detached takes its operation out of that;
		// see the initializer above.
		prevIsolated := c.currIsolated
		c.currIsolated = c.currIsolated || c.hasAttr(e.Attrs, mainActorAttr) || (expSig != nil && expSig.Isolated)
		defer func() { c.currIsolated = prevIsolated }()

		_, oneExpr := func() (*ast.ExprStmt, bool) {
			if len(e.Stmts) != 1 {
				return nil, false
			}
			x, ok := e.Stmts[0].(*ast.ExprStmt)
			return x, ok
		}()
		// A result the closure writes is its result, whatever type
		// parameters it names: the enclosing function's, not a callee's.
		stated := e.Sig != nil && e.Sig.Result != nil
		inferResult := !stated && (unstated || retType == nil || c.mentionsOpenParam(retType, scope))
		if len(e.Stmts) == 1 && (oneExpr || !inferResult) {
			if exprStmt, ok := e.Stmts[0].(*ast.ExprStmt); ok {
				// A result still to be inferred -- the U of a call to
				// map<U> -- is whatever the body gives.
				open := !stated && c.mentionsOpenParam(retType, scope)
				want := retType
				if open {
					want = nil
				}
				inferredRet := c.checkExpr(exprStmt.X, want, closureScope)
				if retType == nil || open || types.Identical(retType, types.Typ[types.Void]) {
					retType = inferredRet
				} else if !types.AssignableTo(inferredRet, retType) {
					c.typeErrorf(exprStmt.X.Pos(), "cannot convert return value of type '%s' to expected return type '%s'", inferredRet, retType)
				}
			} else {
				c.checkStmt(e.Stmts[0], closureScope)
			}
		} else if inferResult {
			// A result still to be inferred is what the returns give
			// (SE-0326), as for a closure of one expression.
			var got []types.Type
			c.inferRet, c.currFuncRet = &got, nil
			for _, s := range e.Stmts {
				c.checkStmt(s, closureScope)
			}
			c.inferRet = nil
			if len(got) > 0 {
				retType = literalDefault(got[0])
			} else if retType != nil {
				retType = types.Typ[types.Void]
			}
		} else {
			for _, s := range e.Stmts {
				c.checkStmt(s, closureScope)
			}
		}

		// A closure throws when it says so, or where a function that
		// may throw is expected: `apply(3, { x in ... })` passes one.
		throws := e.Sig != nil && e.Sig.Throws != nil
		if expSig != nil && expSig.Throws {
			throws = true
		}
		// The same for async: `Task { ... }` passes one.
		async := e.Sig != nil && e.Sig.Async.IsValid()
		if expSig != nil && expSig.Async {
			async = true
		}
		return &types.Signature{Params: params, Results: retType, Throws: throws, Async: async, Isolated: c.currIsolated}

	default:
		return types.Typ[types.Invalid]
	}
}

// unparen is e without the parentheses around it.
func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// implicitTakesOther reports whether an operator's operands are one type,
// so that a leading-dot member on one side names a member of the other's.
// implicitOperand reports whether an operand is a leading-dot member,
// which has no type until something says what it is a member of: `.none`,
// or a call of one, `.seconds(1)`.
func implicitOperand(e ast.Expr) bool {
	switch x := unparen(e).(type) {
	case *ast.ImplicitMemberExpr:
		return true
	case *ast.CallExpr:
		_, ok := unparen(x.Fun).(*ast.ImplicitMemberExpr)
		return ok
	}
	return false
}

func implicitTakesOther(op string) bool {
	switch op {
	case "==", "!=", "??", "~=", "<", "<=", ">", ">=":
		return true
	}
	// `t + .Seconds(1)`: an arithmetic operator's operands are what its
	// declaration takes, which for one on a type of the program's own
	// need not be the other operand's type. See implicitOperandContext.
	return sharesOperandType(op)
}

// foldSign records a signed literal's value on the prefix expression.
func (c *checker) foldSign(e *ast.PrefixExpr, op string) {
	lit, ok := e.X.(*ast.BasicLit)
	if !ok || (op != "-" && op != "+") {
		return
	}
	v, ok := c.info.Values[lit]
	if !ok || !v.IsValid() {
		return
	}
	if op == "+" {
		c.info.Values[e] = v
		return
	}
	switch v.Kind {
	case IntValue:
		c.info.Values[e] = Value{Kind: IntValue, Int: ^v.Int + 1}
	case FloatValue:
		c.info.Values[e] = Value{Kind: FloatValue, Float: -v.Float}
	}
}

// rangeElement returns the bound type if expected is a range, or expected itself.
func (c *checker) rangeElement(expected types.Type) types.Type {
	if bound, _, ok := c.info.AnyRangeOf(expected); ok {
		return bound
	}
	return expected
}

func sharesOperandType(op string) bool {
	switch op {
	case "+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>":
		return true
	}
	return false
}

// initializerFor is an initializer's signature for the instance being
// made: a generic type's, with its parameters standing for the arguments
// the call's own have given the instance.
func initializerFor(inst types.Type, sig *types.Signature) *types.Signature {
	subst := types.InstanceSubst(inst)
	if subst == nil {
		return sig
	}
	if out, ok := types.Substitute(sig, subst).(*types.Signature); ok {
		return out
	}
	return sig
}

// pickInitializer finds an initializer matching the call's argument labels and count.
func (c *checker) pickInitializer(inits []*types.Signature, args []*ast.CallArg) *types.Signature {
	var found *types.Signature
	for _, sig := range inits {
		if sig == nil || !c.labelsFit(sig, args) {
			continue
		}
		if found != nil {
			return nil
		}
		found = sig
	}
	return found
}

// pickInitializerFitting is the one initializer the arguments fit as a
// call's do, leaving out parameters that have defaults:
// `AsyncStream<Int> { c in ... }` is `init(_:_:)` given its closure.
func (c *checker) pickInitializerFitting(inits []*types.Signature, args []*ast.CallArg, scope *Scope) *types.Signature {
	if len(inits) == 0 {
		return nil
	}
	quiet := len(c.info.Diagnostics)
	argTypes := make([]types.Type, len(args))
	for i, arg := range args {
		argTypes[i] = c.checkExpr(arg.X, nil, scope)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	var found *types.Signature
	for _, sig := range inits {
		if sig == nil || !c.sigFits(sig, args, argTypes, c.labelFits) {
			continue
		}
		if found != nil {
			return nil
		}
		found = sig
	}
	return found
}

// initResult is what a call of the initializer sig makes: inst, or an
// optional of it where the initializer is failable.
func initResult(inst types.Type, sig *types.Signature) types.Type {
	if sig != nil && sig.Failable {
		return &types.Optional{Wrapped: inst}
	}
	return inst
}

// pickInitializerByType is pickInitializer for initializers that share
// their labels and differ by type -- the core's `Int(bitPattern: UInt)`
// beside a pointer's, `Int(exactly: Double)` beside `Int(exactly: Float)`:
// the labels narrow first, then the arguments' types, then Swift's
// preference for a literal as the type it is alone. It is nil when none
// fits, or when several still do.
func (c *checker) pickInitializerByType(inits []*types.Signature, args []*ast.CallArg, scope *Scope) *types.Signature {
	var labelled []*types.Signature
	for _, sig := range inits {
		if sig != nil && c.labelsFit(sig, args) {
			labelled = append(labelled, sig)
		}
	}
	// One that fits by its labels is still checked by type: String(i) is
	// not the String(_: Character) an extension declares, but the
	// built-in conversion of an Int.
	if len(labelled) == 0 {
		return nil
	}
	quiet := len(c.info.Diagnostics)
	argTypes := make([]types.Type, len(args))
	for i, arg := range args {
		argTypes[i] = c.checkExpr(arg.X, nil, scope)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	var fits []*types.Signature
	for _, sig := range labelled {
		if c.sigFits(sig, args, argTypes, c.labelFits) {
			fits = append(fits, sig)
		}
	}
	if len(fits) > 1 {
		// One that cannot fail, as `UnicodeScalar(65)` is its
		// init(_: UInt8) beside init?(_: UInt32) and init?(_: Int).
		var sure []*types.Signature
		for _, sig := range fits {
			if !sig.Failable {
				sure = append(sure, sig)
			}
		}
		if len(sure) == 1 {
			return sure[0]
		}
		if keep := c.byLiteralDefaults(fits, args); len(keep) == 1 {
			return fits[keep[0]]
		}
	}
	if len(fits) != 1 {
		return nil
	}
	return fits[0]
}

// labelsFit reports whether arguments fit a signature by their labels
// alone: in order, leaving out a parameter with a default where nothing is
// written for it, as `init(seconds: Int64, nanos: Int64 = 0)` is called
// with `seconds:` only.
func (c *checker) labelsFit(sig *types.Signature, args []*ast.CallArg) bool {
	if len(args) > len(sig.Params) && !hasVariadic(sig) {
		return false
	}
	next := 0
	for _, p := range sig.Params {
		if p.Variadic {
			for first := true; next < len(args); first = false {
				if (first && !c.labelFits(args[next], p)) || (!first && args[next].Label != nil) {
					break
				}
				next++
			}
			continue
		}
		if next < len(args) && c.labelFits(args[next], p) {
			next++
			continue
		}
		if !p.HasDefault {
			return false
		}
	}
	return next == len(args)
}

// soleInitializerOfArity returns the sole initializer taking n arguments, or nil.
func (c *checker) soleInitializerOfArity(inits []*types.Signature, n int) *types.Signature {
	var found *types.Signature
	for _, sig := range inits {
		if sig == nil || len(sig.Params) != n {
			continue
		}
		if found != nil {
			return nil
		}
		found = sig
	}
	return found
}

// namesModule reports whether an expression refers to a module name.
func (c *checker) namesModule(e ast.Expr, scope *Scope) bool {
	id, ok := e.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return false
	}
	name := id.Name.Text(c.file)
	if c.modules[name] == nil {
		return false
	}
	// A function of the same name -- `main` beside module main -- has
	// no members, so a member of the name is the module's.
	switch scope.Lookup(name).(type) {
	case nil, *FuncSymbol:
		return true
	}
	return false
}

// moduleMemberValue resolves `Module.name` in expression position.
func (c *checker) moduleMemberValue(e *ast.MemberExpr, scope *Scope) (types.Type, bool) {
	if e.Name == nil || !c.namesModule(e.X, scope) {
		return nil, false
	}
	module := e.X.(*ast.IdentExpr).Name.Text(c.file)
	name := e.Name.Text(c.file)
	sym := c.modules[module].Lookup(name)
	if sym == nil {
		c.errorf(e.Name.Pos(), "cannot find '%s.%s' in scope: no such name in %s",
			module, name, module)
		return types.Typ[types.Invalid], true
	}
	c.info.Uses[e.Name] = sym
	if _, ok := sym.(*TypeNameSymbol); ok {
		instance := sym.Type()
		// `gpu.Shared<Int32>(count: 4)` names its instance outright, as
		// `Shared<Int32>(count: 4)` does unqualified.
		if e.Args != nil && len(e.Args.Args) > 0 {
			args := make([]types.Type, len(e.Args.Args))
			for i, a := range e.Args.Args {
				args[i] = c.resolveType(a, scope)
			}
			instance = &types.GenericInstance{Base: instance, Args: args}
		}
		return &types.Metatype{Instance: instance}, true
	}
	return sym.Type(), true
}

// isUntypedNil reports whether t is types.UntypedNil.
func isUntypedNil(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.UntypedNil
}

// optionalOperands handles equality comparisons involving optional types.
func (c *checker) optionalOperands(e *ast.BinaryExpr, lhs, rhs types.Type, scope *Scope) bool {
	lo, lIsOpt := lhs.(*types.Optional)
	ro, rIsOpt := rhs.(*types.Optional)
	switch {
	case lIsOpt && rIsOpt:
		return types.AssignableTo(ro.Wrapped, lo.Wrapped) ||
			types.AssignableTo(lo.Wrapped, ro.Wrapped)
	case lIsOpt:
		if t, adopted := c.adoptTree(e.Y, lo.Wrapped, scope); adopted {
			rhs = t
		}
		return types.AssignableTo(rhs, lo.Wrapped)
	case rIsOpt:
		if t, adopted := c.adoptTree(e.X, ro.Wrapped, scope); adopted {
			lhs = t
		}
		return types.AssignableTo(lhs, ro.Wrapped)
	}
	return false
}

// operatorReference types an operator written as a value, `+` in
// `reduce(0, +)`: the declaration of the operator whose parameters are
// the ones the context's function type has, where the context says
// them, and which is recorded for the expression as an operator use is.
func (c *checker) operatorReference(e *ast.OperatorExpr, expected types.Type, scope *Scope) types.Type {
	op := c.operatorSpelling(e)
	want, ok := expected.(*types.Signature)
	if !ok {
		c.typeErrorf(e.Pos(), "cannot infer the type of operator '%s' used as a value here", op)
		return types.Typ[types.Invalid]
	}
	operands := make([]types.Type, len(want.Params))
	for i, p := range want.Params {
		operands[i] = p.Type
	}
	// A parameter the context leaves generic -- the Result of reduce --
	// takes the type of the operand beside it, an operator's operands
	// being alike as a rule.
	var known types.Type
	for _, t := range operands {
		if _, isParam := t.(*types.TypeParam); !isParam && t != nil && !isInvalid(t) {
			known = t
		}
	}
	for i, t := range operands {
		if _, isParam := t.(*types.TypeParam); isParam && known != nil {
			operands[i] = known
		}
	}
	for _, ch := range c.operatorChoices(scope, op, operands) {
		sig := ch.sig()
		// A generic one, specialized for the operands: `sorted(by: <)`
		// over tuples is Swift's `<` on tuples of their types.
		if ch.fn != nil && len(sig.TypeParams) > 0 {
			subst, ok := c.inferOperator(sig, operands)
			if !ok {
				continue
			}
			ch.subst = subst
			c.info.Operators[e] = ch.fn
			spec := Specialization{Params: sig.TypeParams}
			for _, p := range sig.TypeParams {
				spec.Args = append(spec.Args, subst[p])
			}
			c.info.OperatorSpecs[e] = spec
			return ch.sig()
		}
		fits := true
		for i, t := range operands {
			if t == nil || isInvalid(t) {
				continue
			}
			if !types.AssignableTo(t, sig.Params[i].Type) {
				fits = false
			}
		}
		if fits {
			c.chooseOperator(e, ch)
			return sig
		}
	}
	c.typeErrorf(e.Pos(), "no operator '%s' takes '%s'", op, want)
	return types.Typ[types.Invalid]
}

// closuresFit reports whether every closure among the arguments goes
// where a function is taken: a closure given to a runtime method's
// non-function parameter means an extension's method of that name --
// `contains(where:)` rather than `contains(_:)`.
func closuresFit(args []*ast.CallArg, params []*types.Param) bool {
	for i, a := range args {
		if _, isClosure := unparen(a.X).(*ast.ClosureExpr); !isClosure {
			continue
		}
		if i >= len(params) {
			return false
		}
		if _, isFunc := params[i].Type.Underlying().(*types.Signature); !isFunc {
			return false
		}
	}
	return true
}

// valuesFit reports whether the arguments that are not closures convert
// to their parameters, checked without a trace: the runtime's
// `append(_: String)` is not the call `t.append(c)` with c a Character,
// which an extension's append(_: Character) is.
func (c *checker) valuesFit(args []*ast.CallArg, params []*types.Param, scope *Scope) bool {
	for i, a := range args {
		if i >= len(params) {
			return false
		}
		if _, isClosure := unparen(a.X).(*ast.ClosureExpr); isClosure {
			continue
		}
		if !c.fitsQuietly(a.X, params[i].Type, scope) {
			return false
		}
	}
	return true
}

// builtinMethodCall types a call of a method an extension gives a
// built-in type, chosen by the arguments among those of its name and
// typed for the elements of the receiver.
func (c *checker) builtinMethodCall(e *ast.CallExpr, mem *ast.MemberExpr, baseType types.Type, args []*ast.CallArg, scope *Scope) (types.Type, bool) {
	name := mem.Name.Text(c.file)
	recv, methods := c.builtinMethods(baseType, name)
	if len(methods) == 0 {
		return nil, false
	}
	b := c.builtinOf(baseType)
	if b == nil {
		return nil, false
	}
	subst := b.Subst(baseType)
	substituted := func(m *types.Method) *types.Signature {
		if len(subst) == 0 {
			return m.Sig
		}
		if s, ok := types.Substitute(m.Sig, subst).(*types.Signature); ok {
			return s
		}
		return m.Sig
	}
	chosen := methods[0]
	if len(methods) > 1 {
		candidates := make([]*types.Method, len(methods))
		for i, m := range methods {
			copied := *m
			copied.Sig = substituted(m)
			candidates[i] = &copied
		}
		picked := c.methodByArguments(candidates, args, scope)
		if picked == nil {
			return nil, false
		}
		for i, cand := range candidates {
			if cand == picked {
				chosen = methods[i]
			}
		}
	}
	sig := substituted(chosen)
	if chosen.IsMutating {
		c.checkMutableReceiver(mem.X, mem.Name.Pos(), "cannot use mutating member on immutable value", scope)
	}
	c.info.Methods[mem] = &MethodRef{Recv: recv, Method: chosen}
	c.info.Types[mem] = sig
	c.checkMemberConditions(mem, baseType, name)
	return c.checkCallArguments(e, sig, args, scope).Results, true
}

// declaredSubscript resolves `s[args]` on a value of a type that declares
// subscripts -- or on the type, for a static one -- to the one whose
// labels and parameters the arguments fit, and is its result.
func (c *checker) declaredSubscript(e *ast.SubscriptExpr, baseType types.Type, scope *Scope) (types.Type, bool) {
	if baseType == nil || isInvalid(baseType) {
		return nil, false
	}
	recv, static := baseType, false
	if meta, ok := baseType.(*types.Metatype); ok {
		recv, static = meta.Instance, true
	}
	var candidates []*SubscriptRef
	for t := recv; t != nil; {
		for _, sub := range subscriptsOf(t) {
			if sub != nil && sub.IsStatic == static {
				candidates = append(candidates, &SubscriptRef{Recv: t, Subscript: sub})
			}
		}
		cl, ok := t.Underlying().(*types.Class)
		if !ok || cl.Superclass == nil {
			break
		}
		t = cl.Superclass
	}
	// What the protocols a type parameter, an associated type or Self is
	// constrained to require: Collection's subscript, on a C: Collection.
	candidates = append(candidates, requiredSubscripts(recv, static)...)
	// What an extension of a built-in type declares: String's by
	// String.Index, and a generic one's -- Set's by position -- for the
	// elements of the one it is used on.
	if b := c.builtinOf(recv); b != nil {
		for _, sub := range b.Subscripts {
			if sub == nil || sub.IsStatic != static {
				continue
			}
			candidates = append(candidates, &SubscriptRef{Recv: b.Type, Subscript: sub})
		}
	}
	if len(candidates) == 0 {
		switch recv.Underlying().(type) {
		case *types.Struct, *types.Class, *types.Enum:
			c.typeErrorf(e.Lsquare, "value of type '%s' has no subscripts", baseType)
			for _, arg := range e.Args {
				c.checkExpr(arg.X, nil, scope)
			}
			return types.Typ[types.Invalid], true
		}
		return nil, false
	}
	// The subscript whose labels and arity fit, and then whose
	// parameters the arguments convert to: types are tried against
	// each in turn, as an overloaded call's are.
	fits := func(ref *SubscriptRef) bool {
		params := ref.Subscript.Params
		if len(params) != len(e.Args) {
			return false
		}
		for i, arg := range e.Args {
			label := ""
			if arg.Label != nil {
				label = arg.Label.Text(c.file)
			}
			if label != params[i].Label && !(params[i].Label == "_" && label == "") {
				return false
			}
		}
		return true
	}
	var chosen *SubscriptRef
	// The arguments' own types pick first, as an overloaded method's do:
	// `v[1]` takes the subscript by Int, not an earlier one by String.
	quiet := len(c.info.Diagnostics)
	argTypes := make([]types.Type, len(e.Args))
	for i, arg := range e.Args {
		argTypes[i] = c.checkExpr(arg.X, nil, scope)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	var viable []*SubscriptRef
	var sigs []*types.Signature
	for _, ref := range candidates {
		if !fits(ref) || len(ref.Subscript.TypeParams) > 0 {
			continue
		}
		ok := true
		for i, arg := range e.Args {
			if !c.argFitsParam(arg, argTypes[i], ref.Subscript.Params[i]) {
				ok = false
				break
			}
		}
		if ok {
			viable = append(viable, ref)
			sigs = append(sigs, &types.Signature{Params: ref.Subscript.Params})
		}
	}
	if len(viable) > 1 {
		if keep := c.byLiteralDefaults(sigs, e.Args); len(keep) > 0 {
			viable = []*SubscriptRef{viable[keep[0]]}
		}
	}
	if len(viable) > 0 {
		chosen = viable[0]
	}
	for _, ref := range candidates {
		if chosen != nil {
			break
		}
		if !fits(ref) {
			continue
		}
		ok := true
		quiet := len(c.info.Diagnostics)
		for i, arg := range e.Args {
			if _, isLit := literalUnder(arg.X); isLit {
				continue
			}
			got := c.checkExpr(arg.X, ref.Subscript.Params[i].Type, scope)
			if !isInvalid(got) && !types.AssignableTo(got, ref.Subscript.Params[i].Type) {
				ok = false
				break
			}
		}
		c.info.Diagnostics = c.info.Diagnostics[:quiet]
		if ok {
			chosen = ref
			break
		}
	}
	if chosen == nil {
		for _, ref := range candidates {
			if fits(ref) {
				chosen = ref
				break
			}
		}
	}
	if chosen == nil {
		c.typeErrorf(e.Lsquare, "no subscript of '%s' takes these arguments", baseType)
		for _, arg := range e.Args {
			c.checkExpr(arg.X, nil, scope)
		}
		return types.Typ[types.Invalid], true
	}
	// A generic type's subscript is its instance's: Box<Int>'s subscript
	// answers an Int where it was declared answering a T.
	instSub := func(t types.Type) types.Type { return t }
	if inst := genericInstanceOf(recv); inst != nil {
		params := typeParamsOf(inst.Base)
		subst := make(map[*types.TypeParam]types.Type, len(params))
		for i, p := range params {
			if i < len(inst.Args) {
				subst[p] = inst.Args[i]
			}
		}
		instSub = func(t types.Type) types.Type { return types.Substitute(t, subst) }
	} else if b := c.builtinOf(recv); b != nil && len(b.Params) > 0 && chosen.Recv == b.Type {
		subst := b.Subst(recv)
		instSub = func(t types.Type) types.Type { return types.Substitute(t, subst) }
	}
	// Its own generic parameters are what the arguments say.
	var own map[*types.TypeParam]types.Type
	if len(chosen.Subscript.TypeParams) > 0 {
		own = map[*types.TypeParam]types.Type{}
		for i, arg := range e.Args {
			got := c.checkExpr(arg.X, instSub(chosen.Subscript.Params[i].Type), scope)
			if got != nil && !isInvalid(got) {
				types.Unify(instSub(chosen.Subscript.Params[i].Type), got, own)
			}
		}
		outer := instSub
		instSub = func(t types.Type) types.Type { return types.Substitute(outer(t), own) }
		chosen = &SubscriptRef{Recv: chosen.Recv, Subscript: chosen.Subscript, Subst: own}
	}
	for i, arg := range e.Args {
		want := instSub(chosen.Subscript.Params[i].Type)
		got := c.checkExpr(arg.X, want, scope)
		if t, adopted := c.adopt(arg.X, want); adopted {
			got = t
		}
		if !isInvalid(got) && !types.AssignableTo(got, want) {
			c.typeErrorf(arg.X.Pos(), "cannot convert value of type '%s' to expected argument type '%s'", got, want)
		}
	}
	c.info.Subscripts[e] = chosen
	return instSub(chosen.Subscript.Result), true
}

// checkSubscriptWrite says what is wrong with writing through a declared
// subscript where something is: one with no setter, or one of a value
// held in a `let`.
func (c *checker) checkSubscriptWrite(sub *ast.SubscriptExpr, scope *Scope) {
	ref := c.info.Subscripts[sub]
	if ref == nil {
		return
	}
	if !ref.Subscript.Settable {
		c.errorf(sub.Lsquare, "cannot assign through subscript: subscript is get-only")
		return
	}
	if _, isClass := ref.Recv.Underlying().(*types.Class); !isClass && !ref.Subscript.IsStatic && !ref.Subscript.NonmutatingSet {
		c.checkMutableReceiver(sub.X, sub.Lsquare, "cannot assign through subscript", scope)
	}
}

// subscriptsOf is the subscripts a type declares.
func subscriptsOf(t types.Type) []*types.Subscript {
	if inst, ok := t.(*types.GenericInstance); ok {
		t = inst.Base
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		return u.Subscripts
	case *types.Class:
		return u.Subscripts
	case *types.Enum:
		return u.Subscripts
	}
	return nil
}

// payloadOperator records how the payloads of an optional comparison are
// compared: the operator the type they wrap declares or derives, where it
// declares one, found as it would be for two values of that type.
func (c *checker) payloadOperator(scope *Scope, op string, e *ast.BinaryExpr, lhs, rhs types.Type) {
	w := lhs
	for {
		o, ok := w.(*types.Optional)
		if !ok {
			break
		}
		w = o.Wrapped
	}
	other := rhs
	for {
		o, ok := other.(*types.Optional)
		if !ok {
			break
		}
		other = o.Wrapped
	}
	if !types.AssignableTo(other, w) {
		w = other
	}
	c.info.OptionalCompares[e] = w
	c.resolveOperator(scope, op, e, []ast.Expr{e.X, e.Y}, []types.Type{w, w})
}

// initializesOwnProperty reports whether assigning name inside the body initializes a stored property.
func (c *checker) initializesOwnProperty(sym Symbol, name string) bool {
	if !c.inInit || c.currType == nil {
		return false
	}
	own := c.typeScope(c.currType)
	return own != nil && own.LookupLocal(name) == sym
}

// lookupValue looks up a name in scope, falling back to superclass properties.
// instanceMember reports whether name, looked up from scope, is an instance
// method or property of the type whose members are in scope -- found there
// before anything outside the type.
func (c *checker) instanceMember(scope *Scope, name string) bool {
	found, sym := scope.LookupParent(name)
	if found == nil || !found.members {
		return false
	}
	switch s := sym.(type) {
	case *FuncSymbol:
		if d, ok := s.decl.(*ast.FuncDecl); ok {
			return !c.hasModifier(d.Mods, "static") && !c.hasModifier(d.Mods, "class")
		}
	case *VarSymbol:
		if d, ok := s.decl.(*ast.VarDecl); ok {
			return !c.hasModifier(d.Mods, "static") && !c.hasModifier(d.Mods, "class")
		}
	}
	return false
}

func (c *checker) lookupValue(scope *Scope, name string) Symbol {
	if scope != nil {
		if sym := scope.Lookup(name); sym != nil {
			return sym
		}
	}
	cl, ok := c.currType.(*types.Class)
	if !ok {
		return nil
	}
	seen := map[*types.Class]bool{cl: true}
	for super := cl.Superclass; super != nil; {
		// Through an instance of a generic class, a member is typed for
		// the instance's arguments, which its declaration's symbol is
		// not: it is self's member, `self.items`; see implicitSelfMember.
		if _, generic := super.(*types.GenericInstance); generic {
			return nil
		}
		next, ok := super.(*types.Class)
		if !ok {
			next, _ = super.Underlying().(*types.Class)
		}
		if next == nil || seen[next] {
			return nil
		}
		seen[next] = true
		if s := c.typeScopes[next.Name]; s != nil {
			if sym := s.LookupLocal(name); sym != nil {
				return sym
			}
		}
		super = next.Superclass
	}
	return nil
}

// unwrappedContext returns the wrapped type if t is optional, or t itself.
func unwrappedContext(t types.Type) types.Type {
	if o, ok := t.(*types.Optional); ok {
		return o.Wrapped
	}
	return t
}

// isOptionalType reports whether a type is an optional.
func isOptionalType(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Optional)
	return ok
}

// checkMutableReceiver reports an error if a mutating operation is attempted on a `let` binding.
func (c *checker) checkMutableReceiver(recv ast.Expr, at token.Pos, what string, scope *Scope) {
	id, ok := recv.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return
	}
	name := id.Name.Text(c.file)
	if v, ok := c.lookupValue(scope, name).(*VarSymbol); ok && v.IsConst() {
		c.errorf(at, "%s: '%s' is a 'let' constant", what, name)
	}
}

// pointerOffset is the type of `p + n`, `n + p` or `p - n` where p is a
// pointer and n an Int, or false where the operands are not that.
func (c *checker) pointerOffset(e *ast.BinaryExpr, lhs, rhs types.Type, commutes bool) (types.Type, bool) {
	intT := types.Typ[types.Int]
	isOffset := func(x ast.Expr, t types.Type) bool {
		if _, ok := c.adopt(x, intT); ok {
			return true
		}
		return t != nil && types.Identical(t, intT)
	}
	if lhs != nil {
		if _, ok := lhs.Underlying().(*types.Pointer); ok && isOffset(e.Y, rhs) {
			return lhs, true
		}
	}
	if commutes && rhs != nil {
		if _, ok := rhs.Underlying().(*types.Pointer); ok && isOffset(e.X, lhs) {
			return rhs, true
		}
	}
	return nil, false
}

// staticsMaking is the static methods of t named name that return a t,
// which are what a leading-dot call where a t is wanted can mean.
func staticsMaking(t types.Type, name string) []*types.Method {
	_, methods := methodsNamed(&types.Metatype{Instance: t}, name)
	var out []*types.Method
	for _, m := range methods {
		if m.Sig != nil && m.Sig.Results != nil && types.AssignableTo(m.Sig.Results, t) {
			out = append(out, m)
		}
	}
	return out
}

// isArrayType reports whether t is an array.
func isArrayType(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Array)
	return ok
}

// integerBound is `Int64.max` and the other integer types' min and max:
// constants of the type, recorded as values so they lower as literals do.
func (c *checker) integerBound(e *ast.MemberExpr, base types.Type) (types.Type, bool) {
	if e.Name == nil {
		return nil, false
	}
	return c.integerBoundNamed(e, e.Name.Text(c.file), base)
}

// integerBoundNamed is integerBound for any expression naming one: a
// member expression, or `.max` where the context says which integer.
func (c *checker) integerBoundNamed(e ast.Expr, name string, base types.Type) (types.Type, bool) {
	meta, ok := base.(*types.Metatype)
	if !ok {
		return nil, false
	}
	b, ok := meta.Instance.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsInteger == 0 || b.Info()&types.IsUntyped != 0 {
		return nil, false
	}
	var bits uint
	switch b.Kind() {
	case types.Int8, types.UInt8:
		bits = 8
	case types.Int16, types.UInt16:
		bits = 16
	case types.Int32, types.UInt32:
		bits = 32
	case types.Int, types.UInt, types.Int64, types.UInt64:
		bits = 64
	default:
		return nil, false
	}
	signed := b.Info()&types.IsUnsigned == 0
	if name == "bitWidth" {
		c.info.Values[e] = Value{Kind: IntValue, Int: uint64(bits)}
		c.info.Types[e] = types.Typ[types.Int]
		return types.Typ[types.Int], true
	}
	var v uint64
	switch name {
	case "max":
		switch {
		case signed:
			v = 1<<(bits-1) - 1
		case bits == 64:
			v = ^uint64(0)
		default:
			v = 1<<bits - 1
		}
	case "min":
		if signed {
			// Two's complement, sign-extended: the bits of -(2^(bits-1)).
			v = ^uint64(0) << (bits - 1)
		}
	default:
		return nil, false
	}
	c.info.Values[e] = Value{Kind: IntValue, Int: v}
	c.info.Types[e] = meta.Instance
	return meta.Instance, true
}

// typeOf is `type(of: x)`: the metatype of x's type, which Swift's
// standard library declares with a signature no Swift function can
// have -- its result is the dynamic type of its argument.
func (c *checker) typeOf(e *ast.CallExpr, scope *Scope) (types.Type, bool) {
	id, ok := e.Fun.(*ast.IdentExpr)
	if !ok || id.Name == nil || id.Name.Text(c.file) != "type" || scope.Lookup("type") != nil {
		return nil, false
	}
	if e.Args == nil || len(e.Args.Args) != 1 || e.Args.Args[0].Label == nil ||
		e.Args.Args[0].Label.Text(c.file) != "of" {
		return nil, false
	}
	t := c.checkExpr(e.Args.Args[0].X, nil, scope)
	if d := literalDefault(t); d != t {
		t = d
		c.info.Types[e.Args.Args[0].X] = t
	}
	c.info.TypeOfs[e] = t
	out := &types.Metatype{Instance: t}
	c.info.Types[e] = out
	return out, true
}

// isReferenceOperand reports whether t can be an operand of === : a class,
// AnyObject or an existential of a class-bound protocol, optionally, or nil.
func isReferenceOperand(t types.Type) bool {
	if isUntypedNil(t) {
		return true
	}
	if o, ok := t.(*types.Optional); ok {
		t = o.Wrapped
	}
	if t == nil {
		return false
	}
	if _, ok := t.Underlying().(*types.Class); ok {
		return true
	}
	return types.ConformsTo(t, types.AnyObjectProtocol)
}

// literalDefault is the type a literal is where nothing says otherwise:
// an integer an Int, a float a Double. Any other type is itself.
func literalDefault(t types.Type) types.Type {
	b, ok := t.(*types.Basic)
	if !ok {
		return t
	}
	switch b.Kind() {
	case types.UntypedInt:
		return types.Typ[types.Int]
	case types.UntypedFloat:
		return types.Typ[types.Double]
	case types.UntypedString:
		return types.Typ[types.String]
	case types.UntypedBool:
		return types.Typ[types.Bool]
	}
	return t
}

// memoryLayout is `MemoryLayout<T>.size`, `.stride` or `.alignment`: an
// Int the lowering works out once T is known.
func (c *checker) memoryLayout(e *ast.MemberExpr, base types.Type) (types.Type, bool) {
	meta, ok := base.(*types.Metatype)
	if !ok || e.Name == nil {
		return nil, false
	}
	inst, ok := meta.Instance.(*types.GenericInstance)
	if !ok || !c.info.CoreTypes[inst.Base] || len(inst.Args) != 1 {
		return nil, false
	}
	if en, ok := inst.Base.Underlying().(*types.Enum); !ok || en.Name != "MemoryLayout" {
		return nil, false
	}
	switch kind := e.Name.Text(c.file); kind {
	case "size", "stride", "alignment":
		c.info.Layouts[e] = Layout{Of: inst.Args[0], Kind: kind}
		c.info.Types[e] = types.Typ[types.Int]
		return types.Typ[types.Int], true
	}
	return nil, false
}

// isImportedType reports whether another module declared t.
func (c *checker) isImportedType(t types.Type) bool {
	if gi, ok := t.(*types.GenericInstance); ok {
		t = gi.Base
	}
	if t == nil {
		return false
	}
	if m, ok := c.info.ImportedTypes[t]; ok && m != "" && m != "Swift" {
		return true
	}
	m, ok := c.info.ImportedTypes[t.Underlying()]
	return ok && m != "" && m != "Swift"
}

// checkImportedMember holds a property of another module's type to its
// access: one that is not public is that module's own, and its interface
// lists a stored one only so the type can be laid out.
func (c *checker) checkImportedMember(e *ast.MemberExpr, base types.Type, name string) {
	if meta, ok := base.(*types.Metatype); ok {
		base = meta.Instance
	}
	if base == nil || !c.isImportedType(base) {
		return
	}
	var fields []*types.Field
	switch u := base.Underlying().(type) {
	case *types.Struct:
		fields = u.Fields
	case *types.Class:
		fields = u.Fields
	default:
		return
	}
	for _, f := range fields {
		if f != nil && f.Name == name && !f.Exported {
			c.errorf(e.Name.Pos(), "'%s' is inaccessible due to 'internal' protection level", name)
			return
		}
	}
}

// checkImportedInit holds a call of another module's initializer to its
// access, as checkImportedMember does a property.
func (c *checker) checkImportedInit(e *ast.CallExpr, inst types.Type, sig *types.Signature) {
	if !sig.Exported && c.isImportedType(inst) {
		c.typeErrorf(e.Pos(), "'%s' initializer is inaccessible due to 'internal' protection level", inst)
	}
}

// basicInit checks an initializer of a core type that answers something
// other than a conversion does: `Int("42")`, an Int? parsed from text, and
// `String(decoding: bytes, as: UTF8.self)`. It reports false for any other.
func (c *checker) basicInit(e *ast.CallExpr, b *types.Basic, inst types.Type, args []*ast.CallArg, scope *Scope) (types.Type, bool) {
	label := func(i int) string {
		if args[i].Label == nil {
			return ""
		}
		return args[i].Label.Text(c.file)
	}
	switch {
	// `Int(bitPattern: p)` and `UInt(bitPattern: p)` are p's address, and
	// zero for a nil p: Swift's init(bitPattern:) of any pointer, optional
	// or not.
	case (b.Kind() == types.Int || b.Kind() == types.UInt) && len(args) == 1 && label(0) == "bitPattern":
		t := c.checkExpr(args[0].X, nil, scope)
		if o, ok := t.Underlying().(*types.Optional); ok {
			t = o.Wrapped
		}
		if _, isPtr := t.Underlying().(*types.Pointer); !isPtr {
			return nil, false
		}
		return inst, true
	case b.Info()&types.IsNumeric != 0 && len(args) == 1 && label(0) == "":
		t := c.checkExpr(args[0].X, nil, scope)
		if !isString(t) {
			return nil, false
		}
		out := &types.Optional{Wrapped: inst}
		c.info.Types[e] = out
		return out, true
	case b.Kind() == types.String && len(args) == 2 && label(0) == "decoding" && label(1) == "as":
		bytes := &types.Array{Elem: types.Typ[types.UInt8]}
		// Any sequence of bytes -- a slice, a string's utf8 -- is read as
		// the array of them: `String(decoding: buf[0..<n], as: UTF8.self)`.
		quiet := len(c.info.Diagnostics)
		t := c.checkExpr(args[0].X, bytes, scope)
		c.info.Diagnostics = c.info.Diagnostics[:quiet]
		if !types.AssignableTo(t, bytes) && !isInvalid(t) {
			if seq, ok := c.coreProtocol("Sequence"); ok && c.conformsTo(t, seq) {
				span := ast.Span{Lo: args[0].X.Pos(), Hi: args[0].X.End()}
				fun := &ast.IdentExpr{Span: span, Name: &ast.Ident{Span: span, Synth: "Array"}}
				args[0].X = &ast.CallExpr{Span: span, Fun: fun, Args: &ast.CallArgs{Args: []*ast.CallArg{{X: args[0].X}}}}
			}
		}
		if t := c.checkExpr(args[0].X, bytes, scope); !types.AssignableTo(t, bytes) {
			c.typeErrorf(args[0].X.Pos(), "cannot convert value of type '%s' to expected argument type '%s'", t, bytes)
		}
		codec := c.checkExpr(args[1].X, nil, scope)
		if meta, ok := codec.(*types.Metatype); !ok || typeNameOf(meta.Instance) != "UTF8" {
			c.typeErrorf(args[1].X.Pos(), "String(decoding:as:) decodes UTF8 only so far")
		}
		return inst, true
	}
	return nil, false
}

// chainSpine is the expression a postfix step is applied to: what a member
// is read from, what is called, what is subscripted, forced or chained.
func chainSpine(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.MemberExpr:
		return x.X
	case *ast.CallExpr:
		return x.Fun
	case *ast.SubscriptExpr:
		return x.X
	case *ast.ForceExpr:
		return x.X
	case *ast.OptionalExpr:
		return x.X
	}
	return nil
}

// chainRoot reports whether e is the outermost step of an optional chain:
// a member, call, subscript or force whose spine holds an `a?`, and that is
// not itself a step of a longer chain.
func (c *checker) chainRoot(e ast.Expr, scope *Scope) bool {
	switch o := e.(type) {
	case *ast.MemberExpr, *ast.CallExpr, *ast.SubscriptExpr, *ast.ForceExpr:
	case *ast.OptionalExpr:
		// A bare `x?` -- `s? += "c"` -- is a chain of one step.
		if c.info.ChainRoots[e] {
			return true
		}
		return !c.inChain[e] && !c.optionalOfType(o, scope)
	default:
		return false
	}
	// A root stays one when it is checked again -- a closure's body is,
	// once to infer what it returns and once against that.
	if c.info.ChainRoots[e] {
		return true
	}
	if c.inChain[e] {
		return false
	}
	for x := chainSpine(e); x != nil; x = chainSpine(x) {
		if o, ok := x.(*ast.OptionalExpr); ok {
			return !c.optionalOfType(o, scope)
		}
	}
	return false
}

// optionalOfType reports whether `X?` is a type -- `Int?.none` is
// Optional<Int>'s none -- rather than a step of a chain: X names a type
// and no value.
func (c *checker) optionalOfType(e *ast.OptionalExpr, scope *Scope) bool {
	id, ok := unparen(e.X).(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return false
	}
	name := id.Name.Text(c.file)
	if sym := c.lookupValue(scope, name); sym != nil {
		_, isType := sym.(*TypeNameSymbol)
		return isType
	}
	return scope.LookupType(name) != nil || types.LookupUniverse(name) != nil
}

// markChain marks every step below a chain's root as part of it.
func (c *checker) markChain(root ast.Expr) {
	if c.inChain == nil {
		c.inChain = map[ast.Expr]bool{}
	}
	// A bare `x?` is its own step.
	if _, ok := root.(*ast.OptionalExpr); ok {
		c.inChain[root] = true
	}
	for x := chainSpine(root); x != nil; x = chainSpine(x) {
		c.inChain[x] = true
	}
}

// optionalNamed is the optional `Optional` or `Optional<T>` names in
// expression position, where nothing of the program's own has that name:
// `Optional<Int>`, or the optional wanted for a bare `Optional`.
func (c *checker) optionalNamed(e ast.Expr, expected types.Type, scope *Scope) (*types.Optional, bool) {
	// `Int?`, the same optional spelled short.
	if o, ok := e.(*ast.OptionalExpr); ok && c.optionalOfType(o, scope) {
		if meta, ok := c.checkExpr(o, nil, scope).(*types.Metatype); ok {
			if opt, ok := meta.Instance.(*types.Optional); ok {
				return opt, true
			}
		}
		return nil, false
	}
	id, ok := e.(*ast.IdentExpr)
	if !ok || id.Name == nil || id.Name.Text(c.file) != "Optional" ||
		scope.LookupType("Optional") != nil || c.lookupValue(scope, "Optional") != nil {
		return nil, false
	}
	if id.Args != nil {
		if len(id.Args.Args) != 1 {
			return nil, false
		}
		return &types.Optional{Wrapped: c.resolveType(id.Args.Args[0], scope)}, true
	}
	if o, ok := expected.(*types.Optional); ok {
		return o, true
	}
	return &types.Optional{}, true
}

// optionalNone is `.none` of an optional, spelled as an implicit member
// or on `Optional<T>`: the optional itself.
func (c *checker) optionalNone(e ast.Expr, expected types.Type, name string) (types.Type, bool) {
	o, ok := expected.(*types.Optional)
	if !ok || name != "none" || o.Wrapped == nil {
		return nil, false
	}
	c.info.OptionalNones[e] = o
	return o, true
}

// optionalSome is a call that wraps its one argument in an optional --
// `.some(x)`, `Optional(x)`, `Optional<T>(x)`, `Optional.some(x)` -- and
// the optional it makes: of T where one is named, else of what the
// argument is, or of what the context wants where that agrees.
func (c *checker) optionalSome(e *ast.CallExpr, expected types.Type, scope *Scope) (types.Type, bool) {
	if e.Args == nil || len(e.Args.Args) != 1 || e.Args.Args[0].Label != nil {
		return nil, false
	}
	var opt *types.Optional
	switch f := e.Fun.(type) {
	case *ast.ImplicitMemberExpr:
		o, ok := expected.(*types.Optional)
		if !ok || f.Name == nil || f.Name.Text(c.file) != "some" || c.enumCaseSymbol(o.Wrapped, "some") != nil {
			return nil, false
		}
		opt = o
	case *ast.IdentExpr:
		o, ok := c.optionalNamed(f, expected, scope)
		if !ok {
			return nil, false
		}
		opt = o
	case *ast.MemberExpr:
		o, ok := c.optionalNamed(f.X, expected, scope)
		if !ok || f.Name == nil || f.Name.Text(c.file) != "some" {
			return nil, false
		}
		opt = o
	default:
		return nil, false
	}
	arg := c.checkExpr(e.Args.Args[0].X, opt.Wrapped, scope)
	if opt.Wrapped == nil || isInvalid(opt.Wrapped) {
		opt = &types.Optional{Wrapped: arg}
	} else if t, ok := c.adopt(e.Args.Args[0].X, opt.Wrapped); ok {
		arg = t
	}
	if !isInvalid(arg) && !types.AssignableTo(arg, opt.Wrapped) {
		c.typeErrorf(e.Args.Args[0].X.Pos(), "cannot convert value of type '%s' to expected argument type '%s'", arg, opt.Wrapped)
	}
	c.info.Types[e.Fun] = &types.Metatype{Instance: opt}
	c.info.OptionalSomes[e] = opt
	return opt, true
}

// expressedLiteral is the type a literal of this kind is where expected is
// a type expressible by it -- one that conforms to ExpressibleByIntegerLiteral
// and has init(integerLiteral:), and so on -- or an optional of one, which
// the value made is wrapped in. It records the initializer that makes it.
func (c *checker) expressedLiteral(e ast.Expr, expected types.Type, kind types.BasicKind) (types.Type, bool) {
	target := expected
	if o, ok := target.(*types.Optional); ok && kind != types.UntypedNil {
		target = o.Wrapped
	}
	if target == nil {
		return nil, false
	}
	var inits []*types.Signature
	switch u := target.Underlying().(type) {
	case *types.Struct:
		inits = u.Inits
	case *types.Class:
		inits = u.Inits
	default:
		return nil, false
	}
	for _, p := range types.LiteralProtocols(kind) {
		if !types.ConformsToNamed(target, p) {
			continue
		}
		label := literalLabel(p)
		for _, sig := range inits {
			if sig != nil && len(sig.Params) == 1 && sig.Params[0].Label == label {
				c.info.LiteralInits[e] = &LiteralInit{Type: target, Init: sig}
				return expected, true
			}
		}
	}
	return nil, false
}

// literalLabel is the argument label of the initializer a literal protocol
// requires: integerLiteral for ExpressibleByIntegerLiteral.
func literalLabel(protocol string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(protocol, "ExpressibleBy"), "Literal")
	if name == "" {
		return ""
	}
	if name == "Integer" || name == "Float" || name == "Boolean" || name == "String" || name == "Nil" {
		return strings.ToLower(name) + "Literal"
	}
	return strings.ToLower(name[:1]) + name[1:] + "Literal"
}

// checkCaptureItem declares what one item of a closure's capture list
// binds: `[x]` a constant x of the closure's, holding what x holds when
// the closure is made, and `[y = e]` a constant y holding e. `[self]`
// binds nothing: the closure captures self as it would anyway.
func (c *checker) checkCaptureItem(item *ast.CaptureItem, outer, closure *Scope) {
	var name *ast.Ident
	var value ast.Expr
	switch x := item.X.(type) {
	case *ast.SelfExpr:
		t := c.checkExpr(x, nil, outer)
		// `[weak self]` and `[unowned self]` bind a self of the closure's
		// own: an optional for a weak one.
		if item.Spec != nil && item.Spec.Name != nil && !isInvalid(t) {
			if item.Spec.Name.Text(c.file) == "weak" {
				t = &types.Optional{Wrapped: t}
			}
			sym := NewVar("self", t, x.Pos(), true, types.DefaultOwnership)
			closure.Insert(sym)
			c.info.Captures[item] = &Capture{Sym: sym, Value: x}
		}
		return
	case *ast.IdentExpr:
		name, value = x.Name, x
	case *ast.SequenceExpr:
		if len(x.Elements) >= 3 {
			id, isIdent := x.Elements[0].(*ast.IdentExpr)
			op, isOp := x.Elements[1].(*ast.OperatorExpr)
			if isIdent && isOp && op.Text(c.file) == "=" {
				name = id.Name
				value = x.Elements[2]
				if len(x.Elements) > 3 {
					value = &ast.SequenceExpr{Span: ast.Span{Lo: x.Elements[2].Pos(), Hi: x.End()}, Elements: x.Elements[2:]}
				}
			}
		}
	}
	if name == nil || value == nil {
		c.errorf(item.Pos(), "expected 'weak', 'unowned', or no specifier in capture list")
		return
	}
	t := c.checkExpr(value, nil, outer)
	if item.Spec != nil && item.Spec.Name != nil && item.Spec.Name.Text(c.file) == "weak" {
		if _, isOpt := t.(*types.Optional); !isOpt {
			t = &types.Optional{Wrapped: t}
		}
	}
	sym := NewVar(name.Text(c.file), literalDefault(t), name.Pos(), true, types.DefaultOwnership)
	closure.Insert(sym)
	c.info.Captures[item] = &Capture{Sym: sym, Value: value}
}

// coreInits are the initializers of the built-in generic types the core
// writes as generic functions, by type and argument labels ("" for none).
var coreInits = map[string]string{
	"Dictionary(grouping:by:)":          "_dictionaryGrouping",
	"Dictionary(grouping:_:)":           "_dictionaryGrouping",
	"Dictionary(_:uniquingKeysWith:)":   "_dictionaryUniquing",
	"Dictionary(uniqueKeysWithValues:)": "_dictionaryUniqueKeys",
}

// coreInitCall checks `Dictionary(grouping: xs, by: f)` and the like as
// the call of the core function that makes one, whose type arguments the
// arguments infer as for any generic call.
func (c *checker) coreInitCall(e *ast.CallExpr, scope *Scope) (types.Type, bool) {
	id, ok := e.Fun.(*ast.IdentExpr)
	if !ok || id.Name == nil || id.Args != nil || e.Args == nil {
		return nil, false
	}
	name := id.Name.Text(c.file)
	if scope.LookupType(name) != nil {
		return nil, false
	}
	key := name + "("
	for _, a := range e.Args.Args {
		if a.Label != nil {
			key += a.Label.Text(c.file) + ":"
		} else {
			key += "_:"
		}
	}
	key += ")"
	fn, ok := coreInits[key]
	// Set(xs) of an array, or of a String's Characters.
	if key == "Set(_:)" {
		quiet := len(c.info.Diagnostics)
		at := c.checkExpr(e.Args.Args[0].X, nil, scope)
		c.info.Diagnostics = c.info.Diagnostics[:quiet]
		switch {
		case isArrayType(at):
			fn, ok = "_setOfArray", true
		case isString(at):
			fn, ok = "_setOfString", true
		}
	}
	if !ok || c.modules["Swift"] == nil {
		return nil, false
	}
	sym, ok := c.modules["Swift"].Lookup(fn).(*FuncSymbol)
	if !ok {
		return nil, false
	}
	fun := &ast.IdentExpr{Span: id.Span, Name: &ast.Ident{Span: id.Name.Span}}
	call := &ast.CallExpr{Span: e.Span, Fun: fun, Args: &ast.CallArgs{Span: e.Args.Span}}
	for _, a := range e.Args.Args {
		call.Args.Args = append(call.Args.Args, &ast.CallArg{Span: a.Span, X: a.X})
	}
	c.info.Uses[fun.Name] = sym
	sig := c.checkCallArguments(call, sym.Signature(), call.Args.Args, scope)
	c.info.Types[fun] = sig
	c.info.Types[call] = sig.Results
	c.info.CoreCalls[e] = call
	return sig.Results, true
}

// wantsFunctionOfOne reports whether expected is a function of one
// argument, or an optional of one.
func wantsFunctionOfOne(expected types.Type) bool {
	if expected == nil {
		return false
	}
	t := expected.Underlying()
	if o, ok := t.(*types.Optional); ok && o.Wrapped != nil {
		t = o.Wrapped.Underlying()
	}
	sig, ok := t.(*types.Signature)
	return ok && len(sig.Params) == 1
}

// keyPathClosure is the closure `{ $0.a.b }` that key path `\.a.b` reads
// as, made once: each component a step from the one before, starting at
// the closure's argument. A key path whose root is written -- `\T.a` --
// reads the same way; the closure's argument is that T.
func (c *checker) keyPathClosure(e *ast.KeyPathExpr) *ast.ClosureExpr {
	if cl := c.info.KeyPathClosures[e]; cl != nil {
		return cl
	}
	x := keyPathSteps(e)
	if x == nil {
		return nil
	}
	cl := &ast.ClosureExpr{Span: e.Span, Lbrace: e.Pos(), Rbrace: e.End(),
		Stmts: []ast.Stmt{&ast.ExprStmt{Span: e.Span, X: x}}}
	c.info.KeyPathClosures[e] = cl
	return cl
}

// keyPathSteps is `$0.a.b` for key path `\.a.b`: each component a step
// from the one before, starting at a `$0` of its own -- made anew each
// time, as each closure that reads through the path has its own $0.
func keyPathSteps(e *ast.KeyPathExpr) ast.Expr {
	at := ast.Span{Lo: e.Pos(), Hi: e.Pos()}
	var x ast.Expr = &ast.IdentExpr{Span: at, Name: &ast.Ident{Span: at, Synth: "$0"}}
	for _, k := range e.Components {
		span := ast.Span{Lo: e.Pos(), Hi: k.End()}
		switch {
		case k.Name != nil:
			x = &ast.MemberExpr{Span: span, X: x, Dot: k.Dot, Name: k.Name}
			if k.Args != nil {
				x = &ast.CallExpr{Span: span, Fun: x, Args: k.Args}
			}
		case k.Sub != nil:
			x = &ast.SubscriptExpr{Span: span, X: x, Lsquare: k.Sub.Lsquare, Args: k.Sub.Args, Rsquare: k.Sub.Rsquare}
		case k.Question.IsValid():
			x = &ast.OptionalExpr{Span: span, X: x, Question: k.Question}
		case k.Exclaim.IsValid():
			x = &ast.ForceExpr{Span: span, X: x, Exclaim: k.Exclaim}
		case k.Self.IsValid():
		default:
			return nil
		}
	}
	return x
}

// coreKeyPath is core's KeyPath, the type of a key path used as a value.
func (c *checker) coreKeyPath() types.Type {
	core := c.modules["Swift"]
	if core == nil {
		return nil
	}
	if tn, ok := core.elems["KeyPath"].(*TypeNameSymbol); ok {
		return tn.Type()
	}
	return nil
}

// keyPathValue types a key path used as a value -- `\Person.name`, or
// `\.name` where a KeyPath of a known root is wanted -- as the
// KeyPath<Root, Value> made of the closure reading through it,
// `{ $0.name }`, and the one writing through it, `{ $0.name = $1 }`,
// where that one checks: every step can be written.
func (c *checker) keyPathValue(e *ast.KeyPathExpr, expected types.Type, scope *Scope) types.Type {
	kp := c.coreKeyPath()
	if kp == nil {
		c.errorf(e.Pos(), "cannot use a key path other than as a function yet")
		return types.Typ[types.Invalid]
	}
	var root types.Type
	if e.Type != nil {
		root = c.resolveType(e.Type, scope)
	} else if gi, ok := expected.(*types.GenericInstance); ok && gi.Base == kp && len(gi.Args) == 2 {
		root = gi.Args[0]
	}
	if root == nil || isInvalid(root) {
		c.errorf(e.Pos(), "cannot infer key path type from context; consider explicitly specifying a root type")
		return types.Typ[types.Invalid]
	}
	get := c.keyPathClosure(e)
	if get == nil {
		c.errorf(e.Pos(), "cannot use this key path yet")
		return types.Typ[types.Invalid]
	}
	sig, ok := c.checkExpr(get, &types.Signature{Params: []*types.Param{{Type: root}}}, scope).(*types.Signature)
	if !ok || sig.Results == nil || isInvalid(sig.Results) {
		return types.Typ[types.Invalid]
	}
	value := sig.Results
	kv := &KeyPathValue{Type: &types.GenericInstance{Base: kp, Args: []types.Type{root, value}}, Get: get}
	// The writing one, where it checks.
	body := keyPathSteps(e)
	at := ast.Span{Lo: e.Pos(), Hi: e.Pos()}
	assign := &ast.BinaryExpr{Span: e.Span, X: body,
		Op: &ast.OperatorExpr{Span: at, Kind: token.ASSIGN, Synth: "="},
		Y:  &ast.IdentExpr{Span: at, Name: &ast.Ident{Span: at, Synth: "$1"}}}
	set := &ast.ClosureExpr{Span: e.Span, Lbrace: e.Pos(), Rbrace: e.End(),
		Stmts: []ast.Stmt{&ast.ExprStmt{Span: e.Span, X: assign}}}
	quiet := len(c.info.Diagnostics)
	want := &types.Signature{Params: []*types.Param{{Type: root, Ownership: types.InOut}, {Type: value}},
		Results: types.Typ[types.Void]}
	if _, ok := c.checkExpr(set, want, scope).(*types.Signature); ok && len(c.info.Diagnostics) == quiet {
		kv.Set = set
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	c.info.KeyPathValues[e] = kv
	return kv.Type
}

// keyPathSubscript types `x[keyPath: k]` as the Value of k's KeyPath,
// whose Root is x's type.
func (c *checker) keyPathSubscript(e *ast.SubscriptExpr, scope *Scope) (types.Type, bool) {
	kp := c.coreKeyPath()
	if kp == nil {
		return nil, false
	}
	base := c.checkExpr(e.X, nil, scope)
	if isInvalid(base) {
		return base, true
	}
	var want types.Type
	if _, literal := unparen(e.Args[0].X).(*ast.KeyPathExpr); literal {
		want = &types.GenericInstance{Base: kp, Args: []types.Type{base, types.Typ[types.Invalid]}}
	}
	kt := c.checkExpr(e.Args[0].X, want, scope)
	gi, ok := kt.(*types.GenericInstance)
	if !ok || gi.Base != kp || len(gi.Args) != 2 {
		if !isInvalid(kt) {
			c.typeErrorf(e.Args[0].X.Pos(), "cannot convert value of type '%s' to expected argument type 'KeyPath<%s, _>'", kt, base)
		}
		return types.Typ[types.Invalid], true
	}
	if !types.Identical(gi.Args[0], base) {
		c.typeErrorf(e.Args[0].X.Pos(), "key path with root type '%s' cannot be applied to a base of type '%s'", gi.Args[0], base)
	}
	c.info.KeyPathReads[e] = kt
	return gi.Args[1], true
}

// builtinGetOnly reports whether a property of a built-in type -- a
// String's count, an Array's first -- is one only read: the runtime's
// properties are, and one an extension declares is unless it has a setter.
func (c *checker) builtinGetOnly(t types.Type, name string) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Kind() != types.String {
			return false
		}
	case *types.Array, *types.Dictionary, *types.Set:
	default:
		return false
	}
	if b := c.builtinOf(t); b != nil {
		for _, f := range b.Computed {
			if f != nil && f.Name == name {
				return !f.HasSetter
			}
		}
	}
	return true
}

// wildcardsTake is the type a tuple of destinations takes, each `_` in it
// the type of what is assigned there.
func wildcardsTake(x ast.Expr, lhs, rhs types.Type) types.Type {
	switch x := unparen(x).(type) {
	case *ast.WildcardExpr:
		return rhs
	case *ast.TupleExpr:
		lt, ok1 := lhs.(*types.Tuple)
		rt, ok2 := rhs.(*types.Tuple)
		if !ok1 || !ok2 || len(lt.Elements) != len(x.Elems) || len(rt.Elements) != len(x.Elems) {
			return lhs
		}
		elems := make([]*types.TupleElement, len(x.Elems))
		for i, el := range x.Elems {
			elems[i] = &types.TupleElement{Name: lt.Elements[i].Name,
				Type: wildcardsTake(el.X, lt.Elements[i].Type, rt.Elements[i].Type)}
		}
		return &types.Tuple{Elements: elems}
	}
	return lhs
}

// takesArrayLiteral reports whether an array literal can be a t: an array,
// or a type that is ExpressibleByArrayLiteral.
func takesArrayLiteral(t types.Type) bool {
	if isArrayType(t) {
		return true
	}
	if t == nil || isInvalid(t) {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Struct, *types.Class, *types.Enum:
		return types.ConformsToNamed(t, "ExpressibleByArrayLiteral")
	}
	return false
}

// requiredSubscripts is the subscripts the protocols t is constrained to
// require, t standing for their Self: t a type parameter, an associated
// type, or a protocol's Self.
func requiredSubscripts(t types.Type, static bool) []*SubscriptRef {
	var cons []types.Type
	switch tt := t.(type) {
	case *types.TypeParam:
		cons = tt.Constraints
	case *types.Dependent:
		cons = associatedConstraints(tt)
	default:
		return nil
	}
	var out []*SubscriptRef
	for _, con := range cons {
		p, ok := con.Underlying().(*types.Protocol)
		if !ok {
			continue
		}
		for _, up := range allProtocols([]*types.Protocol{p}) {
			for _, sub := range up.Subscripts {
				if sub == nil || sub.IsStatic != static {
					continue
				}
				sig, _ := throughParam(up, t, &types.Signature{Params: sub.Params, Results: sub.Result}).(*types.Signature)
				if sig == nil {
					continue
				}
				copied := *sub
				copied.Params, copied.Result = sig.Params, sig.Results
				out = append(out, &SubscriptRef{Recv: up, Subscript: &copied})
			}
		}
	}
	return out
}

// inferFromSuperclass is the instance of generic class cl whose ancestor
// is want -- Named<Int> for want Container<Int>, where Named<T>:
// Container<T> -- or cl itself where no ancestor is an instance of want's
// class or the ancestor leaves a parameter open.
func inferFromSuperclass(cl types.Type, want *types.GenericInstance) types.Type {
	params := typeParamsOf(cl)
	c, ok := cl.Underlying().(*types.Class)
	if !ok || len(params) == 0 {
		return cl
	}
	for sup := c.Superclass; sup != nil; {
		if gi, ok := sup.(*types.GenericInstance); ok && types.Identical(gi.Base, want.Base) {
			subst := map[*types.TypeParam]types.Type{}
			if !types.Unify(gi, want, subst) {
				return cl
			}
			args := make([]types.Type, len(params))
			for i, p := range params {
				if args[i] = subst[p]; args[i] == nil {
					return cl
				}
			}
			return &types.GenericInstance{Base: cl, Args: args}
		}
		up, ok := sup.Underlying().(*types.Class)
		if !ok {
			break
		}
		sup = up.Superclass
	}
	return cl
}

// isWord reports whether t is Int or UInt, what a pointer's bit pattern is.
func isWord(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && (b.Kind() == types.Int || b.Kind() == types.UInt)
}

// collectionEquals records `a == b` on two arrays or two dictionaries
// whose elements the runtime does not compare as the call of core's
// Swift that does, through each element's own ==: Array's _arrayEquals,
// Dictionary's _dictionaryEquals.
func (c *checker) collectionEquals(e *ast.BinaryExpr, t types.Type, scope *Scope) {
	name := ""
	switch u := t.Underlying().(type) {
	case *types.Array:
		if _, rt := core.ArrayEqual(u); rt {
			return
		}
		name = "_arrayEquals"
	case *types.Dictionary:
		name = "_dictionaryEquals"
	default:
		return
	}
	span := ast.Span{Lo: e.Pos(), Hi: e.End()}
	mem := &ast.MemberExpr{Span: ast.Span{Lo: e.X.Pos(), Hi: e.X.End()}, X: e.X, Dot: e.X.End(),
		Name: &ast.Ident{Span: ast.Span{Lo: e.Op.Pos(), Hi: e.Op.End()}, Synth: name}}
	call := &ast.CallExpr{Span: span, Fun: mem, Args: &ast.CallArgs{Span: span,
		Args: []*ast.CallArg{{Span: ast.Span{Lo: e.Y.Pos(), Hi: e.Y.End()}, X: e.Y}}}}
	if got := c.checkExpr(call, types.Typ[types.Bool], scope); isInvalid(got) {
		return
	}
	c.info.CollectionEquals[e] = call
}
