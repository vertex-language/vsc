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
			ownership := c.ownershipOf(p.Mods)
			params[i] = &types.Param{
				Name:       name,
				Label:      label,
				Type:       c.resolveType(p.Type, scope),
				Ownership:  ownership,
				Variadic:   p.Ellipsis != token.NoPos,
				HasDefault: p.Default != nil,
			}
		}
		throws, thrown := c.throwsOf(t.Throws, scope)
		return &types.Signature{
			Params:  params,
			Results: c.resolveType(t.Result, scope),
			Async:   t.Async != token.NoPos,
			Throws:  throws,
			Thrown:  thrown,
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
// declsOf is the declarations a statement list holds. A declaration
// reaches the parser as a statement wherever a statement may go, and
// the passes below read declarations, not statements.
func declsOf(stmts []ast.Stmt) []ast.Decl {
	out := make([]ast.Decl, 0, len(stmts))
	for _, stmt := range stmts {
		if d, ok := stmt.(*ast.DeclStmt); ok {
			out = append(out, d.D)
		}
	}
	return out
}

// memberDecls is the declarations a type's body holds.
func memberDecls(body *ast.MemberBlock) []ast.Decl {
	if body == nil {
		return nil
	}
	out := make([]ast.Decl, 0, len(body.Members))
	for _, mem := range body.Members {
		if d, ok := mem.(ast.Decl); ok {
			out = append(out, d)
		}
	}
	return out
}

func (c *checker) declarePrecedenceAndOperators(decls []ast.Decl) {
	for _, d := range decls {
		switch d := d.(type) {
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
func (c *checker) declareTypes(decls []ast.Decl, scope *Scope) {
	for _, d := range decls {
		switch d := d.(type) {
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

// declareGenericParams binds a declaration's generic parameters in
// its own scope, so that the members written in terms of them —
// `struct Wrapper<T> { var value: T }` — resolve. The names are bound
// before the constraints are read, because a constraint may name
// another parameter of the same list.
func (c *checker) declareGenericParams(g *ast.GenericParams, scope *Scope) []*types.TypeParam {
	if g == nil {
		return nil
	}
	out := make([]*types.TypeParam, 0, len(g.Params))
	for _, p := range g.Params {
		if p.Name == nil {
			continue
		}
		name := p.Name.Text(c.file)
		tp := &types.TypeParam{Name: name}
		scope.Insert(NewTypeName(name, tp, p.Name.Pos()))
		c.info.Defs[p.Name] = NewTypeName(name, tp, p.Name.Pos())
		out = append(out, tp)
	}
	for i, p := range g.Params {
		if p.Inherit == nil || i >= len(out) {
			continue
		}
		for _, item := range p.Inherit.Items {
			if t := c.resolveType(item.Type, scope); t != nil {
				out[i].Constraints = append(out[i].Constraints, t)
			}
		}
	}
	return out
}

// associatedType is the type of an enum case's associated values:
// the one it has, or a tuple of them, which is the shape a pattern
// destructures and the shape the case's initializer takes.
func (c *checker) associatedType(params []*ast.Param, scope *Scope) types.Type {
	switch len(params) {
	case 0:
		return nil
	case 1:
		return c.resolveType(params[0].Type, scope)
	}
	t := &types.Tuple{}
	for _, p := range params {
		elem := &types.TupleElement{Type: c.resolveType(p.Type, scope)}
		if p.Label != nil {
			elem.Name = p.Label.Text(c.file)
		}
		t.Elements = append(t.Elements, elem)
	}
	return t
}

// storedField reads one binding of a type's `let` or `var` member and
// returns the fields it declares, binding their names in the type's
// scope on the way. A binding with no annotation takes the type of
// its initializer, which is how `var n = 0` is a field of type Int.
func (c *checker) storedField(b *ast.PatternBinding, isConst bool, typeScope *Scope) []*types.Field {
	pat := b.Pat
	var fieldType types.Type
	if tp, ok := pat.(*ast.TypedPattern); ok {
		// The annotation is read in the type's own scope: a member
		// may be written in terms of the type's generic parameters.
		fieldType = c.resolveType(tp.Type, typeScope)
		pat = tp.Pat
	} else if b.Value != nil {
		fieldType = c.checkExpr(b.Value, nil, typeScope)
	}
	c.declarePattern(b.Pat, fieldType, isConst, typeScope)

	idPat, ok := pat.(*ast.IdentPattern)
	if !ok {
		return nil
	}
	return []*types.Field{{
		Name:       idPat.Name.Text(c.file),
		Type:       fieldType,
		IsConst:    isConst,
		HasDefault: b.Value != nil,
	}}
}

// resolveTypeMembers populates fields, enum cases, and superclasses for declared nominal types.
// declareNested declares the types written inside a type's body and
// reads their members, in the enclosing type's scope. A nested type
// is a type: it is reached as `Outer.Inner` from outside and by its
// own name from within, and its members are read the same way any
// other type's are.
func (c *checker) declareNested(body *ast.MemberBlock, typeScope *Scope) {
	nested := memberDecls(body)
	if len(nested) == 0 {
		return
	}
	c.declareTypes(nested, typeScope)
	c.resolveTypeMembers(nested, typeScope)
}

func (c *checker) resolveTypeMembers(decls []ast.Decl, scope *Scope) {
	for _, d := range decls {
		switch d := d.(type) {
		case *ast.StructDecl:
			if t, inner, params, ok := c.openType(d, d.Name, d.Generics, d.Body, scope); ok {
				n := t.(*types.Struct)
				n.TypeParams = params
				n.Conformances = c.protocolsOf(d.Inherit, scope, nil)
				c.readMembers(d.Body, inner, &n.Fields, &n.Methods, nil, &n.Inits)
			}

		case *ast.ClassDecl:
			if t, inner, params, ok := c.openType(d, d.Name, d.Generics, d.Body, scope); ok {
				n := t.(*types.Class)
				n.TypeParams = params
				n.Conformances = c.protocolsOf(d.Inherit, scope, &n.Superclass)
				c.readMembers(d.Body, inner, &n.Fields, &n.Methods, nil, &n.Inits)
			}

		case *ast.ActorDecl:
			if t, inner, params, ok := c.openType(d, d.Name, d.Generics, d.Body, scope); ok {
				n := t.(*types.Class)
				n.TypeParams = params
				n.Conformances = c.protocolsOf(d.Inherit, scope, nil)
				c.readMembers(d.Body, inner, &n.Fields, &n.Methods, nil, &n.Inits)
			}

		case *ast.EnumDecl:
			if t, inner, params, ok := c.openType(d, d.Name, d.Generics, d.Body, scope); ok {
				n := t.(*types.Enum)
				n.TypeParams = params
				n.Conformances = c.protocolsOf(d.Inherit, scope, nil)
				c.readMembers(d.Body, inner, nil, &n.Methods, n, nil)
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

// openType finds the type a declaration declares and opens the scope
// its members are written in, with its generic parameters and its
// nested types already in it. It reports false where the declaration
// has no symbol, which happens only after an earlier error.
func (c *checker) openType(d ast.Decl, name *ast.Ident, generics *ast.GenericParams, body *ast.MemberBlock, scope *Scope) (types.Type, *Scope, []*types.TypeParam, bool) {
	if name == nil {
		return nil, nil, nil, false
	}
	sym := scope.Lookup(name.Text(c.file))
	if sym == nil {
		return nil, nil, nil, false
	}
	typeScope := NewScope(scope, d.Pos(), d.End())
	c.info.Scopes[d] = typeScope
	// Remembered by name, which is how an extension elsewhere in the
	// program finds the scope its members belong in. Two types of the
	// same name in different scopes are one entry, which is as much
	// as an extension can say about which it means today.
	if c.typeScopes == nil {
		c.typeScopes = map[string]*Scope{}
	}
	c.typeScopes[name.Text(c.file)] = typeScope
	params := c.declareGenericParams(generics, typeScope)
	c.declareNested(body, typeScope)
	return sym.Type(), typeScope, params, true
}

// protocolsOf reads an inheritance clause: the protocols a type
// conforms to, and — where super is non-nil, which is to say for a
// class — the superclass, which Swift writes first in the same list
// and tells apart by what the name turns out to denote.
func (c *checker) protocolsOf(inherit *ast.InheritanceClause, scope *Scope, super *types.Type) []*types.Protocol {
	if inherit == nil {
		return nil
	}
	var out []*types.Protocol
	for i, item := range inherit.Items {
		t := c.resolveType(item.Type, scope)
		if i == 0 && super != nil {
			if cl, ok := t.(*types.Class); ok {
				*super = cl
				continue
			}
		}
		if proto, ok := t.(*types.Protocol); ok {
			out = append(out, proto)
		}
	}
	return out
}

// readMembers reads what a type's body declares into the type: its
// stored properties, its methods, and — for an enum, which is the
// only kind that has them — its cases. A nil sink is a member kind
// this type cannot have.
func (c *checker) readMembers(body *ast.MemberBlock, typeScope *Scope, fields *[]*types.Field, methods *[]*types.Method, en *types.Enum, inits *[]*types.Signature) {
	if body == nil || typeScope == nil {
		return
	}
	for _, mem := range body.Members {
		switch m := mem.(type) {
		case *ast.VarDecl:
			if fields == nil {
				continue
			}
			isConst := m.Kind == token.LET
			for _, b := range m.Bindings {
				*fields = append(*fields, c.storedField(b, isConst, typeScope)...)
			}

		case *ast.EnumCaseDecl:
			if en == nil {
				continue
			}
			for _, el := range m.Elements {
				name := el.Name.Text(c.file)
				assoc := c.associatedType(el.Params, typeScope)
				en.Cases = append(en.Cases, &types.EnumCase{Name: name, AssociatedType: assoc})
				sym := NewEnumCase(name, en, assoc, el.Name.Pos())
				typeScope.Insert(sym)
				c.info.Defs[el.Name] = sym
			}

		case *ast.InitDecl:
			// Recorded because its presence is what decides whether
			// the type gets a memberwise initializer: a type that
			// says how it is made does not also get the free answer.
			if inits == nil {
				continue
			}
			sig := c.buildFuncSig(m.Sig, typeScope)
			*inits = append(*inits, sig)

		case *ast.FuncDecl:
			if methods == nil {
				continue
			}
			name := m.Name.Text(c.file)
			sig := c.buildFuncSig(m.Sig, typeScope)
			*methods = append(*methods, &types.Method{Name: name, Sig: sig})
			sym := NewFunc(name, sig, m.Name.Pos())
			sym.SetDecl(m)
			sym.SetAccess(c.accessOf(m.Mods))
			typeScope.Insert(sym)
			c.info.Defs[m.Name] = sym
		}
	}
}

// declareFunctions discovers top-level functions and signatures.
func (c *checker) declareFunctions(decls []ast.Decl, scope *Scope) {
	for _, d := range decls {
		if f, ok := d.(*ast.FuncDecl); ok {
			name := f.Name.Text(c.file)
			sig := c.buildGenericFuncSig(f, scope)
			sym := NewFunc(name, sig, f.Name.Pos())
			sym.SetDecl(f)
			sym.SetAccess(c.accessOf(f.Mods))
			// Two functions may share a base name as long as their
			// full names differ -- the labels are part of it, so
			// `label(a:)` and `label(b:)` are two declarations even
			// though they are one type. Only the second of a pair
			// that agrees on both is a redeclaration.
			if old := scope.Insert(sym); old != nil {
				prev, ok := old.(*FuncSymbol)
				switch {
				case !ok || types.SameDecl(prev.Signature(), sig):
					c.errorf(f.Name.Pos(), "invalid redeclaration of '%s'", name)
				default:
					prev.AddOverload(sym)
				}
			}
			c.info.Defs[f.Name] = sym
		}
	}
}

// ownershipOf reads a parameter's modifiers. What a callee does with
// what it is given is the ownership model's half that crosses a call,
// and it is written here: `borrowing` reads it for the call,
// `consuming` takes it, `inout` writes through it. The underscored
// spellings are the older names for the first two, and every module
// interface in an SDK is written with them.
func (c *checker) ownershipOf(mods []*ast.Modifier) types.OwnershipKind {
	for _, m := range mods {
		if m.Kind == token.INOUT {
			return types.InOut
		}
		if m.Name == nil {
			continue
		}
		switch m.Name.Text(c.file) {
		case "consuming", "__owned":
			return types.Consuming
		case "borrowing", "__shared":
			return types.Borrowing
		}
	}
	return types.DefaultOwnership
}

// throwsOf reads a ThrowsClause. Swift says two things here and this
// keeps them apart: whether the function throws, and what it throws.
// `throws(Never)` says it does not throw, which is the spelling a
// generic signature reaches when its error type is substituted away.
func (c *checker) throwsOf(clause *ast.ThrowsClause, scope *Scope) (throws bool, thrown types.Type) {
	if clause == nil {
		return false, nil
	}
	if clause.Type == nil {
		return true, nil
	}
	thrown = c.resolveType(clause.Type, scope)
	if types.Identical(thrown, types.Typ[types.Never]) {
		return false, nil
	}
	return true, thrown
}

// buildGenericFuncSig reads a function's signature in a scope of its
// own, so that the generic parameters it declares are in scope for
// the types it is written with. The scope is recorded on the
// declaration: the body is checked inside it, where the parameters
// mean the same thing.
func (c *checker) buildGenericFuncSig(f *ast.FuncDecl, scope *Scope) *types.Signature {
	if f.Generics == nil {
		return c.buildFuncSig(f.Sig, scope)
	}
	genScope := c.info.Scopes[f]
	if genScope == nil {
		genScope = NewScope(scope, f.Pos(), f.End())
		c.info.Scopes[f] = genScope
	}
	tps := c.declareGenericParams(f.Generics, genScope)
	sig := c.buildFuncSig(f.Sig, genScope)
	sig.TypeParams = tps
	return sig
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
			ownership := c.ownershipOf(p.Mods)
			params[i] = &types.Param{
				Name:       name,
				Label:      label,
				Type:       c.resolveType(p.Type, scope),
				Ownership:  ownership,
				Variadic:   p.Ellipsis != token.NoPos,
				HasDefault: p.Default != nil,
			}
		}
	}
	var res types.Type = types.Typ[types.Void]
	if sig.Result != nil {
		res = c.resolveType(sig.Result.Type, scope)
	}
	throws, thrown := c.throwsOf(sig.Throws, scope)
	return &types.Signature{
		Params:  params,
		Results: res,
		Async:   sig.Async != token.NoPos,
		Throws:  throws,
		Thrown:  thrown,
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
			// The name is written as an expression — `for x in xs`
			// binds x through one — so it carries a type like any
			// other, and a consumer reading the tree finds it there.
			if typ != nil {
				c.info.Types[idExpr] = typ
			}
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
func (c *checker) resolveExtensions(decls []ast.Decl, scope *Scope) {
	for _, d := range decls {
		ext, ok := d.(*ast.ExtensionDecl)
		if !ok {
			continue
		}
		extType := c.resolveType(ext.Type, scope)
		if extType == nil || isInvalid(extType) {
			continue
		}
		c.info.Extensions[ext] = extType

		// An extension's members are written in the extended type's
		// own scope, so that they see what a member written inside
		// the declaration would see.
		typeScope := c.typeScopes[typeNameOf(extType)]
		if typeScope == nil {
			typeScope = NewScope(scope, ext.Pos(), ext.End())
		}
		c.info.Scopes[ext] = typeScope

		fields, methods, conformances, inits := sinksOf(extType.Underlying())
		if conformances != nil {
			*conformances = append(*conformances, c.protocolsOf(ext.Inherit, scope, nil)...)
		}
		en, _ := extType.Underlying().(*types.Enum)
		c.readMembers(ext.Body, typeScope, fields, methods, en, inits)
	}
}

// typeNameOf is the name a type is declared under.
func typeNameOf(t types.Type) string {
	if named, ok := t.(*types.Named); ok {
		return named.Name
	}
	return t.Underlying().String()
}

// sinksOf is where a type's members are recorded. A struct, a class,
// an enum and an actor keep the same three lists, which is what lets
// an extension add to any of them without knowing which it has. A nil
// sink is a member kind the type cannot hold.
func sinksOf(t types.Type) (fields *[]*types.Field, methods *[]*types.Method, conformances *[]*types.Protocol, inits *[]*types.Signature) {
	switch n := t.(type) {
	case *types.Struct:
		return &n.Fields, &n.Methods, &n.Conformances, &n.Inits
	case *types.Class:
		return &n.Fields, &n.Methods, &n.Conformances, &n.Inits
	case *types.Enum:
		return nil, &n.Methods, &n.Conformances, nil
	}
	return nil, nil, nil, nil
}

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
