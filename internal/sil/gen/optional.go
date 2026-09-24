package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

const (
	optionalSome = "Optional.some"
	optionalNone = "Optional.none"
)

// optionalOf is the optional a type is, if it is one.
func optionalOf(t types.Type) (*types.Optional, bool) {
	if t == nil {
		return nil, false
	}
	o, ok := t.Underlying().(*types.Optional)
	return o, ok
}

// nilValue is the empty case of whatever optional `nil` was written
// in, and reports whether the expression was one.
func (g *gen) nilValue(e ast.Expr) (*sil.Value, bool) {
	t := g.typeOf(e)
	if _, ok := optionalOf(t); !ok {
		return nil, false
	}
	return g.blk.Enum(lowerType(t), optionalNone, nil), true
}

// optionalFor wraps a value into an Optional.some if target type is an optional.
//
// A value is wrapped once for each level the destination has beyond
// its own: a T given for a T?? is some(some(v)), a T? given for a T??
// is some(v).
func (g *gen) optionalFor(at ast.Node, v *sil.Value, from, to types.Type) *sil.Value {
	if v == nil {
		return v
	}
	if relabeled, ok := g.relabel(v, from, to); ok {
		return relabeled
	}
	levels := optionalDepth(to) - optionalDepth(from)
	// A metatype given where an existential metatype goes -- Double.self
	// for an Any.Type -- is the same metadata, retyped.
	if fm, ok := from.(*types.Metatype); ok {
		inner := to
		for i := 0; i < levels; i++ {
			o, _ := optionalOf(inner)
			inner = o.Wrapped
		}
		if tm, ok := inner.(*types.Metatype); ok && !types.Identical(fm, tm) {
			v = g.blk.Builtin("bitcast_RawPointer_RawPointer", lowerType(g.substituted(tm)), v)
		}
	}
	// A subclass's instance where its superclass is wanted is the same
	// reference, upcast -- inside the optionals it is wrapped in below.
	if levels >= 0 {
		inner := to
		for i := 0; i < levels; i++ {
			o, _ := optionalOf(inner)
			inner = o.Wrapped
		}
		if isSubclass(from, inner) {
			v = g.upcast(v, g.substituted(inner))
		}
	}
	// A T? where an (any P)? goes: its payload, if any, made the
	// existential -- `d.delegate = screen`, a Screen? for a Delegate?.
	if levels == 0 {
		fo, fromOpt := optionalOf(g.substituted(from))
		to2, toOpt := optionalOf(g.substituted(to))
		if fromOpt && toOpt && isExistentialType(to2.Wrapped) && !isExistentialType(fo.Wrapped) {
			return g.optionalToExistential(at, v, fo, to2)
		}
	}
	if levels <= 0 {
		return v
	}
	// An existential an optional carries is carried as a value: made in
	// memory as any existential is, and loaded from there.
	if levels > 0 {
		inner := to
		for i := 0; i < levels; i++ {
			o, _ := optionalOf(inner)
			inner = o.Wrapped
		}
		if isExistentialType(inner) {
			v = g.existentialValue(at, v, from, g.substituted(inner))
			if v == nil {
				return nil
			}
		}
	}
	// Innermost first: the type each wrap makes is the destination
	// with the outer levels peeled off.
	wraps := make([]types.Type, 0, levels)
	for t := to; len(wraps) < levels; {
		o, _ := optionalOf(t)
		wraps = append(wraps, t)
		t = o.Wrapped
	}
	for i := len(wraps) - 1; i >= 0; i-- {
		v = g.blk.Enum(lowerType(g.substituted(wraps[i])), optionalSome, v)
	}
	return v
}

