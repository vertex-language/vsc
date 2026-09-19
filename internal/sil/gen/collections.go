package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// Collections handles lowering methods, properties, subscripts, and literals
// for runtime-implemented Array, Dictionary, and Set types.

// A collArg is one argument of a collection call: an expression still to
// lower, or a value already lowered and owned.
type collArg struct {
	expr  ast.Expr
	value *sil.Value
}

func exprArgs(es ...ast.Expr) []collArg {
	out := make([]collArg, len(es))
	for i, e := range es {
		out[i] = collArg{expr: e}
	}
	return out
}

// collectionCall lowers one runtime collection call over recv.
func (g *gen) collectionCall(at ast.Node, m core.Method, recv ast.Expr, args []collArg) *sil.Value {
	var operands []*sil.Value
	var params []sil.Param
	var after []func()
	var access, out *sil.Value
	raw := sil.Object(sil.BuiltinRawPointer)

	// argValue is an argument lowered as an owned value of the parameter's
	// type.
	argValue := func(i int) (*sil.Value, types.Type) {
		pt := m.Params[i].Type
		a := args[i]
		if a.value != nil {
			return a.value, pt
		}
		v := g.rvalue(a.expr)
		if v == nil {
			return nil, pt
		}
		from := g.typeOf(a.expr)
		if _, isEx := existentialOf(pt); isEx {
			return g.existentialFor(a.expr, v, from, pt), pt
		}
		// An optional existential with a value is the existential's own
		// memory: its metadata word is what says it is not empty.
		if o, ok := optionalOf(pt); ok {
			if _, isEx := existentialOf(o.Wrapped); isEx {
				if _, fromOpt := optionalOf(from); !fromOpt {
					return g.existentialFor(a.expr, v, from, o.Wrapped), pt
				}
			}
		}
		return g.optionalFor(a.expr, v, from, pt), pt
	}

	// A receiver that is an array element -- `rows[i].append(x)`, `a[i][j]
	// = v` -- is reached after the arguments are evaluated. See elementAddr.
	if receivesSlot(m) && g.writesThroughElement(recv) {
		args = append([]collArg(nil), args...)
		for _, op := range m.Operands {
			if args == nil || op.Arg >= len(args) || args[op.Arg].value != nil {
				continue
			}
			switch op.Kind {
			case core.OpArgTake, core.OpArgBorrow:
				v, _ := argValue(op.Arg)
				if v == nil {
					return nil
				}
				args[op.Arg].value = v
			case core.OpArgValue:
				v := g.expr(args[op.Arg].expr)
				if v == nil {
					return nil
				}
				args[op.Arg].value = v
			}
		}
	}

	for _, op := range m.Operands {
		switch op.Kind {
		case core.OpReceiverSlot:
			addr := g.lvalue(recv)
			if addr == nil {
				g.refuse(at, "a mutating call on "+g.exprKind(recv)+", which is not a variable")
				return nil
			}
			access = g.blk.BeginAccess(addr, "modify", "unknown")
			operands = append(operands, access)
			params = append(params, sil.Param{Type: lowerType(g.typeOf(recv)).Address(), Convention: sil.ParamInout})
		case core.OpReceiver:
			v := g.expr(recv)
			if v == nil {
				return nil
			}
			operands = append(operands, v)
			params = append(params, sil.Param{Type: lowerType(g.typeOf(recv)), Convention: sil.ParamGuaranteed})
		case core.OpReceiverAddress:
			v := g.rvalue(recv)
			if v == nil {
				return nil
			}
			lt := lowerType(g.typeOf(recv))
			slot := g.blk.AllocStack(lt)
			g.blk.Store(v, slot, storeQualifier(lt))
			if !lt.Trivial() {
				after = append(after, func() { g.destroyLater(g.blk.Load(slot, "take")) })
			}
			operands = append(operands, slot)
			params = append(params, sil.Param{Type: lt.Address(), Convention: sil.ParamInGuaranteed})
		case core.OpArgTake, core.OpArgBorrow:
			v, pt := argValue(op.Arg)
			if v == nil {
				return nil
			}
			lt := lowerType(pt)
			// A value put in an existential is in memory already.
			slot := v
			if !v.Type().IsAddress() {
				slot = g.blk.AllocStack(lt)
				g.blk.Store(v, slot, storeQualifier(lt))
			}
			operands = append(operands, slot)
			conv := sil.ParamIn
			if op.Kind == core.OpArgBorrow {
				conv = sil.ParamInGuaranteed
				if !lt.Trivial() {
					after = append(after, func() { g.destroyLater(g.blk.Load(slot, "take")) })
				}
			}
			params = append(params, sil.Param{Type: lt.Address(), Convention: conv})
		case core.OpArgValue:
			pt := m.Params[op.Arg].Type
			var v *sil.Value
			if args[op.Arg].value != nil {
				v = args[op.Arg].value
			} else {
				v = g.expr(args[op.Arg].expr)
			}
			if v == nil {
				return nil
			}
			lt := lowerType(pt)
			conv := sil.ParamGuaranteed
			if lt.Trivial() {
				conv = sil.ParamUnowned
			}
			operands = append(operands, v)
			params = append(params, sil.Param{Type: lt, Convention: conv})
		case core.OpOut:
			rt := lowerType(m.Result)
			out = g.blk.AllocStack(rt)
			operands = append(operands, out)
			params = append(params, sil.Param{Type: rt.Address(), Convention: sil.ParamInout})
		case core.OpMeta:
			meta, ok := g.stdlibMetadata(at, op.Type)
			if !ok {
				return nil
			}
			operands = append(operands, meta)
			params = append(params, sil.Param{Type: raw})
		}
	}

	direct := !m.ResultOut && !isVoid(m.Result)
	callee := g.m.Func(m.Symbol).SetSourceName(m.Symbol)
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		ft := callee.Type()
		ft.Convention = sil.Thin
		ft.Params = params
		if direct {
			t := lowerType(m.Result)
			callee.SetResult(t, resultConvention(t))
		}
	}
	resultType := lowerType(types.Typ[types.Void])
	if direct {
		resultType = lowerType(m.Result)
	}
	answer := g.blk.Apply(g.blk.FunctionRef(callee), resultType, operands...)
	if access != nil {
		g.blk.EndAccess(access)
	}
	for _, f := range after {
		f()
	}
	switch {
	case m.ResultOut && isOptionalExistential(m.Result):
		// An optional existential stays in memory, as an existential does:
		// the use takes it from there.
		return out
	case m.ResultOut:
		rt := lowerType(m.Result)
		qual := "take"
		if rt.Trivial() {
			qual = "trivial"
		}
		v := g.blk.Load(out, qual)
		g.destroyLater(v)
		return v
	case direct:
		g.destroyLater(answer)
		return answer
	}
	return g.void()
}

