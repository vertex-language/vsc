package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"math"
	"strconv"
)

// expr lowers an expression to a value.
func (g *gen) expr(e ast.Expr) *sil.Value {
	if self, ok := g.info.ImplicitSelf[e]; ok {
		return g.expr(self)
	}
	if g.info.ChainRoots[e] && !g.chainActive[e] {
		return g.chainRoot(e)
	}
	if opt, ok := e.(*ast.OptionalExpr); ok {
		return g.chainStep(opt)
	}
	if opt, ok := g.info.Unwrapped[e]; ok {
		return g.implicitUnwrap(e, opt)
	}
	if li := g.info.LiteralInits[e]; li != nil {
		return g.literalInit(e, li)
	}
	if sig := g.info.Autoclosures[e]; sig != nil {
		return g.autoclosure(e, sig)
	}
	switch n := e.(type) {
	case *ast.StringLit:
		return g.stringLiteral(n)

	case *ast.ArrayLit:
		return g.arrayLiteral(n)
	case *ast.DictLit:
		return g.dictionaryLiteral(n)
	case *ast.BasicLit:
		return g.literal(n)
	case *ast.MagicLit:
		return g.magicLiteral(n)

	case *ast.IdentExpr:
		return g.ident(n)

	case *ast.ForceExpr:
		return g.forceUnwrap(n)

	case *ast.MemberExpr:
		return g.member(n)

	// A key path used as a function is the closure the checker read it as;
	// one used as a value, the KeyPath of its closures.
	case *ast.KeyPathExpr:
		if kv := g.info.KeyPathValues[n]; kv != nil {
			return g.keyPathValue(kv)
		}
		if cl := g.info.KeyPathClosures[n]; cl != nil {
			return g.expr(cl)
		}
		g.unsupported(n)
		return nil

	// `Int.self`, and a type written where a value goes: its metatype.
	case *ast.PostfixSelfExpr, *ast.TypeExpr:
		if meta, ok := g.typeOf(n).(*types.Metatype); ok {
			return g.metatypeValue(n, meta.Instance)
		}
		if ps, ok := n.(*ast.PostfixSelfExpr); ok {
			return g.expr(ps.X)
		}
		g.unsupported(n)
		return nil

	case *ast.SubscriptExpr:
		return g.subscript(n)

	case *ast.AwaitExpr:
		return g.await(n)
	case *ast.TryExpr:
		return g.tryExpr(n)

	// `&a` passes variable address under modify access.
	case *ast.InOutExpr:
		addr := g.lvalue(n.X)
		if addr == nil {
			g.refuse(n, "'&' on "+g.exprKind(n.X)+", which has no address")
			return nil
		}
		if p, ok := pointerOf(g.typeOf(n)); ok {
			kind := "modify"
			if !p.Mutable {
				kind = "read"
			}
			access := g.blk.BeginAccess(addr, kind, "unknown")
			g.endAccessLater(access)
			return g.blk.AddressToPointer(access, lowerType(g.typeOf(n)))
		}
		access := g.blk.BeginAccess(addr, "modify", "unknown")
		g.endAccessLater(access)
		return access

	case *ast.TupleExpr:
		// Parenthesized single expression is not a tuple.
		if len(n.Elems) == 1 && n.Elems[0].Label == nil {
			return g.expr(n.Elems[0].X)
		}
		tt, _ := g.typeOf(n).Underlying().(*types.Tuple)
		elems := make([]*sil.Value, 0, len(n.Elems))
		for i, el := range n.Elems {
			v := g.rvalue(el.X)
			if v == nil {
				return nil
			}
			// An element the tuple holds as an optional is wrapped.
			if tt != nil && i < len(tt.Elements) {
				v = g.optionalFor(el.X, v, g.typeOf(el.X), tt.Elements[i].Type)
			}
			elems = append(elems, v)
		}
		// The tuple owns its elements now, and is let go of as any
		// temporary is, unless something takes it.
		tuple := g.blk.Tuple(lowerType(g.typeOf(n)), elems...)
		g.destroyLater(tuple)
		return tuple

	// `.red` implicit member shorthand.
	case *ast.ImplicitMemberExpr:
		if n.Name == nil {
			return nil
		}
		if t, ok := g.info.OptionalNones[n]; ok {
			return g.blk.Enum(lowerType(g.substituted(t)), optionalNone, nil)
		}
		// `.max` where an integer is wanted is a constant, as `Int.max` is.
		if v := g.constant(n); v != nil {
			return v
		}
		ec, ok := g.info.Uses[n.Name].(*analyzer.EnumCaseSymbol)
		if !ok {
			// A static property of the contextual type, read as
			// Type.name would be.
			if recv := g.typeOf(n); recv != nil {
				name := g.text(n.Name)
				for _, f := range g.staticsOf(recv) {
					if f != nil && f.Name == name {
						return g.staticRead(&ast.MemberExpr{Name: n.Name}, recv, f)
					}
				}
			}
			g.refuse(n, "this implicit member")
			return nil
		}
		return g.enumCase(n, ec)

	case *ast.CastExpr:
		return g.cast(n)

	case *ast.OperatorExpr:
		return g.operatorValue(n)

	case *ast.ClosureExpr:
		return g.closure(n)

	case *ast.CallExpr:
		return g.call(n)

	case *ast.ParenExpr:
		return g.expr(n.X)

	case *ast.SelfExpr:
		// `[weak self]`'s self, inside the closure that took it.
		if sym := g.info.SelfVars[n]; sym != nil {
			if l := g.locals[sym]; l != nil && l.cell != "" {
				return g.cellRead(l)
			}
		}
		if g.self != nil && g.self.addr != nil {
			return g.loaded(g.blk.Load(g.self.addr, loadQualifier(g.self.typ)), g.self.typ)
		}
		return g.selfValue()

	case *ast.SequenceExpr:
		if folded, ok := g.info.Folded[n]; ok {
			return g.expr(folded)
		}

	case *ast.BinaryExpr:
		return g.binary(n)

	case *ast.ConditionalExpr:
		return g.conditional(n)

	case *ast.PrefixExpr:
		return g.prefix(n)

	case *ast.PostfixExpr:
		return g.postfix(n)

	case *ast.StmtExpr:
		return g.stmtExpr(n)
	}
	g.unsupported(e)
	return nil
}

// prefix lowers a prefix operator using core expansion steps.
func (g *gen) prefix(e *ast.PrefixExpr) *sil.Value {
	if v := g.constant(e); v != nil {
		return v
	}
	sym, _ := g.info.Operators[e].(*analyzer.FuncSymbol)
	if ref := g.info.OperatorMethods[e]; ref != nil || (sym != nil && !g.coreOperator(sym)) {
		return g.operatorCall(e, ref, sym, e.X)
	}
	if v, isRange := g.partialRange(e, e.X); isRange {
		return v
	}
	if sym == nil {
		g.expr(e.X)
		g.unsupported(e)
		return nil
	}
	operand := g.typeOf(e.X)
	steps, ok := core.LowerPrefix(sym.Name(), operand)
	if !ok {
		g.expr(e.X)
		g.unsupported(e)
		return nil
	}
	v := g.expr(e.X)
	if v == nil {
		return nil
	}
	result := lowerType(sym.Signature().Results)
	if len(steps) == 0 {
		return v
	}

	flags := make(map[int64]*sil.Value, 1)
	cur := g.machine(v, operand)
	for _, st := range steps {
		cur = g.step(st, cur, flags)
		if cur == nil {
			return nil
		}
	}
	return g.blk.Struct(result, cur)
}

// step emits one builtin of a prefix operator's expansion.
func (g *gen) step(st core.Step, operand *sil.Value, flags map[int64]*sil.Value) *sil.Value {
	out := sil.Object(builtinNamed(st.Result))
	args := []*sil.Value{operand}
	if st.HasConst {
		k := g.blk.IntegerLiteral(sil.Object(builtinNamed(st.Result)), st.Const)
		if st.ConstLeft {
			args = []*sil.Value{k, operand}
		} else {
			args = append(args, k)
		}
	}
	if !st.Overflows {
		return g.blk.Builtin(st.Name, out, args...)
	}

	// -1 reports overflow, 0 wraps.
	want := int64(0)
	if st.Reports {
		want = -1
	}
	flag, ok := flags[want]
	if !ok {
		flag = g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), want)
		flags[want] = flag
	}
	args = append(args, flag)
	pair := sil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: builtinNamed(st.Result)},
		{Type: sil.BuiltinInt1},
	}})
	both := g.blk.Builtin(st.Name, pair, args...)
	value := g.blk.TupleExtract(both, 0, out)
	if st.Reports {
		flag := g.blk.TupleExtract(both, 1, sil.Object(sil.BuiltinInt1))
		g.blk.CondFail(flag, "arithmetic overflow")
	}
	return value
}

// rvalue lowers an expression and consumes it for transfer of ownership.
func (g *gen) rvalue(e ast.Expr) *sil.Value {
	v := g.expr(e)
	if v == nil {
		return nil
	}
	return g.consume(v)
}

// consume transfers ownership: copies guaranteed values and forgets cleanups for owned values.
func (g *gen) consume(v *sil.Value) *sil.Value {
	if v == nil {
		return nil
	}
	if v.Ownership() == sil.Guaranteed {
		return g.blk.CopyValue(v)
	}
	g.forget(v)
	return v
}

// literal lowers a basic literal to a struct wrapping the builtin value.
func (g *gen) literal(e *ast.BasicLit) *sil.Value {
	if e.Kind == token.NIL {
		if v, ok := g.nilValue(e); ok {
			return v
		}
	}
	return g.constant(e)
}

// constant emits a constant folded value for an expression.
func (g *gen) constant(e ast.Expr) *sil.Value {
	v, ok := g.info.Values[e]
	if !ok {
		return nil
	}
	t := g.typeOf(e)
	// A literal typed as an optional, however deep, is the number
	// wrapped that deep.
	payload := t
	for {
		o, isOpt := optionalOf(payload)
		if !isOpt {
			break
		}
		payload = o.Wrapped
	}
	var made *sil.Value
	switch v.Kind {
	case analyzer.IntValue:
		if isFloat(payload) {
			// The value is two's complement: -1000 is held as its bits, and
			// read unsigned it is a number near 2^64.
			n := int64(v.Int)
			bits := int64(math.Float64bits(float64(n)))
			if b, ok := payload.Underlying().(*types.Basic); ok && b.Kind() == types.Float {
				bits = int64(math.Float32bits(float32(n)))
			}
			made = g.blk.Struct(lowerType(payload),
				g.blk.FloatLiteral(sil.Object(builtinFor(payload)), bits))
			break
		}
		raw := g.blk.IntegerLiteral(sil.Object(builtinFor(payload)), int64(v.Int))
		made = g.blk.Struct(lowerType(payload), raw)
	case analyzer.FloatValue:
		bits := int64(math.Float64bits(v.Float))
		if b, ok := payload.Underlying().(*types.Basic); ok && b.Kind() == types.Float {
			bits = int64(math.Float32bits(float32(v.Float)))
		}
		raw := g.blk.FloatLiteral(sil.Object(builtinFor(payload)), bits)
		made = g.blk.Struct(lowerType(payload), raw)
	case analyzer.BoolValue:
		n := int64(0)
		if v.Bool {
			n = 1
		}
		raw := g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), n)
		made = g.blk.Struct(lowerType(payload), raw)
	default:
		return nil
	}
	return g.optionalFor(e, made, payload, t)
}

