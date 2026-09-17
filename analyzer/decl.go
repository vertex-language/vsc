package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/derive"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// resolveType converts an AST type node into a semantic types.Type, caching results.
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

// pointerType returns the pointer type corresponding to name and elem, if applicable.
func pointerType(name string, elem types.Type) (types.Type, bool) {
	switch name {
	case "UnsafePointer":
		return &types.Pointer{Elem: elem}, true
	case "UnsafeMutablePointer":
		return &types.Pointer{Elem: elem, Mutable: true}, true
	}
	return nil, false
}

func (c *checker) resolveTypeUncached(astType ast.Type, scope *Scope) types.Type {
	switch t := astType.(type) {
	// `Self` is the protocol's stand-in for its conformer inside a
	// protocol, and the type itself inside a type's own declaration.
	case *ast.SelfType:
		if tn := scope.LookupType("Self"); tn != nil && tn.Type() != nil {
			return tn.Type()
		}
		if c.currType != nil {
			return c.currType
		}
		c.errorf(t.Pos(), "cannot find type 'Self' in scope")
		return types.Typ[types.Invalid]
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
			if name == "Set" && len(t.Args.Args) == 1 {
				return &types.Set{Elem: c.resolveType(t.Args.Args[0], scope)}
			}
			if p, ok := pointerType(name, c.resolveType(t.Args.Args[0], scope)); ok &&
				len(t.Args.Args) == 1 {
				return p
			}
		}

		var base types.Type
		// Check lexical scope first
		if tn := scope.LookupType(name); tn != nil {
			base = tn.Type()
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
			// Swift's Task<Success, Failure>: what it keeps is Success.
			if c.isCoreTask(base) {
				return taskOf(base, args[0])
			}
			return &types.GenericInstance{Base: base, Args: args}
		}
		return base

	case *ast.ParenType:
		return c.resolveType(t.X, scope)

	case *ast.OptionalType:
		wrapped := c.resolveType(t.Base, scope)
		return &types.Optional{Wrapped: wrapped}

	case *ast.UnwrappedType:
		wrapped := c.resolveType(t.Base, scope)
		return &types.Optional{Wrapped: wrapped, Implicit: true}

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
			Params:   params,
			Results:  c.resolveType(t.Result, scope),
			Async:    t.Async != token.NoPos,
			Throws:   throws,
			Thrown:   thrown,
			Rethrows: t.Throws != nil && t.Throws.Kind == token.RETHROWS,
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

// resolveMemberType resolves a qualified type name (e.g. `Module.Type` or `Type.Member`).
func (c *checker) resolveMemberType(t *ast.MemberType, scope *Scope) types.Type {
	name := t.Name.Text(c.file)
	if q, ok := t.X.(*ast.IdentType); ok && (q.Args == nil || len(q.Args.Args) == 0) {
		if qual := q.Name.Text(c.file); c.modules[qual] != nil {
			return c.moduleMember(t, qual, name, scope)
		}
	}
	// Associated type via type parameter or Self (e.g. `C.Element`).
	if base := c.resolveType(t.X, scope); base != nil {
		if tp, ok := base.(*types.TypeParam); ok {
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
		// Associated type on concrete conformer or nested type.
		if answer := types.AssocOf(base, name); answer != nil {
			return answer
		}
		if inner := c.nestedType(base, name, t, scope); inner != nil {
			return inner
		}
	}
	c.errorf(t.Pos(), "cannot find type '%s' in scope: a type declared inside "+
		"another type is not modelled yet", c.spellingOf(t))
	return types.Typ[types.Invalid]
}

// nestedType resolves a type declared inside outer, or returns nil.
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
		case name == "Set" && len(args) == 1:
			return &types.Set{Elem: args[0]}
		}
		if len(args) == 1 {
			if p, ok := pointerType(name, args[0]); ok {
				return p
			}
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
	if c.isCoreTask(base) {
		return taskOf(base, args[0])
	}
	return &types.GenericInstance{Base: base, Args: args}
}

// spellingOf returns the source text spelling of an AST type.
func (c *checker) spellingOf(t ast.Type) string {
	switch n := t.(type) {
	case *ast.IdentType:
		return n.Name.Text(c.file)
	case *ast.MemberType:
		return c.spellingOf(n.X) + "." + n.Name.Text(c.file)
	}
	return string(c.file.Slice(t.Pos(), t.End()))
}

// declsOf extracts declarations from a list of statements.
// declarationsOnly reports whether a file holds nothing but
// declarations: no top-level code, and so no order to it.
func declarationsOnly(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		if _, ok := stmt.(*ast.DeclStmt); !ok {
			return false
		}
	}
	return true
}

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
			en := &types.Enum{Name: name, Indirect: c.hasModifier(d.Mods, "indirect")}
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

