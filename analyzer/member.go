package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"strconv"

	"github.com/vertex-language/vsc/types"
)

// Member lookup: what a name after a dot denotes.
//
// The rule that matters here is when to complain. A type declared in
// the source this compiler is reading has a member list, so a name
// that is not in it is a mistake and is reported. A type modelled
// without members — every builtin, until there is a library to read
// them from — is not evidence of anything, and a member of one is
// Invalid in silence.

// lookupMember finds a member of t by name, or returns nil.
//
// A metatype is looked through: `E.a` names a case of E, and the type
// this compiler builds for `E` in expression position is E's
// metatype. Static and instance members are not yet told apart, which
// is an over-acceptance this will lose when they are.
// lookupMemberFor is lookupMember, recording into Info which method a
// member expression named.
//
// The recording is here rather than at the call site because this is
// where the walk ends: a method may be found on a superclass or through
// a generic instance's base, and only the step that found it knows which
// type that was.
func (c *checker) lookupMemberFor(e *ast.MemberExpr, t types.Type, name string) types.Type {
	got := c.lookupMember(t, name)
	if e != nil && e.Name != nil {
		if recv, m := c.findMethod(t, name); m != nil {
			c.info.Methods[e] = &MethodRef{Recv: recv, Method: m}
		}
		// `Outer.Inner` names a type, and a consumer has to reach the
		// same symbol a bare name would reach.
		if meta, ok := t.(*types.Metatype); ok {
			if scope := c.typeScopes[typeNameOf(meta.Instance)]; scope != nil {
				if sym, ok := scope.LookupLocal(name).(*TypeNameSymbol); ok {
					c.info.Uses[e.Name] = sym
				}
			}
		}
		// `E.b` names a case, and a consumer has to know which one.
		// The symbol is the enum's own scope's, put there when its
		// cases were read, so this records the use rather than making
		// a second symbol for the same case.
		if sym := c.enumCaseSymbol(t, name); sym != nil {
			c.info.Uses[e.Name] = sym
		}
	}
	return got
}

// nestedIn is the type declared inside another under this name, or
// nil. See resolveMemberType, which answers the same question for a
// name written where a type is wanted.
func (c *checker) nestedIn(outer types.Type, name string) types.Type {
	if inst, ok := outer.(*types.GenericInstance); ok {
		outer = inst.Base
	}
	scope := c.typeScopes[typeNameOf(outer)]
	if scope == nil {
		return nil
	}
	sym, _ := scope.LookupLocal(name).(*TypeNameSymbol)
	if sym == nil {
		return nil
	}
	return sym.Type()
}

// enumCaseSymbol is the case of an enum a name refers to, or nil where
// the type is not an enum or has no such case.
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
	scope := c.typeScopes[typeNameOf(t)]
	if scope == nil {
		return nil
	}
	sym, _ := scope.Lookup(name).(*EnumCaseSymbol)
	return sym
}

// findMethod is the method a name refers to and the nominal type that
// declares it, seeing through a generic instance and up a superclass
// chain the way lookupMember does.
func (c *checker) findMethod(t types.Type, name string) (types.Type, *types.Method) {
	if t == nil {
		return nil, nil
	}
	// See lookupMember: a static method is reached through the type
	// and an instance method through an instance, and never the other
	// way round.
	onType := false
	if meta, ok := t.(*types.Metatype); ok {
		onType = true
		t = meta.Instance
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		return c.findMethod(inst.Base, name)
	}
	// A method on a type parameter is one its constraints promise. It
	// resolves to the protocol's requirement rather than to any
	// implementation -- which implementation is a question about the
	// type argument, and there is not one here yet. Lowering asks it
	// again once there is, with the parameter substituted; see
	// vil/gen/generic.go.
	if tp, ok := t.(*types.TypeParam); ok {
		for _, con := range tp.Constraints {
			if found, m := requirementOf(con, name); m != nil {
				sig, _ := throughParam(con, tp, m.Sig).(*types.Signature)
				if sig == nil {
					sig = m.Sig
				}
				return found, &types.Method{Name: m.Name, Sig: sig}
			}
		}
		return nil, nil
	}
	// A method on `C.Item` is one the associated type's own
	// constraints promise: `associatedtype Item: Number` says that
	// whatever Item turns out to be has a value(), and that is the
	// whole of what is known about it here.
	if dep, ok := t.(*types.Dependent); ok {
		for _, con := range associatedConstraints(dep) {
			if found, m := requirementOf(con, name); m != nil {
				sig, _ := throughParam(con, dep, m.Sig).(*types.Signature)
				if sig == nil {
					sig = m.Sig
				}
				return found, &types.Method{Name: m.Name, Sig: sig}
			}
		}
		return nil, nil
	}
	// A method on `any P` is one P promises. Which implementation
	// runs is not decided here and cannot be: the value's type
	// arrives with the value, which is the whole of what an
	// existential is.
	if ex, ok := t.(*types.Existential); ok {
		for _, p := range ex.Protocols {
			if found, m := requirementOf(p, name); m != nil {
				return found, m
			}
		}
		return nil, nil
	}
	// A protocol named where a type is wanted is an existential of
	// itself: `func f(_ p: P)` takes a value that satisfies P, which
	// is what `any P` says the long way.
	if p, ok := t.(*types.Protocol); ok {
		return requirementOf(p, name)
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
		return c.findMethod(b.Superclass, name)
	}
	return nil, nil
}