// builtinFor returns the underlying machine builtin type for basic types.
func builtinFor(t types.Type) types.Type {
	if t == nil {
		return sil.BuiltinInt64
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return sil.BuiltinInt64
	}
	switch b.Kind() {
	case types.Int8, types.UInt8:
		return sil.BuiltinInt8
	case types.Int16, types.UInt16:
		return sil.BuiltinInt16
	case types.Int32, types.UInt32:
		return sil.BuiltinInt32
	case types.Bool:
		return sil.BuiltinInt1
	case types.Float:
		return sil.BuiltinFPIEEE32
	case types.Double:
		return sil.BuiltinFPIEEE64
	}
	return sil.BuiltinInt64
}

// ident lowers an identifier reference to a value or storage address.
func (g *gen) ident(e *ast.IdentExpr) *sil.Value {
	sym := g.info.Uses[e.Name]
	if sym == nil {
		return nil
	}
	l := g.locals[sym]
	if l == nil {
		// A computed module-level variable, read by calling its getter.
		if v, ok := g.moduleGetterCall(sym); ok {
			return v
		}
		// A module-level variable, read through its addressor.
		if addr, t, ok := g.moduleVarAddr(sym); ok {
			access := g.blk.BeginAccess(addr, "read", "unknown")
			v := g.blk.Load(access, loadQualifier(t))
			g.blk.EndAccess(access)
			return g.loaded(v, t)
		}
		// Function value reference.
		if fs, ok := sym.(*analyzer.FuncSymbol); ok {
			return g.funcValue(fs)
		}
		// Implicit self stored property.
		if v, ok := g.implicitSelf(e, sym); ok {
			return v
		}
		// Implicit static member.
		if v, ok := g.implicitStatic(e); ok {
			return v
		}
		// Implicit computed member.
		if v, ok := g.implicitComputed(e); ok {
			return v
		}
		g.unsupported(e)
		return nil
	}
	if l.cell != "" {
		return g.cellRead(l)
	}
	// Storage that is not a value in the first place: the use gets
	// where it lives, because there is no loading it out.
	if l.mem {
		if g.storage == nil {
			g.storage = map[*sil.Value]bool{}
		}
		g.storage[l.addr] = true
		return l.addr
	}
	if l.addr != nil {
		access := g.blk.BeginAccess(l.addr, "read", "unknown")
		v := g.blk.Load(access, loadQualifier(l.typ))
		g.blk.EndAccess(access)
		return g.loaded(v, l.typ)
	}
	if l.value.Ownership() == sil.Owned {
		return g.borrow(l.value)
	}
	// A parameter passed by address -- an existential, @in_guaranteed --
	// is the caller's storage, which a use reads and never ends.
	if l.value.Type().IsAddress() {
		if g.storage == nil {
			g.storage = map[*sil.Value]bool{}
		}
		g.storage[l.value] = true
	}
	return l.value
}

// implicitSelf reads a stored property of the receiver for an implicit self reference.
func (g *gen) implicitSelf(e *ast.IdentExpr, sym analyzer.Symbol) (*sil.Value, bool) {
	if g.recv == nil || e.Name == nil {
		return nil, false
	}
	name := g.text(e.Name)
	owner, field, ok := storedField(g.recv, name)
	if !ok {
		return nil, false
	}
	t := lowerType(field.Type)
	member := memberName(owner, name)
	// Read property via address if receiver is mutable storage.
	if g.self != nil && g.self.addr != nil && !isClass(g.recv) {
		addr := g.blk.StructElementAddr(g.self.addr, member, t.Address())
		access := g.blk.BeginAccess(addr, "read", "unknown")
		v := g.blk.Load(access, loadQualifier(t))
		g.blk.EndAccess(access)
		return g.loaded(v, t), true
	}
	self := g.selfValue()
	if self == nil {
		return nil, false
	}
	if isClass(g.recv) {
		addr := g.blk.RefElementAddr(self, member, t)
		if field.Ref != "" {
			return g.refLoad(addr, field), true
		}
		access := g.blk.BeginAccess(addr, "read", "dynamic")
		v := g.blk.Load(access, loadQualifier(t))
		g.blk.EndAccess(access)
		return g.loaded(v, t), true
	}
	return g.blk.StructExtract(self, member, t), true
}

// storedField finds a stored property by name in the type or its superclass chain.
func storedField(t types.Type, name string) (types.Type, *types.Field, bool) {
	for _, held := range storedChain(t) {
		if held.field.Name == name {
			return held.owner, held.field, true
		}
	}
	return nil, nil, false
}

// borrow begins a borrow scope cleaned up when the current scope ends.
func (g *gen) borrow(v *sil.Value) *sil.Value {
	b := g.blk.BeginBorrow(v)
	g.endBorrowLater(b)
	return b
}

// member lowers a property read or member access expression.
func (g *gen) member(e *ast.MemberExpr) *sil.Value {
	if e.Name == nil {
		return nil
	}
	// `Optional<T>.none`.
	if t, ok := g.info.OptionalNones[e]; ok {
		return g.blk.Enum(lowerType(g.substituted(t)), optionalNone, nil)
	}
	// A constant the checker worked out: `Int64.max`.
	if v := g.constant(e); v != nil {
		return v
	}
	// MemoryLayout<T>.size, now that T is known.
	if l, ok := g.info.Layouts[e]; ok {
		t := g.substituted(l.Of)
		var n int64
		switch l.Kind {
		case "size":
			n = types.Sizeof(t, types.DefaultTarget64)
		case "stride":
			n = types.Strideof(t, types.DefaultTarget64)
		case "alignment":
			n = types.Alignof(t, types.DefaultTarget64)
		}
		intT := types.Typ[types.Int]
		return g.blk.Struct(lowerType(intT), g.blk.IntegerLiteral(sil.Object(builtinFor(intT)), n))
	}
	// A property an existential's protocol requires: its getter witness.
	if ex, ok := existentialOf(g.typeOf(e.X)); ok {
		if v, handled := g.existentialProperty(e, ex); handled {
			return v
		}
	}
	// Enum case member.
	if ec, ok := g.info.Uses[e.Name].(*analyzer.EnumCaseSymbol); ok {
		return g.enumCase(e, ec)
	}
	// Module-qualified function value.
	if fs, ok := g.info.Uses[e.Name].(*analyzer.FuncSymbol); ok {
		return g.funcValue(fs)
	}
	// A module-qualified variable -- `main.limit` -- read as its name is.
	if id, ok := e.X.(*ast.IdentExpr); ok && id.Name != nil && g.info.Uses[id.Name] == nil {
		if sym := g.info.Uses[e.Name]; sym != nil {
			if v, ok := g.moduleGetterCall(sym); ok {
				return v
			}
			if addr, t, ok := g.moduleVarAddr(sym); ok {
				access := g.blk.BeginAccess(addr, "read", "unknown")
				v := g.blk.Load(access, loadQualifier(t))
				g.blk.EndAccess(access)
				return g.loaded(v, t)
			}
			if v, ok := g.importedVarRead(e, sym); ok {
				return v
			}
		}
	}
	if id, ok := e.X.(*ast.IdentExpr); ok && id.Name != nil && g.info.Uses[id.Name] == nil {
		g.refuse(e, "'"+g.text(e.X)+"."+g.text(e.Name)+"', which names something "+
			"other than a function through a module")
		return nil
	}
	// Raw value read for raw-representable enums.
	if en, ok := rawValueRead(g.typeOf(e.X), g.text(e.Name)); ok {
		return g.rawValue(e, en)
	}
	// Pointee load.
	if p, ok := g.isPointee(e); ok {
		return g.pointeeRead(e, p)
	}
	// Computed property getter call.
	if recv, f, ok := g.computedProperty(e); ok {
		return g.getter(e, recv, f)
	}
	// Static property read via accessor.
	if recv, f, ok := g.staticProperty(e); ok {
		return g.staticRead(e, recv, f)
	}
	if v, ok := g.sliceProperty(e); ok {
		return v
	}
	if v, ok := g.taskValue(e); ok {
		return v
	}
	if v, ok := g.stringUTF8(e); ok {
		return v
	}
	// Collection / stdlib property calls.
	if m, ok := core.LowerCollectionProperty(g.typeOf(e.X), g.text(e.Name)); ok {
		return g.collectionCall(e, m, e.X, nil)
	}
	if m, ok := core.LowerMember(g.typeOf(e.X), g.text(e.Name)); ok {
		return g.stdlibGetter(e, m)
	}
	if meta, isMeta := g.typeOf(e.X).(*types.Metatype); isMeta {
		if m, ok := core.LowerStaticMember(meta.Instance, g.text(e.Name)); ok {
			return g.runtimeCall(m.Symbol, lowerType(m.Result))
		}
	}
	base := g.expr(e.X)
	if base == nil {
		return nil
	}
	t := lowerType(g.typeOf(e))
	name := memberName(g.typeOf(e.X), e.Name.Text(g.file))

	if !isClass(g.typeOf(e.X)) {
		// Non-trivial fields extracted from owned structs must be copied.
		if !t.Trivial() && base.Ownership() == sil.Owned {
			borrow := g.blk.BeginBorrow(base)
			field := g.blk.CopyValue(g.blk.StructExtract(borrow, name, t))
			g.blk.EndBorrow(borrow)
			g.destroyLater(field)
			return field
		}
		return g.blk.StructExtract(base, name, t)
	}
	addr := g.blk.RefElementAddr(base, name, t)
	if _, f, ok := storedField(g.typeOf(e.X), e.Name.Text(g.file)); ok && f.Ref != "" {
		return g.refLoad(addr, f)
	}
	access := g.blk.BeginAccess(addr, "read", "dynamic")
	v := g.blk.Load(access, loadQualifier(t))
	g.blk.EndAccess(access)
	return g.loaded(v, t)
}

// loaded registers a cleanup for non-trivial copied loads.
func (g *gen) loaded(v *sil.Value, t sil.Type) *sil.Value {
	if v != nil && !t.Trivial() {
		g.destroyTemp(v)
	}
	return v
}

