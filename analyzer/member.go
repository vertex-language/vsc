package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"strconv"

	"github.com/vertex-language/vsc/types"
)

// lookupMemberFor looks up member name on type t and records method references, type uses, or enum cases.
func (c *checker) lookupMemberFor(e *ast.MemberExpr, t types.Type, name string) types.Type {
	got := c.lookupMember(t, name)
	if e != nil && e.Name != nil {
		if recv, m := c.findMethod(t, name); m != nil {
			c.info.Methods[e] = &MethodRef{Recv: recv, Method: m}
		}
		if meta, ok := t.(*types.Metatype); ok {
			if scope := c.typeScope(meta.Instance); scope != nil {
				if sym, ok := scope.LookupLocal(name).(*TypeNameSymbol); ok {
					c.info.Uses[e.Name] = sym
				}
			}
		}
		if sym := c.enumCaseSymbol(t, name); sym != nil {
			c.info.Uses[e.Name] = sym
		}
	}
	return got
}

// nestedIn returns the named type nested inside outer, or nil.
func (c *checker) nestedIn(outer types.Type, name string) types.Type {
	if inst, ok := outer.(*types.GenericInstance); ok {
		outer = inst.Base
	}
	scope := c.typeScope(outer)
	if scope == nil {
		return nil
	}
	sym, _ := scope.LookupLocal(name).(*TypeNameSymbol)
	if sym == nil {
		return nil
	}
	return sym.Type()
}

// enumCaseSymbol returns the enum case symbol named name on t, or nil.
func (c *checker) enumCaseSymbol(t types.Type, name string) *EnumCaseSymbol {
	if t == nil {
		return nil
	}
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		t = inst.Base
	}
	if _, ok := t.Underlying().(*types.Enum); !ok {
		return nil
	}
	scope := c.typeScope(t)
	if scope == nil {
		return nil
	}
	sym, _ := scope.Lookup(name).(*EnumCaseSymbol)
	return sym
}

// findMethod finds a method named name on t and returns its declaring type and method object.
func (c *checker) findMethod(t types.Type, name string) (types.Type, *types.Method) {
	if found, m := c.findOwnMethod(t, name); m != nil {
		return found, m
	}
	static := false
	inst := t
	if meta, ok := t.(*types.Metatype); ok {
		static, inst = true, meta.Instance
	}
	if q, m := c.extensionMethodOf(inst, name, static); m != nil {
		return q, m
	}
	return nil, nil
}

// findOwnMethod is findMethod without what protocol extensions add.
func (c *checker) findOwnMethod(t types.Type, name string) (types.Type, *types.Method) {
	if t == nil {
		return nil, nil
	}
	onType := false
	if meta, ok := t.(*types.Metatype); ok {
		onType = true
		t = meta.Instance
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		if onType {
			return c.findMethod(&types.Metatype{Instance: inst.Base}, name)
		}
		return c.findMethod(inst.Base, name)
	}
	// Method requirement on a type parameter constraint.
	if tp, ok := t.(*types.TypeParam); ok {
		if tp.Same != nil {
			if onType {
				return c.findMethod(&types.Metatype{Instance: tp.Same}, name)
			}
			return c.findMethod(tp.Same, name)
		}
		for _, con := range tp.Constraints {
			if found, m := requirementOf(con, name); m != nil {
				sig, _ := throughParam(con, tp, m.Sig).(*types.Signature)
				if sig == nil {
					sig = m.Sig
				}
				return found, &types.Method{Name: m.Name, Sig: sig, IsStatic: m.IsStatic, IsMutating: m.IsMutating}
			}
		}
		return nil, nil
	}
	// Method requirement on a dependent member type constraint.
	if dep, ok := t.(*types.Dependent); ok {
		for _, con := range associatedConstraints(dep) {
			if found, m := requirementOf(con, name); m != nil {
				sig, _ := throughParam(con, dep, m.Sig).(*types.Signature)
				if sig == nil {
					sig = m.Sig
				}
				return found, &types.Method{Name: m.Name, Sig: sig, IsStatic: m.IsStatic, IsMutating: m.IsMutating}
			}
		}
		return nil, nil
	}
	if ex, ok := t.(*types.Existential); ok {
		for _, p := range ex.Protocols {
			if found, m := requirementOf(p, name); m != nil {
				if sig, ok := existentialSame(ex, found, m.Sig).(*types.Signature); ok && sig != m.Sig {
					return found, &types.Method{Name: m.Name, Sig: sig, IsStatic: m.IsStatic, IsMutating: m.IsMutating, IsConsuming: m.IsConsuming, Origin: m}
				}
				return found, m
			}
		}
		return nil, nil
	}
	if p, ok := t.(*types.Protocol); ok {
		return requirementOf(p, name)
	}
	if b := c.builtinOf(t); b != nil {
		for _, m := range b.Methods {
			if m.Name == name && m.IsStatic == onType {
				return b.Type, m
			}
		}
	}
	var methods []*types.Method
	switch b := t.Underlying().(type) {
	case *types.Struct:
		methods = b.Methods
	case *types.Class:
		methods = b.Methods
	case *types.Enum:
		methods = b.Methods
	default:
		return nil, nil
	}
	for _, m := range methods {
		if m.Name == name && m.IsStatic == onType {
			return t, m
		}
	}
	if b, ok := t.Underlying().(*types.Class); ok && b.Superclass != nil {
		// A class method or static one is inherited as an instance
		// method is: looked for on the superclass's metatype.
		if onType {
			return c.findMethod(&types.Metatype{Instance: b.Superclass}, name)
		}
		return c.findMethod(b.Superclass, name)
	}
	return nil, nil
}

