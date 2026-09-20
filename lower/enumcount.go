package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// A payload enum whose cases carry references is counted case by case:
// what a copy retains and a destroy releases is what the active case
// carries, found by its tag. A switch hands a bound payload to its arm,
// which owns it from then on; an arm that binds nothing, or the default,
// lets it go on the way there.

// enumCounted reports whether any of an enum's cases carries something
// counted.
func enumCounted(e *types.Enum) bool {
	for _, k := range e.Cases {
		if k != nil && k.AssociatedType != nil && !sil.Object(sil.CaseStorage(k)).Trivial() {
			return true
		}
	}
	return false
}

// caseOwned is where the references a case carries sit inside the enum,
// which is where they sit inside the payload: it starts at zero.
func caseOwned(payload types.Type) ([]ownedWord, bool) {
	if payload == nil || sil.Object(payload).Trivial() {
		return nil, true
	}
	if tu, ok := payload.Underlying().(*types.Tuple); ok {
		image, ok := tupleImage(tu)
		if !ok {
			return nil, false
		}
		return ownedWords(image, 0)
	}
	return ownedWords(&types.Struct{Fields: []*types.Field{{Name: "payload", Type: payload}}}, 0)
}

// enumWords is a payload enum's words, wherever the value is held.
func (c *fn) enumWords(in *sil.Inst, v *sil.Value, e *types.Enum) ([]ir.Value, error) {
	if parts, ok := c.parts(v); ok {
		return parts, nil
	}
	if from, ok := c.mem[v]; ok {
		image, ok := enumImage(e)
		if !ok {
			return nil, c.fail(ErrUnsupported, in.Op(), e.Name)
		}
		words := make([]ir.Value, len(image.Fields))
		for i := range words {
			words[i] = c.b.I64.Load(c.fieldAddr(from, int64(i*8)))
		}
		return words, nil
	}
	got, err := c.operand(in, v)
	if err != nil {
		return nil, err
	}
	return []ir.Value{got}, nil
}

// countPayload retains or releases what one case carries, read out of
// the enum's words.
func (c *fn) countPayload(in *sil.Inst, words []ir.Value, payload types.Type, retain bool) error {
	owned, ok := caseOwned(payload)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(),
			"a case carrying something whose copy is not a retain: "+payload.String())
	}
	objects, strings := stdlib.Release, stdlib.StringRelease
	if retain {
		objects, strings = stdlib.Retain, stdlib.StringRetain
	}
	for _, w := range owned {
		if w.enum != nil {
			// An enum inside the payload: its own words, at its offset.
			image, ok := enumImage(w.enum)
			lo := int(w.offset / 8)
			if !ok || w.offset%8 != 0 || lo+len(image.Fields) > len(words) {
				return c.fail(ErrUnsupported, in.Op(), "an enum inside a payload that is not in whole words")
			}
			if err := c.countEnumWords(in, w.enum, words[lo:lo+len(image.Fields)], retain); err != nil {
				return err
			}
			continue
		}
		word, err := c.readFromWords(in, words, types.Typ[types.Int64], w.offset)
		if err != nil {
			return err
		}
		p, err := c.asPointer(in, word)
		if err != nil {
			return err
		}
		name := objects
		if w.string {
			name = strings
		}
		c.b.Call(c.l.runtimeFunc(name, ir.NewSig().Param(ir.TypePtr)), p)
	}
	return nil
}

// enumRefCount is a retain or a release of a payload enum: of what its
// active case carries, and of nothing when that case carries nothing
// counted.
func (c *fn) enumRefCount(in *sil.Inst, e *types.Enum, retain bool) error {
	words, err := c.enumWords(in, in.Args()[0], e)
	if err != nil {
		return err
	}
	return c.countEnumWords(in, e, words, retain)
}

