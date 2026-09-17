package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

const (
	// allocateUninitializedArray is the runtime's
	// vertex_array_allocate(count, element metadata) -> (array,
	// elements): storage for count elements, and where they go.
	allocateUninitializedArray = stdlib.ArrayAllocate
)

// arrayLiteral builds the Array a literal denotes.
func (g *gen) arrayLiteral(e *ast.ArrayLit) *sil.Value {
	t := g.typeOf(e)
	if s, isSet := t.Underlying().(*types.Set); isSet {
		return g.setLiteral(e, s)
	}
	arr, ok := arrayOf(t)
	if !ok {
		g.refuse(e, "an array literal whose element type is not known")
		return nil
	}
	elems := make([]*sil.Value, 0, len(e.Items))
	for _, item := range e.Items {
		v := g.rvalue(item)
		if v == nil {
			return nil
		}
		// An element of [Any] is its value in an existential, as an
		// argument to a variadic Any... is.
		elems = append(elems, g.existentialFor(item, v, g.typeOf(item), arr.Elem))
	}
	return g.makeArray(e, t, arr.Elem, elems)
}

// arrayInit lowers `[T]()` and `[T](repeating:count:)`. The repeated value
// goes to the runtime in memory of its own, which the runtime takes.
func (g *gen) arrayInit(e *ast.CallExpr) *sil.Value {
	t := g.typeOf(e)
	arr, ok := arrayOf(t)
	if !ok {
		g.refuse(e, "an array initializer whose element type is not known")
		return nil
	}
	if e.Args == nil || len(e.Args.Args) == 0 {
		return g.makeArray(e, t, arr.Elem, nil)
	}
	args := e.Args.Args
	if len(args) != 2 {
		g.refuse(e, "this array initializer")
		return nil
	}
	value := g.rvalue(args[0].X)
	count := g.rvalue(args[1].X)
	if value == nil || count == nil {
		return nil
	}
	meta, ok := g.stdlibMetadata(e, arr.Elem)
	if !ok {
		return nil
	}
	et := lowerType(arr.Elem)
	slot := g.blk.AllocStack(et)
	if value.Type().IsAddress() {
		// The temporary it is in is not used again: the value moves.
		g.blk.CopyAddr(value, slot, "take", "init")
	} else {
		g.blk.Store(value, slot, storeQualifier(et))
	}
	raw := sil.Object(sil.BuiltinRawPointer)
	helper := g.arrayHelper(stdlib.ArrayRepeating,
		[]sil.Param{{Type: lowerType(types.Typ[types.Int])}, {Type: raw}},
		lowerType(t), sil.ResultOwned)
	made := g.blk.Apply(g.blk.FunctionRef(helper), lowerType(t),
		count, g.blk.AddressToPointer(slot, raw), meta)
	g.destroyLater(made)
	return made
}

// makeArray builds an Array holding values already lowered (for literals and variadics).
func (g *gen) makeArray(at ast.Node, t, elem types.Type, elems []*sil.Value) *sil.Value {
	meta, ok := g.stdlibMetadata(at, elem)
	if !ok {
		return nil
	}
	raw := sil.Object(sil.BuiltinRawPointer)
	pair := sil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: t},
		{Type: sil.BuiltinRawPointer},
	}})
	alloc := g.arrayHelper(allocateUninitializedArray,
		[]sil.Param{{Type: sil.Object(sil.BuiltinWord)}}, pair, sil.ResultOwned)
	got := g.blk.Apply(g.blk.FunctionRef(alloc), pair,
		g.blk.IntegerLiteral(sil.Object(sil.BuiltinWord), int64(len(elems))), meta)

	parts := g.blk.DestructureTuple(got, lowerType(t), raw)
	if len(parts) != 2 {
		return nil
	}
	fresh, base := parts[0], g.blk.PointerToAddress(parts[1], lowerType(elem).Address())
	for i, v := range elems {
		addr := base
		if i > 0 {
			addr = g.blk.IndexAddr(base,
				g.blk.IntegerLiteral(sil.Object(sil.BuiltinWord), int64(i)))
		}
		if v.Type().IsAddress() {
			// Moved out of the temporary it was made in.
			g.blk.CopyAddr(v, addr, "take", "init")
			continue
		}
		g.blk.Store(v, addr, storeQualifier(lowerType(elem)))
	}

	g.destroyLater(fresh)
	return fresh
}

