package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// withUnsafeBytes lowers `a.withUnsafeBytes { raw in ... }` over an
// array: the closure is called with the bytes the elements are, where
// they are, and what it returns is the call's value. It reports false
// for any other call.
//
// `withUnsafeBufferPointer` is the same storage given as elements rather
// than as bytes: the base typed, and the count a count of elements.
func (g *gen) withUnsafeBytes(e *ast.CallExpr, mem *ast.MemberExpr) (*sil.Value, bool) {
	if mem.Name == nil || e.Args == nil || len(e.Args.Args) != 1 {
		return nil, false
	}
	name := g.text(mem.Name)
	if name != "withUnsafeBytes" && name != "withUnsafeBufferPointer" {
		return nil, false
	}
	elements := name == "withUnsafeBufferPointer"
	recvType := g.typeOf(mem.X)
	if recvType == nil {
		return nil, false
	}
	// An array's bytes, or a slice's: from its start, as many as lie
	// between its bounds.
	var elem types.Type
	_, isSlice := g.arraySlice(recvType)
	if arr, ok := recvType.Underlying().(*types.Array); ok {
		elem = arr.Elem
	} else if el, ok := g.arraySlice(recvType); ok {
		elem = el
	} else {
		return nil, false
	}
	body := e.Args.Args[0].X
	sig, _ := g.typeOf(body).Underlying().(*types.Signature)
	if sig == nil || len(sig.Params) != 1 || sig.Throws {
		g.refuse(e, "a withUnsafeBytes whose body is not one closure that does not throw")
		return nil, true
	}
	bufType := sig.Params[0].Type
	buf, _ := bufType.Underlying().(*types.Struct)
	if buf == nil || len(buf.Fields) != 2 {
		g.refuse(e, "a withUnsafeBytes whose body does not take an UnsafeRawBufferPointer")
		return nil, true
	}
	baseType, ok := optionalOf(buf.Fields[0].Type)
	if !ok {
		g.refuse(e, "an UnsafeRawBufferPointer whose base address is not optional")
		return nil, true
	}

	recv := g.expr(mem.X)
	if recv == nil {
		return nil, true
	}
	fn := g.expr(body)
	if fn == nil {
		return nil, true
	}

	intT := types.Typ[types.Int]
	word := sil.Object(builtinFor(intT))
	storage := recv
	var start, count *sil.Value
	if isSlice {
		src := recv
		if recv.Ownership() == sil.Owned {
			src = g.blk.BeginBorrow(recv)
			g.endBorrowLater(src)
		}
		lo, hi := g.sliceBounds(src, recvType)
		storage = g.blk.StructExtract(src, memberName(recvType, "base"), lowerType(&types.Array{Elem: elem}))
		start = g.machine(lo, intT)
		count = g.checkedWord("ssub_with_overflow_Int64", g.machine(hi, intT), start)
	} else {
		m, ok := core.LowerMember(recvType, "count")
		if !ok {
			g.refuse(e, "a withUnsafeBytes over an array whose count this cannot read")
			return nil, true
		}
		n := g.readProperty(mem.X, storage, recvType, m, "count")
		if n == nil {
			return nil, true
		}
		count = g.machine(n, intT)
	}
	bytes := count
	if !elements {
		stride := types.Strideof(elem, types.DefaultTarget64)
		bytes = g.checkedWord("smul_with_overflow_Int64", count, g.blk.IntegerLiteral(word, stride))
	}

	// Where they start is where the storage keeps its elements -- see
	// stdlib/ABI.md -- moved on to a slice's first.
	base := g.blk.Builtin("vertexArrayElements_RawPointer", lowerType(baseType.Wrapped), storage)
	if isSlice {
		at := g.blk.IndexAddr(g.blk.PointerToAddress(base, lowerType(elem).Address()), start)
		base = g.blk.AddressToPointer(at, lowerType(baseType.Wrapped))
	}
	if elements {
		base = g.blk.AddressToPointer(g.blk.PointerToAddress(base, lowerType(elem).Address()),
			lowerType(baseType.Wrapped))
	}
	someBase := g.blk.Enum(lowerType(buf.Fields[0].Type), optionalSome, base)
	value := g.blk.Struct(lowerType(bufType), someBase, g.blk.Struct(lowerType(intT), bytes))

	v := g.blk.Apply(fn, lowerType(sig.Results), value)
	g.destroyLater(v)
	return v, true
}