// declareGenericParams binds generic parameters in scope and records constraints.
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

// associatedType returns the payload type of an enum case.
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
	f := &types.Field{
		Name:         idPat.Name.Text(c.file),
		Type:         fieldType,
		IsConst:      isConst,
		HasDefault:   b.Value != nil,
		HasObservers: c.hasObservers(b),
		HasSetter:    c.hasSetter(b),
	}
	if b.Value != nil {
		c.info.FieldDefaults[f] = b.Value
	}
	return []*types.Field{f}
}

// hasSetter reports whether a computed binding may be written to: it
// declares a `set` (or a `_modify`/`unsafeMutableAddress` standing in for
// one). A binding that is just an expression body, or only a `get`, is
// read-only.
func (c *checker) hasSetter(b *ast.PatternBinding) bool {
	if b == nil || b.Accessors == nil {
		return false
	}
	for _, a := range b.Accessors.Accessors {
		if a == nil || a.Keyword == nil {
			continue
		}
		switch a.Keyword.Text(c.file) {
		case "set", "_modify", "unsafeMutableAddress":
			return true
		}
	}
	return false
}

// isComputed reports whether a binding represents a computed property.
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

// hasObservers reports whether a binding was declared with willSet or didSet.
func (c *checker) hasObservers(b *ast.PatternBinding) bool {
	if b == nil || b.Accessors == nil {
		return false
	}
	for _, a := range b.Accessors.Accessors {
		if a == nil || a.Keyword == nil {
			continue
		}
		switch a.Keyword.Text(c.file) {
		case "willSet", "didSet":
			return true
		}
	}
	return false
}

// isMutating reports whether the modifier list contains mutating.
func (c *checker) isMutating(mods []*ast.Modifier) bool {
	for _, m := range mods {
		if m != nil && m.Name != nil && m.Name.Text(c.file) == "mutating" {
			return true
		}
	}
	return false
}

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