// receivesSlot reports whether a runtime method takes its receiver's storage.
func receivesSlot(m core.Method) bool {
	for _, op := range m.Operands {
		if op.Kind == core.OpReceiverSlot {
			return true
		}
	}
	return false
}

// collectionMethodCall is a call through a dot that names a collection
// method the runtime implements, or nil and false where it is not one.
func (g *gen) collectionMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (*sil.Value, bool) {
	if mem.Name == nil {
		return nil, false
	}
	// One the checker resolved to a method an extension declares --
	// `contains(where:)` beside the runtime's `contains(_:)` -- is that.
	if g.info.Methods[mem] != nil {
		return nil, false
	}
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	labels := make([]string, len(args))
	exprs := make([]ast.Expr, len(args))
	for i, a := range args {
		if a.Label != nil {
			labels[i] = g.text(a.Label)
		}
		exprs[i] = a.X
	}
	m, ok := core.LowerCollectionMethod(g.typeOf(mem.X), g.text(mem.Name), labels)
	if !ok {
		return nil, false
	}
	// `bytes.append(contentsOf: s.utf8)`: the string's bytes go straight
	// onto the array, as they do in Swift, where utf8 is a view and not
	// an array made for the call.
	if m.Symbol == stdlib.ArrayAppendContents && len(exprs) == 1 {
		if s, ok := g.utf8Of(exprs[0]); ok {
			if arr, isArr := g.typeOf(mem.X).Underlying().(*types.Array); isArr && isUInt8(arr.Elem) {
				direct := core.Method{Symbol: stdlib.ArrayAppendUTF8,
					Params: []*types.Param{{Name: "contentsOf", Label: "contentsOf", Type: g.typeOf(s)}}, Result: types.Typ[types.Void],
					Mutating: true,
					Operands: []core.Operand{{Kind: core.OpReceiverSlot}, {Kind: core.OpArgValue, Arg: 0}}}
				return g.collectionCall(e, direct, mem.X, exprArgs(s)), true
			}
		}
	}
	return g.collectionCall(e, m, mem.X, exprArgs(exprs...)), true
}