// countEnumWords retains or releases what the active case of the enum
// whose words these are carries.
func (c *fn) countEnumWords(in *sil.Inst, e *types.Enum, words []ir.Value, retain bool) error {
	var tag ir.I32
	if len(e.Cases) > 1 {
		got, err := c.readFromWords(in, words, types.Typ[types.UInt8], payloadArea(e))
		if err != nil {
			return err
		}
		t, ok := got.(ir.I32)
		if !ok {
			return c.fail(ErrType, in.Op(), "a tag this cannot compare")
		}
		tag = t
	} else {
		tag = c.b.I32.Const(0)
	}
	c.conts++
	n := itoa(c.conts)
	done := c.out.Block("counted" + n)
	for i, k := range e.Cases {
		if k == nil || k.AssociatedType == nil || sil.Object(sil.CaseStorage(k)).Trivial() {
			continue
		}
		t, ok := enumTagOf(e, k.Name)
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "no case "+k.Name+" in "+e.Name)
		}
		body := c.out.Block("case" + n + "_" + itoa(i))
		next := c.out.Block("next" + n + "_" + itoa(i))
		c.b.BrIf(c.b.I32.Eq(tag, c.b.I32.Const(t)), body.To(), next.To())
		c.b = body
		if err := c.countPayload(in, words, sil.CaseStorage(k), retain); err != nil {
			return err
		}
		c.b.Br(done.To())
		c.b = next
	}
	c.b.Br(done.To())
	c.b = done
	return nil
}

// optionalEnumRefCount is a retain or a release of an optional payload
// enum: of what the enum's active case carries when there is an enum,
// and of nothing when the optional is empty. The enum's words come
// first and the tag after them, zero where there is a value.
func (c *fn) optionalEnumRefCount(in *sil.Inst, e *types.Enum, retain bool) error {
	image, ok := enumImage(e)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), e.Name)
	}
	parts, held := c.parts(in.Args()[0])
	if !held {
		// Too wide for registers: the words and the tag are read out of
		// where it lives.
		if from, inMemory := c.mem[in.Args()[0]]; inMemory {
			parts = nil
			for i := range image.Fields {
				parts = append(parts, c.b.I64.Load(c.fieldAddr(from, int64(i)*8)))
			}
			parts = append(parts, c.b.I32.ULoad8(c.fieldAddr(from, int64(len(image.Fields))*8)))
			held = true
		}
	}
	if !held || len(parts) != len(image.Fields)+1 {
		return c.fail(ErrUnsupported, in.Op(), "an optional enum that is not its words and a tag")
	}
	tag, ok := c.toWord(parts[len(parts)-1], 8)
	if !ok {
		return c.fail(ErrType, in.Op(), "a tag this cannot read")
	}
	c.conts++
	n := itoa(c.conts)
	present := c.out.Block("present" + n)
	done := c.out.Block("absent" + n)
	c.b.BrIf(c.b.I64.Eq(tag, c.b.I64.Const(0)), present.To(), done.To())
	c.b = present
	if err := c.enumRefCount(in, e, retain); err != nil {
		return err
	}
	c.b.Br(done.To())
	c.b = done
	return nil
}

// dropEdge is the edge into an arm that does not take what its case
// carries: it releases that, then branches.
func (c *fn) dropEdge(in *sil.Inst, words []ir.Value, payload types.Type, dest *ir.Block) (*ir.Block, error) {
	c.conts++
	edge := c.out.Block("dropped" + itoa(c.conts))
	cur := c.b
	c.b = edge
	err := c.countPayload(in, words, payload, false)
	if err == nil {
		c.b.Br(dest.To())
	}
	c.b = cur
	return edge, err
}

