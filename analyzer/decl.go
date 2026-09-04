package analyzer

import (
	"fmt"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// resolveType converts an AST type node into a semantic types.Type.
//
// The answer is remembered. A type is written in one place and read
// in several — a binding's annotation is resolved once to check the
// initializer and again to declare the name — and a type that does
// not resolve should be reported once, not once per reader.
func (c *checker) resolveType(astType ast.Type, scope *Scope) types.Type {
	if astType == nil {
		return nil
	}
	if t, ok := c.resolved[astType]; ok {
		return t
	}
	t := c.resolveTypeUncached(astType, scope)
	if c.resolved == nil {
		c.resolved = make(map[ast.Type]types.Type)
	}
	c.resolved[astType] = t
	return t
}

func (c *checker) resolveTypeUncached(astType ast.Type, scope *Scope) types.Type {
	switch t := astType.(type) {
	case *ast.IdentType:
		name := t.Name.Text(c.file)
		if t.Args != nil && len(t.Args.Args) > 0 {
			if name == "Array" && len(t.Args.Args) == 1 {
				return &types.Array{Elem: c.resolveType(t.Args.Args[0], scope)}
			}
			if name == "Optional" && len(t.Args.Args) == 1 {
				return &types.Optional{Wrapped: c.resolveType(t.Args.Args[0], scope)}
			}
			if name == "Dictionary" && len(t.Args.Args) == 2 {
				return &types.Dictionary{
					Key:   c.resolveType(t.Args.Args[0], scope),
					Value: c.resolveType(t.Args.Args[1], scope),
				}
			}
		}

		var base types.Type
		// Check lexical scope first
		if sym := scope.Lookup(name); sym != nil {
			if tn, ok := sym.(*TypeNameSymbol); ok {
				base = tn.Type()
			}
		}
		// Check universe builtins
		if base == nil {
			if u := types.LookupUniverse(name); u != nil {
				base = u
			}
		}
		if base == nil {
			c.errorf(t.Pos(), "cannot find type '%s' in scope", name)
			return types.Typ[types.Invalid]
		}
		if t.Args != nil && len(t.Args.Args) > 0 {
			args := make([]types.Type, len(t.Args.Args))
			for i, arg := range t.Args.Args {
				args[i] = c.resolveType(arg, scope)
			}
			return &types.GenericInstance{Base: base, Args: args}
		}
		return base

	case *ast.ParenType:
		return c.resolveType(t.X, scope)

	case *ast.OptionalType:
		wrapped := c.resolveType(t.Base, scope)
		return &types.Optional{Wrapped: wrapped}

	case *ast.ArrayType:
		elem := c.resolveType(t.Elem, scope)
		return &types.Array{Elem: elem}

	case *ast.DictType:
		key := c.resolveType(t.Key, scope)
		val := c.resolveType(t.Value, scope)
		return &types.Dictionary{Key: key, Value: val}

	case *ast.TupleType:
		elems := make([]*types.TupleElement, len(t.Elems))
		for i, elem := range t.Elems {
			var label string
			if elem.Name != nil {
				label = elem.Name.Text(c.file)
			}
			elems[i] = &types.TupleElement{
				Name: label,
				Type: c.resolveType(elem.Type, scope),
			}
		}
		return &types.Tuple{Elements: elems}

	case *ast.FuncType:
		params := make([]*types.Param, len(t.Params))
		for i, p := range t.Params {
			var name, label string
			if p.Name != nil {
				name = p.Name.Text(c.file)
			}
			if p.Label != nil {
				label = p.Label.Text(c.file)
			}
			ownership := types.DefaultOwnership
			for _, m := range p.Mods {
				if m.Kind == token.INOUT {
					ownership = types.InOut
				}
			}
			params[i] = &types.Param{
				Name:      name,
				Label:     label,
				Type:      c.resolveType(p.Type, scope),
				Ownership: ownership,
				Variadic:  p.Ellipsis != token.NoPos,
			}
		}
		var throws types.Type
		if t.Throws != nil {
			if t.Throws.Type != nil {
				throws = c.resolveType(t.Throws.Type, scope)
			} else {
				throws = types.Typ[types.Never] // general throws
			}
		}
		return &types.Signature{
			Params:  params,
			Results: c.resolveType(t.Result, scope),
			Async:   t.Async != token.NoPos,
			Throws:  throws,
		}

	case *ast.AnyType:
		return &types.Existential{}

	case *ast.MetatypeType:
		inst := c.resolveType(t.Base, scope)
		return &types.Metatype{Instance: inst}

	case *ast.BoxedType:
		inner := c.resolveType(t.Base, scope)
		if p, ok := inner.(*types.Protocol); ok {
			return &types.Existential{Protocols: []*types.Protocol{p}}
		}
		return &types.Existential{}

	case *ast.OpaqueType:
		inner := c.resolveType(t.Base, scope)
		if p, ok := inner.(*types.Protocol); ok {
			return &types.Opaque{Constraints: []*types.Protocol{p}}
		}
		return &types.Opaque{Base: inner}

	case *ast.MemberType:
		base := c.resolveType(t.X, scope)
		name := t.Name.Text(c.file)
		return types.NewNamed(fmt.Sprintf("%s.%s", base, name), "", nil)

	default:
		return types.Typ[types.Invalid]
	}
}

// declarePrecedenceAndOperators discovers precedencegroup and operator declarations.
func (c *checker) declarePrecedenceAndOperators(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		declStmt, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		switch d := declStmt.D.(type) {
		case *ast.PrecedenceGroupDecl:
			groupName := d.Name.Text(c.file)
			grp := &PrecedenceGroup{
				Name:  groupName,
				Assoc: AssocNone,
			}
			for _, attr := range d.Attrs {
				switch a := attr.(type) {
				case *ast.PrecedenceRelation:
					kw := a.Keyword.Text(c.file)
					for _, name := range a.Names {
						n := name.Text(c.file)
						if kw == "higherThan" {
							grp.HigherThan = append(grp.HigherThan, n)
						} else if kw == "lowerThan" {
							grp.LowerThan = append(grp.LowerThan, n)
						}
					}
				case *ast.PrecedenceAssignment:
					val := string(c.file.Slice(a.Value.Lo, a.Value.Hi))
					grp.Assignment = val == "true"
				}
			}
			c.pg.AddGroup(grp)

		case *ast.OperatorDecl:
			opName := d.Name.Text(c.file)
			if d.Group != nil {
				grpName := d.Group.Text(c.file)
				c.pg.AddOperator(opName, grpName)
			}
		}
	}
}