// declareNested declares types inside a nominal type's body and reads their members.
func (c *checker) declareNested(body *ast.MemberBlock, typeScope *Scope, outer types.Type) {
	nested := memberDecls(body)
	if len(nested) == 0 {
		return
	}
	c.declareTypes(nested, typeScope)
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
				n.RawType = c.rawTypeOf(d.Inherit, scope)
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
	typeScope.members = true
	typeScope.Insert(NewTypeName("Self", sym.Type(), name.Pos()))
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

// readMembers populates fields, methods, enum cases, initializers, and computed properties.
func (c *checker) readMembers(body *ast.MemberBlock, typeScope *Scope, fields *[]*types.Field, methods *[]*types.Method, en *types.Enum, inits *[]*types.Signature, computed, statics *[]*types.Field) {
	if body == nil || typeScope == nil {
		return
	}
	// Every other member is declared before a property's initializer is
	// read, as swiftc declares them all first: `static let start = origin()`
	// names a method written after it. Properties keep their own order,
	// which is their layout.
	members := make([]ast.Node, 0, len(body.Members))
	for _, mem := range body.Members {
		if _, isVar := mem.(*ast.VarDecl); !isVar {
			members = append(members, mem)
		}
	}
	for _, mem := range body.Members {
		if _, isVar := mem.(*ast.VarDecl); isVar {
			members = append(members, mem)
		}
	}
	for _, mem := range members {
		switch m := mem.(type) {
		case *ast.VarDecl:
			isConst := m.Kind == token.LET
			static := isStatic(m.Mods)
			for _, b := range m.Bindings {
				got := c.storedField(b, isConst, typeScope)
				for _, f := range got {
					f.Exported = exported(c.accessOf(m.Mods))
				}
				switch {
				case static:
					if statics == nil {
						continue
					}
					if c.isComputed(b) {
						for _, f := range got {
							f.IsComputed = true
						}
					}
					*statics = append(*statics, got...)
				case c.isComputed(b):
					if computed == nil {
						continue
					}
					for _, f := range got {
						f.IsComputed = true
					}
					*computed = append(*computed, got...)
				default:
					if fields == nil {
						c.errorf(b.Pos(), "enums must not contain stored properties")
						continue
					}
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
				label := ""
				if len(el.Params) == 1 && el.Params[0].Label != nil {
					label = el.Params[0].Label.Text(c.file)
				}
				indirect := assoc != nil && (en.Indirect || m.Indirect.IsValid())
				en.Cases = append(en.Cases, &types.EnumCase{Name: name, AssociatedType: assoc, Label: label, Indirect: indirect})
				sym := NewEnumCase(name, en, assoc, el.Name.Pos())
				sym.label = label
				typeScope.Insert(sym)
				c.info.Defs[el.Name] = sym
			}

		case *ast.InitDecl:
			if inits == nil {
				continue
			}
			sig := c.buildFuncSig(m.Sig, typeScope)
			sig.Exported = exported(c.accessOf(m.Mods))
			*inits = append(*inits, sig)

		case *ast.FuncDecl:
			if methods == nil {
				continue
			}
			name := m.Name.Text(c.file)
			// A method's own generic parameters are in scope for its
			// signature, beside the type's.
			sig := c.buildGenericFuncSig(m, typeScope)
			unlabelOperator(name, sig)
			*methods = append(*methods, &types.Method{
				Name: name, Sig: sig, IsStatic: isStatic(m.Mods),
				IsMutating: c.isMutating(m.Mods),
				Exported:   exported(c.accessOf(m.Mods)),
			})
			sym := NewFunc(name, sig, m.Name.Pos())
			sym.SetDecl(m)
			sym.SetAccess(c.accessOf(m.Mods))
			// Methods may share a name, told apart by their labels.
			if prev, ok := typeScope.Insert(sym).(*FuncSymbol); ok {
				prev.AddOverload(sym)
			}
			c.info.Defs[m.Name] = sym
		}
	}
}

// declareFunctions discovers top-level functions and signatures.
func (c *checker) declareFunctions(decls []ast.Decl, scope *Scope) {
	for _, d := range decls {
		// A computed variable is a getter, declared with the functions so
		// that a file checked before the one declaring it can read it.
		if v, ok := d.(*ast.VarDecl); ok {
			c.declareComputedVars(v, scope)
			continue
		}
		if f, ok := d.(*ast.FuncDecl); ok {
			if f.Recv != nil {
				continue
			}
			name := f.Name.Text(c.file)
			sig := c.buildGenericFuncSig(f, scope)
			unlabelOperator(name, sig)
			sym := NewFunc(name, sig, f.Name.Pos())
			sym.SetDecl(f)
			sym.SetAccess(c.accessOf(f.Mods))
			c.declaredHere(sym, sym.Access())
			// Functions may share a name if signatures differ (overloading).
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

// declareComputedVars declares the computed bindings of a variable
// declaration, each with the type it is annotated with.
func (c *checker) declareComputedVars(v *ast.VarDecl, scope *Scope) {
	for _, b := range v.Bindings {
		if b.Body == nil && b.Accessors == nil {
			continue
		}
		tp, ok := b.Pat.(*ast.TypedPattern)
		if !ok {
			continue
		}
		c.declarePatternInit(b.Pat, c.resolveType(tp.Type, scope), v.Kind == token.LET, true, scope)
	}
}

// ownershipOf returns the ownership convention (borrowing, consuming, inout) specified by mods.
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

// throwsOf returns whether the throws clause throws and the thrown error type.
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

// buildGenericFuncSig reads a function's signature in a scope containing its generic parameters.
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
				c.info.DefaultFiles[p.Default] = c.file
			}
		}
	}
	var res types.Type = types.Typ[types.Void]
	if sig.Result != nil {
		res = c.resolveType(sig.Result.Type, scope)
	}
	throws, thrown := c.throwsOf(sig.Throws, scope)
	return &types.Signature{
		Params:   params,
		Results:  res,
		Async:    sig.Async != token.NoPos,
		Throws:   throws,
		Thrown:   thrown,
		Rethrows: sig.Throws != nil && sig.Throws.Kind == token.RETHROWS,
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
		sym.deferred = isConst && !isInitialized
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
		// Each name takes the type of the element in its position, where
		// the value is a tuple of as many elements.
		var tu *types.Tuple
		if typ != nil {
			tu, _ = typ.Underlying().(*types.Tuple)
		}
		for i, elem := range p.Elems {
			var et types.Type
			if tu != nil && len(tu.Elements) == len(p.Elems) {
				et = tu.Elements[i].Type
			}
			c.declarePatternInit(elem.Pat, et, isConst, isInitialized, scope)
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
		var extType types.Type
		var fields, computed, statics *[]*types.Field
		var methods *[]*types.Method
		var conformances *[]*types.Protocol
		var inits *[]*types.Signature
		builtin, isBuiltin := c.extendedBuiltin(ext.Type, scope)
		if isBuiltin && builtin != nil {
			extType = builtin.Type
			fields, methods, conformances, inits, computed, statics =
				&builtin.Fields, &builtin.Methods, &builtin.Conformances, &builtin.Inits, &builtin.Computed, &builtin.Statics
		} else {
			extType = c.resolveType(ext.Type, scope)
			if extType == nil || isInvalid(extType) {
				continue
			}
			fields, methods, conformances, inits, computed, statics = sinksOf(extType.Underlying())
		}
		c.info.Extensions[ext] = extType

		typeScope := c.typeScopes[typeNameOf(extType)]
		if typeScope == nil {
			typeScope = NewScope(scope, ext.Pos(), ext.End())
			typeScope.members = true
		}
		c.info.Scopes[ext] = typeScope

		if conformances != nil {
			*conformances = append(*conformances, c.protocolsOf(ext.Inherit, scope, nil)...)
		}
		en, _ := extType.Underlying().(*types.Enum)
		var methodsBefore, computedBefore int
		if methods != nil {
			methodsBefore = len(*methods)
		}
		if computed != nil {
			computedBefore = len(*computed)
		}
		c.readMembers(ext.Body, typeScope, fields, methods, en, inits, computed, statics)
		if isBuiltin && builtin != nil && c.importing != "" {
			if builtin.Modules == nil {
				builtin.Modules = map[any]string{}
			}
			for _, m := range (*methods)[methodsBefore:] {
				builtin.Modules[m] = c.importing
			}
			for _, f := range (*computed)[computedBefore:] {
				builtin.Modules[f] = c.importing
			}
		}
		if conds := c.extensionConditions(ext, extType, typeScope); len(conds) > 0 {
			if methods != nil {
				for _, m := range (*methods)[methodsBefore:] {
					c.info.setConditions(m, conds)
				}
			}
			if computed != nil {
				for _, f := range (*computed)[computedBefore:] {
					c.info.setConditions(f, conds)
				}
			}
		}
	}
}

// A memberCondition is what an extension's where clause asks of one of
// its type's parameters before its members may be used.
type memberCondition struct {
	param *types.TypeParam
	proto *types.Protocol
}

// extensionConditions is what an extension's where clause asks of the
// parameters of the type it extends.
func (c *checker) extensionConditions(ext *ast.ExtensionDecl, extType types.Type, scope *Scope) []memberCondition {
	if ext.Where == nil {
		return nil
	}
	params := c.typeParamsOf(extType)
	var out []memberCondition
	for _, req := range ext.Where.Reqs {
		r, ok := req.(*ast.ConformanceReq)
		if !ok {
			continue
		}
		id, ok := r.Left.(*ast.IdentType)
		if !ok || id.Name == nil {
			continue
		}
		name := id.Name.Text(c.file)
		var tp *types.TypeParam
		for _, p := range params {
			if p.Name == name {
				tp = p
			}
		}
		if tp == nil {
			continue
		}
		if p, ok := protocolOf(c.resolveType(r.Right, scope)); ok {
			out = append(out, memberCondition{param: tp, proto: p})
		}
	}
	return out
}

// checkMemberConditions reports a member of a generic instance that an
// extension adds only where the type's parameters conform to something
// this instance's arguments do not.
func (c *checker) checkMemberConditions(e *ast.MemberExpr, base types.Type, name string) {
	if meta, ok := base.(*types.Metatype); ok {
		base = meta.Instance
	}
	inst, ok := base.(*types.GenericInstance)
	if !ok {
		b := c.builtinOf(base)
		if b == nil || len(b.Params) == 0 {
			return
		}
		inst = &types.GenericInstance{Base: b.Type, Args: b.Args(base)}
	}
	var candidates []any
	kind := "property"
	if b := c.builtinOf(inst.Base); b != nil && len(b.Params) > 0 {
		candidates = membersNamed(b.Methods, b.Computed, name)
	}
	switch u := inst.Base.Underlying().(type) {
	case *types.Struct:
		candidates = membersNamed(u.Methods, u.Computed, name)
	case *types.Class:
		candidates = membersNamed(u.Methods, u.Computed, name)
	case *types.Enum:
		candidates = membersNamed(u.Methods, u.Computed, name)
	}
	if len(candidates) == 0 {
		return
	}
	params := c.typeParamsOf(inst.Base)
	argOf := func(tp *types.TypeParam) types.Type {
		for i, p := range params {
			if p == tp && i < len(inst.Args) {
				return inst.Args[i]
			}
		}
		return nil
	}
	var unmet *memberCondition
	var unmetArg types.Type
	for _, m := range candidates {
		if _, isMethod := m.(*types.Method); isMethod {
			kind = "instance method"
			if m.(*types.Method).IsStatic {
				kind = "static method"
			}
		}
		met := true
		for _, cond := range c.info.conditions[m] {
			arg := argOf(cond.param)
			if arg == nil || c.conformsTo(arg, cond.proto) {
				continue
			}
			met = false
			if unmet == nil {
				cc := cond
				unmet, unmetArg = &cc, arg
			}
			break
		}
		if met {
			return
		}
	}
	if unmet != nil {
		c.typeErrorf(e.Name.Pos(), "referencing %s '%s' on '%s' requires that '%s' conform to '%s'",
			kind, name, builtinName(inst.Base), unmetArg, unmet.proto.Name)
	}
}

// builtinName is a type's name as a diagnostic about its members says it:
// Array rather than [Element].
func builtinName(t types.Type) string {
	if k := BuiltinKey(t); k != "" {
		return k
	}
	return typeNameOf(t)
}

// membersNamed is the methods and computed properties of a name.
func membersNamed(methods []*types.Method, computed []*types.Field, name string) []any {
	var out []any
	for _, m := range methods {
		if m != nil && m.Name == name {
			out = append(out, m)
		}
	}
	for _, f := range computed {
		if f != nil && f.Name == name {
			out = append(out, f)
		}
	}
	return out
}

// resolveReceivers attaches receiver methods to their target types.
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
			typeScope.members = true
		}
		_, methods, _, _, _, _ := sinksOf(recv.Underlying())
		if methods == nil {
			c.errorf(fn.Recv.Pos(), "cannot add a method to '%s'", recv)
			continue
		}
		name := fn.Name.Text(c.file)
		sig := c.buildFuncSig(fn.Sig, typeScope)
		unlabelOperator(name, sig)
		*methods = append(*methods, &types.Method{
			Name: name, Sig: sig, IsStatic: isStatic(fn.Mods),
			IsMutating: c.ownershipOf(fn.Recv.Mods) == types.InOut,
			Exported:   exported(c.accessOf(fn.Mods)),
		})
		sym := NewFunc(name, sig, fn.Name.Pos())
		sym.SetDecl(fn)
		sym.SetAccess(c.accessOf(fn.Mods))
		if prev, ok := typeScope.Insert(sym).(*FuncSymbol); ok {
			prev.AddOverload(sym)
		}
		c.info.Defs[fn.Name] = sym
	}
}

