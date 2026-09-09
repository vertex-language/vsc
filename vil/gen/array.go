package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// An array literal.
//
// Three steps, which are SILGen's own for the same source: ask the
// standard library for storage that has not been written to yet,
// write the elements into it, and hand it back to be finished.
//
//	%1 = function_ref @$ss27_allocateUninitializedArrayySayxG_BptBwlF
//	%2 = apply %1<Int32>(%0)
//	(%3, %4) = destructure_tuple %2
//	store %14 to %10
//	%31 = function_ref @$ss27_finalizeUninitializedArrayySayxGABnlF
//	%32 = apply %31<Int32>(%3)
//
// The third step is not emitted, because there is nothing to call and
// nothing to do. `_finalizeUninitializedArray` is
// @_alwaysEmitIntoClient -- libswiftCore does not export it, and
// swiftc emits a copy of it into every module that writes an array
// literal -- and the copy's body is `_endMutation`, which in the
// shipping runtime is `ldr x8, [x20]` and `str x8, [x20]`: a word
// loaded and stored back. It exists to flip a bit that only a
// runtime built with internal checks reads. What comes out of the
// allocation is the finished array, which is also what `swiftc -O`
// concludes: its `make()` is an adrp and a ret.
//
// The allocation is generic, and a generic function is called with
// the metadata for each of its type parameters after the arguments
// the source wrote: swiftc's own code puts the count in x0 and
// Int32's metadata in x1. That metadata is what limits this. A type
// declared in this module has none -- see lower.trivialMetadata for
// why -- so an array of one is refused by name, and an array of a
// type the standard library declares is not, because libswiftCore
// exports the accessor that answers for it.

const (
	// allocateUninitializedArray is
	// _allocateUninitializedArray<T>(_ count: Builtin.Word) ->
	// (Array<T>, Builtin.RawPointer).
	allocateUninitializedArray = "$ss27_allocateUninitializedArrayySayxG_BptBwlF"
)

// arrayLiteral builds the Array a literal denotes.
func (g *gen) arrayLiteral(e *ast.ArrayLit) *vil.Value {
	t := g.typeOf(e)
	arr, ok := arrayOf(t)
	if !ok {
		g.refuse(e, "an array literal whose element type is not known")
		return nil
	}
	elems := make([]*vil.Value, 0, len(e.Items))
	for _, item := range e.Items {
		v := g.rvalue(item)
		if v == nil {
			return nil
		}
		elems = append(elems, v)
	}
	return g.makeArray(e, t, arr.Elem, elems)
}

// makeArray builds an Array holding values already lowered.
//
// Two callers, and they are the same thing said twice in the source:
// a literal, and the arguments a variadic parameter collects. Swift
// treats them alike -- SILGen's code for `total(40, 2)` is the code
// it writes for `[40, 2]` -- and so does this.
func (g *gen) makeArray(at ast.Node, t, elem types.Type, elems []*vil.Value) *vil.Value {
	meta, ok := g.stdlibMetadata(at, elem)
	if !ok {
		return nil
	}
	raw := vil.Object(vil.BuiltinRawPointer)
	pair := vil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: t},
		{Type: vil.BuiltinRawPointer},
	}})
	alloc := g.arrayHelper(allocateUninitializedArray,
		[]vil.Param{{Type: vil.Object(vil.BuiltinWord)}}, pair, vil.ResultOwned)
	got := g.blk.Apply(g.blk.FunctionRef(alloc), pair,
		g.blk.IntegerLiteral(vil.Object(vil.BuiltinWord), int64(len(elems))), meta)

	// Taken apart rather than read from: the pair is owned, and what
	// is owned is the array in it. destructure_tuple is what hands
	// that ownership on, which is what SILGen writes here too.
	parts := g.blk.DestructureTuple(got, lowerType(t), raw)
	if len(parts) != 2 {
		return nil
	}
	fresh, base := parts[0], g.blk.PointerToAddress(parts[1], lowerType(elem).Address())
	for i, v := range elems {
		addr := base
		if i > 0 {
			addr = g.blk.IndexAddr(base,
				g.blk.IntegerLiteral(vil.Object(vil.BuiltinWord), int64(i)))
		}
		// A value that lives in storage is copied rather than stored:
		// an existential is an address, and storing one would write
		// the address itself where its bytes belong.
		if v.Type().IsAddress() {
			g.blk.CopyAddr(v, addr, "init")
			continue
		}
		g.blk.Store(v, addr, storeQualifier(lowerType(elem)))
	}

	g.destroyLater(fresh)
	return fresh
}