// arrayHelper declares a standard-library runtime helper for array operations.
func (g *gen) arrayHelper(symbol string, params []sil.Param, result sil.Type,
	conv sil.ResultConvention) *sil.Func {
	f := g.m.Func(symbol).SetSourceName(symbol)
	if !g.needsType(f) {
		return f
	}
	f.SetLinkage(sil.PublicExternal)
	ft := f.Type()
	ft.Convention = sil.Thin
	ft.Params = append(append([]sil.Param(nil), params...),
		sil.Param{Type: sil.Object(sil.BuiltinRawPointer)})
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

// stdlibMetadata returns the type metadata value required for runtime calls or generics.
func (g *gen) stdlibMetadata(at ast.Node, t types.Type) (*sil.Value, bool) {
	// A type the core declares has a record the runtime exports, and
	// the metadata is one word inside it: the value witness table comes
	// first, as it does in every full record.
	if b, ok := t.Underlying().(*types.Basic); ok {
		name, ok := metadataRecords[b.Kind()]
		if !ok {
			g.refuse(at, "the metadata for '"+t.String()+"', which this compiler "+
				"cannot name")
			return nil, false
		}
		return g.blk.MetadataGlobal(lowerType(t), stdlib.Metadata(name), stdlib.MetadataOffset), true
	}
	if ex, ok := existentialOf(t); ok {
		switch len(ex.Protocols) {
		case 0:
			return g.blk.MetadataGlobal(lowerType(t), stdlib.Metadata("Any"), stdlib.MetadataOffset), true
		case 1:
			return g.blk.MetadataGlobal(lowerType(t), stdlib.Metadata("Existential1"), stdlib.MetadataOffset), true
		}
	}
	// Every function value is a code pointer and a counted context, so
	// one record serves every function type.
	if _, ok := t.Underlying().(*types.Signature); ok {
		return g.blk.MetadataGlobal(lowerType(t), stdlib.Metadata("Function"), stdlib.MetadataOffset), true
	}
	// An Optional or an Array of something is declared nowhere, so every
	// module that needs its metadata emits a record of its own.
	switch t.Underlying().(type) {
	case *types.Optional, *types.Array, *types.Dictionary, *types.Set:
		sym, ok := g.structuralMetadata(at, t)
		if !ok {
			return nil, false
		}
		return g.blk.TypeMetadata(lowerType(t), sym+"Ma"), true
	}
	// A nominal type has an accessor, and which module exports it is
	// the only difference: another module's library did, and this
	// module emits its own. Either way the call is the same.
	if chain := nominalChain(t); len(chain) > 0 {
		module := g.moduleOfType(t)
		if !g.importedType(t) && !g.needMetadata(at, t) {
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

// metadataRecords is the name of the record the runtime exports for a
// type the core declares. See stdlib.Metadata.
var metadataRecords = map[types.BasicKind]string{
	types.Int:    "Int",
	types.UInt:   "UInt",
	types.Int8:   "Int8",
	types.Int16:  "Int16",
	types.Int32:  "Int32",
	types.Int64:  "Int64",
	types.UInt8:  "UInt8",
	types.UInt16: "UInt16",
	types.UInt32: "UInt32",
	types.UInt64: "UInt64",
	types.Bool:   "Bool",
	types.Float:  "Float",
	types.Double: "Double",
	types.String: "String",
}

// subscript lowers an array or dictionary element read.
func (g *gen) subscript(e *ast.SubscriptExpr) *sil.Value {
	t := g.typeOf(e.X)
	// `a[lo..<hi]`: a slice of it.
	if v, ok := g.arraySliceSubscript(e); ok {
		return v
	}
	// `d[key, default: value]`: the value, or the default.
	if d, isDict := t.Underlying().(*types.Dictionary); isDict && g.defaultSubscript(e) {
		get, ok := core.DictionaryGetDefault(d)
		if !ok {
			g.refuse(e, "a dictionary whose key type '"+d.Key.String()+"' the runtime does not hash yet")
			return nil
		}
		return g.collectionCall(e, get, e.X, exprArgs(e.Args[0].X, e.Args[1].X))
	}
	// `d[key]`: the value, or nil.
	if d, isDict := t.Underlying().(*types.Dictionary); isDict && len(e.Args) == 1 {
		get, ok := core.DictionaryGet(d)
		if !ok {
			g.refuse(e, "a dictionary whose key type '"+d.Key.String()+"' the runtime does not hash yet")
			return nil
		}
		return g.collectionCall(e, get, e.X, exprArgs(e.Args[0].X))
	}
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

// elementAt lowers an element read for an already-lowered array and index.
func (g *gen) elementAt(at ast.Node, base *sil.Value, t types.Type,
	index *sil.Value, m core.Member) *sil.Value {
	meta, ok := g.stdlibMetadata(at, m.Element)
	if !ok {
		return nil
	}
	elem := lowerType(m.Result)
	_, isExistential := existentialOf(m.Result)

	raw := sil.Object(sil.BuiltinRawPointer)
	callee := g.m.Func(m.Symbol).SetSourceName("subscript")
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		ft := callee.Type()
		ft.Convention = sil.Thin
		ft.Params = []sil.Param{
			{Type: lowerType(types.Typ[types.Int])},
			{Type: lowerType(t), Convention: sil.ParamGuaranteed},
			{Type: raw},
		}
		callee.SetResult(raw, sil.ResultUnowned)
	}
	p := g.blk.Apply(g.blk.FunctionRef(callee), raw, index, base, meta)
	addr := g.blk.PointerToAddress(p, elem.Address())
	// An existential is read as a temporary copy of the element, in memory,
	// which the use takes.
	if isExistential {
		slot := g.blk.AllocStack(elem)
		g.blk.CopyAddr(addr, slot, "init")
		return slot
	}
	v := g.blk.Load(addr, loadQualifier(elem))
	g.destroyLater(v)
	return v
}

// forInArray lowers `for x in a` as a counted loop over the array's elements.
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
		g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt64), 0)), slot, storeQualifier(it))

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
	inner := len(g.scopes)
	g.push()
	value := g.elementAt(s.Seq, seq, seqType, at, elem)
	if value == nil {
		return
	}
	// An existential element is its own copy in memory, bound as storage.
	if _, isEx := existentialOf(arr.Elem); isEx && value != nil {
		if name := patternIdent(s.Pat); name != nil {
			if sym := g.info.Defs[name]; sym != nil {
				g.locals[sym] = &local{addr: value, typ: lowerType(arr.Elem), mem: true}
			}
		}
		g.destroyAddrLater(value)
	} else if s.Case.IsValid() {
		// Matched against the pattern as the body begins. See forInBody.
		g.loopCase = &loopElement{value: g.consume(value), typ: arr.Elem}
	} else {
		g.bindLoopVar(s.Pat, g.consume(value), lowerType(arr.Elem))
	}

	g.loops = append(g.loops, loop{header: latch, exit: exit, depth: inner, label: label})
	g.forInBody(s)
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