// optionalToExistential is v, a from, as a to whose payload is an
// existential: nil where v is, and v's payload made the existential
// otherwise.
func (g *gen) optionalToExistential(at ast.Node, v *sil.Value, from, to *types.Optional) *sil.Value {
	wrapped := lowerType(from.Wrapped)
	own := sil.Owned
	if wrapped.Trivial() {
		own = sil.Unowned
	}
	if v.Ownership() != sil.Owned && !lowerType(from).Trivial() {
		v = g.blk.CopyValue(v)
	} else {
		v = g.consume(v)
	}
	some, none, join := g.fn.Block(), g.fn.Block(), g.fn.Block()
	payload := some.Arg(wrapped, own)
	out := join.Arg(lowerType(to), sil.Owned)
	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: none})
	g.blk = some
	ex := g.existentialValue(at, payload, from.Wrapped, to.Wrapped)
	if ex == nil {
		return nil
	}
	g.blk.Br(join, g.blk.Enum(lowerType(to), optionalSome, ex))
	g.blk = none
	g.blk.Br(join, g.blk.Enum(lowerType(to), optionalNone, nil))
	g.blk = join
	return out
}

// existentialValue is v, of type from, as an existential of type to held
// as a value -- what an optional, a case or a field carries -- owned, for
// the caller to consume: a copy of an existential in memory, or one made
// of any other value.
func (g *gen) existentialValue(at ast.Node, v *sil.Value, from, to types.Type) *sil.Value {
	if isExistentialType(from) {
		if !v.Type().IsAddress() {
			return v
		}
		return g.blk.Load(v, "copy")
	}
	slot := g.existentialFor(at, v, from, to)
	if slot == nil || slot == v || !slot.Type().IsAddress() {
		return nil
	}
	return g.blk.Load(slot, "take")
}

// relabel is a tuple as the same elements under other labels -- (x: 1,
// y: 2) where an (Int, Int) is wanted, or the other way -- taken apart and
// put together again, which is all the conversion is.
func (g *gen) relabel(v *sil.Value, from, to types.Type) (*sil.Value, bool) {
	ft, ok := g.substituted(from).(*types.Tuple)
	if !ok {
		return nil, false
	}
	tt, ok := g.substituted(to).(*types.Tuple)
	if !ok || len(ft.Elements) != len(tt.Elements) || types.Identical(ft, tt) || !types.AssignableTo(ft, tt) {
		return nil, false
	}
	elems := make([]sil.Type, len(tt.Elements))
	for i, el := range tt.Elements {
		elems[i] = lowerType(el.Type)
	}
	parts := g.blk.DestructureTuple(g.consume(v), elems...)
	out := g.blk.Tuple(lowerType(tt), parts...)
	g.destroyLater(out)
	return out, true
}

// optionalDepth is how many optionals deep a type is: 0 for an Int, 1
// for an Int?, 2 for an Int??.
func optionalDepth(t types.Type) int {
	n := 0
	for {
		o, ok := optionalOf(t)
		if !ok {
			return n
		}
		n++
		t = o.Wrapped
	}
}

// bindCondition lowers an optional binding condition (if/while/guard let x = v) using switch_enum.
func (g *gen) bindCondition(b *ast.OptionalBinding, fail func() *sil.Block) (*sil.Value, bool) {
	subject := b.Value
	if subject == nil {
		g.refuse(b, "a binding condition with no value")
		return nil, false
	}
	o, isOpt := optionalOf(g.typeOf(subject))
	if !isOpt {
		g.refuse(b, "a binding condition on something that is not an optional")
		return nil, false
	}
	wrapped := lowerType(o.Wrapped)
	v, own := g.switchable(subject, wrapped)
	if v == nil {
		return nil, false
	}

	some := g.fn.Block()
	payload := some.Arg(wrapped, own)
	none := fail()
	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: none})

	g.blk = some
	// `if let (a, b) = pair` takes the payload apart as a case pattern
	// takes what a case carries.
	if _, isTuple := b.Pat.(*ast.TuplePattern); isTuple {
		if !g.bindPatternTo(b.Pat, payload, o.Wrapped) {
			return nil, false
		}
		return payload, true
	}
	_, sym := g.binding(&ast.PatternBinding{Pat: b.Pat})
	// `if var x = o` binds storage the body may change, which starts as
	// what o held.
	if b.Kind == token.VAR && sym != nil {
		name := ""
		if id := patternIdent(b.Pat); id != nil {
			name = g.text(id)
		}
		v := payload
		if own != sil.Owned && !wrapped.Trivial() {
			v = g.blk.CopyValue(payload)
		}
		box := g.blk.AllocBox(wrapped, name, "var")
		addr := g.blk.ProjectBox(box, 0, wrapped)
		g.blk.Store(v, addr, storeQualifier(wrapped))
		g.locals[sym] = &local{addr: addr, box: box, typ: wrapped}
		g.destroyLater(box)
		return box, true
	}
	if sym != nil {
		g.locals[sym] = &local{value: payload, typ: wrapped}
	}
	g.destroyLater(payload)
	return payload, true
}

