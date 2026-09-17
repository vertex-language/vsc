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

	case *ast.LabeledStmt:
		c.checkStmt(s.Stmt, scope)

	// Catch clauses bind 'error' if no explicit pattern is provided.
	case *ast.DoStmt:
		c.checkCodeBlock(s.Body, scope)
		for _, cl := range s.Catches {
			catchScope := NewScope(scope, cl.Pos(), cl.End())
			c.info.Scopes[cl] = catchScope
			for _, item := range cl.Items {
				if item.Pat != nil {
					c.declareCatchPattern(item.Pat, true, catchScope)
				}
				if item.Where != nil {
					c.checkExpr(item.Where.Cond, types.Typ[types.Bool], catchScope)
				}
			}
			if len(cl.Items) == 0 {
				catchScope.Insert(NewVar("error", types.ErrorProtocol, cl.Pos(), true, types.DefaultOwnership))
			}
			c.checkCodeBlock(cl.Body, catchScope)
		}

	case *ast.DeferStmt:
		c.checkCodeBlock(s.Body, scope)

	case *ast.ThrowStmt:
		c.checkExpr(s.X, nil, scope)

	case *ast.DiscardStmt:
		c.checkExpr(s.X, nil, scope)

	case *ast.YieldStmt:
		c.checkExpr(s.X, nil, scope)

	case *ast.IfConfigStmt:
		for _, cl := range s.Clauses {
			for _, st := range cl.Stmts {
				c.checkStmt(st, scope)
			}
		}

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

	case *ast.IfStmt:
		condScope := NewScope(scope, s.Pos(), s.End())
		for _, cond := range s.Conds {
			c.checkCondition(cond, condScope)
		}
		done := [][]*VarSymbol{c.branch(func() { c.checkCodeBlock(s.Body, condScope) })}
		if s.Else != nil {
			done = append(done, c.branch(func() { c.checkStmt(s.Else, scope) }))
		}
		c.joinBranches(done)

	// Guard condition bindings are hoisted into enclosing scope after checking else.
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
		// Determine loop element type from sequence type.
		seqType := c.checkExpr(s.Seq, nil, scope)
		elemType := types.Type(types.Typ[types.Invalid])
		switch seq := seqType.Underlying().(type) {
		case *types.Array:
			elemType = seq.Elem
		case *types.Range:
			elemType = seq.Element
		case *types.Set:
			elemType = seq.Elem
		case *types.Dictionary:
			elemType = &types.Tuple{Elements: []*types.TupleElement{
				{Name: "key", Type: seq.Key}, {Name: "value", Type: seq.Value}}}
		}
		loopScope := NewScope(scope, s.Pos(), s.End())
		c.info.Scopes[s] = loopScope
		if s.Case.IsValid() {
			// `for case P in s` matches each element against P.
			c.declareCasePattern(s.Pat, elemType, loopScope)
		} else {
			c.declarePattern(s.Pat, elemType, true, loopScope)
		}
		if s.Where != nil {
			if t := c.checkExpr(s.Where.Cond, types.Typ[types.Bool], loopScope); t != nil && !isInvalid(t) &&
				!types.Identical(t, types.Typ[types.Bool]) {
				c.typeErrorf(s.Where.Cond.Pos(), "cannot convert value of type '%s' to expected condition type 'Bool'", t)
			}
		}
		c.checkCodeBlock(s.Body, loopScope)

	case *ast.SwitchStmt:
		subjectType := c.checkExpr(s.Subject, nil, scope)
		matchedCases := make(map[string]bool)
		var hasDefault bool
		var caseInits [][]*VarSymbol

		for _, caseStmt := range s.Cases {
			if cs, ok := caseStmt.(*ast.CaseClause); ok {
				caseScope := NewScope(scope, cs.Pos(), cs.End())
				c.info.Scopes[cs] = caseScope

				if cs.Kind == token.DEFAULT {
					hasDefault = true
				}

				for _, item := range cs.Items {
					// Declare pattern bindings before checking where clause condition.
					if item.Pat != nil {
						c.declareCasePattern(item.Pat, subjectType, caseScope)
					}
					if item.Where != nil {
						c.checkExpr(item.Where.Cond, types.Typ[types.Bool], caseScope)
					}
					if item.Where == nil {
						c.collectMatchedCases(item.Pat, matchedCases, &hasDefault)
					}
				}
				caseInits = append(caseInits, c.branch(func() {
					for _, st := range cs.Stmts {
						c.checkStmt(st, caseScope)
					}
				}))
			}
		}
		c.joinBranches(caseInits)

		if !hasDefault && subjectType != nil && !isInvalid(subjectType) {
			switch u := subjectType.Underlying().(type) {
			case *types.Enum:
				var missing []string
				for _, ec := range u.Cases {
					if !matchedCases[ec.Name] {
						missing = append(missing, "."+ec.Name)
					}
				}
				if len(missing) > 0 {
					c.errorf(s.Switch, "switch must be exhaustive (missing: %s)", strings.Join(missing, ", "))
				}
			case *types.Optional:
				if !matchedCases["some"] || !matchedCases["none"] {
					c.errorf(s.Switch, "switch must be exhaustive")
				}
			case *types.Basic:
				if u.Kind() != types.Bool || !matchedCases["true"] || !matchedCases["false"] {
					c.errorf(s.Switch, "switch must be exhaustive")
				}
			case *types.Tuple:
				// Not modelled: a tuple's space is the product of its
				// elements', and a switch over one is refused later.
			default:
				c.errorf(s.Switch, "switch must be exhaustive")
			}
		}

	case *ast.CodeBlock:
		c.checkCodeBlock(s, scope)
	}
}

