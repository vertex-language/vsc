package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Optionals.
//
// `Int32?` is an enum with two cases, and Swift writes it as one:
// SILGen builds a value with `enum $Optional<Int32>, #Optional.some`
// and reads it back by matching on the case. So does this. What the
// bytes are is a question for the other end -- see lower's
// optionalImage, which is where `{ Int32, UInt8 }` and the tag byte
// live.
//
// Two things have to happen here that do not happen for a struct.
//
// `nil` is a case rather than a value: it has no bits of its own and
// its type comes from what it is being written into, which the
// checker has already decided by the time this sees it.
//
// And a value written where an optional is wanted is wrapped on the
// way in. `let a: Int32? = 7` is `.some(7)`, and nothing in the
// source says so -- Swift calls it an injection, SILGen emits it, and
// a compiler that passed the seven along unwrapped would hand the
// callee four bytes of payload and a tag byte of whatever followed.

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
func (g *gen) nilValue(e ast.Expr) (*vil.Value, bool) {
	t := g.typeOf(e)
	if _, ok := optionalOf(t); !ok {
		return nil, false
	}
	return g.blk.Enum(lowerType(t), optionalNone, nil), true
}

// optionalFor wraps a value on its way into an optional, or hands
// back what it was given when no wrapping is called for.
//
// Only one level. `Int32?` from an Int32 is a case with a payload;
// `Int32??` from an Int32 would be two of them, and what the second
// one's bytes are is a question this compiler has not answered -- see
// lower's optionalImage, which knows the one-tag-byte shape and not
// what swiftc does with a second.
func (g *gen) optionalFor(at ast.Node, v *vil.Value, from, to types.Type) *vil.Value {
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

// ifLet lowers an `if` whose condition binds an optional, and reports
// whether the condition was one.
//
// SILGen writes it as a switch on the case rather than a test of a
// bit, which is what it is: `if let x = v` asks which case v holds
// and, in the arm where it holds something, names it.
//
//	switch_enum %v, case #Optional.some: bb1, case #Optional.none: bb2
//	bb1(%x : $Int32):
//
// The payload arrives as the block's argument, which is where the
// binding gets its value -- so `x` is a block argument and not a
// load, and nothing has to hold the optional open across the arm.
func (g *gen) ifLet(s *ast.IfStmt) bool {
	if len(s.Conds) != 1 {
		return false
	}
	b, ok := s.Conds[0].(*ast.OptionalBinding)
	if !ok {
		return false
	}
	subject := b.Value
	if subject == nil {
		// `if let x` with nothing after it rebinds the name it
		// shadows, which needs the outer binding rather than an
		// expression to lower.
		g.refuse(b, "a binding condition with no value")
		return true
	}
	o, isOpt := optionalOf(g.typeOf(subject))
	if !isOpt {
		g.refuse(b, "a binding condition on something that is not an optional")
		return true
	}
	// The payload has to fit in one block argument. A wider one --
	// `Pair?`, which is two registers -- would arrive as several, and
	// what a block argument is here is one register.
	// A payload that owns something is a payload whose lifetime the
	// arm has to account for: the binding holds it, the optional it
	// came out of still holds it, and one of them has to let go. That
	// is what the ownership pass is for and this does not arrange it
	// yet, so it is refused rather than left to the verifier -- which
	// is where it surfaced, as a value consumed on some paths.
	if !lowerType(o.Wrapped).Trivial() {
		g.refuse(b, "a binding condition on an optional of "+o.Wrapped.String()+
			", which owns what it holds")
		return true
	}
	if !oneRegister(o.Wrapped) {
		g.refuse(b, "a binding condition on an optional of "+o.Wrapped.String()+
			", whose payload is more than one register")
		return true
	}

	v := g.rvalue(subject)
	if v == nil {
		return true
	}

	some := g.fn.Block()
	none := g.fn.Block()
	payload := some.Arg(lowerType(o.Wrapped), vil.Unowned)
	g.blk.SwitchEnum(v,
		vil.Case{Member: optionalSome, Dest: some},
		vil.Case{Member: optionalNone, Dest: none})

	// The arm where it holds something, with the name bound to it.
	g.blk = some
	g.push()
	if _, sym := g.binding(&ast.PatternBinding{Pat: b.Pat}); sym != nil {
		g.locals[sym] = &local{value: payload, typ: lowerType(o.Wrapped)}
	}
	g.block(s.Body)
	g.pop()
	someOpen := g.blk != nil && g.blk.Term() == nil
	someEnd := g.blk

	g.blk = none
	if s.Else != nil {
		g.stmt(s.Else)
	}
	noneOpen := g.blk != nil && g.blk.Term() == nil
	noneEnd := g.blk

	switch {
	case !someOpen && !noneOpen:
		g.blk = noneEnd
	default:
		join := g.fn.Block()
		if someOpen {
			someEnd.Br(join)
		}
		if noneOpen {
			noneEnd.Br(join)
		}
		g.blk = join
	}
	return true
}

// oneRegister reports whether a value of this type is held in a
// single register, which is what a block argument may be.
//
// A struct of several fields is several, and the arm of an `if let`
// would have to take them all -- which is a shape this does not build
// yet. Everything else here is one: a scalar, a reference, a function
// value, a struct that wraps one of those.
func oneRegister(t types.Type) bool {
	if t == nil {
		return false
	}
	switch n := t.Underlying().(type) {
	case *types.Struct:
		stored := 0
		var only types.Type
		for _, f := range n.Fields {
			if f == nil {
				continue
			}
			stored++
			only = f.Type
		}
		if stored == 0 {
			return false
		}
		return stored == 1 && oneRegister(only)
	case *types.Tuple:
		return false
	}
	return true
}

// nilCoalescing lowers `a ?? b`: what a holds where it holds
// something, and b where it does not.
//
// The conditional operator's shape, asked of the case rather than of
// a bit. switch_enum says which case the optional holds; the some arm
// carries the payload as its block argument and hands it to the join;
// the none arm evaluates the right operand and hands over that. Only
// the arm that runs evaluates, which is what the @autoclosure on
// Swift's right operand promises -- `a ?? expensive()` does not call
// it where a holds something.
//
// core declares no operator for `??`, so nothing resolves it and the
// checker gives the expression its type directly. That is why this is
// reached from the branch where there is no symbol rather than
// through operate.
func (g *gen) nilCoalescing(e *ast.BinaryExpr) *vil.Value {
	o, isOpt := optionalOf(g.typeOf(e.X))
	if !isOpt {
		g.refuse(e, "'??' on something that is not an optional")
		return nil
	}
	// `a ?? b` where b is optional too answers an optional, which is
	// Swift's other overload and a different shape: the some arm
	// would have to wrap its payload again rather than hand it over.
	if _, rhsOptional := optionalOf(g.typeOf(e.Y)); rhsOptional {
		g.refuse(e, "'??' with an optional on the right, which answers an optional")
		return nil
	}
	// The same two limits a binding condition has, for the same
	// reasons: the payload arrives as one block argument, and one
	// that owns what it holds needs a lifetime this does not arrange.
	wrapped := lowerType(o.Wrapped)
	if !wrapped.Trivial() {
		g.refuse(e, "'??' on an optional of "+o.Wrapped.String()+
			", which owns what it holds")
		return nil
	}
	if !oneRegister(o.Wrapped) {
		g.refuse(e, "'??' on an optional of "+o.Wrapped.String()+
			", whose payload is more than one register")
		return nil
	}

	v := g.rvalue(e.X)
	if v == nil {
		return nil
	}
	result := lowerType(g.typeOf(e))

	some := g.fn.Block()
	none := g.fn.Block()
	join := g.fn.Block()
	payload := some.Arg(wrapped, vil.Unowned)
	answer := join.Arg(result, joinOwnership(result))

	g.blk.SwitchEnum(v,
		vil.Case{Member: optionalSome, Dest: some},
		vil.Case{Member: optionalNone, Dest: none})

	g.blk = some
	g.blk.Br(join, payload)

	g.blk = none
	fallback := g.rvalue(e.Y)
	if fallback == nil {
		return nil
	}
	g.blk.Br(join, fallback)

	g.blk = join
	// Exactly one arm ran, so the join owns one value and destroys it
	// once, where the scope holding it ends.
	g.destroyLater(answer)
	return answer
}

// forceUnwrap lowers `o!`: what the optional holds, or a trap where
// it holds nothing.
//
// switch_enum says which case it is. The some arm carries the payload
// as its block argument and is where everything after the `!`
// continues; the none arm traps, which is what Swift does and what
// makes `!` the assertion it is rather than a conversion.
//
// There is no join: the none arm does not come back, so the value is
// the some arm's argument and the block after it is the some arm.
func (g *gen) forceUnwrap(e *ast.ForceExpr) *vil.Value {
	o, isOptional := optionalOf(g.typeOf(e.X))
	if !isOptional {
		g.refuse(e, "'!' on something that is not an optional")
		return nil
	}
	// The same two limits a binding condition has, for the same
	// reasons: the payload arrives as one block argument, and one
	// that owns what it holds needs a lifetime this does not arrange.
	wrapped := lowerType(o.Wrapped)
	if !wrapped.Trivial() {
		g.refuse(e, "'!' on an optional of "+o.Wrapped.String()+
			", which owns what it holds")
		return nil
	}
	if !oneRegister(o.Wrapped) {
		g.refuse(e, "'!' on an optional of "+o.Wrapped.String()+
			", whose payload is more than one register")
		return nil
	}

	v := g.rvalue(e.X)
	if v == nil {
		return nil
	}
	some := g.fn.Block()
	none := g.fn.Block()
	payload := some.Arg(wrapped, vil.Unowned)
	g.blk.SwitchEnum(v,
		vil.Case{Member: optionalSome, Dest: some},
		vil.Case{Member: optionalNone, Dest: none})

	// Nothing to test: arriving here is the failure. cond_fail takes
	// a condition all the same, so it is given one that holds.
	g.blk = none
	always := g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), -1)
	g.blk.CondFail(always, "Unexpectedly found nil while unwrapping an Optional value")
	g.blk.Unreachable()

	g.blk = some
	return payload
}

