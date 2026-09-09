package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// What a String is, on the machine.
//
// Two words, and swiftc's own code says which: `take(_ s: String)`
// reads x0 and x1, and a function returning one leaves both. The
// first is the count and the flags, the second is the object -- a
// pointer to storage, or the string's own bytes when there are few
// enough of them to fit.
//
// So a String crosses a call exactly as a struct of two words does,
// which is why this is an image rather than a case of its own: the
// packing, the unpacking, the two registers and the memory layout are
// all the ones structOf already answers with. What Swift keeps in
// those two words is Swift's business and nothing here reads it.
//
// # What owns what
//
// The second word may be a reference. A literal's is not -- swiftc
// sets the high bit, which says immortal, and a release of one does
// nothing -- but a String a library builds and hands back is a heap
// allocation with a count on it. Both are let go of the same way,
// through swift_bridgeObjectRelease, which is what the release of a
// String lowers to. See refCount.
var stringWords = &types.Struct{
	Name: "String",
	Fields: []*types.Field{
		{Name: "_countAndFlags", Type: types.Typ[types.UInt64]},
		{Name: "_object", Type: types.Typ[types.UInt64]},
	},
}

// stringImage is the struct a String's bytes look like.
func stringImage() (*types.Struct, bool) { return stringWords, true }

// stringLiteral puts a literal's bytes in the object file and yields
// their address.
//
// The bytes and nothing else: the String is built from them by
// Swift's own initializer, which the call after this one reaches. So
// what this has to get right is only that the bytes are there, that
// they are read-only, and that the same literal written twice is one
// copy -- and the NUL after them, which swiftc writes and which costs
// nothing to match.
func (c *fn) stringLiteral(in *vil.Inst) error {
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

// indexAddr is the address of one element of an array of them: the
// base plus as many strides as the index says.
//
// The stride and not the size: an array's elements are laid out at
// their stride, which is the size rounded up to the alignment, and
// two of them would overlap if this used the size of a type whose
// size and stride differ.
func (c *fn) indexAddr(in *vil.Inst) error {
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
func isStringType(t vil.Type) bool {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return false
	}
	b, ok := t.Formal().Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// bridged reports whether a value is counted by Swift's own bridge
// object retain and release rather than by this compiler's.
//
// A String is, and so is an Array: what each holds is a
// _BridgeStorage, whose reference may be a native object, a tagged
// value, or something immortal, and only Swift's own release knows
// which. Calling vertex_release on one would decrement a word that is
// not a reference count.
func bridged(t vil.Type) bool {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return false
	}
	if _, isArray := t.Formal().Underlying().(*types.Array); isArray {
		return true
	}
	return isStringType(t)
}

// stringObjectWord is where the reference is, among a String's two.
const stringObjectWord = 1

// bridgeRefCount retains or releases whatever a String or an Array
// holds.
//
// Through swift_bridgeObjectRetain and swift_bridgeObjectRelease, and
// not through this compiler's: the object was allocated by Swift, and
// it may not be an allocation at all -- a string literal's second
// word has a bit that says immortal, and the bridge release is the
// function that knows to look at it.
//
// A String is two words and the reference is the second; an Array is
// one and the reference is all of it.
func (c *fn) bridgeRefCount(in *vil.Inst, retain bool) error {
	var word ir.Value
	if isStringType(in.Args()[0].Type()) {
		if fields, ok := c.multi[in.Args()[0]]; ok && len(fields) > stringObjectWord {
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
	slot, name := &c.l.bridgeRelease, "swift_bridgeObjectRelease"
	if retain {
		slot, name = &c.l.bridgeRetain, "swift_bridgeObjectRetain"
	}
	if *slot == nil {
		*slot = c.l.out.ImportFunc(c.l.sym(name),
			ir.NewSig().Param(ir.TypePtr).Ret(ir.TypePtr)).NoUnwind()
	}
	c.b.Call(*slot, ref)
	return nil
}

// asPointer is a reference held in whichever register the value it
// came out of uses: a String's words are words, and an Array is a
// pointer already.
func (c *fn) asPointer(in *vil.Inst, v ir.Value) (ir.Ptr, error) {
	switch got := v.(type) {
	case ir.Ptr:
		return got, nil
	case ir.I64:
		return c.b.Ptr.FromI64(got), nil
	}
	return ir.Ptr{}, c.fail(ErrType, in.Op(), "a reference that is not one")
}