// nilCoalescing lowers `a ?? b` by switching on a and evaluating b only on the none branch.
func (g *gen) nilCoalescing(e *ast.BinaryExpr) *sil.Value {
	o, isOpt := optionalOf(g.typeOf(e.X))
	if !isOpt {
		g.refuse(e, "'??' on something that is not an optional")
		return nil
	}
	// With an optional on the right -- `hit ?? find(other)` -- the
	// answer is an optional: a's value wrapped again where there is
	// one, and b as it is where there is not.
	rt := g.typeOf(e)
	wrapped := lowerType(o.Wrapped)
	v, own := g.switchable(e.X, wrapped)
	if v == nil {
		return nil
	}
	result := lowerType(rt)

	some := g.fn.Block()
	none := g.fn.Block()
	join := g.fn.Block()
	payload := some.Arg(wrapped, own)
	answer := join.Arg(result, joinOwnership(result))

	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: none})

	g.blk = some
	g.blk.Br(join, g.optionalFor(e.X, payload, o.Wrapped, rt))

	// What b makes on the way -- a borrow of the receiver it is called
	// on -- ends here, before the branch, where it is dominated.
	g.blk = none
	g.push()
	fallback := g.rvalue(e.Y)
	if fallback == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return nil
	}
	fallback = g.optionalFor(e.Y, fallback, g.typeOf(e.Y), rt)
	if !result.Trivial() {
		fallback = g.consume(fallback)
	}
	g.pop()
	g.blk.Br(join, fallback)

	g.blk = join
	g.destroyLater(answer)
	return answer
}

// forceUnwrap lowers `o!` by switching on o and trapping on none.
func (g *gen) forceUnwrap(e *ast.ForceExpr) *sil.Value {
	return g.unwrapOrTrap(e, e.X)
}

// implicitUnwrap is the value of an implicitly unwrapped optional used as
// the type it wraps, which is `x!` written by the checker. The analyzer
// types e as what it wraps; evaluated here, it is the optional it was.
func (g *gen) implicitUnwrap(e ast.Expr, opt types.Type) *sil.Value {
	wrapped := g.info.Types[e]
	delete(g.info.Unwrapped, e)
	g.info.Types[e] = opt
	defer func() {
		g.info.Unwrapped[e] = opt
		g.info.Types[e] = wrapped
	}()
	return g.unwrapOrTrap(e, e)
}

// unwrapOrTrap is the payload of the optional x, trapping if it is nil.
func (g *gen) unwrapOrTrap(at ast.Node, x ast.Expr) *sil.Value {
	o, isOptional := optionalOf(g.typeOf(x))
	if !isOptional {
		g.refuse(at, "'!' on something that is not an optional")
		return nil
	}
	wrapped := lowerType(o.Wrapped)
	v, own := g.switchable(x, wrapped)
	if v == nil {
		return nil
	}
	some := g.fn.Block()
	none := g.fn.Block()
	payload := some.Arg(wrapped, own)
	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: none})

	g.blk = none
	always := g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), -1)
	g.blk.CondFail(always, "Unexpectedly found nil while unwrapping an Optional value")
	g.blk.Unreachable()

	g.blk = some
	// A temporary: it ends with the statement, as Swift ends it. Held to
	// the end of the block, `p!.x = y` kept p's object alive after
	// `p = nil`, and its deinit ran late.
	g.destroyTemp(payload)
	return payload
}

