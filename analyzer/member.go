package analyzer

import (
	"github.com/vertex-language/vsc/ast"
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
	if e != nil {
		if recv, m := c.findMethod(t, name); m != nil {
			c.info.Methods[e] = &MethodRef{Recv: recv, Method: m}
		}
	}
	return got
}

// findMethod is the method a name refers to and the nominal type that
// declares it, seeing through a generic instance and up a superclass
// chain the way lookupMember does.
func (c *checker) findMethod(t types.Type, name string) (types.Type, *types.Method) {
	if t == nil {
		return nil, nil
	}
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		return c.findMethod(inst.Base, name)
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
		if m.Name == name {
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
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
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
		for _, m := range b.Methods {
			if m.Name == name {
				return m.Sig
			}
		}
	case *types.Class:
		for _, f := range b.Fields {
			if f.Name == name {
				return known(f.Type)
			}
		}
		for _, m := range b.Methods {
			if m.Name == name {
				return m.Sig
			}
		}
		if b.Superclass != nil {
			return c.lookupMember(b.Superclass, name)
		}
	case *types.Enum:
		for _, cs := range b.Cases {
			if cs.Name == name {
				if cs.AssociatedType != nil {
					return &types.Signature{
						Params:  []*types.Param{{Type: cs.AssociatedType}},
						Results: b,
					}
				}
				return b
			}
		}
		for _, m := range b.Methods {
			if m.Name == name {
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