// enumValueWitnessTable is the value witness table of a payload enum whose
// cases carry counted things: each witness reads the tag and retains or
// releases what that case carries, or nothing for a case that carries
// nothing counted.
func (l *lowerer) enumValueWitnessTable(info sil.TypeMetadata, e *types.Enum, size, align int64) (*ir.Global, bool) {
	name := info.Mangled + "WV"
	if g, ok := l.vwts[name]; ok {
		return g, true
	}
	type counted struct {
		tag   int64
		owned []ownedWord
	}
	var cases []counted
	for _, k := range e.Cases {
		if k == nil || k.AssociatedType == nil || sil.Object(sil.CaseStorage(k)).Trivial() {
			continue
		}
		tag, ok := enumTagOf(e, k.Name)
		if !ok {
			return nil, false
		}
		owned, ok := caseOwned(sil.CaseStorage(k))
		if !ok {
			return nil, false
		}
		cases = append(cases, counted{tag: tag, owned: owned})
	}
	_ = cases
	retain := l.runtimeFunc(stdlib.Retain, ir.NewSig().Param(ir.TypePtr))
	release := l.runtimeFunc(stdlib.Release, ir.NewSig().Param(ir.TypePtr))
	retainString := l.runtimeFunc(stdlib.StringRetain, ir.NewSig().Param(ir.TypePtr))
	releaseString := l.runtimeFunc(stdlib.StringRelease, ir.NewSig().Param(ir.TypePtr))

	// each counts what the value's active case carries, starting in b,
	// and is the block that follows: the enum is one owned thing, at the
	// start of the value.
	each := func(f *ir.Func, b *ir.Block, value ir.Ptr, strings, objects ir.Callee, label string) *ir.Block {
		return countOwned(f, b, value, []ownedWord{{offset: 0, enum: e}}, strings, objects, label)
	}
	fn := func(suffix string) *ir.Func {
		f := l.out.Func(l.sym(info.Mangled + suffix))
		f.Internal()
		return f
	}

	copyFn := fn("WVcopy")
	{
		dst, src := copyFn.ParamPtr("dest"), copyFn.ParamPtr("src")
		copyFn.ParamPtr("metadata")
		copyFn.ReturnsPtr()
		b := copyFn.Entry()
		b.MemCpy(dst, src, b.I64.Const(size))
		each(copyFn, b, dst, retainString, retain, "r").Return(dst)
	}
	destroyFn := fn("WVdestroy")
	{
		v := destroyFn.ParamPtr("value")
		destroyFn.ParamPtr("metadata")
		b := destroyFn.Entry()
		each(destroyFn, b, v, releaseString, release, "d").Return()
	}
	// Retain what arrives before releasing what leaves, so that assigning
	// a value to itself keeps it alive.
	assignCopyFn := fn("WVassigncopy")
	{
		dst, src := assignCopyFn.ParamPtr("dest"), assignCopyFn.ParamPtr("src")
		assignCopyFn.ParamPtr("metadata")
		assignCopyFn.ReturnsPtr()
		b := assignCopyFn.Entry()
		b = each(assignCopyFn, b, src, retainString, retain, "s")
		b = each(assignCopyFn, b, dst, releaseString, release, "t")
		b.MemCpy(dst, src, b.I64.Const(size))
		b.Return(dst)
	}
	takeFn := l.memcpyWitness(size)
	assignTakeFn := fn("WVassigntake")
	{
		dst, src := assignTakeFn.ParamPtr("dest"), assignTakeFn.ParamPtr("src")
		assignTakeFn.ParamPtr("metadata")
		assignTakeFn.ReturnsPtr()
		b := assignTakeFn.Entry()
		b = each(assignTakeFn, b, dst, releaseString, release, "d")
		b.MemCpy(dst, src, b.I64.Const(size))
		b.Return(dst)
	}
	get, store := l.enumTagWitnesses()

	rows := make([]ir.Init, vwWords)
	rows[vwInitBufferWithCopyOfBuffer] = ir.RelocInit(copyFn)
	rows[vwDestroy] = ir.RelocInit(destroyFn)
	rows[vwInitWithCopy] = ir.RelocInit(copyFn)
	rows[vwAssignWithCopy] = ir.RelocInit(assignCopyFn)
	rows[vwInitWithTake] = ir.RelocInit(takeFn)
	rows[vwAssignWithTake] = ir.RelocInit(assignTakeFn)
	rows[vwGetEnumTag] = ir.RelocInit(get)
	rows[vwStoreEnumTag] = ir.RelocInit(store)
	rows[vwSize] = ir.Lit(ir.Int(size))
	stride := alignUp(size, align)
	if stride == 0 {
		stride = 1
	}
	rows[vwStride] = ir.Lit(ir.Int(stride))
	rows[vwFlags] = ir.Lit(ir.Int(witnessFlags(size, align, true)))

	g := l.out.Global(l.sym(name), ir.RO,
		ir.Array(vwWords, ir.StorePtr.FType())).Init(ir.List(rows...)).Align(8)
	if l.vwts == nil {
		l.vwts = map[string]*ir.Global{}
	}
	l.vwts[name] = g
	return g, true
}

// enumCountable reports whether what each case of e carries can be counted
// word by word, so that the enum can be a field of something counted.
func enumCountable(e *types.Enum) bool {
	for _, k := range e.Cases {
		if k == nil || k.AssociatedType == nil || sil.Object(sil.CaseStorage(k)).Trivial() {
			continue
		}
		if _, ok := caseOwned(sil.CaseStorage(k)); !ok {
			return false
		}
	}
	return true
}