// nilComparison lowers `o == nil` and `o != nil`, and reports whether
// the expression was one.
//
// switch_enum over the case, each arm handing the join the answer for
// that case: `== nil` is true where it holds nothing, and `!= nil` is
// the other way round. Which is the same shape as `??` and `!`, and
// for the same reason -- what an optional is, is which case it holds.
func (g *gen) nilComparison(e *ast.BinaryExpr, op string) (*vil.Value, bool) {
	if op != "==" && op != "!=" {
		return nil, false
	}
	subject, ok := e.X, false
	if isNilLiteral(g.fold(e.Y)) {
		subject, ok = e.X, true
	} else if isNilLiteral(g.fold(e.X)) {
		subject, ok = e.Y, true
	}
	if !ok {
		return nil, false
	}
	if _, isOptional := optionalOf(g.typeOf(subject)); !isOptional {
		return nil, false
	}
	v := g.rvalue(subject)
	if v == nil {
		return nil, true
	}

	o, _ := optionalOf(g.typeOf(subject))
	bit := vil.Object(vil.BuiltinInt1)
	some := g.fn.Block()
	none := g.fn.Block()
	join := g.fn.Block()
	answer := join.Arg(bit, vil.None)
	// The some arm declares the payload even though the answer does
	// not read it: the case's edge hands it over, and an arm that
	// takes nothing is an arity the branch does not match.
	some.Arg(lowerType(o.Wrapped), vil.Unowned)
	g.blk.SwitchEnum(v,
		vil.Case{Member: optionalSome, Dest: some},
		vil.Case{Member: optionalNone, Dest: none})

	yes, no := int64(-1), int64(0)
	if op == "!=" {
		yes, no = 0, -1
	}
	g.blk = some
	g.blk.Br(join, g.blk.IntegerLiteral(bit, no))
	g.blk = none
	g.blk.Br(join, g.blk.IntegerLiteral(bit, yes))

	g.blk = join
	return answer, true
}

