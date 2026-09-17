package gen

import (
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
func (g *gen) optionalFor(at ast.Node, v *sil.Value, from, to types.Type) *sil.Value {
	o, ok := optionalOf(to)
	if !ok || v == nil {
		return v
	}
	if _, already := optionalOf(from); already {
		return v
	}
	if _, nested := optionalOf(o.Wrapped); nested {
		g.refuse(at, "an optional of an optional")
		return v
	}
	return g.blk.Enum(lowerType(to), optionalSome, v)
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
	if _, rhsOptional := optionalOf(g.typeOf(e.Y)); rhsOptional {
		g.refuse(e, "'??' with an optional on the right, which answers an optional")
		return nil
	}
	wrapped := lowerType(o.Wrapped)
	v, own := g.switchable(e.X, wrapped)
	if v == nil {
		return nil
	}
	result := lowerType(g.typeOf(e))

	some := g.fn.Block()
	none := g.fn.Block()
	join := g.fn.Block()
	payload := some.Arg(wrapped, own)
	answer := join.Arg(result, joinOwnership(result))

	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: none})

	g.blk = some
	g.blk.Br(join, payload)

	g.blk = none
	fallback := g.rvalue(e.Y)
	if fallback == nil {
		return nil
	}
	if !wrapped.Trivial() {
		fallback = g.consume(fallback)
	}
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
	g.destroyLater(payload)
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
	other := g.rvalue(plain)
	if other == nil {
		return nil
	}
	lhs, rhs := payload, other
	if flipped {
		lhs, rhs = other, payload
	}
	same := g.comparePayloads(e, o.Wrapped, op, lhs, rhs)
	if same == nil {
		return nil
	}
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

	// Evaluate right operand inside branches to preserve domination.
	g.blk = xSome
	ySome, yNone := g.fn.Block(), g.fn.Block()
	yPayload := ySome.Arg(lowerType(yo.Wrapped), sil.Unowned)
	yFull := g.rvalue(ye)
	if yFull == nil {
		return nil
	}
	g.blk.SwitchEnum(yFull,
		sil.Case{Member: optionalSome, Dest: ySome},
		sil.Case{Member: optionalNone, Dest: yNone})

	g.blk = ySome
	same := g.comparePayloads(e, xo.Wrapped, op, xPayload, yPayload)
	if same == nil {
		return nil
	}
	g.blk.Br(join, same)

	g.blk = yNone
	g.blk.Br(join, g.blk.IntegerLiteral(bit, unequal))

	// The left holds nothing, so the answer is whether the right
	// holds nothing too.
	g.blk = xNone
	emptySome, emptyNone := g.fn.Block(), g.fn.Block()
	emptySome.Arg(lowerType(yo.Wrapped), sil.Unowned)
	yEmpty := g.rvalue(ye)
	if yEmpty == nil {
		return nil
	}
	g.blk.SwitchEnum(yEmpty,
		sil.Case{Member: optionalSome, Dest: emptySome},
		sil.Case{Member: optionalNone, Dest: emptyNone})

	g.blk = emptySome
	g.blk.Br(join, g.blk.IntegerLiteral(bit, unequal))
	g.blk = emptyNone
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
func (g *gen) comparePayloads(at ast.Node, wrapped types.Type, op string,
	lhs, rhs *sil.Value) *sil.Value {
	v := g.operate(at, op, wrapped, types.Typ[types.Bool], lhs, rhs)
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
