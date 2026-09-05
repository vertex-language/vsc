package lower

import (
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// A struct as its scalar parts.
//
// Everything below VIL holds a struct as registers, and a register
// holds a scalar. A struct whose fields are scalars is that list; a
// struct with a struct inside it is the same list with the inner
// one's parts spliced in where it sat. Both are the same question
// asked once, which is why one function answers it for all four
// places a struct is taken apart: building one, reading a field out,
// writing it to memory, and passing it to a call.
//
// The offsets are from the start of the outermost struct, so a nested
// field's is the sum of the two — which is exactly what the memory
// layout says it is, and what makes the register form and the memory
// form agree without either knowing about the other.

// A leaf is one scalar inside a struct: what it is, and where.
type leaf struct {
	typ    types.Type
	offset int64
}

// structLeaves is the scalars a struct is made of, in layout order.
//
// It reports false where a field has no layout, or where a field is
// something this cannot take apart — an array, an existential, a
// closure. Refusing the whole struct in that case is deliberate: half
// a list would put every later leaf in the wrong register.
func structLeaves(st *types.Struct) ([]leaf, bool) {
	return appendLeaves(nil, st, 0)
}

func appendLeaves(out []leaf, st *types.Struct, base int64) ([]leaf, bool) {
	if st == nil {
		return nil, false
	}
	off := base
	for _, f := range st.Fields {
		if f == nil || f.Type == nil {
			return nil, false
		}
		size := types.Sizeof(f.Type, types.DefaultTarget64)
		align := types.Alignof(f.Type, types.DefaultTarget64)
		if size <= 0 || align <= 0 {
			return nil, false
		}
		off = alignUp(off, align)

		// A struct inside a struct contributes its own scalars, at
		// its own offsets plus where it sits.
		if inner, ok := f.Type.Underlying().(*types.Struct); ok {
			var got bool
			out, got = appendLeaves(out, inner, off)
			if !got {
				return nil, false
			}
			off += size
			continue
		}
		if _, ok := machineOf(f.Type); !ok {
			return nil, false
		}
		out = append(out, leaf{typ: f.Type, offset: off})
		off += size
	}
	return out, true
}

// leavesOf is structLeaves for a VIL type, and false for anything
// that is not a struct held in registers.
func leavesOf(t vil.Type) ([]leaf, bool) {
	st, ok := structOf(t)
	if !ok {
		return nil, false
	}
	return structLeaves(st)
}

// fieldLeaves is the range of a struct's leaves that one named field
// owns: [lo, hi). A scalar field owns one; a nested struct owns as
// many as it has.
//
// This is what makes struct_extract work on a nested field without
// anything having to hold a struct whole — the answer is a window
// onto the parts that are already there.
func fieldLeaves(t vil.Type, member string) (lo, hi int, ok bool) {
	st, found := structOf(t)
	if !found {
		return 0, 0, false
	}
	name := memberName(member)
	at := 0
	for _, f := range st.Fields {
		if f == nil || f.Type == nil {
			return 0, 0, false
		}
		n := 1
		if inner, isStruct := f.Type.Underlying().(*types.Struct); isStruct {
			ls, got := structLeaves(inner)
			if !got {
				return 0, 0, false
			}
			n = len(ls)
		}
		if f.Name == name {
			return at, at + n, true
		}
		at += n
	}
	return 0, 0, false
}
