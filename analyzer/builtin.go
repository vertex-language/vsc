package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Extensions of the types Swift's standard library declares and this
// compiler builds in -- Int, String, Array, Dictionary, Set, Optional.
//
// Those are not nominal declarations here, so there is nowhere on them to
// keep what an extension adds. Each gets a BuiltinMembers instead, one for
// the compilation, holding the members the program's extensions declare
// and the generic parameters they are written in terms of: Array's
// Element, Dictionary's Key and Value, Set's Element, Optional's Wrapped.
// A use on `[Int]` is a use on `[Element]` with Element standing for Int.

// BuiltinMembers is what extensions add to one built-in type.
type BuiltinMembers struct {
	// Key names the type: "Int", "String", "Array", "Optional", ...
	Key string
	// Type is the type as its extensions see it: Int, or [Element].
	Type types.Type
	// Params are the generic parameters Type is written in, in order.
	Params []*types.TypeParam

	Methods      []*types.Method
	Computed     []*types.Field
	Statics      []*types.Field
	Inits        []*types.Signature
	Fields       []*types.Field
	Conformances []*types.Protocol
	// Modules is the module that declared each member an imported
	// module's extension added: a *types.Method or *types.Field. A member
	// with no entry is the module being compiled's.
	Modules map[any]string

	scope *Scope
}

// BuiltinKey is the key a built-in type's extensions are kept under, or
// "" for a type that is not one.
func BuiltinKey(t types.Type) string {
	if t == nil {
		return ""
	}
	switch u := t.(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Bool, types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.UInt, types.UInt8, types.UInt16, types.UInt32, types.UInt64,
			types.Float, types.Double, types.String, types.Character:
			return u.Name()
		}
		return ""
	case *types.Array:
		return "Array"
	case *types.Dictionary:
		return "Dictionary"
	case *types.Set:
		return "Set"
	case *types.Optional:
		return "Optional"
	}
	return ""
}

// genericBuiltin reports whether name is one of the built-in generic types
// an extension may name bare: `extension Array`.
func genericBuiltin(name string) bool {
	switch name {
	case "Array", "Dictionary", "Set", "Optional":
		return true
	}
	return false
}

// Args is what a use of the type gives its parameters: Int for [Int].
func (b *BuiltinMembers) Args(t types.Type) []types.Type {
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	switch u := t.(type) {
	case *types.Array:
		return []types.Type{u.Elem}
	case *types.Dictionary:
		return []types.Type{u.Key, u.Value}
	case *types.Set:
		return []types.Type{u.Elem}
	case *types.Optional:
		return []types.Type{u.Wrapped}
	}
	return nil
}

// Subst is the substitution a use of the type makes.
func (b *BuiltinMembers) Subst(t types.Type) map[*types.TypeParam]types.Type {
	args := b.Args(t)
	if len(args) != len(b.Params) {
		return nil
	}
	out := make(map[*types.TypeParam]types.Type, len(args))
	for i, p := range b.Params {
		out[p] = args[i]
	}
	return out
}

// builtinMembers is the members kept for a built-in type, made the first
// time its key is asked for, with its parameters declared in a scope of
// its own inside parent.
func (c *checker) builtinMembers(key string, parent *Scope) *BuiltinMembers {
	if key == "" {
		return nil
	}
	if b := c.info.Builtins[key]; b != nil {
		return b
	}
	b := &BuiltinMembers{Key: key}
	param := func(name string, constraints ...string) *types.TypeParam {
		tp := &types.TypeParam{Name: name}
		for _, con := range constraints {
			if p, ok := c.coreProtocol(con); ok {
				tp.Constraints = append(tp.Constraints, p)
			}
		}
		b.Params = append(b.Params, tp)
		return tp
	}
	switch key {
	case "Array":
		b.Type = &types.Array{Elem: param("Element")}
	case "Dictionary":
		k := param("Key", "Hashable")
		b.Type = &types.Dictionary{Key: k, Value: param("Value")}
	case "Set":
		b.Type = &types.Set{Elem: param("Element", "Hashable")}
	case "Optional":
		b.Type = &types.Optional{Wrapped: param("Wrapped")}
	default:
		u := types.LookupUniverse(key)
		if u == nil {
			return nil
		}
		b.Type = u
	}
	b.scope = NewScope(parent, token.NoPos, token.NoPos)
	b.scope.members = true
	for _, p := range b.Params {
		b.scope.Insert(NewTypeName(p.Name, p, token.NoPos))
	}
	c.typeScopes[typeNameOf(b.Type)] = b.scope
	c.info.Builtins[key] = b
	return b
}

// builtinOf is the members extensions gave t's built-in type, or nil.
func (c *checker) builtinOf(t types.Type) *BuiltinMembers {
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	// A literal's type is its default's: `"hi".shout` is String's.
	if b, ok := t.(*types.Basic); ok {
		switch b.Kind() {
		case types.UntypedString:
			t = types.Typ[types.String]
		case types.UntypedInt:
			t = types.Typ[types.Int]
		case types.UntypedFloat:
			t = types.Typ[types.Double]
		case types.UntypedBool:
			t = types.Typ[types.Bool]
		}
	}
	return c.info.Builtins[BuiltinKey(t)]
}