// optionalComparison lowers `==` and `!=` comparisons involving optionals.
func (g *gen) optionalComparison(e *ast.BinaryExpr, op string) (*sil.Value, bool) {
	if op != "==" && op != "!=" {
		return nil, false
	}
	x, y := g.fold(e.X), g.fold(e.Y)
	xNil, yNil := isNilLiteral(x), isNilLiteral(y)
	xOpt, xIsOpt := optionalOf(g.typeOf(x))
	yOpt, yIsOpt := optionalOf(g.typeOf(y))

	switch {
	// `o == nil`: the answer is the case, and no payload is read.
	case xIsOpt && yNil:
		return g.emptyTest(e, x, xOpt, op), true
	case yIsOpt && xNil:
		return g.emptyTest(e, y, yOpt, op), true
	// An optional of something that owns what it holds -- a String? --
	// is compared by the runtime, through the optional's metadata, which
	// ends every value it was lent itself.
	case xIsOpt && !lowerType(xOpt.Wrapped).Trivial():
		return g.runtimeEquality(e, x, y, g.typeOf(x), op), true
	case yIsOpt && !lowerType(yOpt.Wrapped).Trivial():
		return g.runtimeEquality(e, x, y, g.typeOf(y), op), true
	// Both optional: the cases have to agree, and where both hold
	// something the payloads do too.
	case xIsOpt && yIsOpt:
		return g.bothOptional(e, x, y, xOpt, yOpt, op), true
	// One optional and one not. The plain side is a value that is
	// always there, so the empty case is unequal and the full case is
	// the comparison of the payload with it.
	case xIsOpt:
		return g.halfOptional(e, x, y, xOpt, op, false), true
	case yIsOpt:
		return g.halfOptional(e, y, x, yOpt, op, true), true
	}
	return nil, false
}

// comparable reports whether an optional's payload can take part in a
// comparison here, and says why where it cannot.
//
// The same two limits every switch over an optional has: the payload
// arrives as one block argument, and one that owns what it holds
// needs a lifetime this does not arrange.
func (g *gen) comparable(at ast.Node, o *types.Optional) bool {
	if !lowerType(o.Wrapped).Trivial() {
		g.refuse(at, "comparing an optional of "+o.Wrapped.String()+
			", which owns what it holds")
		return false
	}
	return true
}

// boolOf wraps a machine bit in the Bool the expression's type is.
//
// The bit is what a branch takes, and the value is what an expression
// answers. Handing back the bit made the SIL ill-typed -- a
// struct_extract of #Bool._value from a $Builtin.Int1 -- which
// happened to lower to the right thing and would not have to.
func (g *gen) boolOf(bit *sil.Value) *sil.Value {
	return g.blk.Struct(lowerType(types.Typ[types.Bool]), bit)
}

// emptyTest is `o == nil` and `o != nil`: the answer is which case it
// holds, and the payload is never read.
func (g *gen) emptyTest(e *ast.BinaryExpr, subject ast.Expr, o *types.Optional, op string) *sil.Value {
	wrapped := lowerType(o.Wrapped)
	v, own := g.switchable(subject, wrapped)
	if v == nil {
		return nil
	}
	bit := sil.Object(sil.BuiltinInt1)
	some, none, join := g.fn.Block(), g.fn.Block(), g.fn.Block()
	answer := join.Arg(bit, sil.None)
	payload := some.Arg(wrapped, own)
	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: none})

	full, empty := int64(0), int64(-1)
	if op == "!=" {
		full, empty = -1, 0
	}
	g.blk = some
	if own == sil.Owned {
		g.blk.DestroyValue(payload)
	}
	g.blk.Br(join, g.blk.IntegerLiteral(bit, full))
	g.blk = none
	g.blk.Br(join, g.blk.IntegerLiteral(bit, empty))

	g.blk = join
	return g.boolOf(answer)
}

