package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Struct and composite parameter/return ABI lowering.
// Structs up to four 64-bit words are packed into direct registers;
// wider composites and existentials are passed indirectly by address.

// maxDirectWords is the maximum number of registers used for direct struct passing.
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

// structWords computes the register word layout for direct struct passing.
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

// directWords returns the number of direct registers used to pass t, if multi-field struct.
func directWords(t sil.Type) (int, bool) {
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

// packStruct packs field registers into word-sized registers for calling conventions.
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
	case ir.F16:
		wide = c.b.I64.ZExtI32(c.b.I32.BitcastF16(got))
	case ir.BF16:
		wide = c.b.I64.ZExtI32(c.b.I32.BitcastBF16(got))
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
	case ir.TypeF16:
		return c.b.F16().BitcastI32(c.b.I32.WrapI64(bits)), true
	case ir.TypeBF16:
		return c.b.BF16().BitcastI32(c.b.I32.WrapI64(bits)), true
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

// indirect reports whether t is passed/returned by address and its byte size.
func indirect(t sil.Type) (size int64, yes bool) {
	if n, ok := existentialBytes(t); ok {
		return n, true
	}
	st, ok := structOf(t)
	if !ok || st == nil || len(st.Fields) == 0 {
		return 0, false
	}
	if _, ok := structLeaves(st); !ok {
		return 0, false
	}
	n := types.Sizeof(st, types.DefaultTarget64)
	if n <= 0 || n <= maxDirectWords*types.DefaultTarget64.WordSize {
		return 0, false
	}
	return n, true
}

// existentialBytes is how many bytes an existential occupies, and
// whether the type was one.
func existentialBytes(t sil.Type) (int64, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return 0, false
	}
	switch f := t.Formal().(type) {
	case *types.Existential:
		return types.Sizeof(f, types.DefaultTarget64), true
	case *types.Protocol:
		return types.Sizeof(f, types.DefaultTarget64), true
	}
	return 0, false
}

// outResult reports whether an instruction's result is returned via indirect memory storage.
func (c *fn) outResult(in *sil.Inst, v *sil.Value) (int64, bool) {
	if n, yes := indirect(v.Type()); yes {
		return n, true
	}
	switch in.Op() {
	case sil.Apply, sil.TryApply:
	default:
		return 0, false
	}
	// A call whose result is several registers, on a convention that
	// returns one. See splitResult.
	if n, split := c.l.splitResult(v.Type()); split {
		return n, true
	}
	args := in.Args()
	if len(args) == 0 || args[0] == nil || !args[0].Type().IsValid() {
		return 0, false
	}
	ft, _ := args[0].Type().Formal().(*sil.FuncType)
	if ft == nil || len(ft.Results) == 0 || ft.Results[0].Convention != sil.ResultOut {
		return 0, false
	}
	return storageBytes(v.Type())
}

// storageBytes is how much room a value needs in memory, and whether
// its type has a size at all.
func storageBytes(t sil.Type) (int64, bool) {
	if !t.IsValid() || t.Formal() == nil {
		return 0, false
	}
	n := types.Sizeof(t.Formal(), types.DefaultTarget64)
	if n <= 0 {
		return 0, false
	}
	return n, true
}

// storageAlign is what such a value has to be aligned to.
func storageAlign(t sil.Type) int64 {
	if !t.IsValid() || t.Formal() == nil {
		return 8
	}
	if a := types.Alignof(t.Formal(), types.DefaultTarget64); a > 0 {
		return a
	}
	return 8
}

// indirectAlign is what such a value has to be aligned to.
func indirectAlign(t sil.Type) int64 {
	if _, ok := existentialBytes(t); ok {
		return 8
	}
	st, ok := structOf(t)
	if !ok || st == nil {
		return 8
	}
	if a := types.Alignof(st, types.DefaultTarget64); a > 0 {
		return a
	}
	return 8
}

// tupleParts returns the register representation for each tuple element.
func tupleParts(t sil.Type) ([]repr, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, false
	}
	tu, ok := t.Formal().Underlying().(*types.Tuple)
	if !ok || len(tu.Elements) == 0 {
		return nil, false
	}
	out := make([]repr, 0, len(tu.Elements))
	for _, e := range tu.Elements {
		if e == nil || e.Type == nil {
			return nil, false
		}
		// An element of no bytes travels in no register.
		if types.Sizeof(e.Type, types.DefaultTarget64) == 0 {
			continue
		}
		r, ok := machineOf(e.Type)
		if !ok {
			return nil, false
		}
		out = append(out, r)
	}
	return out, true
}

// frameRegs is the registers a value travels in across a call: nothing
// for a type that holds nothing, one pointer for a value passed by
// address, a register per element for a tuple, the memory image cut
// into words for a struct of several fields, and one register for
// everything else.
//
// It says the same thing as the parameter half of lowerer.signature,
// and has to keep saying it: an async function's frame keeps a value in
// exactly this form, because the frame is written by a prologue that
// was handed the value in registers and by a resume stub that was
// handed a call's answer in them. See lower/async.go.
func frameRegs(t sil.Type) ([]ir.RegType, bool) {
	if empty(t) {
		return nil, true
	}
	if _, yes := indirect(t); yes {
		return []ir.RegType{ir.TypePtr}, true
	}
	if rs, ok := tupleParts(t); ok {
		out := make([]ir.RegType, 0, len(rs))
		for _, r := range rs {
			out = append(out, r.reg)
		}
		return out, true
	}
	if n, ok := directWords(t); ok {
		out := make([]ir.RegType, n)
		for i := range out {
			out[i] = ir.TypeI64
		}
		return out, true
	}
	r, ok := machine(t)
	if !ok {
		return nil, false
	}
	return []ir.RegType{r.reg}, true
}