// declareCatchPattern binds what a catch clause's pattern names. The
// error arrives as `any Error`; a pattern that tests its type -- `is T`,
// `let e as T`, `E.case(let x)` -- binds names of that type instead, and
// the type is recorded for the lowering to test the error against.
func (c *checker) declareCatchPattern(pat ast.Pattern, isConst bool, scope *Scope) {
	switch p := pat.(type) {
	case *ast.ValueBindingPattern:
		c.declareCatchPattern(p.Pat, p.Kind == token.LET, scope)
	case *ast.IsPattern:
		c.info.PatternTypes[p] = c.resolveType(p.Type, scope)
	case *ast.AsPattern:
		t := c.resolveType(p.Type, scope)
		c.info.PatternTypes[p] = t
		c.declarePatternInit(p.Pat, t, isConst, true, scope)
	case *ast.EnumCasePattern:
		if p.Type == nil {
			c.errorf(p.Pos(), "a catch pattern needs its enum named, as in 'E.%s': "+
				"the error is 'any Error', which has no cases", p.Name.Text(c.file))
			return
		}
		t := c.resolveType(p.Type, scope)
		c.info.PatternTypes[p] = t
		c.declareCasePattern(p, t, scope)
	default:
		c.declarePattern(pat, types.ErrorProtocol, isConst, scope)
	}
}

func (c *checker) declareCasePattern(pat ast.Pattern, subjectType types.Type, scope *Scope) {
	switch p := pat.(type) {
	case *ast.ValueBindingPattern:
		c.declareBoundPattern(p.Pat, subjectType, p.Kind == token.LET, scope)

	case *ast.ExprPattern:
		if p.X == nil {
			return
		}
		// Match against subject type or range element type.
		t := c.checkExpr(p.X, subjectType, scope)
		if rng, isRange := t.(*types.Range); isRange {
			t = rng.Element
		}
		if subjectType != nil && t != nil &&
			!types.Identical(t, subjectType) &&
			!isInvalid(t) && !isInvalid(subjectType) {
			c.typeErrorf(p.X.Pos(), "expression pattern of type '%s' cannot match values of type '%s'", t, subjectType)
		}
	case *ast.EnumCasePattern:
		// `case Code.refused:` over something that is not that enum
		// names a static property of Code, and matches what it equals.
		if p.Type != nil && p.Args == nil && p.Name != nil {
			if _, isEnum := underlyingEnumOf(subjectType); !isEnum {
				owner := c.resolveType(p.Type, scope)
				name := p.Name.Text(c.file)
				member := c.lookupMember(&types.Metatype{Instance: owner}, name)
				switch {
				case member == nil:
					c.typeErrorf(p.Name.Pos(), "type '%s' has no member '%s'", owner, name)
				case subjectType != nil && !isInvalid(subjectType) && !types.AssignableTo(member, subjectType):
					c.typeErrorf(p.Name.Pos(), "expression pattern of type '%s' cannot match values of type '%s'", member, subjectType)
				default:
					c.info.PatternTypes[p] = owner
				}
				return
			}
		}
		if p.Args == nil {
			return
		}
		assoc := c.associatedTypeOf(subjectType, p.Name)
		elems := p.Args.Elems
		for i, el := range elems {
			c.declareCasePattern(el.Pat, elementAt(assoc, i, len(elems)), scope)
		}
	case *ast.TuplePattern:
		for i, el := range p.Elems {
			c.declareCasePattern(el.Pat, elementAt(subjectType, i, len(p.Elems)), scope)
		}
	}
}