// halfOptional compares an optional with a non-optional value.
func (g *gen) halfOptional(e *ast.BinaryExpr, opt, plain ast.Expr,
	o *types.Optional, op string, flipped bool) *sil.Value {
	if !g.comparable(e, o) {
		return nil
	}
	v := g.rvalue(opt)
	if v == nil {
		return nil
	}
	bit := sil.Object(sil.BuiltinInt1)
	some, none, join := g.fn.Block(), g.fn.Block(), g.fn.Block()
	answer := join.Arg(bit, sil.None)
	payload := some.Arg(lowerType(o.Wrapped), sil.Unowned)
	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: none})

	g.blk = some
	// The plain side is evaluated on this arm alone, so what it borrows
	// is given back on this arm too, before the arms join.
	g.push()
	other := g.rvalue(plain)
	if other == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return nil
	}
	lhs, rhs := payload, other
	if flipped {
		lhs, rhs = other, payload
	}
	same := g.comparePayloads(e, o.Wrapped, op, lhs, rhs)
	if same == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return nil
	}
	g.pop()
	g.blk.Br(join, same)

	// Nothing on one side and something on the other: unequal.
	g.blk = none
	unequal := int64(0)
	if op == "!=" {
		unequal = -1
	}
	g.blk.Br(join, g.blk.IntegerLiteral(bit, unequal))

	g.blk = join
	return g.boolOf(answer)
}

// bothOptional compares two optional values for equality.
func (g *gen) bothOptional(e *ast.BinaryExpr, xe, ye ast.Expr,
	xo, yo *types.Optional, op string) *sil.Value {
	if !g.comparable(e, xo) || !g.comparable(e, yo) {
		return nil
	}
	x := g.rvalue(xe)
	if x == nil {
		return nil
	}
	bit := sil.Object(sil.BuiltinInt1)
	equal, unequal := int64(-1), int64(0)
	if op == "!=" {
		equal, unequal = 0, -1
	}

	xSome, xNone, join := g.fn.Block(), g.fn.Block(), g.fn.Block()
	answer := join.Arg(bit, sil.None)
	xPayload := xSome.Arg(lowerType(xo.Wrapped), sil.Unowned)
	g.blk.SwitchEnum(x,
		sil.Case{Member: optionalSome, Dest: xSome},
		sil.Case{Member: optionalNone, Dest: xNone})

	// Evaluate right operand inside branches to preserve domination,
	// and give back what it borrows on those branches, before the join.
	g.blk = xSome
	ySome, yNone := g.fn.Block(), g.fn.Block()
	yPayload := ySome.Arg(lowerType(yo.Wrapped), sil.Unowned)
	g.push()
	yFull := g.rvalue(ye)
	if yFull == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return nil
	}
	yScope := g.scopes[len(g.scopes)-1]
	g.scopes = g.scopes[:len(g.scopes)-1]
	g.blk.SwitchEnum(yFull,
		sil.Case{Member: optionalSome, Dest: ySome},
		sil.Case{Member: optionalNone, Dest: yNone})

	g.blk = ySome
	same := g.comparePayloads(e, xo.Wrapped, op, xPayload, yPayload)
	if same == nil {
		return nil
	}
	g.emitCleanups(yScope)
	g.blk.Br(join, same)

	g.blk = yNone
	g.emitCleanups(yScope)
	g.blk.Br(join, g.blk.IntegerLiteral(bit, unequal))

	// The left holds nothing, so the answer is whether the right
	// holds nothing too.
	g.blk = xNone
	emptySome, emptyNone := g.fn.Block(), g.fn.Block()
	emptySome.Arg(lowerType(yo.Wrapped), sil.Unowned)
	g.push()
	yEmpty := g.rvalue(ye)
	if yEmpty == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return nil
	}
	emptyScope := g.scopes[len(g.scopes)-1]
	g.scopes = g.scopes[:len(g.scopes)-1]
	g.blk.SwitchEnum(yEmpty,
		sil.Case{Member: optionalSome, Dest: emptySome},
		sil.Case{Member: optionalNone, Dest: emptyNone})

	g.blk = emptySome
	g.emitCleanups(emptyScope)
	g.blk.Br(join, g.blk.IntegerLiteral(bit, unequal))
	g.blk = emptyNone
	g.emitCleanups(emptyScope)
	g.blk.Br(join, g.blk.IntegerLiteral(bit, equal))

	g.blk = join
	return g.boolOf(answer)
}