// utf8Of is the string x reads the bytes of, where x is `s.utf8`.
func (g *gen) utf8Of(x ast.Expr) (ast.Expr, bool) {
	for {
		p, ok := x.(*ast.ParenExpr)
		if !ok {
			break
		}
		x = p.X
	}
	mem, ok := x.(*ast.MemberExpr)
	if !ok || mem.Name == nil || g.text(mem.Name) != "utf8" {
		return nil, false
	}
	t := g.typeOf(mem.X)
	if t == nil {
		return nil, false
	}
	if b, ok := t.Underlying().(*types.Basic); !ok || b.Kind() != types.String {
		return nil, false
	}
	return mem.X, true
}

// isUInt8 reports whether t is UInt8, the element of an array of bytes.
func isUInt8(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.UInt8
}

// subscriptAssign is `a[i] = v` and `d[k] = v`, or false where the target
// is not a collection subscript.
func (g *gen) subscriptAssign(target ast.Expr, value collArg) bool {
	sub, ok := target.(*ast.SubscriptExpr)
	if !ok || !(len(sub.Args) == 1 || g.defaultSubscript(sub)) {
		return false
	}
	switch u := g.typeOf(sub.X).Underlying().(type) {
	case *types.Array:
		g.collectionCall(sub, core.ArraySet(u), sub.X, []collArg{{expr: sub.Args[0].X}, value})
		return true
	case *types.Dictionary:
		m, ok := core.DictionarySet(u)
		if !ok {
			g.refuse(sub, "a dictionary whose key type '"+u.Key.String()+"' the runtime does not hash yet")
			return true
		}
		g.collectionCall(sub, m, sub.X, []collArg{{expr: sub.Args[0].X}, value})
		return true
	}
	return false
}

// defaultSubscript reports whether a subscript is `d[key, default: value]`.
func (g *gen) defaultSubscript(sub *ast.SubscriptExpr) bool {
	return len(sub.Args) == 2 && sub.Args[1].Label != nil && g.text(sub.Args[1].Label) == "default"
}

// dictionaryLiteral is `[k: v, …]` and `[:]`.
func (g *gen) dictionaryLiteral(e *ast.DictLit) *sil.Value {
	d, ok := g.typeOf(e).Underlying().(*types.Dictionary)
	if !ok {
		g.refuse(e, "a dictionary literal whose type is not known")
		return nil
	}
	if !core.RuntimeHashable(d.Key) {
		g.refuse(e, "a dictionary whose key type '"+d.Key.String()+"' the runtime does not hash yet")
		return nil
	}
	return g.hashTableLiteral(e, g.typeOf(e), len(e.Items), func(slot *sil.Value) bool {
		keyMeta, ok := g.stdlibMetadata(e, d.Key)
		if !ok {
			return false
		}
		valueMeta, ok := g.stdlibMetadata(e, d.Value)
		if !ok {
			return false
		}
		for _, item := range e.Items {
			key := g.rvalue(item.Key)
			value := g.rvalue(item.Value)
			if key == nil || value == nil {
				return false
			}
			kt, vt := lowerType(d.Key), lowerType(d.Value)
			ks := g.blk.AllocStack(kt)
			g.blk.Store(key, ks, storeQualifier(kt))
			// A value wanted as an existential is put in one, which is
			// memory already, and the runtime takes it from there.
			var vs *sil.Value
			if _, isEx := existentialOf(d.Value); isEx {
				vs = g.existentialFor(item.Value, value, g.typeOf(item.Value), d.Value)
			} else {
				value = g.optionalFor(item.Value, value, g.typeOf(item.Value), d.Value)
				vs = g.blk.AllocStack(vt)
				g.blk.Store(value, vs, storeQualifier(vt))
			}
			g.runtimeVoid(stdlib.DictionaryInsertLiteral, []sil.Param{
				{Type: lowerType(g.typeOf(e)).Address(), Convention: sil.ParamInout},
				{Type: kt.Address(), Convention: sil.ParamIn},
				{Type: vt.Address(), Convention: sil.ParamIn},
				{Type: sil.Object(sil.BuiltinRawPointer)},
				{Type: sil.Object(sil.BuiltinRawPointer)},
			}, slot, ks, vs, keyMeta, valueMeta)
		}
		return true
	})
}