// typeNameOf returns the declaration name of t.
func typeNameOf(t types.Type) string {
	if t == nil {
		return ""
	}
	if named, ok := t.(*types.Named); ok {
		return named.Name
	}
	return t.Underlying().String()
}

// sinksOf returns member storage slices for nominal types.
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

		// A property requirement is met by a stored property or a
		// computed one: `var description: String { ... }`.
		switch t := tn.Type().(type) {
		case *types.Struct:
			conformances = t.Conformances
			fields = append(append(fields, t.Fields...), t.Computed...)
			methods = t.Methods
		case *types.Class:
			conformances = t.Conformances
			fields = append(append(fields, t.Fields...), t.Computed...)
			methods = t.Methods
		case *types.Enum:
			conformances = t.Conformances
			fields = t.Computed
			methods = t.Methods
		}

		// A diagnostic is about the file the type is declared in, not
		// whichever file was checked last.
		prevFile := c.file
		if site, ok := c.declSites[tn]; ok && site.file != nil {
			c.file = site.file
		}
		for _, proto := range conformances {
			c.checkConformance(tn.Pos(), tn.Type(), typeName, proto, fields, methods)
		}
		c.file = prevFile
	}
}

func (c *checker) checkConformance(pos token.Pos, conformer types.Type, typeName string, proto *types.Protocol, fields []*types.Field, methods []*types.Method) {
	// Substitute conforming type into requirements referencing Self.
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
				// Equatable's `==`, derived for a struct that writes none.
				if req.Name == "==" && (m.Name == derive.EqualsName || m.Name == derive.EnumEqualsName) && types.Identical(m.Sig, want) {
					satisfied = true
					break
				}
			}
			// An enum's cases compare without a `==` written for them.
			if _, isEnum := conformer.Underlying().(*types.Enum); isEnum && req.Name == "==" {
				satisfied = true
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
		for _, need := range a.Constraints {
			if !c.conformsTo(choice, need) {
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

// rawTypeOf returns the raw type specified by an enum's inheritance clause, if any.
func (c *checker) rawTypeOf(inherit *ast.InheritanceClause, scope *Scope) types.Type {
	if inherit == nil || len(inherit.Items) == 0 || inherit.Items[0] == nil {
		return nil
	}
	t := c.resolveType(inherit.Items[0].Type, scope)
	if t == nil || isInvalid(t) {
		return nil
	}
	if _, isProtocol := t.Underlying().(*types.Protocol); isProtocol {
		return nil
	}
	return t
}