// call lowers a call expression to a resolved callee.
func (g *gen) call(e *ast.CallExpr) *sil.Value {
	// A call the checker read as one of a core function's.
	if call := g.info.CoreCalls[e]; call != nil {
		return g.expr(call)
	}
	if t, ok := g.info.TypeOfs[e]; ok {
		return g.typeOfValue(e, t)
	}
	if t, ok := g.info.EmptyCollections[e]; ok {
		return g.emptyCollection(e, t)
	}
	// `.some(x)`, `Optional(x)`: x, wrapped.
	if t, ok := g.info.OptionalSomes[e]; ok {
		from := e.Args.Args[0].X
		v := g.rvalue(from)
		if v == nil {
			return nil
		}
		return g.optionalFor(from, v, g.typeOf(from), g.substituted(t))
	}
	// Array(s) of any other sequence: its elements, appended in order.
	if it := g.info.ArraySequences[e]; it != nil {
		return g.arrayOfSequence(e, it)
	}
	// Array(xs) of an array is the array: a copy, as a value is.
	if _, ok := g.info.ArrayCopies[e]; ok {
		v := g.rvalue(e.Args.Args[0].X)
		if v != nil && !v.Type().Trivial() {
			g.destroyLater(v)
		}
		return v
	}
	if ref, ok := e.Fun.(*ast.InitRefExpr); ok {
		if v, ok := g.delegatingInit(e, ref); ok {
			return v
		}
		if v, ok := g.superInit(e, ref); ok {
			return v
		}
		if v, ok := g.metatypeInit(e, ref); ok {
			return v
		}
	}
	if mem, ok := e.Fun.(*ast.MemberExpr); ok {
		// Module-qualified calls.
		if fn, ok := g.info.Uses[mem.Name].(*analyzer.FuncSymbol); ok {
			return g.callFunc(e, fn)
		}
		if tn, ok := g.info.Uses[mem.Name].(*analyzer.TypeNameSymbol); ok {
			return g.construct(e, tn)
		}
		// Enum case constructor with payload.
		if ec, ok := g.info.Uses[mem.Name].(*analyzer.EnumCaseSymbol); ok {
			return g.payloadCase(e, ec)
		}
		// Collection runtime methods.
		if v, ok := g.withUnsafeBytes(e, mem); ok {
			return v
		}
		if v, ok := g.withCString(e, mem); ok {
			return v
		}
		if v, ok := g.withUnsafeMutableBufferPointer(e, mem); ok {
			return v
		}
		if v, ok := g.collectionMethodCall(e, mem); ok {
			return v
		}
		if v, ok := g.hasherCall(e, mem); ok {
			return v
		}
		return g.method(e, mem)
	}
	// Implicit member enum case call (.line(7)).
	if im, ok := e.Fun.(*ast.ImplicitMemberExpr); ok && im.Name != nil {
		// `.init(...)` where a T is wanted: T(...).
		if meta, isMeta := g.typeOf(im).(*types.Metatype); isMeta && g.text(im.Name) == "init" {
			return g.construct(e, analyzer.NewTypeName(typeName(meta.Instance), meta.Instance, im.Pos()))
		}
		if ec, ok := g.info.Uses[im.Name].(*analyzer.EnumCaseSymbol); ok {
			return g.payloadCase(e, ec)
		}
		// `.seconds(1)`: a static method of the type wanted.
		if ref := g.info.ImplicitMethods[im]; ref != nil {
			return g.staticCall(e, ref, ref.Recv)
		}
	}
	// `Array(repeating:count:)`, as `[T](repeating:count:)` below.
	if g.info.ArrayRepeats[e] {
		return g.arrayInit(e)
	}
	// `[T](...)`, which makes an array.
	if lit, ok := e.Fun.(*ast.ArrayLit); ok && len(lit.Items) == 1 {
		if _, isArray := arrayOf(g.typeOf(e)); isArray {
			return g.arrayInit(e)
		}
	}
	id, ok := e.Fun.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		// Anything else that names a function value -- an element of an
		// array of closures, a parenthesized closure -- is that value,
		// called.
		if t := g.typeOf(e.Fun); t != nil {
			if _, isFunc := t.Underlying().(*types.Signature); isFunc {
				return g.applyValue(e, e.Fun)
			}
		}
		g.unsupported(e)
		return nil
	}
	// A name bound to a function value in this scope -- a closure in a
	// let, a nested function that captures, a top-level let -- is that
	// value, called.
	if sym := g.info.Uses[id.Name]; sym != nil {
		_, isLocal := g.locals[sym]
		_, isGlobal := g.vars[sym]
		if isLocal || isGlobal {
			if t := g.typeOf(id); t != nil {
				if _, isFunc := t.Underlying().(*types.Signature); isFunc {
					return g.applyValue(e, id)
				}
			}
		}
	}
	// Implicit self method call.
	if ref, ok := g.implicitMethod(id); ok {
		// A static method named without its type has no self to pass:
		// `bind(host:port:)` inside another static method is Type.bind.
		if ref.Method.IsStatic {
			return g.staticCall(e, ref, ref.Recv)
		}
		var copied *sil.Value
		v := g.methodCall(e, ref, func() *sil.Value {
			if g.self != nil && g.self.addr != nil {
				if mutatingRef(ref) {
					return g.self.addr
				}
				// Inside a mutating method self is an address, and a
				// method that does not mutate takes its value: a copy,
				// as `self.f()` reads it.
				if !isClass(g.recv) {
					copied = g.loaded(g.blk.Load(g.self.addr, loadQualifier(g.self.typ)), g.self.typ)
					return copied
				}
			}
			return g.selfValue()
		})
		g.endReceiverTemp(copied)
		return v
	}
	// Pointer type conversions.
	if _, ok := pointerOf(g.typeOf(e)); ok {
		if v, isConversion := g.convert(e, g.typeOf(e)); isConversion {
			return v
		}
	}
	// Type constructor call.
	if tn, ok := g.info.Uses[id.Name].(*analyzer.TypeNameSymbol); ok {
		if g.isCoreHasher(tn.Type()) {
			return g.collectionCall(e, core.HasherInit(tn.Type()), nil, nil)
		}
		return g.construct(e, tn)
	}
	// Closure or function value invocation.
	if sym := g.info.Uses[id.Name]; sym != nil {
		if _, isFunc := g.typeOf(id).Underlying().(*types.Signature); isFunc {
			if _, isLocal := g.locals[sym]; isLocal {
				return g.applyValue(e, id)
			}
		}
	}

	sym, _ := g.info.Uses[id.Name].(*analyzer.FuncSymbol)
	if sym == nil {
		g.unsupported(e)
		return nil
	}
	return g.callFunc(e, sym)
}

// callFunc lowers a call to a named declared function.
func (g *gen) callFunc(e *ast.CallExpr, sym *analyzer.FuncSymbol) *sil.Value {
	return g.callFuncTry(e, sym, false)
}

// callFuncTry lowers a call, handling try? optional failure if specified.
func (g *gen) callFuncTry(e *ast.CallExpr, sym *analyzer.FuncSymbol, optional bool) *sil.Value {
	// A core function that is one instruction.
	if op, ok := g.builtinOp(sym); ok && op != "" {
		return g.callBuiltinOp(e, sym, op)
	}
	// Refuse unimported existential return functions.
	if _, isEx := existentialOf(sym.Signature().Results); isEx {
		if _, imported := g.info.Imported[sym]; !imported {
			return nil
		}
	}
	// Generic specialization or imported call. A core function written
	// as source -- zip, stride, min -- is specialized from that source,
	// as the program's own generic functions are.
	if len(sym.Signature().TypeParams) > 0 {
		spec, ok := g.info.Specializations[e]
		if !ok || spec.Empty() {
			g.refuse(e, "a call whose type arguments could not be inferred")
			return nil
		}
		if _, imported := g.info.Imported[sym]; imported && !hasBody(sym) {
			return g.callImportedGeneric(e, sym, spec)
		}
		return g.callGeneric(e, sym, spec, optional)
	}
	callee := g.m.Func(g.symbol(sym)).SetSourceName(sym.Name())
	if g.needsType(callee) {
		g.declare(callee, sym)
	}
	g.emitCoreBody(sym, callee)
	ref := g.blk.FunctionRef(callee)

	args, ok := g.arguments(e, sym.Signature())
	if !ok {
		return nil
	}
	result := lowerType(sym.Signature().Results)
	if sym.Signature().Throws {
		// A function swiftc built boxes its error Swift's way; one another
		// Vertex module declares throws as this one does.
		module, isImported := g.info.Imported[sym]
		imported := isImported && (module == "Swift" || g.info.SwiftModules[module])
		// A `try?` or `try!` written on a call reached through a member --
		// `try? Task.sleep(...)` -- is left for the call that takes it.
		if pendingOptional, pendingTrap := g.tryOn(e); pendingOptional || pendingTrap {
			optional = optional || pendingOptional
			g.tryBang = pendingTrap
		}
		// A rethrows function given nothing that throws cannot fail, so
		// nothing has to catch it: its error edge is never taken.
		if sym.Signature().Rethrows && !optional && !g.argumentThrows(e) {
			g.tryBang = true
		}
		return g.tryApply(e, ref, args, sym.Signature().Results, optional, imported)
	}
	v := g.blk.Apply(ref, result, args...)
	g.destroyLater(v)
	return v
}

// hasBody reports whether a function's declaration has a body: one the
// core writes as source rather than declares for the runtime.
func hasBody(sym *analyzer.FuncSymbol) bool {
	decl, _ := sym.Decl().(*ast.FuncDecl)
	return decl != nil && decl.Body != nil
}

// emitCoreBody lowers a core function written as source, once, where a
// program calls it: the core is not compiled ahead, so a module holds
// its own copy of what it uses.
func (g *gen) emitCoreBody(sym *analyzer.FuncSymbol, callee *sil.Func) {
	if g.info.Imported[sym] != "Swift" || !hasBody(sym) || !callee.IsDeclaration() {
		return
	}
	if err := g.emitSpecialization(sym, callee.Name(), nil); err != nil {
		g.errorAt(sym.Decl(), err.Error())
	}
}

// funcValue converts a declared function into a thick function value.
func (g *gen) funcValue(sym *analyzer.FuncSymbol) *sil.Value {
	callee := g.m.Func(g.symbol(sym)).SetSourceName(sym.Name())
	if g.needsType(callee) {
		g.declare(callee, sym)
	}
	ref := g.blk.FunctionRef(callee)
	v := g.blk.ThinToThickFunction(ref, lowerType(sym.Signature()))
	g.destroyLater(v)
	return v
}

// applyValue calls a function value held in a variable or closure.
func (g *gen) applyValue(e *ast.CallExpr, id ast.Expr) *sil.Value {
	sig, _ := g.typeOf(id).Underlying().(*types.Signature)
	if sig == nil {
		g.unsupported(e)
		return nil
	}
	callee := g.expr(id)
	if callee == nil {
		return nil
	}
	var args []*sil.Value
	if e.Args != nil {
		for _, a := range e.Args.Args {
			// Borrowed where they are, as an ordinary call's arguments
			// are: a closure's parameters are borrowed too, and taking one
			// left a temporary -- or a copy -- that nothing let go.
			v := g.expr(a.X)
			if v == nil {
				return nil
			}
			args = append(args, v)
		}
	}
	if sig.Throws {
		optional, trap := g.tryOn(e)
		g.tryBang = trap
		return g.tryApply(e, callee, args, sig.Results, optional, false)
	}
	v := g.blk.Apply(callee, lowerType(sig.Results), args...)
	g.destroyLater(v)
	return v
}