// associatedTypeOf returns the type carried by an enum case.
func (c *checker) associatedTypeOf(t types.Type, name *ast.Ident) types.Type {
	if t == nil || name == nil {
		return nil
	}
	want := name.Text(c.file)
	// Optional is the enum it is in Swift: some carries what it wraps.
	if o, ok := t.(*types.Optional); ok {
		if want == "some" {
			return o.Wrapped
		}
		return nil
	}
	en, ok := t.Underlying().(*types.Enum)
	if !ok {
		return nil
	}
	for _, cs := range en.Cases {
		if cs.Name == want {
			return cs.AssociatedType
		}
	}
	return nil
}

// elementAt returns the type of the i-th component of an associated or tuple type.
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
		} else if lit, ok := p.X.(*ast.BasicLit); ok {
			switch lit.Kind {
			case token.TRUE:
				matched["true"] = true
			case token.FALSE:
				matched["false"] = true
			case token.NIL:
				matched["none"] = true
			}
		}
	case *ast.OptionalPattern:
		// `let x?` is .some of whatever x matches, all of it only where
		// x matches everything.
		inner := map[string]bool{}
		all := false
		c.collectMatchedCases(p.Pat, inner, &all)
		if all {
			matched["some"] = true
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
	// Functions in a block are visible throughout the block.
	c.declareFunctions(declsOf(block.Stmts), blockScope)
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
		c.declareCasePattern(cn.Pat, valT, scope)
	}
}

// checkStored declares one stored binding and checks its initializer.
//
// It is called once per binding: at module scope by declareModuleVars,
// before any body is checked, and everywhere else by checkDecl as the
// declaration is reached.
func (c *checker) checkStored(b *ast.PatternBinding, isConst bool, scope *Scope) {
	// Once per scope: a closure checked a second time -- against another
	// expected type -- has a scope of its own again, and its bindings
	// have to be declared in it.
	if c.stored[b] == scope {
		return
	}
	if c.stored == nil {
		c.stored = map[*ast.PatternBinding]*Scope{}
	}
	c.stored[b] = scope

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
	if declType == nil {
		declType = types.Typ[types.Invalid]
	}
	c.declarePatternInit(b.Pat, declType, isConst, hasInit, scope)
}

