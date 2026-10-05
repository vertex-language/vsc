package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// String machine layout (two 64-bit words: count/flags and object/small-string payload).
// Managed via vertex_string_retain and vertex_string_release.
var stringWords = &types.Struct{
	Name: "String",
	Fields: []*types.Field{
		{Name: "_countAndFlags", Type: types.Typ[types.UInt64]},
		{Name: "_object", Type: types.Typ[types.UInt64]},
	},
}

// stringImage is the struct a String's bytes look like.
func stringImage() (*types.Struct, bool) { return stringWords, true }

// stringLiteral emits a null-terminated UTF-8 string literal into read-only storage and returns its pointer.
func (c *fn) stringLiteral(in *sil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	text := in.Aux().Text
	if c.l.strings == nil {
		c.l.strings = map[string]*ir.Global{}
	}
	g, ok := c.l.strings[text]
	if !ok {
		c.l.literals++
		g = c.l.out.Global(c.l.sym("$sVSCstr"+itoa(c.l.literals)), ir.RO,
			ir.Array(uint64(len(text)+1), ir.StoreI8.FType())).
			Init(ir.Str(text)).
			Align(1)
		c.l.strings[text] = g
	}
	c.def(res, c.b.Ptr.GetAddr(g))
	return nil
}

// indexAddr computes the element address by scaling the index by the element type's stride.
func (c *fn) indexAddr(in *sil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	base, err := c.addressOf(in, in.Args()[0])
	if err != nil {
		return err
	}
	index, err := c.operand(in, in.Args()[1])
	if err != nil {
		return err
	}
	n, ok := index.(ir.I64)
	if !ok {
		return c.fail(ErrType, in.Op(), "an index that is not a word")
	}
	elem := in.Args()[0].Type().Formal()
	if elem == nil {
		return c.fail(ErrType, in.Op(), "an address with no element type")
	}
	stride := types.Strideof(elem, types.DefaultTarget64)
	if stride <= 0 {
		return c.fail(ErrType, in.Op(), "an element with no stride")
	}
	c.def(res, c.b.Ptr.Add(base, c.b.I64.Mul(n, c.b.I64.Const(stride))))
	return nil
}

// isStringType reports whether a value is one.
func isStringType(t sil.Type) bool {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return false
	}
	b, ok := t.Formal().Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// bridged reports whether a value requires specialized string refcounting rather than generic object refcounting.
func bridged(t sil.Type) bool {
	if isStringType(t) {
		return true
	}
	o, ok := optionalOfType(t)
	return ok && isStringOptional(o)
}

// isStringOptional reports whether the optional wraps a String.
func isStringOptional(o *types.Optional) bool {
	if o == nil || o.Wrapped == nil {
		return false
	}
	b, ok := o.Wrapped.Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// stringObjectWord is where the reference is, among a String's two.
const stringObjectWord = stdlib.StringObjectWord

// bridgeRefCount manages reference counting for String instances via runtime string retain/release.
func (c *fn) bridgeRefCount(in *sil.Inst, retain bool) error {
	var word ir.Value
	if bridged(in.Args()[0].Type()) {
		if fields, ok := c.parts(in.Args()[0]); ok && len(fields) > stringObjectWord {
			word = fields[stringObjectWord]
		} else if from, ok := c.mem[in.Args()[0]]; ok {
			// One that came back through storage rather than in
			// registers -- an array's element does -- so the word is
			// read out of the storage.
			word = c.b.Ptr.Load(c.fieldAddr(from, stringObjectWord*8))
		} else {
			return c.fail(ErrUnsupported, in.Op(), "a String that is neither in its "+
				"two words nor in storage")
		}
	} else {
		got, err := c.operand(in, in.Args()[0])
		if err != nil {
			return err
		}
		word = got
	}
	ref, err := c.asPointer(in, word)
	if err != nil {
		return err
	}
	slot, name := &c.l.bridgeRelease, stdlib.StringRelease
	if retain {
		slot, name = &c.l.bridgeRetain, stdlib.StringRetain
	}
	if *slot == nil {
		*slot = c.l.runtimeFunc(name, ir.NewSig().Param(ir.TypePtr))
	}
	c.b.Call(*slot, ref)
	return nil
}

// asPointer converts an i64 or pointer IR value to a pointer.
func (c *fn) asPointer(in *sil.Inst, v ir.Value) (ir.Ptr, error) {
	switch got := v.(type) {
	case ir.Ptr:
		return got, nil
	case ir.I64:
		return c.b.Ptr.FromI64(got), nil
	}
	return ir.Ptr{}, c.fail(ErrType, in.Op(), "a reference that is not one")
}

// smallStringLiteral lowers the runtime call a short string literal
// makes -- vertex_string_literal(bytes, count, ascii) -- to the two words
// it would return, and reports whether it did. A String of at most
// fifteen bytes holds them in itself (the runtime's smallString): bytes
// 0-7 little-endian in the first word, 8-14 in the second, and 0xE0 | n
// in the second word's top byte. That is a constant, so there is no call
// to make at run time -- which is what swiftc does for a small literal
// too. A longer literal is its bytes' address, tagged, and its count with
// the ASCII flag: no call either.
func (c *fn) smallStringLiteral(in *sil.Inst) (bool, error) {
	res := in.Result()
	args := in.Args()
	if res == nil || len(args) != 4 || c.refNames[args[0]] != "vertex_string_literal" {
		return false, nil
	}
	lit := args[1].Inst()
	n := args[2].Inst()
	if lit == nil || lit.Op() != sil.StringLiteral || n == nil || n.Op() != sil.IntegerLiteral {
		return false, nil
	}
	text := lit.Aux().Text
	if int64(len(text)) != n.Aux().Int {
		return false, nil
	}
	if len(text) > stringSmallCapacity {
		// A longer literal points at its bytes, which the object file
		// keeps for good: the address tagged as a literal's, and the
		// count with the ASCII flag, which is known here too.
		bytes, ok := c.value(args[1])
		p, isPtr := bytes.(ir.Ptr)
		if !ok || !isPtr {
			return false, nil
		}
		flags := uint64(len(text))
		if isASCII(text) {
			flags |= stringFlagASCII
		}
		obj := c.b.I64.Or(c.b.I64.FromPtr(p), c.b.I64.Const(int64(stringTagLiteral)))
		return true, c.spreadInto(res, []ir.Value{c.b.I64.Const(int64(flags)), obj})
	}
	var w0, w1 uint64
	for i := 0; i < len(text); i++ {
		if i < 8 {
			w0 |= uint64(text[i]) << (8 * i)
		} else {
			w1 |= uint64(text[i]) << (8 * (i - 8))
		}
	}
	w1 |= (stringTagSmall | uint64(len(text))) << 56
	regs := []ir.Value{c.b.I64.Const(int64(w0)), c.b.I64.Const(int64(w1))}
	return true, c.spreadInto(res, regs)
}

// The String forms, as the runtime's abi.h declares them.
const (
	stringSmallCapacity = 15
	stringTagSmall      = 0xE0
	stringTagLiteral    = uint64(0x40) << 56
	stringFlagASCII     = uint64(1) << 63
)

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
