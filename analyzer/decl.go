package analyzer

import (
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
		return c.resolveMemberType(t, scope)

	default:
		return types.Typ[types.Invalid]
	}
}

// resolveMemberType resolves a qualified type name: the `A.B` form.
//
// Two different things are written that way. `Swift.Int` is a name in
// a module, and a .swiftinterface writes every name that way -- which
// is why this is what reading one turns on. `Array<Int>.Index` is a
// type declared inside another type, and `Self.Element` is one a
// protocol's conformer supplies; both are members of a type rather
// than of a module.
//
// The qualifier decides which, and a module's name wins: a lookup for
// `Foundation.Data` goes to Foundation's own scope and nowhere else,
// so it cannot be answered with somebody else's Data. Everything else
// is a member of a type.
//
// What this used to do was resolve the qualifier as if it were a type
// -- reporting that there is no type called `Swift`, which is true
// and useless -- and then build a name out of what came back. Every
// `Swift.Int` in an interface became a type called `invalid.Int`,
// which resolved to nothing, matched nothing, and was carried into
// every signature that mentioned it without a word.
func (c *checker) resolveMemberType(t *ast.MemberType, scope *Scope) types.Type {
	name := t.Name.Text(c.file)
	if q, ok := t.X.(*ast.IdentType); ok && (q.Args == nil || len(q.Args.Args) == 0) {
		if qual := q.Name.Text(c.file); c.modules[qual] != nil {
			return c.moduleMember(t, qual, name, scope)
		}
	}
	// `C.Element` -- an associated type reached through a type
	// parameter, or through Self. Which type it is follows from what
	// C turns out to be, so what this yields is the dependency and
	// not an answer; substitution turns it into one. See
	// analyzer/associated.go.
	if base := c.resolveType(t.X, scope); base != nil {
		if tp, ok := base.(*types.TypeParam); ok {
			// A `where` clause may have said what it is, and then it
			// is that: `where C.Item == Int32` makes `C.Item` Int32
			// everywhere the clause is in force.
			if bound := tp.Bound[name]; bound != nil {
				return bound
			}
			if !promises(tp, name) && tp.Promised[name] == nil {
				c.errorf(t.Pos(), "cannot find type '%s' in scope: nothing '%s' "+
					"conforms to declares an associated type '%s'",
					c.spellingOf(t), tp.Name, name)
				return types.Typ[types.Invalid]
			}
			return &types.Dependent{Base: tp, Name: name}
		}
		// A concrete type on the left, and an associated type on the
		// right: `Ints.Item` is what Ints chose.
		if answer := types.AssocOf(base, name); answer != nil {
			return answer
		}
		// Or a type declared inside it: `String.Index` is a member of
		// String's own scope, which is where its declaration put it.
		if inner := c.nestedType(base, name, t, scope); inner != nil {
			return inner
		}
	}
	// A type inside a type. Nothing here declares one yet, so this
	// says so by name rather than inventing a type that stands for it.
	c.errorf(t.Pos(), "cannot find type '%s' in scope: a type declared inside "+
		"another type is not modelled yet", c.spellingOf(t))
	return types.Typ[types.Invalid]
}

// nestedType is a type declared inside another, or nil.
//
// A type's members live in a scope of its own, and a type declared
// among them was put there when the outer one was read -- so this is
// a lookup in that scope rather than anything the type itself
// carries. Generic arguments on the outer name are looked through:
// `Array<Int>.Index` is Array's Index, and what the argument decides
// is the nested type's own parameters rather than which type it is.
func (c *checker) nestedType(outer types.Type, name string, at *ast.MemberType, scope *Scope) types.Type {
	if inst, ok := outer.(*types.GenericInstance); ok {
		outer = inst.Base
	}
	inner := c.typeScopes[typeNameOf(outer)]
	if inner == nil {
		return nil
	}
	sym, _ := inner.LookupLocal(name).(*TypeNameSymbol)
	if sym == nil {
		return nil
	}
	if at.Args == nil || len(at.Args.Args) == 0 {
		return sym.Type()
	}
	args := make([]types.Type, len(at.Args.Args))
	for i, a := range at.Args.Args {
		args[i] = c.resolveType(a, scope)
	}
	return &types.GenericInstance{Base: sym.Type(), Args: args}
}

