package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// How a struct crosses a call boundary.
//
// Swift's answer, read off what `swiftc -O` emits for AArch64 rather
// than guessed at:
//
//	struct W3 { a, b, c: Int64 }    take3 → mov x0, x2      three registers
//	struct W4 { a…d: Int64 }        take4 → mov x0, x3      four registers
//	struct W5 { a…e: Int64 }        take5 → ldr x0, [x0,#32]  a pointer
//
// So a struct of at most four words is passed in that many registers,
// and anything wider is passed by address. The registers are not one
// per field: they are the struct's own memory image, cut into words.
// `struct Mixed { a: Int32; b: Bool }` arrives in a single register
// and Swift reads the flag out of it with `ubfx x0, x0, #32, #1`,
// which is bit 32 — where the layout put it, not where a second
// register would have been.
//
// That is why this works from Offsetof and Sizeof rather than from
// the field list: the packing and the memory layout have to be the
// same thing, or a struct written through a pointer and read out of
// registers would disagree with itself.
//
// Natural alignment is what makes it tractable — a field is aligned
// to its own size, so no field straddles a word — and it is checked
// rather than assumed, because a packed layout would break the rule
// and produce a silent misplacement of every field after it.

// maxDirectWords is how many registers a struct may be passed in
// before it is passed by address. Four, which is Swift's number.
const maxDirectWords = 4

// A part is one scalar's place inside a word: which of the struct's
// leaves, how far up from the bottom of the word, and how wide.
type part struct {
	field int
	shift uint
	width uint
}

// A word is one register's worth of a struct.
type word struct{ parts []part }

// structWords is how a struct is passed, or false where it is not
// passed in registers at all — because it is too wide, because its
// layout is unknown, or because a scalar straddles a word.
//
// It works from the same leaves everything else does, so a struct
// with a struct inside it packs exactly as the flattened one would:
// the nesting is a fact about the source and not about the registers.
func structWords(st *types.Struct) ([]word, bool) {
	ls, ok := structLeaves(st)
	if !ok || len(ls) == 0 {
		return nil, false
	}
	size := int64(0)
	for _, l := range ls {
		end := l.offset + types.Sizeof(l.typ, types.DefaultTarget64)
		if end > size {
			size = end
		}
	}
	n := int((size + 7) / 8)
	if n <= 0 || n > maxDirectWords {
		return nil, false
	}

	words := make([]word, n)
	for i, l := range ls {
		w := int(l.offset / 8)
		shift := uint(l.offset%8) * 8
		width := uint(types.Sizeof(l.typ, types.DefaultTarget64)) * 8
		// A scalar that would run past the end of its word is one
		// this cannot place. Natural alignment forbids it; a layout
		// that did not follow it would produce one, and guessing
		// would put every later scalar in the wrong place.
		if width == 0 || width > 64 || shift+width > 64 || w >= n {
			return nil, false
		}
		words[w].parts = append(words[w].parts, part{field: i, shift: shift, width: width})
	}
	return words, true
}

// alignUp rounds up to a power-of-two boundary.
func alignUp(n, align int64) int64 {
	if align <= 1 {
		return n
	}
	return (n + align - 1) &^ (align - 1)
}

// directWords is how many registers a type is passed in, and whether
// it is one of the types this applies to at all.
//
// Only a struct of more than one field: a single-field struct is that
// field and already has a register of its own, and everything else
// either fits in one or is an address.
func directWords(t vil.Type) (int, bool) {
	ls, ok := leavesOf(t)
	if !ok || len(ls) < 2 {
		return 0, false
	}
	st, _ := structOf(t)
	words, ok := structWords(st)
	if !ok {
		return 0, false
	}
	return len(words), true
}