// construct lowers making an instance of a type.
func (g *gen) construct(e *ast.CallExpr, tn *analyzer.TypeNameSymbol) *sil.Value {
	if en, ok := g.info.RawInits[e]; ok {
		return g.rawInit(e, en)
	}
	t := g.typeOf(e)
	// An init? call is typed as the optional it makes; the type it makes
	// one of is what is constructed.
	if sig := g.info.Inits[e]; sig != nil && sig.Failable {
		if o, ok := t.(*types.Optional); ok {
			t = o.Wrapped
		}
	}
	// An initializer an extension of Int or String declares.
	if sig := g.info.Inits[e]; sig != nil && isBasicValue(t) {
		var args []*ast.CallArg
		if e.Args != nil {
			args = e.Args.Args
		}
		out := *sig
		out.Results = t
		g.emitCoreInit(t, &out)
		return g.applyInit(e, t, &out, args)
	}
	// Int("42") and String(decoding:as:): the runtime reads the text.
	if v, ok := g.basicInit(e, t); ok {
		return v
	}
	// Type conversions: Int32(n).
	if v, isConversion := g.convert(e, t); isConversion {
		return v
	}
	if v, ok := g.stringFromCString(e, t); ok {
		return v
	}
	// String(x): a String is itself, and anything else is what
	// interpolating it would write.
	if b, ok := t.Underlying().(*types.Basic); ok && b.Kind() == types.String &&
		e.Args != nil && len(e.Args.Args) == 1 && e.Args.Args[0].Label == nil {
		arg := e.Args.Args[0].X
		if from := g.typeOf(arg); from != nil {
			if fb, ok := from.Underlying().(*types.Basic); ok && fb.Kind() == types.String {
				v := g.rvalue(arg)
				g.destroyLater(v)
				return v
			}
		}
		return g.describing(e, arg)
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		if cl, isClass := t.Underlying().(*types.Class); isClass {
			return g.makeClass(e, tn, cl, t)
		}
		g.unsupported(e)
		return nil
	}
	// Custom initializers take precedence over memberwise construction:
	// all of them where the body declares one, and where only extensions
	// do, the one the checker chose.
	if st.Memberwise() == nil || g.info.Inits[e] != nil {
		return g.callInit(e, t, st)
	}
	return g.memberwise(e, t, st)
}

// memberwise lowers a call of a struct's memberwise initializer: one value
// per stored property, the argument for it or its default.
func (g *gen) memberwise(e *ast.CallExpr, t types.Type, st *types.Struct) *sil.Value {
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}

	// Evaluate one value per stored property in declaration order.
	values := make([]*sil.Value, 0, len(st.Fields))
	next := 0
	for _, f := range st.Fields {
		if f == nil {
			g.refuse(e, "a constructor of a type whose properties this cannot read")
			return nil
		}
		var from ast.Expr
		var subst map[*types.TypeParam]types.Type
		switch {
		case next < len(args) && g.argIsFor(args[next], f):
			from = args[next].X
			next++
		case f.HasDefault:
			from = g.info.FieldDefaults[f]
			if from == nil {
				from, subst = g.instanceDefault(t, f.Name)
			}
		}
		if from == nil {
			g.refuse(e, "a constructor with no value for '"+f.Name+"'")
			return nil
		}
		var v *sil.Value
		if subst != nil {
			prev := g.subst
			g.subst = subst
			v = g.rvalue(from)
			g.subst = prev
		} else {
			v = g.rvalue(from)
		}
		if v == nil {
			return nil
		}
		// A property of type T? given a T holds it wrapped.
		values = append(values, g.optionalFor(from, v, g.typeOf(from), f.Type))
	}
	if next != len(args) {
		g.refuse(e, "a constructor whose arguments do not match the properties in order")
		return nil
	}
	made := g.blk.Struct(lowerType(t), values...)
	g.destroyLater(made)
	return made
}

// instanceDefault is the value a generic struct's declaration gives a
// property, for an instance of it: the property of the instance is the
// declaration's with its type substituted, and not the one the default is
// recorded against. The default is lowered with the same substitution.
func (g *gen) instanceDefault(t types.Type, name string) (ast.Expr, map[*types.TypeParam]types.Type) {
	gi, ok := t.(*types.GenericInstance)
	if !ok || gi.Base == nil {
		return nil, nil
	}
	var fields []*types.Field
	params := nominalTypeParams(gi.Base)
	switch base := gi.Base.Underlying().(type) {
	case *types.Struct:
		fields = base.Fields
	case *types.Class:
		fields = base.Fields
	}
	if fields == nil || len(params) != len(gi.Args) {
		return nil, nil
	}
	for _, bf := range fields {
		if bf == nil || bf.Name != name {
			continue
		}
		from := g.info.FieldDefaults[bf]
		if from == nil {
			return nil, nil
		}
		subst := make(map[*types.TypeParam]types.Type, len(g.subst)+len(params))
		for k, v := range g.subst {
			subst[k] = v
		}
		for i, p := range params {
			subst[p] = gi.Args[i]
		}
		return from, subst
	}
	return nil, nil
}

// argIsFor reports whether an argument corresponds to the given field by label or position.
func (g *gen) argIsFor(a *ast.CallArg, f *types.Field) bool {
	if a == nil {
		return false
	}
	if a.Label == nil {
		return true
	}
	// A lazy property's storage is labelled as the property is.
	if f.LazyOf != "" {
		return g.text(a.Label) == f.LazyOf
	}
	return g.text(a.Label) == f.Name
}

// makeClass instantiates a class, storing initial property values into allocated reference storage.
func (g *gen) makeClass(e *ast.CallExpr, tn *analyzer.TypeNameSymbol, cl *types.Class, t types.Type) *sil.Value {
	if len(cl.Inits) > 0 {
		return g.callClassInit(e, t, cl)
	}
	if e.Args != nil && len(e.Args.Args) > 0 {
		g.errorAt(e, "'"+tn.Name()+"' has no initializer taking arguments")
		return nil
	}

	// Verify all stored properties have defaults when no init is declared.
	// A `var` of an optional type starts as nil, as it does in Swift.
	held := storedChain(t)
	for _, held := range held {
		if _, ok := g.classDefaults(held.owner)[held.field.Name]; !ok && !g.implicitlyNil(held.owner, held.field) {
			g.errorAt(e, "'"+tn.Name()+"' cannot be made without arguments: '"+
				held.field.Name+"' has no initial value and there is no "+
				"initializer to give it one")
			return nil
		}
	}

	obj := g.blk.AllocRef(lowerType(t))
	// Initialize stored properties in inheritance order (superclasses first).
	for _, held := range held {
		ft := lowerType(held.field.Type)
		var v *sil.Value
		if init, ok := g.classDefaults(held.owner)[held.field.Name]; ok {
			v = g.rvalue(init)
		} else {
			v = g.blk.Enum(ft, optionalNone, nil)
		}
		if v == nil {
			return nil
		}
		addr := g.blk.RefElementAddr(obj, memberName(held.owner, held.field.Name), ft)
		g.blk.Store(v, addr, storeQualifier(ft))
	}
	g.destroyLater(obj)
	return obj
}

// heldField pairs a stored property with the class declaring it.
type heldField struct {
	owner types.Type
	field *types.Field
}

// storedChain returns all stored properties in an instance, superclass first.
func storedChain(t types.Type) []heldField {
	if t == nil {
		return nil
	}
	if st, ok := t.Underlying().(*types.Struct); ok {
		var out []heldField
		for _, f := range st.Fields {
			if f != nil {
				out = append(out, heldField{owner: t, field: f})
			}
		}
		return out
	}
	var chain []types.Type
	seen := map[*types.Class]bool{}
	for cur := t; cur != nil; {
		cl, ok := cur.Underlying().(*types.Class)
		if !ok || seen[cl] {
			break
		}
		seen[cl] = true
		chain = append([]types.Type{cur}, chain...)
		cur = cl.Superclass
	}
	var out []heldField
	for _, owner := range chain {
		cl, _ := owner.Underlying().(*types.Class)
		if cl == nil {
			continue
		}
		for _, f := range cl.Fields {
			if f != nil {
				out = append(out, heldField{owner: owner, field: f})
			}
		}
	}
	return out
}

// classDefaults returns initial values of a class's stored properties mapped by name.
func (g *gen) classDefaults(t types.Type) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	body := classBody(g.classDecl(t))
	if body == nil {
		return out
	}
	for _, mem := range body.Members {
		vd, ok := mem.(*ast.VarDecl)
		if !ok {
			continue
		}
		for _, b := range vd.Bindings {
			if b.Value == nil {
				continue
			}
			if name := g.bindingName(b.Pat); name != "" {
				out[name] = b.Value
			}
		}
	}
	return out
}

// implicitlyNil reports whether a stored property that is given no value
// starts as nil: a `var` of an optional type, as Swift has it.
func (g *gen) implicitlyNil(owner types.Type, f *types.Field) bool {
	if _, ok := optionalOf(f.Type); !ok {
		return false
	}
	// A lazy property's storage: nil until it is first read.
	if f.LazyOf != "" {
		return true
	}
	body := classBody(g.classDecl(owner))
	if body == nil {
		return false
	}
	for _, mem := range body.Members {
		vd, ok := mem.(*ast.VarDecl)
		if !ok || vd.Kind != token.VAR {
			continue
		}
		for _, b := range vd.Bindings {
			if g.bindingName(b.Pat) == f.Name {
				return true
			}
		}
	}
	return false
}

// classDecl finds the AST declaration for a class type.
func (g *gen) classDecl(t types.Type) ast.Decl {
	if t == nil {
		return nil
	}
	want := t.Underlying()
	for _, sym := range g.info.Defs {
		tn, ok := sym.(*analyzer.TypeNameSymbol)
		if !ok || tn.Type() == nil {
			continue
		}
		if tn.Type().Underlying() == want {
			return tn.Decl()
		}
	}
	return nil
}

// classBody returns the member block of a class or actor declaration.
func classBody(d ast.Decl) *ast.MemberBlock {
	switch n := d.(type) {
	case *ast.ClassDecl:
		return n.Body
	case *ast.ActorDecl:
		return n.Body
	}
	return nil
}

// bindingName extracts the identifier from a pattern if it binds a single name.
func (g *gen) bindingName(p ast.Pattern) string {
	if tp, ok := p.(*ast.TypedPattern); ok {
		p = tp.Pat
	}
	if id, ok := p.(*ast.IdentPattern); ok {
		return g.text(id.Name)
	}
	return ""
}

// payloadCase builds an enum case carrying associated values.
func (g *gen) payloadCase(e *ast.CallExpr, ec *analyzer.EnumCaseSymbol) *sil.Value {
	assoc := ec.AssociatedType()
	if assoc == nil {
		g.refuse(e, "a case that carries nothing, called as though it did")
		return nil
	}
	t := g.typeOf(e)
	if t == nil {
		t = ec.Type()
	}
	// The case of the enum as used, whose payload has its generic
	// parameters filled in.
	k := g.caseOf(t, ec.Name())
	if k != nil && k.AssociatedType != nil {
		assoc = k.AssociatedType
	}
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	// A value given for an optional the case carries -- `.pair(a, 3)`
	// where a is an Item and the case takes an Item? -- is wrapped.
	var payload *sil.Value
	switch {
	case len(args) == 1:
		payload = g.rvalue(args[0].X)
		if payload != nil {
			payload = g.optionalFor(args[0].X, payload, g.typeOf(args[0].X), assoc)
		}
	default:
		tu, _ := assoc.Underlying().(*types.Tuple)
		vals := make([]*sil.Value, 0, len(args))
		for i, a := range args {
			v := g.rvalue(a.X)
			if v == nil {
				return nil
			}
			if tu != nil && i < len(tu.Elements) {
				v = g.optionalFor(a.X, v, g.typeOf(a.X), tu.Elements[i].Type)
			}
			vals = append(vals, v)
		}
		payload = g.blk.Tuple(lowerType(assoc), vals...)
	}
	if payload == nil {
		return nil
	}
	// An indirect case carries its payload in a box of its own.
	if k != nil && k.Indirect {
		lt := lowerType(assoc)
		box := g.blk.AllocBoxOf(lowerType(sil.CaseStorage(k)), "", "var")
		g.blk.Store(payload, g.blk.ProjectBox(box, 0, lt), storeQualifier(lt))
		payload = box
	}
	v := g.blk.Enum(lowerType(t), memberName(ec.Type(), ec.Name()), payload)
	g.destroyLater(v)
	return v
}

