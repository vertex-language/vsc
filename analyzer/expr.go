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

// resolveOverload picks the declaration a call names, where the name
// has more than one. It returns nil where there is nothing to choose
// — one declaration, or no single fit — and the caller goes on with
// what it had.
//
// The fit is the arguments: how many, and whether each is assignable
// to the parameter it lands on. That is enough for the calls a name
// is usually overloaded for, and short of what Swift does, which
// ranks the candidates rather than requiring exactly one to fit.
func (c *checker) resolveOverload(fun ast.Expr, args []*ast.CallArg, scope *Scope) *types.Signature {
	id, ok := fun.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return nil
	}
	sym, ok := c.info.Uses[id.Name].(*FuncSymbol)
	if !ok {
		return nil
	}
	candidates := sym.Overloads()
	if len(candidates) < 2 {
		return nil
	}

	quiet := len(c.info.Diagnostics)
	argTypes := make([]types.Type, len(args))
	for i, arg := range args {
		argTypes[i] = c.checkExpr(arg.X, nil, scope)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]

	var fits []*types.Signature
	for _, cand := range candidates {
		sig := cand.Signature()
		if sig == nil || len(sig.Params) != len(argTypes) {
			continue
		}
		ok := true
		for i, t := range argTypes {
			if !c.labelFits(args[i], sig.Params[i]) || !types.AssignableTo(t, sig.Params[i].Type) {
				ok = false
				break
			}
		}
		if ok {
			fits = append(fits, sig)
		}
	}
	if len(fits) == 1 {
		return fits[0]
	}
	return nil
}

// labelFits reports whether an argument's label is the one a
// parameter asks for. Two declarations of a name may differ in
// nothing else — `label(a:)` and `label(b:)` — so a call is not
// resolved without reading them.
func (c *checker) labelFits(arg *ast.CallArg, param *types.Param) bool {
	want := param.Label
	if want == "" {
		want = param.Name
	}
	if arg.Label == nil {
		return want == "" || want == "_"
	}
	return arg.Label.Text(c.file) == want
}

// inferInstance is the type an initializer call produces. For a
// generic type it is the instance the arguments imply: `Box(v: 3)`
// makes a Box<Int>, which the memberwise initializer's parameters —
// the stored properties, in order — are enough to work out.
func (c *checker) inferInstance(instance types.Type, call *ast.CallExpr, scope *Scope) types.Type {
	params := typeParamsOf(instance)
	if len(params) == 0 || call.Args == nil {
		return instance
	}
	fields := storedFieldsOf(instance)
	if len(fields) == 0 {
		return instance
	}

	subst := make(map[*types.TypeParam]types.Type, len(params))
	quiet := len(c.info.Diagnostics)
	for i, arg := range call.Args.Args {
		field := fieldFor(fields, arg, c.file, i)
		if field == nil {
			continue
		}
		types.Unify(field.Type, c.checkExpr(arg.X, nil, scope), subst)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]

	args := make([]types.Type, len(params))
	for i, p := range params {
		bound, ok := subst[p]
		if !ok {
			return instance // not every parameter was said; say nothing
		}
		args[i] = bound
	}
	return &types.GenericInstance{Base: instance, Args: args}
}

// fieldFor matches one argument of an initializer call to the stored
// property it initializes: by label where the call gives one, and by
// position otherwise.
func fieldFor(fields []*types.Field, arg *ast.CallArg, f *token.File, i int) *types.Field {
	if arg.Label != nil {
		name := arg.Label.Text(f)
		for _, field := range fields {
			if field.Name == name {
				return field
			}
		}
		return nil
	}
	if i < len(fields) {
		return fields[i]
	}
	return nil
}

// typeParamsOf is a nominal type's generic parameters.
func typeParamsOf(t types.Type) []*types.TypeParam {
	switch n := t.(type) {
	case *types.Struct:
		return n.TypeParams
	case *types.Class:
		return n.TypeParams
	case *types.Enum:
		return n.TypeParams
	}
	return nil
}