// declareModuleVars declares the module's stored variables, before any
// body is checked.
//
// A module-scope variable is visible to the whole module and not only to
// what the compiler happened to read after it -- Swift has no ordering
// rule for a declaration at that scope, and neither do this checker's
// types and functions, which have had passes of their own all along.
// Without this, a `let` in one file is out of scope in another the
// compiler read first, which is a rule about file order and nothing else.
//
// An initializer is checked here, so one variable may read another only
// where that other is already declared. That much order remains, and it
// is the order Swift's own module-scope initializers run in.
func (c *checker) declareModuleVars(decls []ast.Decl, scope *Scope) {
	for _, decl := range decls {
		d, ok := decl.(*ast.VarDecl)
		if !ok {
			continue
		}
		for _, b := range d.Bindings {
			// A computed variable is a getter and no storage:
			// declareFunctions has it.
			if b.Body != nil || b.Accessors != nil {
				continue
			}
			c.checkStored(b, d.Kind == token.LET, scope)
		}
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
			if b.Body != nil || b.Accessors != nil {
				// A computed variable: a getter and no storage, so there is
				// nothing to initialize and it is never used before it is.
				// declareFunctions declared it already, where it could.
				tp, typed := b.Pat.(*ast.TypedPattern)
				declared := false
				if typed {
					if id, ok := tp.Pat.(*ast.IdentPattern); ok && id.Name != nil {
						_, declared = c.info.Defs[id.Name]
					}
				}
				if !declared {
					var declType types.Type = types.Typ[types.Invalid]
					if typed {
						declType = c.resolveType(tp.Type, scope)
					}
					c.declarePatternInit(b.Pat, declType, isConst, true, scope)
				}
				c.checkBinding(b, scope)
				continue
			}
			c.checkStored(b, isConst, scope)
		}

	case *ast.FuncDecl:
		if d.Recv != nil {
			c.checkReceiverMethod(d, scope)
			break
		}
		// A generic function's body is checked where its parameters were
		// declared, so a local written `var acc: [T]` finds T.
		bodyScope := scope
		if generic := c.info.Scopes[d]; generic != nil && d.Generics != nil {
			bodyScope = generic
		}
		c.checkFuncBody(d, bodyScope)

	case *ast.StructDecl:
		c.checkMembers(d, d.Body, c.declaredType(d.Name, scope))

	case *ast.ClassDecl:
		c.checkMembers(d, d.Body, c.declaredType(d.Name, scope))

	case *ast.ActorDecl:
		c.checkMembers(d, d.Body, c.declaredType(d.Name, scope))

	case *ast.EnumDecl:
		self := c.declaredType(d.Name, scope)
		c.checkMembers(d, d.Body, self)
		numberRawCases(self)

	case *ast.ExtensionDecl:
		c.checkMembers(d, d.Body, c.extensionType(d, scope))
	}
}

// checkReceiverMethod checks a method written outside its type's body.
func (c *checker) checkReceiverMethod(d *ast.FuncDecl, scope *Scope) {
	self := c.info.Receivers[d]
	if self == nil {
		return
	}
	typeScope := c.typeScopes[typeNameOf(self)]
	if typeScope == nil {
		typeScope = scope
	}
	prevType, prevActor := c.currType, c.currActor
	c.currType = self
	if cl, ok := self.(*types.Class); ok && cl.IsActor {
		c.currActor = cl
	} else {
		c.currActor = nil
	}
	defer func() { c.currType, c.currActor = prevType, prevActor }()
	c.checkFuncBody(d, receiverBodyScope(typeScope, scope, d))
}

// receiverBodyScope is where a receiver method's body is checked. The body
// sees the type's members by their bare names, as a method declared inside
// the type would, with one exception: a method whose name is also a type in
// scope does not hide the type. A receiver method is declared at package
// scope, beside the types it names, so `func (w: borrowing Window) Size() ->
// Size` returning `Size(1, 2)` means the type Size in its body as it does in
// its signature; the method is still reached as `w.Size()`.
func receiverBodyScope(typeScope, pkg *Scope, d *ast.FuncDecl) *Scope {
	if typeScope == nil || typeScope == pkg {
		return pkg
	}
	body := NewScope(typeScope.parent, d.Pos(), d.End())
	for curr := typeScope; curr != nil && curr != pkg; curr = curr.parent {
		for name, sym := range curr.elems {
			if _, seen := body.elems[name]; seen {
				continue
			}
			if _, isFunc := sym.(*FuncSymbol); isFunc && pkg.LookupType(name) != nil {
				continue
			}
			body.elems[name] = sym
		}
	}
	return body
}