// enumCase builds a no-payload enum case.
func (g *gen) enumCase(e ast.Expr, ec *analyzer.EnumCaseSymbol) *sil.Value {
	if ec.AssociatedType() != nil {
		g.refuse(e, "an enum case that carries a value")
		return nil
	}
	t := g.typeOf(e)
	if t == nil {
		t = ec.Type()
	}
	return g.blk.Enum(lowerType(t), memberName(ec.Type(), ec.Name()), nil)
}

// method lowers a method call expression.
func (g *gen) method(e *ast.CallExpr, mem *ast.MemberExpr) *sil.Value {
	ref := g.info.Methods[mem]
	if ref == nil || ref.Method == nil || ref.Method.Sig == nil {
		// Not a method but a property holding a function: read it, and
		// call what it holds.
		if t := g.typeOf(mem); t != nil {
			if _, isFunc := t.Underlying().(*types.Signature); isFunc {
				return g.applyValue(e, mem)
			}
		}
		g.unsupported(e)
		return nil
	}
	recv := g.typeOf(mem.X)
	if recv == nil {
		g.unsupported(e)
		return nil
	}
	// Static method call.
	if ref.Method.IsStatic {
		if v, ok := g.classMethodCall(e, mem, ref, recv); ok {
			return v
		}
		return g.staticCall(e, ref, recv)
	}
	// A method a protocol's extension adds is the extension's, lowered
	// for the receiver's type; see protocolExtensionMethod.
	if p, ok := ref.Recv.(*types.Protocol); ok && p.IsExtensionMethod(ref.Method) {
		if _, isEx := existentialOf(g.substituted(recv)); isEx {
			g.refuse(e, "a method of "+p.Name+"'s extension called on an existential")
			return nil
		}
		return g.methodCall(e, ref, func() *sil.Value {
			if mutatingRef(ref) {
				return g.lvalue(mem.X)
			}
			return g.expr(mem.X)
		})
	}
	// Witness table resolution for generic requirements.
	if resolved, ok := g.witness(ref, recv); ok {
		ref = resolved
	}
	// Existential dispatch.
	if ex, ok := existentialOf(recv); ok {
		return g.existentialCall(e, ref, mem, ex)
	}
	if mutatingRef(ref) && g.writesThroughElement(mem.X) {
		if g.lateReceivers == nil {
			g.lateReceivers = map[*ast.CallExpr]bool{}
		}
		g.lateReceivers[e] = true
	}
	// `super.m()`: the superclass's implementation, called directly.
	if _, isSuper := mem.X.(*ast.SuperExpr); isSuper {
		return g.superMethodCall(e, ref)
	}
	var copied *sil.Value
	v := g.methodCall(e, ref, func() *sil.Value {
		// Mutating methods receive receiver storage address.
		if mutatingRef(ref) {
			return g.lvalue(mem.X)
		}
		// Non-mutating receivers are borrowed as @guaranteed.
		copied = g.expr(mem.X)
		return copied
	})
	g.endReceiverTemp(copied)
	return v
}

// endReceiverTemp ends a receiver copied to call a method on it as soon as
// the call returns, where Swift ends that borrow, rather than at the end
// of the statement. Held longer, the copy keeps the variable's storage
// shared: in `write(above())` inside a mutating method, above's copy of
// self would still be alive while write mutates self's array, and every
// element store would copy the whole array.
func (g *gen) endReceiverTemp(v *sil.Value) {
	if v == nil || v.Ownership() != sil.Owned || g.blk == nil || g.blk.Term() != nil {
		return
	}
	if g.isLocalValue(v) {
		return
	}
	for _, s := range g.scopes {
		for i, c := range s.cleanups {
			if c.destroy != v {
				continue
			}
			if !s.formal {
				return
			}
			s.cleanups = append(s.cleanups[:i], s.cleanups[i+1:]...)
			if isExistentialType(v.Type().Formal()) {
				g.blk.DestroyAddr(v)
			} else {
				g.blk.DestroyValue(v)
			}
			return
		}
	}
}

// methodCall emits a method call over the given receiver.
func (g *gen) methodCall(e *ast.CallExpr, ref *analyzer.MethodRef, receiver func() *sil.Value) *sil.Value {
	// Dynamic dispatch for polymorphic classes via vtable.
	if cl, ok := receiverClass(ref.Recv); ok && g.poly[cl] {
		return g.dynamicCall(e, ref, cl, receiver)
	}
	var symbol string
	if spec, name, generic := g.genericMethod(e, ref); generic {
		if name == "" {
			return nil
		}
		ref, symbol = spec, name
	} else {
		symbol = g.methodSymbol(ref)
	}
	callee := g.m.Func(symbol).SetSourceName(ref.Method.Name)
	if g.needsType(callee) {
		g.declareMethod(callee, ref)
	}
	fnRef := g.blk.FunctionRef(callee)

	self, args, ok := g.methodArgs(e, ref.Method.Sig, receiver)
	if !ok {
		return nil
	}
	args = append(args, self)

	if ref.Method.Sig.Throws {
		optional, trap := g.tryOn(e)
		g.tryBang = trap
		return g.tryApply(e, fnRef, args, ref.Method.Sig.Results, optional, false)
	}
	result := lowerType(ref.Method.Sig.Results)
	v := g.blk.Apply(fnRef, result, args...)
	g.destroyLater(v)
	return v
}

// superMethodCall is `super.m(...)` in a method of a class: the
// implementation the superclass has -- its own, or the nearest one above
// it -- called on self directly, not through the table, which would find
// the override making the call.
func (g *gen) superMethodCall(e *ast.CallExpr, ref *analyzer.MethodRef) *sil.Value {
	cl, ok := receiverClass(g.recv)
	if !ok || cl.Superclass == nil || ref.Method.Sig == nil {
		g.refuse(e, "a super call outside a subclass's method")
		return nil
	}
	super, ok := cl.Superclass.Underlying().(*types.Class)
	if !ok {
		g.refuse(e, "a super call on a superclass this cannot read")
		return nil
	}
	key := ref.Method.Name + ref.Method.Sig.String()
	var impl *analyzer.MethodRef
	chain := classChain(super)
	for i := len(chain) - 1; i >= 0 && impl == nil; i-- {
		for _, m := range chain[i].Methods {
			if m != nil && m.Sig != nil && m.Name+m.Sig.String() == key {
				impl = &analyzer.MethodRef{Recv: chain[i], Method: m}
				break
			}
		}
	}
	if impl == nil {
		g.refuse(e, "a super call of a method no superclass implements")
		return nil
	}
	callee := g.m.Func(g.methodSymbol(impl)).SetSourceName(impl.Method.Name)
	if g.needsType(callee) {
		g.declareMethod(callee, impl)
	}
	implClass := impl.Recv.(*types.Class)
	self, args, ok := g.methodArgs(e, impl.Method.Sig, func() *sil.Value {
		return g.blk.Upcast(g.selfValue(), lowerType(implClass))
	})
	if !ok {
		return nil
	}
	args = append(args, self)
	fnRef := g.blk.FunctionRef(callee)
	if impl.Method.Sig.Throws {
		optional, trap := g.tryOn(e)
		g.tryBang = trap
		return g.tryApply(e, fnRef, args, impl.Method.Sig.Results, optional, false)
	}
	v := g.blk.Apply(fnRef, lowerType(impl.Method.Sig.Results), args...)
	g.destroyLater(v)
	return v
}

// dynamicCall dispatches through the receiver's class method table.
func (g *gen) dynamicCall(e *ast.CallExpr, ref *analyzer.MethodRef, cl *types.Class,
	receiver func() *sil.Value) *sil.Value {

	self, args, ok := g.methodArgs(e, ref.Method.Sig, receiver)
	if !ok {
		return nil
	}

	intro := introducer(cl, ref.Method)
	member := intro.Name + "." + ref.Method.Name
	method := g.blk.ClassMethod(self, member, methodType(ref, intro))

	args = append(args, self)
	if ref.Method.Sig.Throws {
		optional, trap := g.tryOn(e)
		g.tryBang = trap
		return g.tryApply(e, method, args, ref.Method.Sig.Results, optional, false)
	}
	result := lowerType(ref.Method.Sig.Results)
	v := g.blk.Apply(method, result, args...)
	g.destroyLater(v)
	return v
}

// methodArgs evaluates receiver then arguments, returning them with receiver
// placed last. A receiver that is an array element is evaluated after
// them, as Swift begins that access once the arguments are evaluated.
func (g *gen) methodArgs(e *ast.CallExpr, sig *types.Signature, receiver func() *sil.Value) (*sil.Value, []*sil.Value, bool) {
	if g.lateReceivers[e] {
		args, ok := g.arguments(e, sig)
		if !ok {
			return nil, nil, false
		}
		self := receiver()
		if self == nil {
			g.unsupported(e)
			return nil, nil, false
		}
		return self, args, true
	}
	self := receiver()
	if self == nil {
		g.unsupported(e)
		return nil, nil, false
	}
	args, ok := g.arguments(e, sig)
	if !ok {
		return nil, nil, false
	}
	return self, args, true
}

// methodType returns the function type for a class_method instruction.
func methodType(ref *analyzer.MethodRef, intro *types.Class) sil.Type {
	sig := ref.Method.Sig
	ft := &sil.FuncType{Convention: sil.Method}
	for _, p := range sig.Params {
		t := lowerType(p.Type)
		ft.Params = append(ft.Params, sil.Param{Type: t, Convention: paramConvention(p, t)})
	}
	self := lowerType(intro)
	ft.Params = append(ft.Params, sil.Param{Type: self, Convention: selfConvention(self)})
	if sig.Results != nil && !isVoid(sig.Results) {
		t := lowerType(sig.Results)
		ft.Results = append(ft.Results, sil.Result{Type: t, Convention: resultConvention(t)})
	}
	if sig.Throws {
		ft.ErrorType = sil.Object(sil.BuiltinNativeObj)
	}
	return sil.Object(ft)
}

// introducer is the base-most class declaring this method, which is
// the class the slot is named for.
func introducer(cl *types.Class, m *types.Method) *types.Class {
	if m == nil || m.Sig == nil {
		return cl
	}
	key := m.Name + m.Sig.String()
	for _, c := range classChain(cl) {
		for _, own := range c.Methods {
			if own != nil && own.Sig != nil && own.Name+own.Sig.String() == key {
				return c
			}
		}
	}
	return cl
}