// arrayHelper declares one of the two standard-library functions an
// array literal is built out of.
//
// The metadata parameter is written down here rather than left
// implicit, because at this level it is an argument like any other:
// the call site has it in hand, and the register it goes in is the
// one after the arguments the source wrote.
func (g *gen) arrayHelper(symbol string, params []vil.Param, result vil.Type,
	conv vil.ResultConvention) *vil.Func {
	f := g.m.Func(symbol).SetSourceName(symbol)
	if !g.needsType(f) {
		return f
	}
	f.SetLinkage(vil.PublicExternal)
	ft := f.Type()
	ft.Convention = vil.Thin
	ft.Params = append(append([]vil.Param(nil), params...),
		vil.Param{Type: vil.Object(vil.BuiltinRawPointer)})
	f.SetResult(result, conv)
	return f
}

// arrayOf is the Array a type is, if it is one.
func arrayOf(t types.Type) (*types.Array, bool) {
	if t == nil {
		return nil, false
	}
	a, ok := t.Underlying().(*types.Array)
	return a, ok
}

// stdlibMetadata is the metadata for a type the standard library
// declares, which is what a generic call has to be given.
//
// The accessors are libswiftCore's own, read back from what swiftc
// emits rather than remembered. Only the types that are one register
// wide: an element is written through one store, and the ones left
// out are left out by name rather than stored half.
//
// A type declared in this module is not here and cannot be: this
// compiler emits no metadata for one, which is the same gap that
// stops an existential it fills in from crossing a boundary.
func (g *gen) stdlibMetadata(at ast.Node, t types.Type) (*vil.Value, bool) {
	if b, ok := t.Underlying().(*types.Basic); ok {
		sym, ok := metadataAccessors[b.Kind()]
		if !ok {
			g.refuse(at, "the metadata for '"+t.String()+"', which this compiler "+
				"cannot name")
			return nil, false
		}
		return g.blk.TypeMetadata(lowerType(t), sym), true
	}
	// `Any` has no accessor: libswiftCore exports the record itself,
	// and the metadata is one word inside it -- the value witness
	// table comes first, as it does in every full record. swiftc's
	// own code for `print` loads `$sypN` and adds eight.
	if ex, ok := t.(*types.Existential); ok && len(ex.Protocols) == 0 {
		return g.blk.MetadataGlobal(lowerType(t), anyMetadata, metadataInRecord), true
	}
	// A nominal type has an accessor, and which module exports it is
	// the only difference: another module's library did, and this
	// module emits its own. Either way the call is the same.
	if chain := nominalChain(t); len(chain) > 0 {
		module := g.moduleOfType(t)
		if module == g.module && !g.needMetadata(at, t) {
			return nil, false
		}
		sym, err := mangle.MetadataAccessor(mangle.Decl{
			Module:   module,
			Context:  chain,
			ModuleOf: g.moduleOfType,
		})
		if err == nil && sym != "" {
			return g.blk.TypeMetadata(lowerType(t), sym), true
		}
	}
	g.refuse(at, "the metadata for '"+t.String()+"', which this compiler cannot name")
	return nil, false
}

const (
	// anyMetadata is the record libswiftCore exports for `Any`, and
	// metadataInRecord is how far into a full record the metadata
	// proper begins: past the value witness table pointer.
	anyMetadata      = "$sypN"
	metadataInRecord = 8
)

// metadataAccessors is the symbol that answers with a primitive
// type's metadata.
//
// All but String are one register wide, which is what an array
// literal needs of an element; a String is two, and an existential
// holds one because its buffer is three.
var metadataAccessors = map[types.BasicKind]string{
	types.Int:    "$sSiMa",
	types.UInt:   "$sSuMa",
	types.Int8:   "$ss4Int8VMa",
	types.Int16:  "$ss5Int16VMa",
	types.Int32:  "$ss5Int32VMa",
	types.Int64:  "$ss5Int64VMa",
	types.UInt8:  "$ss5UInt8VMa",
	types.UInt16: "$ss6UInt16VMa",
	types.UInt32: "$ss6UInt32VMa",
	types.UInt64: "$ss6UInt64VMa",
	types.Bool:   "$sSbMa",
	types.Float:  "$sSfMa",
	types.Double: "$sSdMa",
	types.String: "$sSSMa",
}

// subscript reads one element of an array.
//
// Its getter is generic and hands the element back through storage
// the caller set aside: `<T> (Int, @guaranteed Array<T>) -> @out T`.
// That is not a fact about how wide an element is -- swiftc's own
// call for an [Int32] passes a four-byte slot in x8 -- it is what a
// generic result convention is, because the caller is the only one
// who knows the size.
//
// The bounds check is the getter's. Reading past the end traps in
// libswiftCore rather than here, which is the same place swiftc's own
// code traps.
func (g *gen) subscript(e *ast.SubscriptExpr) *vil.Value {
	t := g.typeOf(e.X)
	m, ok := core.Subscript(t)
	if !ok {
		g.refuse(e, "a subscript of '"+typeName(t)+"'")
		return nil
	}
	if len(e.Args) != 1 {
		g.refuse(e, "a subscript with more than one index")
		return nil
	}
	base := g.expr(e.X)
	index := g.expr(e.Args[0].X)
	if base == nil || index == nil {
		return nil
	}
	return g.elementAt(e, base, t, index, m)
}

