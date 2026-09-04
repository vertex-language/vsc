package analyzer

import (
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// checkStmt type-checks a statement within scope.
func (c *checker) checkStmt(stmt ast.Stmt, scope *Scope) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.DeclStmt:
		c.checkDecl(s.D, scope)

	case *ast.ExprStmt:
		c.checkExpr(s.X, nil, scope)

	case *ast.ReturnStmt:
		var retType types.Type = types.Typ[types.Void]
		if s.X != nil {
			retType = c.checkExpr(s.X, c.currFuncRet, scope)
		}
		if c.currFuncRet != nil && !types.AssignableTo(retType, c.currFuncRet) {
			c.typeErrorf(s.Pos(), "cannot convert return value of type '%s' to expected return type '%s'", retType, c.currFuncRet)
		}

	// A name bound by a condition is visible in the body and nowhere
	// else, which is what lets `if let x = x { … }` shadow the
	// optional it unwrapped.
	case *ast.IfStmt:
		condScope := NewScope(scope, s.Pos(), s.End())
		for _, cond := range s.Conds {
			c.checkCondition(cond, condScope)
		}
		c.checkCodeBlock(s.Body, condScope)
		if s.Else != nil {
			c.checkStmt(s.Else, scope)
		}

	// A guard's bindings outlive it — that is what a guard is for —
	// but the else block runs on the path where they were not bound,
	// so it is checked before they are added.
	case *ast.GuardStmt:
		condScope := NewScope(scope, s.Pos(), s.End())
		for _, cond := range s.Conds {
			c.checkCondition(cond, condScope)
		}
		c.checkCodeBlock(s.Body, scope)
		condScope.hoistInto(scope)

	case *ast.WhileStmt:
		condScope := NewScope(scope, s.Pos(), s.End())
		for _, cond := range s.Conds {
			c.checkCondition(cond, condScope)
		}
		c.checkCodeBlock(s.Body, condScope)

	case *ast.RepeatWhileStmt:
		c.checkCodeBlock(s.Body, scope)
		if s.Cond != nil {
			condT := c.checkExpr(s.Cond, types.Typ[types.Bool], scope)
			if !types.Identical(condT, types.Typ[types.Bool]) {
				c.typeErrorf(s.Cond.Pos(), "repeat-while condition must be of type 'Bool', got '%s'", condT)
			}
		}

	case *ast.ForInStmt:
		// The element type comes from the sequence. Only Array is
		// modelled as one so far; anything else — a range, a
		// dictionary, a type conforming to Sequence — says nothing
		// this compiler can read yet.
		seqType := c.checkExpr(s.Seq, nil, scope)
		elemType := types.Type(types.Typ[types.Invalid])
		if arr, ok := seqType.Underlying().(*types.Array); ok {
			elemType = arr.Elem
		}
		loopScope := NewScope(scope, s.Pos(), s.End())
		c.info.Scopes[s] = loopScope
		c.declarePattern(s.Pat, elemType, true, loopScope)
		c.checkCodeBlock(s.Body, loopScope)

	case *ast.SwitchStmt:
		subjectType := c.checkExpr(s.Subject, nil, scope)
		var matchedCases map[string]bool
		var hasDefault bool
		en, isEnum := subjectType.Underlying().(*types.Enum)
		if isEnum {
			matchedCases = make(map[string]bool)
		}

		for _, caseStmt := range s.Cases {
			if cs, ok := caseStmt.(*ast.CaseClause); ok {
				caseScope := NewScope(scope, cs.Pos(), cs.End())
				c.info.Scopes[cs] = caseScope

				if cs.Kind == token.DEFAULT {
					hasDefault = true
				}

				for _, item := range cs.Items {
					if item.Where != nil {
						c.checkExpr(item.Where.Cond, types.Typ[types.Bool], caseScope)
					}
					if item.Pat != nil {
						c.declareCasePattern(item.Pat, subjectType, caseScope)
					}
					if isEnum && item.Where == nil {
						c.collectMatchedCases(item.Pat, matchedCases, &hasDefault)
					}
				}
				for _, st := range cs.Stmts {
					c.checkStmt(st, caseScope)
				}
			}
		}

		if isEnum && !hasDefault {
			var missing []string
			for _, ec := range en.Cases {
				if !matchedCases[ec.Name] {
					missing = append(missing, "."+ec.Name)
				}
			}
			if len(missing) > 0 {
				c.errorf(s.Switch, "switch must be exhaustive (missing: %s)", strings.Join(missing, ", "))
			}
		}

	case *ast.CodeBlock:
		c.checkCodeBlock(s, scope)
	}
}