// receiverClass returns the underlying class type if t is a class.
func receiverClass(t types.Type) (*types.Class, bool) {
	if t == nil {
		return nil, false
	}
	cl, ok := t.Underlying().(*types.Class)
	return cl, ok
}

// implicitMethod is the receiver's method a bare name refers to, for a
// call written inside one of its own methods.
func (g *gen) implicitMethod(id *ast.IdentExpr) (*analyzer.MethodRef, bool) {
	if id.Name == nil {
		return nil, false
	}
	// A static property's initializer names its type's static methods
	// without the type: `static let rounds = defaultRounds()`.
	if g.recv == nil && g.staticRecv != nil {
		prev := g.recv
		g.recv = g.staticRecv
		ref, ok := g.implicitMethod(id)
		g.recv = prev
		if ok && ref.Method.IsStatic {
			return ref, true
		}
		return nil, false
	}
	if g.recv == nil {
		return nil, false
	}
	// A name the checker resolved to something else -- a type, a local, a
	// function at package scope -- is that, whatever methods share it. A
	// receiver method called `Size` does not make `Size(1, 2)` inside it a
	// call of itself.
	switch g.info.Uses[id.Name].(type) {
	case nil, *analyzer.FuncSymbol:
	default:
		return nil, false
	}
	name := g.text(id.Name)
	var methods []*types.Method
	switch b := g.recv.Underlying().(type) {
	case *types.Struct:
		methods = b.Methods
	case *types.Class:
		methods = b.Methods
	case *types.Enum:
		methods = b.Methods
	default:
		// Inside an extension of a built-in type, the extensions' methods.
		if b := builtinOf(g.info, g.recv); b != nil {
			methods = b.Methods
			break
		}
		return nil, false
	}
	// Of methods sharing the name, the one the checker chose.
	if fn, ok := g.info.Uses[id.Name].(*analyzer.FuncSymbol); ok {
		for _, m := range methods {
			if m != nil && m.Name == name && m.Sig == fn.Signature() {
				return &analyzer.MethodRef{Recv: g.recv, Method: m}, true
			}
		}
	}
	for _, m := range methods {
		if m != nil && m.Name == name {
			return &analyzer.MethodRef{Recv: g.recv, Method: m}, true
		}
	}
	return nil, false
}

// methodSymbol returns the mangled symbol name for a method.
func (g *gen) methodSymbol(ref *analyzer.MethodRef) string {
	d := mangle.Decl{
		Module:    g.memberModule(ref.Recv, ref.Method),
		Name:      ref.Method.Name,
		Signature: ref.Method.Sig,
		Static:    ref.Method.IsStatic,
		ModuleOf:  g.moduleOfType,
		Extended:  extendedBuiltin(ref.Recv),
	}
	d.Context = memberChain(ref.Recv)
	name, err := mangle.Function(d)
	if err != nil {
		g.errorAt(nil, "cannot name '"+ref.Method.Name+"': "+err.Error())
		return ref.Method.Name
	}
	return name
}

// nominalChain returns enclosing nominal types, outermost first.
func nominalChain(t types.Type) []mangle.Nominal {
	nom, ok := nominalOf(t)
	if !ok {
		return nil
	}
	return append(nominalChain(enclosingType(t)), nom)
}

// nominalOf returns the mangling nominal type descriptor.
func nominalOf(t types.Type) (mangle.Nominal, bool) {
	if t == nil {
		return mangle.Nominal{}, false
	}
	name := typeName(t)
	if name == "" {
		return mangle.Nominal{}, false
	}
	switch t.Underlying().(type) {
	case *types.Struct:
		return mangle.Nominal{Name: name, Kind: mangle.Struct}, true
	case *types.Class:
		return mangle.Nominal{Name: name, Kind: mangle.Class}, true
	case *types.Enum:
		return mangle.Nominal{Name: name, Kind: mangle.Enum}, true
	}
	return mangle.Nominal{}, false
}

// declareMethod gives a method's callee its lowered type: the
// parameters the program wrote, then the receiver.
func (g *gen) declareMethod(f *sil.Func, ref *analyzer.MethodRef) {
	sig := ref.Method.Sig
	for _, p := range sig.Params {
		t := lowerType(p.BodyType())
		conv := paramConvention(p, t)
		if byAddress(conv) {
			t = t.Address()
		}
		f.Type().Params = append(f.Type().Params,
			sil.Param{Type: t, Convention: conv})
	}
	f.Type().Params = append(f.Type().Params, selfParam(ref))
	f.Type().Convention = sil.Method
	if sig.Results != nil && !isVoid(sig.Results) {
		t := lowerType(sig.Results)
		f.SetResult(t, resultConvention(t))
	}
	if sig.Throws {
		f.SetThrows(sil.Object(sil.BuiltinNativeObj))
	}
	// As for a function: async is in the type. See declareSignature.
	f.Type().Async = sig.Async
	f.Type().Isolated = sig.Isolated
}

// selfParam returns the self parameter descriptor for a method.
func selfParam(ref *analyzer.MethodRef) sil.Param {
	self := lowerType(ref.Recv)
	if mutatingRef(ref) {
		return sil.Param{Type: self.Address(), Convention: sil.ParamInout}
	}
	return sil.Param{Type: self, Convention: selfConvention(self)}
}

// mutatingRef reports whether the method is mutating on a value type.
func mutatingRef(ref *analyzer.MethodRef) bool {
	return ref != nil && ref.Method != nil && ref.Method.IsMutating && !isClass(ref.Recv)
}

// selfConvention returns the calling convention for the receiver parameter.
func selfConvention(t sil.Type) sil.ParamConvention {
	if t.Trivial() {
		return sil.ParamUnowned
	}
	return sil.ParamGuaranteed
}

// declare gives a callee its lowered type.
func (g *gen) declare(f *sil.Func, sym *analyzer.FuncSymbol) {
	g.declareSignature(f, sym.Signature())
	if module, ok := g.info.Imported[sym]; ok && g.info.SwiftModules[module] {
		f.SetAttr(sil.AttrSwift)
	}
}

// declareSignature sets parameters, results, and throws attributes on a function.
func (g *gen) declareSignature(f *sil.Func, sig *types.Signature) {
	for _, p := range sig.Params {
		t := lowerType(p.BodyType())
		conv := paramConvention(p, t)
		if byAddress(conv) {
			t = t.Address()
		}
		f.Type().Params = append(f.Type().Params,
			sil.Param{Type: t, Convention: conv})
	}
	if sig.Results != nil && !isVoid(sig.Results) {
		t := lowerType(sig.Results)
		f.SetResult(t, resultConvention(t))
	}
	if sig.Throws {
		f.SetThrows(sil.Object(sil.BuiltinNativeObj))
	}
	// Async is part of the type and not of the declaration, because the
	// calling convention differs: an async function is handed a context
	// and answers through the continuation that context names. A
	// declaration that did not say so would be called as an ordinary
	// function -- which links, and is wrong. See lower/async.go.
	f.Type().Async = sig.Async
	f.Type().Isolated = sig.Isolated
}

// binary lowers an operator expression.
func (g *gen) binary(e *ast.BinaryExpr) *sil.Value {
	// A generic operator is the call of it the checker made.
	if call := g.info.OperatorCalls[e]; call != nil {
		return g.expr(call)
	}
	if v, isArray := g.arrayConcat(e); isArray {
		return v
	}
	if v, isRange := g.rangeFormation(e); isRange {
		return v
	}
	// An optional compared: the operator recorded is the payloads'.
	if _, ok := g.info.OptionalCompares[e]; ok {
		if v, isOptional := g.optionalComparison(e, g.text(e.Op)); isOptional {
			return v
		}
	}
	sym, _ := g.info.Operators[e].(*analyzer.FuncSymbol)
	if ref := g.info.OperatorMethods[e]; ref != nil || (sym != nil && !g.coreOperator(sym)) {
		return g.operatorCall(e, ref, sym, e.X, e.Y)
	}
	if d := g.info.DerivedOperators[e]; d != nil {
		return g.derivedOperator(e, d)
	}
	if sym == nil {
		if v, handled := g.identity(e, g.text(e.Op)); handled {
			return v
		}
		if v, handled := g.enumEquality(e, g.text(e.Op)); handled {
			return v
		}
		if g.text(e.Op) == "??" {
			return g.nilCoalescing(e)
		}
		if v, isPointer := g.pointerEquality(e, g.text(e.Op)); isPointer {
			return v
		}
		if v, isOffset := g.pointerOffset(e); isOffset {
			return v
		}
		if v, isOptional := g.optionalComparison(e, g.text(e.Op)); isOptional {
			return v
		}
		if v, isArray := g.arrayEquality(e, g.text(e.Op)); isArray {
			return v
		}
		g.expr(e.X)
		g.expr(e.Y)
		g.unsupported(e)
		return nil
	}
	op := sym.Name()

	if op == "&&" || op == "||" {
		return g.shortCircuit(e, op == "&&")
	}

	operand := g.typeOf(e.X)
	lhs, rhs := g.expr(e.X), g.expr(e.Y)
	if lhs == nil || rhs == nil {
		return nil
	}
	v := g.operate(e, op, operand, sym.Signature().Results, lhs, rhs)
	if v == nil {
		g.unsupported(e)
	}
	return v
}

// operate applies an operator to two lowered values using core or runtime definitions.
func (g *gen) operate(at ast.Node, op string, operand, results types.Type, lhs, rhs *sil.Value) *sil.Value {
	// String and runtime extern operators.
	if ex, ok := core.LowerExtern(op, operand); ok {
		return g.externOperator(at, op, operand, results, ex, lhs, rhs)
	}
	// Shift operators with mask handling.
	if isShift(op) {
		return g.shift(at, op, operand, results, lhs, rhs)
	}
	bi, ok := core.Lower(op, operand)
	if !ok {
		return nil
	}

	a, b := g.machine(lhs, operand), g.machine(rhs, operand)
	result := lowerType(results)

	if !bi.Overflows {
		raw := g.blk.Builtin(bi.Name, sil.Object(builtinNamed(bi.Result)), a, b)
		return g.blk.Struct(result, raw)
	}

	// -1 reports overflow, 0 wraps.
	report := int64(-1)
	if core.Wrapping(op) {
		report = 0
	}
	want := g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), report)
	pair := sil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: builtinNamed(bi.Result)},
		{Type: sil.BuiltinInt1},
	}})
	both := g.blk.Builtin(bi.Name, pair, a, b, want)
	value := g.blk.TupleExtract(both, 0, sil.Object(builtinNamed(bi.Result)))
	if core.Wrapping(op) {
		return g.blk.Struct(result, value)
	}
	flag := g.blk.TupleExtract(both, 1, sil.Object(sil.BuiltinInt1))
	g.blk.CondFail(flag, "arithmetic overflow")
	return g.blk.Struct(result, value)
}

// conditional lowers ternary expressions (a ? b : c).
func (g *gen) conditional(e *ast.ConditionalExpr) *sil.Value {
	cond := g.expr(e.Cond)
	if cond == nil {
		return nil
	}
	bit := g.machine(cond, g.typeOf(e.Cond))
	if want := g.typeOf(e); want != nil {
		if _, isEx := existentialOf(want); isEx {
			then, els := g.typeOf(e.Then), g.typeOf(e.Else)
			if _, thenEx := existentialOf(then); !thenEx && then != nil && els != nil && types.Identical(then, els) {
				return g.existentialFor(e, g.conditionalAs(e, bit, then), then, want)
			}
		}
	}
	return g.conditionalAs(e, bit, g.typeOf(e))
}

