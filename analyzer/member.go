package analyzer

import (
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