// storedFieldsOf is a nominal type's stored properties, which are the
// parameters of the initializer the compiler writes for it.
func storedFieldsOf(t types.Type) []*types.Field {
	switch n := t.(type) {
	case *types.Struct:
		return n.Fields
	case *types.Class:
		return n.Fields
	}
	return nil
}

// lookupMember finds a member of t by name, or returns nil.
//
// A metatype is looked through: `E.a` names a case of E, and the type
// this compiler builds for `E` in expression position is E's
// metatype. Static and instance members are not yet told apart, which
// is an over-acceptance this will lose when they are.
func (c *checker) lookupMember(t types.Type, name string) types.Type {
	if t == nil {
		return nil
	}
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	// A member of `Box<Int>` is the member of Box with T standing for
	// Int: the instance's arguments are what the declaration's
	// parameters meant all along.
	if inst, ok := t.(*types.GenericInstance); ok {
		member := c.lookupMember(inst.Base, name)
		if member == nil {
			return nil
		}
		params := typeParamsOf(inst.Base)
		subst := make(map[*types.TypeParam]types.Type, len(params))
		for i, p := range params {
			if i < len(inst.Args) {
				subst[p] = inst.Args[i]
			}
		}
		return types.Substitute(member, subst)
	}
	switch b := t.Underlying().(type) {
	case *types.Struct:
		for _, f := range b.Fields {
			if f.Name == name {
				return known(f.Type)
			}
		}
		for _, m := range b.Methods {
			if m.Name == name {
				return m.Sig
			}
		}
	case *types.Class:
		for _, f := range b.Fields {
			if f.Name == name {
				return known(f.Type)
			}
		}
		for _, m := range b.Methods {
			if m.Name == name {
				return m.Sig
			}
		}
		if b.Superclass != nil {
			return c.lookupMember(b.Superclass, name)
		}
	case *types.Enum:
		for _, cs := range b.Cases {
			if cs.Name == name {
				if cs.AssociatedType != nil {
					return &types.Signature{
						Params:  []*types.Param{{Type: cs.AssociatedType}},
						Results: b,
					}
				}
				return b
			}
		}
		for _, m := range b.Methods {
			if m.Name == name {
				return m.Sig
			}
		}
	}
	return nil
}

// known turns a member whose type did not resolve into Invalid. The
// member is there either way, and saying so is what keeps a field
// this compiler cannot type yet from being reported as missing.
func known(t types.Type) types.Type {
	if t == nil {
		return types.Typ[types.Invalid]
	}
	return t
}

// membersKnown reports whether this compiler knows the whole member
// list of t. It does for a type declared in the source it is reading,
// and does not for anything it models without members — every builtin
// among them, until there is a library to read them from.
func (c *checker) membersKnown(t types.Type) bool {
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	switch t.Underlying().(type) {
	case *types.Struct, *types.Class, *types.Enum:
		return true
	}
	return false
}