// packStruct builds the registers a struct is passed in out of the
// registers its fields are held in.
//
// Each word is the OR of its fields, each widened to a word, masked
// to its own width so that a negative value does not spill its sign
// into the field above it, and shifted to where the layout put it.
func (c *fn) packStruct(fields []ir.Value, words []word) ([]ir.Value, bool) {
	out := make([]ir.Value, 0, len(words))
	for _, w := range words {
		var acc ir.I64
		first := true
		for _, p := range w.parts {
			if p.field >= len(fields) {
				return nil, false
			}
			bits, ok := c.toWord(fields[p.field], p.width)
			if !ok {
				return nil, false
			}
			if p.shift != 0 {
				bits = c.b.I64.Shl(bits, c.b.I64.Const(int64(p.shift)))
			}
			if first {
				acc, first = bits, false
				continue
			}
			acc = c.b.I64.Or(acc, bits)
		}
		if first {
			// A word no field reached. It holds nothing, and zero is
			// the only honest thing to send.
			acc = c.b.I64.Const(0)
		}
		out = append(out, acc)
	}
	return out, true
}

// unpackStruct takes the scalars back out of the registers a struct
// arrived in, which is packStruct read backwards.
func (c *fn) unpackStruct(regs []ir.Value, words []word, ls []leaf) ([]ir.Value, bool) {
	fields := make([]ir.Value, len(ls))
	for wi, w := range words {
		if wi >= len(regs) {
			return nil, false
		}
		reg, ok := regs[wi].(ir.I64)
		if !ok {
			return nil, false
		}
		for _, p := range w.parts {
			bits := reg
			if p.shift != 0 {
				bits = c.b.I64.UShr(bits, c.b.I64.Const(int64(p.shift)))
			}
			if p.field >= len(ls) {
				return nil, false
			}
			got, ok := c.fromWord(bits, ls[p.field].typ, p.width)
			if !ok {
				return nil, false
			}
			fields[p.field] = got
		}
	}
	for _, f := range fields {
		if f == nil {
			return nil, false
		}
	}
	return fields, true
}

// toWord widens one field's register to a word, keeping only the bits
// the field owns.
func (c *fn) toWord(v ir.Value, width uint) (ir.I64, bool) {
	var wide ir.I64
	switch got := v.(type) {
	case ir.I64:
		wide = got
	case ir.I32:
		wide = c.b.I64.ZExtI32(got)
	case ir.I1:
		wide = c.b.I64.ZExtI1(got)
	case ir.Ptr:
		wide = c.b.I64.FromPtr(got)
	case ir.F64:
		wide = c.b.I64.BitcastF64(got)
	case ir.F32:
		wide = c.b.I64.ZExtI32(c.b.I32.BitcastF32(got))
	default:
		return ir.I64{}, false
	}
	// Masked because a widened value may carry more bits than the
	// field owns — a sign-extended Int8 in an i32 register is all ones
	// above its own width, and those ones belong to the next field.
	if width < 64 {
		wide = c.b.I64.And(wide, c.b.I64.Const(int64((uint64(1)<<width)-1)))
	}
	return wide, true
}

// fromWord narrows a word back to the register a field is held in.
func (c *fn) fromWord(bits ir.I64, t types.Type, width uint) (ir.Value, bool) {
	r, ok := machineOf(t)
	if !ok {
		return nil, false
	}
	switch r.reg {
	case ir.TypeI64:
		return bits, true
	case ir.TypeI32:
		// The field's own width first, then the register's: an Int8
		// is eight bits of the word, sign-extended into i32 the way
		// every other narrow value in this package is held.
		narrow := c.b.I32.WrapI64(bits)
		if width < 32 {
			narrow = c.narrowTo(narrow, width, r.signed)
		}
		return narrow, true
	case ir.TypeI1:
		return c.b.I64.Eq(c.b.I64.And(bits, c.b.I64.Const(1)), c.b.I64.Const(1)), true
	case ir.TypePtr:
		return c.b.Ptr.FromI64(bits), true
	case ir.TypeF64:
		return c.b.F64.BitcastI64(bits), true
	case ir.TypeF32:
		return c.b.F32.BitcastI32(c.b.I32.WrapI64(bits)), true
	}
	return nil, false
}

// narrowTo brings a value in an i32 register down to its declared
// width, the way a load of that width would have left it.
func (c *fn) narrowTo(v ir.I32, width uint, signed bool) ir.I32 {
	shift := c.b.I32.Const(int64(32 - width))
	if signed {
		return c.b.I32.SShr(c.b.I32.Shl(v, shift), shift)
	}
	return c.b.I32.And(v, c.b.I32.Const(int64((uint32(1)<<width)-1)))
}