// withCString lowers `s.withCString { p in ... }`: the closure is given a
// NUL-terminated copy of the string's bytes, which is let go once it
// returns.
func (g *gen) withCString(e *ast.CallExpr, mem *ast.MemberExpr) (*sil.Value, bool) {
	if mem.Name == nil || g.text(mem.Name) != "withCString" || e.Args == nil || len(e.Args.Args) != 1 {
		return nil, false
	}
	recvType := g.typeOf(mem.X)
	if b, ok := recvType.Underlying().(*types.Basic); !ok || b.Kind() != types.String {
		return nil, false
	}
	body := e.Args.Args[0].X
	sig, _ := g.typeOf(body).Underlying().(*types.Signature)
	if sig == nil || len(sig.Params) != 1 || sig.Throws {
		g.refuse(e, "a withCString whose body is not one closure that does not throw")
		return nil, true
	}
	s := g.expr(mem.X)
	if s == nil {
		return nil, true
	}
	fn := g.expr(body)
	if fn == nil {
		return nil, true
	}
	raw := rawPointerType()
	bytes := g.runtimeResult(stdlib.StringCString,
		[]sil.Param{{Type: lowerType(recvType), Convention: sil.ParamGuaranteed}}, raw, s)
	ptrType := sig.Params[0].Type
	var elem types.Type = types.Typ[types.Int8]
	if p, ok := pointerOf(ptrType); ok && p.Elem != nil {
		elem = p.Elem
	}
	arg := g.blk.AddressToPointer(g.blk.PointerToAddress(bytes, lowerType(elem).Address()), lowerType(ptrType))
	v := g.blk.Apply(fn, lowerType(sig.Results), arg)
	g.runtimeResult(stdlib.StringCStringFree,
		[]sil.Param{{Type: raw, Convention: sil.ParamUnowned}}, lowerType(types.Typ[types.Void]), bytes)
	g.destroyLater(v)
	return v, true
}

// cStringArg is a String passed where a C string is wanted: a pointer to
// a NUL-terminated copy of its bytes, as the parameter's pointer type, and
// wrapped where the parameter is optional.
//
// Swift makes this conversion at a call and promises the pointer for the
// duration of that call. The copy is let go at the end of the enclosing
// scope, which is after the call and on every path out of it -- the same
// place a temporary's destroy goes.
func (g *gen) cStringArg(s *sil.Value, to types.Type) *sil.Value {
	ptrType := to
	opt, isOptional := to.Underlying().(*types.Optional)
	if isOptional {
		ptrType = opt.Wrapped
	}
	raw := rawPointerType()
	bytes := g.runtimeResult(stdlib.StringCString,
		[]sil.Param{{Type: lowerType(types.Typ[types.String]), Convention: sil.ParamGuaranteed}}, raw, s)
	var elem types.Type = types.Typ[types.Int8]
	if p, ok := pointerOf(ptrType); ok && p.Elem != nil {
		elem = p.Elem
	}
	arg := g.blk.AddressToPointer(g.blk.PointerToAddress(bytes, lowerType(elem).Address()), lowerType(ptrType))
	sc := g.lexical()
	sc.cleanups = append(sc.cleanups, cleanup{freeCString: bytes})
	if isOptional {
		return g.blk.Enum(lowerType(to), optionalSome, arg)
	}
	return arg
}