func (c *checker) lookupMember(t types.Type, name string) types.Type {
	if t == nil {
		return nil
	}
	// Whether the name is being read off the type or off an instance
	// of it, which is what decides whether a static member is in
	// reach. `Vec.zero()` is one and `v.zero()` is not Swift.
	onType := false
	if meta, ok := t.(*types.Metatype); ok {
		onType = true
		t = meta.Instance
		// `Outer.Inner` in expression position names a type declared
		// inside Outer, which is a metatype like any other type's
		// name is. It is looked for only here, through the outer
		// type's own name: an instance has no such member, and
		// `o.Inner` is not Swift.
		if inner := c.nestedIn(t, name); inner != nil {
			return &types.Metatype{Instance: inner}
		}
	}
	// A member of a type parameter is what its constraints promise.
	// Which implementation provides it is a question about the type
	// argument and is answered when there is one; what is needed here
	// is the type, so that the expression around it can be checked.
	if tp, ok := t.(*types.TypeParam); ok {
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
				return member
			}
		}
		return nil
	}
	if p, ok := t.(*types.Protocol); ok {
		return c.requirementType(p, name)
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
	// A primitive with a property the standard library provides:
	// `s.count` and `a.count` are calls rather than fields, because
	// neither a String nor an Array has a field this compiler is
	// allowed to look at. See core.LowerMember.
	if m, ok := core.LowerMember(t, name); ok && !onType {
		return known(m.Result)
	}
	switch b := t.Underlying().(type) {
	// A tuple's elements are named by number, and by their label
	// where the type gave them one.
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
			return c.lookupMember(b.Superclass, name)
		}
	case *types.Enum:
		// `Code.ok.rawValue` is the value the case was declared with.
		// Swift synthesizes the property for an enum that declares a
		// raw type, and nothing here declares it.
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
					return &types.Signature{
						Params:  caseParams(cs.AssociatedType),
						Results: b,
					}
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

// requirementOf is the method a protocol promises under this name,
// through the protocols it inherits as well as its own.
func requirementOf(c types.Type, name string) (types.Type, *types.Method) {
	p, ok := c.(*types.Protocol)
	if !ok {
		if u, ok := c.Underlying().(*types.Protocol); ok {
			p = u
		} else {
			return nil, nil
		}
	}
	for _, r := range p.Requirements {
		if r != nil && r.Name == name && r.Sig != nil {
			return p, &types.Method{Name: r.Name, Sig: r.Sig}
		}
	}
	for _, up := range p.Inherited {
		if found, m := requirementOf(up, name); m != nil {
			return found, m
		}
	}
	return nil, nil
}

// requirementType is the type a protocol gives a name it promises: a
// method's signature, or a property's type.
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
	return nil
}

// caseParams is what a case that carries something takes when it is
// written like a call.
//
// A case carrying several values carries a tuple of them, and `.box(3,
// 4)` writes the elements rather than the tuple: Swift spreads them,
// so the parameters are the elements and not the one tuple they make.
func caseParams(assoc types.Type) []*types.Param {
	if tu, ok := assoc.Underlying().(*types.Tuple); ok && len(tu.Elements) > 0 {
		out := make([]*types.Param, 0, len(tu.Elements))
		for _, e := range tu.Elements {
			out = append(out, &types.Param{Name: e.Name, Label: e.Name, Type: e.Type})
		}
		return out
	}
	return []*types.Param{{Type: assoc}}
}