// comparePayloads answers the bit two payloads' comparison produces.
//
// operate is what compares them, because the payloads are ordinary
// values of an ordinary type and `==` on that type is whatever core
// says it is -- an instruction for a number, and the enum's own
// comparison for an enum.
//
// A type that declares or derives its own -- an Equatable struct or
// enum -- is compared by that, as the checker recorded it for the
// expression; an enum of cases alone by its tag.
func (g *gen) comparePayloads(at *ast.BinaryExpr, wrapped types.Type, op string,
	lhs, rhs *sil.Value) *sil.Value {
	xs, vals := []ast.Expr{at.X, at.Y}, []*sil.Value{lhs, rhs}
	var v *sil.Value
	sym, _ := g.info.Operators[at].(*analyzer.FuncSymbol)
	switch {
	case g.info.OperatorMethods[at] != nil || (sym != nil && !g.coreOperator(sym)):
		v = g.operatorApply(at, g.info.OperatorMethods[at], sym, xs, vals)
	case g.info.DerivedOperators[at] != nil:
		d := g.info.DerivedOperators[at]
		if d.Swap {
			xs, vals = []ast.Expr{at.Y, at.X}, []*sil.Value{rhs, lhs}
		}
		v = g.operatorApply(at, d.Method, d.Fn, xs, vals)
		if v != nil && d.Negate {
			v = g.notBool(at, v)
		}
	default:
		if e, ok := enumFor(wrapped); ok && !hasPayloadCase(e) {
			verb := "cmp_eq_"
			if op == "!=" {
				verb = "cmp_ne_"
			}
			return g.blk.Builtin(verb+enumMachine(e), sil.Object(sil.BuiltinInt1), lhs, rhs)
		}
		v = g.operate(at, op, wrapped, types.Typ[types.Bool], lhs, rhs)
	}
	if v == nil {
		g.refuse(at, "comparing two optionals of "+wrapped.String())
		return nil
	}
	return g.machine(v, types.Typ[types.Bool])
}

// isNilLiteral reports whether an expression is the literal nil.
func isNilLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.NIL
}