func (c *checker) declareCasePattern(pat ast.Pattern, subjectType types.Type, scope *Scope) {
	switch p := pat.(type) {
	case *ast.ValueBindingPattern:
		c.declarePattern(p.Pat, subjectType, p.Kind == token.LET, scope)
	// `case .circle(let r)` binds r to the case's associated value,
	// not to the enum: what is matched and what is bound are
	// different types, and only the case says which.
	case *ast.EnumCasePattern:
		if p.Args == nil {
			return
		}
		assoc := c.associatedTypeOf(subjectType, p.Name)
		elems := p.Args.Elems
		for i, el := range elems {
			c.declareCasePattern(el.Pat, elementAt(assoc, i, len(elems)), scope)
		}
	case *ast.TuplePattern:
		for _, el := range p.Elems {
			c.declareCasePattern(el.Pat, subjectType, scope)
		}
	}
}

// associatedTypeOf is the type of the values a case of t carries.
func (c *checker) associatedTypeOf(t types.Type, name *ast.Ident) types.Type {
	if t == nil || name == nil {
		return nil
	}
	en, ok := t.Underlying().(*types.Enum)
	if !ok {
		return nil
	}
	want := name.Text(c.file)
	for _, cs := range en.Cases {
		if cs.Name == want {
			return cs.AssociatedType
		}
	}
	return nil
}

// elementAt picks the type one sub-pattern of a case matches: the
// whole associated type where the case carries one value, and the
// element at i where it carries several.
func elementAt(assoc types.Type, i, n int) types.Type {
	if assoc == nil {
		return nil
	}
	if n == 1 {
		return assoc
	}
	if tup, ok := assoc.Underlying().(*types.Tuple); ok && i < len(tup.Elements) {
		return tup.Elements[i].Type
	}
	return nil
}

func (c *checker) collectMatchedCases(pat ast.Pattern, matched map[string]bool, hasDefault *bool) {
	if pat == nil {
		return
	}
	switch p := pat.(type) {
	case *ast.WildcardPattern:
		*hasDefault = true
	case *ast.EnumCasePattern:
		matched[p.Name.Text(c.file)] = true
	case *ast.ExprPattern:
		if mem, ok := p.X.(*ast.MemberExpr); ok {
			matched[mem.Name.Text(c.file)] = true
		} else if id, ok := p.X.(*ast.IdentExpr); ok {
			matched[id.Name.Text(c.file)] = true
		}
	case *ast.ValueBindingPattern:
		if _, ok := p.Pat.(*ast.IdentPattern); ok {
			*hasDefault = true
		} else {
			c.collectMatchedCases(p.Pat, matched, hasDefault)
		}
	case *ast.IdentPattern:
		*hasDefault = true
	}
}

func (c *checker) checkCodeBlock(block *ast.CodeBlock, parent *Scope) {
	if block == nil {
		return
	}
	blockScope := NewScope(parent, block.Pos(), block.End())
	c.info.Scopes[block] = blockScope
	// A function declared in a block is visible throughout it, before
	// its declaration as well as after, so the names come first.
	c.declareFunctions(block.Stmts, blockScope)
	for _, s := range block.Stmts {
		c.checkStmt(s, blockScope)
	}
}

func (c *checker) checkCondition(cond ast.Node, scope *Scope) {
	if cond == nil {
		return
	}
	switch cn := cond.(type) {
	case ast.Expr:
		t := c.checkExpr(cn, types.Typ[types.Bool], scope)
		if !types.Identical(t, types.Typ[types.Bool]) {
			c.typeErrorf(cn.Pos(), "condition must be of type 'Bool', got '%s'", t)
		}

	case *ast.OptionalBinding:
		var innerType types.Type
		if cn.Value != nil {
			initType := c.checkExpr(cn.Value, nil, scope)
			innerType = initType
			if opt, ok := initType.(*types.Optional); ok {
				innerType = opt.Wrapped
			}
		} else {
			// Shorthand: if let x
			if idPat, ok := cn.Pat.(*ast.IdentPattern); ok {
				name := idPat.Name.Text(c.file)
				if sym := scope.Lookup(name); sym != nil {
					innerType = sym.Type()
					if opt, ok := innerType.(*types.Optional); ok {
						innerType = opt.Wrapped
					}
				}
			}
		}
		if innerType == nil {
			innerType = types.Typ[types.Invalid]
		}
		isConst := cn.Kind == token.LET
		c.declarePattern(cn.Pat, innerType, isConst, scope)

	case *ast.CaseCond:
		valT := c.checkExpr(cn.Value, nil, scope)
		c.declarePattern(cn.Pat, valT, true, scope)
	}
}