// withUnsafeMutableBufferPointer lowers `a.withUnsafeMutableBufferPointer
// { bp in ... }` over an array variable: its storage is made the
// variable's alone, and the closure is given where the elements are and
// how many, to read and write while the variable is being modified.
func (g *gen) withUnsafeMutableBufferPointer(e *ast.CallExpr, mem *ast.MemberExpr) (*sil.Value, bool) {
	if mem.Name == nil || e.Args == nil || len(e.Args.Args) != 1 {
		return nil, false
	}
	// withUnsafeMutableBytes is the same storage made the variable's
	// alone, given as bytes rather than as elements.
	name := g.text(mem.Name)
	if name != "withUnsafeMutableBufferPointer" && name != "withUnsafeMutableBytes" {
		return nil, false
	}
	recvType := g.typeOf(mem.X)
	if recvType == nil {
		return nil, false
	}
	arr, ok := recvType.Underlying().(*types.Array)
	if !ok {
		return nil, false
	}
	body := e.Args.Args[0].X
	sig, _ := g.typeOf(body).Underlying().(*types.Signature)
	if sig == nil || len(sig.Params) != 1 || sig.Throws {
		g.refuse(e, "a withUnsafeMutableBufferPointer whose body is not one closure that does not throw")
		return nil, true
	}
	bufType := sig.Params[0].Type
	buf, _ := bufType.Underlying().(*types.Struct)
	if buf == nil || len(buf.Fields) != 2 {
		g.refuse(e, "a withUnsafeMutableBufferPointer whose body does not take an UnsafeMutableBufferPointer")
		return nil, true
	}
	baseType, ok := optionalOf(buf.Fields[0].Type)
	if !ok {
		g.refuse(e, "an UnsafeMutableBufferPointer whose base address is not optional")
		return nil, true
	}
	count, ok := core.LowerMember(recvType, "count")
	if !ok {
		g.refuse(e, "a withUnsafeMutableBufferPointer over an array whose count this cannot read")
		return nil, true
	}
	meta, ok := g.stdlibMetadata(e, arr.Elem)
	if !ok {
		return nil, true
	}
	fn := g.expr(body)
	if fn == nil {
		return nil, true
	}
	addr := g.lvalue(mem.X)
	if addr == nil {
		g.refuse(e, "a withUnsafeMutableBufferPointer on "+g.exprKind(mem.X)+", which is not a variable")
		return nil, true
	}
	access := g.blk.BeginAccess(addr, "modify", "unknown")
	raw := rawPointerType()
	base := g.runtimeResult(stdlib.ArrayUniqueElements,
		[]sil.Param{{Type: raw, Convention: sil.ParamUnowned}, {Type: raw, Convention: sil.ParamUnowned}},
		raw, g.blk.AddressToPointer(access, raw), meta)
	// The count, read once the storage is the variable's alone, so that
	// reading it does not make the storage shared again first.
	lt := lowerType(recvType)
	now := g.loaded(g.blk.Load(access, loadQualifier(lt)), lt)
	n := g.readProperty(mem.X, now, recvType, count, "count")
	if n == nil {
		g.blk.EndAccess(access)
		return nil, true
	}
	if name == "withUnsafeMutableBytes" {
		intT := types.Typ[types.Int]
		word := sil.Object(builtinFor(intT))
		bit := sil.Object(sil.BuiltinInt1)
		pair := sil.Object(&types.Tuple{Elements: []*types.TupleElement{
			{Type: builtinFor(intT)},
			{Type: sil.BuiltinInt1},
		}})
		stride := types.Strideof(arr.Elem, types.DefaultTarget64)
		both := g.blk.Builtin("smul_with_overflow_Int64", pair,
			g.machine(n, intT), g.blk.IntegerLiteral(word, stride), g.blk.IntegerLiteral(bit, -1))
		g.blk.CondFail(g.blk.TupleExtract(both, 1, bit), "arithmetic overflow")
		n = g.blk.Struct(lowerType(intT), g.blk.TupleExtract(both, 0, word))
	}
	typed := g.blk.AddressToPointer(g.blk.PointerToAddress(base, lowerType(arr.Elem).Address()),
		lowerType(baseType.Wrapped))
	someBase := g.blk.Enum(lowerType(buf.Fields[0].Type), optionalSome, typed)
	value := g.blk.Struct(lowerType(bufType), someBase, n)
	v := g.blk.Apply(fn, lowerType(sig.Results), value)
	g.blk.EndAccess(access)
	g.destroyLater(v)
	return v, true
}

