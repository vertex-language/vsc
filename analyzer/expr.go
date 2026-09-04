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

func (c *checker) evalExpr(expr ast.Expr, expected types.Type, scope *Scope) types.Type {
	switch e := expr.(type) {
	case *ast.BasicLit:
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
			return types.Typ[types.Int]
		}

	case *ast.StringLit:
		if expected != nil && types.AssignableTo(types.Typ[types.UntypedString], expected) {
			return expected
		}
		return types.Typ[types.String]

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
		if sig, ok := calleeType.(*types.Signature); ok {
			var args []*ast.CallArg
			if e.Args != nil {
				args = e.Args.Args
			}
			c.checkCallArguments(e, sig, args, scope)
			return sig.Results
		}
		// Struct initializer e.g. Point(x: 1, y: 2)
		if meta, ok := calleeType.(*types.Metatype); ok {
			return meta.Instance
		}
		if _, ok := calleeType.(*types.Struct); ok {
			return calleeType
		}
		if _, ok := calleeType.(*types.Class); ok {
			return calleeType
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

		// Look up member in base
		if st, ok := baseType.Underlying().(*types.Struct); ok {
			for _, f := range st.Fields {
				if f.Name == memberName {
					return f.Type
				}
			}
			for _, m := range st.Methods {
				if m.Name == memberName {
					return m.Sig
				}
			}
		}
		if cl, ok := baseType.Underlying().(*types.Class); ok {
			for _, f := range cl.Fields {
				if f.Name == memberName {
					return f.Type
				}
			}
			for _, m := range cl.Methods {
				if m.Name == memberName {
					return m.Sig
				}
			}
		}
		if en, ok := baseType.Underlying().(*types.Enum); ok {
			for _, cs := range en.Cases {
				if cs.Name == memberName {
					if cs.AssociatedType != nil {
						return &types.Signature{
							Params:  []*types.Param{{Type: cs.AssociatedType}},
							Results: en,
						}
					}
					return en
				}
			}
			for _, m := range en.Methods {
				if m.Name == memberName {
					return m.Sig
				}
			}
		}
		// Method or property fallback
		return types.Typ[types.Int]

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
		if elemType == nil {
			elemType = types.Typ[types.Int]
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
			valType = types.Typ[types.Int]
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
				if paramType == nil {
					paramType = types.Typ[types.Int]
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

func (c *checker) checkCallArguments(call *ast.CallExpr, sig *types.Signature, args []*ast.CallArg, scope *Scope) {
	if sig.Params == nil {
		return
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
		return
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
				c.errorf(arg.Pos(), "incorrect argument label: expected %q, got %q", param.Label, actualLabel)
			}
		}

		argType := c.checkExpr(arg.X, param.Type, scope)
		if !types.AssignableTo(argType, param.Type) {
			c.typeErrorf(arg.Pos(), "cannot convert value of type '%s' to expected argument type '%s'", argType, param.Type)
		}
	}
}