// setLiteral is an array literal written where a Set is wanted.
func (g *gen) setLiteral(e *ast.ArrayLit, s *types.Set) *sil.Value {
	if !core.RuntimeHashable(s.Elem) {
		g.refuse(e, "a set whose element type '"+s.Elem.String()+"' the runtime does not hash yet")
		return nil
	}
	return g.hashTableLiteral(e, g.typeOf(e), len(e.Items), func(slot *sil.Value) bool {
		meta, ok := g.stdlibMetadata(e, s.Elem)
		if !ok {
			return false
		}
		for _, item := range e.Items {
			v := g.rvalue(item)
			if v == nil {
				return false
			}
			et := lowerType(s.Elem)
			es := g.blk.AllocStack(et)
			g.blk.Store(v, es, storeQualifier(et))
			g.runtimeVoid(stdlib.SetInsert, []sil.Param{
				{Type: lowerType(g.typeOf(e)).Address(), Convention: sil.ParamInout},
				{Type: et.Address(), Convention: sil.ParamInGuaranteed},
				{Type: sil.Object(sil.BuiltinRawPointer)},
			}, slot, es, meta)
			if !et.Trivial() {
				g.destroyLater(g.blk.Load(es, "take"))
			}
		}
		return true
	})
}

// hashTableLiteral starts from the empty table, lets fill insert into the
// variable holding it, and hands back what the variable ends up holding.
func (g *gen) hashTableLiteral(at ast.Node, t types.Type, items int, fill func(slot *sil.Value) bool) *sil.Value {
	lt := lowerType(t)
	empty := g.m.Func(stdlib.HashTableEmpty).SetSourceName(stdlib.HashTableEmpty)
	if g.needsType(empty) {
		empty.SetLinkage(sil.PublicExternal)
		empty.Type().Convention = sil.Thin
		empty.SetResult(lt, sil.ResultOwned)
	}
	table := g.blk.Apply(g.blk.FunctionRef(empty), lt)
	if items == 0 {
		g.destroyLater(table)
		return table
	}
	slot := g.blk.AllocStack(lt)
	g.blk.Store(table, slot, storeQualifier(lt))
	if !fill(slot) {
		return nil
	}
	v := g.blk.Load(slot, "take")
	g.destroyLater(v)
	return v
}

// forInHashTable lowers iteration over Sets and Dictionaries by walking table buckets.
// value is nil for a Set.
func (g *gen) forInHashTable(s *ast.ForInStmt, key, value types.Type) {
	seqType := g.typeOf(s.Seq)
	g.push()
	table := g.expr(s.Seq)
	if table == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return
	}
	keyMeta, ok := g.stdlibMetadata(s, key)
	if !ok {
		return
	}
	var valueMeta *sil.Value
	if value != nil {
		if valueMeta, ok = g.stdlibMetadata(s, value); !ok {
			return
		}
	}

	intType := types.Typ[types.Int]
	it := lowerType(intType)
	tt := lowerType(seqType)
	zero := func() *sil.Value {
		return g.blk.Struct(it, g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt64), 0))
	}
	box := g.blk.AllocBox(it, "$bucket", "var")
	borrow := g.blk.BeginBorrow(box, "var_decl")
	slot := g.blk.ProjectBox(borrow, 0, it)
	g.destroyLater(box)
	g.endBorrowLater(borrow)
	g.blk.Store(zero(), slot, storeQualifier(it))

	next := g.m.Func(stdlib.HashTableNext).SetSourceName(stdlib.HashTableNext)
	if g.needsType(next) {
		next.SetLinkage(sil.PublicExternal)
		next.Type().Convention = sil.Thin
		next.Type().Params = []sil.Param{
			{Type: tt, Convention: sil.ParamGuaranteed},
			{Type: it, Convention: sil.ParamUnowned},
		}
		next.SetResult(it, resultConvention(it))
	}

	header := g.fn.Block()
	body := g.fn.Block()
	latch := g.fn.Block()
	exit := g.fn.Block()
	label := g.takeLabel()
	g.blk.Br(header)

	g.blk = header
	from := g.blk.Load(slot, loadQualifier(it))
	at := g.blk.Apply(g.blk.FunctionRef(next), it, table, from)
	g.blk.Store(at, slot, storeQualifier(it))
	done := g.compare("<", at, zero(), intType)
	if done == nil {
		g.refuse(s.Seq, "an index this compiler cannot compare")
		return
	}
	g.blk.CondBr(done, exit, nil, body, nil)

	g.blk = body
	at = g.blk.Load(slot, loadQualifier(it))
	// The copies belong to the iteration, and go at the end of the body.
	inner := len(g.scopes)
	g.push()
	k := g.copyOutOfBucket(stdlib.HashTableKeyAt, tt, table, at, key, keyMeta)
	if value == nil {
		g.bindOrDrop(s.Pat, k, key)
	} else {
		v := g.copyOutOfBucket(stdlib.DictionaryValueAt, tt, table, at, value, valueMeta)
		if tp, ok := s.Pat.(*ast.TuplePattern); ok && len(tp.Elems) == 2 {
			g.bindOrDrop(tp.Elems[0].Pat, k, key)
			g.bindOrDrop(tp.Elems[1].Pat, v, value)
		} else {
			entry := &types.Tuple{Elements: []*types.TupleElement{
				{Name: "key", Type: key}, {Name: "value", Type: value}}}
			g.bindOrDrop(s.Pat, g.blk.Tuple(lowerType(entry), k, v), entry)
		}
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
	step := g.increment(cur, intType)
	if step == nil {
		g.refuse(s.Seq, "an index this compiler cannot step")
		return
	}
	g.blk.Store(step, slot, storeQualifier(it))
	g.blk.Br(header)

	g.blk = exit
	g.pop()
}