func (c *checker) lookupMember(t types.Type, name string) types.Type {
	if m := c.lookupOwnMember(t, name); m != nil {
		return m
	}
	return c.extensionMember(t, name)
}

// extensionMember is the type of the member named name a protocol t
// conforms to gives it through an extension, with Self as t: nil where
// there is none.
func (c *checker) extensionMember(t types.Type, name string) types.Type {
	onType := false
	if meta, ok := t.(*types.Metatype); ok {
		onType, t = true, meta.Instance
	}
	// A property of the name is what the name alone means -- a
	// Collection's first beside Sequence's first(where:); a call finds
	// the method by its arguments.
	if _, f := c.extensionPropertyOf(t, name, onType); f != nil {
		return known(f.Type)
	}
	if _, m := c.extensionMethodOf(t, name, onType); m != nil {
		return m.Sig
	}
	return nil
}

// extensionMethodOf is the method named name an extension of a protocol t
// conforms to declares, with Self replaced by t, and that protocol.
func (c *checker) extensionMethodOf(t types.Type, name string, static bool) (*types.Protocol, *types.Method) {
	if protocolOfSelf(t) != nil || isInvalid(t) {
		return nil, nil
	}
	for _, p := range c.conformancesOfType(t) {
		if q, m := p.ExtensionMethod(name, static); m != nil {
			sig, _ := types.Substitute(m.Sig, map[*types.TypeParam]types.Type{q.Self: t}).(*types.Signature)
			if sig == nil {
				sig = m.Sig
			}
			return q, &types.Method{Name: m.Name, Sig: sig, IsStatic: m.IsStatic, IsMutating: m.IsMutating, Origin: m}
		}
	}
	return nil, nil
}

// extensionPropertyOf is extensionMethodOf for a computed property.
func (c *checker) extensionPropertyOf(t types.Type, name string, static bool) (*types.Protocol, *types.Field) {
	if protocolOfSelf(t) != nil || isInvalid(t) {
		return nil, nil
	}
	for _, p := range c.conformancesOfType(t) {
		if q, f := p.ExtensionProperty(name, static); f != nil {
			out := *f
			out.Type = types.Substitute(f.Type, map[*types.TypeParam]types.Type{q.Self: t})
			out.Origin = f
			return q, &out
		}
	}
	return nil, nil
}

