package gen

import (
	"github.com/vertex-language/vsc/ast"
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