// checkMutatingPlacement rejects 'mutating' on class or actor methods.
func (c *checker) checkMutatingPlacement(d *ast.FuncDecl, self types.Type) {
	if self == nil {
		return
	}
	cl, isClass := self.Underlying().(*types.Class)
	if !isClass {
		return
	}
	kind := "class"
	if cl.IsActor {
		kind = "actor"
	}
	for _, m := range d.Mods {
		if m.Name == nil || m.Name.Text(c.file) != "mutating" {
			continue
		}
		c.errorf(m.Pos(), "'mutating' is not valid on instance methods in %ses", kind)
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
// to the type they are written in.
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

	// An extension's where clause holds inside it: `extension Box where T:
	// Equatable` may compare its Ts. The type's parameters are the type's
	// everywhere else, so what the clause adds is taken back after.
	if ext, ok := d.(*ast.ExtensionDecl); ok && ext.Where != nil {
		defer c.restoreTypeParams(c.typeParamsOf(self))()
		c.applyWhere(ext.Where, typeScope)
	}

	for _, mem := range body.Members {
		c.checkMember(mem, typeScope, self)
	}
}

// typeParamsOf is the generic parameters a nominal type declares, or a
// built-in type's extensions are written in.
func (c *checker) typeParamsOf(t types.Type) []*types.TypeParam {
	if t == nil {
		return nil
	}
	if b := c.builtinOf(t); b != nil && types.Identical(b.Type, t) {
		return b.Params
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		t = inst.Base
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		return u.TypeParams
	case *types.Class:
		return u.TypeParams
	case *types.Enum:
		return u.TypeParams
	}
	return nil
}

// restoreTypeParams saves what type parameters promise and returns what
// puts it back.
func (c *checker) restoreTypeParams(tps []*types.TypeParam) func() {
	type saved struct {
		constraints []types.Type
		bound       map[string]types.Type
		promised    map[string][]types.Type
	}
	was := make([]saved, len(tps))
	for i, tp := range tps {
		was[i] = saved{append([]types.Type(nil), tp.Constraints...), copyBound(tp.Bound), copyPromised(tp.Promised)}
	}
	return func() {
		for i, tp := range tps {
			tp.Constraints, tp.Bound, tp.Promised = was[i].constraints, was[i].bound, was[i].promised
		}
	}
}

func copyBound(m map[string]types.Type) map[string]types.Type {
	if m == nil {
		return nil
	}
	out := make(map[string]types.Type, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyPromised(m map[string][]types.Type) map[string][]types.Type {
	if m == nil {
		return nil
	}
	out := make(map[string][]types.Type, len(m))
	for k, v := range m {
		out[k] = append([]types.Type(nil), v...)
	}
	return out
}

// checkMember checks one member of a type.
func (c *checker) checkMember(mem ast.Node, typeScope *Scope, self types.Type) {
	switch m := mem.(type) {
	case *ast.FuncDecl:
		c.checkMutatingPlacement(m, self)
		// The scope its own generic parameters were declared in, if it
		// has any, so its body sees them too.
		scope := typeScope
		if generic := c.info.Scopes[m]; generic != nil && m.Generics != nil {
			scope = generic
		}
		c.checkFuncBody(m, scope)

	// Initializers return Void (or Void? for failable initializers).
	case *ast.InitDecl:
		result := types.Type(types.Typ[types.Void])
		if m.Question.IsValid() || m.Exclaim.IsValid() {
			result = &types.Optional{Wrapped: types.Typ[types.Void]}
		}
		prevInit, prevName := c.inInit, c.currFuncName
		c.inInit = true
		if m.Sig != nil {
			c.currFuncName = c.declName("init", m.Sig.Params, false)
		}
		c.checkBodyWithParams(m, m.Sig, m.Body, typeScope, result)
		c.inInit, c.currFuncName = prevInit, prevName

	case *ast.DeinitDecl:
		prevName := c.currFuncName
		c.currFuncName = "deinit"
		c.checkBodyWithParams(m, nil, m.Body, typeScope, nil)
		c.currFuncName = prevName

	case *ast.SubscriptDecl:
		sig := &ast.FuncSig{Lparen: m.Lparen, Params: m.Params, Rparen: m.Rparen, Result: m.Result}
		var result types.Type
		if m.Result != nil {
			result = c.resolveType(m.Result.Type, typeScope)
		}
		prevName := c.currFuncName
		c.currFuncName = c.declName("subscript", m.Params, true)
		scope := c.checkBodyWithParams(m, sig, m.Body, typeScope, result)
		c.checkAccessors(m.Accessors, scope, result)
		c.currFuncName = prevName

	case *ast.VarDecl:
		// A lazy one runs when first read, when there is a self.
		static := c.hasModifier(m.Mods, "static") || c.hasModifier(m.Mods, "class") ||
			c.hasModifier(m.Mods, "lazy")
		for _, b := range m.Bindings {
			// A stored property's initializer runs before self exists.
			prev := c.inPropertyInit
			c.inPropertyInit = !static && b.Body == nil && b.Accessors == nil && b.Value != nil
			c.checkBinding(b, typeScope)
			c.inPropertyInit = prev
		}

	case *ast.EnumCaseDecl:
		for _, el := range m.Elements {
			if el.Value == nil {
				continue
			}
			c.checkExpr(el.Value, rawValueOf(self), typeScope)
			c.recordRawValue(self, el)
		}

	case *ast.StructDecl:
		c.checkMembers(m, m.Body, c.declaredType(m.Name, typeScope))
	case *ast.ClassDecl:
		c.checkMembers(m, m.Body, c.declaredType(m.Name, typeScope))
	case *ast.ActorDecl:
		c.checkMembers(m, m.Body, c.declaredType(m.Name, typeScope))
	case *ast.EnumDecl:
		inner := c.declaredType(m.Name, typeScope)
		c.checkMembers(m, m.Body, inner)
		numberRawCases(inner)
	case *ast.ExtensionDecl:
		c.checkMembers(m, m.Body, c.extensionType(m, typeScope))
	}
}

// rawValueOf is the type an enum's cases are numbered or named with.
func rawValueOf(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	if en, ok := t.Underlying().(*types.Enum); ok {
		return en.RawType
	}
	return nil
}

// checkBinding type-checks a property binding and its optional accessors.
func (c *checker) checkBinding(b *ast.PatternBinding, scope *Scope) {
	var declared types.Type
	if tp, ok := b.Pat.(*ast.TypedPattern); ok {
		declared = c.resolveType(tp.Type, scope)
	}
	if b.Value != nil {
		valueType := c.checkExpr(b.Value, declared, scope)
		if declared != nil && !types.AssignableTo(valueType, declared) {
			c.typeErrorf(b.Value.Pos(), "cannot convert value of type '%s' to specified type '%s'", valueType, declared)
		}
	}
	if b.Body != nil || b.Accessors != nil {
		prevName := c.currFuncName
		if name := bindingIdent(b.Pat); name != nil {
			c.currFuncName = name.Text(c.file)
		}
		defer func() { c.currFuncName = prevName }()
	}
	if b.Body != nil {
		c.checkReturningBlock(b.Body, scope, declared)
	}
	c.checkAccessors(b.Accessors, scope, declared)
}

// bindingIdent is the name a property binding declares, or nil.
func bindingIdent(p ast.Pattern) *ast.Ident {
	if tp, ok := p.(*ast.TypedPattern); ok {
		p = tp.Pat
	}
	if id, ok := p.(*ast.IdentPattern); ok {
		return id.Name
	}
	return nil
}

// declName spells a declaration the way #function does: its base name and
// each parameter's argument label, `_` for none -- `m(_:b:)`. A subscript's
// parameters have no label unless one is written.
func (c *checker) declName(base string, params []*ast.Param, subscript bool) string {
	var b strings.Builder
	b.WriteString(base)
	b.WriteByte('(')
	for _, p := range params {
		switch {
		case p.Label != nil:
			b.WriteString(p.Label.Text(c.file))
		case p.Name != nil && !subscript:
			b.WriteString(p.Name.Text(c.file))
		default:
			b.WriteString("_")
		}
		b.WriteByte(':')
	}
	b.WriteByte(')')
	return b.String()
}

// checkAccessors checks accessor blocks (get, set, willSet, didSet).
func (c *checker) checkAccessors(block *ast.AccessorBlock, scope *Scope, valueType types.Type) {
	if block == nil {
		return
	}
	for _, a := range block.Accessors {
		if a.Body == nil {
			continue // protocol declaration: no body to check
		}
		accScope := NewScope(scope, a.Pos(), a.End())
		c.info.Scopes[a] = accScope
		if name := c.accessorValueName(a); name != "" {
			accScope.Insert(NewVar(name, valueType, a.Pos(), true, types.DefaultOwnership))
		}
		result := valueType
		if !c.accessorReturns(a) {
			result = types.Typ[types.Void]
		}
		c.checkReturningBlock(a.Body, accScope, result)
	}
}

// accessorValueName returns the name an accessor's incoming value is bound to, or "" if none.
func (c *checker) accessorValueName(a *ast.Accessor) string {
	if a.Keyword == nil {
		return ""
	}
	switch a.Keyword.Text(c.file) {
	case "set", "_modify":
		if a.Name != nil {
			return a.Name.Text(c.file)
		}
		return "newValue"
	case "willSet":
		if a.Name != nil {
			return a.Name.Text(c.file)
		}
		return "newValue"
	case "didSet":
		if a.Name != nil {
			return a.Name.Text(c.file)
		}
		return "oldValue"
	}
	return ""
}

// accessorReturns reports whether an accessor produces a return value.
func (c *checker) accessorReturns(a *ast.Accessor) bool {
	if a.Keyword == nil {
		return false
	}
	switch a.Keyword.Text(c.file) {
	case "get", "_read", "unsafeAddress", "unsafeMutableAddress":
		return true
	}
	return false
}

// checkBodyWithParams checks a body whose parameters are declared by sig.
func (c *checker) checkBodyWithParams(d ast.Node, sig *ast.FuncSig, body *ast.CodeBlock, scope *Scope, result types.Type) *Scope {
	inner := NewScope(scope, d.Pos(), d.End())
	c.info.Scopes[d] = inner
	if sig != nil {
		for _, p := range c.buildFuncSig(sig, scope).Params {
			inner.Insert(NewVar(p.Name, p.Type, d.Pos(), true, p.Ownership))
		}
	}
	if body != nil {
		c.checkReturningBlock(body, inner, result)
	}
	return inner
}

// implicitReturn turns a body that is a single expression into a return of
// that expression, where the body produces a value: `func f() -> Int { 1 }`
// is `{ return 1 }`, as Swift reads it (SE-0255).
func implicitReturn(body *ast.CodeBlock, result types.Type) {
	if body == nil || len(body.Stmts) != 1 || result == nil || isVoidType(result) {
		return
	}
	st, ok := body.Stmts[0].(*ast.ExprStmt)
	if !ok || st.X == nil {
		return
	}
	body.Stmts[0] = &ast.ReturnStmt{Span: st.Span, X: st.X}
}

// checkReturningBlock checks a block whose returns produce result.
func (c *checker) checkReturningBlock(body *ast.CodeBlock, scope *Scope, result types.Type) {
	implicitReturn(body, result)
	prev := c.currFuncRet
	c.currFuncRet = result
	defer func() { c.currFuncRet = prev }()
	c.checkCodeBlock(body, scope)
}

func (c *checker) checkFuncBody(d *ast.FuncDecl, scope *Scope) {
	fnScope := NewScope(scope, d.Pos(), d.End())
	c.info.Scopes[d] = fnScope

	// Bind explicit receiver if present.
	if d.Recv != nil && d.Recv.Name != nil {
		if self := c.info.Receivers[d]; self != nil {
			own := c.ownershipOf(d.Recv.Mods)
			sym := NewVar(d.Recv.Name.Text(c.file), self, d.Recv.Name.Pos(),
				own != types.InOut, own)
			fnScope.Insert(sym)
			c.info.Defs[d.Recv.Name] = sym
		}
	}

	sig := c.buildFuncSig(d.Sig, scope)
	for _, p := range sig.Params {
		// Parameter bindings default to let unless marked inout.
		sym := NewVar(p.Name, p.BodyType(), d.Name.Pos(),
			p.Ownership != types.InOut, p.Ownership)
		fnScope.Insert(sym)
	}

	prevRet, prevAsync, prevName := c.currFuncRet, c.currAsync, c.currFuncName
	c.currFuncRet, c.currAsync = sig.Results, sig.Async
	if d.Name != nil && d.Sig != nil {
		c.currFuncName = c.declName(d.Name.Text(c.file), d.Sig.Params, false)
	}
	defer func() { c.currFuncRet, c.currAsync, c.currFuncName = prevRet, prevAsync, prevName }()

	if d.Body != nil {
		implicitReturn(d.Body, sig.Results)
		bodyScope := NewScope(fnScope, d.Body.Pos(), d.Body.End())
		c.info.Scopes[d.Body] = bodyScope
		c.declareFunctions(declsOf(d.Body.Stmts), bodyScope)
		for _, st := range d.Body.Stmts {
			c.checkStmt(st, bodyScope)
		}
	}
}

// recordRawValue records the integer raw value for an enum case.
func (c *checker) recordRawValue(self types.Type, el *ast.EnumCaseElem) {
	en, ok := self.Underlying().(*types.Enum)
	if !ok || en.RawType == nil || el.Name == nil {
		return
	}
	v, found := c.info.Values[el.Value]
	if !found || v.Kind != IntValue {
		return
	}
	name := el.Name.Text(c.file)
	for _, k := range en.Cases {
		if k != nil && k.Name == name {
			k.RawInt, k.HasRawInt = int64(v.Int), true
			return
		}
	}
}

// numberRawCases fills in consecutive integer raw values for enum cases lacking explicit values.
func numberRawCases(self types.Type) {
	if self == nil {
		return
	}
	en, ok := self.Underlying().(*types.Enum)
	if !ok || en.RawType == nil {
		return
	}
	b, ok := en.RawType.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsInteger == 0 {
		return
	}
	next := int64(0)
	for _, k := range en.Cases {
		if k == nil {
			continue
		}
		if k.HasRawInt {
			next = k.RawInt + 1
			continue
		}
		k.RawInt, k.HasRawInt = next, true
		next++
	}
}

// underlyingEnumOf is the enum a type is, if it is one.
func underlyingEnumOf(t types.Type) (*types.Enum, bool) {
	if t == nil {
		return nil, false
	}
	e, ok := t.Underlying().(*types.Enum)
	return e, ok
}

// branch checks code that runs instead of other code -- an if's body or
// its else, one case of a switch -- from the initialization state before
// it, and is the deferred lets it gave their value. They are uninitialized
// again after it, for the next branch to give theirs.
func (c *checker) branch(check func()) []*VarSymbol {
	outer := c.initLog
	c.initLog = nil
	check()
	got := c.initLog
	for _, v := range got {
		v.SetInitialized(false)
	}
	c.initLog = outer
	return got
}

// joinBranches initializes, after the branches, every deferred let any of
// them gave its value.
func (c *checker) joinBranches(done [][]*VarSymbol) {
	for _, vs := range done {
		for _, v := range vs {
			if !v.IsInitialized() {
				v.SetInitialized(true)
				c.initLog = append(c.initLog, v)
			}
		}
	}
}

// declareBoundPattern declares the names under a let or var that begins a
// case pattern: `case let .rect(w, h)` binds w and h, as `.rect(let w, let h)`
// does.
func (c *checker) declareBoundPattern(pat ast.Pattern, t types.Type, isConst bool, scope *Scope) {
	switch p := pat.(type) {
	case *ast.EnumCasePattern:
		if p.Args == nil {
			return
		}
		assoc := c.associatedTypeOf(t, p.Name)
		for i, el := range p.Args.Elems {
			c.declareBoundPattern(el.Pat, elementAt(assoc, i, len(p.Args.Elems)), isConst, scope)
		}
	case *ast.TuplePattern:
		for i, el := range p.Elems {
			c.declareBoundPattern(el.Pat, elementAt(t, i, len(p.Elems)), isConst, scope)
		}
	// `let x?` binds what the optional holds.
	case *ast.OptionalPattern:
		var wrapped types.Type
		if o, ok := t.(*types.Optional); ok {
			wrapped = o.Wrapped
		}
		c.declareBoundPattern(p.Pat, wrapped, isConst, scope)
	// Under a let, a name is bound; any other expression -- the "x" of
	// `let (n, "x")` -- is still a value matched against.
	case *ast.ExprPattern:
		if _, isName := p.X.(*ast.IdentExpr); !isName {
			c.checkExpr(p.X, t, scope)
			return
		}
		c.declarePattern(pat, t, isConst, scope)
	default:
		c.declarePattern(pat, t, isConst, scope)
	}
}

// extensionType is the type an extension extends, as its declaration was
// resolved.
func (c *checker) extensionType(d *ast.ExtensionDecl, scope *Scope) types.Type {
	if t := c.info.Extensions[d]; t != nil {
		return t
	}
	return c.resolveType(d.Type, scope)
}

// isVoidType reports whether t is Void, the empty tuple, or Never: a body
// of one of those types has no value for a lone expression to return.
func isVoidType(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Kind() == types.Void || u.Kind() == types.Never
	case *types.Tuple:
		return len(u.Elements) == 0
	}
	return false
}