// conformancesOfType is the protocols t declares it conforms to, in its
// declaration or its extensions, and those its superclasses do.
func (c *checker) conformancesOfType(t types.Type) []*types.Protocol {
	var out []*types.Protocol
	for t != nil {
		if inst, ok := t.(*types.GenericInstance); ok {
			t = inst.Base
		}
		if b := c.builtinOf(t); b != nil {
			out = append(out, b.Conformances...)
		}
		switch u := t.Underlying().(type) {
		case *types.Struct:
			return append(out, u.Conformances...)
		case *types.Enum:
			return append(out, u.Conformances...)
		case *types.Class:
			out = append(out, u.Conformances...)
			t = u.Superclass
			continue
		}
		return out
	}
	return out
}

// lookupOwnMember is lookupMember without what protocol extensions add.
func (c *checker) lookupOwnMember(t types.Type, name string) types.Type {
	if t == nil {
		return nil
	}
	onType := false
	if meta, ok := t.(*types.Metatype); ok {
		onType = true
		t = meta.Instance
		if inner := c.nestedIn(t, name); inner != nil {
			return &types.Metatype{Instance: inner}
		}
	}
	// A member of a type parameter is what its constraints promise.
	// Which implementation provides it is a question about the type
	// argument and is answered when there is one; what is needed here
	// is the type, so that the expression around it can be checked.
	if tp, ok := t.(*types.TypeParam); ok {
		// One an extension's where clause fixes has that type's members.
		if tp.Same != nil {
			if onType {
				return c.lookupMember(&types.Metatype{Instance: tp.Same}, name)
			}
			return c.lookupMember(tp.Same, name)
		}
		for _, con := range tp.Constraints {
			if member := c.requirementType(con, name); member != nil {
				return throughParam(con, tp, member)
			}
		}
		return nil
	}
	// The same for a dependent member type: what `C.Item` offers is
	// what Item's own constraints promise.
	if dep, ok := t.(*types.Dependent); ok {
		for _, con := range associatedConstraints(dep) {
			if member := c.requirementType(con, name); member != nil {
				return throughParam(con, dep, member)
			}
		}
		return nil
	}
	// The same for an existential: what it offers is what its
	// protocols promise.
	if ex, ok := t.(*types.Existential); ok {
		for _, p := range ex.Protocols {
			if member := c.requirementType(p, name); member != nil {
				return existentialSame(ex, p, member)
			}
		}
		return nil
	}
	if p, ok := t.(*types.Protocol); ok {
		return c.requirementType(p, name)
	}
	// An ArraySlice counts what lies between its bounds.
	if c.isArraySlice(t) && !onType {
		switch name {
		case "count":
			return known(types.Typ[types.Int])
		case "isEmpty":
			return known(types.Typ[types.Bool])
		}
	}
	// A Task's value is what its operation returned, once it has: nothing,
	// for the operations core's Task runs.
	if c.isCoreTask(t) && !onType && name == "value" {
		return known(c.taskResult(t))
	}
	// Substitute generic type parameters for specialized instances.
	if inst, ok := t.(*types.GenericInstance); ok {
		// Asked of the instance's type, a static member is asked of the
		// declaration's type: `Stack<Int>.of` is Stack's static of.
		var target types.Type = inst.Base
		if onType {
			target = &types.Metatype{Instance: inst.Base}
		}
		member := c.lookupMember(target, name)
		if member == nil {
			return nil
		}
		// A case with nothing to carry is a value of the instance itself:
		// `Maybe<Int>.none` is a Maybe<Int>.
		if member == inst.Base || member == inst.Base.Underlying() {
			return inst
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
	// A String's utf8 is its bytes, as an array of them.
	if b, ok := t.Underlying().(*types.Basic); ok && b.Kind() == types.String && name == "utf8" && !onType {
		return known(&types.Array{Elem: types.Typ[types.UInt8]})
	}
	if m, ok := core.LowerMember(t, name); ok && !onType {
		return known(m.Result)
	}
	if m, ok := core.LowerStaticMember(t, name); ok && onType {
		return known(m.Result)
	}
	if m, ok := core.LowerCollectionProperty(t, name); ok && !onType {
		return known(m.Result)
	}
	if member := c.builtinMember(t, name); member != nil {
		return member
	}
	if onType {
		if member := c.builtinMember(&types.Metatype{Instance: t}, name); member != nil {
			return member
		}
	}
	switch b := t.Underlying().(type) {
	// Pointers expose 'pointee' on typed dereferenceable pointers.
	case *types.Pointer:
		if name == "pointee" && !onType && b.Dereferenceable() {
			return known(b.Elem)
		}
		return nil
	// Tuple element lookup by integer index or element label.
	case *types.Tuple:
		if i, err := strconv.Atoi(name); err == nil {
			if i >= 0 && i < len(b.Elements) {
				return known(b.Elements[i].Type)
			}
			return nil
		}
		for _, elem := range b.Elements {
			if elem.Name == name {
				return known(elem.Type)
			}
		}
	case *types.Struct:
		for _, f := range b.Fields {
			if f.Name == name {
				return known(f.Type)
			}
		}
		for _, f := range b.Computed {
			if f.Name == name && !onType {
				return known(f.Type)
			}
		}
		for _, f := range b.Statics {
			if f.Name == name && onType {
				return known(f.Type)
			}
		}
		for _, m := range b.Methods {
			if m.Name == name && m.IsStatic == onType {
				return m.Sig
			}
		}
	case *types.Class:
		for _, f := range b.Fields {
			if f.Name == name {
				return known(f.Type)
			}
		}
		for _, f := range b.Computed {
			if f.Name == name && !onType {
				return known(f.Type)
			}
		}
		for _, f := range b.Statics {
			if f.Name == name && onType {
				return known(f.Type)
			}
		}
		for _, m := range b.Methods {
			if m.Name == name && m.IsStatic == onType {
				return m.Sig
			}
		}
		if b.Superclass != nil {
			if onType {
				return c.lookupMember(&types.Metatype{Instance: b.Superclass}, name)
			}
			return c.lookupMember(b.Superclass, name)
		}
	case *types.Enum:
		if name == "rawValue" && !onType && b.RawType != nil {
			return known(b.RawType)
		}
		for _, f := range b.Computed {
			if f.Name == name && !onType {
				return known(f.Type)
			}
		}
		for _, f := range b.Statics {
			if f.Name == name && onType {
				return known(f.Type)
			}
		}
		for _, cs := range b.Cases {
			if cs.Name == name {
				if cs.AssociatedType != nil {
					sig := &types.Signature{
						Params:  caseParams(cs.AssociatedType, cs.Label),
						Results: b,
					}
					// A case of a generic enum made through its bare name
					// says which instance by what it carries.
					if onType && len(b.TypeParams) > 0 {
						args := make([]types.Type, len(b.TypeParams))
						for i, p := range b.TypeParams {
							args[i] = p
						}
						sig.TypeParams = b.TypeParams
						sig.Results = &types.GenericInstance{Base: t, Args: args}
					}
					return sig
				}
				return b
			}
		}
		for _, m := range b.Methods {
			if m.Name == name && m.IsStatic == onType {
				return m.Sig
			}
		}
	}
	return nil
}

// known returns Invalid if t is nil, preserving known member existence.
func known(t types.Type) types.Type {
	if t == nil {
		return types.Typ[types.Invalid]
	}
	return t
}

// membersKnown reports whether the analyzer has complete member visibility for t.
func (c *checker) membersKnown(t types.Type) bool {
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	switch u := t.Underlying().(type) {
	case *types.Struct, *types.Class, *types.Enum:
		return true
	case *types.Pointer:
		return true
	// A String's and a collection's members are the ones core and the
	// runtime give them, all of which the lookup knows: anything else is
	// not there, and saying so here is better than a call that checks and
	// then cannot be lowered.
	case *types.Array, *types.Dictionary, *types.Set:
		return true
	case *types.Basic:
		return u.Kind() == types.String
	}
	return false
}

// requirementOf finds a required method by name across p and its inherited protocols.
func requirementOf(c types.Type, name string) (types.Type, *types.Method) {
	p, ok := c.(*types.Protocol)
	if !ok {
		if u, ok := c.Underlying().(*types.Protocol); ok {
			p = u
		} else {
			return nil, nil
		}
	}
	if found, m := declaredRequirement(p, name, map[*types.Protocol]bool{}); m != nil {
		return found, m
	}
	// Not a requirement, but what an extension gives every conformer: the
	// most refined protocol's first, so `Pet`'s `speak` hides `Animal`'s.
	for _, static := range []bool{false, true} {
		if q, m := p.ExtensionMethod(name, static); m != nil {
			return q, m
		}
	}
	return nil, nil
}

// declaredRequirement is the requirement named name that p or a protocol
// it inherits declares, and which protocol declares it.
func declaredRequirement(p *types.Protocol, name string, seen map[*types.Protocol]bool) (types.Type, *types.Method) {
	if p == nil || seen[p] {
		return nil, nil
	}
	seen[p] = true
	for _, r := range p.Requirements {
		if r != nil && r.Name == name && r.Sig != nil {
			return p, &types.Method{Name: r.Name, Sig: r.Sig, IsStatic: r.IsStatic, IsMutating: r.IsMutating}
		}
	}
	for _, up := range p.Inherited {
		if found, m := declaredRequirement(up, name, seen); m != nil {
			return found, m
		}
	}
	return nil, nil
}

// requirementType returns the type of a requirement named name across p and its inherited protocols.
func (c *checker) requirementType(con types.Type, name string) types.Type {
	p, ok := con.(*types.Protocol)
	if !ok {
		if u, ok := con.Underlying().(*types.Protocol); ok {
			p = u
		} else {
			return nil
		}
	}
	for _, r := range p.Requirements {
		if r == nil || r.Name != name {
			continue
		}
		if r.Sig != nil {
			return r.Sig
		}
		return r.Type
	}
	for _, up := range p.Inherited {
		if t := c.requirementType(up, name); t != nil {
			return t
		}
	}
	// Not a requirement, but what an extension gives every conformer.
	if _, m := p.ExtensionMethod(name, false); m != nil {
		return m.Sig
	}
	if _, f := p.ExtensionProperty(name, false); f != nil {
		return known(f.Type)
	}
	if _, m := p.ExtensionMethod(name, true); m != nil {
		return m.Sig
	}
	if _, f := p.ExtensionProperty(name, true); f != nil {
		return known(f.Type)
	}
	return nil
}

// caseParams builds parameters for an enum case constructor from its associated type.
func caseParams(assoc types.Type, label string) []*types.Param {
	if tu, ok := assoc.Underlying().(*types.Tuple); ok && len(tu.Elements) > 0 {
		out := make([]*types.Param, 0, len(tu.Elements))
		for _, e := range tu.Elements {
			out = append(out, &types.Param{Name: e.Name, Label: e.Name, Type: e.Type})
		}
		return out
	}
	return []*types.Param{{Name: label, Label: label, Type: assoc}}
}

// typeScope is a declared type's member scope: the one that type was
// declared with, or -- for a type not seen declaring one -- the scope of
// the last type of its name.
func (c *checker) typeScope(t types.Type) *Scope {
	if t == nil {
		return nil
	}
	if s := c.scopesByType[t]; s != nil {
		return s
	}
	return c.typeScopes[typeNameOf(t)]
}

// rememberTypeScope records a type's member scope by the type itself.
func (c *checker) rememberTypeScope(t types.Type, s *Scope) {
	if t == nil || s == nil {
		return
	}
	if c.scopesByType == nil {
		c.scopesByType = map[types.Type]*Scope{}
	}
	c.scopesByType[t] = s
}

// existentialSame is t, a requirement's type as protocol p declares it,
// with each primary associated type the existential gives replaced by
// what it gives: next() of `any Source<String>` answers a String.
func existentialSame(ex *types.Existential, p types.Type, t types.Type) types.Type {
	if len(ex.Same) == 0 || t == nil {
		return t
	}
	return types.MapDependents(t, func(d *types.Dependent) types.Type {
		tp, ok := d.Base.(*types.TypeParam)
		if !ok || tp.Name != "Self" {
			return nil
		}
		return ex.Same[d.Name]
	})
}
