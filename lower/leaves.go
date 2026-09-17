package lower

import (
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// leaf decomposition flattens aggregate struct types into scalar register sequences
// with relative byte offsets for register allocation and memory access.

// leaf describes a single scalar field within an aggregate and its byte offset.
type leaf struct {
	typ    types.Type
	offset int64
}

// structLeaves flattens a struct's fields into scalar leaves in layout order.
// Returns false if any field lacks a known scalar layout.
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
		// A field of no bytes -- a one-case enum, an empty struct -- holds
		// nothing and has no leaf, as Swift leaves it out of a value's
		// explosion. fieldLeaves gives it an empty range to match.
		if size == 0 && align > 0 {
			continue
		}
		if size <= 0 || align <= 0 {
			return nil, false
		}
		off = alignUp(off, align)

		// Recursively flatten nested structs and String aggregates (two words).
		inner, ok := f.Type.Underlying().(*types.Struct)
		if b, isBasic := f.Type.Underlying().(*types.Basic); isBasic && b.Kind() == types.String {
			inner, ok = stringWords, true
		}
		if _, isFunc := f.Type.Underlying().(*types.Signature); isFunc {
			inner, ok = funcWords, true
		}
		// A payload enum is its words. What follows it starts at its
		// size, in the part of its last word the enum does not use.
		if e, isEnum := f.Type.Underlying().(*types.Enum); isEnum && hasPayload(e) {
			inner, ok = enumImage(e)
		}
		// An optional is laid out as what it wraps: a String's or a
		// function's two words, which spare a representation for nil,
		// or the payload with a tag byte after it.
		if o, isOpt := f.Type.Underlying().(*types.Optional); isOpt {
			switch {
			case isStringOptional(o):
				inner, ok = stringWords, true
			case isFuncOptional(o):
				inner, ok = funcWords, true
			default:
				if st, _, spare := spareStructOptional(o); spare {
					inner, ok = st, true
				} else if image, tagged := optionalImage(o); tagged {
					inner, ok = image, true
				}
			}
		}
		if ok {
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

// leavesOf returns structLeaves for a SIL struct type.
func leavesOf(t sil.Type) ([]leaf, bool) {
	st, ok := structOf(t)
	if !ok {
		return nil, false
	}
	return structLeaves(st)
}

// fieldLeaves returns the half-open range [lo, hi) of flattened leaves occupied by member.
func fieldLeaves(t sil.Type, member string) (lo, hi int, ok bool) {
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
		if types.Sizeof(f.Type, types.DefaultTarget64) == 0 {
			// No bytes, no leaf. See appendLeaves.
			n = 0
		} else if b, isBasic := f.Type.Underlying().(*types.Basic); isBasic && b.Kind() == types.String {
			n = 2
		} else if _, isFunc := f.Type.Underlying().(*types.Signature); isFunc {
			// A function value is its code and its context. See funcWords.
			n = 2
		} else if e, isEnum := f.Type.Underlying().(*types.Enum); isEnum && hasPayload(e) {
			image, got := enumImage(e)
			if !got {
				return 0, 0, false
			}
			n = len(image.Fields)
		} else if o, isOpt := f.Type.Underlying().(*types.Optional); isOpt {
			// As appendLeaves lays an optional out.
			if isStringOptional(o) || isFuncOptional(o) {
				n = 2
			} else if st, _, spare := spareStructOptional(o); spare {
				ls, got := structLeaves(st)
				if !got {
					return 0, 0, false
				}
				n = len(ls)
			} else if image, tagged := optionalImage(o); tagged {
				ls, got := structLeaves(image)
				if !got {
					return 0, 0, false
				}
				n = len(ls)
			}
		} else if inner, isStruct := f.Type.Underlying().(*types.Struct); isStruct {
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