// conditionalAs lowers ternary branches joining as the given type.
func (g *gen) conditionalAs(e *ast.ConditionalExpr, bit *sil.Value, t types.Type) *sil.Value {
	result := lowerType(t)

	thenBlk := g.fn.Block()
	elseBlk := g.fn.Block()
	join := g.fn.Block()
	answer := join.Arg(result, joinOwnership(result))

	g.blk.CondBr(bit, thenBlk, nil, elseBlk, nil)

	g.blk = thenBlk
	yes := g.branchArm(func() *sil.Value {
		v := g.rvalue(e.Then)
		return g.optionalFor(e.Then, v, g.typeOf(e.Then), t)
	})
	if yes == nil {
		return nil
	}
	g.blk.Br(join, yes)

	g.blk = elseBlk
	no := g.branchArm(func() *sil.Value {
		v := g.rvalue(e.Else)
		return g.optionalFor(e.Else, v, g.typeOf(e.Else), t)
	})
	if no == nil {
		return nil
	}
	g.blk.Br(join, no)

	g.blk = join
	g.destroyLater(answer)
	return answer
}

// joinOwnership returns the ownership convention for a branch join argument.
func joinOwnership(t sil.Type) sil.Ownership {
	if t.Trivial() {
		return sil.None
	}
	return sil.Owned
}

// branchArm evaluates an expression within a branch-local scope.
func (g *gen) branchArm(eval func() *sil.Value) *sil.Value {
	g.push()
	v := eval()
	if v == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return nil
	}
	if v.Ownership() == sil.Owned || v.Ownership() == sil.Guaranteed {
		v = g.consume(v)
	}
	g.pop()
	return v
}

// shortCircuit lowers && and || as conditional branches.
func (g *gen) shortCircuit(e *ast.BinaryExpr, isAnd bool) *sil.Value {
	lhs := g.expr(e.X)
	if lhs == nil {
		return nil
	}
	bit := g.machine(lhs, g.typeOf(e.X))
	result := lowerType(g.typeOf(e))

	rhsBlk := g.fn.Block()
	decided := g.fn.Block()
	join := g.fn.Block()
	answer := join.Arg(result, joinOwnership(result))

	if isAnd {
		g.blk.CondBr(bit, rhsBlk, nil, decided, nil)
	} else {
		g.blk.CondBr(bit, decided, nil, rhsBlk, nil)
	}

	g.blk = rhsBlk
	rhs := g.branchArm(func() *sil.Value { return g.expr(e.Y) })
	if rhs == nil {
		return nil
	}
	g.blk.Br(join, rhs)

	g.blk = decided
	n := int64(0)
	if !isAnd {
		n = 1
	}
	raw := g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), n)
	g.blk.Br(join, g.blk.Struct(result, raw))

	g.blk = join
	return answer
}

// machine reaches through a primitive's struct to the word inside it,
// which is what the builtin operates on.
func (g *gen) machine(v *sil.Value, t types.Type) *sil.Value {
	field, name, ok := core.Layout(t)
	if !ok {
		return v
	}
	return g.blk.StructExtract(v, typeName(t)+"."+field,
		sil.Object(builtinNamed(name)))
}

// builtinNamed is the builtin type core named.
func builtinNamed(name string) types.Type {
	switch name {
	case "Int1":
		return sil.BuiltinInt1
	case "Int8":
		return sil.BuiltinInt8
	case "Int16":
		return sil.BuiltinInt16
	case "Int32":
		return sil.BuiltinInt32
	case "FPIEEE32":
		return sil.BuiltinFPIEEE32
	case "FPIEEE64":
		return sil.BuiltinFPIEEE64
	}
	return sil.BuiltinInt64
}

// selfValue is the receiver, which is the last parameter of a method.
func (g *gen) selfValue() *sil.Value {
	if g.fn == nil {
		return nil
	}
	// A convenience initializer's self is what its self.init made.
	if g.convenience {
		return g.convSelf
	}
	args := g.fn.Entry().Args()
	if len(args) == 0 {
		return nil
	}
	return args[len(args)-1]
}

// loadQualifier says what a load does to ownership.
func loadQualifier(t sil.Type) string {
	if t.Trivial() {
		return "trivial"
	}
	return "copy"
}

// memberName returns the qualified name: Type.member.
func memberName(base types.Type, name string) string {
	if base == nil {
		return name
	}
	// A labelled tuple's element is named by its label, however it is
	// written: r.0 of a (min: Int, max: Int) is its min.
	if tu, ok := base.Underlying().(*types.Tuple); ok {
		if i, err := strconv.Atoi(name); err == nil && i >= 0 && i < len(tu.Elements) && tu.Elements[i].Name != "" {
			name = tu.Elements[i].Name
		}
	}
	return typeName(base) + "." + name
}

func typeName(t types.Type) string {
	if named, ok := t.(*types.Named); ok {
		return named.Name
	}
	return t.String()
}

func isClass(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Class)
	return ok
}

// staticCall lowers a call to a static method of a type: `Vec.zero()`.
func (g *gen) staticCall(e *ast.CallExpr, ref *analyzer.MethodRef, recv types.Type) *sil.Value {
	if meta, ok := recv.(*types.Metatype); ok {
		recv = meta.Instance
	}
	if symbol, ok := g.coreTask(recv, ref.Method.Name); ok {
		// Task.detached makes a Task as Task's initializer does.
		if ref.Method.Name == "detached" {
			return g.startTask(e, g.typeOf(e), symbol, ref.Method.Sig)
		}
		return g.taskCall(e, symbol, ref.Method.Sig, nil)
	}
	if symbol, ok := g.coreMainActor(recv, ref.Method.Name); ok {
		return g.mainActorCall(e, symbol, ref.Method.Sig)
	}
	// `T.make()` in a specialization is the concrete type's own.
	if resolved, ok := g.witness(ref, recv); ok {
		ref = resolved
	}
	// A static method of a generic type, specialized for the instance.
	var symbol string
	if spec, name, generic := g.genericMethod(e, ref); generic {
		if name == "" {
			return nil
		}
		ref, symbol = spec, name
	} else {
		// A class's static method a superclass declares is the
		// superclass's function.
		owner := recv
		if _, isClass := receiverClass(recv); isClass {
			if _, declared := receiverClass(ref.Recv); declared {
				owner = ref.Recv
			}
		}
		symbol = g.staticSymbol(ref, owner)
	}
	callee := g.m.Func(symbol).SetSourceName(ref.Method.Name)
	if g.needsType(callee) {
		g.declareSignature(callee, ref.Method.Sig)
	}
	fnRef := g.blk.FunctionRef(callee)

	// Borrowed where they are, as a function's arguments are, with a
	// default for anything left out. Taking each one left a copy that
	// nothing let go.
	args, ok := g.arguments(e, ref.Method.Sig)
	if !ok {
		return nil
	}
	// One that may fail goes out on both edges, as a method that may does.
	if ref.Method.Sig.Throws {
		optional, trap := g.tryOn(e)
		g.tryBang = trap
		return g.tryApply(e, fnRef, args, ref.Method.Sig.Results, optional, false)
	}
	v := g.blk.Apply(fnRef, lowerType(ref.Method.Sig.Results), args...)
	g.destroyLater(v)
	return v
}

// classMethodCall is a class method of a class with subclasses called
// through a metatype that may be a subclass's -- `type(of: x).kind()`, or
// `Self.kind()` in an instance method -- which the metatype's table
// dispatches. One called through the class's name is its own, and false.
func (g *gen) classMethodCall(e *ast.CallExpr, mem *ast.MemberExpr, ref *analyzer.MethodRef, recv types.Type) (*sil.Value, bool) {
	meta, ok := recv.(*types.Metatype)
	if !ok {
		return nil, false
	}
	cl, ok := receiverClass(meta.Instance)
	if !ok || !g.poly[cl] {
		return nil, false
	}
	var self *sil.Value
	if id, ok := mem.X.(*ast.IdentExpr); ok && id.Name != nil {
		if g.text(id.Name) != "Self" || g.recv == nil || g.selfValue() == nil || !isClass(g.recv) {
			return nil, false
		}
		// Self in an instance method is the instance's dynamic type.
		self = g.blk.Builtin("vertexObjectType_NativeObject", lowerType(meta), g.selfValue())
	} else {
		self = g.expr(mem.X)
	}
	if self == nil {
		return nil, true
	}
	intro := introducer(cl, ref.Method)
	sig := ref.Method.Sig
	ft := &sil.FuncType{Convention: sil.Thin}
	for _, p := range sig.Params {
		t := lowerType(p.BodyType())
		conv := paramConvention(p, t)
		if byAddress(conv) {
			t = t.Address()
		}
		ft.Params = append(ft.Params, sil.Param{Type: t, Convention: conv})
	}
	if sig.Results != nil && !isVoid(sig.Results) {
		t := lowerType(sig.Results)
		ft.Results = append(ft.Results, sil.Result{Type: t, Convention: resultConvention(t)})
	}
	if sig.Throws {
		ft.ErrorType = sil.Object(sil.BuiltinNativeObj)
	}
	method := g.blk.ClassMethod(self, intro.Name+"."+ref.Method.Name, sil.Object(ft))
	args, ok := g.arguments(e, sig)
	if !ok {
		return nil, true
	}
	if sig.Throws {
		optional, trap := g.tryOn(e)
		g.tryBang = trap
		return g.tryApply(e, method, args, sig.Results, optional, false), true
	}
	v := g.blk.Apply(method, lowerType(sig.Results), args...)
	g.destroyLater(v)
	return v, true
}

// staticSymbol names a static method of the type (mangled with Z suffix).
func (g *gen) staticSymbol(ref *analyzer.MethodRef, recv types.Type) string {
	d := mangle.Decl{
		Module:    g.memberModule(recv, ref.Method),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      ref.Method.Name,
		Signature: ref.Method.Sig,
		Static:    true,
		ModuleOf:  g.moduleOfType,
	}
	name, err := mangle.Function(d)
	if err != nil {
		g.errorAt(nil, "cannot name '"+ref.Method.Name+"': "+err.Error())
		return ref.Method.Name
	}
	return name
}

// coreOperator reports whether an operator is one of core's, which lower
// to builtins rather than to a call.
func (g *gen) coreOperator(sym *analyzer.FuncSymbol) bool {
	// One the core writes in source -- == on two metatypes -- is a
	// function like any other, called as a program's own operator is.
	return g.info.Imported[sym] == "Swift" && !hasBody(sym)
}

// operatorCall lowers an operator that means a function of the program's
// own -- a static method of a type, ref, or a function declared at the top
// of a module, sym -- as the call it is, its operands the arguments.
func (g *gen) operatorCall(at ast.Expr, ref *analyzer.MethodRef, sym *analyzer.FuncSymbol, operands ...ast.Expr) *sil.Value {
	vals := make([]*sil.Value, len(operands))
	for i, x := range operands {
		if vals[i] = g.expr(x); vals[i] == nil {
			return nil
		}
	}
	return g.operatorApply(at, ref, sym, operands, vals)
}