// promises reports whether one of a type parameter's constraints
// declares an associated type of this name.
func promises(tp *types.TypeParam, name string) bool {
	for _, con := range tp.Constraints {
		p, ok := con.(*types.Protocol)
		if !ok {
			continue
		}
		for _, up := range allProtocols([]*types.Protocol{p}) {
			for _, a := range up.Associated {
				if a != nil && a.Name == name {
					return true
				}
			}
		}
	}
	return false
}

// moduleMember is a type named through the module it came from.
func (c *checker) moduleMember(t *ast.MemberType, module, name string, scope *Scope) types.Type {
	var args []types.Type
	if t.Args != nil && len(t.Args.Args) > 0 {
		args = make([]types.Type, len(t.Args.Args))
		for i, arg := range t.Args.Args {
			args[i] = c.resolveType(arg, scope)
		}
	}
	// The three the language spells for itself, said the long way.
	// `Swift.Array<Int>` is `[Int]` and has to be that type rather
	// than a generic instance standing beside it -- which is the same
	// decision the unqualified spelling takes, a few lines above.
	if module == "Swift" {
		switch {
		case name == "Array" && len(args) == 1:
			return &types.Array{Elem: args[0]}
		case name == "Optional" && len(args) == 1:
			return &types.Optional{Wrapped: args[0]}
		case name == "Dictionary" && len(args) == 2:
			return &types.Dictionary{Key: args[0], Value: args[1]}
		}
	}
	var base types.Type
	if sym, ok := c.modules[module].Lookup(name).(*TypeNameSymbol); ok {
		base = sym.Type()
	}
	// Swift's own scope holds what core declares; the rest of what
	// Swift names is the universe, which an unqualified name reaches
	// the same way. `Swift.Array` and `Array` are one type, and
	// answering only one of them would be the more surprising of the
	// two behaviours.
	if base == nil && module == "Swift" {
		base = types.LookupSwiftUniverse(name)
	}
	if base == nil {
		c.errorf(t.Pos(), "cannot find type '%s' in scope: no such type in %s",
			c.spellingOf(t), module)
		return types.Typ[types.Invalid]
	}
	if len(args) == 0 {
		return base
	}
	return &types.GenericInstance{Base: base, Args: args}
}

