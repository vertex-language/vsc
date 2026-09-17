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