// declareTypes discovers all nominal type declarations (Struct, Class, Enum, Protocol, Typealias).
func (c *checker) declareTypes(stmts []ast.Stmt, scope *Scope) {
	for _, stmt := range stmts {
		declStmt, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		switch d := declStmt.D.(type) {
		case *ast.StructDecl:
			name := d.Name.Text(c.file)
			st := &types.Struct{Name: name, Copyable: true}
			sym := NewTypeName(name, st, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym

		case *ast.ClassDecl:
			name := d.Name.Text(c.file)
			cl := &types.Class{Name: name}
			sym := NewTypeName(name, cl, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym

		case *ast.ActorDecl:
			name := d.Name.Text(c.file)
			cl := &types.Class{Name: name, IsActor: true}
			sym := NewTypeName(name, cl, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym

		case *ast.EnumDecl:
			name := d.Name.Text(c.file)
			en := &types.Enum{Name: name}
			sym := NewTypeName(name, en, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym

		case *ast.ProtocolDecl:
			name := d.Name.Text(c.file)
			pr := &types.Protocol{Name: name}
			sym := NewTypeName(name, pr, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym

			if d.Inherit != nil {
				for _, item := range d.Inherit.Items {
					inhType := c.resolveType(item.Type, scope)
					if inhProto, ok := inhType.(*types.Protocol); ok {
						pr.Inherited = append(pr.Inherited, inhProto)
					}
				}
			}

			if d.Body != nil {
				for _, mem := range d.Body.Members {
					switch m := mem.(type) {
					case *ast.FuncDecl:
						mName := m.Name.Text(c.file)
						sig := c.buildFuncSig(m.Sig, scope)
						pr.Requirements = append(pr.Requirements, &types.Requirement{
							Name: mName,
							Sig:  sig,
						})
					case *ast.VarDecl:
						for _, b := range m.Bindings {
							var propType types.Type
							if tp, ok := b.Pat.(*ast.TypedPattern); ok {
								propType = c.resolveType(tp.Type, scope)
								if idPat, ok := tp.Pat.(*ast.IdentPattern); ok {
									pr.Requirements = append(pr.Requirements, &types.Requirement{
										Name:    idPat.Name.Text(c.file),
										Type:    propType,
										IsVar:   m.Kind == token.VAR,
										IsConst: m.Kind == token.LET,
									})
								}
							}
						}
					}
				}
			}

		case *ast.TypealiasDecl:
			name := d.Name.Text(c.file)
			alias := types.NewNamed(name, "", nil)
			sym := NewTypeName(name, alias, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym
		}
	}
}

// resolveTypeMembers populates fields, enum cases, and superclasses for declared nominal types.
func (c *checker) resolveTypeMembers(stmts []ast.Stmt, scope *Scope) {
	for _, stmt := range stmts {
		declStmt, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		switch d := declStmt.D.(type) {
		case *ast.StructDecl:
			sym := scope.Lookup(d.Name.Text(c.file))
			if sym == nil {
				continue
			}
			st := sym.Type().(*types.Struct)
			typeScope := NewScope(scope, d.Pos(), d.End())
			c.info.Scopes[d] = typeScope

			if d.Inherit != nil {
				for _, item := range d.Inherit.Items {
					inhType := c.resolveType(item.Type, scope)
					if proto, ok := inhType.(*types.Protocol); ok {
						st.Conformances = append(st.Conformances, proto)
					}
				}
			}

			if d.Body != nil {
				for _, mem := range d.Body.Members {
					switch m := mem.(type) {
					case *ast.VarDecl:
						isConst := m.Kind == token.LET
						for _, b := range m.Bindings {
							var fieldType types.Type
							if tp, ok := b.Pat.(*ast.TypedPattern); ok {
								fieldType = c.resolveType(tp.Type, scope)
							}
							c.declarePattern(b.Pat, fieldType, isConst, typeScope)
							if idPat, ok := b.Pat.(*ast.IdentPattern); ok {
								st.Fields = append(st.Fields, &types.Field{
									Name:    idPat.Name.Text(c.file),
									Type:    fieldType,
									IsConst: isConst,
								})
							} else if tp, ok := b.Pat.(*ast.TypedPattern); ok {
								if idPat, ok := tp.Pat.(*ast.IdentPattern); ok {
									st.Fields = append(st.Fields, &types.Field{
										Name:    idPat.Name.Text(c.file),
										Type:    fieldType,
										IsConst: isConst,
									})
								}
							}
						}
					case *ast.FuncDecl:
						fName := m.Name.Text(c.file)
						sig := c.buildFuncSig(m.Sig, typeScope)
						st.Methods = append(st.Methods, &types.Method{
							Name: fName,
							Sig:  sig,
						})
						fSym := NewFunc(fName, sig, m.Name.Pos())
						fSym.SetDecl(m)
						typeScope.Insert(fSym)
						c.info.Defs[m.Name] = fSym
					}
				}
			}

		case *ast.ClassDecl:
			sym := scope.Lookup(d.Name.Text(c.file))
			if sym == nil {
				continue
			}
			cl := sym.Type().(*types.Class)
			typeScope := NewScope(scope, d.Pos(), d.End())
			c.info.Scopes[d] = typeScope

			if d.Inherit != nil {
				for i, item := range d.Inherit.Items {
					inhType := c.resolveType(item.Type, scope)
					if i == 0 {
						if supClass, ok := inhType.(*types.Class); ok {
							cl.Superclass = supClass
							continue
						}
					}
					if proto, ok := inhType.(*types.Protocol); ok {
						cl.Conformances = append(cl.Conformances, proto)
					}
				}
			}

			if d.Body != nil {
				for _, mem := range d.Body.Members {
					switch m := mem.(type) {
					case *ast.VarDecl:
						isConst := m.Kind == token.LET
						for _, b := range m.Bindings {
							var fieldType types.Type
							if tp, ok := b.Pat.(*ast.TypedPattern); ok {
								fieldType = c.resolveType(tp.Type, scope)
							}
							c.declarePattern(b.Pat, fieldType, isConst, typeScope)
							if idPat, ok := b.Pat.(*ast.IdentPattern); ok {
								cl.Fields = append(cl.Fields, &types.Field{
									Name:    idPat.Name.Text(c.file),
									Type:    fieldType,
									IsConst: isConst,
								})
							} else if tp, ok := b.Pat.(*ast.TypedPattern); ok {
								if idPat, ok := tp.Pat.(*ast.IdentPattern); ok {
									cl.Fields = append(cl.Fields, &types.Field{
										Name:    idPat.Name.Text(c.file),
										Type:    fieldType,
										IsConst: isConst,
									})
								}
							}
						}
					case *ast.FuncDecl:
						fName := m.Name.Text(c.file)
						sig := c.buildFuncSig(m.Sig, typeScope)
						cl.Methods = append(cl.Methods, &types.Method{
							Name: fName,
							Sig:  sig,
						})
						fSym := NewFunc(fName, sig, m.Name.Pos())
						fSym.SetDecl(m)
						typeScope.Insert(fSym)
						c.info.Defs[m.Name] = fSym
					}
				}
			}

		case *ast.ActorDecl:
			sym := scope.Lookup(d.Name.Text(c.file))
			if sym == nil {
				continue
			}
			cl := sym.Type().(*types.Class)
			typeScope := NewScope(scope, d.Pos(), d.End())
			c.info.Scopes[d] = typeScope

			if d.Inherit != nil {
				for _, item := range d.Inherit.Items {
					inhType := c.resolveType(item.Type, scope)
					if proto, ok := inhType.(*types.Protocol); ok {
						cl.Conformances = append(cl.Conformances, proto)
					}
				}
			}

			if d.Body != nil {
				for _, mem := range d.Body.Members {
					switch m := mem.(type) {
					case *ast.VarDecl:
						isConst := m.Kind == token.LET
						for _, b := range m.Bindings {
							var fieldType types.Type
							if tp, ok := b.Pat.(*ast.TypedPattern); ok {
								fieldType = c.resolveType(tp.Type, scope)
							}
							c.declarePattern(b.Pat, fieldType, isConst, typeScope)
							if idPat, ok := b.Pat.(*ast.IdentPattern); ok {
								cl.Fields = append(cl.Fields, &types.Field{
									Name:    idPat.Name.Text(c.file),
									Type:    fieldType,
									IsConst: isConst,
								})
							} else if tp, ok := b.Pat.(*ast.TypedPattern); ok {
								if idPat, ok := tp.Pat.(*ast.IdentPattern); ok {
									cl.Fields = append(cl.Fields, &types.Field{
										Name:    idPat.Name.Text(c.file),
										Type:    fieldType,
										IsConst: isConst,
									})
								}
							}
						}
					case *ast.FuncDecl:
						fName := m.Name.Text(c.file)
						sig := c.buildFuncSig(m.Sig, typeScope)
						cl.Methods = append(cl.Methods, &types.Method{
							Name: fName,
							Sig:  sig,
						})
						fSym := NewFunc(fName, sig, m.Name.Pos())
						fSym.SetDecl(m)
						typeScope.Insert(fSym)
						c.info.Defs[m.Name] = fSym
					}
				}
			}

		case *ast.EnumDecl:
			sym := scope.Lookup(d.Name.Text(c.file))
			if sym == nil {
				continue
			}
			en := sym.Type().(*types.Enum)
			typeScope := NewScope(scope, d.Pos(), d.End())
			c.info.Scopes[d] = typeScope

			if d.Inherit != nil {
				for _, item := range d.Inherit.Items {
					inhType := c.resolveType(item.Type, scope)
					if proto, ok := inhType.(*types.Protocol); ok {
						en.Conformances = append(en.Conformances, proto)
					}
				}
			}

			if d.Body != nil {
				for _, mem := range d.Body.Members {
					switch m := mem.(type) {
					case *ast.EnumCaseDecl:
						for _, el := range m.Elements {
							caseName := el.Name.Text(c.file)
							var assocType types.Type
							if len(el.Params) > 0 {
								assocType = c.resolveType(el.Params[0].Type, scope)
							}
							enCase := &types.EnumCase{
								Name:           caseName,
								AssociatedType: assocType,
							}
							en.Cases = append(en.Cases, enCase)
							caseSym := NewEnumCase(caseName, en, assocType, el.Name.Pos())
							typeScope.Insert(caseSym)
							c.info.Defs[el.Name] = caseSym
						}
					case *ast.FuncDecl:
						fName := m.Name.Text(c.file)
						sig := c.buildFuncSig(m.Sig, typeScope)
						en.Methods = append(en.Methods, &types.Method{
							Name: fName,
							Sig:  sig,
						})
						fSym := NewFunc(fName, sig, m.Name.Pos())
						fSym.SetDecl(m)
						typeScope.Insert(fSym)
						c.info.Defs[m.Name] = fSym
					}
				}
			}

		case *ast.TypealiasDecl:
			sym := scope.Lookup(d.Name.Text(c.file))
			if sym != nil && d.Type != nil {
				underlying := c.resolveType(d.Type, scope)
				if named, ok := sym.Type().(*types.Named); ok {
					named.SetUnderlying(underlying)
				}
			}
		}
	}
}

// declareFunctions discovers top-level functions and signatures.
func (c *checker) declareFunctions(stmts []ast.Stmt, scope *Scope) {
	for _, stmt := range stmts {
		declStmt, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		if f, ok := declStmt.D.(*ast.FuncDecl); ok {
			name := f.Name.Text(c.file)
			sig := c.buildFuncSig(f.Sig, scope)
			sym := NewFunc(name, sig, f.Name.Pos())
			sym.SetDecl(f)
			if old := scope.Insert(sym); old != nil {
				c.errorf(f.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[f.Name] = sym
		}
	}
}

func (c *checker) buildFuncSig(sig *ast.FuncSig, scope *Scope) *types.Signature {
	if sig == nil {
		return &types.Signature{Results: types.Typ[types.Void]}
	}
	var params []*types.Param
	if sig.Params != nil {
		params = make([]*types.Param, len(sig.Params))
		for i, p := range sig.Params {
			var name, label string
			if p.Label != nil {
				label = p.Label.Text(c.file)
			}
			if p.Name != nil {
				name = p.Name.Text(c.file)
			} else if label != "_" {
				name = label
			}
			ownership := types.DefaultOwnership
			for _, m := range p.Mods {
				if m.Kind == token.INOUT {
					ownership = types.InOut
				}
			}
			params[i] = &types.Param{
				Name:      name,
				Label:     label,
				Type:      c.resolveType(p.Type, scope),
				Ownership: ownership,
				Variadic:  p.Ellipsis != token.NoPos,
			}
		}
	}
	var res types.Type = types.Typ[types.Void]
	if sig.Result != nil {
		res = c.resolveType(sig.Result.Type, scope)
	}
	var throws types.Type
	if sig.Throws != nil {
		if sig.Throws.Type != nil {
			throws = c.resolveType(sig.Throws.Type, scope)
		} else {
			throws = types.Typ[types.Never]
		}
	}
	return &types.Signature{
		Params:  params,
		Results: res,
		Async:   sig.Async != token.NoPos,
		Throws:  throws,
	}
}

// declarePatternInit binds pattern variables into scope with an explicit initialized flag.
func (c *checker) declarePatternInit(pat ast.Pattern, typ types.Type, isConst bool, isInitialized bool, scope *Scope) {
	if pat == nil {
		return
	}
	switch p := pat.(type) {
	case *ast.IdentPattern:
		name := p.Name.Text(c.file)
		sym := NewVar(name, typ, p.Name.Pos(), isConst, types.DefaultOwnership)
		sym.SetInitialized(isInitialized)
		if prev := scope.Insert(sym); prev != nil {
			c.errorf(p.Name.Pos(), "invalid redeclaration of '%s'", name)
		}
		c.info.Defs[p.Name] = sym

	case *ast.ExprPattern:
		if idExpr, ok := p.X.(*ast.IdentExpr); ok {
			name := idExpr.Name.Text(c.file)
			sym := NewVar(name, typ, idExpr.Name.Pos(), isConst, types.DefaultOwnership)
			sym.SetInitialized(isInitialized)
			scope.Insert(sym)
			c.info.Defs[idExpr.Name] = sym
		}

	case *ast.TypedPattern:
		annotated := c.resolveType(p.Type, scope)
		c.declarePatternInit(p.Pat, annotated, isConst, isInitialized, scope)

	case *ast.TuplePattern:
		for _, elem := range p.Elems {
			c.declarePatternInit(elem.Pat, nil, isConst, isInitialized, scope)
		}

	case *ast.ValueBindingPattern:
		c.declarePatternInit(p.Pat, typ, p.Kind == token.LET, isInitialized, scope)
	}
}

// declarePattern binds pattern variables into scope (defaulting to initialized).
func (c *checker) declarePattern(pat ast.Pattern, typ types.Type, isConst bool, scope *Scope) {
	c.declarePatternInit(pat, typ, isConst, true, scope)
}

// resolveExtensions processes extension declarations and attaches conformances and members.
func (c *checker) resolveExtensions(stmts []ast.Stmt, scope *Scope) {
	for _, stmt := range stmts {
		declStmt, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		ext, ok := declStmt.D.(*ast.ExtensionDecl)
		if !ok {
			continue
		}

		extType := c.resolveType(ext.Type, scope)
		if extType == nil || extType == types.Typ[types.Invalid] {
			continue
		}

		nominalType := extType.Underlying()
		typeName := nominalType.String()
		if named, ok := extType.(*types.Named); ok {
			typeName = named.Name
		}

		var typeScope *Scope
		for node, sc := range c.info.Scopes {
			switch nd := node.(type) {
			case *ast.StructDecl:
				if nd.Name.Text(c.file) == typeName {
					typeScope = sc
				}
			case *ast.ClassDecl:
				if nd.Name.Text(c.file) == typeName {
					typeScope = sc
				}
			case *ast.ActorDecl:
				if nd.Name.Text(c.file) == typeName {
					typeScope = sc
				}
			case *ast.EnumDecl:
				if nd.Name.Text(c.file) == typeName {
					typeScope = sc
				}
			}
			if typeScope != nil {
				break
			}
		}
		if typeScope == nil {
			typeScope = NewScope(scope, ext.Pos(), ext.End())
		}
		c.info.Scopes[ext] = typeScope

		if ext.Inherit != nil {
			for _, item := range ext.Inherit.Items {
				protoType := c.resolveType(item.Type, scope)
				if proto, ok := protoType.(*types.Protocol); ok {
					switch nt := nominalType.(type) {
					case *types.Struct:
						nt.Conformances = append(nt.Conformances, proto)
					case *types.Class:
						nt.Conformances = append(nt.Conformances, proto)
					case *types.Enum:
						nt.Conformances = append(nt.Conformances, proto)
					}
				}
			}
		}

		if ext.Body != nil {
			for _, mem := range ext.Body.Members {
				switch m := mem.(type) {
				case *ast.FuncDecl:
					fName := m.Name.Text(c.file)
					sig := c.buildFuncSig(m.Sig, typeScope)
					method := &types.Method{Name: fName, Sig: sig}
					switch nt := nominalType.(type) {
					case *types.Struct:
						nt.Methods = append(nt.Methods, method)
					case *types.Class:
						nt.Methods = append(nt.Methods, method)
					case *types.Enum:
						nt.Methods = append(nt.Methods, method)
					}
					sym := NewFunc(fName, sig, m.Name.Pos())
					sym.SetDecl(m)
					typeScope.Insert(sym)
					c.info.Defs[m.Name] = sym

				case *ast.VarDecl:
					isConst := m.Kind == token.LET
					for _, b := range m.Bindings {
						var propType types.Type
						if tp, ok := b.Pat.(*ast.TypedPattern); ok {
							propType = c.resolveType(tp.Type, scope)
						}
						c.declarePattern(b.Pat, propType, isConst, typeScope)
						if idPat, ok := b.Pat.(*ast.IdentPattern); ok {
							field := &types.Field{
								Name:    idPat.Name.Text(c.file),
								Type:    propType,
								IsConst: isConst,
							}
							switch nt := nominalType.(type) {
							case *types.Struct:
								nt.Fields = append(nt.Fields, field)
							case *types.Class:
								nt.Fields = append(nt.Fields, field)
							}
						}
					}
				}
			}
		}
	}
}

// checkProtocolConformances validates that all types adopting protocols fulfill their requirements.
func (c *checker) checkProtocolConformances(scope *Scope) {
	for _, sym := range scope.elems {
		tn, ok := sym.(*TypeNameSymbol)
		if !ok {
			continue
		}
		var typeName = tn.Name()
		var conformances []*types.Protocol
		var fields []*types.Field
		var methods []*types.Method

		switch t := tn.Type().(type) {
		case *types.Struct:
			conformances = t.Conformances
			fields = t.Fields
			methods = t.Methods
		case *types.Class:
			conformances = t.Conformances
			fields = t.Fields
			methods = t.Methods
		case *types.Enum:
			conformances = t.Conformances
			methods = t.Methods
		}

		for _, proto := range conformances {
			c.checkConformance(tn.Pos(), typeName, proto, fields, methods)
		}
	}
}

func (c *checker) checkConformance(pos token.Pos, typeName string, proto *types.Protocol, fields []*types.Field, methods []*types.Method) {
	for _, req := range proto.Requirements {
		satisfied := false
		if req.Sig != nil {
			for _, m := range methods {
				if m.Name == req.Name && types.Identical(m.Sig, req.Sig) {
					satisfied = true
					break
				}
			}
		} else if req.Type != nil {
			for _, f := range fields {
				if f.Name == req.Name && types.AssignableTo(f.Type, req.Type) {
					satisfied = true
					break
				}
			}
		}
		if !satisfied {
			c.errorf(pos, "type '%s' does not conform to protocol '%s': missing '%s'", typeName, proto.Name, req.Name)
		}
	}
	for _, inh := range proto.Inherited {
		c.checkConformance(pos, typeName, inh, fields, methods)
	}
}
