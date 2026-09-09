package analyzer

import (
	"fmt"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// checkExpr checks expression e against an optional expected type and
// returns its semantic type.
//
// It never returns nil. An expression this checker cannot yet read is
// Invalid, which every caller may go on to ask questions of; a nil
// would have to be tested for at each of them, and one missed test is
// a crash on a program that is not even wrong.
func (c *checker) checkExpr(expr ast.Expr, expected types.Type, scope *Scope) types.Type {
	if expr == nil {
		return types.Typ[types.Invalid]
	}

	typ := c.evalExpr(expr, expected, scope)
	if typ == nil {
		typ = types.Typ[types.Invalid]
	}
	c.info.Types[expr] = typ
	return typ
}

// reconcileLiterals settles the type of a numeric literal against
// what it is combined with. A literal has no type of its own in
// Swift: `celsius * 9` is Double arithmetic and `9 * celsius` is the
// same arithmetic written the other way round, because in both the
// literal is what gives way.
func (c *checker) reconcileLiterals(x ast.Expr, lhs types.Type, y ast.Expr, rhs types.Type, scope *Scope) (types.Type, types.Type) {
	if types.Identical(lhs, rhs) {
		return lhs, rhs
	}
	if t, ok := c.adopt(x, rhs); ok {
		return t, rhs
	}
	if t, ok := c.adopt(y, lhs); ok {
		return lhs, t
	}
	// `n != 1 + 2` is the same question as `n != 3`, and was not
	// getting the same answer: a single literal took its type from the
	// other side and a sum of two did not, so the comparison became
	// Int32 against Int. A tree made only of literals and the
	// operators that keep their operands' type is one literal as far
	// as this is concerned.
	if t, ok := c.adoptTree(x, rhs, scope); ok {
		return t, rhs
	}
	if t, ok := c.adoptTree(y, lhs, scope); ok {
		return lhs, t
	}
	return lhs, rhs
}

// adoptTree re-reads an expression made only of literals as the type
// it is being combined with.
//
// Re-checking rather than rewriting: the expression's parts each need
// the new type recorded against them, its operators need resolving
// against the new type, and checkExpr is what does both.
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

// isLiteralTree reports whether an expression is made of numeric
// literals and nothing else -- so that it has no type of its own to
// insist on, and takes whatever the context has.
func (c *checker) isLiteralTree(e ast.Expr) bool {
	switch n := e.(type) {
	case *ast.BasicLit:
		return n.Kind == token.INT_LIT || n.Kind == token.FLOAT_LIT
	case *ast.ParenExpr:
		return c.isLiteralTree(n.X)
	case *ast.PrefixExpr:
		return c.isLiteralTree(n.X)
	case *ast.BinaryExpr:
		if n.Op == nil {
			return false
		}
		op := string(c.file.Slice(n.Op.Lo, n.Op.Hi))
		return sharesOperandType(op) && c.isLiteralTree(n.X) && c.isLiteralTree(n.Y)
	}
	return false
}

// adopt re-reads a numeric literal as the type it is being combined
// with, and records that reading. It reports whether the literal took
// the type.
func (c *checker) adopt(e ast.Expr, want types.Type) (types.Type, bool) {
	if want == nil {
		return nil, false
	}
	// `-5` is a negative literal rather than a negation applied to
	// one -- checkPrefix reads the sign and the magnitude together --
	// so it is as adoptable as `5` is. Looking only at a bare literal
	// made `someInt32 != -5` an error against a type no annotation
	// could fix, where swiftc takes it.
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
	// Only a concrete numeric type is adopted. An optional is not:
	// `maybe() ?? 0` gives 0 the wrapped type, not the optional one,
	// and that is the coalescing rule's business rather than this.
	b, ok := want.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsNumeric == 0 || !types.AssignableTo(untyped, want) {
		return nil, false
	}
	// Both nodes: the expression, because that is what the operator
	// around it reads, and the literal, because that is what carries
	// the value down to lowering.
	c.info.Types[e] = want
	c.info.Types[lit] = want
	return want, true
}

// literalUnder is the numeric literal an expression is, through a
// leading sign.
func literalUnder(e ast.Expr) (*ast.BasicLit, bool) {
	if p, ok := e.(*ast.PrefixExpr); ok && p.Op != nil {
		if inner, ok := p.X.(*ast.BasicLit); ok {
			e = inner
		}
	}
	lit, ok := e.(*ast.BasicLit)
	return lit, ok
}

// checkPrefix reads a PrefixExpression. Swift declares `-`, `+` and
// `!` on its own types, and those are the ones that can be read
// without a declaration in hand; every other spelling is an operator
// some program declared, and resolving it is overload resolution's
// business.
func (c *checker) checkPrefix(e *ast.PrefixExpr, expected types.Type, scope *Scope) types.Type {
	// A value written where a `T?` is wanted is a `T` that is
	// injected on the way in, so the context the operand sees is the
	// wrapped type. `-7` written there is a negated Int32 and not a
	// negation applied to an `Int32?`, which is what it was read as
	// and refused for.
	expected = unwrappedContext(expected)
	op := ""
	if e.Op != nil {
		op = string(c.file.Slice(e.Op.Pos(), e.Op.End()))
	}
	// `-1` is a negative literal, not a negation applied to one: the
	// magnitude and the sign are read together, so that the bound
	// checked against Int is the one Swift checks. `-128` as an Int8
	// is the case that shows it -- 128 does not fit and -128 does.
	if lit, ok := e.X.(*ast.BasicLit); ok && (op == "-" || op == "+") {
		c.negated[lit] = op == "-"
	}
	inner := c.checkExpr(e.X, expected, scope)
	// The signed value belongs to the expression, so that whatever
	// reads a constant finds one here rather than an operator applied
	// to a magnitude. This used to be recorded in c.negated and never
	// read again, which left `-128` as a negation of 128 all the way
	// down to lowering, where it was refused.
	c.foldSign(e, op)
	if sym := c.builtinOperator(scope, op, inner); sym != nil {
		c.info.Operators[e] = sym
		return sym.Signature().Results
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

// checkStmtExpr reads an if or a switch used as a value. Every branch
// must produce one, and they must agree: the type of the whole is the
// type of the first branch that has one.
func (c *checker) checkStmtExpr(e *ast.StmtExpr, expected types.Type, scope *Scope) types.Type {
	// What it branches on is checked whatever its branches produce.
	switch s := e.Stmt.(type) {
	case *ast.IfStmt:
		for cur := ast.Stmt(s); cur != nil; {
			ifStmt, ok := cur.(*ast.IfStmt)
			if !ok {
				break
			}
			for _, cond := range ifStmt.Conds {
				c.checkCondition(cond, scope)
			}
			cur = ifStmt.Else
		}
	case *ast.SwitchStmt:
		c.checkExpr(s.Subject, nil, scope)
	}

	var result types.Type
	for _, blk := range branchBlocks(e.Stmt) {
		t := c.checkBranchValue(blk, expected, scope)
		switch {
		case t == nil || isInvalid(t):
			// The branch said nothing; another may still say it.
		case result == nil:
			result = t
		case !types.Identical(result, t):
			c.typeErrorf(blk.Pos(), "branches have mismatching types '%s' and '%s'", result, t)
			return types.Typ[types.Invalid]
		}
	}
	if result == nil {
		c.checkStmt(e.Stmt, scope)
		return types.Typ[types.Invalid]
	}
	return result
}

// branchBlocks collects the blocks an if or a switch expression
// produces its value from.
func branchBlocks(s ast.Stmt) []*ast.CodeBlock {
	var out []*ast.CodeBlock
	for s != nil {
		switch n := s.(type) {
		case *ast.IfStmt:
			if n.Body != nil {
				out = append(out, n.Body)
			}
			s = n.Else
			continue
		case *ast.CodeBlock:
			out = append(out, n)
		}
		break
	}
	return out
}

// checkBranchValue checks a branch and returns the type of the value
// it produces: the expression its last statement is.
func (c *checker) checkBranchValue(blk *ast.CodeBlock, expected types.Type, scope *Scope) types.Type {
	blockScope := NewScope(scope, blk.Pos(), blk.End())
	c.info.Scopes[blk] = blockScope
	var last types.Type
	for i, st := range blk.Stmts {
		if es, ok := st.(*ast.ExprStmt); ok && i == len(blk.Stmts)-1 {
			last = c.checkExpr(es.X, expected, blockScope)
			continue
		}
		c.checkStmt(st, blockScope)
	}
	return last
}

// checkInterpolation checks the expressions inside `\( … )`. They
// are ordinary expressions in the scope the literal is written in.
func (c *checker) checkInterpolation(in *ast.Interpolation, scope *Scope) {
	if in.X != nil {
		c.checkExpr(in.X, nil, scope)
	}
	for _, arg := range in.Args {
		c.checkExpr(arg.X, nil, scope)
	}
}

// adopts reports whether an untyped literal takes the type that was
// wanted where it was written.
//
// It does where that type is one the literal could be: `let n: Int32 =
// 1` makes the 1 an Int32. It does not where the wanted type is an
// existential -- `print(1)` wants `Any`, and what goes in the box is
// an Int, not something whose type is `Any`. A literal that adopted
// `Any` would have no type to describe at run time, which is the one
// thing an existential needs of what is inside it.
func adopts(want, untyped types.Type) bool {
	if want == nil {
		return false
	}
	switch want.(type) {
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
			if adopts(expected, types.Typ[types.UntypedInt]) {
				return expected
			}
			return types.Typ[types.Int]
		case token.FLOAT_LIT:
			if adopts(expected, types.Typ[types.UntypedFloat]) {
				return expected
			}
			return types.Typ[types.Double]
		case token.TRUE, token.FALSE:
			return types.Typ[types.Bool]
		case token.NIL:
			if expected != nil {
				if _, ok := expected.(*types.Optional); ok {
					return expected
				}
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
			return expected
		}
		return types.Typ[types.String]

	// Parentheses group; they do not change what is inside them.
	case *ast.ParenExpr:
		return c.checkExpr(e.X, expected, scope)

	// `self` and `Self` inside a member: the type the member is
	// written in, and its metatype.
	case *ast.SelfExpr:
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

	// `X.self` is the value X names, which for a type is its
	// metatype; `T.Type` written as an expression is the same thing.
	case *ast.PostfixSelfExpr:
		return c.checkExpr(e.X, nil, scope)

	case *ast.TypeExpr:
		return &types.Metatype{Instance: c.resolveType(e.Type, scope)}

	// `&x` in an argument: the type is the operand's, and what the
	// ampersand says is about how it is passed.
	case *ast.InOutExpr:
		return c.checkExpr(e.X, expected, scope)

	// The prefix operators Swift declares on its numeric types. Any
	// other spelling is a declared operator, which needs a
	// declaration to resolve against.
	case *ast.PrefixExpr:
		return c.checkPrefix(e, expected, scope)

	// An if or a switch standing where a value goes: its type is the
	// type its branches agree on.
	case *ast.StmtExpr:
		return c.checkStmtExpr(e, expected, scope)

	case *ast.MagicLit:
		switch e.Kind {
		case token.POUND_FILE, token.POUND_FUNCTION, token.POUND_FILEPATH:
			return types.Typ[types.String]
		case token.POUND_LINE, token.POUND_COLUMN:
			return types.Typ[types.Int]
		default:
			return types.Typ[types.String]
		}

	case *ast.IdentExpr:
		name := e.Name.Text(c.file)
		sym := scope.Lookup(name)
		if sym == nil {
			// A builtin type's name: it is in no scope, and in
			// expression position it denotes its own metatype.
			if u := types.LookupUniverse(name); u != nil {
				return &types.Metatype{Instance: u}
			}
			c.errorf(e.Name.Pos(), "cannot find '%s' in scope", name)
			return types.Typ[types.Invalid]
		}
		if v, ok := sym.(*VarSymbol); ok {
			if !v.IsInitialized() {
				c.errorf(e.Name.Pos(), "'%s' used before being initialized", name)
			}
			if v.IsConsumed() {
				c.errorf(e.Name.Pos(), "'%s' used after consume", name)
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
				instance = &types.GenericInstance{Base: instance, Args: args}
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
		opName := string(c.file.Slice(e.Op.Lo, e.Op.Hi))
		if opName == "=" {
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
				sym := scope.Lookup(name)
				if sym != nil {
					c.info.Uses[id.Name] = sym
					lhs = sym.Type()
					if v, ok := sym.(*VarSymbol); ok {
						if v.IsConst() {
							if v.IsInitialized() {
								c.errorf(e.Op.Pos(), "cannot assign to value: '%s' is a 'let' constant", name)
							} else {
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
				lhs = c.checkExpr(mem, nil, scope)
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
			} else {
				lhs = c.checkExpr(e.X, nil, scope)
			}

			rhs := c.checkExpr(e.Y, lhs, scope)
			if t, ok := c.adopt(e.Y, lhs); ok {
				rhs = t
			}
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
			operandCtx = rangeElement(expected)
		}
		lhs := c.checkExpr(e.X, operandCtx, scope)
		rhs := c.checkExpr(e.Y, operandCtx, scope)
		lhs, rhs = c.reconcileLiterals(e.X, lhs, e.Y, rhs, scope)

		// An operator is a function, and core declares them. Where
		// one resolves, the call decides the type and the rules
		// below are not consulted — they are what answers for the
		// operators core does not declare.
		if sym := c.builtinOperator(scope, opName, lhs, rhs); sym != nil {
			c.info.Operators[e] = sym
			return sym.Signature().Results
		}
		// A literal operand has no type of its own to match a
		// declaration with. `2 <+> 3` defaulted both to Int, so an
		// operator declared over Int32 did not fit and the result
		// fell back to the left operand's type -- which is the wrong
		// answer whatever the operator returns, and silently so where
		// the two happen to agree. Read them again in the
		// parameters' types, which is what gives a literal its type
		// everywhere else.
		if sym := c.operatorAdoptingLiterals(scope, opName, e, lhs, rhs); sym != nil {
			c.info.Operators[e] = sym
			return sym.Signature().Results
		}

		switch opName {
		case "==", "!=", "<", "<=", ">", ">=":
			// `nil == o` is `o == nil` written the other way round.
			// Only the second was accepted: nil is not comparable to
			// anything on its own, and asking that of the left
			// operand rejected the order Swift takes.
			if (opName == "==" || opName == "!=") &&
				(isUntypedNil(lhs) && isOptionalType(rhs) ||
					isUntypedNil(rhs) && isOptionalType(lhs)) {
				return types.Typ[types.Bool]
			}
			if !types.Comparable(lhs) {
				c.typeErrorf(e.Op.Pos(), "type '%s' is not comparable", lhs)
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
			// Nil coalescing. Swift has two forms and this answers
			// both: `T? ?? T` is a T, and `T? ?? T?` is a T?.
			if opt, ok := lhs.(*types.Optional); ok {
				// The right operand is read again in the wrapped
				// type's context, so an untyped literal adopts it.
				// Without this, `a ?? 0` against an `int32?` left the
				// literal at its default of Int, failed the
				// assignability test below, and fell out of the case
				// still optional -- a wrong type rather than a
				// diagnostic.
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
						c.errorf(e.Op.Pos(), "cannot assign to value: '%s' is a 'let' constant", name)
					}
				}
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
			// As for `=` above: `n += 1` gives 1 whatever n is.
			if t, ok := c.adopt(e.Y, lhs); ok {
				rhs = t
			}
			if !types.AssignableTo(rhs, lhs) {
				c.typeErrorf(e.Op.Pos(), "cannot assign value of type '%s' to type '%s'", rhs, lhs)
			}
			return types.Typ[types.Void]

		// A range is its two bounds, which have to be one type: `0..<n`
		// is a Range<Int> because n is an Int, and reconcileLiterals
		// above is what gave the literal that type. Swift requires the
		// bound to be Comparable and there is no protocol machinery
		// here, so what is required instead is that the two agree.
		case "...", "..<":
			if !types.Identical(lhs, rhs) {
				if !isInvalid(lhs) && !isInvalid(rhs) {
					c.typeErrorf(e.Op.Pos(), "cannot form a range from '%s' to '%s'", lhs, rhs)
				}
				return types.Typ[types.Invalid]
			}
			return &types.Range{Element: lhs, Closed: opName == "..."}

		default:
			// Custom operator: if operands compatible return lhs
			return lhs
		}

	case *ast.ConditionalExpr:
		condT := c.checkExpr(e.Cond, types.Typ[types.Bool], scope)
		if !types.Identical(condT, types.Typ[types.Bool]) {
			c.typeErrorf(e.Cond.Pos(), "condition must be of type 'Bool', got '%s'", condT)
		}
		thenT := c.checkExpr(e.Then, expected, scope)
		elseT := c.checkExpr(e.Else, expected, scope)
		// The two arms need not have one type between them. What has
		// to hold is that both fit where the whole expression is
		// going: `let g: Greeter = c ? Polite() : Brief()` is two
		// structs and one protocol, and Swift takes it.
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
		if mem, ok := e.Fun.(*ast.MemberExpr); ok && !c.namesModule(mem.X, scope) {
			baseType := c.checkExpr(mem.X, nil, scope)
			if cl, ok := baseType.Underlying().(*types.Class); ok && cl.IsActor && c.currActor != cl && !c.inAwait {
				memberName := mem.Name.Text(c.file)
				c.errorf(e.Pos(), "actor-isolated method '%s' cannot be called synchronously without 'await'", memberName)
			}
		}

		// `.line(7)` written where a Shape is wanted: the callee is
		// the case, and only the context says which enum's. The
		// expected type of the call is the expected type of the case
		// it names, which is what resolves it.
		var calleeWant types.Type
		if _, ok := e.Fun.(*ast.ImplicitMemberExpr); ok {
			calleeWant = expected
		}
		calleeType := c.checkExpr(e.Fun, calleeWant, scope)
		var args []*ast.CallArg
		if e.Args != nil {
			args = e.Args.Args
		}
		if sig, ok := calleeType.Underlying().(*types.Signature); ok {
			if chosen := c.resolveOverload(e.Fun, args, scope); chosen != nil {
				sig = chosen
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
			inst := c.inferInstance(meta.Instance, e, scope)
			if st, ok := inst.Underlying().(*types.Struct); ok {
				if sig := st.Memberwise(); sig != nil {
					c.checkCallArguments(e, sig, args, scope)
					return inst
				}
				// A type that declares its own initializers is
				// checked against the one the call names. Not doing
				// so left the arguments with no context at all, so a
				// literal took its default type -- `P(v: 14)` passed
				// an Int to a parameter declared Int32, and the
				// mistake surfaced far away, in lowering, as a
				// builtin whose operand was the wrong width.
				if sig := c.pickInitializer(st.Inits, args); sig != nil {
					c.checkCallArguments(e, sig, args, scope)
					return inst
				}
				// Nothing fitted. Where exactly one initializer takes
				// this many arguments, the call is checked against it
				// anyway: the mistake is then reported as what it is
				// -- a wrong label, a wrong type -- rather than as
				// the absence of a match, which says nothing about
				// how to fix it.
				if sig := c.soleInitializerOfArity(st.Inits, len(args)); sig != nil {
					c.checkCallArguments(e, sig, args, scope)
					return inst
				}
				if len(st.Inits) > 0 {
					c.typeErrorf(e.Pos(),
						"no initializer of '%s' takes these arguments", inst)
				}
			}
			// A class is made by an initializer it declares -- there
			// is no memberwise one, Swift gives that to structs
			// alone -- so the call is checked against the one it
			// names. Not doing so left every argument with no
			// context: `Counter(n: 41)` passed an Int where the
			// initializer takes an Int32, and the mistake surfaced in
			// lowering as an argument whose width did not match the
			// signature it was going to.
			if cl, ok := inst.Underlying().(*types.Class); ok && len(cl.Inits) > 0 {
				if sig := c.pickInitializer(cl.Inits, args); sig != nil {
					c.checkCallArguments(e, sig, args, scope)
					return inst
				}
				if sig := c.soleInitializerOfArity(cl.Inits, len(args)); sig != nil {
					c.checkCallArguments(e, sig, args, scope)
					return inst
				}
				c.typeErrorf(e.Pos(), "no initializer of '%s' takes these arguments", inst)
				return inst
			}
			// Not checked against anything, but the arguments are
			// still expressions and still have to be looked at: a
			// mistake inside one is a mistake wherever it appears.
			for _, arg := range args {
				c.checkExpr(arg.X, nil, scope)
			}
			return inst
		}
		// Whatever is being called, the arguments are expressions.
		for _, arg := range args {
			c.checkExpr(arg.X, nil, scope)
		}
		c.typeErrorf(e.Pos(), "cannot call value of non-function type '%s'", calleeType)
		return types.Typ[types.Invalid]

	// `.red` where a Color is wanted. There is no base to check: the
	// context supplies it, which is the whole of what makes the syntax
	// shorter than `Color.red`. Swift resolves any static member this
	// way; only an enum case is resolved here, because an enum case is
	// the only static member this checker has.
	case *ast.ImplicitMemberExpr:
		if e.Name == nil {
			return types.Typ[types.Invalid]
		}
		if expected == nil {
			c.typeErrorf(e.Dot, "reference to member '%s' cannot be resolved without a contextual type", e.Name.Text(c.file))
			return types.Typ[types.Invalid]
		}
		name := e.Name.Text(c.file)
		sym := c.enumCaseSymbol(expected, name)
		if sym == nil {
			if !isInvalid(expected) {
				c.typeErrorf(e.Name.Pos(), "type '%s' has no member '%s'", expected, name)
			}
			return types.Typ[types.Invalid]
		}
		// Recorded under the name, which is where gen reads it: an
		// implicit member and a written-out one name the same case and
		// lower the same way.
		c.info.Uses[e.Name] = sym
		c.checkAccess(e.Name.Pos(), e.Name.Text(c.file), sym)
		// A case that carries something, named the short way, is
		// called: `.line(7)` is the case and 7 is what it carries.
		// What the call is checked against is the case's own
		// signature, and the result is the enum either way.
		if sym.AssociatedType() != nil {
			return &types.Signature{
				Params:  caseParams(sym.AssociatedType()),
				Results: expected,
			}
		}
		return expected

	case *ast.MemberExpr:
		// `Chrono.epochYearUTC` is a name in a module rather than a
		// member of a value, and the two are written the same way.
		if t, ok := c.moduleMemberValue(e, scope); ok {
			return t
		}
		baseType := c.checkExpr(e.X, nil, scope)
		memberName := e.Name.Text(c.file)

		// `p?.x` reaches through the optional: the member is looked
		// up on what p holds. Two layers come off -- the one `p?`
		// added, since a `?` in isolation is an optional of whatever
		// it followed, and p's own -- and without that the lookup ran
		// on a doubly optional type, found nothing, and answered
		// Invalid. Which reported nothing, so the chain was
		// assignable to anything and `??` saw a left side that was
		// not an optional at all.
		_, chained := e.X.(*ast.OptionalExpr)
		if chained {
			for i := 0; i < 2; i++ {
				o, isOpt := baseType.(*types.Optional)
				if !isOpt {
					break
				}
				baseType = o.Wrapped
			}
		}

		if cl, ok := baseType.Underlying().(*types.Class); ok && cl.IsActor && c.currActor != cl && !c.inAwait {
			for _, f := range cl.Fields {
				if f.Name == memberName {
					c.errorf(e.Name.Pos(), "actor-isolated property '%s' cannot be referenced synchronously without 'await'", memberName)
				}
			}
		}

		if t := c.lookupMemberFor(e, baseType, memberName); t != nil {
			// `p?.x` is a chain: it is the member where p holds
			// something and nothing where it does not, so it is an
			// optional whatever the member is. Losing that made the
			// chain assignable to a non-optional -- which Swift
			// refuses -- and left `??` with a left side that was not
			// an optional at all.
			//
			// Flattened rather than nested: `p?.x` where x is itself
			// optional is one optional, which is what Swift answers.
			if chained {
				if _, already := t.(*types.Optional); !already {
					return &types.Optional{Wrapped: t}
				}
			}
			return t
		}
		// A type declared in this compilation has a member list, so a
		// name that is not in it is a mistake and is reported. A type
		// whose members are not modelled — a builtin, an array, a
		// type parameter — is not evidence of anything, and silence
		// is the honest answer for it.
		if !isInvalid(baseType) && c.membersKnown(baseType) {
			c.typeErrorf(e.Name.Pos(), "value of type '%s' has no member '%s'", baseType, memberName)
		}
		return types.Typ[types.Invalid]

	case *ast.SubscriptExpr:
		baseType := c.checkExpr(e.X, nil, scope)
		var index types.Type
		var result types.Type = types.Typ[types.Invalid]
		switch b := baseType.Underlying().(type) {
		case *types.Array:
			index, result = types.Typ[types.Int], b.Elem
		case *types.Dictionary:
			index, result = b.Key, &types.Optional{Wrapped: b.Value}
		}
		// The arguments are read whatever the base turns out to be:
		// a user-declared subscript is not modelled yet, and its
		// arguments are expressions all the same.
		for i, arg := range e.Args {
			want := index
			if i > 0 {
				want = nil
			}
			c.checkExpr(arg.X, want, scope)
		}
		return result

	case *ast.ArrayLit:
		var elemType types.Type
		if arrT, ok := expected.(*types.Array); ok {
			elemType = arrT.Elem
		}
		for _, el := range e.Items {
			et := c.checkExpr(el, elemType, scope)
			if elemType == nil {
				elemType = et
			}
		}
		// An empty literal with nothing to infer from says nothing
		// about its element type, and neither does this.
		if elemType == nil {
			elemType = types.Typ[types.Invalid]
		}
		return &types.Array{Elem: elemType}

	case *ast.DictLit:
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
		// The context reaches the elements. `(20, 22)` written where
		// a `(Int32, Int32)` is wanted is two Int32s: without that,
		// each literal takes its own default type and the tuple is a
		// different type from the one it was written for.
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
			}
			t := c.checkExpr(el.X, elemWant, scope)
			elems[i] = &types.TupleElement{Name: label, Type: t}
		}
		return &types.Tuple{Elements: elems}

	case *ast.CastExpr:
		valT := c.checkExpr(e.X, nil, scope)
		targetT := c.resolveType(e.Type, scope)
		if e.Kind == token.IS {
			return types.Typ[types.Bool]
		}
		if e.Question != token.NoPos {
			return &types.Optional{Wrapped: targetT}
		}
		_ = valT
		return targetT

	case *ast.TryExpr:
		// `try?` turns a call that may fail into an optional: the
		// value where it did not, nothing where it did. `try` and
		// `try!` are the call's own type -- one propagates the
		// failure and the other insists there will not be one.
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
		return &types.Optional{Wrapped: inner}

	case *ast.ClosureExpr:
		closureScope := NewScope(scope, e.Pos(), e.End())
		c.info.Scopes[e] = closureScope

		var expSig *types.Signature
		if expected != nil {
			if s, ok := expected.Underlying().(*types.Signature); ok {
				expSig = s
			}
		}

		var params []*types.Param
		if e.Sig != nil && e.Sig.Params != nil {
			for i, p := range e.Sig.Params.Params {
				name := p.Name.Text(c.file)
				var paramType types.Type
				if p.Type != nil {
					paramType = c.resolveType(p.Type, scope)
				} else if expSig != nil && i < len(expSig.Params) {
					paramType = expSig.Params[i].Type
				}
				// A closure parameter with no annotation takes its
				// type from the context the closure appears in. Where
				// there is none, there is nothing to say.
				if paramType == nil {
					paramType = types.Typ[types.Invalid]
				}
				params = append(params, &types.Param{Name: name, Type: paramType})
				v := NewVar(name, paramType, p.Name.Pos(), true, types.DefaultOwnership)
				closureScope.Insert(v)
				c.info.Defs[p.Name] = v
			}
		} else if expSig != nil {
			for i, p := range expSig.Params {
				shorthandName := fmt.Sprintf("$%d", i)
				v := NewVar(shorthandName, p.Type, e.Pos(), true, types.DefaultOwnership)
				closureScope.Insert(v)
				params = append(params, &types.Param{Name: shorthandName, Type: p.Type})
			}
		}

		var retType types.Type = types.Typ[types.Void]
		if expSig != nil && expSig.Results != nil {
			retType = expSig.Results
		}
		if e.Sig != nil && e.Sig.Result != nil {
			retType = c.resolveType(e.Sig.Result.Type, scope)
		}

		prevRet := c.currFuncRet
		c.currFuncRet = retType
		defer func() { c.currFuncRet = prevRet }()

		if len(e.Stmts) == 1 {
			if exprStmt, ok := e.Stmts[0].(*ast.ExprStmt); ok {
				inferredRet := c.checkExpr(exprStmt.X, retType, closureScope)
				if retType == nil || types.Identical(retType, types.Typ[types.Void]) {
					retType = inferredRet
				} else if !types.AssignableTo(inferredRet, retType) {
					c.typeErrorf(exprStmt.X.Pos(), "cannot convert return value of type '%s' to expected return type '%s'", inferredRet, retType)
				}
			} else {
				c.checkStmt(e.Stmts[0], closureScope)
			}
		} else {
			for _, s := range e.Stmts {
				c.checkStmt(s, closureScope)
			}
		}

		return &types.Signature{Params: params, Results: retType}

	default:
		return types.Typ[types.Invalid]
	}
}

// foldSign records a signed literal's value on the expression that
// spells it.
//
// The magnitude is held as a uint64 and the negation is done in that
// width on purpose: Int.min's magnitude is 2^63, which has no positive
// int64 to negate, and two's complement gives the right bits for it
// and for everything smaller by the same arithmetic.
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

// sharesOperandType reports whether an operator's result is the same
// type as the values it was applied to, which is what makes it
// transparent to an annotation.
// rangeElement is the type a range's bounds are asked for, given the
// type the range itself was asked for.
func rangeElement(expected types.Type) types.Type {
	if r, ok := expected.(*types.Range); ok {
		return r.Element
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

// pickInitializer is the initializer a call's arguments name.
//
// The arity and the labels, which is what tells two initializers of
// one type apart -- the same question resolveOverload asks about two
// functions of one name, and answered the same way. The argument
// types are not part of it yet: they have no context until a
// candidate is chosen, which is the whole reason this exists.
func (c *checker) pickInitializer(inits []*types.Signature, args []*ast.CallArg) *types.Signature {
	var found *types.Signature
	for _, sig := range inits {
		if sig == nil || len(sig.Params) != len(args) {
			continue
		}
		fits := true
		for i, param := range sig.Params {
			if !c.labelFits(args[i], param) {
				fits = false
				break
			}
		}
		if !fits {
			continue
		}
		if found != nil {
			// Two that fit is not an answer. The call is reported
			// against neither rather than against whichever came
			// first.
			return nil
		}
		found = sig
	}
	return found
}

// soleInitializerOfArity is the one initializer taking n arguments,
// where there is exactly one.
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

// namesModule reports whether an expression is a bare module name.
//
// A value in scope under that name wins. Nothing declares a module,
// so its name is only what nothing else has taken -- which is also
// what keeps this from changing the meaning of a program that has a
// variable called Chrono in it.
func (c *checker) namesModule(e ast.Expr, scope *Scope) bool {
	id, ok := e.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return false
	}
	name := id.Name.Text(c.file)
	return c.modules[name] != nil && scope.Lookup(name) == nil
}

// moduleMemberValue resolves `Module.name` in expression position and
// reports whether it was one.
//
// An interface writes every name qualified -- `Swift.min`, and the
// module's own names through the module -- so this is the other half
// of what reading one needs; resolveType is the half about types. The
// symbol is recorded under the member's own identifier, which is
// where every reader of Uses looks, so a call through a module name
// reaches the same symbol a bare call would.
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
	// A type named through its module is still a type: `Swift.Int32(n)`
	// is the conversion `Int32(n)`, and both reach it as a metatype.
	if _, ok := sym.(*TypeNameSymbol); ok {
		return &types.Metatype{Instance: sym.Type()}, true
	}
	return sym.Type(), true
}

// isUntypedNil reports whether a type is the literal nil's, which has
// no type of its own until something gives it one.
func isUntypedNil(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.UntypedNil
}

// unwrappedContext is the type an operand is checked against where an
// optional is wanted: what a `T?` wraps, because a value written
// there is a `T` and the injection happens above the operator.
//
// One level, which is all the injection does.
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