// elementAt is subscript with the array and the index already
// lowered, which is what a for-in over one wants.
func (g *gen) elementAt(at ast.Node, base *vil.Value, t types.Type,
	index *vil.Value, m core.Member) *vil.Value {
	meta, ok := g.stdlibMetadata(at, m.Element)
	if !ok {
		return nil
	}
	result := lowerType(m.Result)

	callee := g.m.Func(m.Symbol).SetSourceName("subscript")
	if g.needsType(callee) {
		callee.SetLinkage(vil.PublicExternal)
		ft := callee.Type()
		ft.Convention = vil.Thin
		ft.Params = []vil.Param{
			{Type: lowerType(types.Typ[types.Int])},
			{Type: lowerType(t), Convention: vil.ParamGuaranteed},
			{Type: vil.Object(vil.BuiltinRawPointer)},
		}
		callee.SetResult(result, vil.ResultOut)
	}
	v := g.blk.Apply(g.blk.FunctionRef(callee), result, index, base, meta)
	g.destroyLater(v)
	return v
}

// forInArray lowers `for x in a` as the index walk it is.
//
// SILGen does not do this. It emits the desugaring the language
// defines -- makeIterator(), then next() until it returns none -- and
// every piece of that is generic standard library: Collection,
// IndexingIterator, Optional. What is emitted here is what `swiftc -O`
// arrives at once it has specialized and inlined all of that away,
// which is the same program: the array evaluated once, its count read
// once, an index, and a comparison.
//
// Both facts are load-bearing. The array is evaluated once, however
// many times the body runs, and so is the count -- a for-in over
// `f()` calls f one time, and a body that appended would not change
// where the loop stops. The bounds check is the getter's, which is
// where swiftc's is too.
func (g *gen) forInArray(s *ast.ForInStmt, arr *types.Array) {
	seqType := g.typeOf(s.Seq)
	count, ok := core.LowerMember(seqType, "count")
	if !ok {
		g.refuse(s, "a for-in over '"+typeName(seqType)+"', whose count this "+
			"compiler cannot read")
		return
	}
	elem, ok := core.Subscript(seqType)
	if !ok {
		g.refuse(s, "a for-in over '"+typeName(seqType)+"', whose elements this "+
			"compiler cannot read")
		return
	}

	// The scope the array and the count live in: both are evaluated
	// once, and what owns the array lets go of it where the loop ends.
	g.push()

	seq := g.expr(s.Seq)
	if seq == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return
	}
	n := g.readProperty(s.Seq, seq, seqType, count, "count")
	if n == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return
	}

	index := types.Typ[types.Int]
	it := lowerType(index)
	box := g.blk.AllocBox(it, "$index", "var")
	borrow := g.blk.BeginBorrow(box, "var_decl")
	slot := g.blk.ProjectBox(borrow, 0, it)
	g.destroyLater(box)
	g.endBorrowLater(borrow)
	g.blk.Store(g.blk.Struct(it,
		g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt64), 0)), slot, storeQualifier(it))

	header := g.fn.Block()
	body := g.fn.Block()
	latch := g.fn.Block()
	exit := g.fn.Block()
	label := g.takeLabel()
	g.blk.Br(header)

	g.blk = header
	at := g.blk.Load(slot, loadQualifier(it))
	more := g.compare("<", at, n, index)
	if more == nil {
		g.refuse(s.Seq, "an index this compiler cannot compare")
		return
	}
	g.blk.CondBr(more, body, nil, exit, nil)

	g.blk = body
	at = g.blk.Load(slot, loadQualifier(it))
	// The scope comes first, because reading an element produces a
	// value the iteration owns: an element that is a String is a
	// copy, and it is let go of at the end of the body rather than
	// after the loop, where it would not be alive.
	inner := len(g.scopes)
	g.push()
	value := g.elementAt(s.Seq, seq, seqType, at, elem)
	if value == nil {
		return
	}
	// The binding takes it over: what the iteration owns is the bound
	// value, and destroying both would release it twice.
	g.bindLoopVar(s.Pat, g.consume(value), lowerType(arr.Elem))

	g.loops = append(g.loops, loop{header: latch, exit: exit, depth: inner, label: label})
	g.block(s.Body)
	g.loops = g.loops[:len(g.loops)-1]
	if g.blk != nil && g.blk.Term() == nil {
		g.pop()
		g.blk.Br(latch)
	} else {
		g.scopes = g.scopes[:len(g.scopes)-1]
	}

	g.blk = latch
	cur := g.blk.Load(slot, loadQualifier(it))
	next := g.increment(cur, index)
	if next == nil {
		g.refuse(s.Seq, "an index this compiler cannot step")
		return
	}
	g.blk.Store(next, slot, storeQualifier(it))
	g.blk.Br(header)

	g.blk = exit
	g.pop()
}