// extendedBuiltin is the type an extension of a built-in type extends,
// where it names one: `extension Array` or `extension Int`.
func (c *checker) extendedBuiltin(t ast.Type, scope *Scope) (*BuiltinMembers, bool) {
	id, ok := t.(*ast.IdentType)
	if !ok || id.Name == nil || (id.Args != nil && len(id.Args.Args) > 0) {
		return nil, false
	}
	name := id.Name.Text(c.file)
	if tn := scope.LookupType(name); tn != nil {
		// A type of the program's own by that name hides the built-in one;
		// core's name for the built-in type is the built-in type.
		if u := types.LookupUniverse(name); u == nil || tn.Type() != u {
			return nil, false
		}
	}
	if genericBuiltin(name) {
		return c.builtinMembers(name, scope), true
	}
	if u := types.LookupUniverse(name); u != nil && BuiltinKey(u) == name {
		return c.builtinMembers(name, scope), true
	}
	return nil, false
}

// builtinMember is the type of a member extensions gave t, substituted for
// t's arguments, or nil.
func (c *checker) builtinMember(t types.Type, name string) types.Type {
	b := c.builtinOf(t)
	if b == nil {
		return nil
	}
	_, onType := t.(*types.Metatype)
	subst := b.Subst(t)
	sub := func(x types.Type) types.Type {
		if len(subst) == 0 {
			return x
		}
		return types.Substitute(x, subst)
	}
	if !onType {
		for _, f := range b.Computed {
			if f.Name == name {
				return sub(f.Type)
			}
		}
	} else {
		for _, f := range b.Statics {
			if f.Name == name {
				return sub(f.Type)
			}
		}
	}
	for _, m := range b.Methods {
		if m.Name == name && m.IsStatic == onType {
			return sub(m.Sig)
		}
	}
	return nil
}

// builtinMethods is the methods of a name extensions gave t's type, and
// the type they were declared on.
func (c *checker) builtinMethods(t types.Type, name string) (types.Type, []*types.Method) {
	b := c.builtinOf(t)
	if b == nil {
		return nil, nil
	}
	_, onType := t.(*types.Metatype)
	var out []*types.Method
	for _, m := range b.Methods {
		if m.Name == name && m.IsStatic == onType {
			out = append(out, m)
		}
	}
	return b.Type, out
}

// conformsTo is types.ConformsTo, with the conformances extensions gave
// the built-in types.
func (c *checker) conformsTo(t types.Type, p *types.Protocol) bool {
	if types.ConformsTo(t, p) {
		return true
	}
	if b := c.builtinOf(t); b != nil {
		for _, have := range b.Conformances {
			if types.Identical(have, p) || types.ConformsTo(have, p) {
				return true
			}
		}
	}
	return false
}

// implicitSelfMember is `self.name` for a name used alone, or called, inside
// an extension of a built-in type, where no declaration in scope has the
// name and the type has the member; nil otherwise. A member the extensions
// declare is in the extension's scope already, and found the ordinary way.
func (c *checker) implicitSelfMember(expr ast.Expr, scope *Scope) ast.Expr {
	if c.currType == nil || BuiltinKey(c.currType) == "" {
		return nil
	}
	selfAt := func(id *ast.IdentExpr) *ast.MemberExpr {
		return &ast.MemberExpr{Span: id.Span, X: &ast.SelfExpr{Span: id.Span}, Dot: id.Pos(), Name: id.Name}
	}
	unbound := func(id *ast.IdentExpr) (string, bool) {
		if id == nil || id.Name == nil || id.Args != nil {
			return "", false
		}
		name := id.Name.Text(c.file)
		if c.lookupValue(scope, name) != nil || types.LookupUniverse(name) != nil {
			return "", false
		}
		return name, true
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		name, ok := unbound(e)
		if !ok {
			// A name bound to one of the type's methods, not called,
			// beside a property of the same name -- `first` inside an
			// extension of Array, where first(where:) is a method too --
			// is the property, which is what a name alone reads.
			if e.Name == nil || e.Args != nil {
				return nil
			}
			name = e.Name.Text(c.file)
			found, sym := scope.LookupParent(name)
			if _, isFunc := sym.(*FuncSymbol); !isFunc || found == nil || !found.members {
				return nil
			}
			if _, isProperty := core.LowerCollectionProperty(c.currType, name); !isProperty {
				if _, isMember := core.LowerMember(c.currType, name); !isMember {
					return nil
				}
			}
			return selfAt(e)
		}
		if c.lookupMember(c.currType, name) == nil {
			return nil
		}
		return selfAt(e)
	case *ast.CallExpr:
		id, ok := e.Fun.(*ast.IdentExpr)
		if !ok {
			return nil
		}
		name, ok := unbound(id)
		if !ok {
			return nil
		}
		var labels []string
		if e.Args != nil {
			for _, a := range e.Args.Args {
				label := ""
				if a.Label != nil {
					label = a.Label.Text(c.file)
				}
				labels = append(labels, label)
			}
		}
		if _, isCore := core.LowerCollectionMethod(c.currType, name, labels); !isCore {
			if _, m := c.findMethod(c.currType, name); m == nil {
				return nil
			}
		}
		return &ast.CallExpr{Span: e.Span, Fun: selfAt(id), Args: e.Args, Trailing: e.Trailing}
	}
	return nil
}