// derivedOperator lowers `a != b` as `!(a == b)`, and `>`, `<=` and `>=`
// through `<`. The operands are evaluated in the order they are written,
// whichever order they are passed in.
func (g *gen) derivedOperator(e *ast.BinaryExpr, d *analyzer.DerivedOperator) *sil.Value {
	lhs, rhs := g.expr(e.X), g.expr(e.Y)
	if lhs == nil || rhs == nil {
		return nil
	}
	xs, vals := []ast.Expr{e.X, e.Y}, []*sil.Value{lhs, rhs}
	if d.Swap {
		xs, vals = []ast.Expr{e.Y, e.X}, []*sil.Value{rhs, lhs}
	}
	// One of the core's comparisons, which are instructions or runtime
	// calls rather than functions with bodies: `a >= b` on Strings is
	// `!(a < b)` compared in place.
	if d.Method == nil && d.Fn != nil && g.coreOperator(d.Fn) && !hasBody(d.Fn) && len(d.Fn.Signature().Params) == 2 {
		if bit := g.compare(d.Fn.Name(), vals[0], vals[1], d.Fn.Signature().Params[0].Type); bit != nil {
			v := g.blk.Struct(lowerType(types.Typ[types.Bool]), bit)
			if !d.Negate {
				return v
			}
			return g.notBool(e, v)
		}
	}
	v := g.operatorApply(e, d.Method, d.Fn, xs, vals)
	if v == nil || !d.Negate {
		return v
	}
	return g.notBool(e, v)
}

// operatorApply calls an operator's function with operand values already
// evaluated; xs are the expressions they came from, in the same order.
func (g *gen) operatorApply(at ast.Expr, ref *analyzer.MethodRef, sym *analyzer.FuncSymbol, xs []ast.Expr, vals []*sil.Value) *sil.Value {
	if ref != nil {
		if _, abstract := ref.Recv.(*types.Protocol); abstract {
			return g.requirementOperator(at, ref, xs, vals)
		}
	}
	var symbol, name string
	var sig *types.Signature
	if ref != nil {
		sig, name = ref.Method.Sig, ref.Method.Name
		symbol = g.staticSymbol(ref, ref.Recv)
	} else {
		sig, name = sym.Signature(), sym.Name()
		symbol = g.symbol(sym)
	}
	if len(sig.TypeParams) > 0 || sig.Async || sig.Throws {
		g.refuse(at, "an operator that is generic, async or throws")
		return nil
	}
	callee := g.m.Func(symbol).SetSourceName(name)
	if g.needsType(callee) {
		g.declareSignature(callee, sig)
	}
	if sym != nil {
		g.emitCoreBody(sym, callee)
	}
	fnRef := g.blk.FunctionRef(callee)
	want := existentialParams(sig)
	args := make([]*sil.Value, len(vals))
	for i, v := range vals {
		if i < len(sig.Params) {
			v = g.optionalFor(xs[i], v, g.typeOf(xs[i]), sig.Params[i].Type)
		}
		args[i] = g.boxArg(xs[i], v, want, i)
	}
	v := g.blk.Apply(fnRef, lowerType(sig.Results), args...)
	g.destroyLater(v)
	return v
}

// arrayEquality lowers `==` and `!=` on two arrays through the runtime,
// which compares elements of the types it hashes. It reports false where
// the operands are not arrays.
func (g *gen) arrayEquality(e *ast.BinaryExpr, op string) (*sil.Value, bool) {
	if op != "==" && op != "!=" {
		return nil, false
	}
	a, ok := g.typeOf(e.X).Underlying().(*types.Array)
	if !ok {
		return nil, false
	}
	m, ok := core.ArrayEqual(a)
	if !ok {
		g.refuse(e, "comparing arrays of '"+a.Elem.String()+"', whose elements the runtime does not compare yet")
		return nil, true
	}
	v := g.collectionCall(e, m, e.X, exprArgs(e.Y))
	if v == nil || op == "==" {
		return v, true
	}
	return g.notBool(e, v), true
}

// notBool is a Bool inverted.
func (g *gen) notBool(at ast.Node, v *sil.Value) *sil.Value {
	boolT := types.Typ[types.Bool]
	steps, ok := core.LowerPrefix("!", boolT)
	if !ok {
		g.refuse(at, "a Bool this compiler cannot invert")
		return nil
	}
	flags := make(map[int64]*sil.Value, 1)
	cur := g.machine(v, boolT)
	for _, st := range steps {
		if cur = g.step(st, cur, flags); cur == nil {
			return nil
		}
	}
	return g.blk.Struct(lowerType(boolT), cur)
}

// emptyCollection lowers `Set<T>()` and its like as the empty literal of
// the type, which is what each is.
func (g *gen) emptyCollection(e *ast.CallExpr, t types.Type) *sil.Value {
	if _, isDict := t.Underlying().(*types.Dictionary); isDict {
		lit := &ast.DictLit{Span: e.Span}
		g.info.Types[lit] = t
		return g.dictionaryLiteral(lit)
	}
	lit := &ast.ArrayLit{Span: e.Span}
	g.info.Types[lit] = t
	return g.arrayLiteral(lit)
}

// basicInit lowers an initializer of a core type that the runtime answers:
// `Int("42")`, an optional read from text, and String(decoding:as:).
func (g *gen) basicInit(e *ast.CallExpr, t types.Type) (*sil.Value, bool) {
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	if o, ok := optionalOf(t); ok && len(args) == 1 && args[0].Label == nil && isString(g.typeOf(args[0].X)) {
		b, ok := o.Wrapped.Underlying().(*types.Basic)
		if !ok {
			return nil, false
		}
		symbol := stdlib.IntegerParse
		if b.Info()&types.IsFloat != 0 {
			symbol = stdlib.FloatParse
		}
		m := core.Method{Symbol: symbol, Params: []*types.Param{{Type: types.Typ[types.String]}}, Result: t,
			ResultOut: true, Operands: []core.Operand{{Kind: core.OpArgValue, Arg: 0}, {Kind: core.OpOut},
				{Kind: core.OpMeta, Type: t}}}
		return g.collectionCall(e, m, nil, exprArgs(args[0].X)), true
	}
	if isString(t) && len(args) == 2 && args[0].Label != nil && g.text(args[0].Label) == "decoding" {
		bytes := &types.Array{Elem: types.Typ[types.UInt8]}
		m := core.Method{Symbol: stdlib.StringDecodingUTF8, Params: []*types.Param{{Type: bytes}},
			Result: t, Operands: []core.Operand{{Kind: core.OpArgValue, Arg: 0}}}
		return g.collectionCall(e, m, nil, exprArgs(args[0].X)), true
	}
	return nil, false
}

// memberModule is the module a member of recv is declared in: the type's,
// or for a member an extension gives a built-in type, the extension's.
func (g *gen) memberModule(recv types.Type, member any) string {
	if meta, ok := recv.(*types.Metatype); ok {
		recv = meta.Instance
	}
	key := analyzer.BuiltinKey(recv)
	if key == "" {
		return g.moduleOfType(recv)
	}
	if b := g.info.Builtins[key]; b != nil && member != nil {
		if m := b.Modules[member]; m != "" {
			return m
		}
	}
	return g.module
}

// memberChain is the nominal types a member of recv is nested in, which
// for a built-in type is none: it is named as extended instead.
func memberChain(recv types.Type) []mangle.Nominal {
	if extendedBuiltin(recv) != nil {
		return nil
	}
	return nominalChain(recv)
}

// extendedBuiltin is recv, where a member of it is an extension's of a
// built-in type, and nil otherwise.
func extendedBuiltin(recv types.Type) types.Type {
	if meta, ok := recv.(*types.Metatype); ok {
		recv = meta.Instance
	}
	if analyzer.BuiltinKey(recv) == "" {
		return nil
	}
	return recv
}

// metatypeValue is t's metatype as a value: the metadata of t, which is
// what every metatype value is here.
func (g *gen) metatypeValue(at ast.Node, t types.Type) *sil.Value {
	t = g.substituted(t)
	v, ok := g.stdlibMetadata(at, t)
	if !ok {
		return nil
	}
	v.SetType(lowerType(&types.Metatype{Instance: t}))
	return v
}

// typeOfValue lowers `type(of: x)`: for a class instance its dynamic type,
// which its header says; for anything else the type it statically is. x
// is evaluated either way.
func (g *gen) typeOfValue(e *ast.CallExpr, static types.Type) *sil.Value {
	arg := e.Args.Args[0].X
	static = g.substituted(static)
	meta := lowerType(&types.Metatype{Instance: static})
	// An existential says what it holds, which the runtime reads.
	if isExistentialType(static) {
		addr := g.existentialPlace(arg)
		if addr == nil {
			return nil
		}
		f := g.m.Func(stdlib.ExistentialType).SetSourceName(stdlib.ExistentialType)
		if g.needsType(f) {
			f.SetLinkage(sil.PublicExternal)
			f.Type().Convention = sil.Thin
			f.Type().Params = []sil.Param{{Type: rawPointerType(), Convention: sil.ParamUnowned}}
			f.SetResult(meta, sil.ResultUnowned)
		}
		return g.blk.Apply(g.blk.FunctionRef(f), meta, g.blk.AddressToPointer(addr, rawPointerType()))
	}
	if isClass(static) {
		obj := g.rvalue(arg)
		if obj == nil {
			return nil
		}
		g.destroyTemp(obj)
		return g.blk.Builtin("vertexObjectType_NativeObject", meta, obj)
	}
	v := g.rvalue(arg)
	if v == nil {
		return nil
	}
	g.destroyTemp(v)
	return g.metatypeValue(e, static)
}

// identity lowers `===` and `!==`: whether two references are to the one
// object. Each side is a class reference, an optional one -- nil is the
// null pointer -- or an AnyObject, whose container holds the reference
// in its first word.
func (g *gen) identity(e *ast.BinaryExpr, op string) (*sil.Value, bool) {
	if op != "===" && op != "!==" {
		return nil, false
	}
	a, b := g.objectPointer(e.X), g.objectPointer(e.Y)
	if a == nil || b == nil {
		return nil, true
	}
	boolT := types.Typ[types.Bool]
	verb := "cmp_eq_RawPointer"
	if op == "!==" {
		verb = "cmp_ne_RawPointer"
	}
	bit := g.blk.Builtin(verb, sil.Object(sil.BuiltinInt1), a, b)
	return g.blk.Struct(lowerType(boolT), bit), true
}

// objectPointer is the address of the object a reference expression refers
// to, as a raw pointer: null for a nil optional.
func (g *gen) objectPointer(x ast.Expr) *sil.Value {
	t := g.typeOf(x)
	if o, ok := optionalOf(t); ok {
		t = o.Wrapped
	}
	if isExistentialType(t) {
		addr := g.existentialPlace(x)
		if addr == nil {
			return nil
		}
		word := g.blk.PointerToAddress(g.blk.AddressToPointer(addr, rawPointerType()), rawPointerType().Address())
		return g.blk.Load(word, loadQualifier(rawPointerType()))
	}
	v := g.rvalue(x)
	if v == nil {
		return nil
	}
	g.destroyTemp(v)
	return v
}