func (c *checker) checkDecl(decl ast.Decl, scope *Scope) {
	if decl == nil {
		return
	}
	switch d := decl.(type) {
	case *ast.VarDecl:
		isConst := d.Kind == token.LET
		for _, b := range d.Bindings {
			var expectedType types.Type
			if tp, ok := b.Pat.(*ast.TypedPattern); ok {
				expectedType = c.resolveType(tp.Type, scope)
			}
			var initType types.Type
			hasInit := b.Value != nil
			if hasInit {
				initType = c.checkExpr(b.Value, expectedType, scope)
				if expectedType != nil && !types.AssignableTo(initType, expectedType) {
					c.typeErrorf(b.Value.Pos(), "cannot convert value of type '%s' to specified type '%s'", initType, expectedType)
				}
			}
			declType := expectedType
			if declType == nil {
				declType = initType
			}
			// A binding with neither an annotation nor an initializer
			// this compiler can read has no type, and saying Int
			// would be inventing one.
			if declType == nil {
				declType = types.Typ[types.Invalid]
			}
			c.declarePatternInit(b.Pat, declType, isConst, hasInit, scope)
		}

	case *ast.FuncDecl:
		c.checkFuncBody(d, scope)

	case *ast.StructDecl:
		c.checkMembers(d, d.Body, c.declaredType(d.Name, scope))

	case *ast.ClassDecl:
		c.checkMembers(d, d.Body, c.declaredType(d.Name, scope))

	case *ast.ActorDecl:
		c.checkMembers(d, d.Body, c.declaredType(d.Name, scope))

	case *ast.EnumDecl:
		c.checkMembers(d, d.Body, c.declaredType(d.Name, scope))

	case *ast.ExtensionDecl:
		c.checkMembers(d, d.Body, c.resolveType(d.Type, scope))
	}
}

// declaredType is the type a nominal declaration's name denotes.
func (c *checker) declaredType(name *ast.Ident, scope *Scope) types.Type {
	if name == nil {
		return nil
	}
	if sym := scope.Lookup(name.Text(c.file)); sym != nil {
		return sym.Type()
	}
	return nil
}

// checkMembers checks the bodies of a type's members, with self bound
// to the type they are written in. Every nominal declaration and an
// extension reach this the same way, because inside the braces they
// are the same thing: declarations with a self.
func (c *checker) checkMembers(d ast.Decl, body *ast.MemberBlock, self types.Type) {
	typeScope := c.info.Scopes[d]
	if body == nil || typeScope == nil {
		return
	}

	prevType, prevActor := c.currType, c.currActor
	c.currType = self
	if cl, ok := self.(*types.Class); ok && cl.IsActor {
		c.currActor = cl
	} else {
		c.currActor = nil
	}
	defer func() { c.currType, c.currActor = prevType, prevActor }()

	for _, mem := range body.Members {
		if f, ok := mem.(*ast.FuncDecl); ok {
			c.checkFuncBody(f, typeScope)
		}
	}
}

func (c *checker) checkFuncBody(d *ast.FuncDecl, scope *Scope) {
	fnScope := NewScope(scope, d.Pos(), d.End())
	c.info.Scopes[d] = fnScope

	sig := c.buildFuncSig(d.Sig, scope)
	for _, p := range sig.Params {
		sym := NewVar(p.Name, p.Type, d.Name.Pos(), true, p.Ownership)
		fnScope.Insert(sym)
	}

	prevRet := c.currFuncRet
	c.currFuncRet = sig.Results
	defer func() { c.currFuncRet = prevRet }()

	// The body is a scope of its own: Swift lets a local shadow a
	// parameter, and only a second declaration in one scope is a
	// redeclaration.
	if d.Body != nil {
		bodyScope := NewScope(fnScope, d.Body.Pos(), d.Body.End())
		c.info.Scopes[d.Body] = bodyScope
		c.declareFunctions(d.Body.Stmts, bodyScope)
		for _, st := range d.Body.Stmts {
			c.checkStmt(st, bodyScope)
		}
	}
}