// chainRoot lowers an optional chain: its steps run where every `a?` along
// it finds a value, and answer that value's result; any `a?` that finds
// nothing goes straight to the empty answer. A result that is optional
// already is the answer as it is, as Swift flattens it.
func (g *gen) chainRoot(e ast.Expr) *sil.Value {
	rt := lowerType(g.typeOf(e))
	none, join := g.fn.Block(), g.fn.Block()
	answer := join.Arg(rt, joinOwnership(rt))

	if g.chainActive == nil {
		g.chainActive = map[ast.Expr]bool{}
	}
	g.chainActive[e] = true
	g.chainNone = append(g.chainNone, none)
	g.push()
	g.chainDepth = append(g.chainDepth, len(g.scopes)-1)
	v := g.expr(e)
	inner := g.typeOf(e)
	delete(g.chainActive, e)
	g.chainNone = g.chainNone[:len(g.chainNone)-1]
	g.chainDepth = g.chainDepth[:len(g.chainDepth)-1]
	if v == nil {
		g.pop()
		return nil
	}
	// The answer outlives the steps' temporaries, which end here.
	out := g.consume(v)
	g.pop()
	if _, already := optionalOf(inner); already || isVoid(inner) {
		if isVoid(inner) {
			out = g.blk.Enum(rt, optionalSome, nil)
		}
		g.blk.Br(join, out)
	} else {
		g.blk.Br(join, g.blk.Enum(rt, optionalSome, out))
	}

	g.blk = none
	g.blk.Br(join, g.blk.Enum(rt, optionalNone, nil))
	g.blk = join
	g.destroyLater(answer)
	return answer
}

// chainStep lowers `a?` inside a chain: a's value where there is one, and
// a jump to the chain's empty answer where there is not.
func (g *gen) chainStep(e *ast.OptionalExpr) *sil.Value {
	if len(g.chainNone) == 0 {
		g.refuse(e, "an '?' outside an optional chain")
		return nil
	}
	o, ok := optionalOf(g.typeOf(e.X))
	if !ok {
		g.refuse(e, "an '?' on something that is not an optional")
		return nil
	}
	wrapped := lowerType(o.Wrapped)
	v, own := g.switchable(e.X, wrapped)
	if v == nil {
		return nil
	}
	some, empty := g.fn.Block(), g.fn.Block()
	payload := some.Arg(wrapped, own)
	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: empty})
	g.blk = empty
	// What the chain has made so far -- an earlier step's payload -- is
	// let go of on the way out, as it would be at the chain's end.
	g.unwindTo(g.chainDepth[len(g.chainDepth)-1])
	g.blk.Br(g.chainNone[len(g.chainNone)-1])
	g.blk = some
	if own == sil.Owned {
		g.destroyLater(payload)
	}
	return payload
}

// chainStepAddr lowers `a?` as a destination inside a chain -- `h.inner?.n
// = 5`, `o?.bump()` -- as the address of a's payload where a holds one,
// and a jump to the chain's empty answer where it does not. Which case
// the storage holds is read from a copy of it; the payload is then
// written in place, so the write changes this optional and no other.
func (g *gen) chainStepAddr(e *ast.OptionalExpr) *sil.Value {
	if len(g.chainNone) == 0 {
		g.refuse(e, "an '?' outside an optional chain")
		return nil
	}
	o, ok := optionalOf(g.typeOf(e.X))
	if !ok {
		g.refuse(e, "an '?' on something that is not an optional")
		return nil
	}
	addr := g.lvalue(e.X)
	if addr == nil {
		return nil
	}
	// A weak or unowned property is read through the runtime -- a strong
	// reference, or nil once its object has ended -- into a temporary
	// the chain goes on from.
	if f := g.refSlots[addr]; f != nil {
		v := g.refLoad(addr, f)
		lt := lowerType(o)
		tmp := g.blk.AllocStack(lt)
		g.blk.Store(g.blk.CopyValue(v), tmp, "init")
		g.destroyAddrLater(tmp)
		addr = tmp
	}
	wrapped := lowerType(o.Wrapped)
	access := g.blk.BeginAccess(addr, "read", "unknown")
	v := g.blk.Load(access, loadQualifier(access.Type()))
	g.blk.EndAccess(access)
	own := sil.Owned
	if wrapped.Trivial() {
		own = sil.Unowned
	}
	some, empty := g.fn.Block(), g.fn.Block()
	payload := some.Arg(wrapped, own)
	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: empty})
	g.blk = empty
	g.unwindTo(g.chainDepth[len(g.chainDepth)-1])
	g.blk.Br(g.chainNone[len(g.chainNone)-1])
	g.blk = some
	// The copy told the tag; the payload written is the one in storage.
	if own == sil.Owned {
		g.blk.DestroyValue(payload)
	}
	return g.blk.UncheckedTakeEnumDataAddr(addr, optionalSome, wrapped)
}