// spellingOf is a qualified type as it was written, which is what a
// diagnostic about it should say. Reporting the pieces separately
// reports a mistake nobody made.
func (c *checker) spellingOf(t ast.Type) string {
	switch n := t.(type) {
	case *ast.IdentType:
		return n.Name.Text(c.file)
	case *ast.MemberType:
		return c.spellingOf(n.X) + "." + n.Name.Text(c.file)
	}
	return string(c.file.Slice(t.Pos(), t.End()))
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
			c.declaredHere(sym, c.accessOf(d.Mods))

		case *ast.ClassDecl:
			name := d.Name.Text(c.file)
			cl := &types.Class{Name: name}
			sym := NewTypeName(name, cl, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym
			c.declaredHere(sym, c.accessOf(d.Mods))

		case *ast.ActorDecl:
			name := d.Name.Text(c.file)
			cl := &types.Class{Name: name, IsActor: true}
			sym := NewTypeName(name, cl, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym
			c.declaredHere(sym, c.accessOf(d.Mods))

		case *ast.EnumDecl:
			name := d.Name.Text(c.file)
			en := &types.Enum{Name: name}
			sym := NewTypeName(name, en, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym
			c.declaredHere(sym, c.accessOf(d.Mods))

		case *ast.ProtocolDecl:
			c.declareProtocol(d, scope)

		case *ast.TypealiasDecl:
			name := d.Name.Text(c.file)
			alias := types.NewNamed(name, "", nil)
			sym := NewTypeName(name, alias, d.Name.Pos())
			sym.SetDecl(d)
			if old := scope.Insert(sym); old != nil {
				c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
			}
			c.info.Defs[d.Name] = sym
			c.declaredHere(sym, c.accessOf(d.Mods))
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

// isComputed reports whether a binding is a computed property rather
// than stored storage.
//
// The test is what the accessors are, not that there are any. `var x:
// Int { 1 }` and `var x: Int { get }` are computed; `var x: Int = 0 {
// didSet { … } }` is stored with an observer on it, and an observer
// does not take the storage away.
func (c *checker) isComputed(b *ast.PatternBinding) bool {
	if b == nil {
		return false
	}
	if b.Body != nil {
		return true
	}
	if b.Accessors == nil {
		return false
	}
	for _, a := range b.Accessors.Accessors {
		if a == nil || a.Keyword == nil {
			continue
		}
		switch a.Keyword.Text(c.file) {
		case "get", "set", "_read", "_modify", "unsafeAddress", "unsafeMutableAddress":
			return true
		}
	}
	return false
}

// isStatic reports whether a declaration belongs to the type rather
// than to an instance of it.
func isStatic(mods []*ast.Modifier) bool {
	for _, m := range mods {
		if m == nil {
			continue
		}
		if m.Kind == token.STATIC || m.Kind == token.CLASS {
			return true
		}
	}
	return false
}

// resolveTypeMembers populates fields, enum cases, and superclasses for declared nominal types.
// declareNested declares the types written inside a type's body and
// reads their members, in the enclosing type's scope. A nested type
// is a type: it is reached as `Outer.Inner` from outside and by its
// own name from within, and its members are read the same way any
// other type's are.
func (c *checker) declareNested(body *ast.MemberBlock, typeScope *Scope, outer types.Type) {
	nested := memberDecls(body)
	if len(nested) == 0 {
		return
	}
	c.declareTypes(nested, typeScope)
	// Each one remembers what it is inside, which is what its symbol
	// has to say: `String.Index` is spelled String and then Index.
	for _, d := range nested {
		var name *ast.Ident
		switch d := d.(type) {
		case *ast.StructDecl:
			name = d.Name
		case *ast.ClassDecl:
			name = d.Name
		case *ast.ActorDecl:
			name = d.Name
		case *ast.EnumDecl:
			name = d.Name
		default:
			continue
		}
		if name == nil {
			continue
		}
		sym, _ := typeScope.LookupLocal(name.Text(c.file)).(*TypeNameSymbol)
		if sym == nil {
			continue
		}
		setEnclosing(sym.Type(), outer)
	}
	c.resolveTypeMembers(nested, typeScope)
}

// setEnclosing records the type a nested one is declared inside.
func setEnclosing(inner, outer types.Type) {
	if inner == nil || outer == nil {
		return
	}
	switch n := inner.(type) {
	case *types.Struct:
		n.In = outer
	case *types.Class:
		n.In = outer
	case *types.Enum:
		n.In = outer
	}
}

func (c *checker) resolveTypeMembers(decls []ast.Decl, scope *Scope) {
	for _, d := range decls {
		switch d := d.(type) {
		case *ast.ProtocolDecl:
			c.resolveProtocol(d, scope)

		case *ast.StructDecl:
			if t, inner, params, ok := c.openType(d, d.Name, d.Generics, d.Body, scope); ok {
				n := t.(*types.Struct)
				n.TypeParams = params
				n.Conformances = c.protocolsOf(d.Inherit, scope, nil)
				c.readMembers(d.Body, inner, &n.Fields, &n.Methods, nil, &n.Inits, &n.Computed, &n.Statics)
			}

		case *ast.ClassDecl:
			if t, inner, params, ok := c.openType(d, d.Name, d.Generics, d.Body, scope); ok {
				n := t.(*types.Class)
				n.TypeParams = params
				n.Conformances = c.protocolsOf(d.Inherit, scope, &n.Superclass)
				c.readMembers(d.Body, inner, &n.Fields, &n.Methods, nil, &n.Inits, &n.Computed, &n.Statics)
			}

		case *ast.ActorDecl:
			if t, inner, params, ok := c.openType(d, d.Name, d.Generics, d.Body, scope); ok {
				n := t.(*types.Class)
				n.TypeParams = params
				n.Conformances = c.protocolsOf(d.Inherit, scope, nil)
				c.readMembers(d.Body, inner, &n.Fields, &n.Methods, nil, &n.Inits, &n.Computed, &n.Statics)
			}

		case *ast.EnumDecl:
			if t, inner, params, ok := c.openType(d, d.Name, d.Generics, d.Body, scope); ok {
				n := t.(*types.Enum)
				n.TypeParams = params
				n.Conformances = c.protocolsOf(d.Inherit, scope, nil)
				c.readMembers(d.Body, inner, nil, &n.Methods, n, nil, &n.Computed, &n.Statics)
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
	c.declareNested(body, typeScope, sym.Type())
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
func (c *checker) readMembers(body *ast.MemberBlock, typeScope *Scope, fields *[]*types.Field, methods *[]*types.Method, en *types.Enum, inits *[]*types.Signature, computed, statics *[]*types.Field) {
	if body == nil || typeScope == nil {
		return
	}
	for _, mem := range body.Members {
		switch m := mem.(type) {
		case *ast.VarDecl:
			if fields == nil || computed == nil || statics == nil {
				continue
			}
			// Only what the instance actually holds. A computed
			// property is a getter and a setter with nothing behind
			// them, and a static one is the type's storage rather
			// than an instance's -- counting either as a field
			// changes the type's layout, which is a silent wrong
			// answer for everything that reads a field after it and
			// for every call that passes one.
			isConst := m.Kind == token.LET
			static := isStatic(m.Mods)
			for _, b := range m.Bindings {
				got := c.storedField(b, isConst, typeScope)
				switch {
				case static:
					*statics = append(*statics, got...)
				case c.isComputed(b):
					*computed = append(*computed, got...)
				default:
					*fields = append(*fields, got...)
				}
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
			*methods = append(*methods, &types.Method{
				Name: name, Sig: sig, IsStatic: isStatic(m.Mods),
			})
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
			// A receiver method is a member of the type its clause
			// names, not a name at this level. resolveReceivers
			// declares it where it belongs.
			if f.Recv != nil {
				continue
			}
			name := f.Name.Text(c.file)
			sig := c.buildGenericFuncSig(f, scope)
			sym := NewFunc(name, sig, f.Name.Pos())
			sym.SetDecl(f)
			sym.SetAccess(c.accessOf(f.Mods))
			c.declaredHere(sym, sym.Access())
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
	// The `where` clause before the signature is read, because it may
	// say what a parameter's associated type is and the signature may
	// then name it: `-> C.Item` under `where C.Item == Int32` is a
	// function returning Int32.
	c.applyWhere(f.Where, genScope)
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
			pt := c.resolveType(p.Type, scope)
			// A default is an expression and is checked like one, in
			// the parameter's own type. Nothing did, so nothing knew
			// what `= 3` was -- which matters at every call that
			// leaves the parameter out, because that is where Swift
			// evaluates it.
			if p.Default != nil {
				c.checkExpr(p.Default, pt, scope)
			}
			params[i] = &types.Param{
				Name:       name,
				Label:      label,
				Type:       pt,
				Ownership:  ownership,
				Variadic:   p.Ellipsis != token.NoPos,
				HasDefault: p.Default != nil,
			}
			if p.Default != nil {
				c.info.Defaults[params[i]] = p.Default
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

		fields, methods, conformances, inits, computed, statics := sinksOf(extType.Underlying())
		if conformances != nil {
			*conformances = append(*conformances, c.protocolsOf(ext.Inherit, scope, nil)...)
		}
		en, _ := extType.Underlying().(*types.Enum)
		c.readMembers(ext.Body, typeScope, fields, methods, en, inits, computed, statics)
	}
}

// resolveReceivers attaches every receiver method to the type its
// clause names.
//
// A receiver method is an extension member written the other way
// round, so this does for one function what resolveExtensions does
// for a block of them: resolve the type, put the method in the type's
// own scope, and add it to the type's method list so a call through a
// value of that type finds it.
func (c *checker) resolveReceivers(decls []ast.Decl, scope *Scope) {
	for _, d := range decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil {
			continue
		}
		recv := c.resolveType(fn.Recv.Type, scope)
		if recv == nil || isInvalid(recv) {
			continue
		}
		// A class receiver is a reference: a method that changes a
		// property changes the object, and the receiver itself needs
		// no mutability. Swift has no `mutating` on a class method,
		// and `inout` here would have to mean rebinding the
		// reference, which no method can do. Refused by name rather
		// than quietly read as borrowing.
		if _, isClass := recv.Underlying().(*types.Class); isClass &&
			c.ownershipOf(fn.Recv.Mods) == types.InOut {
			c.errorf(fn.Recv.Pos(), "an 'inout' receiver is not allowed on a class: "+
				"'%s' is a reference, and a method that changes a property needs no mutable receiver", recv)
			continue
		}
		c.info.Receivers[fn] = recv

		typeScope := c.typeScopes[typeNameOf(recv)]
		if typeScope == nil {
			typeScope = NewScope(scope, fn.Pos(), fn.End())
		}
		_, methods, _, _, _, _ := sinksOf(recv.Underlying())
		if methods == nil {
			c.errorf(fn.Recv.Pos(), "cannot add a method to '%s'", recv)
			continue
		}
		name := fn.Name.Text(c.file)
		sig := c.buildFuncSig(fn.Sig, typeScope)
		*methods = append(*methods, &types.Method{
			Name: name, Sig: sig, IsStatic: isStatic(fn.Mods),
		})
		sym := NewFunc(name, sig, fn.Name.Pos())
		sym.SetDecl(fn)
		sym.SetAccess(c.accessOf(fn.Mods))
		typeScope.Insert(sym)
		c.info.Defs[fn.Name] = sym
	}
}

// typeNameOf is the name a type is declared under, which is also the
// key its own scope is remembered by.
func typeNameOf(t types.Type) string {
	if t == nil {
		return ""
	}
	if named, ok := t.(*types.Named); ok {
		return named.Name
	}
	return t.Underlying().String()
}

// sinksOf is where a type's members are recorded. A struct, a class,
// an enum and an actor keep the same three lists, which is what lets
// an extension add to any of them without knowing which it has. A nil
// sink is a member kind the type cannot hold.
func sinksOf(t types.Type) (fields *[]*types.Field, methods *[]*types.Method, conformances *[]*types.Protocol, inits *[]*types.Signature, computed, statics *[]*types.Field) {
	switch n := t.(type) {
	case *types.Struct:
		return &n.Fields, &n.Methods, &n.Conformances, &n.Inits, &n.Computed, &n.Statics
	case *types.Class:
		return &n.Fields, &n.Methods, &n.Conformances, &n.Inits, &n.Computed, &n.Statics
	case *types.Enum:
		return nil, &n.Methods, &n.Conformances, nil, &n.Computed, &n.Statics
	}
	return nil, nil, nil, nil, nil, nil
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
			c.checkConformance(tn.Pos(), tn.Type(), typeName, proto, fields, methods)
		}
	}
}

func (c *checker) checkConformance(pos token.Pos, conformer types.Type, typeName string, proto *types.Protocol, fields []*types.Field, methods []*types.Method) {
	// What the conformer promised has to be read with what it chose
	// put in: a requirement saying `-> Self.Item` is a requirement to
	// return Int32 for a type that made Item Int32, and comparing the
	// two without that substitution compares a signature against a
	// dependency and finds them different every time.
	// Self stands for the conforming type, and every `Self.Item` in
	// the requirements follows from that: substituting the one
	// answers the others, because what Item is was recorded on the
	// type when it conformed. See types.AssocOf.
	var subst map[*types.TypeParam]types.Type
	if proto.Self != nil && conformer != nil {
		subst = map[*types.TypeParam]types.Type{proto.Self: conformer}
	}
	for _, req := range proto.Requirements {
		satisfied := false
		if req.Sig != nil {
			want, _ := types.Substitute(req.Sig, subst).(*types.Signature)
			if want == nil {
				want = req.Sig
			}
			for _, m := range methods {
				if m.Name == req.Name && types.Identical(m.Sig, want) {
					satisfied = true
					break
				}
			}
		} else if req.Type != nil {
			want := types.Substitute(req.Type, subst)
			for _, f := range fields {
				if f.Name == req.Name && types.AssignableTo(f.Type, want) {
					satisfied = true
					break
				}
			}
		}
		if !satisfied {
			c.errorf(pos, "type '%s' does not conform to protocol '%s': missing '%s'", typeName, proto.Name, req.Name)
		}
	}
	// An associated type nobody chose. The typealias was not written
	// and no implementation implied one, so the protocol's other
	// requirements are about a type that does not exist -- which is a
	// conformance that does not hold, said where it can still be
	// fixed rather than where it is next used.
	for _, a := range proto.Associated {
		if a == nil {
			continue
		}
		choice := types.AssocOf(conformer, a.Name)
		if choice == nil {
			c.errorf(pos, "type '%s' does not conform to protocol '%s': nothing says "+
				"what '%s' is", typeName, proto.Name, a.Name)
			continue
		}
		// And what was chosen has to be what the protocol asked for.
		// `associatedtype Item: Number` is a promise the conformer
		// makes about its choice, and the requirements that use Item
		// were written believing it.
		for _, need := range a.Constraints {
			if !types.ConformsTo(choice, need) {
				c.errorf(pos, "type '%s' does not conform to protocol '%s': '%s' is "+
					"'%s', which does not conform to '%s'",
					typeName, proto.Name, a.Name, choice, need.Name)
			}
		}
	}
	for _, inh := range proto.Inherited {
		c.checkConformance(pos, conformer, typeName, inh, fields, methods)
	}
}