// stringUTF8 lowers `s.utf8`: the string's bytes, as an array of them.
func (g *gen) stringUTF8(e *ast.MemberExpr) (*sil.Value, bool) {
	if e.Name == nil || g.text(e.Name) != "utf8" {
		return nil, false
	}
	t := g.typeOf(e.X)
	if t == nil {
		return nil, false
	}
	if b, ok := t.Underlying().(*types.Basic); !ok || b.Kind() != types.String {
		return nil, false
	}
	s := g.expr(e.X)
	if s == nil {
		return nil, true
	}
	v := g.runtimeResult(stdlib.StringUTF8Array,
		[]sil.Param{{Type: lowerType(t), Convention: sil.ParamGuaranteed}}, lowerType(g.typeOf(e)), s)
	g.destroyLater(v)
	return v, true
}

// stringFromCString lowers `String(cString: x)`: the bytes before the NUL
// that x, an array of CChar or a pointer to them, holds.
func (g *gen) stringFromCString(e *ast.CallExpr, t types.Type) (*sil.Value, bool) {
	if b, ok := t.Underlying().(*types.Basic); !ok || b.Kind() != types.String {
		return nil, false
	}
	if e.Args == nil || len(e.Args.Args) != 1 || e.Args.Args[0].Label == nil ||
		g.text(e.Args.Args[0].Label) != "cString" {
		return nil, false
	}
	arg := e.Args.Args[0].X
	from := g.typeOf(arg)
	raw := rawPointerType()
	var bytes *sil.Value
	if _, isArray := from.Underlying().(*types.Array); isArray {
		a := g.expr(arg)
		if a == nil {
			return nil, true
		}
		bytes = g.blk.Builtin("vertexArrayElements_RawPointer", raw, a)
	} else {
		if _, ok := pointerOf(from); !ok {
			g.refuse(e, "a String(cString:) of something other than an array or a pointer")
			return nil, true
		}
		p := g.expr(arg)
		if p == nil {
			return nil, true
		}
		bytes = g.blk.AddressToPointer(g.blk.PointerToAddress(p, lowerType(types.Typ[types.Int8]).Address()), raw)
	}
	v := g.runtimeResult(stdlib.StringFromCString,
		[]sil.Param{{Type: raw, Convention: sil.ParamUnowned}}, lowerType(t), bytes)
	g.destroyLater(v)
	return v, true
}

// arraySlice is the element type of the core's ArraySlice t, if t is one.
func (g *gen) arraySlice(t types.Type) (types.Type, bool) {
	inst, ok := t.(*types.GenericInstance)
	if !ok || inst.Base == nil || !g.info.CoreTypes[inst.Base] || len(inst.Args) != 1 {
		return nil, false
	}
	st, ok := inst.Base.Underlying().(*types.Struct)
	if !ok || st.Name != "ArraySlice" {
		return nil, false
	}
	return inst.Args[0], true
}

// sliceBounds is a slice's start and end index, taken out of it.
func (g *gen) sliceBounds(v *sil.Value, t types.Type) (start, end *sil.Value) {
	it := lowerType(types.Typ[types.Int])
	return g.blk.StructExtract(v, memberName(t, "startIndex"), it),
		g.blk.StructExtract(v, memberName(t, "endIndex"), it)
}

// checkedWord is a checked arithmetic builtin on two Int words, which
// traps where the result does not fit.
func (g *gen) checkedWord(name string, a, b *sil.Value) *sil.Value {
	intT := types.Typ[types.Int]
	word := sil.Object(builtinFor(intT))
	bit := sil.Object(sil.BuiltinInt1)
	pair := sil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: builtinFor(intT)},
		{Type: sil.BuiltinInt1},
	}})
	both := g.blk.Builtin(name, pair, a, b, g.blk.IntegerLiteral(bit, -1))
	g.blk.CondFail(g.blk.TupleExtract(both, 1, bit), "arithmetic overflow")
	return g.blk.TupleExtract(both, 0, word)
}