// isNilLiteral reports whether an expression is the literal nil.
func isNilLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.NIL
}

// optionalChain lowers `p?.x`: the member where p holds something,
// and nothing where it does not.
//
// The same switch as everything else about an optional, with the
// member read inside the some arm -- which is what makes the chain a
// chain: the read only happens where there is something to read it
// from. Both arms hand the join an optional, because that is what a
// chain answers whatever the member is.
func (g *gen) optionalChain(e *ast.MemberExpr, opt *ast.OptionalExpr) *vil.Value {
	o, isOptional := optionalOf(g.typeOf(opt.X))
	if !isOptional {
		return nil
	}
	result := g.typeOf(e)
	if _, answersOptional := optionalOf(result); !answersOptional {
		return nil
	}
	// The same two limits every switch over an optional has, and they
	// are the optional's rather than the block argument's: lower
	// takes a block argument of several registers, and does not take
	// an optional whose payload is more than one -- optionalImage
	// knows the one-tag-byte shape and no other.
	wrapped := lowerType(o.Wrapped)
	if !wrapped.Trivial() {
		g.refuse(e, "a chain through an optional of "+o.Wrapped.String()+
			", which owns what it holds")
		return nil
	}
	if !oneRegister(o.Wrapped) {
		g.refuse(e, "a chain through an optional of "+o.Wrapped.String()+
			", whose payload is more than one register")
		return nil
	}
	field, isStored := storedField(o.Wrapped, g.text(e.Name))
	if !isStored {
		// A computed property through a chain is a call that only
		// happens on one arm, which is more than this arranges.
		g.refuse(e, "a chain to '"+g.text(e.Name)+"', which is not a stored property")
		return nil
	}

	v := g.rvalue(opt.X)
	if v == nil {
		return nil
	}
	rt := lowerType(result)
	some := g.fn.Block()
	none := g.fn.Block()
	join := g.fn.Block()
	payload := some.Arg(wrapped, vil.Unowned)
	answer := join.Arg(rt, joinOwnership(rt))
	g.blk.SwitchEnum(v,
		vil.Case{Member: optionalSome, Dest: some},
		vil.Case{Member: optionalNone, Dest: none})

	g.blk = some
	member := memberName(o.Wrapped, field.Name)
	ft := lowerType(field.Type)
	var read *vil.Value
	if isClass(o.Wrapped) {
		addr := g.blk.RefElementAddr(payload, member, ft)
		access := g.blk.BeginAccess(addr, "read", "dynamic")
		read = g.loaded(g.blk.Load(access, loadQualifier(ft)), ft)
		g.blk.EndAccess(access)
	} else {
		read = g.blk.StructExtract(payload, member, ft)
	}
	g.blk.Br(join, g.blk.Enum(rt, optionalSome, read))

	g.blk = none
	g.blk.Br(join, g.blk.Enum(rt, optionalNone, nil))

	g.blk = join
	g.destroyLater(answer)
	return answer
}