// copyOutOfBucket calls the runtime function that copies a key or a
// value out of a bucket, and answers the copy, owned.
func (g *gen) copyOutOfBucket(symbol string, tt sil.Type, table, at *sil.Value, t types.Type, meta *sil.Value) *sil.Value {
	lt := lowerType(t)
	out := g.blk.AllocStack(lt)
	g.runtimeVoid(symbol, []sil.Param{
		{Type: tt, Convention: sil.ParamGuaranteed},
		{Type: lowerType(types.Typ[types.Int]), Convention: sil.ParamUnowned},
		{Type: lt.Address(), Convention: sil.ParamInout},
		{Type: sil.Object(sil.BuiltinRawPointer)},
	}, table, at, out, meta)
	qual := "take"
	if lt.Trivial() {
		qual = "trivial"
	}
	return g.blk.Load(out, qual)
}

// bindOrDrop binds a loop pattern's name to an owned value, or lets the
// value go where the pattern binds nothing.
func (g *gen) bindOrDrop(pat ast.Pattern, v *sil.Value, t types.Type) {
	if patternIdent(pat) == nil {
		g.destroyLater(v)
		return
	}
	g.bindLoopVar(pat, v, lowerType(t))
}

// runtimeVoid calls a runtime function that answers nothing.
func (g *gen) runtimeVoid(symbol string, params []sil.Param, args ...*sil.Value) {
	callee := g.m.Func(symbol).SetSourceName(symbol)
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		callee.Type().Convention = sil.Thin
		callee.Type().Params = params
	}
	g.blk.Apply(g.blk.FunctionRef(callee), lowerType(types.Typ[types.Void]), args...)
}

// isCoreHasher reports whether t is core's Hasher, whose initializer and
// methods are the runtime's.
func (g *gen) isCoreHasher(t types.Type) bool {
	if t == nil || !g.info.CoreTypes[t] {
		return false
	}
	st, ok := t.Underlying().(*types.Struct)
	return ok && st.Name == "Hasher"
}

// hasherCall lowers a call of one of Hasher's methods, or reports that the
// call is not one.
func (g *gen) hasherCall(e *ast.CallExpr, mem *ast.MemberExpr) (*sil.Value, bool) {
	if mem.Name == nil || !g.isCoreHasher(g.typeOf(mem.X)) {
		return nil, false
	}
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	switch g.text(mem.Name) {
	case "combine":
		if len(args) != 1 {
			break
		}
		return g.collectionCall(e, core.HasherCombine(g.typeOf(args[0].X)), mem.X, exprArgs(args[0].X)), true
	case "finalize":
		if len(args) != 0 {
			break
		}
		return g.collectionCall(e, core.HasherFinalize(), mem.X, nil), true
	}
	g.refuse(e, "a call of Hasher's '"+g.text(mem.Name)+"'")
	return nil, true
}

// isOptionalExistential reports whether t is an optional of an existential,
// which is kept in memory as the existential itself is.
func isOptionalExistential(t types.Type) bool {
	o, ok := optionalOf(t)
	if !ok {
		return false
	}
	_, isEx := existentialOf(o.Wrapped)
	return isEx
}