// sliceProperty lowers `s.count` and `s.isEmpty` of an ArraySlice: what
// lies between its bounds.
func (g *gen) sliceProperty(e *ast.MemberExpr) (*sil.Value, bool) {
	if e.Name == nil {
		return nil, false
	}
	name := g.text(e.Name)
	if name != "count" && name != "isEmpty" {
		return nil, false
	}
	t := g.typeOf(e.X)
	if _, ok := g.arraySlice(t); !ok {
		return nil, false
	}
	v := g.expr(e.X)
	if v == nil {
		return nil, true
	}
	intT := types.Typ[types.Int]
	lo, hi := g.sliceBounds(v, t)
	start, end := g.machine(lo, intT), g.machine(hi, intT)
	if name == "isEmpty" {
		same := g.blk.Builtin("cmp_eq_Int64", sil.Object(sil.BuiltinInt1), start, end)
		return g.blk.Struct(lowerType(types.Typ[types.Bool]), same), true
	}
	return g.blk.Struct(lowerType(intT), g.checkedWord("ssub_with_overflow_Int64", end, start)), true
}

// arraySliceSubscript lowers `a[lo..<hi]` and `a[lo...hi]`: the array and
// its bounds, checked against its count.
func (g *gen) arraySliceSubscript(e *ast.SubscriptExpr) (*sil.Value, bool) {
	t := g.typeOf(e)
	if _, ok := g.arraySlice(t); !ok || len(e.Args) != 1 {
		return nil, false
	}
	_, kind, ok := g.anyRangeOf(e.Args[0].X)
	if !ok {
		return nil, false
	}
	arrType := g.typeOf(e.X)
	count, ok := core.LowerMember(arrType, "count")
	if !ok {
		g.refuse(e, "a slice of an array whose count this cannot read")
		return nil, true
	}
	arr := g.rvalue(e.X)
	if arr == nil {
		return nil, true
	}
	intT := types.Typ[types.Int]
	word := sil.Object(builtinFor(intT))
	bit := sil.Object(sil.BuiltinInt1)
	n := g.machine(g.readProperty(e.X, arr, arrType, count, "count"), intT)
	// The bounds: both written, or one of them and the array's own end
	// for the other -- `a[2...]` is from 2 to the count, `a[..<2]` from 0.
	var start, end *sil.Value
	switch kind {
	case "Range", "ClosedRange":
		lo, hi := g.rangeBounds(e.Args[0].X)
		if lo == nil || hi == nil {
			return nil, true
		}
		start, end = g.machine(lo, intT), g.machine(hi, intT)
	case "PartialRangeFrom":
		lo := g.partialBound(e.Args[0].X)
		if lo == nil {
			return nil, true
		}
		start, end = g.machine(lo, intT), n
	default:
		hi := g.partialBound(e.Args[0].X)
		if hi == nil {
			return nil, true
		}
		start, end = g.blk.IntegerLiteral(word, 0), g.machine(hi, intT)
	}
	if kind == "ClosedRange" || kind == "PartialRangeThrough" {
		end = g.checkedWord("sadd_with_overflow_Int64", end, g.blk.IntegerLiteral(word, 1))
	}
	g.blk.CondFail(g.blk.Builtin("cmp_slt_Int64", bit, start, g.blk.IntegerLiteral(word, 0)),
		"Array index is out of range")
	g.blk.CondFail(g.blk.Builtin("cmp_slt_Int64", bit, end, start),
		"Range requires lowerBound <= upperBound")
	g.blk.CondFail(g.blk.Builtin("cmp_sgt_Int64", bit, end, n),
		"Array index is out of range")
	v := g.blk.Struct(lowerType(t), arr, g.blk.Struct(lowerType(intT), start), g.blk.Struct(lowerType(intT), end))
	g.destroyLater(v)
	return v, true
}