// switchable evaluates an optional expression for switching and determines payload ownership.
func (g *gen) switchable(e ast.Expr, wrapped sil.Type) (*sil.Value, sil.Ownership) {
	g.push()
	if wrapped.Trivial() {
		// Nothing to own on the arms, but the subject may still have
		// made temporaries -- a dictionary lookup's key -- which end
		// before the switch, on both of its arms.
		v := g.rvalue(e)
		if v == nil {
			g.scopes = g.scopes[:len(g.scopes)-1]
			return nil, sil.Unowned
		}
		g.pop()
		return v, sil.Unowned
	}
	v := g.rvalue(e)
	if v == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return nil, sil.Owned
	}
	switch {
	case v.Ownership() != sil.Owned || g.isLocalValue(v):
		// Borrowed, or a name's own: the switch gets a copy.
		v = g.blk.CopyValue(v)
	case g.pendingDestroy(v):
		// A temporary: the switch takes it over from its cleanup.
		g.forget(v)
	}
	// Otherwise an owned value nothing else will end, which the switch
	// consumes as it is.
	g.pop()
	return v, sil.Owned
}

// pendingDestroy reports whether a value has a cleanup waiting to
// destroy it: a temporary, rather than a name's own value.
func (g *gen) pendingDestroy(v *sil.Value) bool {
	for _, s := range g.scopes {
		for _, c := range s.cleanups {
			if c.destroy == v {
				return true
			}
		}
	}
	return false
}

// isLocalValue reports whether a value is what a name in scope holds.
func (g *gen) isLocalValue(v *sil.Value) bool {
	for _, l := range g.locals {
		if l != nil && l.value == v {
			return true
		}
	}
	return false
}

// runtimeEquality is `x == y` or `x != y` on two values of type t -- either
// operand converted to it where it is not already -- answered by the
// runtime from t's metadata.
func (g *gen) runtimeEquality(e *ast.BinaryExpr, x, y ast.Expr, t types.Type, op string) *sil.Value {
	v := g.collectionCall(e, core.ValuesEqual(t), nil, exprArgs(x, y))
	if v == nil || op == "==" {
		return v
	}
	return g.notBool(e, v)
}

// isSubclass reports whether from is a class that inherits, at some
// remove, from the class to.
func isSubclass(from, to types.Type) bool {
	if from == nil || to == nil || types.Identical(from, to) {
		return false
	}
	target, ok := to.Underlying().(*types.Class)
	if !ok {
		return false
	}
	seen := map[*types.Class]bool{}
	for cur := from; cur != nil; {
		cl, ok := cur.Underlying().(*types.Class)
		if !ok || seen[cl] {
			return false
		}
		// An instance of a generic class is its own substituted class:
		// IntBag's Container<Int> is the Container<Int> wanted.
		if cl == target || types.Identical(cur, to) {
			return true
		}
		seen[cl] = true
		cur = cl.Superclass
	}
	return false
}

// upcast is v, an instance of a subclass, as the superclass to: borrowed
// and copied where v is owned, so what comes back is owned as v was.
func (g *gen) upcast(v *sil.Value, to types.Type) *sil.Value {
	t := lowerType(to)
	if v.Ownership() != sil.Owned {
		return g.blk.Upcast(v, t)
	}
	pending := g.pendingDestroy(v)
	b := g.blk.BeginBorrow(v)
	out := g.blk.CopyValue(g.blk.Upcast(b, t))
	g.blk.EndBorrow(b)
	if pending {
		// v's cleanup ends it later; the copy is the caller's, as v was.
		g.destroyLater(out)
		return out
	}
	g.blk.DestroyValue(v)
	return out
}