// checkPrefix reads a PrefixExpression. Swift declares `-`, `+` and
// `!` on its own types, and those are the ones that can be read
// without a declaration in hand; every other spelling is an operator
// some program declared, and resolving it is overload resolution's
// business.
func (c *checker) checkPrefix(e *ast.PrefixExpr, expected types.Type, scope *Scope) types.Type {
	op := ""
	if e.Op != nil {
		op = string(c.file.Slice(e.Op.Pos(), e.Op.End()))
	}
	// `-1` is a negative literal, not a negation applied to one: the
	// magnitude and the sign are read together, so that the bound
	// checked against Int is the one Swift checks.
	if lit, ok := e.X.(*ast.BasicLit); ok && lit.Kind == token.INT_LIT && (op == "-" || op == "+") {
		c.negated[lit] = op == "-"
	}
	inner := c.checkExpr(e.X, expected, scope)
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

func (c *checker) evalExpr(expr ast.Expr, expected types.Type, scope *Scope) types.Type {
	switch e := expr.(type) {
	case *ast.BasicLit:
		c.valueOf(e)
		switch e.Kind {
		case token.INT_LIT:
			if expected != nil && types.AssignableTo(types.Typ[types.UntypedInt], expected) {
				return expected
			}
			return types.Typ[types.Int]
		case token.FLOAT_LIT:
			if expected != nil && types.AssignableTo(types.Typ[types.UntypedFloat], expected) {
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
		if expected != nil && types.AssignableTo(types.Typ[types.UntypedString], expected) {
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
			// Check if it's a universe type used as a metatype (e.g. Int in expression position)
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
			rhs := c.checkExpr(e.Y, nil, scope)
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

			if !types.AssignableTo(rhs, lhs) {
				c.typeErrorf(e.Op.Pos(), "cannot assign value of type '%s' to type '%s'", rhs, lhs)
			}
			return types.Typ[types.Void]
		}

		lhs := c.checkExpr(e.X, nil, scope)
		rhs := c.checkExpr(e.Y, nil, scope)

		switch opName {
		case "==", "!=", "<", "<=", ">", ">=":
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
			// Nil coalescing: T? ?? T -> T
			if opt, ok := lhs.(*types.Optional); ok {
				if types.AssignableTo(rhs, opt.Wrapped) {
					return opt.Wrapped
				}
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
			if !types.AssignableTo(rhs, lhs) {
				c.typeErrorf(e.Op.Pos(), "cannot assign value of type '%s' to type '%s'", rhs, lhs)
			}
			return types.Typ[types.Void]

		case "...", "..<":
			// Range operator
			return types.NewNamed("Range", "", lhs)

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
		elseT := c.checkExpr(e.Else, thenT, scope)
		if types.AssignableTo(elseT, thenT) {
			return thenT
		}
		if types.AssignableTo(thenT, elseT) {
			return elseT
		}
		c.typeErrorf(e.Colon, "result values in '? :' expression have mismatching types '%s' and '%s'", thenT, elseT)
		return thenT

	case *ast.CallExpr:
		if mem, ok := e.Fun.(*ast.MemberExpr); ok {
			baseType := c.checkExpr(mem.X, nil, scope)
			if cl, ok := baseType.Underlying().(*types.Class); ok && cl.IsActor && c.currActor != cl && !c.inAwait {
				memberName := mem.Name.Text(c.file)
				c.errorf(e.Pos(), "actor-isolated method '%s' cannot be called synchronously without 'await'", memberName)
			}
		}

		calleeType := c.checkExpr(e.Fun, nil, scope)
		if sig, ok := calleeType.Underlying().(*types.Signature); ok {
			var args []*ast.CallArg
			if e.Args != nil {
				args = e.Args.Args
			}
			if chosen := c.resolveOverload(e.Fun, args, scope); chosen != nil {
				sig = chosen
			}
			return c.checkCallArguments(e, sig, args, scope).Results
		}
		// An initializer call. Initializers are not modelled yet, so
		// the arguments are not checked against one — but where the
		// type is generic they are what says which type is being
		// made, and that much can be read off the stored properties.
		if meta, ok := calleeType.(*types.Metatype); ok {
			return c.inferInstance(meta.Instance, e, scope)
		}
		c.typeErrorf(e.Pos(), "cannot call value of non-function type '%s'", calleeType)
		return types.Typ[types.Invalid]

	case *ast.MemberExpr:
		baseType := c.checkExpr(e.X, nil, scope)
		memberName := e.Name.Text(c.file)

		if cl, ok := baseType.Underlying().(*types.Class); ok && cl.IsActor && c.currActor != cl && !c.inAwait {
			for _, f := range cl.Fields {
				if f.Name == memberName {
					c.errorf(e.Name.Pos(), "actor-isolated property '%s' cannot be referenced synchronously without 'await'", memberName)
				}
			}
		}

		if t := c.lookupMember(baseType, memberName); t != nil {
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
		if arr, ok := baseType.Underlying().(*types.Array); ok {
			if len(e.Args) > 0 {
				c.checkExpr(e.Args[0].X, types.Typ[types.Int], scope)
			}
			return arr.Elem
		}
		if dict, ok := baseType.Underlying().(*types.Dictionary); ok {
			if len(e.Args) > 0 {
				c.checkExpr(e.Args[0].X, dict.Key, scope)
			}
			return &types.Optional{Wrapped: dict.Value}
		}
		return types.Typ[types.Invalid]

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
		elems := make([]*types.TupleElement, len(e.Elems))
		for i, el := range e.Elems {
			var label string
			if el.Label != nil {
				label = el.Label.Text(c.file)
			}
			t := c.checkExpr(el.X, nil, scope)
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

// checkCallArguments checks a call's arguments against a signature
// and returns the signature the call was made with: the same one,
// unless it was generic, in which case it is the one its type
// parameters were inferred into.
func (c *checker) checkCallArguments(call *ast.CallExpr, sig *types.Signature, args []*ast.CallArg, scope *Scope) *types.Signature {
	sig = c.inferGenericCall(sig, args, scope)
	if sig.Params == nil {
		return sig
	}
	// Check argument count (if not variadic)
	minParams := len(sig.Params)
	isVariadic := false
	if minParams > 0 && sig.Params[minParams-1].Variadic {
		minParams--
		isVariadic = true
	}

	if (!isVariadic && len(args) != len(sig.Params)) || (isVariadic && len(args) < minParams) {
		c.errorf(call.Pos(), "incorrect argument count: expected %d, got %d", len(sig.Params), len(args))
		return sig
	}

	for i, arg := range args {
		var param *types.Param
		if i < len(sig.Params) {
			param = sig.Params[i]
		} else if isVariadic {
			param = sig.Params[len(sig.Params)-1]
		}
		if param == nil {
			break
		}

		// Check label matching
		if param.Label != "" && param.Label != "_" {
			if arg.Label == nil || arg.Label.Text(c.file) != param.Label {
				var actualLabel string
				if arg.Label != nil {
					actualLabel = arg.Label.Text(c.file)
				}
				c.errorf(arg.Pos(), "incorrect argument label (have '%s:', expected '%s:')", actualLabel, param.Label)
			}
		}

		argType := c.checkExpr(arg.X, param.Type, scope)
		if !types.AssignableTo(argType, param.Type) {
			c.typeErrorf(arg.Pos(), "cannot convert value of type '%s' to expected argument type '%s'", argType, param.Type)
		}
	}
	return sig
}

// inferGenericCall binds a generic signature's type parameters to the
// argument types and returns the signature that results. `identity(3)`
// is a call to `(Int) -> Int`, and that is the signature the rest of
// the check should see.
//
// An argument that binds nothing leaves its parameter as it was, and
// the call is checked against a type parameter it cannot satisfy —
// which is the honest outcome: this compiler has no constraint solver
// yet, and a call it cannot infer is a call it does not understand.
func (c *checker) inferGenericCall(sig *types.Signature, args []*ast.CallArg, scope *Scope) *types.Signature {
	if len(sig.TypeParams) == 0 || len(args) == 0 {
		return sig
	}
	// The arguments are read twice: once to infer, once to check
	// against what was inferred. Only the second reading reports —
	// a speculative pass that says nothing is the same rule the
	// parser follows when it tries a production and backs out.
	subst := make(map[*types.TypeParam]types.Type, len(sig.TypeParams))
	quiet := len(c.info.Diagnostics)
	for i, arg := range args {
		if i >= len(sig.Params) {
			break
		}
		types.Unify(sig.Params[i].Type, c.checkExpr(arg.X, nil, scope), subst)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	if len(subst) == 0 {
		return sig
	}
	if out, ok := types.Substitute(sig, subst).(*types.Signature); ok {
		return out
	}
	return sig
}
